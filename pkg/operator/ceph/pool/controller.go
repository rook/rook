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
	"strings"

	"github.com/coreos/pkg/capnslog"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mgr"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/ceph/config"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/csi/peermap"
	"github.com/rook/rook/pkg/operator/k8sutil"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
	blockPoolChannels map[string]*blockPoolHealth
}

type blockPoolHealth struct {
	stopChan          chan struct{}
	monitoringRunning bool
}

// Add creates a new CephBlockPool Controller and adds it to the Manager. The Manager will set fields on the Controller
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
	return &ReconcileCephBlockPool{
		client:            mgr.GetClient(),
		scheme:            mgrScheme,
		context:           context,
		blockPoolChannels: make(map[string]*blockPoolHealth),
	}
}

func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}
	logger.Info("successfully started")

	// Watch for changes on the CephBlockPool CRD object
	err = c.Watch(&source.Kind{Type: &cephv1.CephBlockPool{TypeMeta: controllerTypeMeta}}, &handler.EnqueueRequestForObject{}, opcontroller.WatchControllerPredicate())
	if err != nil {
		return err
	}

	// Build Handler function to return the list of ceph block pool
	// This is used by the watchers below
	handlerFunc, err := opcontroller.ObjectToCRMapper(mgr.GetClient(), &cephv1.CephBlockPoolList{}, mgr.GetScheme())
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

// Reconcile reads that state of the cluster for a CephBlockPool object and makes changes based on the state read
// and what is in the CephBlockPool.Spec
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileCephBlockPool) Reconcile(context context.Context, request reconcile.Request) (reconcile.Result, error) {
	// workaround because the rook logging mechanism is not compatible with the controller-runtime logging interface
	reconcileResponse, err := r.reconcile(request)
	if err != nil {
		logger.Errorf("failed to reconcile. %v", err)
	}

	return reconcileResponse, err
}

