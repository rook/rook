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
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	cephutil "github.com/rook/rook/pkg/daemon/ceph/util"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

var (
	// HealthCheckInterval is the interval to check if the mons are in quorum
	HealthCheckInterval = 45 * time.Second
	// MonOutTimeout is the duration to wait before removing/failover to a new mon pod
	MonOutTimeout = 10 * time.Minute

	retriesBeforeNodeDrainFailover = 1
	timeZero                       = time.Duration(0)
	// Check whether mons are on the same node once per operator restart since it's a rare scheduling condition
	needToCheckMonsOnSameNode = true
	// Version of Ceph where the arbiter failover is supported
	arbiterFailoverSupportedCephVersion = version.CephVersion{Major: 16, Minor: 2, Extra: 7}
)

// HealthChecker aggregates the mon/cluster info needed to check the health of the monitors
type HealthChecker struct {
	monCluster *Cluster
	interval   time.Duration
}

func updateMonTimeout(monCluster *Cluster) {
	// If the env was passed by the operator config, use that value
	// This is an old behavior where we maintain backward compatibility
	monTimeoutEnv := os.Getenv("ROOK_MON_OUT_TIMEOUT")
	if monTimeoutEnv != "" {
		parsedInterval, err := time.ParseDuration(monTimeoutEnv)
		// We ignore the error here since the default is 10min and it's unlikely to be a problem
		if err == nil {
			MonOutTimeout = parsedInterval
		}
		// No env var, let's use the CR value if any
	} else {
		monCRDTimeoutSetting := monCluster.spec.HealthCheck.DaemonHealth.Monitor.Timeout
		if monCRDTimeoutSetting != "" {
			if monTimeout, err := time.ParseDuration(monCRDTimeoutSetting); err == nil {
				if monTimeout == timeZero {
					logger.Warning("monitor failover is disabled")
				}
				MonOutTimeout = monTimeout
			}
		}
	}
	// A third case is when the CRD is not set, in which case we use the default from MonOutTimeout
}

func updateMonInterval(monCluster *Cluster, h *HealthChecker) {
	// If the env was passed by the operator config, use that value
	// This is an old behavior where we maintain backward compatibility
	healthCheckIntervalEnv := os.Getenv("ROOK_MON_HEALTHCHECK_INTERVAL")
	if healthCheckIntervalEnv != "" {
		parsedInterval, err := time.ParseDuration(healthCheckIntervalEnv)
		// We ignore the error here since the default is 45s and it's unlikely to be a problem
		if err == nil {
			h.interval = parsedInterval
		}
		// No env var, let's use the CR value if any
	} else {
		checkInterval := monCluster.spec.HealthCheck.DaemonHealth.Monitor.Interval
		// allow overriding the check interval
		if checkInterval != nil {
			logger.Debugf("ceph mon status in namespace %q check interval %q", monCluster.Namespace, checkInterval.Duration.String())
			h.interval = checkInterval.Duration
		}
	}
	// A third case is when the CRD is not set, in which case we use the default from HealthCheckInterval
}

// NewHealthChecker creates a new HealthChecker object
func NewHealthChecker(monCluster *Cluster) *HealthChecker {
	h := &HealthChecker{
		monCluster: monCluster,
		interval:   HealthCheckInterval,
	}
	return h
}

// Check periodically checks the health of the monitors
func (hc *HealthChecker) Check(monitoringRoutines map[string]*controller.ClusterHealth, daemon string) {
	for {
		// Update Mon Timeout with CR details
		updateMonTimeout(hc.monCluster)

		// Update Mon Interval with CR details
		updateMonInterval(hc.monCluster, hc)

		// We must perform this check otherwise the case will check an index that does not exist anymore and
		// we will get an invalid pointer error and the go routine will panic
		if _, ok := monitoringRoutines[daemon]; !ok {
			logger.Infof("ceph cluster %q has been deleted. stopping monitoring of mons", hc.monCluster.Namespace)
			return
		}

		select {
		case <-monitoringRoutines[daemon].InternalCtx.Done():
			logger.Infof("stopping monitoring of mons in namespace %q", hc.monCluster.Namespace)
			delete(monitoringRoutines, daemon)
			return

		case <-time.After(hc.interval):
			logger.Debugf("checking health of mons")
			err := hc.monCluster.checkHealth(monitoringRoutines[daemon].InternalCtx)
			if err != nil {
				logger.Warningf("failed to check mon health. %v", err)
			}
		}
	}
}

