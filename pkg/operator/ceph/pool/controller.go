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

// Package pool to manage a rook pool.
package pool

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/coreos/pkg/capnslog"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/util/dependents"
	"github.com/rook/rook/pkg/util/exec"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/config"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/csi/peermap"
	"github.com/rook/rook/pkg/operator/ceph/reporting"
	"github.com/rook/rook/pkg/operator/k8sutil"
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
	poolApplicationNameRBD = "rbd"
	controllerName         = "ceph-block-pool-controller"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", controllerName)

var cephBlockPoolKind = reflect.TypeOf(cephv1.CephBlockPool{}).Name()

// Sets the type meta for the controller main object
var controllerTypeMeta = metav1.TypeMeta{
	Kind:       cephBlockPoolKind,
	APIVersion: fmt.Sprintf("%s/%s", cephv1.CustomResourceGroup, cephv1.Version),
}

var _ reconcile.Reconciler = &ReconcileCephBlockPool{}

// ReconcileCephBlockPool reconciles a CephBlockPool object
type ReconcileCephBlockPool struct {
	client            client.Client
	scheme            *runtime.Scheme
	context           *clusterd.Context
	clusterInfo       *cephclient.ClusterInfo
	blockPoolContexts map[string]*blockPoolHealth
	opManagerContext  context.Context
	recorder          record.EventRecorder
	opConfig          opcontroller.OperatorConfig
}

type blockPoolHealth struct {
	internalCtx    context.Context
	internalCancel context.CancelFunc
	started        bool
}

// Add creates a new CephBlockPool Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context, opConfig opcontroller.OperatorConfig) error {
	return add(opManagerContext, mgr, newReconciler(mgr, context, opManagerContext, opConfig))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context, opConfig opcontroller.OperatorConfig) reconcile.Reconciler {
	return &ReconcileCephBlockPool{
		client:            mgr.GetClient(),
		scheme:            mgr.GetScheme(),
		context:           context,
		blockPoolContexts: make(map[string]*blockPoolHealth),
		opManagerContext:  opManagerContext,
		recorder:          mgr.GetEventRecorderFor("rook-" + controllerName),
		opConfig:          opConfig,
	}
}

func add(opManagerContext context.Context, mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}
	logger.Info("successfully started")

	// Watch for changes on the CephBlockPool CRD object
	err = c.Watch(
		source.Kind(
			mgr.GetCache(),
			&cephv1.CephBlockPool{TypeMeta: controllerTypeMeta},
			&handler.TypedEnqueueRequestForObject[*cephv1.CephBlockPool]{},
			opcontroller.WatchControllerPredicate[*cephv1.CephBlockPool](mgr.GetScheme()),
		),
	)
	if err != nil {
		return err
	}

	// Build Handler function to return the list of ceph block pool
	// This is used by the watchers below
	configHandler, err := opcontroller.ObjectToCRMapper[*cephv1.CephBlockPoolList, *corev1.ConfigMap](
		opManagerContext,
		mgr.GetClient(),
		&cephv1.CephBlockPoolList{},
		mgr.GetScheme(),
	)
	if err != nil {
		return err
	}

	// Watch for ConfigMap "rook-ceph-mon-endpoints" update and reconcile, which will reconcile update the bootstrap peer token
	err = c.Watch(
		source.Kind(
			mgr.GetCache(),
			&corev1.ConfigMap{TypeMeta: metav1.TypeMeta{Kind: "ConfigMap", APIVersion: corev1.SchemeGroupVersion.String()}},
			handler.TypedEnqueueRequestsFromMapFunc(configHandler),
			mon.PredicateMonEndpointChanges(),
		),
	)
	if err != nil {
		return err
	}

	// Build Handler function to return the list of ceph block pool
	// This is used by the watchers below
	secretHandler, err := opcontroller.ObjectToCRMapper[*cephv1.CephBlockPoolList, *corev1.Secret](
		opManagerContext,
		mgr.GetClient(),
		&cephv1.CephBlockPoolList{},
		mgr.GetScheme(),
	)
	if err != nil {
		return err
	}
	// Watch for updates to the secret triggered by changes in the peer pool token
	err = c.Watch(
		source.Kind(
			mgr.GetCache(),
			&corev1.Secret{TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: corev1.SchemeGroupVersion.String()}},
			handler.TypedEnqueueRequestsFromMapFunc(secretHandler),
			opcontroller.WatchPeerTokenSecretPredicate(),
		),
	)
	if err != nil {
		return err
	}

	return nil
}

