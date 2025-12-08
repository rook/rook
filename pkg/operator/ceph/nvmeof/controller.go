/*
Copyright 2025 The Rook Authors. All rights reserved.

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

package nvmeof

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	csiopv1 "github.com/ceph/ceph-csi-operator/api/v1"
	"github.com/coreos/pkg/capnslog"
	addonsv1alpha1 "github.com/csi-addons/kubernetes-csi-addons/api/csiaddons/v1alpha1"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/reporting"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"gopkg.in/ini.v1"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	packageName                   = "ceph-nvmeof-gateway"
	controllerName                = packageName + "-controller"
	CephNVMeOFGatewayNameLabelKey = "ceph_nvmeof_gateway"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", packageName)

var objectsToWatch = []client.Object{
	&v1.Service{TypeMeta: metav1.TypeMeta{Kind: "Service", APIVersion: v1.SchemeGroupVersion.String()}},
	&appsv1.Deployment{TypeMeta: metav1.TypeMeta{Kind: "Deployment", APIVersion: appsv1.SchemeGroupVersion.String()}},
}

var controllerTypeMeta = metav1.TypeMeta{
	Kind:       reflect.TypeFor[cephv1.CephNVMeOFGateway]().Name(),
	APIVersion: fmt.Sprintf("%s/%s", cephv1.CustomResourceGroup, cephv1.Version),
}

var currentAndDesiredCephVersion = opcontroller.CurrentAndDesiredCephVersion

type ReconcileCephNVMeOFGateway struct {
	client                client.Client
	scheme                *runtime.Scheme
	context               *clusterd.Context
	cephClusterSpec       *cephv1.ClusterSpec
	clusterInfo           *cephclient.ClusterInfo
	opManagerContext      context.Context
	opConfig              opcontroller.OperatorConfig
	recorder              record.EventRecorder
	shouldRotateCephxKeys bool
}

// Add creates a new CephNVMeOFGateway Controller and adds it to the Manager.
func Add(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context, opConfig opcontroller.OperatorConfig) error {
	logger.Debug("Add() called: initializing CephNVMeOFGateway controller")
	logger.Debugf("Add() context: operator namespace=%s", opConfig.OperatorNamespace)
	return add(mgr, newReconciler(mgr, context, opManagerContext, opConfig))
}

func newReconciler(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context, opConfig opcontroller.OperatorConfig) reconcile.Reconciler {
	logger.Debug("newReconciler() called: creating new ReconcileCephNVMeOFGateway instance")
	logger.Debugf("newReconciler() operator config: namespace=%s", opConfig.OperatorNamespace)
	reconciler := &ReconcileCephNVMeOFGateway{
		client:           mgr.GetClient(),
		scheme:           mgr.GetScheme(),
		context:          context,
		opManagerContext: opManagerContext,
		opConfig:         opConfig,
		recorder:         mgr.GetEventRecorderFor("rook-" + controllerName),
	}
	logger.Debug("newReconciler() completed: reconciler instance created successfully")
	return reconciler
}

func watchOwnedCoreObject[T client.Object](c controller.Controller, mgr manager.Manager, obj T) error {
	return c.Watch(
		source.Kind(
			mgr.GetCache(),
			obj,
			handler.TypedEnqueueRequestForOwner[T](
				mgr.GetScheme(),
				mgr.GetRESTMapper(),
				&cephv1.CephNVMeOFGateway{},
			),
			opcontroller.WatchPredicateForNonCRDObject[T](&cephv1.CephNVMeOFGateway{TypeMeta: controllerTypeMeta}, mgr.GetScheme()),
		),
	)
}

func add(mgr manager.Manager, r reconcile.Reconciler) error {
	logger.Debug("add() called: setting up controller and watches")

	logger.Debugf("add() creating controller with name: %s", controllerName)
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		logger.Errorf("add() failed to create controller: %v", err)
		return err
	}
	logger.Debug("add() controller created successfully")
	logger.Info("successfully started")

	logger.Debug("add() adding addonsv1alpha1 to scheme")
	err = addonsv1alpha1.AddToScheme(mgr.GetScheme())
	if err != nil {
		logger.Errorf("add() failed to add addonsv1alpha1 to scheme: %v", err)
		return err
	}
	logger.Debug("add() addonsv1alpha1 added to scheme successfully")

	logger.Debug("add() adding csiopv1 to scheme")
	err = csiopv1.AddToScheme(mgr.GetScheme())
	if err != nil {
		logger.Errorf("add() failed to add csiopv1 to scheme: %v", err)
		return err
	}
	logger.Debug("add() csiopv1 added to scheme successfully")

	logger.Debug("add() setting up watch for CephNVMeOFGateway CRD")
	err = c.Watch(
		source.Kind(
			mgr.GetCache(),
			&cephv1.CephNVMeOFGateway{TypeMeta: controllerTypeMeta},
			&handler.TypedEnqueueRequestForObject[*cephv1.CephNVMeOFGateway]{},
			opcontroller.WatchControllerPredicate[*cephv1.CephNVMeOFGateway](mgr.GetScheme()),
		),
	)
	if err != nil {
		logger.Errorf("add() failed to watch CephNVMeOFGateway CRD: %v", err)
		return err
	}
	logger.Debug("add() watch for CephNVMeOFGateway CRD set up successfully")

	logger.Debugf("add() setting up watches for %d additional resource types", len(objectsToWatch))
	for i, t := range objectsToWatch {
		logger.Debugf("add() setting up watch for resource type %d: %s", i+1, t.GetObjectKind().GroupVersionKind().String())
		err = watchOwnedCoreObject(c, mgr, t)
		if err != nil {
			logger.Errorf("add() failed to watch resource type %s: %v", t.GetObjectKind().GroupVersionKind().String(), err)
			return err
		}
		logger.Debugf("add() watch for resource type %s set up successfully", t.GetObjectKind().GroupVersionKind().String())
	}

	logger.Debug("add() completed: all watches configured successfully")
	return nil
}

// Reconcile reads the state of the cluster for a CephNVMeOFGateway object and makes changes based on the state read.
func (r *ReconcileCephNVMeOFGateway) Reconcile(context context.Context, request reconcile.Request) (reconcile.Result, error) {
	logger.Debugf("Reconcile() called: namespace=%s, name=%s", request.Namespace, request.Name)
	defer opcontroller.RecoverAndLogException()
	reconcileResponse, cephNVMeOFGateway, err := r.reconcile(request)
	logger.Debugf("Reconcile() reconcile() returned: requeueAfter=%v, error=%v", reconcileResponse.RequeueAfter, err)
	result, reportErr := reporting.ReportReconcileResult(logger, r.recorder, request, &cephNVMeOFGateway, reconcileResponse, err)
	logger.Debugf("Reconcile() completed: final result requeueAfter=%v", result.RequeueAfter)
	return result, reportErr
}

func (r *ReconcileCephNVMeOFGateway) reconcile(request reconcile.Request) (reconcile.Result, cephv1.CephNVMeOFGateway, error) {
	logger.Debugf("reconcile() started: fetching CephNVMeOFGateway %s/%s", request.Namespace, request.Name)

	cephNVMeOFGateway := &cephv1.CephNVMeOFGateway{}
	err := r.client.Get(r.opManagerContext, request.NamespacedName, cephNVMeOFGateway)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debugf("reconcile() CephNVMeOFGateway %s/%s not found. Ignoring since object must be deleted.", request.Namespace, request.Name)
			return reconcile.Result{}, *cephNVMeOFGateway, nil
		}
		logger.Errorf("reconcile() failed to get CephNVMeOFGateway %s/%s: %v", request.Namespace, request.Name, err)
		return reconcile.Result{}, *cephNVMeOFGateway, errors.Wrap(err, "failed to get CephNVMeOFGateway")
	}

	logger.Debugf("reconcile() successfully fetched CephNVMeOFGateway: name=%s, namespace=%s, generation=%d",
		cephNVMeOFGateway.Name, cephNVMeOFGateway.Namespace, cephNVMeOFGateway.ObjectMeta.Generation)
	logger.Debugf("reconcile() spec: instances=%d, pool=%s, group=%s",
		cephNVMeOFGateway.Spec.Instances, cephNVMeOFGateway.Spec.Pool, cephNVMeOFGateway.Spec.Group)

	observedGeneration := cephNVMeOFGateway.ObjectMeta.Generation
	logger.Debugf("reconcile() observed generation: %d", observedGeneration)

	logger.Debug("reconcile() checking and adding finalizer if needed")
	generationUpdated, err := opcontroller.AddFinalizerIfNotPresent(r.opManagerContext, r.client, cephNVMeOFGateway)
	if err != nil {
		logger.Errorf("reconcile() failed to add finalizer: %v", err)
		return reconcile.Result{}, *cephNVMeOFGateway, errors.Wrap(err, "failed to add finalizer")
	}
	if generationUpdated {
		logger.Debug("reconcile() finalizer was added, generation updated, requeuing")
		return reconcile.Result{}, *cephNVMeOFGateway, nil
	}
	logger.Debug("reconcile() finalizer check completed, no update needed")

	if cephNVMeOFGateway.Status == nil {
		logger.Debug("reconcile() status is nil, initializing status with empty/uninitialized values")
		cephxUninitialized := keyring.UninitializedCephxStatus()
		err := r.updateStatus(k8sutil.ObservedGenerationNotAvailable, request.NamespacedName, &cephxUninitialized, k8sutil.EmptyStatus)
		if err != nil {
			logger.Errorf("reconcile() failed to set empty status: %v", err)
			return opcontroller.ImmediateRetryResult, *cephNVMeOFGateway, errors.Wrapf(err, "failed set empty status")
		}
		cephNVMeOFGateway.Status = &cephv1.NVMeOFGatewayStatus{
			Status: cephv1.Status{},
			Cephx:  cephv1.LocalCephxStatus{Daemon: cephxUninitialized},
		}
		logger.Debug("reconcile() status initialized successfully")
	} else {
		logger.Debugf("reconcile() status exists: phase=%s, observedGeneration=%d",
			cephNVMeOFGateway.Status.Phase, cephNVMeOFGateway.Status.ObservedGeneration)
	}

	logger.Debug("reconcile() checking if cluster is ready to reconcile")
	cephCluster, isReadyToReconcile, cephClusterExists, reconcileResponse := opcontroller.IsReadyToReconcile(r.opManagerContext, r.client, request.NamespacedName, controllerName)
	logger.Debugf("reconcile() cluster readiness check: isReady=%v, clusterExists=%v, requeueAfter=%v",
		isReadyToReconcile, cephClusterExists, reconcileResponse.RequeueAfter)

	if !isReadyToReconcile {
		logger.Debug("reconcile() cluster is not ready to reconcile")
		if !cephNVMeOFGateway.GetDeletionTimestamp().IsZero() && !cephClusterExists {
			logger.Debug("reconcile() gateway is being deleted and cluster doesn't exist, removing finalizer")
			err := opcontroller.RemoveFinalizer(r.opManagerContext, r.client, cephNVMeOFGateway)
			if err != nil {
				logger.Errorf("reconcile() failed to remove finalizer: %v", err)
				return reconcile.Result{}, *cephNVMeOFGateway, errors.Wrap(err, "failed to remove finalizer")
			}
			logger.Debug("reconcile() finalizer removed successfully")
			r.recorder.Event(cephNVMeOFGateway, v1.EventTypeNormal, string(cephv1.ReconcileSucceeded), "successfully removed finalizer")
			return reconcile.Result{}, *cephNVMeOFGateway, nil
		}
		logger.Debugf("reconcile() requeuing because cluster is not ready: requeueAfter=%v",
			reconcileResponse.RequeueAfter)
		return reconcileResponse, *cephNVMeOFGateway, nil
	}
	logger.Debug("reconcile() cluster is ready, proceeding with reconciliation")
	r.cephClusterSpec = &cephCluster.Spec
	logger.Debugf("reconcile() ceph cluster spec loaded: namespace=%s, external=%v",
		cephCluster.Namespace, cephCluster.Spec.External.Enable)

	logger.Debug("reconcile() loading cluster info")
	r.clusterInfo, _, _, err = opcontroller.LoadClusterInfo(r.context, r.opManagerContext, request.NamespacedName.Namespace, r.cephClusterSpec)
	if err != nil {
		logger.Errorf("reconcile() failed to load cluster info: %v", err)
		return reconcile.Result{}, *cephNVMeOFGateway, errors.Wrap(err, "failed to populate cluster info")
	}
	logger.Debugf("reconcile() cluster info loaded: fsid=%s, namespace=%s",
		r.clusterInfo.FSID, r.clusterInfo.Namespace)

	if !cephNVMeOFGateway.GetDeletionTimestamp().IsZero() {
		logger.Debugf("reconcile() deletion timestamp detected for gateway %q, processing deletion", cephNVMeOFGateway.Name)
		logger.Infof("deleting ceph nvmeof gateway %q", cephNVMeOFGateway.Name)
		r.recorder.Eventf(cephNVMeOFGateway, v1.EventTypeNormal, string(cephv1.ReconcileStarted), "deleting CephNVMeOFGateway %q", cephNVMeOFGateway.Name)

		logger.Debug("reconcile() retrieving current ceph version for deletion")
		runningCephVersion, err := cephclient.LeastUptodateDaemonVersion(r.context, r.clusterInfo, config.MonType)
		if err != nil {
			logger.Errorf("reconcile() failed to retrieve ceph version: %v", err)
			return reconcile.Result{}, *cephNVMeOFGateway, errors.Wrapf(err, "failed to retrieve current ceph version")
		}
		logger.Debugf("reconcile() current ceph version: %s", runningCephVersion.String())
		r.clusterInfo.CephVersion = runningCephVersion

		logger.Debug("reconcile() removing finalizer during deletion")
		err = opcontroller.RemoveFinalizer(r.opManagerContext, r.client, cephNVMeOFGateway)
		if err != nil {
			logger.Errorf("reconcile() failed to remove finalizer during deletion: %v", err)
			return reconcile.Result{}, *cephNVMeOFGateway, errors.Wrap(err, "failed to remove finalizer")
		}
		logger.Debug("reconcile() finalizer removed successfully during deletion")
		r.recorder.Event(cephNVMeOFGateway, v1.EventTypeNormal, string(cephv1.ReconcileSucceeded), "successfully removed finalizer")
		return reconcile.Result{}, *cephNVMeOFGateway, nil
	}
	logger.Debug("reconcile() gateway is not being deleted, proceeding with normal reconciliation")

	logger.Debug("reconcile() detecting ceph versions (running vs desired)")
	runningCephVersion, desiredCephVersion, err := currentAndDesiredCephVersion(
		r.opManagerContext, r.opConfig.Image, cephNVMeOFGateway.Namespace, controllerName,
		k8sutil.NewOwnerInfo(cephNVMeOFGateway, r.scheme), r.context, r.cephClusterSpec, r.clusterInfo,
	)
	if err != nil {
		if strings.Contains(err.Error(), opcontroller.UninitializedCephConfigError) {
			logger.Debug("reconcile() operator not initialized, requeuing")
			logger.Info(opcontroller.OperatorNotInitializedMessage)
			return opcontroller.WaitForRequeueIfOperatorNotInitialized, *cephNVMeOFGateway, nil
		}
		logger.Errorf("reconcile() failed to detect ceph version: %v", err)
		return reconcile.Result{}, *cephNVMeOFGateway, errors.Wrap(err, "failed to detect ceph version")
	}
	logger.Debugf("reconcile() ceph versions detected: running=%s, desired=%s",
		runningCephVersion.String(), desiredCephVersion.String())

	if !cephCluster.Spec.External.Enable && !reflect.DeepEqual(*runningCephVersion, *desiredCephVersion) {
		logger.Debugf("reconcile() ceph cluster is upgrading: running=%s != desired=%s, requeuing",
			runningCephVersion.String(), desiredCephVersion.String())
		return opcontroller.WaitForRequeueIfCephClusterIsUpgrading, *cephNVMeOFGateway,
			opcontroller.ErrorCephUpgradingRequeue(desiredCephVersion, runningCephVersion)
	}
	logger.Debug("reconcile() ceph versions match or external cluster, proceeding")
	r.clusterInfo.CephVersion = *runningCephVersion

	logger.Debug("reconcile() validating gateway specification")
	if err := validateGateway(cephNVMeOFGateway); err != nil {
		logger.Errorf("reconcile() gateway validation failed: %v", err)
		return reconcile.Result{}, *cephNVMeOFGateway, errors.Wrapf(err, "invalid configuration")
	}
	logger.Debug("reconcile() gateway specification is valid")

	logger.Debug("reconcile() checking if cephx keys should be rotated")
	r.shouldRotateCephxKeys, err = keyring.ShouldRotateCephxKeys(cephCluster.Spec.Security.CephX.Daemon, *runningCephVersion,
		*desiredCephVersion, cephNVMeOFGateway.Status.Cephx.Daemon)
	if err != nil {
		logger.Errorf("reconcile() failed to determine if cephx keys should be rotated: %v", err)
		return reconcile.Result{}, *cephNVMeOFGateway, errors.Wrap(err, "failed to determine if cephx keys should be rotated")
	}
	logger.Debugf("reconcile() cephx key rotation check: shouldRotate=%v", r.shouldRotateCephxKeys)
	if r.shouldRotateCephxKeys {
		logger.Infof("cephx keys will be rotated for %q", request.NamespacedName)
		logger.Debugf("reconcile() cephx key rotation will be performed for gateway %s/%s",
			request.Namespace, request.Name)
	}

	logger.Debug("reconcile() starting to reconcile ceph nvmeof gateway deployments")
	_, err = r.reconcileCreateCephNVMeOFGateway(cephNVMeOFGateway)
	if err != nil {
		logger.Errorf("reconcile() failed to reconcile gateway deployments: %v", err)
		return reconcile.Result{}, *cephNVMeOFGateway, errors.Wrap(err, "failed to create deployments")
	}
	logger.Debug("reconcile() gateway deployments reconciled successfully")

	logger.Debug("reconcile() updating gateway status to Ready")
	cephxStatus := keyring.UpdatedCephxStatus(r.shouldRotateCephxKeys, cephCluster.Spec.Security.CephX.Daemon, r.clusterInfo.CephVersion, cephNVMeOFGateway.Status.Cephx.Daemon)
	err = r.updateStatus(observedGeneration, request.NamespacedName, &cephxStatus, k8sutil.ReadyStatus)
	if err != nil {
		logger.Errorf("reconcile() failed to update status: %v", err)
		return opcontroller.ImmediateRetryResult, *cephNVMeOFGateway, errors.Wrapf(err, "failed to update status")
	}
	logger.Debugf("reconcile() status updated successfully: phase=Ready, observedGeneration=%d", observedGeneration)

	logger.Debug("reconcile() completed successfully")
	return reconcile.Result{}, *cephNVMeOFGateway, nil
}

func (r *ReconcileCephNVMeOFGateway) reconcileCreateCephNVMeOFGateway(cephNVMeOFGateway *cephv1.CephNVMeOFGateway) (reconcile.Result, error) {
	logger.Debugf("reconcileCreateCephNVMeOFGateway() started: gateway=%s/%s, desiredInstances=%d",
		cephNVMeOFGateway.Namespace, cephNVMeOFGateway.Name, cephNVMeOFGateway.Spec.Instances)

	if r.cephClusterSpec.External.Enable {
		logger.Debug("reconcileCreateCephNVMeOFGateway() external cluster enabled, validating versions")
		_, err := opcontroller.ValidateCephVersionsBetweenLocalAndExternalClusters(r.context, r.clusterInfo)
		if err != nil {
			logger.Errorf("reconcileCreateCephNVMeOFGateway() version validation failed: %v", err)
			return reconcile.Result{}, errors.Wrap(err, "refusing to run new crd")
		}
		logger.Debug("reconcileCreateCephNVMeOFGateway() version validation passed")
	}

	logger.Debug("reconcileCreateCephNVMeOFGateway() listing existing deployments")
	listOps := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s,%s=%s", k8sutil.AppAttr, AppName, CephNVMeOFGatewayNameLabelKey, cephNVMeOFGateway.Name),
	}
	logger.Debugf("reconcileCreateCephNVMeOFGateway() label selector: %s", listOps.LabelSelector)
	deployments, err := r.context.Clientset.AppsV1().Deployments(cephNVMeOFGateway.Namespace).List(r.opManagerContext, listOps)
	if err != nil && !kerrors.IsNotFound(err) {
		logger.Errorf("reconcileCreateCephNVMeOFGateway() failed to list deployments: %v", err)
		return reconcile.Result{}, errors.Wrapf(err, "failed to list deployments")
	}

	currentGatewayCount := 0
	if deployments != nil {
		currentGatewayCount = len(deployments.Items)
		logger.Debugf("reconcileCreateCephNVMeOFGateway() found %d existing deployments", currentGatewayCount)
		for i, dep := range deployments.Items {
			logger.Debugf("reconcileCreateCephNVMeOFGateway() deployment %d: name=%s", i+1, dep.Name)
		}
	} else {
		logger.Debug("reconcileCreateCephNVMeOFGateway() no existing deployments found")
	}

	logger.Debugf("reconcileCreateCephNVMeOFGateway() current count=%d, desired count=%d",
		currentGatewayCount, cephNVMeOFGateway.Spec.Instances)

	if currentGatewayCount > cephNVMeOFGateway.Spec.Instances {
		logger.Debugf("reconcileCreateCephNVMeOFGateway() scaling down required: %d -> %d",
			currentGatewayCount, cephNVMeOFGateway.Spec.Instances)
		logger.Infof("scaling down from %d to %d", currentGatewayCount, cephNVMeOFGateway.Spec.Instances)
		err := r.downCephNVMeOFGateway(cephNVMeOFGateway, currentGatewayCount)
		if err != nil {
			logger.Errorf("reconcileCreateCephNVMeOFGateway() scale down failed: %v", err)
			return reconcile.Result{}, errors.Wrapf(err, "failed to scale down")
		}
		logger.Debug("reconcileCreateCephNVMeOFGateway() scale down completed successfully")
	} else if currentGatewayCount < cephNVMeOFGateway.Spec.Instances {
		logger.Debugf("reconcileCreateCephNVMeOFGateway() scaling up required: %d -> %d",
			currentGatewayCount, cephNVMeOFGateway.Spec.Instances)
	} else {
		logger.Debug("reconcileCreateCephNVMeOFGateway() no scaling needed, counts match")
	}

	logger.Debug("reconcileCreateCephNVMeOFGateway() ensuring gateway deployments are up")
	err = r.upCephNVMeOFGateway(cephNVMeOFGateway)
	if err != nil {
		logger.Errorf("reconcileCreateCephNVMeOFGateway() failed to ensure gateway is up: %v", err)
		return reconcile.Result{}, errors.Wrapf(err, "failed to update gateway")
	}
	logger.Debug("reconcileCreateCephNVMeOFGateway() gateway deployments ensured successfully")

	logger.Debug("reconcileCreateCephNVMeOFGateway() completed successfully")
	return reconcile.Result{}, nil
}

func (r *ReconcileCephNVMeOFGateway) upCephNVMeOFGateway(cephNVMeOFGateway *cephv1.CephNVMeOFGateway) error {
	logger.Debugf("upCephNVMeOFGateway() started: gateway=%s/%s, instances=%d",
		cephNVMeOFGateway.Namespace, cephNVMeOFGateway.Name, cephNVMeOFGateway.Spec.Instances)

	logger.Debugf("upCephNVMeOFGateway() creating %d gateway instances", cephNVMeOFGateway.Spec.Instances)
	for i := 0; i < cephNVMeOFGateway.Spec.Instances; i++ {
		daemonID := fmt.Sprintf("%d", i)
		logger.Debugf("upCephNVMeOFGateway() processing instance %d (daemonID=%s)", i+1, daemonID)

		var configMapName, configHash string
		var err error

		if cephNVMeOFGateway.Spec.ConfigMapRef == "" {
			logger.Debugf("upCephNVMeOFGateway() no ConfigMapRef specified, creating default configmap for daemonID=%s", daemonID)
			configMapName, configHash, err = r.createConfigMap(cephNVMeOFGateway, daemonID)
			if err != nil {
				logger.Errorf("upCephNVMeOFGateway() failed to create configmap for daemonID=%s: %v", daemonID, err)
				return errors.Wrapf(err, "failed to create configmap for %q", daemonID)
			}
			logger.Infof("configmap %q created/updated for nvmeof gateway %q instance %q with hash %q", configMapName, cephNVMeOFGateway.Name, daemonID, configHash)
			logger.Debugf("upCephNVMeOFGateway() configmap created: name=%s, hash=%s", configMapName, configHash)
		} else {
			logger.Debugf("upCephNVMeOFGateway() using custom ConfigMapRef: %s", cephNVMeOFGateway.Spec.ConfigMapRef)
			configMapName = cephNVMeOFGateway.Spec.ConfigMapRef
			configHash = "" // Will be empty for custom configmap
		}

		logger.Debugf("upCephNVMeOFGateway() creating deployment spec for daemonID=%s", daemonID)
		deployment, err := r.makeDeployment(cephNVMeOFGateway, daemonID, configMapName, configHash)
		if err != nil {
			logger.Errorf("upCephNVMeOFGateway() failed to make deployment for daemonID=%s: %v", daemonID, err)
			return errors.Wrapf(err, "failed to make deployment for %q", daemonID)
		}
		logger.Debugf("upCephNVMeOFGateway() deployment spec created: name=%s, namespace=%s",
			deployment.Name, deployment.Namespace)

		logger.Debugf("upCephNVMeOFGateway() setting owner reference on deployment %s", deployment.Name)
		err = controllerutil.SetControllerReference(cephNVMeOFGateway, deployment, r.scheme)
		if err != nil {
			logger.Errorf("upCephNVMeOFGateway() failed to set owner reference on deployment %s: %v", deployment.Name, err)
			return errors.Wrapf(err, "failed to set owner reference for deployment %q", deployment.Name)
		}
		logger.Debugf("upCephNVMeOFGateway() owner reference set successfully on deployment %s", deployment.Name)

		logger.Debugf("upCephNVMeOFGateway() creating/updating deployment %s", deployment.Name)
		_, err = k8sutil.CreateOrUpdateDeployment(r.opManagerContext, r.context.Clientset, deployment)
		if err != nil {
			logger.Errorf("upCephNVMeOFGateway() failed to create/update deployment %s: %v", deployment.Name, err)
			return errors.Wrapf(err, "failed to create/update deployment for %q", daemonID)
		}
		logger.Debugf("upCephNVMeOFGateway() deployment %s created/updated successfully", deployment.Name)

		logger.Debugf("upCephNVMeOFGateway() creating service for daemonID=%s", daemonID)
		err = r.createCephNVMeOFService(cephNVMeOFGateway, daemonID)
		if err != nil {
			logger.Errorf("upCephNVMeOFGateway() failed to create service for daemonID=%s: %v", daemonID, err)
			return errors.Wrapf(err, "failed to create service for %q", daemonID)
		}
		logger.Debugf("upCephNVMeOFGateway() service for daemonID=%s created successfully", daemonID)
	}

	logger.Debugf("upCephNVMeOFGateway() completed successfully: created %d instances", cephNVMeOFGateway.Spec.Instances)
	return nil
}

func (r *ReconcileCephNVMeOFGateway) downCephNVMeOFGateway(cephNVMeOFGateway *cephv1.CephNVMeOFGateway, currentCount int) error {
	logger.Debugf("downCephNVMeOFGateway() started: gateway=%s/%s, currentCount=%d, desiredCount=%d",
		cephNVMeOFGateway.Namespace, cephNVMeOFGateway.Name, currentCount, cephNVMeOFGateway.Spec.Instances)

	deletionsNeeded := currentCount - cephNVMeOFGateway.Spec.Instances
	logger.Debugf("downCephNVMeOFGateway() need to delete %d instances", deletionsNeeded)

	for i := cephNVMeOFGateway.Spec.Instances; i < currentCount; i++ {
		daemonID := fmt.Sprintf("%d", i)
		name := instanceName(cephNVMeOFGateway, daemonID)
		logger.Debugf("downCephNVMeOFGateway() deleting instance %d: daemonID=%s, resourceName=%s", i+1, daemonID, name)

		logger.Debugf("downCephNVMeOFGateway() deleting deployment %s", name)
		err := r.context.Clientset.AppsV1().Deployments(cephNVMeOFGateway.Namespace).Delete(r.opManagerContext, name, metav1.DeleteOptions{})
		if err != nil && !kerrors.IsNotFound(err) {
			logger.Errorf("downCephNVMeOFGateway() failed to delete deployment %s: %v", name, err)
			return errors.Wrapf(err, "failed to delete deployment %q", name)
		}
		if kerrors.IsNotFound(err) {
			logger.Debugf("downCephNVMeOFGateway() deployment %s not found (already deleted)", name)
		} else {
			logger.Debugf("downCephNVMeOFGateway() deployment %s deleted successfully", name)
		}

		logger.Debugf("downCephNVMeOFGateway() deleting service %s", name)
		err = r.context.Clientset.CoreV1().Services(cephNVMeOFGateway.Namespace).Delete(r.opManagerContext, name, metav1.DeleteOptions{})
		if err != nil && !kerrors.IsNotFound(err) {
			logger.Errorf("downCephNVMeOFGateway() failed to delete service %s: %v", name, err)
			return errors.Wrapf(err, "failed to delete service %q", name)
		}
		if kerrors.IsNotFound(err) {
			logger.Debugf("downCephNVMeOFGateway() service %s not found (already deleted)", name)
		} else {
			logger.Debugf("downCephNVMeOFGateway() service %s deleted successfully", name)
		}
	}

	logger.Debugf("downCephNVMeOFGateway() completed successfully: deleted %d instances", deletionsNeeded)
	return nil
}

// getNVMeOFGatewayConfig generates a complete nvmeof.conf configuration file
// with all values filled in (no placeholders). User overrides from nvmeofConfig
// are merged on top of the default configuration.
func getNVMeOFGatewayConfig(poolName, podName, podIP, anaGroup string, userConfig map[string]map[string]string) (string, error) {
	cfg := ini.Empty()
	// Set default [gateway] section
	gatewaySection, err := cfg.NewSection("gateway")
	if err != nil {
		return "", errors.Wrap(err, "failed to create gateway section")
	}
	gatewaySection.Key("name").SetValue(podName)
	gatewaySection.Key("group").SetValue(anaGroup)
	gatewaySection.Key("addr").SetValue(podIP)
	gatewaySection.Key("port").SetValue("5500")
	gatewaySection.Key("enable_auth").SetValue("False")
	gatewaySection.Key("state_update_notify").SetValue("True")
	gatewaySection.Key("state_update_timeout_in_msec").SetValue("2000")
	gatewaySection.Key("state_update_interval_sec").SetValue("5")
	gatewaySection.Key("enable_spdk_discovery_controller").SetValue("False")
	gatewaySection.Key("encryption_key").SetValue("/etc/ceph/encryption.key")
	gatewaySection.Key("rebalance_period_sec").SetValue("7")
	gatewaySection.Key("max_gws_in_grp").SetValue("16")
	gatewaySection.Key("max_ns_to_change_lb_grp").SetValue("8")
	gatewaySection.Key("verify_listener_ip").SetValue("False")
	gatewaySection.Key("enable_monitor_client").SetValue("True")

	// Set default [gateway-logs] section
	gatewayLogsSection, err := cfg.NewSection("gateway-logs")
	if err != nil {
		return "", errors.Wrap(err, "failed to create gateway-logs section")
	}
	gatewayLogsSection.Key("log_level").SetValue("debug")

	// Set default [discovery] section
	discoverySection, err := cfg.NewSection("discovery")
	if err != nil {
		return "", errors.Wrap(err, "failed to create discovery section")
	}
	discoverySection.Key("addr").SetValue("0.0.0.0")
	discoverySection.Key("port").SetValue("8009")

	// Set default [ceph] section
	cephSection, err := cfg.NewSection("ceph")
	if err != nil {
		return "", errors.Wrap(err, "failed to create ceph section")
	}
	cephSection.Key("id").SetValue("admin")
	cephSection.Key("pool").SetValue(poolName)
	cephSection.Key("config_file").SetValue("/etc/ceph/ceph.conf")

	// Set default [mtls] section
	mtlsSection, err := cfg.NewSection("mtls")
	if err != nil {
		return "", errors.Wrap(err, "failed to create mtls section")
	}
	mtlsSection.Key("server_key").SetValue("./server.key")
	mtlsSection.Key("client_key").SetValue("./client.key")
	mtlsSection.Key("server_cert").SetValue("./server.crt")
	mtlsSection.Key("client_cert").SetValue("./client.crt")

	// Set default [spdk] section
	spdkSection, err := cfg.NewSection("spdk")
	if err != nil {
		return "", errors.Wrap(err, "failed to create spdk section")
	}
	spdkSection.Key("bdevs_per_cluster").SetValue("32")
	spdkSection.Key("mem_size").SetValue("4096")
	spdkSection.Key("tgt_path").SetValue("/usr/local/bin/nvmf_tgt")
	spdkSection.Key("timeout").SetValue("60.0")
	spdkSection.Key("rpc_socket").SetValue("/var/tmp/spdk.sock")

	// Set default [monitor] section
	monitorSection, err := cfg.NewSection("monitor")
	if err != nil {
		return "", errors.Wrap(err, "failed to create monitor section")
	}
	monitorSection.Key("port").SetValue("5499")

	// Apply user overrides
	for sectionName, options := range userConfig {
		section := cfg.Section(sectionName)
		if section == nil {
			// Section doesn't exist, create it
			var createErr error
			section, createErr = cfg.NewSection(sectionName)
			if createErr != nil {
				return "", errors.Wrapf(createErr, "failed to create section %q", sectionName)
			}
		}
		for key, value := range options {
			section.Key(key).SetValue(value)
		}
	}

	// Write to string with proper formatting
	var buf strings.Builder
	_, err = cfg.WriteTo(&buf)
	if err != nil {
		return "", errors.Wrap(err, "failed to write config to string")
	}

	return buf.String(), nil
}

func (r *ReconcileCephNVMeOFGateway) generateConfigMap(nvmeof *cephv1.CephNVMeOFGateway, daemonID string) (*v1.ConfigMap, error) {
	logger.Debugf("generateConfigMap() started: gateway=%s/%s, daemonID=%s", nvmeof.Namespace, nvmeof.Name, daemonID)

	poolName := nvmeof.Spec.Pool
	anaGroup := nvmeof.Spec.Group
	podName := instanceName(nvmeof, daemonID)
	// Use placeholder that will be replaced at runtime with actual pod IP
	// The init container will replace @@POD_IP@@ with the actual pod IP
	podIP := "@@POD_IP@@"

	logger.Debugf("generateConfigMap() using pool=%s, group=%s, podName=%s", poolName, anaGroup, podName)

	logger.Debug("generateConfigMap() generating config content")
	configContent, err := getNVMeOFGatewayConfig(poolName, podName, podIP, anaGroup, nvmeof.Spec.NVMeOFConfig)
	if err != nil {
		logger.Errorf("generateConfigMap() failed to generate config: %v", err)
		return nil, errors.Wrap(err, "failed to generate nvmeof config")
	}
	data := map[string]string{
		"config": configContent,
	}
	logger.Debugf("generateConfigMap() config content length: %d bytes", len(configContent))

	configMapName := fmt.Sprintf("rook-ceph-nvmeof-%s-%s-config", nvmeof.Name, daemonID)
	logger.Debugf("generateConfigMap() configmap name: %s", configMapName)

	configMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: nvmeof.Namespace,
			Labels: map[string]string{
				"app":                         AppName,
				CephNVMeOFGatewayNameLabelKey: nvmeof.Name,
				"instance":                    daemonID,
			},
		},
		Data: data,
	}

	logger.Debugf("generateConfigMap() configmap generated: name=%s, namespace=%s, labels=%v",
		configMapName, nvmeof.Namespace, configMap.Labels)
	return configMap, nil
}

func (r *ReconcileCephNVMeOFGateway) createConfigMap(cephNVMeOFGateway *cephv1.CephNVMeOFGateway, daemonID string) (string, string, error) {
	logger.Debugf("createConfigMap() started: gateway=%s/%s, daemonID=%s", cephNVMeOFGateway.Namespace, cephNVMeOFGateway.Name, daemonID)

	logger.Debug("createConfigMap() generating configmap spec")
	configMap, err := r.generateConfigMap(cephNVMeOFGateway, daemonID)
	if err != nil {
		logger.Errorf("createConfigMap() failed to generate configmap: %v", err)
		return "", "", err
	}

	logger.Debugf("createConfigMap() setting owner reference on configmap %s", configMap.Name)
	err = controllerutil.SetControllerReference(cephNVMeOFGateway, configMap, r.scheme)
	if err != nil {
		logger.Errorf("createConfigMap() failed to set owner reference: %v", err)
		return "", "", errors.Wrapf(err, "failed to set owner reference for nvmeof configmap %q", configMap.Name)
	}
	logger.Debug("createConfigMap() owner reference set successfully")

	logger.Debugf("createConfigMap() creating configmap %s in namespace %s", configMap.Name, cephNVMeOFGateway.Namespace)
	if _, err := r.context.Clientset.CoreV1().ConfigMaps(cephNVMeOFGateway.Namespace).Create(r.opManagerContext, configMap, metav1.CreateOptions{}); err != nil {
		if !kerrors.IsAlreadyExists(err) {
			logger.Errorf("createConfigMap() failed to create configmap: %v", err)
			return "", "", errors.Wrap(err, "failed to create nvmeof config map")
		}

		logger.Debugf("createConfigMap() configmap %q already exists, updating it", configMap.Name)
		if _, err = r.context.Clientset.CoreV1().ConfigMaps(cephNVMeOFGateway.Namespace).Update(r.opManagerContext, configMap, metav1.UpdateOptions{}); err != nil {
			logger.Errorf("createConfigMap() failed to update configmap: %v", err)
			return "", "", errors.Wrap(err, "failed to update nvmeof config map")
		}
		logger.Debugf("createConfigMap() configmap %q updated successfully", configMap.Name)
	} else {
		logger.Debugf("createConfigMap() configmap %q created successfully", configMap.Name)
	}

	configHash := k8sutil.Hash(fmt.Sprintf("%v", configMap.Data))
	logger.Debugf("createConfigMap() computed config hash: %s", configHash)
	logger.Debugf("createConfigMap() completed: name=%s, hash=%s", configMap.Name, configHash)
	return configMap.Name, configHash, nil
}

func validateGateway(g *cephv1.CephNVMeOFGateway) error {
	logger.Debugf("validateGateway() started: gateway=%s/%s", g.Namespace, g.Name)
	logger.Debugf("validateGateway() checking instances: value=%d", g.Spec.Instances)
	if g.Spec.Instances < 1 {
		logger.Debug("validateGateway() validation failed: instances < 1")
		return errors.New("at least one gateway instance is required")
	}
	logger.Debugf("validateGateway() instances check passed: %d >= 1", g.Spec.Instances)

	logger.Debugf("validateGateway() checking group: value=%q", g.Spec.Group)
	if g.Spec.Group == "" {
		logger.Debug("validateGateway() validation failed: group is empty")
		return errors.New("gateway group name is required")
	}
	logger.Debugf("validateGateway() group check passed: %q", g.Spec.Group)

	logger.Debugf("validateGateway() checking pool: value=%q", g.Spec.Pool)
	if g.Spec.Pool == "" {
		logger.Debug("validateGateway() validation failed: pool is empty")
		return errors.New("pool name is required")
	}
	logger.Debugf("validateGateway() pool check passed: %q", g.Spec.Pool)

	logger.Debug("validateGateway() all validations passed")
	return nil
}

func (r *ReconcileCephNVMeOFGateway) updateStatus(observedGeneration int64, namespacedName types.NamespacedName, cephxStatus *cephv1.CephxStatus, status string) error {
	logger.Debugf("updateStatus() started: namespace=%s, name=%s, status=%s, observedGeneration=%d",
		namespacedName.Namespace, namespacedName.Name, status, observedGeneration)
	if cephxStatus != nil {
		logger.Debugf("updateStatus() cephxStatus: keyGeneration=%d, keyCephVersion=%s",
			cephxStatus.KeyGeneration, cephxStatus.KeyCephVersion)
	}

	nvmeof := &cephv1.CephNVMeOFGateway{}

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		logger.Debug("updateStatus() fetching current gateway resource for status update")
		err := r.client.Get(r.opManagerContext, namespacedName, nvmeof)
		if err != nil {
			if kerrors.IsNotFound(err) {
				logger.Debug("updateStatus() gateway not found, skipping status update")
				return nil
			}
			logger.Errorf("updateStatus() failed to get gateway: %v", err)
			return errors.Wrapf(err, "failed to get for status update")
		}
		logger.Debugf("updateStatus() gateway fetched: current phase=%s, observedGeneration=%d",
			func() string {
				if nvmeof.Status != nil {
					return nvmeof.Status.Phase
				}
				return "nil"
			}(), func() int64 {
				if nvmeof.Status != nil {
					return nvmeof.Status.ObservedGeneration
				}
				return -1
			}())

		if nvmeof.Status == nil {
			logger.Debug("updateStatus() status is nil, initializing")
			nvmeof.Status = &cephv1.NVMeOFGatewayStatus{}
		}

		oldPhase := nvmeof.Status.Phase
		nvmeof.Status.Phase = status
		logger.Debugf("updateStatus() updating phase: %s -> %s", oldPhase, status)

		if observedGeneration != k8sutil.ObservedGenerationNotAvailable {
			oldObservedGen := nvmeof.Status.ObservedGeneration
			nvmeof.Status.ObservedGeneration = observedGeneration
			logger.Debugf("updateStatus() updating observedGeneration: %d -> %d", oldObservedGen, observedGeneration)
		} else {
			logger.Debug("updateStatus() skipping observedGeneration update (not available)")
		}

		if cephxStatus != nil {
			logger.Debug("updateStatus() updating cephx status")
			nvmeof.Status.Cephx.Daemon = *cephxStatus
		} else {
			logger.Debug("updateStatus() no cephx status to update")
		}

		logger.Debug("updateStatus() applying status update to API server")
		if err := reporting.UpdateStatus(r.client, nvmeof); err != nil {
			logger.Errorf("updateStatus() failed to update status: %v", err)
			return errors.Wrapf(err, "failed to set status")
		}
		logger.Debug("updateStatus() status update applied successfully")
		return nil
	})
	if err != nil {
		logger.Errorf("updateStatus() retry loop failed: %v", err)
		return err
	}
	logger.Debugf("updateStatus() completed successfully: status=%s", status)
	return nil
}
