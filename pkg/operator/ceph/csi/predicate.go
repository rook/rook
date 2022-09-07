/*
Copyright 2021 The Rook Authors. All rights reserved.

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

package csi

import (
	"context"

	"github.com/google/go-cmp/cmp"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// predicateController is the predicate function to trigger reconcile on operator configuration cm change
func predicateController(ctx context.Context, c client.Client, opNamespace string) predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			// if the operator configuration file is created we want to reconcile
			if cm, ok := e.Object.(*v1.ConfigMap); ok {
				// We don't want to use cm.Generation here, it case the operator was stopped and the
				// ConfigMap was created
				return cm.Name == opcontroller.OperatorSettingConfigMapName
			}

			// If a Ceph Cluster is created we want to reconcile the csi driver
			if cephCluster, ok := e.Object.(*cephv1.CephCluster); ok {
				// If there are more than one ceph cluster in the same namespace do not reconcile
				if opcontroller.DuplicateCephClusters(ctx, c, e.Object, false) {
					return false
				}

				// We still have users that don't use the ConfigMap to configure the
				// operator/csi and still rely on environment variables from the operator pod's
				// spec. So we still want to reconcile the controller if the ConfigMap cannot be found.
				err := c.Get(ctx, types.NamespacedName{Name: opcontroller.OperatorSettingConfigMapName, Namespace: opNamespace}, &v1.ConfigMap{})
				if err != nil && kerrors.IsNotFound(err) {
					logger.Debugf("could not find operator configuration ConfigMap, will reconcile the csi controller")
					return true
				}

				// This allows us to avoid a double reconcile of the CSI controller if this is not
				// the first generation of the CephCluster. So only return true if this is the very
				// first instance of the CephCluster
				// Corner case is when the cluster is created but the operator is down AND the cm
				// does not exist... However, these days the operator config map is part of the
				// operator.yaml so it's probably acceptable?
				// This does not account for the case where the cephcluster is already deployed and
				// the upgrade the operator or restart it. However, the CM check above should catch that
				return cephCluster.Generation == 1
			}

			return false
		},

		UpdateFunc: func(e event.UpdateEvent) bool {
			resourceQtyComparer := cmp.Comparer(func(x, y resource.Quantity) bool { return x.Cmp(y) == 0 })
			if old, ok := e.ObjectOld.(*v1.ConfigMap); ok {
				if new, ok := e.ObjectNew.(*v1.ConfigMap); ok {
					if old.Name == opcontroller.OperatorSettingConfigMapName && new.Name == opcontroller.OperatorSettingConfigMapName {
						diff := cmp.Diff(old.Data, new.Data, resourceQtyComparer)
						logger.Debugf("operator configmap diff:\n %s", diff)
						if diff != "" {
							return true
						}
					}
				}
			}

			return false
		},

		DeleteFunc: func(e event.DeleteEvent) bool {
			// if the operator configuration file is deleted we want to reconcile to apply the
			// configuration based on environment variables present in the operator's pod spec
			if cm, ok := e.Object.(*v1.ConfigMap); ok {
				return cm.Name == opcontroller.OperatorSettingConfigMapName
			}

			// if cephCluster is deleted, trigger reconcile to cleanup the csi driver resources
			// if zero cephClusters exist.
			if _, ok := e.Object.(*cephv1.CephCluster); ok {
				return true
			}

			return false
		},

		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	}
}
