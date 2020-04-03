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
	"strings"
	"time"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// OperatorSettingConfigMapName refers to ConfigMap that configures rook ceph operator
const OperatorSettingConfigMapName string = "rook-ceph-operator-config"

var (
	// ImmediateRetryResult Return this for a immediate retry of the reconciliation loop with the same request object.
	ImmediateRetryResult = reconcile.Result{Requeue: true}
	// WaitForRequeueIfCephClusterNotReadyAfter requeue after 10sec if the operator is not ready
	WaitForRequeueIfCephClusterNotReadyAfter = 10 * time.Second
	// WaitForRequeueIfCephClusterNotReady waits for the CephCluster to be ready
	WaitForRequeueIfCephClusterNotReady = reconcile.Result{Requeue: true, RequeueAfter: WaitForRequeueIfCephClusterNotReadyAfter}
)

// IsReadyToReconcile determines if a controller is ready to reconcile or not
func IsReadyToReconcile(client client.Client, clustercontext *clusterd.Context, namespacedName types.NamespacedName, controllerName string) (cephv1.ClusterSpec, bool, bool, reconcile.Result) {
	namespacedName.Name = namespacedName.Namespace
	cephClusterExists := true

	// Running ceph commands won't work and the controller will keep re-queuing so I believe it's fine not to check
	// Make sure a CephCluster exists before doing anything
	cephCluster := &cephv1.CephCluster{}
	err := client.Get(context.TODO(), namespacedName, cephCluster)
	if err != nil {
		if kerrors.IsNotFound(err) {
			cephClusterExists = false
			logger.Errorf("%q: CephCluster resource %q not found in namespace %q", controllerName, namespacedName.Name, namespacedName.Namespace)
			return cephv1.ClusterSpec{}, false, cephClusterExists, WaitForRequeueIfCephClusterNotReady
		} else if err != nil {
			logger.Errorf("%q:failed to fetch CephCluster %v", controllerName, err)
			return cephv1.ClusterSpec{}, false, cephClusterExists, ImmediateRetryResult
		}
	}

	logger.Debugf("%q: CephCluster resource %q found in namespace %q", controllerName, namespacedName.Name, namespacedName.Namespace)

	// If the cluster is healthy
	// Test a Ceph command to verify the Operator is ready
	// This is done to silence errors when the operator just started and cannot reconcile yet
	status, err := cephclient.Status(clustercontext, namespacedName.Namespace)
	if err != nil {
		if strings.Contains(err.Error(), "error calling conf_read_file") {
			logger.Infof("%q: operator is not ready to run ceph command, cannot reconcile yet.", controllerName)
			return cephCluster.Spec, false, cephClusterExists, WaitForRequeueIfCephClusterNotReady
		}
		// We should not arrive there
		logger.Errorf("%q: ceph command error %v", controllerName, err)
		return cephCluster.Spec, false, cephClusterExists, ImmediateRetryResult
	}

	// If Ceph status is ok we can reconcile
	if status.Health.Status == "HEALTH_OK" || status.Health.Status == "HEALTH_WARN" {
		logger.Debugf("%q: ceph status is %q, operator is ready to run ceph command, reconciling", controllerName, status.Health.Status)
		return cephCluster.Spec, true, cephClusterExists, reconcile.Result{}
	}

	logger.Infof("%s: CephCluster %q found but skipping reconcile since Ceph health is %q", controllerName, namespacedName.Name, status.Health.Status)
	return cephCluster.Spec, false, cephClusterExists, WaitForRequeueIfCephClusterNotReady
}