func (c *Cluster) checkHealth(ctx context.Context) error {
	c.acquireOrchestrationLock()
	defer c.releaseOrchestrationLock()

	// If cluster details are not initialized
	if !c.ClusterInfo.IsInitialized(true) {
		return errors.New("skipping mon health check since cluster details are not initialized")
	}

	// If the cluster is converged and no mons were specified
	if c.spec.Mon.Count == 0 && !c.spec.External.Enable {
		return errors.New("skipping mon health check since there are no monitors")
	}

	logger.Debugf("Checking health for mons in cluster %q", c.ClusterInfo.Namespace)

	// For an external connection we use a special function to get the status
	if c.spec.External.Enable {
		quorumStatus, err := cephclient.GetMonQuorumStatus(c.context, c.ClusterInfo)
		if err != nil {
			return errors.Wrap(err, "failed to get external mon quorum status")
		}

		err = c.handleExternalMonStatus(quorumStatus)
		if err != nil {
			return errors.Wrap(err, "failed to get external mon quorum status")
		}

		// handle active manager
		err = controller.ConfigureExternalMetricsEndpoint(c.context, c.spec.Monitoring, c.ClusterInfo, c.ownerInfo)
		if err != nil {
			return errors.Wrap(err, "failed to configure external metrics endpoint")
		}

		return nil
	}

	// connect to the mons
	// get the status and check for quorum
	quorumStatus, err := cephclient.GetMonQuorumStatus(c.context, c.ClusterInfo)
	if err != nil {
		return errors.Wrap(err, "failed to get mon quorum status")
	}
	logger.Debugf("Mon quorum status: %+v", quorumStatus)

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
			if inQuorum && len(quorumStatus.Quorum) > desiredMonCount {
				logger.Warningf("mon %q not in source of truth but in quorum, removing", mon.Name)
				if err := c.removeMon(mon.Name); err != nil {
					logger.Warningf("failed to remove mon %q. %v", mon.Name, err)
				}
				// only remove one extra mon per health check
				return nil
			}
			logger.Warningf(
				"mon %q not in source of truth and not in quorum, not enough mons to remove now (wanted: %d, current: %d)",
				mon.Name,
				desiredMonCount,
				len(quorumStatus.MonMap.Mons),
			)
		}

		if inQuorum {
			logger.Debugf("mon %q found in quorum", mon.Name)
			// delete the "timeout" for a mon if the pod is in quorum again
			if _, ok := c.monTimeoutList[mon.Name]; ok {
				delete(c.monTimeoutList, mon.Name)
				logger.Infof("mon %q is back in quorum, removed from mon out timeout list", mon.Name)
			}
			continue
		}

		logger.Debugf("mon %q NOT found in quorum. Mon quorum status: %+v", mon.Name, quorumStatus)

		// if the time out is set to 0 this indicate that we don't want to trigger mon failover
		if MonOutTimeout == timeZero {
			logger.Warningf("mon %q NOT found in quorum and health timeout is 0, mon will never fail over", mon.Name)
			return nil
		}

		allMonsInQuorum = false

		// If not yet set, add the current time, for the timeout
		// calculation, to the list
		if _, ok := c.monTimeoutList[mon.Name]; !ok {
			c.monTimeoutList[mon.Name] = time.Now()
		}

		// when the timeout for the mon has been reached, continue to the
		// normal failover mon pod part of the code
		if time.Since(c.monTimeoutList[mon.Name]) <= MonOutTimeout {
			timeToFailover := int(MonOutTimeout.Seconds() - time.Since(c.monTimeoutList[mon.Name]).Seconds())
			logger.Warningf("mon %q not found in quorum, waiting for timeout (%d seconds left) before failover", mon.Name, timeToFailover)
			continue
		}

		// retry only once before the mon failover if the mon pod is not scheduled
		monLabelSelector := fmt.Sprintf("%s=%s,%s=%s", k8sutil.AppAttr, AppName, controller.DaemonIDLabel, mon.Name)
		isScheduled, err := k8sutil.IsPodScheduled(ctx, c.context.Clientset, c.Namespace, monLabelSelector)
		if err != nil {
			logger.Warningf("failed to check if mon %q is assigned to a node, continuing with mon failover. %v", mon.Name, err)
		} else if !isScheduled && retriesBeforeNodeDrainFailover > 0 {
			logger.Warningf("mon %q NOT found in quorum after timeout. Mon pod is not scheduled. Retrying with a timeout of %.2f seconds before failover", mon.Name, MonOutTimeout.Seconds())
			delete(c.monTimeoutList, mon.Name)
			retriesBeforeNodeDrainFailover = retriesBeforeNodeDrainFailover - 1
			return nil
		}
		retriesBeforeNodeDrainFailover = 1

		logger.Warningf("mon %q NOT found in quorum and timeout exceeded, mon will be failed over", mon.Name)
		if !c.failMon(len(quorumStatus.MonMap.Mons), desiredMonCount, mon.Name) {
			// The failover was skipped, so we continue to see if another mon needs to failover
			continue
		}

		// only deal with one unhealthy mon per health check
		return nil
	}

	// after all unhealthy mons have been removed or failed over
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

	if allMonsInQuorum && len(quorumStatus.MonMap.Mons) == desiredMonCount {
		// remove any pending/not needed mon canary deployment if everything is ok
		logger.Debug("mon cluster is healthy, removing any existing canary deployment")
		c.removeCanaryDeployments()

		// Check whether two healthy mons are on the same node when they should not be.
		// This should be a rare event to find them on the same node, so we just need to check
		// once per operator restart.
		if needToCheckMonsOnSameNode {
			needToCheckMonsOnSameNode = false
			return c.evictMonIfMultipleOnSameNode()
		}
	}

	return nil
}