// Reconcile reads that state of the cluster for a CephBlockPool object and makes changes based on the state read
// and what is in the CephBlockPool.Spec
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileCephBlockPool) Reconcile(context context.Context, request reconcile.Request) (reconcile.Result, error) {
	defer opcontroller.RecoverAndLogException()
	// workaround because the rook logging mechanism is not compatible with the controller-runtime logging interface
	reconcileResponse, cephBlockPool, err := r.reconcile(request)

	return reporting.ReportReconcileResult(logger, r.recorder, request, &cephBlockPool, reconcileResponse, err)
}

func (r *ReconcileCephBlockPool) reconcile(request reconcile.Request) (reconcile.Result, cephv1.CephBlockPool, error) {
	// Fetch the CephBlockPool instance
	cephBlockPool := &cephv1.CephBlockPool{}
	err := r.client.Get(r.opManagerContext, request.NamespacedName, cephBlockPool)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("CephBlockPool resource not found. Ignoring since object must be deleted.")
			// If there was a previous error or if a user removed this resource's finalizer, it's
			// possible Rook didn't clean up the monitoring routine for this resource. Ensure the
			// routine is stopped when we see the resource is gone.
			cephBlockPool.Name = request.Name
			cephBlockPool.Namespace = request.Namespace
			r.cancelMirrorMonitoring(cephBlockPool)
			return reconcile.Result{}, *cephBlockPool, nil
		}
		// Error reading the object - requeue the request.
		return opcontroller.ImmediateRetryResult, *cephBlockPool, errors.Wrap(err, "failed to get CephBlockPool")
	}
	// update observedGeneration local variable with current generation value,
	// because generation can be changed before reconcile got completed
	// CR status will be updated at end of reconcile, so to reflect the reconcile has finished
	observedGeneration := cephBlockPool.ObjectMeta.Generation

	// Set a finalizer so we can do cleanup before the object goes away
	generationUpdated, err := opcontroller.AddFinalizerIfNotPresent(r.opManagerContext, r.client, cephBlockPool)
	if err != nil {
		return opcontroller.ImmediateRetryResult, *cephBlockPool, errors.Wrap(err, "failed to add finalizer")
	}
	if generationUpdated {
		logger.Infof("reconciling the ceph block pool %q after adding finalizer", cephBlockPool.Name)
		return reconcile.Result{}, *cephBlockPool, nil
	}

	var statusErr error
	// The CR was just created, initializing status fields
	if cephBlockPool.Status == nil {
		// The pool is not available so let's not build the status Info yet
		err = r.updateStatus(request.NamespacedName, cephv1.ConditionProgressing, k8sutil.ObservedGenerationNotAvailable, &cephv1.CephxStatus{})
		if err != nil {
			return opcontroller.ImmediateRetryResult, *cephBlockPool, errors.Wrapf(err, "failed to update %q status to %q", request.NamespacedName, cephv1.ConditionProgressing)
		}
	}

	// Make sure a CephCluster is present otherwise do nothing
	cephCluster, isReadyToReconcile, cephClusterExists, reconcileResponse := opcontroller.IsReadyToReconcile(r.opManagerContext, r.client, request.NamespacedName, controllerName)
	if !isReadyToReconcile {
		// This handles the case where the Ceph Cluster is gone and we want to delete that CR
		// We skip the deletePool() function since everything is gone already
		//
		// Also, only remove the finalizer if the CephCluster is gone
		// If not, we should wait for it to be ready
		// This handles the case where the operator is not ready to accept Ceph command but the cluster exists
		if !cephBlockPool.GetDeletionTimestamp().IsZero() && !cephClusterExists {
			// don't leak the health checker routine if we are force-deleting
			r.cancelMirrorMonitoring(cephBlockPool)

			// Remove finalizer
			err = opcontroller.RemoveFinalizer(r.opManagerContext, r.client, cephBlockPool)
			if err != nil {
				return opcontroller.ImmediateRetryResult, *cephBlockPool, errors.Wrap(err, "failed to remove finalizer")
			}

			// Return and do not requeue. Successful deletion.
			return reconcile.Result{}, *cephBlockPool, nil
		}
		return reconcileResponse, *cephBlockPool, nil
	}

	// Populate clusterInfo during each reconcile
	clusterInfo, _, _, err := opcontroller.LoadClusterInfo(r.context, r.opManagerContext, request.NamespacedName.Namespace, &cephCluster.Spec)
	if err != nil {
		return opcontroller.ImmediateRetryResult, *cephBlockPool, errors.Wrap(err, "failed to populate cluster info")
	}
	r.clusterInfo = clusterInfo

	// Initialize the channel for this pool
	// This allows us to track multiple CephBlockPool in the same namespace
	blockPoolChannelKey := blockPoolChannelKeyName(cephBlockPool)
	_, blockPoolContextsExists := r.blockPoolContexts[blockPoolChannelKey]
	if !blockPoolContextsExists {
		internalCtx, internalCancel := context.WithCancel(r.opManagerContext)
		r.blockPoolContexts[blockPoolChannelKey] = &blockPoolHealth{
			internalCtx:    internalCtx,
			internalCancel: internalCancel,
		}
	}

	poolSpec := cephBlockPool.ToNamedPoolSpec()
	// DELETE: the CR was deleted
	if !cephBlockPool.GetDeletionTimestamp().IsZero() {
		if err := r.handleDeletionBlocked(cephBlockPool, &cephCluster); err != nil {
			return opcontroller.WaitForRequeueIfFinalizerBlocked, *cephBlockPool, err
		}

		// If the ceph block pool is still in the map, we must remove it during CR deletion
		// We must remove it first otherwise the checker will panic since the status/info will be nil
		r.cancelMirrorMonitoring(cephBlockPool)

		r.recorder.Event(cephBlockPool, corev1.EventTypeNormal, string(cephv1.ReconcileStarted), "starting blockpool deletion")

		logger.Infof("deleting pool %q", poolSpec.Name)
		err = deletePool(r.context, clusterInfo, &poolSpec)
		if err != nil {
			return opcontroller.ImmediateRetryResult, *cephBlockPool, errors.Wrapf(err, "failed to delete pool %q. ", cephBlockPool.Name)
		}

		// disable RBD stats collection if cephBlockPool was deleted
		if err := configureRBDStats(r.context, clusterInfo, cephBlockPool.Name); err != nil {
			logger.Errorf("failed to disable stats collection for pool(s). %v", err)
		}

		// Remove finalizer
		err = opcontroller.RemoveFinalizer(r.opManagerContext, r.client, cephBlockPool)
		if err != nil {
			r.recorder.Event(cephBlockPool, corev1.EventTypeWarning, string(cephv1.ReconcileFailed), "failed to remove finalizer")
			return opcontroller.ImmediateRetryResult, *cephBlockPool, errors.Wrap(err, "failed to remove finalizer")
		}

		r.recorder.Event(cephBlockPool, corev1.EventTypeNormal, string(cephv1.ReconcileSucceeded), "successfully removed finalizer")

		// Return and do not requeue. Successful deletion.
		return reconcile.Result{}, *cephBlockPool, nil
	}

	// validate the pool settings
	if err := validatePool(r.context, clusterInfo, &cephCluster.Spec, cephBlockPool); err != nil {
		if strings.Contains(err.Error(), opcontroller.UninitializedCephConfigError) {
			logger.Info(opcontroller.OperatorNotInitializedMessage)
			return opcontroller.WaitForRequeueIfOperatorNotInitialized, *cephBlockPool, nil
		}
		return opcontroller.ImmediateRetryResult, *cephBlockPool, errors.Wrapf(err, "invalid pool CR %q spec", cephBlockPool.Name)
	}

	// Get CephCluster version
	cephVersion, err := opcontroller.GetImageVersion(cephCluster)
	if err != nil {
		return opcontroller.ImmediateRetryResult, *cephBlockPool, errors.Wrapf(err, "failed to fetch ceph version from cephcluster %q", cephCluster.Name)
	}
	r.clusterInfo.CephVersion = *cephVersion

	// CREATE/UPDATE
	reconcileResponse, err = r.reconcileCreatePool(clusterInfo, &cephCluster.Spec, cephBlockPool)
	if err != nil {
		if strings.Contains(err.Error(), opcontroller.UninitializedCephConfigError) {
			logger.Info(opcontroller.OperatorNotInitializedMessage)
			return opcontroller.WaitForRequeueIfOperatorNotInitialized, *cephBlockPool, nil
		}
		statusErr = r.updateStatus(request.NamespacedName, cephv1.ConditionFailure, k8sutil.ObservedGenerationNotAvailable, nil)
		if statusErr != nil {
			logger.Errorf("failed to update %q status to %q: %v", request.NamespacedName, cephv1.ConditionFailure, statusErr)
		}
		return reconcileResponse, *cephBlockPool, errors.Wrapf(err, "failed to create pool %q.", cephBlockPool.GetName())
	}

	// enable/disable RBD stats collection based on cephBlockPool spec
	if err := configureRBDStats(r.context, clusterInfo, ""); err != nil {
		return reconcile.Result{}, *cephBlockPool, errors.Wrap(err, "failed to enable/disable stats collection for pool(s)")
	}
	checker := cephclient.NewMirrorChecker(r.context, r.client, r.clusterInfo, request.NamespacedName, &poolSpec, cephBlockPool)
	// ADD PEERS
	logger.Debug("reconciling create rbd mirror peer configuration")
	if cephBlockPool.Spec.Mirroring.Enabled {
		// Always create a bootstrap peer token in case another cluster wants to add us as a peer
		reconcileResponse, err = opcontroller.CreateBootstrapPeerSecret(r.context, clusterInfo, cephBlockPool, k8sutil.NewOwnerInfo(cephBlockPool, r.scheme))
		if err != nil {
			statusErr = r.updateStatus(request.NamespacedName, cephv1.ConditionFailure, k8sutil.ObservedGenerationNotAvailable, nil)
			if statusErr != nil {
				return opcontroller.ImmediateRetryResult, *cephBlockPool, errors.Wrapf(statusErr, "failed to update %q status to %q", request.NamespacedName, cephv1.ConditionFailure)
			}
			return reconcileResponse, *cephBlockPool, errors.Wrapf(err, "failed to create rbd-mirror bootstrap peer for pool %q.", cephBlockPool.GetName())
		}

		// update rbdMirror cephXStatus immediately after bootstrapping the peer token
		statusErr = r.updateStatus(request.NamespacedName, cephv1.ConditionProgressing, observedGeneration, cephCluster.Status.Cephx.RBDMirrorPeer)
		if statusErr != nil {
			return opcontroller.ImmediateRetryResult, *cephBlockPool, errors.Wrapf(statusErr, "failed to update %q status to %q", request.NamespacedName, cephv1.ConditionProgressing)
		}

		// Check if rbd-mirror CR and daemons are running
		logger.Debug("listing rbd-mirror CR")

		// Add bootstrap peer if any
		logger.Debug("reconciling ceph bootstrap peers import")
		reconcileResponse, err = r.reconcileAddBootstrapPeer(cephBlockPool, request.NamespacedName)
		if err != nil {
			return reconcileResponse, *cephBlockPool, errors.Wrap(err, "failed to add ceph rbd mirror peer")
		}

		// ReconcilePoolIDMap updates the `rook-ceph-csi-mapping-config` with local and peer cluster pool ID map
		err = peermap.ReconcilePoolIDMap(r.opManagerContext, r.context, r.clusterInfo, cephBlockPool)
		if err != nil {
			return reconcileResponse, *cephBlockPool, errors.Wrapf(err, "failed to update pool ID mapping config for the pool %q", cephBlockPool.Name)
		}

		// update ObservedGeneration in status at the end of reconcile
		// Set Ready status, we are done reconciling
		statusErr = r.updateStatus(request.NamespacedName, cephv1.ConditionReady, observedGeneration, nil)

		if cephBlockPool.Spec.StatusCheck.Mirror.Disabled {
			// Stop monitoring the mirroring status of this pool
			if blockPoolContextsExists && r.blockPoolContexts[blockPoolChannelKey].started {
				logger.Info("stop monitoring the mirroring status of the pool %q", cephBlockPool.Name)
				r.cancelMirrorMonitoring(cephBlockPool)
				// Reset the MirrorHealthCheckSpec
				checker.UpdateStatusMirroring(nil, nil, nil, "")
			}
		} else {
			// Start monitoring of the pool
			if r.blockPoolContexts[blockPoolChannelKey].started {
				logger.Debug("pool monitoring go routine already running!")
			} else {
				if cephBlockPool.Spec.Mirroring.Mode != "init-only" {
					r.blockPoolContexts[blockPoolChannelKey].started = true
					// Run the goroutine to update the mirroring status and skip when blockpool mirroing mode in init-only as radosnamespace mirroring is the right place to check
					// mirroring status when blockpool mirroring mode is init-only.
					go checker.CheckMirroring(r.blockPoolContexts[blockPoolChannelKey].internalCtx)
				}
			}
		}

		// If not mirrored there is no Status Info field to fulfil
	} else {
		// disable mirroring
		err = r.disableMirroring(poolSpec.Name)
		if err != nil {
			logger.Warningf("failed to disable mirroring on pool %q running in ceph cluster namespace %q. %v", poolSpec.Name, r.clusterInfo.Namespace, err)
		}
		// update ObservedGeneration in status at the end of reconcile
		// Set Ready status, we are done reconciling
		statusErr = r.updateStatus(request.NamespacedName, cephv1.ConditionReady, observedGeneration, &cephv1.CephxStatus{})

		// Stop monitoring the mirroring status of this pool
		if blockPoolContextsExists && r.blockPoolContexts[blockPoolChannelKey].started {
			r.cancelMirrorMonitoring(cephBlockPool)
			// Reset the MirrorHealthCheckSpec
			checker.UpdateStatusMirroring(nil, nil, nil, "")
		}
	}

	if statusErr != nil {
		return opcontroller.ImmediateRetryResult, *cephBlockPool, errors.Wrapf(statusErr, "failed to update status of pool %q to %q.", cephBlockPool.Name, cephv1.ConditionReady)
	}

	// Return and do not requeue
	logger.Debug("done reconciling")
	return reconcile.Result{}, *cephBlockPool, nil
}

