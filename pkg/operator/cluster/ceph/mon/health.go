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

	// Source of thruth of which mons should exist is our *clusterInfo*
	monsNotFound := map[string]interface{}{}
	for _, mon := range c.clusterInfo.Monitors {
		monsNotFound[mon.Name] = struct{}{}
	}

	// first handle mons that are not in quorum but in the cecph mon map
	//failover the unhealthy mons
	for _, mon := range status.MonMap.Mons {
		inQuorum := monInQuorum(mon, status.Quorum)
		// if the mon is in quorum remove it from our check for "existence"
		//else see below condition
		if _, ok := monsNotFound[mon.Name]; ok {
			delete(monsNotFound, mon.Name)
		} else {
			// when the mon isn't in the clusterInfo, but is in qorum and there are
			//enough mons, remove it else remove it on the next run
			if inQuorum && len(status.MonMap.Mons) > c.Size {
				logger.Warningf("mon %s not in source of truth but in quorum, removing", mon.Name)
				c.removeMon(mon.Name)
			} else {
				logger.Warningf(
					"mon %s not in source of truth and not in quorum, not enough mons to remove now (wanted: %d, current: %d)",
					mon.Name,
					c.Size,
					len(status.MonMap.Mons),
				)
			}
		}

		if inQuorum {
			logger.Debugf("mon %s found in quorum", mon.Name)
			// delete the "timeout" for a mon if the pod is in quorum again
			if _, ok := c.monTimeoutList[mon.Name]; ok {
				delete(c.monTimeoutList, mon.Name)
				logger.Infof("mon %s is back in quorum, removed from mon out timeout list", mon.Name)
			}
		} else {
			logger.Debugf("mon %s NOT found in quorum. Mon status: %+v", mon.Name, status)

			// If not yet set, add the current time, for the timeout
			// calculation, to the list
			if _, ok := c.monTimeoutList[mon.Name]; !ok {
				c.monTimeoutList[mon.Name] = time.Now()
			}

			// when the timeout for the mon has been reached, continue to the
			// normal failover/delete mon pod part of the code
			if time.Since(c.monTimeoutList[mon.Name]) <= MonOutTimeout {
				logger.Warningf("mon %s not found in quorum, still in mon out timeout", mon.Name)
				continue
			}

			logger.Warningf("mon %s NOT found in quorum and timeout exceeded, mon will be failed over", mon.Name)
			c.failMon(len(status.MonMap.Mons), mon.Name)
			// only deal with one unhealthy mon per health check
			return nil
		}
	}

	// after all unhealthy mons have been removed/failovered
	//handle all mons that haven't been in the Ceph mon map
	for mon := range monsNotFound {
		logger.Warningf("mon %s NOT found in ceph mon map, failover", mon)
		c.failMon(len(c.clusterInfo.Monitors), mon)
		// only deal with one "not found in ceph mon map" mon per health check
		return nil
	}

	// check if there are more than two mons running on the same node, failover one mon in that case
	done, err := c.checkMonsOnSameNode()
	if done || err != nil {
		return err
	}

	done, err = c.checkMonsOnValidNodes()
	if done || err != nil {
		return err
	}

	return nil
}

func (c *Cluster) checkMonsOnSameNode() (bool, error) {
	nodesUsed := map[string]struct{}{}
	for name, node := range c.mapping.Node {
		// when the node is already in the list we have more than one mon on that node
		if _, ok := nodesUsed[node.Name]; ok {
			// get list of available nodes for mons
			availableNodes, _, err := c.getAvailableMonNodes()
			if err != nil {
				return true, fmt.Errorf("failed to get available mon nodes. %+v", err)
			}
			// if there are enough nodes for one mon "that is too much" to be failovered,
			// fail it over to an other node
			if len(availableNodes) > 0 {
				logger.Infof("rebalance: enough nodes available %d to failover mon %s", len(availableNodes), name)
				c.failMon(len(c.clusterInfo.Monitors), name)
			} else {
				logger.Debugf("rebalance: not enough nodes available to failover mon %s", name)
			}

			// deal with one mon too much on a node at a time
			return true, nil
		}
		nodesUsed[node.Name] = struct{}{}
	}
	return false, nil
}

