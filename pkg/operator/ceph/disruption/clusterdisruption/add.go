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

package clusterdisruption

import (
	ctx "context"
	"reflect"

	"github.com/rook/rook/pkg/operator/ceph/disruption/controllerconfig"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/types"
)

// Watch for CephBlockPools and enqueue the CephCluster in the namespace
// Watch for CephFileSystems and enqueue the CephCluster in the namespace
// Watch for CephObjectStores and enqueue the CephCluster in the namespace
var objectsToWatch = []client.Object{
	&cephv1.CephBlockPool{},
	&cephv1.CephFilesystem{},
	&cephv1.CephObjectStore{},
}

func cephClusterPredicate[T *cephv1.CephCluster]() predicate.TypedFuncs[T] {
	return predicate.TypedFuncs[T]{
		CreateFunc: func(e event.TypedCreateEvent[T]) bool {
			logger.Debug("create event from ceph cluster CR")
			return true
		},
		UpdateFunc: func(e event.TypedUpdateEvent[T]) bool {
			objOld := (*cephv1.CephCluster)(e.ObjectOld)
			objNew := (*cephv1.CephCluster)(e.ObjectNew)

			return !reflect.DeepEqual(objOld.Spec, objNew.Spec)
		},
	}
}

func pdbPredicate[T *policyv1.PodDisruptionBudget]() predicate.TypedFuncs[T] {
	return predicate.TypedFuncs[T]{
		CreateFunc: func(e event.TypedCreateEvent[T]) bool {
			// Do not reconcile when PDB is created
			return false
		},
		UpdateFunc: func(e event.TypedUpdateEvent[T]) bool {
			pdb := (*policyv1.PodDisruptionBudget)(e.ObjectNew)

			// reconcile for the main PDB update event when first OSD goes down, that is,  when `DisruptionsAllowed` gets updated to 0.
			return pdb.Name == osdPDBAppName && pdb.Spec.MaxUnavailable.IntVal == 1 && pdb.Status.DisruptionsAllowed == 0
		},
		DeleteFunc: func(e event.TypedDeleteEvent[T]) bool {
			// Do not reconcile when PDB is deleted
			return false
		},
	}
}

func watchNamespacedObject[T client.Object](c controller.Controller, mgr manager.Manager, obj T) error {
	// enqueues with an empty name that is populated by the reconciler.
	// There is a one-per-namespace limit on CephClusters
	return c.Watch(
		source.Kind(
			mgr.GetCache(),
			obj,
			handler.TypedEnqueueRequestsFromMapFunc(
				func(context ctx.Context, obj T) []reconcile.Request {
					// The name will be populated in the reconcile
					namespace := obj.GetNamespace()
					if len(namespace) == 0 {
						logger.Errorf("enqueueByNamespace received an obj without a namespace. %+v", obj)
						return []reconcile.Request{}
					}
					req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: namespace}}
					return []reconcile.Request{req}
				},
			),
		),
	)
}

// Add adds a new Controller to the Manager based on clusterdisruption.ReconcileClusterDisruption and registers the relevant watches and handlers.
// Read more about how Managers, Controllers, and their Watches, Handlers, Predicates, etc work here:
// https://godoc.org/github.com/kubernetes-sigs/controller-runtime/pkg
func Add(mgr manager.Manager, context *controllerconfig.Context) error {
	// This will be used to associate namespaces and cephclusters.
	sharedClusterMap := &ClusterMap{}

	reconcileClusterDisruption := &ReconcileClusterDisruption{
		client:     mgr.GetClient(),
		scheme:     mgr.GetScheme(),
		context:    context,
		clusterMap: sharedClusterMap,
	}
	reconciler := reconcile.Reconciler(reconcileClusterDisruption)
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: reconciler})
	if err != nil {
		return err
	}

	// Watch for CephClusters
	err = c.Watch(
		source.Kind(
			mgr.GetCache(),
			&cephv1.CephCluster{},
			&handler.TypedEnqueueRequestForObject[*cephv1.CephCluster]{},
			cephClusterPredicate(),
		),
	)
	if err != nil {
		return err
	}

	// Watch for main PodDisruptionBudget and enqueue the CephCluster in the namespace
	err = c.Watch(
		source.Kind(
			mgr.GetCache(),
			&policyv1.PodDisruptionBudget{},
			handler.TypedEnqueueRequestsFromMapFunc(
				func(context ctx.Context, pdb *policyv1.PodDisruptionBudget) []reconcile.Request {
					namespace := pdb.GetNamespace()
					req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: namespace}}
					return []reconcile.Request{req}
				},
			),
			pdbPredicate(),
		),
	)
	if err != nil {
		return err
	}

	for _, t := range objectsToWatch {
		err = watchNamespacedObject(c, mgr, t)
		if err != nil {
			return err
		}
	}

	return nil
}