// handlePoolDeletionBlocked updates the blockpool CR status with conditions about
// whether the pool is empty or has dependents that block deletion.
// If the pool is not empty and force deletion is specified, create a cleanup job
// to delete the images and snapshots forcefully.
func (r *ReconcileCephBlockPool) handleDeletionBlocked(cephBlockPool *cephv1.CephBlockPool, cephCluster *cephv1.CephCluster) error {
	poolSpec := cephBlockPool.ToNamedPoolSpec()
	deletionBlocked := false

	deps, err := cephBlockPoolDependents(r.context, r.clusterInfo, cephBlockPool)
	if err != nil {
		return err
	}
	var depCondition cephv1.Condition
	if deps.Empty() {
		_, _, depCondition = reporting.GenerateConditionUnblockedDueToDependents(cephBlockPool)
	} else {
		deletionBlocked = true
		_, _, depCondition = reporting.GenerateConditionBlockedDueToDependents(cephBlockPool, deps)
	}
	logger.Info(depCondition.Message)

	radosNamespaces := deps.OfKind(radosNamespacesKeyName)
	isEmpty, emptyMessage, err := cephclient.IsPoolEmpty(r.context, r.clusterInfo, poolSpec.Name, radosNamespaces)
	if err != nil {
		return err
	}
	var emptyCondition cephv1.Condition
	if isEmpty {
		emptyCondition = dependents.DeletionBlockedDueToNonEmptyPoolCondition(false, emptyMessage)
	} else {
		deletionBlocked = true
		emptyCondition = dependents.DeletionBlockedDueToNonEmptyPoolCondition(true, emptyMessage)
	}
	logger.Info(emptyCondition.Message)

	nsName := types.NamespacedName{Namespace: cephBlockPool.Namespace, Name: cephBlockPool.Name}
	err = reporting.UpdateStatusConditionsWithRetry(
		r.opManagerContext, r.client, cephBlockPool, nsName, cephBlockPool.Kind, emptyCondition, depCondition)
	if err != nil {
		logger.Warningf("failed to update %q status with deletion blocked conditions: %v", nsName.String(), err)
	}

	if !isEmpty {
		// Force deletion if desired
		if opcontroller.ForceDeleteRequested(cephBlockPool.GetAnnotations()) {
			cleanupErr := r.cleanup(cephBlockPool, cephCluster)
			if cleanupErr != nil {
				return errors.Wrapf(cleanupErr, "failed to create clean up job for ceph blockpool %q", cephBlockPool.Name)
			}
		}
	}

	if deletionBlocked {
		return errors.Errorf("pool %q cannot be deleted because it is not empty or has dependents", cephBlockPool.Name)
	}
	return nil
}

