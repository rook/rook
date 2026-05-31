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

package bucket

import (
	"context"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const rookOBCWatchOperatorNamespace = "ROOK_OBC_WATCH_OPERATOR_NAMESPACE"

// reconcile on ConfigMap change
func cmPredicate[T *corev1.ConfigMap]() predicate.TypedFuncs[T] {
	return predicate.TypedFuncs[T]{
		CreateFunc: func(e event.TypedCreateEvent[T]) bool {
			cm := (*corev1.ConfigMap)(e.Object)

			return cm.Name == controller.OperatorSettingConfigMapName && cm.Generation == 1
		},

		UpdateFunc: func(e event.TypedUpdateEvent[T]) bool {
			cmOld := (*corev1.ConfigMap)(e.ObjectOld)
			cmNew := (*corev1.ConfigMap)(e.ObjectNew)

			if cmOld.GetName() == controller.OperatorSettingConfigMapName && cmNew.GetName() == controller.OperatorSettingConfigMapName {
				if cmOld.Data[rookOBCWatchOperatorNamespace] != cmNew.Data[rookOBCWatchOperatorNamespace] {
					logger.Infof("%s changed. reconciling bucket controller", rookOBCWatchOperatorNamespace)

					// We reload the manager so that the controller restarts and goes
					// through the CreateFunc again. Then the CephCluster watcher will be triggered
					controller.ReloadManager()
				}
			}

			return false
		},

		DeleteFunc: func(e event.TypedDeleteEvent[T]) bool {
			return false
		},

		GenericFunc: func(e event.TypedGenericEvent[T]) bool {
			return false
		},
	}
}

// trigger reconcile on CephCluster change
func cephClusterPredicate[T *cephv1.CephCluster](ctx context.Context, c client.Client) predicate.TypedFuncs[T] {
	return predicate.TypedFuncs[T]{
		CreateFunc: func(e event.TypedCreateEvent[T]) bool {
			// If a Ceph Cluster is created we want to reconcile the bucket provisioner
			// If there are more than one ceph cluster in the same namespace do not reconcile
			return !controller.DuplicateCephClusters(ctx, c, e.Object, false)
		},

		UpdateFunc: func(e event.TypedUpdateEvent[T]) bool {
			return false
		},

		DeleteFunc: func(e event.TypedDeleteEvent[T]) bool {
			return false
		},

		GenericFunc: func(e event.TypedGenericEvent[T]) bool {
			return false
		},
	}
}