// failMon compares the monCount against desiredMonCount
// Returns whether the failover request was attempted. If false,
// the operator should check for other mons to failover.
func (c *Cluster) failMon(monCount, desiredMonCount int, name string) bool {
	if monCount > desiredMonCount {
		// no need to create a new mon since we have an extra
		if err := c.removeMon(name); err != nil {
			logger.Errorf("failed to remove mon %q. %v", name, err)
		}
		return true
	}

	if err := c.allowFailover(name); err != nil {
		logger.Warningf("aborting mon %q failover. %v", name, err)
		return false
	}

	// prevent any voluntary mon drain while failing over
	if err := c.blockMonDrain(types.NamespacedName{Name: monPDBName, Namespace: c.Namespace}); err != nil {
		logger.Errorf("failed to block mon drain. %v", err)
	}

	// bring up a new mon to replace the unhealthy mon
	if err := c.failoverMon(name); err != nil {
		logger.Errorf("failed to failover mon %q. %v", name, err)
	}

	// allow any voluntary mon drain after failover
	if err := c.allowMonDrain(types.NamespacedName{Name: monPDBName, Namespace: c.Namespace}); err != nil {
		logger.Errorf("failed to allow mon drain. %v", err)
	}
	return true
}

func (c *Cluster) allowFailover(name string) error {
	if !c.spec.IsStretchCluster() {
		// always failover if not a stretch cluster
		return nil
	}
	if name != c.arbiterMon {
		// failover if it's a non-arbiter
		return nil
	}
	if c.ClusterInfo.CephVersion.IsAtLeast(arbiterFailoverSupportedCephVersion) {
		// failover the arbiter if at least v16.2.7
		return nil
	}

	// Ceph does not support updating the arbiter mon in older versions
	return errors.Errorf("refusing to failover arbiter mon %q on a stretched cluster until upgrading to ceph version %s", name, arbiterFailoverSupportedCephVersion.String())
}

