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

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/k8sutil"
	policyv1 "k8s.io/api/policy/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	"k8s.io/apimachinery/pkg/types"
)

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

	cephClusterPredicate := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			logger.Debug("create event from ceph cluster CR")
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldCluster, ok := e.ObjectOld.DeepCopyObject().(*cephv1.CephCluster)
			if !ok {
				return false
			}
			newCluster, ok := e.ObjectNew.DeepCopyObject().(*cephv1.CephCluster)
			if !ok {
				return false
			}
			return !reflect.DeepEqual(oldCluster.Spec, newCluster.Spec)
		},
	}

	// Watch for CephClusters
	err = c.Watch(&source.Kind{Type: &cephv1.CephCluster{}}, &handler.EnqueueRequestForObject{}, cephClusterPredicate)
	if err != nil {
		return err
	}

	usePDBV1Beta1, err := k8sutil.UsePDBV1Beta1Version(reconcileClusterDisruption.context.ClusterdContext.Clientset)
	if err != nil {
		return errors.Wrap(err, "failed to fetch pdb version")
	}

	// Only reconcile for PDB update event when allowed disruptions for the main OSD PDB is 0.
	// This means that one of the OSD is down due to node drain or any other reason
	pdbPredicate := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			// Do not reconcile when PDB is created
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if usePDBV1Beta1 {
				pdb, ok := e.ObjectNew.DeepCopyObject().(*policyv1beta1.PodDisruptionBudget)
				if !ok {
					return false
				}
				return pdb.Name == osdPDBAppName && pdb.Status.DisruptionsAllowed == 0
			}
			pdb, ok := e.ObjectNew.DeepCopyObject().(*policyv1.PodDisruptionBudget)
			if !ok {
				return false
			}
			return pdb.Name == osdPDBAppName && pdb.Status.DisruptionsAllowed == 0
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			// Do not reconcile when PDB is deleted
			return false
		},
	}

	// Watch for main PodDisruptionBudget and enqueue the CephCluster in the namespace
	if usePDBV1Beta1 {
		err = c.Watch(
			&source.Kind{Type: &policyv1beta1.PodDisruptionBudget{}},
			handler.EnqueueRequestsFromMapFunc(handler.MapFunc(func(obj client.Object) []reconcile.Request {
				pdb, ok := obj.(*policyv1beta1.PodDisruptionBudget)
				if !ok {
					// Not a pdb, returning empty
					logger.Error("PDB handler received non-PDB")
					return []reconcile.Request{}
				}
				namespace := pdb.GetNamespace()
				req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: namespace}}
				return []reconcile.Request{req}
			}),
			),
			pdbPredicate,
		)
		if err != nil {
			return err
		}
	} else {
		err = c.Watch(
			&source.Kind{Type: &policyv1.PodDisruptionBudget{}},
			handler.EnqueueRequestsFromMapFunc(handler.MapFunc(func(obj client.Object) []reconcile.Request {
				pdb, ok := obj.(*policyv1.PodDisruptionBudget)
				if !ok {
					// Not a pdb, returning empty
					logger.Error("PDB handler received non-PDB")
					return []reconcile.Request{}
				}
				namespace := pdb.GetNamespace()
				req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: namespace}}
				return []reconcile.Request{req}
			}),
			),
			pdbPredicate,
		)
		if err != nil {
			return err
		}
	}

	// enqueues with an empty name that is populated by the reconciler.
	// There is a one-per-namespace limit on CephClusters
	enqueueByNamespace := handler.EnqueueRequestsFromMapFunc(handler.MapFunc(func(obj client.Object) []reconcile.Request {
		// The name will be populated in the reconcile
		namespace := obj.GetNamespace()
		if len(namespace) == 0 {
			logger.Errorf("enqueueByNamespace received an obj without a namespace. %+v", obj)
			return []reconcile.Request{}
		}
		req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: namespace}}
		return []reconcile.Request{req}
	}),
	)

	// Watch for CephBlockPools and enqueue the CephCluster in the namespace
	err = c.Watch(&source.Kind{Type: &cephv1.CephBlockPool{}}, enqueueByNamespace)
	if err != nil {
		return err
	}

	// Watch for CephFileSystems and enqueue the CephCluster in the namespace
	err = c.Watch(&source.Kind{Type: &cephv1.CephFilesystem{}}, enqueueByNamespace)
	if err != nil {
		return err
	}

	// Watch for CephObjectStores and enqueue the CephCluster in the namespace
	err = c.Watch(&source.Kind{Type: &cephv1.CephObjectStore{}}, enqueueByNamespace)
	if err != nil {
		return err
	}

	return nil
}