func (r *ReconcileCephBlockPool) reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the CephBlockPool instance
	cephBlockPool := &cephv1.CephBlockPool{}
	err := r.client.Get(context.TODO(), request.NamespacedName, cephBlockPool)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("CephBlockPool resource not found. Ignoring since object must be deleted.")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed to get CephBlockPool")
	}

	// Set a finalizer so we can do cleanup before the object goes away
	err = opcontroller.AddFinalizerIfNotPresent(r.client, cephBlockPool)
	if err != nil {
		return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed to add finalizer")
	}

	// The CR was just created, initializing status fields
	if cephBlockPool.Status == nil {
		updateStatus(r.client, request.NamespacedName, cephv1.ConditionProgressing, nil)
	}

	// Make sure a CephCluster is present otherwise do nothing
	cephCluster, isReadyToReconcile, cephClusterExists, reconcileResponse := opcontroller.IsReadyToReconcile(r.client, r.context, request.NamespacedName, controllerName)
	if !isReadyToReconcile {
		// This handles the case where the Ceph Cluster is gone and we want to delete that CR
		// We skip the deletePool() function since everything is gone already
		//
		// Also, only remove the finalizer if the CephCluster is gone
		// If not, we should wait for it to be ready
		// This handles the case where the operator is not ready to accept Ceph command but the cluster exists
		if !cephBlockPool.GetDeletionTimestamp().IsZero() && !cephClusterExists {
			// Remove finalizer
			err = opcontroller.RemoveFinalizer(r.client, cephBlockPool)
			if err != nil {
				return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed to remove finalizer")
			}

			// Return and do not requeue. Successful deletion.
			return reconcile.Result{}, nil
		}
		return reconcileResponse, nil
	}

	// Populate clusterInfo during each reconcile
	clusterInfo, _, _, err := mon.LoadClusterInfo(r.context, request.NamespacedName.Namespace)
	if err != nil {
		return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed to populate cluster info")
	}
	r.clusterInfo = clusterInfo
	r.clusterInfo.NetworkSpec = cephCluster.Spec.Network

	// Initialize the channel for this pool
	// This allows us to track multiple CephBlockPool in the same namespace
	blockPoolChannelKey := fmt.Sprintf("%s-%s", cephBlockPool.Namespace, cephBlockPool.Name)
	_, poolChannelExists := r.blockPoolChannels[blockPoolChannelKey]
	if !poolChannelExists {
		r.blockPoolChannels[blockPoolChannelKey] = &blockPoolHealth{
			stopChan:          make(chan struct{}),
			monitoringRunning: false,
		}
	}

	// DELETE: the CR was deleted
	if !cephBlockPool.GetDeletionTimestamp().IsZero() {
		// If the ceph block pool is still in the map, we must remove it during CR deletion
		// We must remove it first otherwise the checker will panic since the status/info will be nil
		if poolChannelExists {
			r.cancelMirrorMonitoring(blockPoolChannelKey)
		}

		logger.Infof("deleting pool %q", cephBlockPool.Name)
		err := deletePool(r.context, clusterInfo, cephBlockPool)
		if err != nil {
			return opcontroller.ImmediateRetryResult, errors.Wrapf(err, "failed to delete pool %q. ", cephBlockPool.Name)
		}

		// disable RBD stats collection if cephBlockPool was deleted
		if err := configureRBDStats(r.context, clusterInfo); err != nil {
			logger.Errorf("failed to disable stats collection for pool(s). %v", err)
		}

		// Remove finalizer
		err = opcontroller.RemoveFinalizer(r.client, cephBlockPool)
		if err != nil {
			return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed to remove finalizer")
		}

		// Return and do not requeue. Successful deletion.
		return reconcile.Result{}, nil
	}

	// validate the pool settings
	if err := ValidatePool(r.context, clusterInfo, &cephCluster.Spec, cephBlockPool); err != nil {
		if strings.Contains(err.Error(), opcontroller.UninitializedCephConfigError) {
			logger.Info(opcontroller.OperatorNotInitializedMessage)
			return opcontroller.WaitForRequeueIfOperatorNotInitialized, nil
		}
		return opcontroller.ImmediateRetryResult, errors.Wrapf(err, "invalid pool CR %q spec", cephBlockPool.Name)
	}

	// Get CephCluster version
	cephVersion, err := opcontroller.GetImageVersion(cephCluster)
	if err != nil {
		return opcontroller.ImmediateRetryResult, errors.Wrapf(err, "failed to fetch ceph version from cephcluster %q", cephCluster.Name)
	}
	r.clusterInfo.CephVersion = *cephVersion

	// If the CephCluster has enabled the "pg_autoscaler" module and is running Nautilus
	// we force the pg_autoscale_mode to "on"
	_, propertyExists := cephBlockPool.Spec.Parameters[cephclient.PgAutoscaleModeProperty]
	if mgr.IsModuleInSpec(cephCluster.Spec.Mgr.Modules, mgr.PgautoscalerModuleName) &&
		!cephVersion.IsAtLeastOctopus() &&
		!propertyExists {
		if len(cephBlockPool.Spec.Parameters) == 0 {
			cephBlockPool.Spec.Parameters = make(map[string]string)
		}
		cephBlockPool.Spec.Parameters[cephclient.PgAutoscaleModeProperty] = cephclient.PgAutoscaleModeOn
	}

	// CREATE/UPDATE
	reconcileResponse, err = r.reconcileCreatePool(clusterInfo, &cephCluster.Spec, cephBlockPool)
	if err != nil {
		if strings.Contains(err.Error(), opcontroller.UninitializedCephConfigError) {
			logger.Info(opcontroller.OperatorNotInitializedMessage)
			return opcontroller.WaitForRequeueIfOperatorNotInitialized, nil
		}
		updateStatus(r.client, request.NamespacedName, cephv1.ConditionFailure, nil)
		return reconcileResponse, errors.Wrapf(err, "failed to create pool %q.", cephBlockPool.GetName())
	}

	// enable/disable RBD stats collection based on cephBlockPool spec
	if err := configureRBDStats(r.context, clusterInfo); err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to enable/disable stats collection for pool(s)")
	}

	checker := newMirrorChecker(r.context, r.client, r.clusterInfo, request.NamespacedName, &cephBlockPool.Spec, cephBlockPool.Name)
	// ADD PEERS
	logger.Debug("reconciling create rbd mirror peer configuration")
	if cephBlockPool.Spec.Mirroring.Enabled {
		// Always create a bootstrap peer token in case another cluster wants to add us as a peer
		reconcileResponse, err = opcontroller.CreateBootstrapPeerSecret(r.context, clusterInfo, cephBlockPool, k8sutil.NewOwnerInfo(cephBlockPool, r.scheme))
		if err != nil {
			updateStatus(r.client, request.NamespacedName, cephv1.ConditionFailure, nil)
			return reconcileResponse, errors.Wrapf(err, "failed to create rbd-mirror bootstrap peer for pool %q.", cephBlockPool.GetName())
		}

		// Check if rbd-mirror CR and daemons are running
		logger.Debug("listing rbd-mirror CR")
		// Run the goroutine to update the mirroring status
		if !cephBlockPool.Spec.StatusCheck.Mirror.Disabled {
			// Start monitoring of the pool
			if r.blockPoolChannels[blockPoolChannelKey].monitoringRunning {
				logger.Debug("pool monitoring go routine already running!")
			} else {
				r.blockPoolChannels[blockPoolChannelKey].monitoringRunning = true
				go checker.checkMirroring(r.blockPoolChannels[blockPoolChannelKey].stopChan)
			}
		}

		// Add bootstrap peer if any
		logger.Debug("reconciling ceph bootstrap peers import")
		reconcileResponse, err = r.reconcileAddBoostrapPeer(cephBlockPool, request.NamespacedName)
		if err != nil {
			return reconcileResponse, errors.Wrap(err, "failed to add ceph rbd mirror peer")
		}

		// ReconcilePoolIDMap updates the `rook-ceph-csi-mapping-config` with local and peer cluster pool ID map
		err = peermap.ReconcilePoolIDMap(r.context, r.clusterInfo, cephBlockPool)
		if err != nil {
			return reconcileResponse, errors.Wrapf(err, "failed to update pool ID mapping config for the pool %q", cephBlockPool.Name)
		}

		// Set Ready status, we are done reconciling
		updateStatus(r.client, request.NamespacedName, cephv1.ConditionReady, opcontroller.GenerateStatusInfo(cephBlockPool))

		// If not mirrored there is no Status Info field to fulfil
	} else {
		// Set Ready status, we are done reconciling
		updateStatus(r.client, request.NamespacedName, cephv1.ConditionReady, nil)

		// Stop monitoring the mirroring status of this pool
		if poolChannelExists && r.blockPoolChannels[blockPoolChannelKey].monitoringRunning {
			r.cancelMirrorMonitoring(blockPoolChannelKey)
			// Reset the MirrorHealthCheckSpec
			checker.updateStatusMirroring(nil, nil, nil, "")
		}
	}

	// Return and do not requeue
	logger.Debug("done reconciling")
	return reconcile.Result{}, nil
}

