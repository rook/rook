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

package nfs

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/reporting"
	"github.com/rook/rook/pkg/operator/k8sutil"
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
	controllerName = "ceph-nfs-controller"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", controllerName)

// List of object resources to watch by the controller
var objectsToWatch = []client.Object{
	&v1.Service{TypeMeta: metav1.TypeMeta{Kind: "Service", APIVersion: v1.SchemeGroupVersion.String()}},
	&v1.ConfigMap{TypeMeta: metav1.TypeMeta{Kind: "ConfigMap", APIVersion: v1.SchemeGroupVersion.String()}},
	&appsv1.Deployment{TypeMeta: metav1.TypeMeta{Kind: "Deployment", APIVersion: appsv1.SchemeGroupVersion.String()}},
}

var cephNFSKind = reflect.TypeOf(cephv1.CephNFS{}).Name()

// Sets the type meta for the controller main object
var controllerTypeMeta = metav1.TypeMeta{
	Kind:       cephNFSKind,
	APIVersion: fmt.Sprintf("%s/%s", cephv1.CustomResourceGroup, cephv1.Version),
}

var currentAndDesiredCephVersion = opcontroller.CurrentAndDesiredCephVersion

// ReconcileCephNFS reconciles a cephNFS object
type ReconcileCephNFS struct {
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

// Add creates a new cephNFS Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context, opConfig opcontroller.OperatorConfig) error {
	return add(mgr, newReconciler(mgr, context, opManagerContext, opConfig))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context, opConfig opcontroller.OperatorConfig) reconcile.Reconciler {
	return &ReconcileCephNFS{
		client:           mgr.GetClient(),
		scheme:           mgr.GetScheme(),
		context:          context,
		opManagerContext: opManagerContext,
		opConfig:         opConfig,
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
				&cephv1.CephNFS{},
			),
			opcontroller.WatchPredicateForNonCRDObject[T](&cephv1.CephNFS{TypeMeta: controllerTypeMeta}, mgr.GetScheme()),
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

	// Watch for changes on the cephNFS CRD object
	err = c.Watch(
		source.Kind(
			mgr.GetCache(),
			&cephv1.CephNFS{TypeMeta: controllerTypeMeta},
			&handler.TypedEnqueueRequestForObject[*cephv1.CephNFS]{},
			opcontroller.WatchControllerPredicate[*cephv1.CephNFS](mgr.GetScheme()),
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

// Reconcile reads that state of the cluster for a cephNFS object and makes changes based on the state read
// and what is in the cephNFS.Spec
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileCephNFS) Reconcile(context context.Context, request reconcile.Request) (reconcile.Result, error) {
	defer opcontroller.RecoverAndLogException()
	// workaround because the rook logging mechanism is not compatible with the controller-runtime logging interface
	reconcileResponse, cephNFS, err := r.reconcile(request)

	return reporting.ReportReconcileResult(logger, r.recorder, request, &cephNFS, reconcileResponse, err)
}

func (r *ReconcileCephNFS) reconcile(request reconcile.Request) (reconcile.Result, cephv1.CephNFS, error) {
	// Fetch the cephNFS instance
	cephNFS := &cephv1.CephNFS{}
	err := r.client.Get(r.opManagerContext, request.NamespacedName, cephNFS)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("cephNFS resource not found. Ignoring since object must be deleted.")
			return reconcile.Result{}, *cephNFS, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, *cephNFS, errors.Wrap(err, "failed to get cephNFS")
	}
	// update observedGeneration local variable with current generation value,
	// because generation can be changed before reconcile got completed
	// CR status will be updated at end of reconcile, so to reflect the reconcile has finished
	observedGeneration := cephNFS.ObjectMeta.Generation

	// Set a finalizer so we can do cleanup before the object goes away
	generationUpdated, err := opcontroller.AddFinalizerIfNotPresent(r.opManagerContext, r.client, cephNFS)
	if err != nil {
		return reconcile.Result{}, *cephNFS, errors.Wrap(err, "failed to add finalizer")
	}
	if generationUpdated {
		logger.Infof("reconciling the nfs %q after adding finalizer", cephNFS.Name)
		return reconcile.Result{}, *cephNFS, nil
	}

	// The CR was just created, initializing status fields
	if cephNFS.Status == nil {
		cephxUninitialized := keyring.UninitializedCephxStatus()
		err := r.updateStatus(k8sutil.ObservedGenerationNotAvailable, request.NamespacedName, &cephxUninitialized, k8sutil.EmptyStatus)
		if err != nil {
			return opcontroller.ImmediateRetryResult, *cephNFS, errors.Wrapf(err, "failed set empty status to the cephNFS %q", request.NamespacedName)
		}
		// Initialize cephx status for new resources
		cephNFS.Status = &cephv1.NFSStatus{
			Status: cephv1.Status{},
			Cephx: cephv1.LocalCephxStatus{
				Daemon: cephxUninitialized,
			},
		}
	}

	if err := cephNFS.Spec.Security.Validate(); err != nil {
		return reconcile.Result{Requeue: true, RequeueAfter: 15 * time.Second}, *cephNFS,
			errors.Wrapf(err, "failed to validate security spec for CephNFS %q",
				types.NamespacedName{Namespace: cephNFS.Namespace, Name: cephNFS.Name})
	}

	// Make sure a CephCluster is present otherwise do nothing
	cephCluster, isReadyToReconcile, cephClusterExists, reconcileResponse := opcontroller.IsReadyToReconcile(r.opManagerContext, r.client, request.NamespacedName, controllerName)
	if !isReadyToReconcile {
		// This handles the case where the Ceph Cluster is gone and we want to delete that CR
		// We skip the deleteStore() function since everything is gone already
		//
		// Also, only remove the finalizer if the CephCluster is gone
		// If not, we should wait for it to be ready
		// This handles the case where the operator is not ready to accept Ceph command but the cluster exists
		if !cephNFS.GetDeletionTimestamp().IsZero() && !cephClusterExists {
			// Remove finalizer
			err := opcontroller.RemoveFinalizer(r.opManagerContext, r.client, cephNFS)
			if err != nil {
				return reconcile.Result{}, *cephNFS, errors.Wrap(err, "failed to remove finalizer")
			}

			r.recorder.Event(cephNFS, v1.EventTypeNormal, string(cephv1.ReconcileSucceeded), "successfully removed finalizer")
			// Return and do not requeue. Successful deletion.
			return reconcile.Result{}, *cephNFS, nil
		}
		return reconcileResponse, *cephNFS, nil
	}
	r.cephClusterSpec = &cephCluster.Spec

	// Populate clusterInfo
	// Always populate it during each reconcile
	r.clusterInfo, _, _, err = opcontroller.LoadClusterInfo(r.context, r.opManagerContext, request.NamespacedName.Namespace, r.cephClusterSpec)
	if err != nil {
		return reconcile.Result{}, *cephNFS, errors.Wrap(err, "failed to populate cluster info")
	}

	// DELETE: the CR was deleted
	if !cephNFS.GetDeletionTimestamp().IsZero() {
		logger.Infof("deleting ceph nfs %q", cephNFS.Name)
		r.recorder.Eventf(cephNFS, v1.EventTypeNormal, string(cephv1.ReconcileStarted), "deleting CephNFS %q", cephNFS.Name)

		// Detect running Ceph version
		runningCephVersion, err := cephclient.LeastUptodateDaemonVersion(r.context, r.clusterInfo, config.MonType)
		if err != nil {
			return reconcile.Result{}, *cephNFS, errors.Wrapf(err, "failed to retrieve current ceph %q version", config.MonType)
		}
		r.clusterInfo.CephVersion = runningCephVersion

		err = r.removeServersFromDatabase(cephNFS, 0)
		if err != nil {
			return reconcile.Result{}, *cephNFS, errors.Wrapf(err, "failed to delete filesystem %q. ", cephNFS.Name)
		}

		// Remove finalizer
		err = opcontroller.RemoveFinalizer(r.opManagerContext, r.client, cephNFS)
		if err != nil {
			return reconcile.Result{}, *cephNFS, errors.Wrap(err, "failed to remove finalizer")
		}
		r.recorder.Event(cephNFS, v1.EventTypeNormal, string(cephv1.ReconcileSucceeded), "successfully removed finalizer")

		// Return and do not requeue. Successful deletion.
		return reconcile.Result{}, *cephNFS, nil
	}

	// Detect desired CephCluster version
	runningCephVersion, desiredCephVersion, err := currentAndDesiredCephVersion(
		r.opManagerContext,
		r.opConfig.Image,
		cephNFS.Namespace,
		controllerName,
		k8sutil.NewOwnerInfo(cephNFS, r.scheme),
		r.context,
		r.cephClusterSpec,
		r.clusterInfo,
	)
	if err != nil {
		if strings.Contains(err.Error(), opcontroller.UninitializedCephConfigError) {
			logger.Info(opcontroller.OperatorNotInitializedMessage)
			return opcontroller.WaitForRequeueIfOperatorNotInitialized, *cephNFS, nil
		}
		return reconcile.Result{}, *cephNFS, errors.Wrap(err, "failed to detect running and desired ceph version")
	}

	// If the version of the Ceph monitor differs from the CephCluster CR image version we assume
	// the cluster is being upgraded. So the controller will just wait for the upgrade to finish and
	// then versions should match. Obviously using the cmd reporter job adds up to the deployment time
	// Skip waiting for upgrades to finish in case of external cluster.
	if !cephCluster.Spec.External.Enable && !reflect.DeepEqual(*runningCephVersion, *desiredCephVersion) {
		// Upgrade is in progress, let's wait for the mons to be done
		return opcontroller.WaitForRequeueIfCephClusterIsUpgrading, *cephNFS,
			opcontroller.ErrorCephUpgradingRequeue(desiredCephVersion, runningCephVersion)
	}
	r.clusterInfo.CephVersion = *runningCephVersion

	cephNFS.Spec.RADOS.Pool = nfsDefaultPoolName
	cephNFS.Spec.RADOS.Namespace = cephNFS.Name

	// validate the store settings
	if err := validateGanesha(r.context, r.clusterInfo, cephNFS); err != nil {
		return reconcile.Result{}, *cephNFS, errors.Wrapf(err, "invalid ceph nfs %q arguments", cephNFS.Name)
	}

	// Determine if we should rotate CephX keys for NFS daemons
	r.shouldRotateCephxKeys, err = keyring.ShouldRotateCephxKeys(cephCluster.Spec.Security.CephX.Daemon, *runningCephVersion,
		*desiredCephVersion, cephNFS.Status.Cephx.Daemon)
	if err != nil {
		return reconcile.Result{}, *cephNFS, errors.Wrap(err, "failed to determine if cephx keys should be rotated")
	}
	if r.shouldRotateCephxKeys {
		logger.Infof("cephx keys for CephNFS %q will be rotated", request.NamespacedName)
	}

	// Check for the existence of the .nfs pool
	err = r.configureNFSPool(cephNFS)
	if err != nil {
		return reconcile.Result{}, *cephNFS, errors.Wrapf(err, "failed to configure nfs pool %q", cephNFS.Spec.RADOS.Pool)
	}

	// CREATE/UPDATE
	logger.Debug("reconciling ceph nfs deployments")
	_, err = r.reconcileCreateCephNFS(cephNFS)
	if err != nil {
		return reconcile.Result{}, *cephNFS, errors.Wrap(err, "failed to create ceph nfs deployments")
	}

	// update NFS cephx status
	cephxStatus := keyring.UpdatedCephxStatus(r.shouldRotateCephxKeys, cephCluster.Spec.Security.CephX.Daemon, r.clusterInfo.CephVersion, cephNFS.Status.Cephx.Daemon)

	// update ObservedGeneration in status at the end of reconcile
	// Set Ready status, we are done reconciling
	err = r.updateStatus(observedGeneration, request.NamespacedName, &cephxStatus, k8sutil.ReadyStatus)
	if err != nil {
		return opcontroller.ImmediateRetryResult, *cephNFS, errors.Wrapf(err, "failed to update cephx status to the cephNFS %q", request.NamespacedName)
	}

	// Return and do not requeue
	logger.Debug("done reconciling ceph nfs")
	return reconcile.Result{}, *cephNFS, nil
}

func (r *ReconcileCephNFS) reconcileCreateCephNFS(cephNFS *cephv1.CephNFS) (reconcile.Result, error) {
	if r.cephClusterSpec.External.Enable {
		_, err := opcontroller.ValidateCephVersionsBetweenLocalAndExternalClusters(r.context, r.clusterInfo)
		if err != nil {
			// This handles the case where the operator is running, the external cluster has been upgraded and a CR creation is called
			// If that's a major version upgrade we fail, if it's a minor version, we continue, it's not ideal but not critical
			return reconcile.Result{}, errors.Wrap(err, "refusing to run new crd")
		}
	}

	// list nfs deployments that belong to this CephNFS
	listOps := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s,%s=%s", k8sutil.AppAttr, AppName, CephNFSNameLabelKey, cephNFS.Name),
	}
	deployments, err := r.context.Clientset.AppsV1().Deployments(cephNFS.Namespace).List(r.opManagerContext, listOps)
	if err != nil && !kerrors.IsNotFound(err) {
		return reconcile.Result{}, errors.Wrapf(err, "failed to list deployments for CephNFS %q", cephNFS.Name)
	}
	currentNFSServerCount := 0
	if deployments != nil {
		currentNFSServerCount = len(deployments.Items)
	}

	// Scale down case (CR value cephNFS.Spec.Server.Active changed)
	if currentNFSServerCount > cephNFS.Spec.Server.Active {
		logger.Infof("scaling down ceph nfs %q from %d to %d", cephNFS.Name, currentNFSServerCount, cephNFS.Spec.Server.Active)
		err := r.downCephNFS(cephNFS, currentNFSServerCount)
		if err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "failed to scale down ceph nfs %q", cephNFS.Name)
		}
	}

	// Update existing deployments and create new ones in the scale up case
	logger.Infof("updating ceph nfs %q", cephNFS.Name)
	err = r.upCephNFS(cephNFS)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to update ceph nfs %q", cephNFS.Name)
	}

	return reconcile.Result{}, nil
}

