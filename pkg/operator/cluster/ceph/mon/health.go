/*
Copyright 2016 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package mon for the Ceph monitors.
package mon

import (
	"fmt"
	"time"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/daemon/ceph/mon"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	// HealthCheckInterval interval to check the mons to be in quorum
	HealthCheckInterval = 45 * time.Second
	// MonOutTimeout the duration to wait before removing/failover to a new mon pod
	MonOutTimeout = 300 * time.Second
)

// HealthChecker check health for the monitors
type HealthChecker struct {
	monCluster *Cluster
}

// NewHealthChecker creates a new HealthChecker object
func NewHealthChecker(monCluster *Cluster) *HealthChecker {
	return &HealthChecker{
		monCluster: monCluster,
	}
}

// Check periodically the health of the monitors
func (hc *HealthChecker) Check(stopCh chan struct{}) {
	for {
		select {
		case <-stopCh:
			logger.Infof("Stopping monitoring of cluster in namespace %s", hc.monCluster.Namespace)
			return

		case <-time.After(HealthCheckInterval):
			logger.Debugf("checking health of mons")
			err := hc.monCluster.checkHealth()
			if err != nil {
				logger.Infof("failed to check mon health. %+v", err)
			}
		}
	}
}

func (c *Cluster) checkHealth() error {
	logger.Debugf("Checking health for mons. %+v", c.clusterInfo)

	// connect to the mons
	// get the status and check for quorum
	status, err := client.GetMonStatus(c.context, c.clusterInfo.Name, true)
	if err != nil {
		return fmt.Errorf("failed to get mon status. %+v", err)
	}
	logger.Debugf("Mon status: %+v", status)

	// source of thruth of which mons should exist is our *clusterInfo*
	monsTruth := map[string]interface{}{}
	for _, mon := range c.clusterInfo.Monitors {
		monsTruth[mon.Name] = struct{}{}
	}

	// first handle mons that are not in quorum but in the cecph mon map
	// failover the unhealthy mons
	for _, mon := range status.MonMap.Mons {
		if _, ok := monsTruth[mon.Name]; !ok {
			logger.Warningf("mon %s is not in source of truth but in mon map, trying to remove", mon.Name)
			return c.removeMon(mon.Name)
		}

		// all mons below this line are in the source of truth, remove them from
		// the list as below we remove the mons that remained (not in quorum)
		inQuorum := monInQuorum(mon, status.Quorum)
		if inQuorum {
			logger.Debugf("mon %s found in quorum", mon.Name)
			// delete the "timeout" for a mon if the pod is in quorum again
			if _, ok := c.monTimeoutList[mon.Name]; ok {
				delete(c.monTimeoutList, mon.Name)
				logger.Infof("mon %s is back in quorum again", mon.Name)
			}
		} else {
			logger.Warningf("mon %s NOT found in quorum. %+v", mon.Name, status)
			// only deal with one unhealthy mon per health check
			return c.failMonWithTimeout(len(status.MonMap.Mons), mon.Name)
		}
	}

	// check if mons are on valid nodes (aka placement check)
	done, err := c.checkMonsOnValidNodes()
	if done || err != nil {
		return err
	}

	// if there should be more mons than wanted by the user
	if len(c.clusterInfo.Monitors) > c.Size {
		// get first mon in monitor list and remove it from the node
		for monName := range c.clusterInfo.Monitors {
			logger.Debugf("too many mons running (current: %d, wanted: %d)", len(c.clusterInfo.Monitors), c.Size)
			return c.removeMon(monName)
		}
		// if we have less than wanted mons, start new mons
	} else if len(c.clusterInfo.Monitors) < c.Size {
		return c.startMons()
	}

	return nil
}

func (c *Cluster) checkMonsOnValidNodes() (bool, error) {
	for _, m := range c.clusterInfo.Monitors {
		monID, err := getMonID(m.Name)
		if err != nil {
			return false, err
		}
		nodeName := c.mapping.MonsToNodes[monID]
		// get node to use for validNode() func
		node, err := c.context.Clientset.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
		if err != nil {
			return true, err
		}
		monName := getMonNameForID(monID)

		// update node IPs for hostNetwork modewhen they have changed
		if c.HostNetwork {
			if err = c.checkMonNodeIP(monID, *node); err != nil {
				return false, err
			}
		}

		// check if node the mon is on is still valid
		if !validNode(*node, c.placement) {
			logger.Warningf("node %s isn't valid anymore, failover mon %s", nodeName, monName)
			c.failMon(len(c.clusterInfo.Monitors), monName)
			return true, nil
		}
		logger.Debugf("node %s with mon %s is still valid", nodeName, monName)
	}
	return false, nil
}

func (c *Cluster) checkMonNodeIP(monID int, node v1.Node) error {
	if ip, ok := c.mapping.Addresses[monID]; ok {
		currentIP, err := getNodeIPFromNode(node)
		if err != nil {
			return fmt.Errorf("failed to get IP from node %s. %+v", node.Name, err)
		}
		if ip != currentIP {
			c.mapping.Addresses[monID] = currentIP
			if err := c.saveConfigChanges(); err != nil {
				return fmt.Errorf("failed to save config after updating node %s IP for mon with ID %d. %+v", node.Name, monID, err)
			}
		}
	}
	return nil
}

// failMonWithTimeout takes the mon out timeout into account before failing a mon
func (c *Cluster) failMonWithTimeout(monCount int, name string) error {
	// If not yet set, add the current time, for the timeout
	// calculation, to the list
	if _, ok := c.monTimeoutList[name]; !ok {
		c.monTimeoutList[name] = time.Now()
	}

	// when the timeout for the mon has been reached, continue to the
	// normal failover/delete mon pod part of the code
	if time.Since(c.monTimeoutList[name]) <= MonOutTimeout {
		logger.Warningf("mon %s NOT found in quorum, STILL in mon out timeout", name)
		return nil
	}

	return c.failMon(monCount, name)
}

// failMon monCount is compared against c.Size (wanted mon count)
func (c *Cluster) failMon(monCount int, name string) error {
	// when the "quorum" allows removal of a mon, remove it else fail it over
	if checkQuorumConsensusForRemoval(monCount, 1) {
		logger.Debugf("removing mon as quorum ", monCount%2)
		// no need to create a new mon since we have a "spare" mon for some reasons..
		// example reason: changed monCount in Cluster CRD
		if err := c.removeMon(name); err != nil {
			return fmt.Errorf("failed to remove mon %s. %+v", name, err)
		}
	} else {
		// check if enough nodes are available, when not no changes are done until
		// enough nodes are available
		availableNodes, err := c.getAvailableMonNodes()
		if err != nil {
			return err
		}
		logger.Infof("found %d running nodes without mons", len(availableNodes))
		if len(availableNodes) == 0 {
			logger.Warningf("not enough nodes without mons available (available: %d)", len(availableNodes))
			return nil
		}

		// bring up a new mon to replace the unhealthy mon
		if err := c.failoverMon(name); err != nil {
			return fmt.Errorf("failed to failover mon %s. %+v", name, err)
		}
	}
	return nil
}

func (c *Cluster) failoverMon(name string) error {
	logger.Infof("failing over monitor %s", name)

	// Start a new monitor
	m := &monConfig{
		ID:   c.maxMonID + 1,
		Name: getMonNameForID(c.maxMonID + 1),
		Port: int32(mon.DefaultPort),
	}
	logger.Infof("starting new mon %s", m.Name)

	// Create the service endpoint
	serviceIP, err := c.createService(m)
	if err != nil {
		return fmt.Errorf("failed to create mon service. %+v", err)
	}

	if !c.HostNetwork {
		m.PublicIP = serviceIP
		c.clusterInfo.Monitors[m.Name] = mon.ToCephMon(m.Name, m.PublicIP, m.Port)
	}

	mConf := []*monConfig{m}

	// Assign the pod to a node
	if err = c.assignMons(mConf); err != nil {
		return fmt.Errorf("failed to assign pods to mons. %+v", err)
	}

	// Start the pod
	if err = c.startPods(mConf); err != nil {
		return fmt.Errorf("failed to start new mon %s. %+v", m.Name, err)
	}

	// Only increment the max mon id if the new pod started successfully
	c.maxMonID++

	return c.removeMon(name)
}

func (c *Cluster) removeMon(name string) error {
	logger.Infof("removing monitor %s", name)

	// Remove the mon pod if it is still there
	var gracePeriod int64
	propagation := metav1.DeletePropagationForeground
	options := &metav1.DeleteOptions{GracePeriodSeconds: &gracePeriod, PropagationPolicy: &propagation}
	if err := c.context.Clientset.Extensions().ReplicaSets(c.Namespace).Delete(name, options); err != nil {
		if errors.IsNotFound(err) {
			logger.Infof("dead mon %s was already gone", name)
		} else {
			return fmt.Errorf("failed to remove dead mon pod %s. %+v", name, err)
		}
	}

	// Remove the service endpoint
	if err := c.context.Clientset.CoreV1().Services(c.Namespace).Delete(name, options); err != nil {
		if errors.IsNotFound(err) {
			logger.Infof("dead mon service %s was already gone", name)
		} else {
			return fmt.Errorf("failed to remove dead mon pod %s. %+v", name, err)
		}
	}

	// Remove the bad monitor from quorum
	if err := removeMonitorFromQuorum(c.context, c.clusterInfo.Name, name); err != nil {
		return fmt.Errorf("failed to remove mon %s from quorum. %+v", name, err)
	}
	if _, ok := c.clusterInfo.Monitors[name]; ok {
		delete(c.clusterInfo.Monitors, name)
	}

	id, err := getMonID(name)
	if err != nil {
		return fmt.Errorf("failed to get mon id from name %s. %+v", name, err)
	}

	if c.HostNetwork {
		if _, ok := c.mapping.Addresses[id]; ok {
			delete(c.mapping.Addresses, id)
		} else {
			logger.Debugf("no address mapping for mon %s", name)
		}
	}

	if err := c.saveConfigChanges(); err != nil {
		return fmt.Errorf("error saving config after failing over mon %s. %+v", name, err)
	}

	return nil
}

func removeMonitorFromQuorum(context *clusterd.Context, clusterName, name string) error {
	logger.Debugf("removing monitor %s from quorum", name)
	args := []string{"mon", "remove", name}
	if _, err := client.ExecuteCephCommand(context, clusterName, args); err != nil {
		return fmt.Errorf("mon %s remove failed: %+v", name, err)
	}

	logger.Infof("removed monitor %s", name)
	return nil
}

func (c *Cluster) saveConfigChanges() error {
	if err := c.saveMonConfig(); err != nil {
		return fmt.Errorf("failed to save mon config. %+v", err)
	}

	// make sure to rewrite the config so NO new connections are made to the removed mon
	if err := WriteConnectionConfig(c.context, c.clusterInfo); err != nil {
		return fmt.Errorf("failed to write connection config. %+v", err)
	}

	return nil
}