func (c *Cluster) removeOrphanMonResources() {
	if c.spec.Mon.VolumeClaimTemplate == nil {
		logger.Debug("skipping check for orphaned mon pvcs since using the host path")
		return
	}

	logger.Info("checking for orphaned mon resources")

	opts := metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=%s", k8sutil.AppAttr, AppName)}
	pvcs, err := c.context.Clientset.CoreV1().PersistentVolumeClaims(c.Namespace).List(c.ClusterInfo.Context, opts)
	if err != nil {
		logger.Infof("failed to check for orphaned mon pvcs. %v", err)
		return
	}

	for _, pvc := range pvcs.Items {
		logger.Debugf("checking if pvc %q is orphaned", pvc.Name)

		_, err := c.context.Clientset.AppsV1().Deployments(c.Namespace).Get(c.ClusterInfo.Context, pvc.Name, metav1.GetOptions{})
		if err == nil {
			logger.Debugf("skipping pvc removal since the mon daemon %q still requires it", pvc.Name)
			continue
		}
		if !kerrors.IsNotFound(err) {
			logger.Infof("skipping pvc removal since the mon daemon %q might still require it. %v", pvc.Name, err)
			continue
		}

		logger.Infof("removing pvc %q since it is no longer needed for the mon daemon", pvc.Name)
		var gracePeriod int64 // delete immediately
		propagation := metav1.DeletePropagationForeground
		options := &metav1.DeleteOptions{GracePeriodSeconds: &gracePeriod, PropagationPolicy: &propagation}
		err = c.context.Clientset.CoreV1().PersistentVolumeClaims(c.Namespace).Delete(c.ClusterInfo.Context, pvc.Name, *options)
		if err != nil {
			logger.Warningf("failed to delete orphaned monitor pvc %q. %v", pvc.Name, err)
		}
	}
}

func (c *Cluster) updateMonDeploymentReplica(name string, enabled bool) error {
	// get the existing deployment
	d, err := c.context.Clientset.AppsV1().Deployments(c.Namespace).Get(c.ClusterInfo.Context, resourceName(name), metav1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to get mon %q", name)
	}

	// set the desired number of replicas
	var desiredReplicas int32
	if enabled {
		desiredReplicas = 1
	}
	originalReplicas := *d.Spec.Replicas
	d.Spec.Replicas = &desiredReplicas

	// update the deployment
	logger.Infof("scaling the mon %q deployment to replica %d", name, desiredReplicas)
	_, err = c.context.Clientset.AppsV1().Deployments(c.Namespace).Update(c.ClusterInfo.Context, d, metav1.UpdateOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to update mon %q replicas from %d to %d", name, originalReplicas, desiredReplicas)
	}
	return nil
}