func (r *ReconcileCephBlockPool) reconcileCreatePool(clusterInfo *cephclient.ClusterInfo, cephCluster *cephv1.ClusterSpec, cephBlockPool *cephv1.CephBlockPool) (reconcile.Result, error) {
	poolSpec := cephBlockPool.ToNamedPoolSpec()
	err := createPool(r.context, clusterInfo, cephCluster, &poolSpec)
	if err != nil {
		return opcontroller.ImmediateRetryResult, errors.Wrapf(err, "failed to configure pool %q.", cephBlockPool.GetName())
	}

	// Let's return here so that on the initial creation we don't check for update right away
	return reconcile.Result{}, nil
}

func (r *ReconcileCephBlockPool) cleanup(cephblockpool *cephv1.CephBlockPool, cephCluster *cephv1.CephCluster) error {
	logger.Infof("starting cleanup of the ceph resources for CephBlockPool %q in k8s namespace %q", cephblockpool.Name, cephblockpool.Namespace)
	cleanupConfig := map[string]string{
		opcontroller.CephBlockPoolNameEnv: cephblockpool.Name,
	}
	cleanup := opcontroller.NewResourceCleanup(cephblockpool, cephCluster, r.opConfig.Image, cleanupConfig)
	jobName := k8sutil.TruncateNodeNameForJob("cleanup-cephblockpool-%s", cephblockpool.Name)
	err := cleanup.StartJob(r.clusterInfo.Context, r.context.Clientset, jobName)
	if err != nil {
		return errors.Wrapf(err, "failed to run clean up job to clean the cephblockpool %q", cephblockpool.Name)
	}
	return nil
}

