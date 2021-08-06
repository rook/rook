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

package machinelabel

import (
	mapiv1 "github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"
	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/operator/ceph/disruption/controllerconfig"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	osdPodLabelKey         = "app"
	osdPODLabelValue       = "rook-ceph-osd"
	osdClusterNameLabelKey = "rook_cluster"
)

// Add adds a new Controller to the Manager based on machinelabel.ReconcileMachineLabel and registers the relevant watches and handlers.
// Read more about how Managers, Controllers, and their Watches, Handlers, Predicates, etc work here:
// https://godoc.org/github.com/kubernetes-sigs/controller-runtime/pkg
func Add(mgr manager.Manager, context *controllerconfig.Context) error {
	reconcileMachineLabel := &ReconcileMachineLabel{
		client:  mgr.GetClient(),
		scheme:  mgr.GetScheme(),
		options: context,
	}

	reconciler := reconcile.Reconciler(reconcileMachineLabel)
	// create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: reconciler})
	if err != nil {
		return errors.Wrapf(err, "could not create controller %q", controllerName)
	}

	// Watch for the machines and enqueue the machineRequests if the machine is occupied by the osd pods
	err = c.Watch(&source.Kind{Type: &mapiv1.Machine{}}, handler.EnqueueRequestsFromMapFunc(
		handler.MapFunc(func(obj client.Object) []reconcile.Request {
			clusterNamespace, isNamespacePresent := obj.GetLabels()[MachineFencingNamespaceLabelKey]
			if !isNamespacePresent || len(clusterNamespace) == 0 {
				return []reconcile.Request{}
			}
			clusterName, isClusterNamePresent := obj.GetLabels()[MachineFencingLabelKey]
			if !isClusterNamePresent || len(clusterName) == 0 {
				return []reconcile.Request{}
			}
			req := reconcile.Request{NamespacedName: types.NamespacedName{Name: clusterName, Namespace: clusterNamespace}}
			return []reconcile.Request{req}
		}),
	))
	if err != nil {
		return errors.Wrap(err, "could not watch machines")
	}

	// Watch for the osd pods and enqueue the CephCluster in the namespace from the pods
	return c.Watch(&source.Kind{Type: &corev1.Pod{}}, handler.EnqueueRequestsFromMapFunc(
		handler.MapFunc(func(obj client.Object) []reconcile.Request {
			_, ok := obj.(*corev1.Pod)
			if !ok {
				return []reconcile.Request{}
			}
			labels := obj.GetLabels()
			if value, present := labels[osdPodLabelKey]; !present || value != osdPODLabelValue {
				return []reconcile.Request{}
			}
			namespace := obj.GetNamespace()
			rookClusterName, present := labels[osdClusterNameLabelKey]
			if !present {
				return []reconcile.Request{}
			}
			req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: rookClusterName}}
			return []reconcile.Request{req}
		}),
	))
}
