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
	"github.com/rook/rook/pkg/operator/ceph/disruption/controllerconfig"
	"github.com/rook/rook/pkg/operator/ceph/disruption/nodedrain"
	"github.com/rook/rook/pkg/operator/k8sutil"

	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"

	appsv1 "k8s.io/api/apps/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	"k8s.io/apimachinery/pkg/types"
)

// Add adds a new Controller to the Manager based on clusterdisruption.ReconcileClusterDisruption and registers the relevant watches and handlers.
// Read more about how Managers, Controllers, and their Watches, Handlers, Predicates, etc work here:
// https://godoc.org/github.com/kubernetes-sigs/controller-runtime/pkg
func Add(mgr manager.Manager, context *controllerconfig.Context) error {

	// Add the cephv1 scheme to the manager scheme
	mgrScheme := mgr.GetScheme()
	cephv1.AddToScheme(mgr.GetScheme())

	// this will be used to associate namespaces and cephclusters.
	sharedClusterMap := &ClusterMap{}

	reconcileClusterDisruption := &ReconcileClusterDisruption{
		client:              mgr.GetClient(),
		scheme:              mgrScheme,
		context:             context,
		clusterMap:          sharedClusterMap,
		osdCrushLocationMap: &OSDCrushLocationMap{Context: context.ClusterdContext},
	}
	reconciler := reconcile.Reconciler(reconcileClusterDisruption)
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: reconciler})
	if err != nil {
		return err
	}

	// enqueues with an empty name that is populated by the reconciler.
	// There is a one-per-namespace limit on CephClusters
	enqueueByNamespace := &handler.EnqueueRequestsFromMapFunc{
		ToRequests: handler.ToRequestsFunc(func(obj handler.MapObject) []reconcile.Request {
			// The name will be populated in the reconcile
			namespace := obj.Meta.GetNamespace()
			if len(namespace) == 0 {
				logger.Errorf("enqueByNamespace received an obj without a namespace. %+v", obj)
				return []reconcile.Request{}
			}
			req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: namespace}}
			return []reconcile.Request{req}
		}),
	}

	// Watch for CephClusters
	err = c.Watch(&source.Kind{Type: &cephv1.CephCluster{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for PodDisruptionBudgets and enqueue the CephCluster in the namespace
	err = c.Watch(
		&source.Kind{Type: &policyv1beta1.PodDisruptionBudget{}},
		&handler.EnqueueRequestsFromMapFunc{
			ToRequests: handler.ToRequestsFunc(func(obj handler.MapObject) []reconcile.Request {
				_, ok := obj.Object.(*policyv1beta1.PodDisruptionBudget)
				if !ok {
					// not a pdb, returning empty
					logger.Errorf("PDB handler received non-PDB")
					return []reconcile.Request{}
				}
				labels := obj.Meta.GetLabels()

				// only enqueue osdDisruptionAppLabels
				_, ok = labels[osdDisruptionAppName]
				if !ok {
					return []reconcile.Request{}
				}
				// // The name will be populated in the reconcile
				namespace := obj.Meta.GetNamespace()
				req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: namespace}}

				return []reconcile.Request{req}
			}),
		},
	)
	if err != nil {
		return err
	}

	// Watch for canary Deployments created by the nodedrain controller and enqueue all Cephclusters
	err = c.Watch(
		&source.Kind{Type: &appsv1.Deployment{}},
		&handler.EnqueueRequestsFromMapFunc{
			ToRequests: handler.ToRequestsFunc(func(obj handler.MapObject) []reconcile.Request {
				_, ok := obj.Object.(*appsv1.Deployment)
				if !ok {
					// not a Deployment, returning empty
					logger.Errorf("deployment handler received non-Deployment")
					return []reconcile.Request{}
				}

				// don't enqueue if it isn't a canary Deployment
				labels := obj.Meta.GetLabels()
				appLabel, ok := labels[k8sutil.AppAttr]
				if !ok || appLabel != nodedrain.CanaryAppName {
					return []reconcile.Request{}
				}

				// Enqueue all CephClusters
				clusterMap := sharedClusterMap.GetClusterMap()
				numClusters := len(clusterMap)
				if numClusters == 0 {
					return []reconcile.Request{}
				}
				reqs := make([]reconcile.Request, 0)
				for namespace := range clusterMap {
					// The name will be populated in the reconcile
					reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{Namespace: namespace}})
				}

				return reqs
			}),
		},
	)
	if err != nil {
		return err
	}

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