func (r *ReconcileCephBlockPool) reconcileCreatePool(clusterInfo *cephclient.ClusterInfo, cephCluster *cephv1.ClusterSpec, cephBlockPool *cephv1.CephBlockPool) (reconcile.Result, error) {
	err := createPool(r.context, clusterInfo, cephCluster, cephBlockPool)
	if err != nil {
		return opcontroller.ImmediateRetryResult, errors.Wrapf(err, "failed to create pool %q.", cephBlockPool.GetName())
	}

	// Let's return here so that on the initial creation we don't check for update right away
	return reconcile.Result{}, nil
}

// Create the pool
func createPool(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo, clusterSpec *cephv1.ClusterSpec, p *cephv1.CephBlockPool) error {
	// create the pool
	logger.Infof("creating pool %q in namespace %q", p.Name, p.Namespace)
	if err := cephclient.CreatePoolWithProfile(context, clusterInfo, clusterSpec, p.Name, p.Spec, poolApplicationNameRBD); err != nil {
		return errors.Wrapf(err, "failed to create pool %q", p.Name)
	}

	return nil
}

// Delete the pool
func deletePool(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo, p *cephv1.CephBlockPool) error {
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

func configureRBDStats(clusterContext *clusterd.Context, clusterInfo *cephclient.ClusterInfo) error {
	logger.Debug("configuring RBD per-image IO statistics collection")
	namespaceListOpt := client.InNamespace(clusterInfo.Namespace)
	cephBlockPoolList := &cephv1.CephBlockPoolList{}
	var enableStatsForPools []string
	err := clusterContext.Client.List(context.TODO(), cephBlockPoolList, namespaceListOpt)
	if err != nil {
		return errors.Wrap(err, "failed to retrieve list of CephBlockPool")
	}
	for _, cephBlockPool := range cephBlockPoolList.Items {
		if cephBlockPool.GetDeletionTimestamp() == nil && cephBlockPool.Spec.EnableRBDStats {
			// list of CephBlockPool with enableRBDStats set to true and not marked for deletion
			enableStatsForPools = append(enableStatsForPools, cephBlockPool.Name)
		}
	}
	logger.Debugf("RBD per-image IO statistics will be collected for pools: %v", enableStatsForPools)
	monStore := config.GetMonStore(clusterContext, clusterInfo)
	if len(enableStatsForPools) == 0 {
		err = monStore.Delete("mgr.", "mgr/prometheus/rbd_stats_pools")
	} else {
		err = monStore.Set("mgr.", "mgr/prometheus/rbd_stats_pools", strings.Join(enableStatsForPools, ","))
	}
	if err != nil {
		return errors.Wrapf(err, "failed to enable rbd_stats_pools")
	}
	logger.Debug("configured RBD per-image IO statistics collection")
	return nil
}

func (r *ReconcileCephBlockPool) cancelMirrorMonitoring(cephBlockPoolName string) {
	// Close the channel to stop the mirroring status
	close(r.blockPoolChannels[cephBlockPoolName].stopChan)

	// Remove ceph block pool from the map
	delete(r.blockPoolChannels, cephBlockPoolName)
}
