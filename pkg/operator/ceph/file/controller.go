/*
Copyright 2016 The Rook Authors. All rights reserved.

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

// Package file manages a CephFS filesystem and the required daemons.
package file

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
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/config"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/reporting"
	"github.com/rook/rook/pkg/operator/k8sutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	controllerName = "ceph-file-controller"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", controllerName)

// List of object resources to watch by the controller
var objectsToWatch = []client.Object{
	&corev1.Secret{TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: corev1.SchemeGroupVersion.String()}},
	&appsv1.Deployment{TypeMeta: metav1.TypeMeta{Kind: "Deployment", APIVersion: appsv1.SchemeGroupVersion.String()}},
}

var cephFilesystemKind = reflect.TypeOf(cephv1.CephFilesystem{}).Name()

// Sets the type meta for the controller main object
var controllerTypeMeta = metav1.TypeMeta{
	Kind:       cephFilesystemKind,
	APIVersion: fmt.Sprintf("%s/%s", cephv1.CustomResourceGroup, cephv1.Version),
}

var currentAndDesiredCephVersion = opcontroller.CurrentAndDesiredCephVersion

// ReconcileCephFilesystem reconciles a CephFilesystem object
type ReconcileCephFilesystem struct {
	client           client.Client
	recorder         record.EventRecorder
	scheme           *runtime.Scheme
	context          *clusterd.Context
	cephClusterSpec  *cephv1.ClusterSpec
	clusterInfo      *cephclient.ClusterInfo
	fsContexts       map[string]*fsHealth
	opManagerContext context.Context
	opConfig         opcontroller.OperatorConfig
}

type fsHealth struct {
	internalCtx    context.Context
	internalCancel context.CancelFunc
	started        bool
}

// Add creates a new CephFilesystem Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context, opConfig opcontroller.OperatorConfig) error {
	return add(opManagerContext, mgr, newReconciler(mgr, context, opManagerContext, opConfig))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context, opConfig opcontroller.OperatorConfig) reconcile.Reconciler {
	return &ReconcileCephFilesystem{
		client:           mgr.GetClient(),
		recorder:         mgr.GetEventRecorderFor("rook-" + controllerName),
		scheme:           mgr.GetScheme(),
		context:          context,
		fsContexts:       make(map[string]*fsHealth),
		opManagerContext: opManagerContext,
		opConfig:         opConfig,
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
				&cephv1.CephFilesystem{},
			),
			opcontroller.WatchPredicateForNonCRDObject[T](&cephv1.CephFilesystem{TypeMeta: controllerTypeMeta}, mgr.GetScheme()),
		),
	)
}

func add(opManagerContext context.Context, mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}
	logger.Info("successfully started")

	// Watch for changes on the CephFilesystem CRD object
	err = c.Watch(
		source.Kind(
			mgr.GetCache(),
			&cephv1.CephFilesystem{TypeMeta: controllerTypeMeta},
			&handler.TypedEnqueueRequestForObject[*cephv1.CephFilesystem]{},
			opcontroller.WatchControllerPredicate[*cephv1.CephFilesystem](mgr.GetScheme()),
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

	// Build Handler function to return the list of ceph filesystems
	// This is used by the watchers below
	handlerFunc, err := opcontroller.ObjectToCRMapper[*cephv1.CephFilesystemList, *corev1.ConfigMap](opManagerContext, mgr.GetClient(), &cephv1.CephFilesystemList{}, mgr.GetScheme())
	if err != nil {
		return err
	}

	// Watch for ConfigMap "rook-ceph-mon-endpoints" update and reconcile, which will reconcile update the bootstrap peer token
	err = c.Watch(
		source.Kind(
			mgr.GetCache(),
			&corev1.ConfigMap{TypeMeta: metav1.TypeMeta{Kind: "ConfigMap", APIVersion: corev1.SchemeGroupVersion.String()}},
			handler.TypedEnqueueRequestsFromMapFunc(handlerFunc),
			mon.PredicateMonEndpointChanges(),
		),
	)
	if err != nil {
		return err
	}

	return nil
}

// Reconcile reads that state of the cluster for a CephFilesystem object and makes changes based on the state read
// and what is in the cephFilesystem.Spec
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileCephFilesystem) Reconcile(context context.Context, request reconcile.Request) (reconcile.Result, error) {
	// workaround because the rook logging mechanism is not compatible with the controller-runtime logging interface
	reconcileResponse, cephFilesystem, err := r.reconcile(request)
	if err != nil {
		logger.Errorf("failed to reconcile %v", err)
	}

	return reporting.ReportReconcileResult(logger, r.recorder, request, &cephFilesystem, reconcileResponse, err)
}

func (r *ReconcileCephFilesystem) reconcile(request reconcile.Request) (reconcile.Result, cephv1.CephFilesystem, error) {
	// Fetch the cephFilesystem instance
	cephFilesystem := &cephv1.CephFilesystem{}
	err := r.client.Get(r.opManagerContext, request.NamespacedName, cephFilesystem)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("cephFilesystem resource not found. Ignoring since object must be deleted.")
			// If there was a previous error or if a user removed this resource's finalizer, it's
			// possible Rook didn't clean up the monitoring routine for this resource. Ensure the
			// routine is stopped when we see the resource is gone.
			cephFilesystem.Name = request.Name
			cephFilesystem.Namespace = request.Namespace
			r.cancelMirrorMonitoring(cephFilesystem)
			return reconcile.Result{}, *cephFilesystem, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, *cephFilesystem, errors.Wrap(err, "failed to get cephFilesystem")
	}

	// update observedGeneration local variable with current generation value,
	// because generation can be changed before reconcile got completed
	// CR status will be updated at end of reconcile, so to reflect the reconcile has finished
	observedGeneration := cephFilesystem.ObjectMeta.Generation

	// Set a finalizer so we can do cleanup before the object goes away
	err = opcontroller.AddFinalizerIfNotPresent(r.opManagerContext, r.client, cephFilesystem)
	if err != nil {
		return reconcile.Result{}, *cephFilesystem, errors.Wrap(err, "failed to add finalizer")
	}

	// The CR was just created, initialize status as 'Progressing'
	if cephFilesystem.Status == nil {
		updatedCephFS := r.updateStatus(k8sutil.ObservedGenerationNotAvailable, request.NamespacedName, cephv1.ConditionProgressing, nil)
		if updatedCephFS == nil || updatedCephFS.Status == nil {
			return reconcile.Result{}, *cephFilesystem, errors.Errorf("failed to update ceph filesystem status")
		}
		cephFilesystem = updatedCephFS
	}

	// Make sure a CephCluster is present otherwise do nothing
	cephCluster, isReadyToReconcile, cephClusterExists, reconcileResponse := opcontroller.IsReadyToReconcile(r.opManagerContext, r.client, request.NamespacedName, controllerName)
	if !isReadyToReconcile {
		// This handles the case where the Ceph Cluster is gone and we want to delete that CR
		// We skip the deleteFilesystem() function since everything is gone already
		//
		// Also, only remove the finalizer if the CephCluster is gone
		// If not, we should wait for it to be ready
		// This handles the case where the operator is not ready to accept Ceph command but the cluster exists
		if !cephFilesystem.GetDeletionTimestamp().IsZero() && !cephClusterExists {
			// don't leak the health checker routine if we are force deleting
			r.cancelMirrorMonitoring(cephFilesystem)

			// Remove finalizer
			err := opcontroller.RemoveFinalizer(r.opManagerContext, r.client, cephFilesystem)
			if err != nil {
				return reconcile.Result{}, *cephFilesystem, errors.Wrap(err, "failed to remove finalizer")
			}

			// Return and do not requeue. Successful deletion.
			return reconcile.Result{}, *cephFilesystem, nil
		}
		return reconcileResponse, *cephFilesystem, nil
	}
	r.cephClusterSpec = &cephCluster.Spec

	// Initialize the contexts, they allow us to track multiple CephFilesystems in the same namespace
	_, fsContextsExists := r.fsContexts[fsChannelKeyName(cephFilesystem)]
	if !fsContextsExists {
		internalCtx, internalCancel := context.WithCancel(r.opManagerContext)
		r.fsContexts[fsChannelKeyName(cephFilesystem)] = &fsHealth{
			internalCtx:    internalCtx,
			internalCancel: internalCancel,
		}
	}

	// Populate clusterInfo
	// Always populate it during each reconcile
	clusterInfo, _, _, err := opcontroller.LoadClusterInfo(r.context, r.opManagerContext, request.NamespacedName.Namespace, r.cephClusterSpec)
	if err != nil {
		return reconcile.Result{}, *cephFilesystem, errors.Wrap(err, "failed to populate cluster info")
	}
	r.clusterInfo = clusterInfo

	// DELETE: the CR was deleted
	if !cephFilesystem.GetDeletionTimestamp().IsZero() {
		deps, err := CephFilesystemDependents(r.context, r.clusterInfo, cephFilesystem)
		if err != nil {
			return reconcile.Result{}, *cephFilesystem, err
		}
		if !deps.Empty() {
			err := reporting.ReportDeletionBlockedDueToDependents(r.opManagerContext, logger, r.client, cephFilesystem, deps)
			return opcontroller.WaitForRequeueIfFinalizerBlocked, *cephFilesystem, err
		}
		reporting.ReportDeletionNotBlockedDueToDependents(r.opManagerContext, logger, r.client, r.recorder, cephFilesystem)

		runningCephVersion, err := cephclient.LeastUptodateDaemonVersion(r.context, clusterInfo, config.MonType)
		if err != nil {
			return reconcile.Result{}, *cephFilesystem,
				errors.Wrapf(err, "failed to retrieve current ceph %q version", config.MonType)
		}
		r.clusterInfo.CephVersion = runningCephVersion

		// Detect against running version only
		logger.Debugf("deleting filesystem %q", cephFilesystem.Name)
		err = r.reconcileDeleteFilesystem(cephFilesystem)
		if err != nil {
			return reconcile.Result{}, *cephFilesystem,
				errors.Wrapf(err, "failed to delete filesystem %q. ", cephFilesystem.Name)
		}

		// If the ceph fs still in the map, we must remove it during CR deletion
		r.cancelMirrorMonitoring(cephFilesystem)

		// Remove finalizer
		err = opcontroller.RemoveFinalizer(r.opManagerContext, r.client, cephFilesystem)
		if err != nil {
			return reconcile.Result{}, *cephFilesystem, errors.Wrap(err, "failed to remove finalizer")
		}

		// Return and do not requeue. Successful deletion.
		return reconcile.Result{}, *cephFilesystem, nil
	}

	// Detect desired CephCluster version
	runningCephVersion, desiredCephVersion, err := currentAndDesiredCephVersion(
		r.opManagerContext,
		r.opConfig.Image,
		cephFilesystem.Namespace,
		controllerName,
		k8sutil.NewOwnerInfo(cephFilesystem, r.scheme),
		r.context,
		r.cephClusterSpec,
		r.clusterInfo,
	)
	if err != nil {
		if strings.Contains(err.Error(), opcontroller.UninitializedCephConfigError) {
			logger.Info(opcontroller.OperatorNotInitializedMessage)
			return opcontroller.WaitForRequeueIfOperatorNotInitialized, *cephFilesystem, nil
		}
		return reconcile.Result{}, *cephFilesystem, errors.Wrap(err, "failed to detect running and desired ceph version")
	}
	r.clusterInfo.CephVersion = *runningCephVersion

	// If the version of the Ceph monitor differs from the CephCluster CR image version we assume
	// the cluster is being upgraded. So the controller will just wait for the upgrade to finish and
	// then versions should match. Obviously using the cmd reporter job adds up to the deployment time
	// Skip waiting for upgrades to finish in case of external cluster.
	if !cephCluster.Spec.External.Enable && !reflect.DeepEqual(*runningCephVersion, *desiredCephVersion) {
		// Upgrade is in progress, let's wait for the mons to be done
		return opcontroller.WaitForRequeueIfCephClusterIsUpgrading, *cephFilesystem,
			opcontroller.ErrorCephUpgradingRequeue(desiredCephVersion, runningCephVersion)
	}

	// validate the filesystem settings
	if err := validateFilesystem(r.context, r.clusterInfo, r.cephClusterSpec, cephFilesystem); err != nil {
		if strings.Contains(err.Error(), opcontroller.UninitializedCephConfigError) {
			logger.Info(opcontroller.OperatorNotInitializedMessage)
			return opcontroller.WaitForRequeueIfOperatorNotInitialized, *cephFilesystem, nil
		}
		return reconcile.Result{}, *cephFilesystem,
			errors.Wrapf(err, "invalid object filesystem %q arguments", cephFilesystem.Name)
	}

	// RECONCILE
	logger.Debug("reconciling ceph filesystem store deployments")
	reconcileResponse, err = r.reconcileCreateFilesystem(cephFilesystem)
	if err != nil {
		r.updateStatus(k8sutil.ObservedGenerationNotAvailable, request.NamespacedName, cephv1.ConditionFailure, nil)
		return reconcileResponse, *cephFilesystem, err
	}

	statusUpdated := false

	// Enable mirroring if needed
	if cephFilesystem.Spec.Mirroring != nil {
		// Disable mirroring on that filesystem if needed
		if !cephFilesystem.Spec.Mirroring.Enabled {
			err = cephclient.DisableFilesystemSnapshotMirror(r.context, r.clusterInfo, cephFilesystem.Name)
			if err != nil {
				return reconcile.Result{}, *cephFilesystem,
					errors.Wrapf(err, "failed to disable mirroring on filesystem %q", cephFilesystem.Name)
			}
		} else {
			logger.Info("reconciling cephfs-mirror mirroring configuration")
			err = r.reconcileMirroring(cephFilesystem, request.NamespacedName)
			if err != nil {
				return opcontroller.ImmediateRetryResult, *cephFilesystem,
					errors.Wrapf(err, "failed to configure mirroring for filesystem %q.", cephFilesystem.Name)
			}

			// Always create a bootstrap peer token in case another cluster wants to add us as a peer
			logger.Info("reconciling create cephfs-mirror peer configuration")
			reconcileResponse, err = opcontroller.CreateBootstrapPeerSecret(r.context, r.clusterInfo, cephFilesystem, k8sutil.NewOwnerInfo(cephFilesystem, r.scheme))
			if err != nil {
				r.updateStatus(k8sutil.ObservedGenerationNotAvailable, request.NamespacedName, cephv1.ConditionFailure, nil)
				return reconcileResponse, *cephFilesystem,
					errors.Wrapf(err, "failed to create cephfs-mirror bootstrap peer for filesystem %q.", cephFilesystem.Name)
			}

			logger.Info("reconciling add cephfs-mirror peer configuration")
			err = r.reconcileAddBootstrapPeer(cephFilesystem, request.NamespacedName)
			if err != nil {
				return opcontroller.ImmediateRetryResult, *cephFilesystem,
					errors.Wrapf(err, "failed to configure mirroring for filesystem %q.", cephFilesystem.Name)
			}

			// update ObservedGeneration in status at the end of reconcile
			// Set Ready status, we are done reconciling
			if r.updateStatus(observedGeneration, request.NamespacedName, cephv1.ConditionReady, opcontroller.GenerateStatusInfo(cephFilesystem)) != nil {
				statusUpdated = true
			}

			// Run go routine check for mirroring status
			if !cephFilesystem.Spec.StatusCheck.Mirror.Disabled {
				// Start monitoring cephfs-mirror status
				if r.fsContexts[fsChannelKeyName(cephFilesystem)].started {
					logger.Debug("ceph filesystem mirror status monitoring go routine already running!")
				} else {
					checker := newMirrorChecker(r.context, r.client, r.clusterInfo, request.NamespacedName, &cephFilesystem.Spec, cephFilesystem.Name)
					go checker.checkMirroring(r.fsContexts[fsChannelKeyName(cephFilesystem)].internalCtx)
					r.fsContexts[fsChannelKeyName(cephFilesystem)].started = true
				}
			}
		}
	}

	if !statusUpdated {
		// update ObservedGeneration in status at the end of reconcile
		// Set Ready status, we are done reconciling$
		// TODO: set status to Ready **only** if the filesystem is ready
		r.updateStatus(observedGeneration, request.NamespacedName, cephv1.ConditionReady, nil)
	}

	return reconcile.Result{}, *cephFilesystem, nil
}

func (r *ReconcileCephFilesystem) reconcileCreateFilesystem(cephFilesystem *cephv1.CephFilesystem) (reconcile.Result, error) {
	if r.cephClusterSpec.External.Enable {
		_, err := opcontroller.ValidateCephVersionsBetweenLocalAndExternalClusters(r.context, r.clusterInfo)
		if err != nil {
			// This handles the case where the operator is running, the external cluster has been upgraded and a CR creation is called
			// If that's a major version upgrade we fail, if it's a minor version, we continue, it's not ideal but not critical
			return reconcile.Result{}, errors.Wrapf(err, "refusing to run new crd")
		}
	}

	// preservePoolsOnDelete being set to true has data-loss concerns and is deprecated (see #6492).
	// If preservePoolsOnDelete is set to true, assume the user means preserveFilesystemOnDelete instead.
	if cephFilesystem.Spec.PreservePoolsOnDelete {
		if !cephFilesystem.Spec.PreserveFilesystemOnDelete {
			logger.Warning("preservePoolsOnDelete (currently set 'true') has been deprecated in favor of preserveFilesystemOnDelete (currently set 'false') due to data loss concerns so Rook will assume preserveFilesystemOnDelete 'true'")
			cephFilesystem.Spec.PreserveFilesystemOnDelete = true
		}
	}

	ownerInfo := k8sutil.NewOwnerInfo(cephFilesystem, r.scheme)
	err := createFilesystem(r.context, r.clusterInfo, *cephFilesystem, r.cephClusterSpec, ownerInfo, r.cephClusterSpec.DataDirHostPath)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to create filesystem %q", cephFilesystem.Name)
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileCephFilesystem) reconcileDeleteFilesystem(cephFilesystem *cephv1.CephFilesystem) error {
	ownerInfo := k8sutil.NewOwnerInfo(cephFilesystem, r.scheme)
	err := deleteFilesystem(r.context, r.clusterInfo, *cephFilesystem, r.cephClusterSpec, ownerInfo, r.cephClusterSpec.DataDirHostPath)
	if err != nil {
		return err
	}

	return nil
}

func (r *ReconcileCephFilesystem) reconcileMirroring(cephFilesystem *cephv1.CephFilesystem, namespacedName types.NamespacedName) error {
	// Enable the mgr module
	err := cephclient.MgrEnableModule(r.context, r.clusterInfo, "mirroring", false)
	if err != nil {
		return errors.Wrap(err, "failed to enable mirroring mgr module")
	}

	// Enable snapshot mirroring on that filesystem
	err = cephclient.EnableFilesystemSnapshotMirror(r.context, r.clusterInfo, cephFilesystem.Name)
	if err != nil {
		return errors.Wrapf(err, "failed to enable mirroring on filesystem %q", cephFilesystem.Name)
	}

	// Add snapshot schedules
	if cephFilesystem.Spec.Mirroring.SnapShotScheduleEnabled() {
		// Enable the snap_schedule module
		err = cephclient.MgrEnableModule(r.context, r.clusterInfo, "snap_schedule", false)
		if err != nil {
			return errors.Wrap(err, "failed to enable snap_schedule mgr module")
		}

		// Enable snapshot schedules
		for _, snap := range cephFilesystem.Spec.Mirroring.SnapshotSchedules {
			err = cephclient.AddSnapshotSchedule(r.context, r.clusterInfo, snap.Path, snap.Interval, snap.StartTime, cephFilesystem.Name)
			if err != nil {
				return errors.Wrapf(err, "failed to add snapshot schedules on filesystem %q", cephFilesystem.Name)
			}
		}
		// Enable snapshot retention
		for _, retention := range cephFilesystem.Spec.Mirroring.SnapshotRetention {
			err = cephclient.AddSnapshotScheduleRetention(r.context, r.clusterInfo, retention.Path, retention.Duration, cephFilesystem.Name)
			if err != nil {
				return errors.Wrapf(err, "failed to add snapshot retention on filesystem %q", cephFilesystem.Name)
			}
		}
	}

	return nil
}

func (r *ReconcileCephFilesystem) reconcileAddBootstrapPeer(cephFilesystem *cephv1.CephFilesystem, namespacedName types.NamespacedName) error {
	if cephFilesystem.Spec.Mirroring.Peers == nil {
		return nil
	}
	// List all the peers secret, we can have more than one peer we might want to configure
	// For each, get the Kubernetes Secret and import the "peer token" so that we can configure the mirroring
	for _, peerSecret := range cephFilesystem.Spec.Mirroring.Peers.SecretNames {
		logger.Debugf("fetching bootstrap peer kubernetes secret %q", peerSecret)
		s, err := r.context.Clientset.CoreV1().Secrets(r.clusterInfo.Namespace).Get(r.opManagerContext, peerSecret, metav1.GetOptions{})
		// We don't care about IsNotFound here, we still need to fail
		if err != nil {
			return errors.Wrapf(err, "failed to fetch kubernetes secret %q fs-mirror bootstrap peer", peerSecret)
		}

		// Validate peer secret content
		err = opcontroller.ValidatePeerToken(cephFilesystem, s.Data)
		if err != nil {
			return errors.Wrapf(err, "failed to validate fs-mirror bootstrap peer secret %q data", peerSecret)
		}

		// Add fs-mirror peer
		err = cephclient.ImportFSMirrorBootstrapPeer(r.context, r.clusterInfo, cephFilesystem.Name, string(s.Data["token"]))
		if err != nil {
			return errors.Wrap(err, "failed to import filesystem bootstrap peer token")
		}
	}

	return nil
}

func fsChannelKeyName(f *cephv1.CephFilesystem) string {
	return types.NamespacedName{Namespace: f.Namespace, Name: f.Name}.String()
}

// cancel mirror monitoring. This is a noop if monitoring is not running.
func (r *ReconcileCephFilesystem) cancelMirrorMonitoring(cephFilesystem *cephv1.CephFilesystem) {
	_, fsContextsExists := r.fsContexts[fsChannelKeyName(cephFilesystem)]
	if fsContextsExists {
		// Cancel the context to stop the mirroring status
		r.fsContexts[fsChannelKeyName(cephFilesystem)].internalCancel()

		// Remove ceph fs from the map
		delete(r.fsContexts, fsChannelKeyName(cephFilesystem))
	}
}
