/*
Copyright 2018 The Rook Authors. All rights reserved.

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

package mon

import (
	"fmt"
	"time"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	scheduler "k8s.io/kubernetes/pkg/scheduler/api"
)

var (
	// HealthCheckInterval is the interval to check if the mons are in quorum
	HealthCheckInterval = 45 * time.Second
	// MonOutTimeout is the duration to wait before removing/failover to a new mon pod
	MonOutTimeout = 600 * time.Second
)

// HealthChecker aggregates the mon/cluster info needed to check the health of the monitors
type HealthChecker struct {
	monCluster  *Cluster
	clusterSpec *cephv1.ClusterSpec
}

// NewHealthChecker creates a new HealthChecker object
func NewHealthChecker(monCluster *Cluster, clusterSpec *cephv1.ClusterSpec) *HealthChecker {
	return &HealthChecker{
		monCluster:  monCluster,
		clusterSpec: clusterSpec,
	}
}

// Check periodically checks the health of the monitors
func (hc *HealthChecker) Check(stopCh chan struct{}) {
	// Populate spec with clusterSpec
	if hc.clusterSpec.External.Enable {
		hc.monCluster.spec = *hc.clusterSpec
	}

	for {
		select {
		case <-stopCh:
			logger.Infof("Stopping monitoring of mons in namespace %s", hc.monCluster.Namespace)
			return

		case <-time.After(HealthCheckInterval):
			logger.Debugf("checking health of mons")
			err := hc.monCluster.checkHealth()
			if err != nil {
				logger.Warningf("failed to check mon health. %+v", err)
			}
		}
	}
}

func (c *Cluster) checkHealth() error {
	c.acquireOrchestrationLock()
	defer c.releaseOrchestrationLock()

	logger.Debugf("Checking health for mons in cluster. %s", c.ClusterInfo.Name)

	// connect to the mons
	// get the status and check for quorum
	status, err := client.GetMonStatus(c.context, c.ClusterInfo.Name, true)
	if err != nil {
		return fmt.Errorf("failed to get mon status. %+v", err)
	}
	logger.Debugf("Mon status: %+v", status)
	if c.spec.External.Enable {
		return c.handleExternalMonStatus(status)
	}

	// Use a local mon count in case the user updates the crd in another goroutine.
	// We need to complete a health check with a consistent value.
	desiredMonCount, msg, err := c.getTargetMonCount()
	if err != nil {
		return fmt.Errorf("failed to get target mon count. %+v", err)
	}
	logger.Debugf(msg)

	// Source of truth of which mons should exist is our *clusterInfo*
	monsNotFound := map[string]interface{}{}
	for _, mon := range c.ClusterInfo.Monitors {
		monsNotFound[mon.Name] = struct{}{}
	}

	// first handle mons that are not in quorum but in the ceph mon map
	// failover the unhealthy mons
	allMonsInQuorum := true
	for _, mon := range status.MonMap.Mons {
		inQuorum := monInQuorum(mon, status.Quorum)
		// if the mon is in quorum remove it from our check for "existence"
		// else see below condition
		if _, ok := monsNotFound[mon.Name]; ok {
			delete(monsNotFound, mon.Name)
		} else {
			// when the mon isn't in the clusterInfo, but is in quorum and there are
			// enough mons, remove it else remove it on the next run
			if inQuorum && len(status.MonMap.Mons) > desiredMonCount {
				logger.Warningf("mon %s not in source of truth but in quorum, removing", mon.Name)
				c.removeMon(mon.Name)
			} else {
				logger.Warningf(
					"mon %s not in source of truth and not in quorum, not enough mons to remove now (wanted: %d, current: %d)",
					mon.Name,
					desiredMonCount,
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
			allMonsInQuorum = false

			// If not yet set, add the current time, for the timeout
			// calculation, to the list
			if _, ok := c.monTimeoutList[mon.Name]; !ok {
				c.monTimeoutList[mon.Name] = time.Now()
			}

			// when the timeout for the mon has been reached, continue to the
			// normal failover/delete mon pod part of the code
			if time.Since(c.monTimeoutList[mon.Name]) <= MonOutTimeout {
				logger.Warningf("mon %s not found in quorum, waiting for timeout before failover", mon.Name)
				continue
			}

			logger.Warningf("mon %s NOT found in quorum and timeout exceeded, mon will be failed over", mon.Name)
			c.failMon(len(status.MonMap.Mons), desiredMonCount, mon.Name)
			// only deal with one unhealthy mon per health check
			return nil
		}
	}

	// after all unhealthy mons have been removed/failovered
	// handle all mons that haven't been in the Ceph mon map
	for mon := range monsNotFound {
		logger.Warningf("mon %s NOT found in ceph mon map, failover", mon)
		c.failMon(len(c.ClusterInfo.Monitors), desiredMonCount, mon)
		// only deal with one "not found in ceph mon map" mon per health check
		return nil
	}

	// find any mons that invalidate our placement policy, and if necessary,
	// reschedule them to other nodes.
	done, err := c.resolveInvalidMonitorPlacement(desiredMonCount)
	if done || err != nil {
		return err
	}

	// create/start new mons when there are fewer mons than the desired count in the CRD
	if len(status.MonMap.Mons) < desiredMonCount {
		logger.Infof("adding mons. currently %d mons are in quorum and the desired count is %d.", len(status.MonMap.Mons), desiredMonCount)
		return c.startMons(desiredMonCount)
	}

	// remove extra mons if the desired count has decreased in the CRD and all the mons are currently healthy
	if allMonsInQuorum && len(status.MonMap.Mons) > desiredMonCount {
		if desiredMonCount < 2 && len(status.MonMap.Mons) == 2 {
			logger.Warningf("cannot reduce mon quorum size from 2 to 1")
		} else {
			logger.Infof("removing an extra mon. currently %d are in quorum and only %d are desired", len(status.MonMap.Mons), desiredMonCount)
			return c.removeMon(status.MonMap.Mons[0].Name)
		}
	}

	return nil
}

func (c *Cluster) checkMonsOnSameNode(desiredMonCount int) (bool, error) {
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
				c.failMon(len(c.ClusterInfo.Monitors), desiredMonCount, name)
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
		valid, err := k8sutil.ValidNode(*node, cephv1.GetMonPlacement(c.spec.Placement))
		if err != nil {
			logger.Warning("failed to validate node %s %v", node.Name, err)
		} else if !valid {
			logger.Warningf("node %s isn't valid anymore, failover mon %s", nInfo.Name, mon)
			c.failoverMon(mon)
			return true, nil
		}
		logger.Debugf("node %s with mon %s is still valid", nInfo.Name, mon)
	}
	return false, nil
}

// failMon compares the monCount against desiredMonCount
func (c *Cluster) failMon(monCount, desiredMonCount int, name string) {
	if monCount > desiredMonCount {
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
	m := c.newMonConfig(c.maxMonID + 1)
	logger.Infof("starting new mon: %+v", m)

	// Create the service endpoint
	serviceIP, err := c.createService(m)
	if err != nil {
		return fmt.Errorf("failed to create mon service. %+v", err)
	}

	mConf := []*monConfig{m}

	// Assign the pod to a node
	if err = c.assignMons(mConf); err != nil {
		return fmt.Errorf("failed to place new mon on a node. %+v", err)
	}

	if c.HostNetwork {
		node, ok := c.mapping.Node[m.DaemonName]
		if !ok {
			return fmt.Errorf("mon %s doesn't exist in assignment map", m.DaemonName)
		}
		m.PublicIP = node.Address
	} else {
		m.PublicIP = serviceIP
	}
	c.ClusterInfo.Monitors[m.DaemonName] = cephconfig.NewMonInfo(m.DaemonName, m.PublicIP, m.Port)

	// Start the deployment
	if err = c.startDeployments(mConf, true); err != nil {
		return fmt.Errorf("failed to start new mon %s. %+v", m.DaemonName, err)
	}

	// Only increment the max mon id if the new pod started successfully
	c.maxMonID++

	return c.removeMon(name)
}

func (c *Cluster) removeMon(daemonName string) error {
	logger.Infof("ensuring removal of unhealthy monitor %s", daemonName)

	resourceName := resourceName(daemonName)

	// Remove the mon pod if it is still there
	var gracePeriod int64
	propagation := metav1.DeletePropagationForeground
	options := &metav1.DeleteOptions{GracePeriodSeconds: &gracePeriod, PropagationPolicy: &propagation}
	if err := c.context.Clientset.AppsV1().Deployments(c.Namespace).Delete(resourceName, options); err != nil {
		if errors.IsNotFound(err) {
			logger.Infof("dead mon %s was already gone", resourceName)
		} else {
			return fmt.Errorf("failed to remove dead mon deployment %s. %+v", resourceName, err)
		}
	}

	// Remove the bad monitor from quorum
	if err := removeMonitorFromQuorum(c.context, c.ClusterInfo.Name, daemonName); err != nil {
		return fmt.Errorf("failed to remove mon %s from quorum. %+v", daemonName, err)
	}
	delete(c.ClusterInfo.Monitors, daemonName)
	// check if a mapping exists for the mon
	if _, ok := c.mapping.Node[daemonName]; ok {
		nodeName := c.mapping.Node[daemonName].Name
		delete(c.mapping.Node, daemonName)
		// if node->port "mapping" has been created, decrease or delete it
		if port, ok := c.mapping.Port[nodeName]; ok {
			if port == DefaultMsgr1Port {
				delete(c.mapping.Port, nodeName)
			}
			// don't clean up if a node port is higher than the default port, other
			// mons could be on the same node with > DefaultPort ports, decreasing could
			// cause port collisions
			// This can be solved by using a map[nodeName][]int32 for the ports to
			// even better check which ports are open for the HostNetwork mode
		}
	}

	// Remove the service endpoint
	if err := c.context.Clientset.CoreV1().Services(c.Namespace).Delete(resourceName, options); err != nil {
		if errors.IsNotFound(err) {
			logger.Infof("dead mon service %s was already gone", resourceName)
		} else {
			return fmt.Errorf("failed to remove dead mon service %s. %+v", resourceName, err)
		}
	}

	if err := c.saveMonConfig(); err != nil {
		return fmt.Errorf("failed to save mon config after failing over mon %s. %+v", daemonName, err)
	}

	// make sure to rewrite the config so NO new connections are made to the removed mon
	if err := WriteConnectionConfig(c.context, c.ClusterInfo); err != nil {
		return fmt.Errorf("failed to write connection config after failing over mon %s. %+v", daemonName, err)
	}

	return nil
}

func removeMonitorFromQuorum(context *clusterd.Context, clusterName, name string) error {
	logger.Debugf("removing monitor %s", name)
	args := []string{"mon", "remove", name}
	if _, err := client.NewCephCommand(context, clusterName, args).Run(); err != nil {
		return fmt.Errorf("mon %s remove failed: %+v", name, err)
	}

	logger.Infof("removed monitor %s", name)
	return nil
}

func (c *Cluster) resolveInvalidMonitorPlacement(desiredMonCount int) (bool, error) {
	nodeChoice, err := c.findInvalidMonitorPlacement(desiredMonCount)
	if err != nil {
		return true, fmt.Errorf("failed to find invalid mon placement %+v", err)
	}

	// no violation found
	if nodeChoice == nil {
		return false, nil
	}

	for name, nodeInfo := range c.mapping.Node {
		if nodeInfo.Name == nodeChoice.Node.Name {
			logger.Infof("rebalance: rescheduling mon %s from node %s", name, nodeInfo.Name)
			c.failMon(len(c.ClusterInfo.Monitors), desiredMonCount, name)
			return true, nil
		}
	}

	logger.Warningf("rebalance: no mon pod found on node %s", nodeChoice.Node.Name)

	return false, nil
}

func (c *Cluster) findInvalidMonitorPlacement(desiredMonCount int) (*NodeUsage, error) {
	nodeZones, err := c.getNodeMonUsage()
	if err != nil {
		return nil, fmt.Errorf("failed to get node monitor usage. %+v", err)
	}

	// compute two helpful global flags:
	//  - does an empty zone exist
	//  - does an empty node exist
	emptyZone := false
	emptyNode := false
	for zi := range nodeZones {
		monFoundInZone := false
		for ni := range nodeZones[zi] {
			nodeUsage := &nodeZones[zi][ni]
			if nodeUsage.MonCount > 0 {
				monFoundInZone = true
			} else if nodeUsage.MonValid {
				// only consider valid nodes since this flag determines if a mon
				// may be scheduled on to a new node.
				emptyNode = true
			}
		}
		if !monFoundInZone {
			// emptyZone is used below to imply that an empty zone exists AND
			// that zone contains a valid node (i.e. a node that new pods can be
			// assigned). this check enforces that assumption by checking that
			// the zone contains at least one valid node.
			validNodeInZone := false
			for _, nodeUsage := range nodeZones[zi] {
				if nodeUsage.MonValid {
					validNodeInZone = true
					break
				}
			}
			if validNodeInZone {
				emptyZone = true
			}
			logger.Debugf("rebalance: empty zone found. validNodeInZone: %t, emptyZone %t",
				validNodeInZone, emptyZone)
		}
	}

	logger.Debugf("rebalance: desired mon count: %d, empty zone found: %t, empty node found: %t",
		desiredMonCount, emptyZone, emptyNode)

	for zi := range nodeZones {
		zoneMonCount := 0
		var nodeChoice *NodeUsage

		// for each node consider two cases: too many monitors are running on a
		// node, or monitors are running on invalid nodes.
		for ni := range nodeZones[zi] {
			nodeUsage := &nodeZones[zi][ni]
			zoneMonCount += nodeUsage.MonCount

			// if this node has too many monitors, and an underused node exists,
			// then consider moving the monitor. note that this check is
			// independent of the setting `AllowMultiplePerNode` since we also
			// want to avoid in general keeping multiple monitors on one node.
			if nodeUsage.MonCount > 1 && emptyNode {
				logger.Infof("rebalance: chose overloaded node %s with %d mons",
					nodeUsage.Node.Name, nodeUsage.MonCount)
				nodeChoice = nodeUsage
				break
			}

			// check for mons on invalid nodes. but reschedule pod only when it
			// is invalid for reasons other than schedulability which should not
			// affect pods that are already scheduled.
			if nodeUsage.MonCount > 0 && !nodeUsage.MonValid {
				placement := cephv1.GetMonPlacement(c.spec.Placement)
				placement.Tolerations = append(placement.Tolerations,
					v1.Toleration{
						Key:      scheduler.TaintNodeUnschedulable,
						Effect:   "",
						Operator: v1.TolerationOpExists,
					})
				valid, err := k8sutil.ValidNodeNoSched(*nodeUsage.Node, placement)
				if err != nil {
					logger.Warningf("rebalance: failed to validate node %s %+v",
						nodeUsage.Node.Name, err)
				} else if !valid {
					logger.Infof("rebalance: chose invalid node %s",
						nodeUsage.Node.Name)
					nodeChoice = nodeUsage
					break
				}
			}
		}

		// if we didn't already select a node with a monitor for rescheduling,
		// then consider that a zone may be overloaded and can be rebalanced.
		// this case occurs if the zone has more than 1 monitor, and some other
		// zone exists without any monitors.
		if nodeChoice == nil && zoneMonCount > 1 && emptyZone {
			for ni := range nodeZones[zi] {
				nodeUsage := &nodeZones[zi][ni]
				if nodeUsage.MonCount > 0 {
					// select the node with the most monitors assigned
					if nodeChoice == nil || nodeUsage.MonCount > nodeChoice.MonCount {
						nodeChoice = nodeUsage
					}
				}
			}
			if nodeChoice != nil {
				logger.Infof("rebalance: chose node %s from overloaded zone",
					nodeChoice.Node.Name)
			}
		}

		if nodeChoice != nil {
			return nodeChoice, nil
		}
	}

	logger.Debugf("rebalance: no mon placement violations or fixes available")

	return nil, nil
}

func (c *Cluster) handleExternalMonStatus(status client.MonStatusResponse) error {
	return nil
}