// Create the pool
func createPool(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo, clusterSpec *cephv1.ClusterSpec, p *cephv1.NamedPoolSpec) error {
	// Set the application name to rbd by default, but override later for special pools
	if p.Application == "" {
		p.Application = poolApplicationNameRBD
	}
	// create the pool
	logger.Infof("creating pool %q in namespace %q", p.Name, clusterInfo.Namespace)
	if err := cephclient.CreatePool(context, clusterInfo, clusterSpec, p); err != nil {
		return errors.Wrapf(err, "failed to configure pool %q", p.Name)
	}

	if p.Application != poolApplicationNameRBD {
		return nil
	}
	logger.Infof("initializing pool %q for RBD use", p.Name)
	args := []string{"pool", "init", p.Name}
	output, err := cephclient.NewRBDCommand(context, clusterInfo, args).RunWithTimeout(exec.CephCommandsTimeout)
	if err != nil {
		return errors.Wrapf(err, "failed to initialize pool %q for RBD use. %s", p.Name, string(output))
	}
	logger.Infof("successfully initialized pool %q for RBD use", p.Name)

	return nil
}

// Delete the pool
func deletePool(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo, p *cephv1.NamedPoolSpec) error {
	pools, err := cephclient.ListPoolSummaries(context, clusterInfo)
	if err != nil {
		return errors.Wrap(err, "failed to list pools")
	}

	// Only delete the pool if it exists...
	for _, pool := range pools {
		if pool.Name == p.Name {
			err := cephclient.DeletePool(context, clusterInfo, p.Name)
			if err != nil {
				return errors.Wrapf(err, "failed to delete pool %q", p.Name)
			}
		}
	}

	return nil
}