func (c *Cluster) checkMonsOnValidNodes() (bool, error) {
	for mon, nInfo := range c.mapping.Node {
		// get node to use for validNode() func
		node, err := c.context.Clientset.CoreV1().Nodes().Get(nInfo.Name, metav1.GetOptions{})
		if err != nil {
			return true, err
		}
		// check if node the mon is on is still valid
		if !validNode(*node, c.placement) {
			logger.Warningf("node %s isn't valid anymore, failover mon %s", nInfo.Name, mon)
			c.failoverMon(mon)
			return true, nil
		}
		logger.Debugf("node %s with mon %s is still valid", nInfo.Name, mon)
	}
	return false, nil
}

// failMon monCount is compared against c.Size (wanted mon count)
func (c *Cluster) failMon(monCount int, name string) {
	if monCount > c.Size {
		// no need to create a new mon since we have an extra
		if err := c.removeMon(name); err != nil {
			logger.Errorf("failed to remove mon %s. %+v", name, err)
		}
	} else {
		// bring up a new mon to replace the unhealthy mon
		if err := c.failoverMon(name); err != nil {
			logger.Errorf("failed to failover mon %s. %+v", name, err)
		}
	}
}

func (c *Cluster) failoverMon(name string) error {
	logger.Infof("Failing over monitor %s", name)

	// Start a new monitor
	m := &monConfig{Name: fmt.Sprintf("%s%d", appName, c.maxMonID+1), Port: int32(mon.DefaultPort)}
	logger.Infof("starting new mon %s", m.Name)

	// Create the service endpoint
	serviceIP, err := c.createService(m)
	if err != nil {
		return fmt.Errorf("failed to create mon service. %+v", err)
	}

	mConf := []*monConfig{m}

	// Assign the pod to a node
	if err = c.assignMons(mConf); err != nil {
		return fmt.Errorf("failed to assign pods to mons. %+v", err)
	}

	if c.HostNetwork {
		node, ok := c.mapping.Node[m.Name]
		if !ok {
			return fmt.Errorf("mon %s doesn't exist in assignment map", m.Name)
		}
		m.PublicIP = node.Address
	} else {
		m.PublicIP = serviceIP
	}
	c.clusterInfo.Monitors[m.Name] = mon.ToCephMon(m.Name, m.PublicIP, m.Port)

	// Start the pod
	if err = c.startPods(mConf); err != nil {
		return fmt.Errorf("failed to start new mon %s. %+v", m.Name, err)
	}

	// Only increment the max mon id if the new pod started successfully
	c.maxMonID++

	return c.removeMon(name)
}

func (c *Cluster) removeMon(name string) error {
	logger.Infof("ensuring removal of unhealthy monitor %s", name)

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

	// Remove the bad monitor from quorum
	if err := removeMonitorFromQuorum(c.context, c.clusterInfo.Name, name); err != nil {
		return fmt.Errorf("failed to remove mon %s from quorum. %+v", name, err)
	}
	delete(c.clusterInfo.Monitors, name)
	// check if a mapping exists for the mon
	if _, ok := c.mapping.Node[name]; ok {
		nodeName := c.mapping.Node[name].Name
		delete(c.mapping.Node, name)
		// if node->port "mapping" has been created, decrease or delete it
		if port, ok := c.mapping.Port[nodeName]; ok {
			if port == mon.DefaultPort {
				delete(c.mapping.Port, nodeName)
			}
			// don't clean up if a node port is higher than the default port, other
			//mons could be on the same node with > DefaultPort ports, decreasing could
			//cause port collsions
			// This can be solved by using a map[nodeName][]int32 for the ports to
			//even better check which ports are open for the HostNetwork mode
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

	if err := c.saveMonConfig(); err != nil {
		return fmt.Errorf("failed to save mon config after failing over mon %s. %+v", name, err)
	}

	// make sure to rewrite the config so NO new connections are made to the removed mon
	if err := WriteConnectionConfig(c.context, c.clusterInfo); err != nil {
		return fmt.Errorf("failed to write connection config after failing over mon %s. %+v", name, err)
	}

	return nil
}

func removeMonitorFromQuorum(context *clusterd.Context, clusterName, name string) error {
	logger.Debugf("removing monitor %s", name)
	args := []string{"mon", "remove", name}
	if _, err := client.ExecuteCephCommand(context, clusterName, args); err != nil {
		return fmt.Errorf("mon %s remove failed: %+v", name, err)
	}

	logger.Infof("removed monitor %s", name)
	return nil
}
