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

package mirror

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/reporting"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/log"
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
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	controllerName = "ceph-filesystem-mirror-controller"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "cephfs-mirror-controller")

// List of object resources to watch by the controller
var objectsToWatch = []client.Object{
	&v1.ConfigMap{TypeMeta: metav1.TypeMeta{Kind: "ConfigMap", APIVersion: v1.SchemeGroupVersion.String()}},
	&v1.Secret{TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: v1.SchemeGroupVersion.String()}},
	&appsv1.Deployment{TypeMeta: metav1.TypeMeta{Kind: "Deployment", APIVersion: appsv1.SchemeGroupVersion.String()}},
}

// Sets the type meta for the controller main object
var controllerTypeMeta = metav1.TypeMeta{
	Kind:       reflect.TypeFor[cephv1.CephFilesystemMirror]().Name(),
	APIVersion: fmt.Sprintf("%s/%s", cephv1.CustomResourceGroup, cephv1.Version),
}

var currentAndDesiredCephVersion = opcontroller.CurrentAndDesiredCephVersion

// ReconcileFilesystemMirror reconciles a CephFilesystemMirror object
type ReconcileFilesystemMirror struct {
	context               *clusterd.Context
	clusterInfo           *cephclient.ClusterInfo
	client                client.Client
	scheme                *runtime.Scheme
	cephClusterSpec       *cephv1.ClusterSpec
	opManagerContext      context.Context
	opConfig              opcontroller.OperatorConfig
	recorder              record.EventRecorder
	shouldRotateCephxKeys bool
}

// Add creates a new CephFilesystemMirror Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context, opConfig opcontroller.OperatorConfig) error {
	return add(mgr, newReconciler(mgr, context, opManagerContext, opConfig))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context, opConfig opcontroller.OperatorConfig) reconcile.Reconciler {
	return &ReconcileFilesystemMirror{
		client:           mgr.GetClient(),
		scheme:           mgr.GetScheme(),
		context:          context,
		opConfig:         opConfig,
		opManagerContext: opManagerContext,
		recorder:         mgr.GetEventRecorderFor("rook-" + controllerName),
	}
}

func watchOwnedCoreObject[T client.Object](c controller.Controller, mgr manager.Manager, obj T) error {
	return c.Watch(
		source.Kind(
			mgr.GetCache(),
			obj,
			handler.TypedEnqueueRequestForOwner[T](
				mgr.GetScheme(),
				mgr.GetRESTMapper(),
				&cephv1.CephFilesystemMirror{},
			),
			opcontroller.WatchPredicateForNonCRDObject[T](&cephv1.CephFilesystemMirror{TypeMeta: controllerTypeMeta}, mgr.GetScheme()),
		),
	)
}

func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}
	logger.Info("successfully started")

	// Watch for changes on the CephFilesystemMirror CRD object
	err = c.Watch(
		source.Kind(
			mgr.GetCache(),
			&cephv1.CephFilesystemMirror{TypeMeta: controllerTypeMeta},
			&handler.TypedEnqueueRequestForObject[*cephv1.CephFilesystemMirror]{},
			opcontroller.WatchControllerPredicate[*cephv1.CephFilesystemMirror](mgr.GetScheme()),
		),
	)
	if err != nil {
		return err
	}

	// Watch all other resources
	for _, t := range objectsToWatch {
		err = watchOwnedCoreObject(c, mgr, t)
		if err != nil {
			return err
		}
	}

	return nil
}

// Reconcile reads that state of the cluster for a CephFilesystemMirror object and makes changes based on the state read
// and what is in the CephFilesystemMirror.Spec
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileFilesystemMirror) Reconcile(context context.Context, request reconcile.Request) (reconcile.Result, error) {
	defer opcontroller.RecoverAndLogException()
	// workaround because the rook logging mechanism is not compatible with the controller-runtime logging interface
	reconcileResponse, cephFilesystemMirror, err := r.reconcile(request)
	if err != nil {
		err := r.updateStatus(k8sutil.ObservedGenerationNotAvailable, request.NamespacedName, k8sutil.FailedStatus, nil)
		if err != nil {
			return opcontroller.ImmediateRetryResult, errors.Wrapf(err, "failed set ready failure status to the cephFileSystemMirror %q", request.NamespacedName)
		}
		log.NamedError(request.NamespacedName, logger, "failed to reconcile %v", err)
	}

	return reporting.ReportReconcileResult(logger, r.recorder, request, &cephFilesystemMirror, reconcileResponse, err)
}