// generateStatsPoolList combines existingStatsPools and rookStatsPools, removes items in removePools,
// removes duplicates, ensures no empty strings, and returns a comma-separated string in a deterministic order.
func generateStatsPoolList(existingStatsPools []string, rookStatsPools []string, removePools []string) string {
	poolList := []string{}

	// Helper function to add a poolList if it's not in the removePools list and not already in poolList
	addUniquePool := func(pool string) {
		if pool == "" {
			return
		}
		// Check if the pool should be removed or already exists in poolList
		if contains(removePools, pool) || contains(poolList, pool) {
			return
		}
		poolList = append(poolList, pool)
	}
	for _, pool := range existingStatsPools {
		addUniquePool(pool)
	}
	for _, pool := range rookStatsPools {
		addUniquePool(pool)
	}

	sort.Strings(poolList) // Sort the list to ensure deterministic output

	return strings.Join(poolList, ",")
}

// Helper function to check if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func configureRBDStats(clusterContext *clusterd.Context, clusterInfo *cephclient.ClusterInfo, deletedPool string) error {
	logger.Debug("configuring RBD per-image IO statistics collection")
	namespaceListOpt := client.InNamespace(clusterInfo.Namespace)
	cephBlockPoolList := &cephv1.CephBlockPoolList{}
	var rookStatsPools []string
	var removePools []string

	err := clusterContext.Client.List(clusterInfo.Context, cephBlockPoolList, namespaceListOpt)
	if err != nil {
		return errors.Wrap(err, "failed to retrieve list of CephBlockPool")
	}
	for _, cephBlockPool := range cephBlockPoolList.Items {
		if cephBlockPool.GetDeletionTimestamp() == nil && cephBlockPool.Spec.EnableRBDStats {
			// add to list of CephBlockPool with enableRBDStats set to true and not marked for deletion
			rookStatsPools = append(rookStatsPools, cephBlockPool.ToNamedPoolSpec().Name)
		} else {
			removePools = append(removePools, cephBlockPool.ToNamedPoolSpec().Name)
		}
	}
	if deletedPool != "" {
		removePools = append(removePools, deletedPool)
	}
	monStore := config.GetMonStore(clusterContext, clusterInfo)
	// Check for existing rbd stats pools
	existingStatsPools, e := monStore.Get("mgr", "mgr/prometheus/rbd_stats_pools")
	if e != nil {
		return errors.Wrapf(e, "failed to get rbd_stats_pools")
	}
	existingStatsPoolsList := strings.Split(existingStatsPools, ",")
	enableStatsForPools := generateStatsPoolList(existingStatsPoolsList, rookStatsPools, removePools)
	logger.Debugf("RBD per-image IO statistics will be collected for pools: %v", enableStatsForPools)
	if len(enableStatsForPools) == 0 {
		err = monStore.Delete("mgr", "mgr/prometheus/rbd_stats_pools")
	} else {
		// appending existing rbd stats pools if any
		err = monStore.Set("mgr", "mgr/prometheus/rbd_stats_pools", enableStatsForPools)
	}
	if err != nil {
		return errors.Wrapf(err, "failed to enable rbd_stats_pools")
	}
	logger.Debug("configured RBD per-image IO statistics collection")
	return nil
}

