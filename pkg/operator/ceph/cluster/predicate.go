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

// Package cluster to manage a Ceph cluster.
package cluster

import (
	"context"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	discoverDaemon "github.com/rook/rook/pkg/daemon/discover"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

func shouldReconcileChangedNode(objOld, objNew *corev1.Node) bool {
	// do not watch node if only resourceversion got changed
	resourceQtyComparer := cmpopts.IgnoreFields(v1.ObjectMeta{}, "ResourceVersion")
	diff := cmp.Diff(objOld.Spec, objNew.Spec, resourceQtyComparer)

	// do not watch node if only LastHeartbeatTime got changed
	resourceQtyComparer = cmpopts.IgnoreFields(corev1.NodeCondition{}, "LastHeartbeatTime")
	diff1 := cmp.Diff(objOld.Spec, objNew.Spec, resourceQtyComparer)

	if diff == "" && diff1 == "" {
		return false
	}
	return true
}

// predicateForNodeWatcher is the predicate function to trigger reconcile on Node events
func predicateForNodeWatcher[T *corev1.Node](ctx context.Context, client client.Client, context *clusterd.Context, opNamespace string) predicate.TypedFuncs[T] {
	return predicate.TypedFuncs[T]{
		CreateFunc: func(e event.TypedCreateEvent[T]) bool {
			obj := (*corev1.Node)(e.Object)

			clientCluster := newClientCluster(client, obj.GetNamespace(), context)
			return clientCluster.onK8sNode(ctx, obj, opNamespace)
		},

		UpdateFunc: func(e event.TypedUpdateEvent[T]) bool {
			objOld := (*corev1.Node)(e.ObjectOld)
			objNew := (*corev1.Node)(e.ObjectNew)

			if !shouldReconcileChangedNode(objOld, objNew) {
				return false
			}

			clientCluster := newClientCluster(client, objNew.GetNamespace(), context)
			return clientCluster.onK8sNode(ctx, objNew, opNamespace)
		},

		DeleteFunc: func(e event.TypedDeleteEvent[T]) bool {
			return false
		},

		GenericFunc: func(e event.TypedGenericEvent[T]) bool {
			return false
		},
	}
}

// predicateForHotPlugCMWatcher is the predicate function to trigger reconcile on ConfigMap events (hot-plug)
func predicateForHotPlugCMWatcher[T *corev1.ConfigMap](client client.Client) predicate.TypedFuncs[T] {
	return predicate.TypedFuncs[T]{
		UpdateFunc: func(e event.TypedUpdateEvent[T]) bool {
			objOld := (*corev1.ConfigMap)(e.ObjectOld)
			objNew := (*corev1.ConfigMap)(e.ObjectNew)

			if !isHotPlugCM(objNew) {
				return false
			}

			clientCluster := newClientCluster(client, objNew.GetNamespace(), &clusterd.Context{})
			return clientCluster.onDeviceCMUpdate(objOld, objNew)
		},

		DeleteFunc: func(e event.TypedDeleteEvent[T]) bool {
			// TODO: if the configmap goes away we could retrigger rook-discover DS
			// However at this point the returned bool can only trigger a reconcile of the CephCluster object
			// Definitely non-trivial but nice to have in the future
			return false
		},

		CreateFunc: func(e event.TypedCreateEvent[T]) bool {
			return false
		},

		GenericFunc: func(e event.TypedGenericEvent[T]) bool {
			return false
		},
	}
}

// isHotPlugCM informs whether the object is the cm for hot-plug disk
func isHotPlugCM(cm *corev1.ConfigMap) bool {
	// Get the labels
	labels := cm.GetLabels()

	labelVal, labelKeyExist := labels[k8sutil.AppAttr]
	if labelKeyExist && labelVal == discoverDaemon.AppName {
		return true
	}

	return false
}

func watchControllerPredicate[T *cephv1.CephCluster](ctx context.Context, c client.Client) predicate.TypedFuncs[T] {
	return predicate.TypedFuncs[T]{
		CreateFunc: func(e event.TypedCreateEvent[T]) bool {
			if controller.DuplicateCephClusters(ctx, c, e.Object, true) {
				return false
			}
			logger.Debug("create event from a CR")
			return true
		},
		DeleteFunc: func(e event.TypedDeleteEvent[T]) bool {
			logger.Debug("delete event from a CR")
			return true
		},
		UpdateFunc: func(e event.TypedUpdateEvent[T]) bool {
			// We still need to check on update event since the user must delete the additional CR
			// Until this is done, the user can still update the CR and the operator will reconcile
			// This should not happen
			if controller.DuplicateCephClusters(ctx, c, e.ObjectOld, true) {
				return false
			}

			// resource.Quantity has non-exportable fields, so we use its comparator method
			resourceQtyComparer := cmp.Comparer(func(x, y resource.Quantity) bool { return x.Cmp(y) == 0 })

			objOld := (*cephv1.CephCluster)(e.ObjectOld)
			objNew := (*cephv1.CephCluster)(e.ObjectNew)

			logger.Debug("update event on CephCluster CR")
			// If the labels "do_not_reconcile" is set on the object, let's not reconcile that request
			if controller.IsDoNotReconcile(objNew.GetLabels()) {
				logger.Debugf("object %q matched on update but %q label is set, doing nothing", controller.DoNotReconcileLabelName, objNew.Name)
				return false
			}
			diff := cmp.Diff(objOld.Spec, objNew.Spec, resourceQtyComparer)
			if diff != "" {
				logger.Infof("CR has changed for %q. diff=%s", objNew.Name, diff)

				if objNew.Spec.CleanupPolicy.HasDataDirCleanPolicy() {
					logger.Infof("skipping orchestration for cluster object %q in namespace %q because its cleanup policy is set. not reloading the manager", objNew.GetName(), objNew.GetNamespace())
					return false
				}

				// Stop any ongoing orchestration
				controller.ReloadManager()

				return false

			} else if !objOld.GetDeletionTimestamp().Equal(objNew.GetDeletionTimestamp()) {
				logger.Infof("CR %q is going be deleted, cancelling any ongoing orchestration", objNew.Name)

				// Stop any ongoing orchestration
				controller.ReloadManager()

				return false

			} else if objOld.GetGeneration() != objNew.GetGeneration() {
				logger.Debugf("skipping resource %q update with unchanged spec", objNew.Name)
			}

			return false
		},
		GenericFunc: func(e event.TypedGenericEvent[T]) bool {
			return false
		},
	}
}
