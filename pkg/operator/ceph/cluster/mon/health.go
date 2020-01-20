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
	"strings"
	"time"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	cephutil "github.com/rook/rook/pkg/daemon/ceph/util"
	cephspec "github.com/rook/rook/pkg/operator/ceph/spec"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
				logger.Warningf("failed to check mon health. %v", err)
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
	quorumStatus, err := client.GetMonQuorumStatus(c.context, c.ClusterInfo.Name, true)
	if err != nil {
		return errors.Wrapf(err, "failed to get mon quorum status")
	}
	logger.Debugf("Mon quorum status: %+v", quorumStatus)
	if c.spec.External.Enable {
		return c.handleExternalMonStatus(quorumStatus)
	}

	// Use a local mon count in case the user updates the crd in another goroutine.
	// We need to complete a health check with a consistent value.
	desiredMonCount := c.spec.Mon.Count
	logger.Debugf("targeting the mon count %d", desiredMonCount)

	// Source of truth of which mons should exist is our *clusterInfo*
	monsNotFound := map[string]interface{}{}
	for _, mon := range c.ClusterInfo.Monitors {
		monsNotFound[mon.Name] = struct{}{}
	}

	// first handle mons that are not in quorum but in the ceph mon map
	// failover the unhealthy mons
	allMonsInQuorum := true
	for _, mon := range quorumStatus.MonMap.Mons {
		inQuorum := monInQuorum(mon, quorumStatus.Quorum)
		// if the mon is in quorum remove it from our check for "existence"
		// else see below condition
		if _, ok := monsNotFound[mon.Name]; ok {
			delete(monsNotFound, mon.Name)
		} else {
			// when the mon isn't in the clusterInfo, but is in quorum and there are
			// enough mons, remove it else remove it on the next run
			if inQuorum && len(quorumStatus.MonMap.Mons) > desiredMonCount {
				logger.Warningf("mon %s not in source of truth but in quorum, removing", mon.Name)
				c.removeMon(mon.Name)
			} else {
				logger.Warningf(
					"mon %s not in source of truth and not in quorum, not enough mons to remove now (wanted: %d, current: %d)",
					mon.Name,
					desiredMonCount,
					len(quorumStatus.MonMap.Mons),
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
			logger.Debugf("mon %s NOT found in quorum. Mon quorum status: %+v", mon.Name, quorumStatus)
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
			c.failMon(len(quorumStatus.MonMap.Mons), desiredMonCount, mon.Name)
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

	// create/start new mons when there are fewer mons than the desired count in the CRD
	if len(quorumStatus.MonMap.Mons) < desiredMonCount {
		logger.Infof("adding mons. currently %d mons are in quorum and the desired count is %d.", len(quorumStatus.MonMap.Mons), desiredMonCount)
		return c.startMons(desiredMonCount)
	}

	// remove extra mons if the desired count has decreased in the CRD and all the mons are currently healthy
	if allMonsInQuorum && len(quorumStatus.MonMap.Mons) > desiredMonCount {
		if desiredMonCount < 2 && len(quorumStatus.MonMap.Mons) == 2 {
			logger.Warningf("cannot reduce mon quorum size from 2 to 1")
		} else {
			logger.Infof("removing an extra mon. currently %d are in quorum and only %d are desired", len(quorumStatus.MonMap.Mons), desiredMonCount)
			return c.removeMon(quorumStatus.MonMap.Mons[0].Name)
		}
	}

	return nil
}

// failMon compares the monCount against desiredMonCount
func (c *Cluster) failMon(monCount, desiredMonCount int, name string) {
	if monCount > desiredMonCount {
		// no need to create a new mon since we have an extra
		if err := c.removeMon(name); err != nil {
			logger.Errorf("failed to remove mon %q. %v", name, err)
		}
	} else {
		// bring up a new mon to replace the unhealthy mon
		if err := c.failoverMon(name); err != nil {
			logger.Errorf("failed to failover mon %q. %v", name, err)
		}
	}
}

func (c *Cluster) failoverMon(name string) error {
	logger.Infof("Failing over monitor %q", name)

	// Start a new monitor
	m := c.newMonConfig(c.maxMonID + 1)
	logger.Infof("starting new mon: %+v", m)

	mConf := []*monConfig{m}

	// Assign the pod to a node
	if err := c.assignMons(mConf); err != nil {
		return errors.Wrapf(err, "failed to place new mon on a node")
	}

	if c.Network.IsHost() {
		node, ok := c.mapping.Node[m.DaemonName]
		if !ok {
			return errors.Errorf("mon %s doesn't exist in assignment map", m.DaemonName)
		}
		m.PublicIP = node.Address
	} else {
		// Create the service endpoint
		serviceIP, err := c.createService(m)
		if err != nil {
			return errors.Wrapf(err, "failed to create mon service")
		}
		m.PublicIP = serviceIP
	}
	c.ClusterInfo.Monitors[m.DaemonName] = cephconfig.NewMonInfo(m.DaemonName, m.PublicIP, m.Port)

	// Start the deployment
	if err := c.startDeployments(mConf, true); err != nil {
		return errors.Wrapf(err, "failed to start new mon %s", m.DaemonName)
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
		if kerrors.IsNotFound(err) {
			logger.Infof("dead mon %s was already gone", resourceName)
		} else {
			return errors.Wrapf(err, "failed to remove dead mon deployment %s", resourceName)
		}
	}

	// Remove the bad monitor from quorum
	if err := removeMonitorFromQuorum(c.context, c.ClusterInfo.Name, daemonName); err != nil {
		return errors.Wrapf(err, "failed to remove mon %s from quorum", daemonName)
	}
	delete(c.ClusterInfo.Monitors, daemonName)
	// check if a mapping exists for the mon
	if _, ok := c.mapping.Node[daemonName]; ok {
		delete(c.mapping.Node, daemonName)
	}

	// Remove the service endpoint
	if err := c.context.Clientset.CoreV1().Services(c.Namespace).Delete(resourceName, options); err != nil {
		if kerrors.IsNotFound(err) {
			logger.Infof("dead mon service %s was already gone", resourceName)
		} else {
			return errors.Wrapf(err, "failed to remove dead mon service %s", resourceName)
		}
	}

	if err := c.saveMonConfig(); err != nil {
		return errors.Wrapf(err, "failed to save mon config after failing over mon %s", daemonName)
	}

	return nil
}

func removeMonitorFromQuorum(context *clusterd.Context, clusterName, name string) error {
	logger.Debugf("removing monitor %s", name)
	args := []string{"mon", "remove", name}
	if _, err := client.NewCephCommand(context, clusterName, args).Run(); err != nil {
		return errors.Wrapf(err, "mon %s remove failed", name)
	}

	logger.Infof("removed monitor %s", name)
	return nil
}

func (c *Cluster) handleExternalMonStatus(status client.MonStatusResponse) error {
	// We don't need to validate Ceph version if no image is present
	if c.spec.CephVersion.Image == "" {
		_, err := cephspec.ValidateCephVersionsBetweenLocalAndExternalClusters(c.context, c.Namespace, c.ClusterInfo.CephVersion)
		if err != nil {
			return errors.Wrapf(err, "failed to validate external ceph version")
		}
	}

	changed, err := c.addOrRemoveExternalMonitor(status)
	if err != nil {
		return errors.Wrapf(err, "failed to add or remove external mon")
	}

	// let's save the monitor's config if anything happened
	if changed {
		if err := c.saveMonConfig(); err != nil {
			return errors.Wrapf(err, "failed to save mon config after adding/removing external mon")
		}
	}

	return nil
}

func (c *Cluster) addOrRemoveExternalMonitor(status client.MonStatusResponse) (bool, error) {
	var changed bool
	oldClusterInfoMonitors := map[string]*cephconfig.MonInfo{}
	// clearing the content of clusterinfo monitors
	// and populate oldClusterInfoMonitors with monitors from clusterinfo
	// later c.ClusterInfo.Monitors get populated again
	for monName, mon := range c.ClusterInfo.Monitors {
		oldClusterInfoMonitors[mon.Name] = mon
		delete(c.ClusterInfo.Monitors, monName)
	}
	logger.Debugf("ClusterInfo is now Empty, refilling it from status.MonMap.Mons")

	monCount := len(status.MonMap.Mons)
	if monCount%2 == 0 {
		logger.Warningf("external cluster mon count is even (%d), should be uneven, continuing.", monCount)
	}

	if monCount == 1 {
		logger.Warning("external cluster mon count is 1, consider adding new monitors.")
	}

	// Iterate over the mons first and compare it with ClusterInfo
	for _, mon := range status.MonMap.Mons {
		inQuorum := monInQuorum(mon, status.Quorum)
		// if the mon was not in clusterInfo
		if _, ok := oldClusterInfoMonitors[mon.Name]; !ok {
			// If the mon is part of the quorum
			if inQuorum {
				// let's add it to ClusterInfo
				// FYI mon.PublicAddr is "10.97.171.131:6789/0"
				// so we need to remove '/0'
				endpointSlash := strings.Split(mon.PublicAddr, "/")
				endpoint := endpointSlash[0]

				// find IP and Port of that Mon
				monIP := cephutil.GetIPFromEndpoint(endpoint)
				monPort := cephutil.GetPortFromEndpoint(endpoint)
				logger.Infof("new external mon %q found: %s, adding it", mon.Name, endpoint)
				c.ClusterInfo.Monitors[mon.Name] = cephconfig.NewMonInfo(mon.Name, monIP, monPort)
			} else {
				logger.Debugf("mon %q is not in quorum and not in ClusterInfo", mon.Name)
			}
			changed = true
		} else {
			// mon is in ClusterInfo
			logger.Debugf("mon %q is in ClusterInfo, let's test if it's in quorum", mon.Name)
			if !inQuorum {
				// this mon was in clusterInfo but is not part of the quorum anymore
				// thus don't add it again to ClusterInfo
				logger.Infof("monitor %q is not part of the external cluster monitor quorum, removing it", mon.Name)
				changed = true
			} else {
				// this mon was in clusterInfo and is still in the quorum
				// add it again
				c.ClusterInfo.Monitors[mon.Name] = oldClusterInfoMonitors[mon.Name]
				logger.Debugf("everything is fine mon %q in the clusterInfo and its quorum status is %v", mon.Name, inQuorum)
			}
		}
	}
	// compare old clusterInfo with new ClusterInfo
	// if length differ -> the are different
	// then check if all elements are the same
	if len(oldClusterInfoMonitors) != len(c.ClusterInfo.Monitors) {
		changed = true
	} else {
		for _, mon := range c.ClusterInfo.Monitors {
			if old, ok := oldClusterInfoMonitors[mon.Name]; !ok || *old != *mon {
				changed = true
			}
		}
	}

	logger.Debugf("ClusterInfo.Monitors is %+v", c.ClusterInfo.Monitors)
	return changed, nil
}
