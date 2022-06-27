/*
Copyright 2019 The Rook Authors. All rights reserved.

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

// Package cluster to manage Kubernetes storage.
package cluster

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/config"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/reporting"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	// defaultStatusCheckInterval is the interval to check the status of the ceph cluster
	defaultStatusCheckInterval = 60 * time.Second
)

// cephStatusChecker aggregates the mon/cluster info needed to check the health of the monitors
type cephStatusChecker struct {
	context     *clusterd.Context
	clusterInfo *cephclient.ClusterInfo
	interval    *time.Duration
	client      client.Client
	isExternal  bool
}

// newCephStatusChecker creates a new HealthChecker object
func newCephStatusChecker(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo, clusterSpec *cephv1.ClusterSpec) *cephStatusChecker {
	c := &cephStatusChecker{
		context:     context,
		clusterInfo: clusterInfo,
		interval:    &defaultStatusCheckInterval,
		client:      context.Client,
		isExternal:  clusterSpec.External.Enable,
	}

	// allow overriding the check interval with an env var on the operator
	// Keep the existing behavior
	var checkInterval *time.Duration
	checkIntervalCRSetting := clusterSpec.HealthCheck.DaemonHealth.Status.Interval
	checkIntervalEnv := os.Getenv("ROOK_CEPH_STATUS_CHECK_INTERVAL")
	if checkIntervalEnv != "" {
		if duration, err := time.ParseDuration(checkIntervalEnv); err == nil {
			checkInterval = &duration
		}
	} else if checkIntervalCRSetting != nil {
		checkInterval = &checkIntervalCRSetting.Duration
	}
	if checkInterval != nil {
		logger.Infof("ceph status check interval is %s", checkInterval.String())
		c.interval = checkInterval
	}

	return c
}

// checkCephStatus periodically checks the health of the cluster
func (c *cephStatusChecker) checkCephStatus(monitoringRoutines map[string]*opcontroller.ClusterHealth, daemon string) {
	// check the status immediately before starting the loop
	c.checkStatus(monitoringRoutines[daemon].InternalCtx)

	for {
		// We must perform this check otherwise the case will check an index that does not exist anymore and
		// we will get an invalid pointer error and the go routine will panic
		if _, ok := monitoringRoutines[daemon]; !ok {
			logger.Infof("ceph cluster %q has been deleted. stopping ceph status check", c.clusterInfo.Namespace)
			return
		}
		select {
		case <-monitoringRoutines[daemon].InternalCtx.Done():
			logger.Infof("stopping monitoring of ceph status")
			delete(monitoringRoutines, daemon)
			return

		case <-time.After(*c.interval):
			c.checkStatus(monitoringRoutines[daemon].InternalCtx)
		}
	}
}

// checkStatus queries the status of ceph health then updates the CR status
func (c *cephStatusChecker) checkStatus(ctx context.Context) {
	var status cephclient.CephStatus
	var err error

	logger.Debugf("checking health of cluster")

	condition := cephv1.ConditionReady
	reason := cephv1.ClusterCreatedReason
	if c.isExternal {
		condition = cephv1.ConditionConnected
		reason = cephv1.ClusterConnectedReason
	}

	// Check ceph's status
	status, err = cephclient.StatusWithUser(c.context, c.clusterInfo)
	if err != nil {
		if strings.Contains(err.Error(), opcontroller.UninitializedCephConfigError) {
			logger.Info("skipping ceph status since operator is still initializing")
			return
		}
		logger.Errorf("failed to get ceph status. %v", err)

		message := "Failed to configure ceph cluster"
		if c.isExternal {
			message = "Failed to configure external ceph cluster"
		}
		status := cephStatusOnError(err.Error())
		c.updateCephStatus(status, condition, reason, message, v1.ConditionFalse)
		return
	}

	logger.Debugf("cluster status: %+v", status)
	message := "Cluster created successfully"
	if c.isExternal {
		message = "Cluster connected successfully"
	}
	c.updateCephStatus(&status, condition, reason, message, v1.ConditionTrue)

	if status.Health.Status != "HEALTH_OK" {
		logger.Debug("checking for stuck pods on not ready nodes")
		if err := c.forceDeleteStuckRookPodsOnNotReadyNodes(ctx); err != nil {
			logger.Errorf("failed to delete pod on not ready nodes. %v", err)
		}
	}

	c.configureHealthSettings(status)
}

func (c *cephStatusChecker) configureHealthSettings(status cephclient.CephStatus) {
	// loop through the health codes and log what we find
	for healthCode, check := range status.Health.Checks {
		logger.Debugf("Health: %q, code: %q, message: %q", check.Severity, healthCode, check.Summary.Message)
	}

	// disable the insecure global id if there are no old clients
	if _, ok := status.Health.Checks["AUTH_INSECURE_GLOBAL_ID_RECLAIM_ALLOWED"]; ok {
		if _, ok := status.Health.Checks["AUTH_INSECURE_GLOBAL_ID_RECLAIM"]; !ok {
			logger.Info("Disabling the insecure global ID as no legacy clients are currently connected. If you still require the insecure connections, see the CVE to suppress the health warning and re-enable the insecure connections. https://docs.ceph.com/en/latest/security/CVE-2021-20288/")
			config.DisableInsecureGlobalID(c.context, c.clusterInfo)
		} else {
			logger.Warning("insecure clients are connected to the cluster, to resolve the AUTH_INSECURE_GLOBAL_ID_RECLAIM health warning please refer to the upgrade guide to ensure all Ceph daemons are updated.")
		}
	}
}

// updateStatus updates an object with a given status
func (c *cephStatusChecker) updateCephStatus(status *cephclient.CephStatus, condition cephv1.ConditionType, reason cephv1.ConditionReason, message string, conditionStatus v1.ConditionStatus) {
	clusterName := c.clusterInfo.NamespacedName()
	cephCluster, err := c.context.RookClientset.CephV1().CephClusters(clusterName.Namespace).Get(c.clusterInfo.Context, clusterName.Name, metav1.GetOptions{})
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("CephCluster resource not found. Ignoring since object must be deleted.")
			return
		}
		logger.Errorf("failed to retrieve ceph cluster %q in namespace %q to update status to %+v", clusterName.Name, clusterName.Namespace, status)
		return
	}

	// Update with Ceph Status
	cephCluster.Status.CephStatus = toCustomResourceStatus(cephCluster.Status, status)

	// versions store the ceph version of all the ceph daemons and overall cluster version
	versions, err := cephclient.GetAllCephDaemonVersions(c.context, c.clusterInfo)
	if err != nil {
		logger.Errorf("failed to get ceph daemons versions. %v", err)
	} else {
		// Update status with Ceph versions
		cephCluster.Status.CephStatus.Versions = versions
	}

	// Update condition
	logger.Debugf("updating ceph cluster %q status and condition to %+v, %v, %s, %s", clusterName.Namespace, status, conditionStatus, reason, message)
	opcontroller.UpdateClusterCondition(c.context, cephCluster, c.clusterInfo.NamespacedName(), k8sutil.ObservedGenerationNotAvailable, condition, conditionStatus, reason, message, true)
}

// toCustomResourceStatus converts the ceph status to the struct expected for the CephCluster CR status
func toCustomResourceStatus(currentStatus cephv1.ClusterStatus, newStatus *cephclient.CephStatus) *cephv1.CephStatus {
	s := &cephv1.CephStatus{
		Health:      newStatus.Health.Status,
		LastChecked: formatTime(time.Now().UTC()),
		Details:     make(map[string]cephv1.CephHealthMessage),
	}
	for name, message := range newStatus.Health.Checks {
		s.Details[name] = cephv1.CephHealthMessage{
			Severity: message.Severity,
			Message:  message.Summary.Message,
		}
	}

	if newStatus.PgMap.TotalBytes != 0 {
		s.Capacity.TotalBytes = newStatus.PgMap.TotalBytes
		s.Capacity.UsedBytes = newStatus.PgMap.UsedBytes
		s.Capacity.AvailableBytes = newStatus.PgMap.AvailableBytes
		s.Capacity.LastUpdated = formatTime(time.Now().UTC())
	}

	if currentStatus.CephStatus != nil {
		s.PreviousHealth = currentStatus.CephStatus.PreviousHealth
		s.LastChanged = currentStatus.CephStatus.LastChanged
		if currentStatus.CephStatus.Health != s.Health {
			s.PreviousHealth = currentStatus.CephStatus.Health
			s.LastChanged = s.LastChecked
		}
		if newStatus.PgMap.TotalBytes == 0 {
			s.Capacity = currentStatus.CephStatus.Capacity
		}
	}
	// update fsid on cephcluster Status
	s.FSID = newStatus.FSID

	return s
}

func formatTime(t time.Time) string {
	return t.Format(time.RFC3339)
}

func (c *ClusterController) updateClusterCephVersion(image string, cephVersion cephver.CephVersion) {
	logger.Infof("cluster %q: version %q detected for image %q", c.namespacedName.Namespace, cephVersion.String(), image)

	cephCluster, err := c.context.RookClientset.CephV1().CephClusters(c.namespacedName.Namespace).Get(c.OpManagerCtx, c.namespacedName.Name, metav1.GetOptions{})
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("CephCluster resource not found. Ignoring since object must be deleted.")
			return
		}
		logger.Errorf("failed to retrieve ceph cluster %q to update ceph version to %+v. %v", c.namespacedName.Name, cephVersion, err)
		return
	}

	cephClusterVersion := &cephv1.ClusterVersion{
		Image:   image,
		Version: opcontroller.GetCephVersionLabel(cephVersion),
	}
	// update the Ceph version on the retrieved cluster object
	// do not overwrite the ceph status that is updated in a separate goroutine
	cephCluster.Status.CephVersion = cephClusterVersion
	if err := reporting.UpdateStatus(c.client, cephCluster); err != nil {
		logger.Errorf("failed to update cluster %q version. %v", c.namespacedName.Name, err)
		return
	}
}

func cephStatusOnError(errorMessage string) *cephclient.CephStatus {
	details := make(map[string]cephclient.CheckMessage)
	details["error"] = cephclient.CheckMessage{
		Severity: "Urgent",
		Summary: cephclient.Summary{
			Message: errorMessage,
		},
	}

	return &cephclient.CephStatus{
		Health: cephclient.HealthStatus{
			Status: "HEALTH_ERR",
			Checks: details,
		},
	}
}

// forceDeleteStuckPodsOnNotReadyNodes lists all the nodes that are in NotReady state and
// gets all the pods on the failed node and force delete the pods stuck in terminating state.
func (c *cephStatusChecker) forceDeleteStuckRookPodsOnNotReadyNodes(ctx context.Context) error {
	nodes, err := k8sutil.GetNotReadyKubernetesNodes(ctx, c.context.Clientset)
	if err != nil {
		return errors.Wrap(err, "failed to get NotReady nodes")
	}
	for _, node := range nodes {
		pods, err := c.getRookPodsOnNode(node.Name)
		if err != nil {
			logger.Errorf("failed to get pods on NotReady node %q. %v", node.Name, err)
		}
		for _, pod := range pods {
			if err := k8sutil.ForceDeletePodIfStuck(ctx, c.context, pod); err != nil {
				logger.Warningf("skipping forced delete of stuck pod %q. %v", pod.Name, err)
			}
		}
	}
	return nil
}

func (c *cephStatusChecker) getRookPodsOnNode(node string) ([]v1.Pod, error) {
	clusterName := c.clusterInfo.NamespacedName()
	appLabels := []string{
		"csi-rbdplugin-provisioner",
		"csi-rbdplugin",
		"csi-cephfsplugin-provisioner",
		"csi-cephfsplugin",
		"rook-ceph-operator",
		"rook-ceph-mon",
		"rook-ceph-osd",
		"rook-ceph-crashcollector",
		"rook-ceph-mgr",
		"rook-ceph-mds",
		"rook-ceph-rgw",
	}
	podsOnNode := []v1.Pod{}
	listOpts := metav1.ListOptions{
		FieldSelector: fmt.Sprintf("spec.nodeName=%s", node),
	}
	pods, err := c.context.Clientset.CoreV1().Pods(clusterName.Namespace).List(c.clusterInfo.Context, listOpts)
	if err != nil {
		return podsOnNode, errors.Wrapf(err, "failed to get pods on node %q", node)
	}
	for _, pod := range pods.Items {
		for _, label := range appLabels {
			if pod.Labels["app"] == label {
				podsOnNode = append(podsOnNode, pod)
				break
			}
		}

	}
	return podsOnNode, nil
}
