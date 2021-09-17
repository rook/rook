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

package agent

import (
	"context"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/agent/flexvolume"
	opagent "github.com/rook/rook/pkg/operator/ceph/agent"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/provisioner"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	flexcontroller "sigs.k8s.io/sig-storage-lib-external-provisioner/v6/controller"
)

const (
	controllerName        = "rook-ceph-operator-flex-controller"
	provisionerName       = "ceph.rook.io/block"
	provisionerNameLegacy = "rook.io/block"
)

var (
	// The supported configurations for the volume provisioner
	provisionerConfigs = map[string]string{
		provisionerName:       flexvolume.FlexvolumeVendor,
		provisionerNameLegacy: flexvolume.FlexvolumeVendorLegacy,
	}
)

// ReconcileAgent reconciles the Flex driver
type ReconcileAgent struct {
	client           client.Client
	context          *clusterd.Context
	opManagerContext context.Context
	opConfig         opcontroller.OperatorConfig
}

// Add creates a new CephClient Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context, opConfig opcontroller.OperatorConfig) error {
	return add(mgr, newReconciler(mgr, context, opManagerContext, opConfig))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context, opConfig opcontroller.OperatorConfig) reconcile.Reconciler {
	return &ReconcileAgent{
		client:           mgr.GetClient(),
		context:          context,
		opConfig:         opConfig,
		opManagerContext: opManagerContext,
	}
}

func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}
	logger.Infof("%s successfully started", controllerName)

	// Watch for ConfigMap (operator config)
	err = c.Watch(&source.Kind{
		Type: &v1.ConfigMap{TypeMeta: metav1.TypeMeta{Kind: "ConfigMap", APIVersion: v1.SchemeGroupVersion.String()}}}, &handler.EnqueueRequestForObject{}, predicateController())
	if err != nil {
		return err
	}

	return nil
}

// Reconcile reads that state of the operator config map and makes changes based on the state read
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileAgent) Reconcile(context context.Context, request reconcile.Request) (reconcile.Result, error) {
	// workaround because the rook logging mechanism is not compatible with the controller-runtime logging interface
	reconcileResponse, err := r.reconcile(request)
	if err != nil {
		logger.Errorf("failed to reconcile %v", err)
	}

	return reconcileResponse, err
}

func (r *ReconcileAgent) reconcile(request reconcile.Request) (reconcile.Result, error) {
	if opcontroller.FlexDriverEnabled(r.context) {
		rookAgent := opagent.New(r.context.Clientset)
		if err := rookAgent.Start(r.opConfig.OperatorNamespace, r.opConfig.Image, r.opConfig.ServiceAccount); err != nil {
			return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed to start agent daemonset")
		}

		serverVersion, err := r.context.Clientset.Discovery().ServerVersion()
		if err != nil {
			return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed to get server version")
		}
		for name, vendor := range provisionerConfigs {
			volumeProvisioner := provisioner.New(r.context, vendor)
			pc := flexcontroller.NewProvisionController(
				r.context.Clientset,
				name,
				volumeProvisioner,
				serverVersion.GitVersion,
			)
			// TODO: register each go routine!!!
			go pc.Run(r.opManagerContext)
			logger.Infof("rook-provisioner %q started using %q flex vendor dir", name, vendor)
		}
	}

	logger.Info("done reconciling")
	return reconcile.Result{}, nil
}