func blockPoolChannelKeyName(p *cephv1.CephBlockPool) string {
	return types.NamespacedName{Namespace: p.Namespace, Name: p.Name}.String()
}

// cancel mirror monitoring. This is a noop if monitoring is not running.
func (r *ReconcileCephBlockPool) cancelMirrorMonitoring(cephBlockPool *cephv1.CephBlockPool) {
	channelKey := blockPoolChannelKeyName(cephBlockPool)

	_, poolContextExists := r.blockPoolContexts[channelKey]
	if poolContextExists {
		// Cancel the context to stop the go routine
		r.blockPoolContexts[channelKey].internalCancel()

		// Remove ceph block pool from the map
		delete(r.blockPoolContexts, channelKey)
	}
}

func (r *ReconcileCephBlockPool) disableMirroring(pool string) error {
	mirrorInfo, err := cephclient.GetPoolMirroringInfo(r.context, r.clusterInfo, pool)
	if err != nil {
		return errors.Wrapf(err, "failed to get mirroring info for the pool %q", pool)
	}
	if mirrorInfo.Mode == "disabled" {
		return nil
	}

	mirroringEnabled, err := r.isAnyRadosNamespaceMirrored(pool)
	if err != nil {
		return errors.Wrap(err, "failed to check if any rados namespace is mirrored")
	}
	if mirroringEnabled {
		logger.Debugf("disabling mirroring on pool %q is not possible. There are mirrored rados namespaces in the pool running in ceph cluster namespace %q", pool, r.clusterInfo.Namespace)
		return errors.New("mirroring must be disabled in all radosnamespaces in the pool before disabling mirroring in the pool")
	}

	if mirrorInfo.Mode == "image" {
		mirroredPools, err := cephclient.GetMirroredPoolImages(r.context, r.clusterInfo, pool)
		if err != nil {
			return errors.Wrapf(err, "failed to list mirrored images for pool %q", pool)
		}

		if len(*mirroredPools.Images) > 0 {
			msg := fmt.Sprintf("there are images in the pool %q. Please manually disable mirroring for each image", pool)
			logger.Errorf("%s", msg)
			return errors.New(msg)
		}
	}

	// Remove storage cluster peers
	for _, peer := range mirrorInfo.Peers {
		if peer.UUID != "" {
			err := cephclient.RemoveClusterPeer(r.context, r.clusterInfo, pool, peer.UUID)
			if err != nil {
				return errors.Wrapf(err, "failed to remove cluster peer with UUID %q for the pool %q", peer.UUID, pool)
			}
			logger.Infof("successfully removed peer site %q for the pool %q", peer.UUID, pool)
		}
	}

	// Disable mirroring on pool
	err = cephclient.DisablePoolMirroring(r.context, r.clusterInfo, pool)
	if err != nil {
		return errors.Wrapf(err, "failed to disable mirroring for pool %q", pool)
	}
	logger.Infof("successfully disabled mirroring on the pool %q", pool)

	return nil
}