func (r *ReconcileFilesystemMirror) reconcile(request reconcile.Request) (reconcile.Result, cephv1.CephFilesystemMirror, error) {
	// Fetch the CephFilesystemMirror instance
	filesystemMirror := &cephv1.CephFilesystemMirror{}
	err := r.client.Get(r.opManagerContext, request.NamespacedName, filesystemMirror)
	if err != nil {
		if kerrors.IsNotFound(err) {
			log.NamedDebug(request.NamespacedName, logger, "CephFilesystemMirror resource not found. Ignoring since object must be deleted.")
			return reconcile.Result{}, *filesystemMirror, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, *filesystemMirror, errors.Wrap(err, "failed to get CephFilesystemMirror")
	}

	// update observedGeneration local variable with current generation value,
	// because generation can be changed before reconcile got completed
	// CR status will be updated at end of reconcile, so to reflect the reconcile has finished
	observedGeneration := filesystemMirror.ObjectMeta.Generation

	// The CR was just created, initializing status fields
	if filesystemMirror.Status == nil {
		cephxUninitialized := keyring.UninitializedCephxStatus()
		err := r.updateStatus(k8sutil.ObservedGenerationNotAvailable, request.NamespacedName, k8sutil.EmptyStatus, &cephxUninitialized)
		if err != nil {
			return opcontroller.ImmediateRetryResult, *filesystemMirror, errors.Wrapf(err, "failed set empty status to the cephFileSystemMirror %q", request.NamespacedName)
		}
		filesystemMirror.Status = &cephv1.FileMirrorStatus{
			Cephx: cephv1.LocalCephxStatus{
				Daemon: cephxUninitialized,
			},
		}
	}

	// Make sure a CephCluster is present otherwise do nothing
	cephCluster, isReadyToReconcile, _, reconcileResponse := opcontroller.IsReadyToReconcile(r.opManagerContext, r.client, request.NamespacedName, controllerName)
	if !isReadyToReconcile {
		log.NamedDebug(request.NamespacedName, logger, "CephCluster resource not ready in namespace %q, retrying in %q.", request.NamespacedName.Namespace, reconcileResponse.RequeueAfter.String())
		r.recorder.Event(filesystemMirror, v1.EventTypeNormal, string(cephv1.ReconcileSucceeded), "successfully removed finalizer")

		return reconcileResponse, *filesystemMirror, nil
	}

	// Assign the clusterSpec
	r.cephClusterSpec = &cephCluster.Spec

	// Populate clusterInfo
	r.clusterInfo, _, _, err = opcontroller.LoadClusterInfo(r.context, r.opManagerContext, request.NamespacedName.Namespace, r.cephClusterSpec)
	if err != nil {
		return opcontroller.ImmediateRetryResult, *filesystemMirror, errors.Wrap(err, "failed to populate cluster info")
	}

	// Detect desired CephCluster version
	runningCephVersion, desiredCephVersion, err := currentAndDesiredCephVersion(
		r.opManagerContext,
		r.opConfig.Image,
		filesystemMirror.Namespace,
		controllerName,
		k8sutil.NewOwnerInfo(filesystemMirror, r.scheme),
		r.context,
		r.cephClusterSpec,
		r.clusterInfo,
	)
	if err != nil {
		if strings.Contains(err.Error(), opcontroller.UninitializedCephConfigError) {
			logger.Info(opcontroller.OperatorNotInitializedMessage)
			return opcontroller.WaitForRequeueIfOperatorNotInitialized, *filesystemMirror, nil
		}
		return reconcile.Result{}, *filesystemMirror, errors.Wrap(err, "failed to detect running and desired ceph version")
	}

	// If the version of the Ceph monitor differs from the CephCluster CR image version we assume
	// the cluster is being upgraded. So the controller will just wait for the upgrade to finish and
	// then versions should match. Obviously using the cmd reporter job adds up to the deployment time
	// Skip waiting for upgrades to finish in case of external cluster.
	if !cephCluster.Spec.External.Enable && !reflect.DeepEqual(*runningCephVersion, *desiredCephVersion) {
		// Upgrade is in progress, let's wait for the mons to be done
		return opcontroller.WaitForRequeueIfCephClusterIsUpgrading, *filesystemMirror,
			opcontroller.ErrorCephUpgradingRequeue(desiredCephVersion, runningCephVersion)
	}
	r.clusterInfo.CephVersion = *runningCephVersion

	// check if cephRBDMirror daemon keys should be rotated or not
	r.shouldRotateCephxKeys, err = keyring.ShouldRotateCephxKeys(cephCluster.Spec.Security.CephX.Daemon, *runningCephVersion, *runningCephVersion, filesystemMirror.Status.Cephx.Daemon)
	if err != nil {
		return reconcile.Result{}, *filesystemMirror, errors.Wrapf(err, "failed to determine if cephx keys should be rotated for the cephFileSystemMirror %q", request.NamespacedName)
	}
	if r.shouldRotateCephxKeys {
		log.NamedInfo(request.NamespacedName, logger, "cephx keys for CephFilesystemMirror will be rotated")
	}

	// CREATE/UPDATE
	log.NamedDebug(request.NamespacedName, logger, "reconciling ceph filesystem mirror deployments")
	reconcileResponse, err = r.reconcileFilesystemMirror(filesystemMirror)
	if err != nil {
		return opcontroller.ImmediateRetryResult, *filesystemMirror, errors.Wrap(err, "failed to create ceph filesystem mirror deployments")
	}

	cephxStatus := keyring.UpdatedCephxStatus(r.shouldRotateCephxKeys, r.cephClusterSpec.Security.CephX.Daemon, r.clusterInfo.CephVersion, filesystemMirror.Status.Cephx.Daemon)

	// update ObservedGeneration in status at the end of reconcile
	// Set Ready status, we are done reconciling
	err = r.updateStatus(observedGeneration, request.NamespacedName, k8sutil.ReadyStatus, &cephxStatus)
	if err != nil {
		return opcontroller.ImmediateRetryResult, *filesystemMirror, errors.Wrapf(err, "failed set ready status to the cephFileSystemMirror %q", request.NamespacedName)
	}

	// Return and do not requeue
	log.NamedDebug(request.NamespacedName, logger, "done reconciling ceph filesystem mirror")
	return reconcile.Result{}, *filesystemMirror, nil
}

func (r *ReconcileFilesystemMirror) reconcileFilesystemMirror(filesystemMirror *cephv1.CephFilesystemMirror) (reconcile.Result, error) {
	if r.cephClusterSpec.External.Enable {
		_, err := opcontroller.ValidateCephVersionsBetweenLocalAndExternalClusters(r.context, r.clusterInfo)
		if err != nil {
			// This handles the case where the operator is running, the external cluster has been upgraded and a CR creation is called
			// If that's a major version upgrade we fail, if it's a minor version, we continue, it's not ideal but not critical
			return opcontroller.ImmediateRetryResult, errors.Wrap(err, "refusing to run new crd")
		}
	}

	err := r.start(filesystemMirror)
	if err != nil {
		return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed to start filesystem mirror")
	}

	return reconcile.Result{}, nil
}

// updateStatus updates an object with a given status
func (r *ReconcileFilesystemMirror) updateStatus(observedGeneration int64, namespacedName types.NamespacedName, status string, cephx *cephv1.CephxStatus) error {
	fsMirror := &cephv1.CephFilesystemMirror{}

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		err := r.client.Get(r.opManagerContext, namespacedName, fsMirror)
		if err != nil {
			if kerrors.IsNotFound(err) {
				log.NamedDebug(namespacedName, logger, "CephFilesystemMirror resource not found. Ignoring since object must be deleted.")
				return nil
			}
			return errors.Wrapf(err, "failed to retrieve filesystem mirror %q to update status to %v", namespacedName, status)
		}

		if fsMirror.Status == nil {
			fsMirror.Status = &cephv1.FileMirrorStatus{}
		}

		fsMirror.Status.Phase = status
		if observedGeneration != k8sutil.ObservedGenerationNotAvailable {
			fsMirror.Status.ObservedGeneration = observedGeneration
		}

		if cephx != nil {
			fsMirror.Status.Cephx.Daemon = *cephx
		}

		if err := reporting.UpdateStatus(r.client, fsMirror); err != nil {
			return errors.Wrapf(err, "failed to set filesystem mirror %q status to %+v", namespacedName, status)
		}
		return nil
	})
	if err != nil {
		return err
	}

	log.NamedDebug(namespacedName, logger, "filesystem mirror status updated to %q", status)
	return nil
}