// updateStatus updates an object with a given status
func (r *ReconcileCephNFS) updateStatus(observedGeneration int64, namespacedName types.NamespacedName, cephxStatus *cephv1.CephxStatus, status string) error {
	nfs := &cephv1.CephNFS{}
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		err := r.client.Get(r.opManagerContext, namespacedName, nfs)
		if err != nil {
			if kerrors.IsNotFound(err) {
				logger.Debugf("CephNFS resource %q not found for updating status. Ignoring since object must be deleted.", namespacedName)
				return nil
			}
			return errors.Wrapf(err, "failed to get CephNFS %q for updating status to %+v", namespacedName, status)
		}
		if nfs.Status == nil {
			nfs.Status = &cephv1.NFSStatus{}
		}

		nfs.Status.Phase = status
		if observedGeneration != k8sutil.ObservedGenerationNotAvailable {
			nfs.Status.ObservedGeneration = observedGeneration
		}

		if cephxStatus != nil {
			nfs.Status.Cephx.Daemon = *cephxStatus
		}

		if err := reporting.UpdateStatus(r.client, nfs); err != nil {
			return errors.Wrapf(err, "failed to set CephNFS %q status to %+v", namespacedName, status)
		}
		return nil
	})
	if err != nil {
		return err
	}
	logger.Debugf("CephNFS %q status updated to %q", namespacedName, status)
	return nil
}
