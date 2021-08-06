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

package machinedisruption

import (
	healthchecking "github.com/openshift/machine-api-operator/pkg/apis/healthchecking/v1alpha1"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/ceph/disruption/controllerconfig"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// Add adds a new Controller to the Manager based on machinedisruption.ReconcileMachineDisruption and registers the relevant watches and handlers.
// Read more about how Managers, Controllers, and their Watches, Handlers, Predicates, etc work here:
// https://godoc.org/github.com/kubernetes-sigs/controller-runtime/pkg
func Add(mgr manager.Manager, ctx *controllerconfig.Context) error {
	reconcileMachineDisruption := &MachineDisruptionReconciler{
		client:  mgr.GetClient(),
		scheme:  mgr.GetScheme(),
		context: ctx,
	}

	// TODO CHANGE ME (the context)
	reconciler := reconcile.Reconciler(reconcileMachineDisruption)
	// create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: reconciler})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &cephv1.CephCluster{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return c.Watch(&source.Kind{Type: &healthchecking.MachineDisruptionBudget{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &cephv1.CephCluster{},
	})
}
