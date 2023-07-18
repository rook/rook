/*
Copyright 2023 The Rook Authors. All rights reserved.

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

// Package cosi implements the controller for the Ceph COSI Driver.
package cosi

import (
	"context"
	"os"
	"reflect"
	"time"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/reporting"
	"github.com/rook/rook/pkg/operator/k8sutil"
	appsv1 "k8s.io/api/apps/v1"
	kapiv1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	packageName               = "ceph-cosi"
	controllerName            = "ceph-cosi-controller"
	CephCOSIDriverName        = "ceph-cosi-driver"
	COSISideCarName           = "objectstorage-provisioner-sidecar"
	cosiSocketMountPath       = "/var/lib/cosi"
	DefaultServiceAccountName = "objectstorage-provisioner"
	cosiSocketVolumeName      = "socket"
)

var (
	logger                              = capnslog.NewPackageLogger("github.com/rook/rook", controllerName)
	waitForRequeueObjectStoreNotPresent = reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}
)

// ReconcileCephCOSIDriver reconciles the Ceph COSI Driver
type ReconcileCephCOSIDriver struct {
	client           client.Client
	context          *clusterd.Context
	scheme           *runtime.Scheme
	opManagerContext context.Context
	recorder         record.EventRecorder
}

// Add creates a new CephCOSIDriver Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context, opConfig opcontroller.OperatorConfig) error {
	return add(mgr, newReconciler(mgr, context, opManagerContext))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context) reconcile.Reconciler {
	return &ReconcileCephCOSIDriver{
		client:           mgr.GetClient(),
		context:          context,
		scheme:           mgr.GetScheme(),
		opManagerContext: opManagerContext,
		recorder:         mgr.GetEventRecorderFor(controllerName),
	}
}

func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	controller, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return errors.Wrapf(err, "failed to create %s controller", controllerName)
	}

	logger.Info("successfully started")
	// Watch for changes to CephCOSIDriver
	err = controller.Watch(source.Kind(mgr.GetCache(), &cephv1.CephCOSIDriver{}), &handler.EnqueueRequestForObject{}, opcontroller.WatchControllerPredicate())
	if err != nil {
		return errors.Wrap(err, "failed to watch for CephCOSIDriver object changes")
	}

	// Watch for changes to CephObjectStore as arbitrary resource and predicate functions
	err = controller.Watch(source.Kind(mgr.GetCache(), &cephv1.CephObjectStore{}), &handler.EnqueueRequestForObject{}, opcontroller.WatchControllerPredicate())
	if err != nil {
		return errors.Wrap(err, "failed to watch for CephObjectStore object changes")
	}

	return nil
}

// Reconcile reads that state of the cluster for a CephCOSIDriver object and makes changes based on the state read
// and what is in the CephCOSIDriver.Spec
func (r *ReconcileCephCOSIDriver) Reconcile(context context.Context, request reconcile.Request) (reconcile.Result, error) {
	reconcileResponse, cephCOSIDriver, err := r.reconcile(request)

	return reporting.ReportReconcileResult(logger, r.recorder, request, &cephCOSIDriver, reconcileResponse, err)
}

func (r *ReconcileCephCOSIDriver) reconcile(request reconcile.Request) (reconcile.Result, cephv1.CephCOSIDriver, error) {
	cephCOSIDriver := &cephv1.CephCOSIDriver{}

	// Fetch the CephCOSIDriver instance
	err := r.client.Get(r.opManagerContext, request.NamespacedName, cephCOSIDriver)
	logger.Debugf("CephCOSIDriver: %+v", cephCOSIDriver)
	if err != nil && client.IgnoreNotFound(err) != nil {
		return reconcile.Result{}, *cephCOSIDriver, errors.Wrapf(err, "failed to get Ceph COSI Driver %s", request.NamespacedName)
	}

	// While in experimental mode, the COSI driver is not enabled by default
	cosiDeploymentStrategy := cephv1.COSIDeploymentStrategyNever

	// Get the setting from the CephCOSIDriver CR if exists
	if !reflect.DeepEqual(cephCOSIDriver.Spec, cephv1.CephCOSIDriverSpec{}) && cephCOSIDriver.Spec.DeploymentStrategy != "" {
		cosiDeploymentStrategy = cephCOSIDriver.Spec.DeploymentStrategy
	}

	if cosiDeploymentStrategy == cephv1.COSIDeploymentStrategyNever {
		logger.Debug("Ceph COSI Driver is disabled, delete if exists")
		cephCOSIDriverDeployment := &appsv1.Deployment{}
		err = r.client.Get(r.opManagerContext, request.NamespacedName, cephCOSIDriverDeployment)
		if kerrors.IsNotFound(err) {
			// nothing to delete
			return reconcile.Result{}, *cephCOSIDriver, nil
		}
		if err != nil && client.IgnoreNotFound(err) != nil {
			return reconcile.Result{}, *cephCOSIDriver, errors.Wrap(err, "failed to get Ceph COSI Driver Deployment")
		}
		err = r.client.Delete(r.opManagerContext, cephCOSIDriverDeployment)
		if err != nil && client.IgnoreNotFound(err) != nil {
			return reconcile.Result{}, *cephCOSIDriver, errors.Wrap(err, "failed to delete Ceph COSI Driver Deployment")
		}
		return reconcile.Result{}, *cephCOSIDriver, nil
	}

	r.recorder.Eventf(cephCOSIDriver, kapiv1.EventTypeNormal, "Reconciling", "Reconciling Ceph COSI Driver %q", cephCOSIDriver.Name)
	// Set the default CephCOSIDriver name if not already set
	if cephCOSIDriver.Name == "" {
		cephCOSIDriver.Name = CephCOSIDriverName
	}

	// Set the default CephCOSIDriver namespace if not already set
	if cephCOSIDriver.Namespace == "" {
		cephCOSIDriver.Namespace = os.Getenv(k8sutil.PodNamespaceEnvVar)
	}

	// Check whether object store is running
	list := &cephv1.CephObjectStoreList{}
	err = r.client.List(r.opManagerContext, list)
	if err != nil {
		return reconcile.Result{}, *cephCOSIDriver, errors.Wrap(err, "failed to list CephObjectStore")
	}

	if len(list.Items) == 0 && cosiDeploymentStrategy == cephv1.COSIDeploymentStrategyAuto {
		logger.Info("no object stores found, skipping cosi driver config")
		return waitForRequeueObjectStoreNotPresent, *cephCOSIDriver, nil
	}

	// Start the Ceph COSI Driver
	err = r.startCephCOSIDriver(cephCOSIDriver)
	if err != nil {
		return reconcile.Result{}, *cephCOSIDriver, errors.Wrap(err, "failed to start Ceph COSI Driver")
	}

	return reconcile.Result{}, *cephCOSIDriver, nil
}

// Start the Ceph COSI Driver
func (r *ReconcileCephCOSIDriver) startCephCOSIDriver(cephCOSIDriver *cephv1.CephCOSIDriver) error {
	// Create the Ceph COSI Driver Deployment
	cephcosidriverDeployment, err := createCephCOSIDriverDeployment(cephCOSIDriver)
	if err != nil {
		return errors.Wrap(err, "failed to create Ceph COSI Driver Deployment CRD")
	}

	logger.Infof("starting Ceph COSI Driver deployment %q in namespace %q", cephcosidriverDeployment.Name, cephcosidriverDeployment.Namespace)

	err = r.client.Create(r.opManagerContext, cephcosidriverDeployment)
	if err != nil {
		if !kerrors.IsAlreadyExists(err) {
			return errors.Wrap(err, "failed to create Ceph COSI Driver deployment")
		}
		logger.Info("Ceph COSI Driver deployment already exists, updating")
		err = r.client.Update(r.opManagerContext, cephcosidriverDeployment)
		if err != nil {
			return errors.Wrap(err, "failed to update Ceph COSI Driver deployment")
		}
	} else {
		logger.Info("Ceph COSI Driver deployment started")
	}

	return nil
}
