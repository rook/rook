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
	opconfig "github.com/rook/rook/pkg/operator/ceph/config"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/file/mirror"
	"github.com/rook/rook/pkg/operator/k8sutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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

// ReconcileCephFilesystem reconciles a CephFilesystem object
type ReconcileCephFilesystem struct {
	client          client.Client
	scheme          *runtime.Scheme
	context         *clusterd.Context
	cephClusterSpec *cephv1.ClusterSpec
	clusterInfo     *cephclient.ClusterInfo
	fsChannels      map[string]*fsHealth
}

type fsHealth struct {
	stopChan          chan struct{}
	monitoringRunning bool
}

// Add creates a new CephFilesystem Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, context *clusterd.Context) error {
	return add(mgr, newReconciler(mgr, context))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, context *clusterd.Context) reconcile.Reconciler {
	// Add the cephv1 scheme to the manager scheme so that the controller knows about it
	mgrScheme := mgr.GetScheme()
	if err := cephv1.AddToScheme(mgr.GetScheme()); err != nil {
		panic(err)
	}
	return &ReconcileCephFilesystem{
		client:     mgr.GetClient(),
		scheme:     mgrScheme,
		context:    context,
		fsChannels: make(map[string]*fsHealth),
	}
}

func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}
	logger.Info("successfully started")

	// Watch for changes on the CephFilesystem CRD object
	err = c.Watch(&source.Kind{Type: &cephv1.CephFilesystem{TypeMeta: controllerTypeMeta}}, &handler.EnqueueRequestForObject{}, opcontroller.WatchControllerPredicate())
	if err != nil {
		return err
	}

	// Watch all other resources
	for _, t := range objectsToWatch {
		err = c.Watch(&source.Kind{Type: t}, &handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    &cephv1.CephFilesystem{},
		}, opcontroller.WatchPredicateForNonCRDObject(&cephv1.CephFilesystem{TypeMeta: controllerTypeMeta}, mgr.GetScheme()))
		if err != nil {
			return err
		}
	}

	// Build Handler function to return the list of ceph filesystems
	// This is used by the watchers below
	handlerFunc, err := opcontroller.ObjectToCRMapper(mgr.GetClient(), &cephv1.CephFilesystemList{}, mgr.GetScheme())
	if err != nil {
		return err
	}

	// Watch for CephCluster Spec changes that we want to propagate to us
	err = c.Watch(&source.Kind{Type: &cephv1.CephCluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       opcontroller.ClusterResource.Kind,
			APIVersion: opcontroller.ClusterResource.APIVersion,
		},
	},
	}, handler.EnqueueRequestsFromMapFunc(handlerFunc), opcontroller.WatchCephClusterPredicate())
	if err != nil {
		return err
	}

	// Watch for ConfigMap "rook-ceph-mon-endpoints" update and reconcile, which will reconcile update the bootstrap peer token
	err = c.Watch(&source.Kind{Type: &corev1.ConfigMap{TypeMeta: metav1.TypeMeta{Kind: "ConfigMap", APIVersion: corev1.SchemeGroupVersion.String()}}}, handler.EnqueueRequestsFromMapFunc(handlerFunc), mon.PredicateMonEndpointChanges())
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
	reconcileResponse, err := r.reconcile(request)
	if err != nil {
		logger.Errorf("failed to reconcile %v", err)
	}

	return reconcileResponse, err
}

