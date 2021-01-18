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
	"strings"
	"time"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// OperatorSettingConfigMapName refers to ConfigMap that configures rook ceph operator
const OperatorSettingConfigMapName string = "rook-ceph-operator-config"

var (
	// ImmediateRetryResult Return this for a immediate retry of the reconciliation loop with the same request object.
	ImmediateRetryResult = reconcile.Result{Requeue: true}

	// WaitForRequeueIfCephClusterNotReady waits for the CephCluster to be ready
	WaitForRequeueIfCephClusterNotReady = reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}

	// WaitForRequeueIfFinalizerBlocked waits for resources to be cleaned up before the finalizer can be removed
	WaitForRequeueIfFinalizerBlocked = reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}

	// OperatorCephBaseImageVersion is the ceph version in the operator image
	OperatorCephBaseImageVersion string
)

// CheckForCancelledOrchestration checks whether a cancellation has been requested
func CheckForCancelledOrchestration(context *clusterd.Context) error {
	defer context.RequestCancelOrchestration.UnSet()

	// Check whether we need to cancel the orchestration
	if context.RequestCancelOrchestration.IsSet() {
		return errors.New("CANCELLING CURRENT ORCHESTRATION")
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
		logger.Debugf("%q: ignoring ceph status %q because only cause is %q (full status is %q)", controllerName, cephCluster.Status.CephStatus.Health, healthErrKeys[0], cephCluster.Status.CephStatus)
	}
	return ignoreHealthErr
}

// IsReadyToReconcile determines if a controller is ready to reconcile or not
func IsReadyToReconcile(c client.Client, clustercontext *clusterd.Context, namespacedName types.NamespacedName, controllerName string) (cephv1.CephCluster, bool, bool, reconcile.Result) {
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
	cephClusterExists = true
	cephCluster = clusterList.Items[0]

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
			logger.Infof("%s: CephCluster %q found but skipping reconcile since ceph health is %q", controllerName, cephCluster.Name, cephCluster.Status.CephStatus)
		}
	}else {
		logger.Infof("%s: CephCluster %q found but skipping reconcile since ceph health is unknown", controllerName, cephCluster.Name)
	}

	return cephCluster, false, cephClusterExists, WaitForRequeueIfCephClusterNotReady
}

// ClusterOwnerRef represents the owner reference of the CephCluster CR
func ClusterOwnerRef(clusterName, clusterID string) metav1.OwnerReference {
	blockOwner := true
	return metav1.OwnerReference{
		APIVersion:         fmt.Sprintf("%s/%s", ClusterResource.Group, ClusterResource.Version),
		Kind:               ClusterResource.Kind,
		Name:               clusterName,
		UID:                types.UID(clusterID),
		BlockOwnerDeletion: &blockOwner,
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
