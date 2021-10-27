/*
Copyright 2020 The Rook Authors. All rights reserved.

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

package controller

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/exec"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	// OperatorSettingConfigMapName refers to ConfigMap that configures rook ceph operator
	OperatorSettingConfigMapName string = "rook-ceph-operator-config"

	// UninitializedCephConfigError refers to the error message printed by the Ceph CLI when there is no ceph configuration file
	// This typically is raised when the operator has not finished initializing
	UninitializedCephConfigError = "error calling conf_read_file"

	// OperatorNotInitializedMessage is the message we print when the Operator is not ready to reconcile, typically the ceph.conf has not been generated yet
	OperatorNotInitializedMessage = "skipping reconcile since operator is still initializing"

	// CancellingOrchestrationMessage is the message to indicate a reconcile was cancelled
	CancellingOrchestrationMessage = "CANCELLING CURRENT ORCHESTRATION"
)

var (
	// ImmediateRetryResult Return this for a immediate retry of the reconciliation loop with the same request object.
	ImmediateRetryResult = reconcile.Result{Requeue: true}

	// ImmediateRetryResultNoBackoff Return this for a immediate retry of the reconciliation loop with the same request object.
	// Override the exponential backoff behavior by setting the RequeueAfter time explicitly.
	ImmediateRetryResultNoBackoff = reconcile.Result{Requeue: true, RequeueAfter: time.Second}

	// WaitForRequeueIfCephClusterNotReady waits for the CephCluster to be ready
	WaitForRequeueIfCephClusterNotReady = reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}

	// WaitForRequeueIfFinalizerBlocked waits for resources to be cleaned up before the finalizer can be removed
	WaitForRequeueIfFinalizerBlocked = reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}

	// WaitForRequeueIfOperatorNotInitialized waits for resources to be cleaned up before the finalizer can be removed
	WaitForRequeueIfOperatorNotInitialized = reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}

	// OperatorCephBaseImageVersion is the ceph version in the operator image
	OperatorCephBaseImageVersion string
)

func FlexDriverEnabled(context *clusterd.Context) bool {
	// Ignore the error. In the remote chance that the configmap fails to be read, we will default to disabling the flex driver
	value, _ := k8sutil.GetOperatorSetting(context.Clientset, OperatorSettingConfigMapName, "ROOK_ENABLE_FLEX_DRIVER", "false")
	return value == "true"
}

func DiscoveryDaemonEnabled(context *clusterd.Context) bool {
	// Ignore the error. In the remote chance that the configmap fails to be read, we will default to disabling the discovery daemon
	value, _ := k8sutil.GetOperatorSetting(context.Clientset, OperatorSettingConfigMapName, "ROOK_ENABLE_DISCOVERY_DAEMON", "false")
	return value == "true"
}

// SetCephCommandsTimeout sets the timeout value of Ceph commands which are executed from Rook
func SetCephCommandsTimeout(context *clusterd.Context) {
	strTimeoutSeconds, _ := k8sutil.GetOperatorSetting(context.Clientset, OperatorSettingConfigMapName, "ROOK_CEPH_COMMANDS_TIMEOUT_SECONDS", "15")
	timeoutSeconds, err := strconv.Atoi(strTimeoutSeconds)
	if err != nil || timeoutSeconds < 1 {
		logger.Warningf("ROOK_CEPH_COMMANDS_TIMEOUT is %q but it should be >= 1, set the default value 15", strTimeoutSeconds)
		timeoutSeconds = 15
	}
	exec.CephCommandsTimeout = time.Duration(timeoutSeconds) * time.Second
}

// CheckForCancelledOrchestration checks whether a cancellation has been requested
func CheckForCancelledOrchestration(context *clusterd.Context) error {
	defer context.RequestCancelOrchestration.UnSet()

	// Check whether we need to cancel the orchestration
	if context.RequestCancelOrchestration.IsSet() {
		return errors.New(CancellingOrchestrationMessage)
	}

	return nil
}

// canIgnoreHealthErrStatusInReconcile determines whether a status of HEALTH_ERR in the CephCluster can be ignored safely.
func canIgnoreHealthErrStatusInReconcile(cephCluster cephv1.CephCluster, controllerName string) bool {
	// Get a list of all the keys causing the HEALTH_ERR status.
	var healthErrKeys = make([]string, 0)
	for key, health := range cephCluster.Status.CephStatus.Details {
		if health.Severity == "HEALTH_ERR" {
			healthErrKeys = append(healthErrKeys, key)
		}
	}

	// If there is only one cause for HEALTH_ERR and it's on the allowed list of errors, ignore it.
	var allowedErrStatus = []string{"MDS_ALL_DOWN"}
	var ignoreHealthErr = len(healthErrKeys) == 1 && contains(allowedErrStatus, healthErrKeys[0])
	if ignoreHealthErr {
		logger.Debugf("%q: ignoring ceph status %q because only cause is %q (full status is %+v)", controllerName, cephCluster.Status.CephStatus.Health, healthErrKeys[0], cephCluster.Status.CephStatus)
	}
	return ignoreHealthErr
}

// IsReadyToReconcile determines if a controller is ready to reconcile or not
func IsReadyToReconcile(c client.Client, namespacedName types.NamespacedName, controllerName string) (cephv1.CephCluster, bool, bool, reconcile.Result) {
	cephClusterExists := false

	// Running ceph commands won't work and the controller will keep re-queuing so I believe it's fine not to check
	// Make sure a CephCluster exists before doing anything
	var cephCluster cephv1.CephCluster
	clusterList := &cephv1.CephClusterList{}
	err := c.List(context.TODO(), clusterList, client.InNamespace(namespacedName.Namespace))
	if err != nil {
		logger.Errorf("%q: failed to fetch CephCluster %v", controllerName, err)
		return cephCluster, false, cephClusterExists, ImmediateRetryResult
	}
	if len(clusterList.Items) == 0 {
		logger.Debugf("%q: no CephCluster resource found in namespace %q", controllerName, namespacedName.Namespace)
		return cephCluster, false, cephClusterExists, WaitForRequeueIfCephClusterNotReady
	}
	cephCluster = clusterList.Items[0]

	// If the cluster has a cleanup policy to destroy the cluster and it has been marked for deletion, treat it as if it does not exist
	if cephCluster.Spec.CleanupPolicy.HasDataDirCleanPolicy() && !cephCluster.DeletionTimestamp.IsZero() {
		logger.Infof("%q: CephCluster %q has a destructive cleanup policy, allowing resources to be deleted", controllerName, namespacedName)
		return cephCluster, false, cephClusterExists, WaitForRequeueIfCephClusterNotReady
	}

	cephClusterExists = true
	logger.Debugf("%q: CephCluster resource %q found in namespace %q", controllerName, cephCluster.Name, namespacedName.Namespace)

	// read the CR status of the cluster
	if cephCluster.Status.CephStatus != nil {
		var operatorDeploymentOk = cephCluster.Status.CephStatus.Health == "HEALTH_OK" || cephCluster.Status.CephStatus.Health == "HEALTH_WARN"

		if operatorDeploymentOk || canIgnoreHealthErrStatusInReconcile(cephCluster, controllerName) {
			logger.Debugf("%q: ceph status is %q, operator is ready to run ceph command, reconciling", controllerName, cephCluster.Status.CephStatus.Health)
			return cephCluster, true, cephClusterExists, WaitForRequeueIfCephClusterNotReady
		}

		details := cephCluster.Status.CephStatus.Details
		message, ok := details["error"]
		if ok && len(details) == 1 && strings.Contains(message.Message, "Error initializing cluster client") {
			logger.Infof("%s: skipping reconcile since operator is still initializing", controllerName)
		} else {
			logger.Infof("%s: CephCluster %q found but skipping reconcile since ceph health is %+v", controllerName, cephCluster.Name, cephCluster.Status.CephStatus)
		}
	}

	logger.Debugf("%q: CephCluster %q initial reconcile is not complete yet...", controllerName, namespacedName.Namespace)
	return cephCluster, false, cephClusterExists, WaitForRequeueIfCephClusterNotReady
}

// ClusterOwnerRef represents the owner reference of the CephCluster CR
func ClusterOwnerRef(clusterName, clusterID string) metav1.OwnerReference {
	blockOwner := true
	controller := true
	return metav1.OwnerReference{
		APIVersion:         fmt.Sprintf("%s/%s", ClusterResource.Group, ClusterResource.Version),
		Kind:               ClusterResource.Kind,
		Name:               clusterName,
		UID:                types.UID(clusterID),
		BlockOwnerDeletion: &blockOwner,
		Controller:         &controller,
	}
}

// ClusterResource operator-kit Custom Resource Definition
var ClusterResource = k8sutil.CustomResource{
	Name:       "cephcluster",
	Plural:     "cephclusters",
	Group:      cephv1.CustomResourceGroup,
	Version:    cephv1.Version,
	Kind:       reflect.TypeOf(cephv1.CephCluster{}).Name(),
	APIVersion: fmt.Sprintf("%s/%s", cephv1.CustomResourceGroup, cephv1.Version),
}