func (c *Cluster) failoverMon(name string) error {
	logger.Infof("Failing over monitor %q", name)

	// Scale down the failed mon to allow a new one to start
	if err := c.updateMonDeploymentReplica(name, false); err != nil {
		// attempt to continue with the failover even if the bad mon could not be stopped
		logger.Warningf("failed to stop mon %q for failover. %v", name, err)
	}
	newMonSucceeded := false
	defer func() {
		if newMonSucceeded {
			// do nothing if the new mon was started successfully, the deployment will anyway be deleted
			return
		}
		if err := c.updateMonDeploymentReplica(name, true); err != nil {
			// attempt to continue even if the bad mon could not be restarted
			logger.Warningf("failed to restart failed mon %q after new mon wouldn't start. %v", name, err)
		}
	}()

	// remove the failed mon from a local list of the existing mons for finding a stretch zone
	existingMons := c.clusterInfoToMonConfig(name)

	zone, err := c.findAvailableZoneIfStretched(existingMons)
	if err != nil {
		return errors.Wrap(err, "failed to find available stretch zone")
	}

	// Start a new monitor
	m := c.newMonConfig(c.maxMonID+1, zone)
	logger.Infof("starting new mon: %+v", m)

	mConf := []*monConfig{m}

	// Assign the pod to a node
	if err := c.assignMons(mConf); err != nil {
		return errors.Wrap(err, "failed to place new mon on a node")
	}

	if c.spec.Network.IsHost() {
		schedule, ok := c.mapping.Schedule[m.DaemonName]
		if !ok {
			return errors.Errorf("mon %s doesn't exist in assignment map", m.DaemonName)
		}
		m.PublicIP = schedule.Address
	} else {
		// Create the service endpoint
		serviceIP, err := c.createService(m)
		if err != nil {
			return errors.Wrap(err, "failed to create mon service")
		}
		m.PublicIP = serviceIP
	}
	c.ClusterInfo.Monitors[m.DaemonName] = cephclient.NewMonInfo(m.DaemonName, m.PublicIP, m.Port)

	// Start the deployment
	if err := c.startDeployments(mConf, true); err != nil {
		return errors.Wrapf(err, "failed to start new mon %s", m.DaemonName)
	}

	// Assign to a zone if a stretch cluster
	if c.spec.IsStretchCluster() {
		// Update the arbiter mon for the stretch cluster if it changed
		if err := c.ConfigureArbiter(); err != nil {
			return errors.Wrap(err, "failed to configure stretch arbiter")
		}
	}

	// Only increment the max mon id if the new pod started successfully
	c.maxMonID++
	newMonSucceeded = true

	return c.removeMon(name)
}

// make a best effort to remove the mon and all its resources
func (c *Cluster) removeMon(daemonName string) error {
	logger.Infof("ensuring removal of unhealthy monitor %s", daemonName)

	resourceName := resourceName(daemonName)

	// Remove the mon pod if it is still there
	var gracePeriod int64
	propagation := metav1.DeletePropagationForeground
	options := &metav1.DeleteOptions{GracePeriodSeconds: &gracePeriod, PropagationPolicy: &propagation}
	if err := c.context.Clientset.AppsV1().Deployments(c.Namespace).Delete(c.ClusterInfo.Context, resourceName, *options); err != nil {
		if kerrors.IsNotFound(err) {
			logger.Infof("dead mon %s was already gone", resourceName)
		} else {
			logger.Errorf("failed to remove dead mon deployment %q. %v", resourceName, err)
		}
	}

	// Remove the bad monitor from quorum
	if err := c.removeMonitorFromQuorum(daemonName); err != nil {
		logger.Errorf("failed to remove mon %q from quorum. %v", daemonName, err)
	}
	delete(c.ClusterInfo.Monitors, daemonName)

	delete(c.mapping.Schedule, daemonName)

	// Remove the service endpoint
	if err := c.context.Clientset.CoreV1().Services(c.Namespace).Delete(c.ClusterInfo.Context, resourceName, *options); err != nil {
		if kerrors.IsNotFound(err) {
			logger.Infof("dead mon service %s was already gone", resourceName)
		} else {
			logger.Errorf("failed to remove dead mon service %q. %v", resourceName, err)
		}
	}

	// Remove the PVC backing the mon if it existed
	if err := c.context.Clientset.CoreV1().PersistentVolumeClaims(c.Namespace).Delete(c.ClusterInfo.Context, resourceName, metav1.DeleteOptions{}); err != nil {
		if kerrors.IsNotFound(err) {
			logger.Infof("mon pvc did not exist %q", resourceName)
		} else {
			logger.Errorf("failed to remove dead mon pvc %q. %v", resourceName, err)
		}
	}

	if err := c.saveMonConfig(); err != nil {
		return errors.Wrapf(err, "failed to save mon config after failing over mon %s", daemonName)
	}

	// Update cluster-wide RBD bootstrap peer token since Monitors have changed
	_, err := controller.CreateBootstrapPeerSecret(c.context, c.ClusterInfo, &cephv1.CephCluster{ObjectMeta: metav1.ObjectMeta{Name: c.ClusterInfo.NamespacedName().Name, Namespace: c.Namespace}}, c.ownerInfo)
	if err != nil {
		return errors.Wrap(err, "failed to update cluster rbd bootstrap peer token")
	}

	return nil
}

