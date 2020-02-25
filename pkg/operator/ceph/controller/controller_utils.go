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
	"github.com/rook/rook/pkg/operator/k8sutil"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var (
	// ImmediateRetryResult Return this for a immediate retry of the reconciliation loop with the same request object.
	ImmediateRetryResult = reconcile.Result{Requeue: true}
	// WaitForRequeueIfCephClusterNotReadyAfter requeue after 10sec if the operator is not ready
	WaitForRequeueIfCephClusterNotReadyAfter = 10 * time.Second
	// WaitForRequeueIfCephClusterNotReady waits for the CephCluster to be ready
	WaitForRequeueIfCephClusterNotReady = reconcile.Result{Requeue: true, RequeueAfter: WaitForRequeueIfCephClusterNotReadyAfter}
)

// IsReadyToReconcile determines if a controller is ready to reconcile or not
func IsReadyToReconcile(client client.Client, clustercontext *clusterd.Context, namespacedName types.NamespacedName) (cephv1.ClusterSpec, bool, reconcile.Result) {
	namespacedName.Name = namespacedName.Namespace

	// Running ceph commands won't work and the controller will keep re-queuing so I believe it's fine not to check
	// Make sure a CephCluster exists before doing anything
	cephCluster := &cephv1.CephCluster{}
	err := client.Get(context.TODO(), namespacedName, cephCluster)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Errorf("CephCluster resource %q not found in namespace %q", namespacedName.Name, namespacedName.Namespace)
			return cephv1.ClusterSpec{}, false, ImmediateRetryResult
		} else if err != nil {
			logger.Errorf("failed to fetch CephCluster %v", err)
			return cephv1.ClusterSpec{}, false, ImmediateRetryResult
		}
	}

	// If the cluster is ready
	// Not using k8sutil.ReadyStatus to avoid import cycles
	if cephCluster.Status.Phase == k8sutil.ReadyStatus {
		// Test a Ceph command to verify the Operator is ready
		// This is done to silence errors when the operator just started and cannot reconcile yet
		_, err = cephclient.Status(clustercontext, namespacedName.Namespace, true)
		if err != nil {
			if strings.Contains(err.Error(), "error calling conf_read_file") {
				logger.Info("operator is not ready to run ceph command, cannot reconcile yet.")
				return cephCluster.Spec, false, WaitForRequeueIfCephClusterNotReady
			}
			// We should not arrive there
			logger.Errorf("ceph command error %v", err)
			return cephCluster.Spec, false, ImmediateRetryResult
		}
		return cephCluster.Spec, true, reconcile.Result{}
	}

	return cephCluster.Spec, false, ImmediateRetryResult
}

// Contains checks if an item exists in a given list.
func Contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}

	return false
}

// Removes any element from a list
func Remove(list []string, s string) []string {
	for i, v := range list {
		if v == s {
			list = append(list[:i], list[i+1:]...)
		}
	}

	return list
}