func (r *ReconcileCephBlockPool) isAnyRadosNamespaceMirrored(poolName string) (bool, error) {
	logger.Debugf("list rados namespace in pool %q running in ceph cluster namespace %q", poolName, r.clusterInfo.Namespace)

	list, err := cephclient.ListRadosNamespacesInPool(r.context, r.clusterInfo, poolName)
	if err != nil {
		return false, errors.Wrapf(err, "failed to list rados namespace in pool %q", poolName)
	}
	logger.Debugf("rados namespace list %v in pool %q running in ceph cluster namespace %q", list, poolName, r.clusterInfo.Namespace)

	for _, namespace := range list {
		poolAndRadosNamespaceName := fmt.Sprintf("%s/%s", poolName, namespace)
		mirrorInfo, err := cephclient.GetPoolMirroringInfo(r.context, r.clusterInfo, poolAndRadosNamespaceName)
		if err != nil {
			return false, errors.Wrapf(err, "failed to get mirroring info for the rados namespace %q", poolAndRadosNamespaceName)
		}
		logger.Debugf("mirroring info for the rados namespace %q running in ceph cluster namespace %q: %v", poolAndRadosNamespaceName, r.clusterInfo.Namespace, mirrorInfo)
		if mirrorInfo.Mode != "disabled" {
			return true, nil
		}
	}

	return false, nil
}