func (c *Cluster) removeMonitorFromQuorum(name string) error {
	logger.Debugf("removing monitor %s", name)
	args := []string{"mon", "remove", name}
	if _, err := cephclient.NewCephCommand(c.context, c.ClusterInfo, args).Run(); err != nil {
		return errors.Wrapf(err, "mon %s remove failed", name)
	}

	logger.Infof("removed monitor %s", name)
	return nil
}

func (c *Cluster) handleExternalMonStatus(status cephclient.MonStatusResponse) error {
	// We don't need to validate Ceph version if no image is present
	if c.spec.CephVersion.Image != "" {
		_, err := controller.ValidateCephVersionsBetweenLocalAndExternalClusters(c.context, c.ClusterInfo)
		if err != nil {
			return errors.Wrap(err, "failed to validate external ceph version")
		}
	}

	changed, err := c.addOrRemoveExternalMonitor(status)
	if err != nil {
		return errors.Wrap(err, "failed to add or remove external mon")
	}

	// let's save the monitor's config if anything happened
	if changed {
		if err := c.saveMonConfig(); err != nil {
			return errors.Wrap(err, "failed to save mon config after adding/removing external mon")
		}
	}

	return nil
}

func (c *Cluster) addOrRemoveExternalMonitor(status cephclient.MonStatusResponse) (bool, error) {
	var changed bool
	oldClusterInfoMonitors := map[string]*cephclient.MonInfo{}
	// clearing the content of clusterinfo monitors
	// and populate oldClusterInfoMonitors with monitors from clusterinfo
	// later c.ClusterInfo.Monitors get populated again
	for monName, mon := range c.ClusterInfo.Monitors {
		oldClusterInfoMonitors[mon.Name] = mon
		delete(c.ClusterInfo.Monitors, monName)
	}
	logger.Debugf("ClusterInfo is now Empty, refilling it from status.MonMap.Mons")

	monCount := len(status.MonMap.Mons)
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
				c.ClusterInfo.Monitors[mon.Name] = cephclient.NewMonInfo(mon.Name, monIP, monPort)
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

func (c *Cluster) evictMonIfMultipleOnSameNode() error {
	if c.spec.Mon.AllowMultiplePerNode {
		logger.Debug("skipping check for multiple mons on same node since multiple mons are allowed")
		return nil
	}

	logger.Info("checking if multiple mons are on the same node")

	// Get all the mon pods
	label := fmt.Sprintf("app=%s", AppName)
	pods, err := c.context.Clientset.CoreV1().Pods(c.Namespace).List(c.ClusterInfo.Context, metav1.ListOptions{LabelSelector: label})
	if err != nil {
		return errors.Wrap(err, "failed to list mon pods")
	}

	nodesToMons := map[string]string{}
	for _, pod := range pods.Items {
		logger.Debugf("analyzing mon pod %q on node %q", pod.Name, pod.Spec.NodeName)
		if _, ok := pod.Labels["mon_canary"]; ok {
			logger.Debugf("skipping mon canary pod %q", pod.Name)
			continue
		}
		if pod.Spec.NodeName == "" {
			logger.Warningf("mon %q is not assigned to a node", pod.Name)
			continue
		}
		monName := pod.Labels["mon"]
		previousMonName, ok := nodesToMons[pod.Spec.NodeName]
		if !ok {
			// remember this node is taken by this mon
			nodesToMons[pod.Spec.NodeName] = monName
			continue
		}

		logger.Warningf("Both mons %q and %q are on node %q. Evicting mon %q", monName, previousMonName, pod.Spec.NodeName, monName)
		return c.failoverMon(monName)
	}

	return nil
}