func (r *ReconcileCephFilesystem) reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the cephFilesystem instance
	cephFilesystem := &cephv1.CephFilesystem{}
	err := r.client.Get(context.TODO(), request.NamespacedName, cephFilesystem)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("cephFilesystem resource not found. Ignoring since object must be deleted.")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, errors.Wrap(err, "failed to get cephFilesystem")
	}

	// Set a finalizer so we can do cleanup before the object goes away
	err = opcontroller.AddFinalizerIfNotPresent(r.client, cephFilesystem)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to add finalizer")
	}

	// The CR was just created, initializing status fields
	if cephFilesystem.Status == nil {
		updateStatus(r.client, request.NamespacedName, k8sutil.EmptyStatus, nil)
	}

	// Make sure a CephCluster is present otherwise do nothing
	cephCluster, isReadyToReconcile, cephClusterExists, reconcileResponse := opcontroller.IsReadyToReconcile(r.client, request.NamespacedName, controllerName)
	if !isReadyToReconcile {
		// This handles the case where the Ceph Cluster is gone and we want to delete that CR
		// We skip the deleteFilesystem() function since everything is gone already
		//
		// Also, only remove the finalizer if the CephCluster is gone
		// If not, we should wait for it to be ready
		// This handles the case where the operator is not ready to accept Ceph command but the cluster exists
		if !cephFilesystem.GetDeletionTimestamp().IsZero() && !cephClusterExists {
			// Remove finalizer
			err := opcontroller.RemoveFinalizer(r.client, cephFilesystem)
			if err != nil {
				return reconcile.Result{}, errors.Wrap(err, "failed to remove finalizer")
			}

			// Return and do not requeue. Successful deletion.
			return reconcile.Result{}, nil
		}
		return reconcileResponse, nil
	}
	r.cephClusterSpec = &cephCluster.Spec

	// Initialize the channel, it allows us to track multiple CephFilesystems in the same namespace
	_, fsChannelExists := r.fsChannels[fsChannelKeyName(cephFilesystem)]
	if !fsChannelExists {
		r.fsChannels[fsChannelKeyName(cephFilesystem)] = &fsHealth{
			stopChan:          make(chan struct{}),
			monitoringRunning: false,
		}
	}

	// Populate clusterInfo
	// Always populate it during each reconcile
	clusterInfo, _, _, err := mon.LoadClusterInfo(r.context, request.NamespacedName.Namespace)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to populate cluster info")
	}
	r.clusterInfo = clusterInfo

	// Populate CephVersion
	currentCephVersion, err := cephclient.LeastUptodateDaemonVersion(r.context, r.clusterInfo, opconfig.MonType)
	if err != nil {
		if strings.Contains(err.Error(), opcontroller.UninitializedCephConfigError) {
			logger.Info(opcontroller.OperatorNotInitializedMessage)
			return opcontroller.WaitForRequeueIfOperatorNotInitialized, nil
		}
		return reconcile.Result{}, errors.Wrapf(err, "failed to retrieve current ceph %q version", opconfig.MonType)
	}
	r.clusterInfo.CephVersion = currentCephVersion

	// DELETE: the CR was deleted
	if !cephFilesystem.GetDeletionTimestamp().IsZero() {
		logger.Debugf("deleting filesystem %q", cephFilesystem.Name)
		err = r.reconcileDeleteFilesystem(cephFilesystem)
		if err != nil {
			return reconcile.Result{}, errors.Wrapf(err, "failed to delete filesystem %q. ", cephFilesystem.Name)
		}

		// If the ceph fs still in the map, we must remove it during CR deletion
		if fsChannelExists {
			// Close the channel to stop the mirroring status
			close(r.fsChannels[fsChannelKeyName(cephFilesystem)].stopChan)

			// Remove ceph fs from the map
			delete(r.fsChannels, fsChannelKeyName(cephFilesystem))
		}

		// Remove finalizer
		err = opcontroller.RemoveFinalizer(r.client, cephFilesystem)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to remove finalizer")
		}

		// Return and do not requeue. Successful deletion.
		return reconcile.Result{}, nil
	}

	// validate the filesystem settings
	if err := validateFilesystem(r.context, r.clusterInfo, r.cephClusterSpec, cephFilesystem); err != nil {
		if strings.Contains(err.Error(), opcontroller.UninitializedCephConfigError) {
			logger.Info(opcontroller.OperatorNotInitializedMessage)
			return opcontroller.WaitForRequeueIfOperatorNotInitialized, nil
		}
		return reconcile.Result{}, errors.Wrapf(err, "invalid object filesystem %q arguments", cephFilesystem.Name)
	}

	// RECONCILE
	logger.Debug("reconciling ceph filesystem store deployments")
	reconcileResponse, err = r.reconcileCreateFilesystem(cephFilesystem)
	if err != nil {
		updateStatus(r.client, request.NamespacedName, cephv1.ConditionFailure, nil)
		return reconcileResponse, err
	}

	statusUpdated := false

	// Enable mirroring if needed
	if r.clusterInfo.CephVersion.IsAtLeast(mirror.PeerAdditionMinVersion) {
		// Disable mirroring on that filesystem if needed
		if cephFilesystem.Spec.Mirroring != nil {
			if !cephFilesystem.Spec.Mirroring.Enabled {
				err = cephclient.DisableFilesystemSnapshotMirror(r.context, r.clusterInfo, cephFilesystem.Name)
				if err != nil {
					return reconcile.Result{}, errors.Wrapf(err, "failed to disable mirroring on filesystem %q", cephFilesystem.Name)
				}
			} else {
				logger.Info("reconciling cephfs-mirror mirroring configuration")
				err = r.reconcileMirroring(cephFilesystem, request.NamespacedName)
				if err != nil {
					return opcontroller.ImmediateRetryResult, errors.Wrapf(err, "failed to configure mirroring for filesystem %q.", cephFilesystem.Name)
				}

				// Always create a bootstrap peer token in case another cluster wants to add us as a peer
				logger.Info("reconciling create cephfs-mirror peer configuration")
				reconcileResponse, err = opcontroller.CreateBootstrapPeerSecret(r.context, r.clusterInfo, cephFilesystem, k8sutil.NewOwnerInfo(cephFilesystem, r.scheme))
				if err != nil {
					updateStatus(r.client, request.NamespacedName, cephv1.ConditionFailure, nil)
					return reconcileResponse, errors.Wrapf(err, "failed to create cephfs-mirror bootstrap peer for filesystem %q.", cephFilesystem.Name)
				}

				logger.Info("reconciling add cephfs-mirror peer configuration")
				err = r.reconcileAddBoostrapPeer(cephFilesystem, request.NamespacedName)
				if err != nil {
					return opcontroller.ImmediateRetryResult, errors.Wrapf(err, "failed to configure mirroring for filesystem %q.", cephFilesystem.Name)
				}

				// Set Ready status, we are done reconciling
				updateStatus(r.client, request.NamespacedName, cephv1.ConditionReady, opcontroller.GenerateStatusInfo(cephFilesystem))
				statusUpdated = true

				// Run go routine check for mirroring status
				if !cephFilesystem.Spec.StatusCheck.Mirror.Disabled {
					// Start monitoring cephfs-mirror status
					if r.fsChannels[fsChannelKeyName(cephFilesystem)].monitoringRunning {
						logger.Debug("ceph filesystem mirror status monitoring go routine already running!")
					} else {
						checker := newMirrorChecker(r.context, r.client, r.clusterInfo, request.NamespacedName, &cephFilesystem.Spec, cephFilesystem.Name)
						r.fsChannels[fsChannelKeyName(cephFilesystem)].monitoringRunning = true
						go checker.checkMirroring(r.fsChannels[fsChannelKeyName(cephFilesystem)].stopChan)
					}
				}
			}
		}
	}
	if !statusUpdated {
		// Set Ready status, we are done reconciling
		updateStatus(r.client, request.NamespacedName, cephv1.ConditionReady, nil)
	}

	// Return and do not requeue
	logger.Debug("done reconciling")
	return reconcile.Result{}, nil
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

func (r *ReconcileCephFilesystem) reconcileAddBoostrapPeer(cephFilesystem *cephv1.CephFilesystem, namespacedName types.NamespacedName) error {
	if cephFilesystem.Spec.Mirroring.Peers == nil {
		return nil
	}
	ctx := context.TODO()
	// List all the peers secret, we can have more than one peer we might want to configure
	// For each, get the Kubernetes Secret and import the "peer token" so that we can configure the mirroring
	for _, peerSecret := range cephFilesystem.Spec.Mirroring.Peers.SecretNames {
		logger.Debugf("fetching bootstrap peer kubernetes secret %q", peerSecret)
		s, err := r.context.Clientset.CoreV1().Secrets(r.clusterInfo.Namespace).Get(ctx, peerSecret, metav1.GetOptions{})
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

func fsChannelKeyName(cephFilesystem *cephv1.CephFilesystem) string {
	return fmt.Sprintf("%s-%s", cephFilesystem.Namespace, cephFilesystem.Name)
}
