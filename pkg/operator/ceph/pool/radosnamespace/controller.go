/*
Copyright 2022 The Rook Authors. All rights reserved.

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

// Package radosnamespace to manage rbd pool namespaces
package radosnamespace

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	csiopv1a1 "github.com/ceph/ceph-csi-operator/api/v1alpha1"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/csi"
	"github.com/rook/rook/pkg/operator/ceph/reporting"
	"github.com/rook/rook/pkg/operator/k8sutil"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
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

	cephcsi "github.com/ceph/ceph-csi/api/deploy/kubernetes"
)

const (
	controllerName = "blockpool-rados-namespace-controller"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", controllerName)

var poolNamespace = reflect.TypeOf(cephv1.CephBlockPoolRadosNamespace{}).Name()

// Sets the type meta for the controller main object
var controllerTypeMeta = metav1.TypeMeta{
	Kind:       poolNamespace,
	APIVersion: fmt.Sprintf("%s/%s", cephv1.CustomResourceGroup, cephv1.Version),
}

// ReconcileCephBlockPoolRadosNamespace reconciles a CephBlockPoolRadosNamespace object
type ReconcileCephBlockPoolRadosNamespace struct {
	client                 client.Client
	scheme                 *runtime.Scheme
	context                *clusterd.Context
	clusterInfo            *cephclient.ClusterInfo
	radosNamespaceContexts map[string]*mirrorHealth
	opManagerContext       context.Context
	opConfig               opcontroller.OperatorConfig
}

type mirrorHealth struct {
	internalCtx    context.Context
	internalCancel context.CancelFunc
	started        bool
}

// Add creates a new CephBlockPoolRadosNamespace Controller and adds it to the
// Manager. The Manager will set fields on the Controller and Start it when the
// Manager is Started.
func Add(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context, opConfig opcontroller.OperatorConfig) error {
	return add(mgr, newReconciler(mgr, context, opManagerContext, opConfig))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, context *clusterd.Context, opManagerContext context.Context, opConfig opcontroller.OperatorConfig) reconcile.Reconciler {
	return &ReconcileCephBlockPoolRadosNamespace{
		client:                 mgr.GetClient(),
		scheme:                 mgr.GetScheme(),
		context:                context,
		radosNamespaceContexts: make(map[string]*mirrorHealth),
		opManagerContext:       opManagerContext,
		opConfig:               opConfig,
	}
}

func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}
	logger.Info("successfully started")

	// Watch for changes on the CephBlockPoolRadosNamespace CRD object
	err = c.Watch(source.Kind[client.Object](mgr.GetCache(), &cephv1.CephBlockPoolRadosNamespace{TypeMeta: controllerTypeMeta}, &handler.EnqueueRequestForObject{}, opcontroller.WatchControllerPredicate()))
	if err != nil {
		return err
	}

	err = csiopv1a1.AddToScheme(mgr.GetScheme())
	if err != nil {
		return err
	}

	return nil
}

// Reconcile reads that state of the cluster for a CephBlockPoolRadosNamespace
// object and makes changes based on the state read and what is in the
// CephBlockPoolRadosNamespace.Spec The Controller will requeue the Request to be
// processed again if the returned error is non-nil or Result.Requeue is true,
// otherwise upon completion it will remove the work from the queue.
func (r *ReconcileCephBlockPoolRadosNamespace) Reconcile(context context.Context, request reconcile.Request) (reconcile.Result, error) {
	// workaround because the rook logging mechanism is not compatible with the controller-runtime logging interface
	reconcileResponse, err := r.reconcile(request)
	if err != nil {
		logger.Errorf("failed to reconcile %q %v", request.NamespacedName, err)
	}

	return reconcileResponse, err
}

func (r *ReconcileCephBlockPoolRadosNamespace) reconcile(request reconcile.Request) (reconcile.Result, error) {
	namespacedName := request.NamespacedName
	// Fetch the CephBlockPoolRadosNamespace instance
	cephBlockPoolRadosNamespace := &cephv1.CephBlockPoolRadosNamespace{}
	err := r.client.Get(r.opManagerContext, request.NamespacedName, cephBlockPoolRadosNamespace)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debugf("cephBlockPoolRadosNamespace resource %q not found. Ignoring since object must be deleted.", namespacedName)
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, errors.Wrap(err, "failed to get cephBlockPoolRadosNamespace")
	}

	// Set a finalizer so we can do cleanup before the object goes away
	err = opcontroller.AddFinalizerIfNotPresent(r.opManagerContext, r.client, cephBlockPoolRadosNamespace)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to add finalizer")
	}

	// The CR was just created, initializing status fields
	if cephBlockPoolRadosNamespace.Status == nil {
		r.updateStatus(r.client, request.NamespacedName, cephv1.ConditionProgressing)
	}

	poolAndRadosNamespaceName := fmt.Sprintf("%s/%s", cephBlockPoolRadosNamespace.Spec.BlockPoolName, getRadosNamespaceName(cephBlockPoolRadosNamespace))

	// Make sure a CephCluster is present otherwise do nothing
	cephCluster, isReadyToReconcile, cephClusterExists, reconcileResponse := opcontroller.IsReadyToReconcile(r.opManagerContext, r.client, request.NamespacedName, controllerName)
	if !isReadyToReconcile {
		// This handles the case where the Ceph Cluster is gone and we want to delete that CR
		// We skip the deleteRadosNamespace() function since everything is gone already
		//
		// Also, only remove the finalizer if the CephCluster is gone
		// If not, we should wait for it to be ready
		// This handles the case where the operator is not ready to accept Ceph command but the cluster exists
		if !cephBlockPoolRadosNamespace.GetDeletionTimestamp().IsZero() && !cephClusterExists {
			// don't leak the health checker routine if we are force-deleting
			r.cancelMirrorMonitoring(radosNamespaceChannelKeyName(cephBlockPoolRadosNamespace.Namespace, poolAndRadosNamespaceName))
			// Remove finalizer
			err = opcontroller.RemoveFinalizer(r.opManagerContext, r.client, cephBlockPoolRadosNamespace)
			if err != nil {
				return opcontroller.ImmediateRetryResult, errors.Wrap(err, "failed to remove finalizer")
			}

			// Return and do not requeue. Successful deletion.
			return reconcile.Result{}, nil
		}
		return reconcileResponse, nil
	}

	// Populate clusterInfo during each reconcile
	r.clusterInfo, _, _, err = opcontroller.LoadClusterInfo(r.context, r.opManagerContext, request.NamespacedName.Namespace, &cephCluster.Spec)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to populate cluster info")
	}
	r.clusterInfo.Context = r.opManagerContext

	// DELETE: the CR was deleted
	if !cephBlockPoolRadosNamespace.GetDeletionTimestamp().IsZero() {
		logger.Debugf("delete cephBlockPoolRadosNamespace %q", namespacedName)
		// On external cluster, we don't delete the rados namespace, it has to be deleted manually
		if cephCluster.Spec.External.Enable {
			logger.Warning("external rados namespace %q deletion is not supported, delete it manually", namespacedName)
		} else {
			err := r.deleteRadosNamespace(cephBlockPoolRadosNamespace)
			if err != nil {
				if strings.Contains(err.Error(), opcontroller.UninitializedCephConfigError) {
					logger.Info(opcontroller.OperatorNotInitializedMessage)
					return opcontroller.WaitForRequeueIfOperatorNotInitialized, nil
				}
				if opcontroller.ForceDeleteRequested(cephBlockPoolRadosNamespace.GetAnnotations()) {
					cleanupErr := r.cleanup(cephBlockPoolRadosNamespace, &cephCluster)
					if cleanupErr != nil {
						return reconcile.Result{}, errors.Wrapf(cleanupErr, "failed to create clean up job for ceph blockpool rados namespace %q", namespacedName)
					}
				}
				return reconcile.Result{}, errors.Wrapf(err, "failed to delete ceph blockpool rados namespace %q", cephBlockPoolRadosNamespace.Name)
			}
			// If the ceph block pool is still in the map, we must remove it during CR deletion
			// We must remove it first otherwise the checker will panic since the status/info will be nil
			r.cancelMirrorMonitoring(radosNamespaceChannelKeyName(cephBlockPoolRadosNamespace.Namespace, poolAndRadosNamespaceName))
		}
		err = csi.SaveClusterConfig(r.context.Clientset, buildClusterID(cephBlockPoolRadosNamespace), cephCluster.Namespace, r.clusterInfo, nil)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to save cluster config")
		}
		// Remove finalizer
		err = opcontroller.RemoveFinalizer(r.opManagerContext, r.client, cephBlockPoolRadosNamespace)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to remove finalizer")
		}

		// Return and do not requeue. Successful deletion.
		return reconcile.Result{}, nil
	}

	cephBlockPoolRadosNamespaceName := cephBlockPoolRadosNamespace.Name
	if cephBlockPoolRadosNamespace.Spec.Name != "" {
		cephBlockPoolRadosNamespaceName = cephBlockPoolRadosNamespace.Spec.Name
	}

	if cephCluster.Spec.External.Enable {
		logger.Debug("skip creating external radosnamespace in external mode, create it manually, the controller will assume it's there")
		err = r.updateClusterConfig(cephBlockPoolRadosNamespace, cephCluster)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to save cluster config")
		}
		r.updateStatus(r.client, namespacedName, cephv1.ConditionReady)
		if csi.EnableCSIOperator() {
			err = csi.CreateUpdateClientProfileRadosNamespace(r.clusterInfo.Context, r.client, r.clusterInfo, cephBlockPoolRadosNamespaceName, buildClusterID(cephBlockPoolRadosNamespace), cephCluster.Name)
			if err != nil {
				return reconcile.Result{}, errors.Wrap(err, "failed to create ceph csi-op config CR for RadosNamespace")
			}
		}
		return reconcile.Result{}, nil
	}

	// cephversion check is only required for enabling mirroring
	if cephBlockPoolRadosNamespace.Spec.Mirroring != nil {
		// Get CephCluster version
		cephVersion, err := opcontroller.GetImageVersion(cephCluster)
		if err != nil {
			return opcontroller.ImmediateRetryResult, errors.Wrapf(err, "failed to fetch ceph version from cephcluster %q running in namespace %q", cephCluster.Name, cephCluster.Namespace)
		}
		if cephVersion != nil {
			r.clusterInfo.CephVersion = *cephVersion
		}
	}

	// Build the NamespacedName to fetch the CephBlockPool and make sure it exists, if not we cannot
	// create the rados namespace
	cephBlockPool := &cephv1.CephBlockPool{}
	pool := cephBlockPoolRadosNamespace.Spec.BlockPoolName
	cephBlockPoolNamespacedName := types.NamespacedName{Name: pool, Namespace: request.Namespace}

	err = r.client.Get(r.opManagerContext, cephBlockPoolNamespacedName, cephBlockPool)
	if err != nil {
		if kerrors.IsNotFound(err) {
			return reconcile.Result{}, errors.Wrapf(err, "failed to fetch ceph blockpool %q, cannot create rados namespace %q", pool, cephBlockPoolRadosNamespace.Name)
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, errors.Wrap(err, "failed to get cephBlockPoolRadosNamespace")
	}

	// If the cephBlockPool is not ready to accept commands, we should wait for it to be ready
	if cephBlockPool.Status.Phase != cephv1.ConditionReady {
		// We know the CR is present so it should a matter of second for it to become ready
		return reconcile.Result{Requeue: true, RequeueAfter: 10 * time.Second}, errors.Wrapf(err, "failed to fetch ceph blockpool %q, cannot create rados namespace %q", pool, cephBlockPoolRadosNamespace.Name)
	}
	// Create or Update rados namespace
	err = r.createOrUpdateRadosNamespace(cephBlockPoolRadosNamespace)
	if err != nil {
		if strings.Contains(err.Error(), opcontroller.UninitializedCephConfigError) {
			logger.Info(opcontroller.OperatorNotInitializedMessage)
			return opcontroller.WaitForRequeueIfOperatorNotInitialized, nil
		}
		r.updateStatus(r.client, request.NamespacedName, cephv1.ConditionFailure)
		return reconcile.Result{}, errors.Wrapf(err, "failed to create or update ceph pool rados namespace %q", cephBlockPoolRadosNamespace.Name)
	}

	err = r.updateClusterConfig(cephBlockPoolRadosNamespace, cephCluster)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to save cluster config")
	}

	err = r.reconcileMirroring(cephBlockPoolRadosNamespace, cephBlockPool)
	if err != nil {
		return reconcile.Result{}, err
	}

	r.updateStatus(r.client, namespacedName, cephv1.ConditionReady)

	if csi.EnableCSIOperator() {
		err = csi.CreateUpdateClientProfileRadosNamespace(r.clusterInfo.Context, r.client, r.clusterInfo, cephBlockPoolRadosNamespaceName, buildClusterID(cephBlockPoolRadosNamespace), cephCluster.Name)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to create ceph csi-op config CR for RadosNamespace")
		}
	}

	// Return and do not requeue
	logger.Debugf("done reconciling cephBlockPoolRadosNamespace %q", namespacedName)
	return reconcile.Result{}, nil
}

func getRadosNamespaceName(cephBlockPoolRadosNamespace *cephv1.CephBlockPoolRadosNamespace) string {
	if cephBlockPoolRadosNamespace.Spec.Name != "" {
		return cephBlockPoolRadosNamespace.Spec.Name
	}
	return cephBlockPoolRadosNamespace.Name
}

func (r *ReconcileCephBlockPoolRadosNamespace) updateClusterConfig(cephBlockPoolRadosNamespace *cephv1.CephBlockPoolRadosNamespace, cephCluster cephv1.CephCluster) error {
	// Update CSI config map
	// If the mon endpoints change, the mon health check go routine will take care of updating the
	// config map, so no special care is needed in this controller
	csiClusterConfigEntry := csi.CSIClusterConfigEntry{
		Namespace: r.clusterInfo.Namespace,
		ClusterInfo: cephcsi.ClusterInfo{
			Monitors: csi.MonEndpoints(r.clusterInfo.AllMonitors(), cephCluster.Spec.RequireMsgr2()),
			RBD: cephcsi.RBD{
				RadosNamespace: getRadosNamespaceName(cephBlockPoolRadosNamespace),
			},
			CephFS: cephcsi.CephFS{
				KernelMountOptions: r.clusterInfo.CSIDriverSpec.CephFS.KernelMountOptions,
				FuseMountOptions:   r.clusterInfo.CSIDriverSpec.CephFS.FuseMountOptions,
			},
			ReadAffinity: cephcsi.ReadAffinity{
				Enabled:             r.clusterInfo.CSIDriverSpec.ReadAffinity.Enabled,
				CrushLocationLabels: r.clusterInfo.CSIDriverSpec.ReadAffinity.CrushLocationLabels,
			},
		},
	}

	csiClusterConfigEntry.RBD.NetNamespaceFilePath = ""

	// Save cluster config in the csi config map
	err := csi.SaveClusterConfig(r.context.Clientset, buildClusterID(cephBlockPoolRadosNamespace), cephCluster.Namespace, r.clusterInfo, &csiClusterConfigEntry)
	if err != nil {
		return errors.Wrap(err, "failed to save cluster config")
	}

	return nil
}

// Create the ceph blockpool rados namespace
func (r *ReconcileCephBlockPoolRadosNamespace) createOrUpdateRadosNamespace(cephBlockPoolRadosNamespace *cephv1.CephBlockPoolRadosNamespace) error {
	namespacedName := fmt.Sprintf("%s/%s", cephBlockPoolRadosNamespace.Namespace, cephBlockPoolRadosNamespace.Name)
	logger.Infof("creating ceph blockpool rados namespace %q", namespacedName)

	err := cephclient.CreateRadosNamespace(r.context, r.clusterInfo, cephBlockPoolRadosNamespace.Spec.BlockPoolName, getRadosNamespaceName(cephBlockPoolRadosNamespace))
	if err != nil {
		return errors.Wrapf(err, "failed to create ceph blockpool rados namespace %q", cephBlockPoolRadosNamespace.Name)
	}

	return nil
}

// Delete the ceph blockpool rados namespace
func (r *ReconcileCephBlockPoolRadosNamespace) deleteRadosNamespace(cephBlockPoolRadosNamespace *cephv1.CephBlockPoolRadosNamespace) error {
	namespacedName := fmt.Sprintf("%s/%s", cephBlockPoolRadosNamespace.Namespace, cephBlockPoolRadosNamespace.Name)
	logger.Infof("deleting ceph blockpool rados namespace object %q", namespacedName)

	if err := cephclient.DeleteRadosNamespace(r.context, r.clusterInfo, cephBlockPoolRadosNamespace.Spec.BlockPoolName, getRadosNamespaceName(cephBlockPoolRadosNamespace)); err != nil {
		return errors.Wrapf(err, "failed to delete ceph blockpool rados namespace %q", cephBlockPoolRadosNamespace.Name)
	}

	logger.Infof("deleted ceph blockpool rados namespace %q", namespacedName)
	return nil
}

// updateStatus updates an object with a given status
func (r *ReconcileCephBlockPoolRadosNamespace) updateStatus(client client.Client, name types.NamespacedName, status cephv1.ConditionType) {
	cephBlockPoolRadosNamespace := &cephv1.CephBlockPoolRadosNamespace{}
	if err := client.Get(r.opManagerContext, name, cephBlockPoolRadosNamespace); err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debugf("CephBlockPoolRadosNamespace resource %q not found. Ignoring since object must be deleted.", name)
			return
		}
		logger.Warningf("failed to retrieve ceph blockpool rados namespace %q to update status to %q. %v", name, status, err)
		return
	}
	if cephBlockPoolRadosNamespace.Status == nil {
		cephBlockPoolRadosNamespace.Status = &cephv1.CephBlockPoolRadosNamespaceStatus{}
	}

	cephBlockPoolRadosNamespace.Status.Phase = status
	cephBlockPoolRadosNamespace.Status.Info = map[string]string{"clusterID": buildClusterID(cephBlockPoolRadosNamespace)}
	if err := reporting.UpdateStatus(client, cephBlockPoolRadosNamespace); err != nil {
		logger.Errorf("failed to set ceph blockpool rados namespace %q status to %q. %v", name, status, err)
		return
	}
	logger.Debugf("ceph blockpool rados namespace %q status updated to %q", name, status)
}

func buildClusterID(cephBlockPoolRadosNamespace *cephv1.CephBlockPoolRadosNamespace) string {
	clusterID := fmt.Sprintf("%s-%s-block-%s", cephBlockPoolRadosNamespace.Namespace, cephBlockPoolRadosNamespace.Spec.BlockPoolName, getRadosNamespaceName(cephBlockPoolRadosNamespace))
	return k8sutil.Hash(clusterID)
}

func (r *ReconcileCephBlockPoolRadosNamespace) cleanup(radosNamespace *cephv1.CephBlockPoolRadosNamespace, cephCluster *cephv1.CephCluster) error {
	logger.Infof("starting cleanup of the ceph resources for radosNamespace %q in namespace %q", radosNamespace.Name, radosNamespace.Namespace)
	cleanupConfig := map[string]string{
		opcontroller.CephBlockPoolNameEnv:           radosNamespace.Spec.BlockPoolName,
		opcontroller.CephBlockPoolRadosNamespaceEnv: getRadosNamespaceName(radosNamespace),
	}
	cleanup := opcontroller.NewResourceCleanup(radosNamespace, cephCluster, r.opConfig.Image, cleanupConfig)
	jobName := k8sutil.TruncateNodeNameForJob("cleanup-radosnamespace-%s", fmt.Sprintf("%s-%s", radosNamespace.Spec.BlockPoolName, radosNamespace.Name))
	err := cleanup.StartJob(r.clusterInfo.Context, r.context.Clientset, jobName)
	if err != nil {
		return errors.Wrapf(err, "failed to run clean up job to clean the ceph resources in radosNamespace %q", radosNamespace.Name)
	}
	return nil
}

func checkBlockPoolMirroring(cephBlockPool *cephv1.CephBlockPool) bool {
	return !(cephBlockPool.Spec.Mirroring.Enabled)
}

func (r *ReconcileCephBlockPoolRadosNamespace) reconcileMirroring(cephBlockPoolRadosNamespace *cephv1.CephBlockPoolRadosNamespace, cephBlockPool *cephv1.CephBlockPool) error {
	poolAndRadosNamespaceName := fmt.Sprintf("%s/%s", cephBlockPool.Name, getRadosNamespaceName(cephBlockPoolRadosNamespace))
	mirrorInfo, err := cephclient.GetPoolMirroringInfo(r.context, r.clusterInfo, poolAndRadosNamespaceName)
	if err != nil {
		return errors.Wrapf(err, "failed to get mirroring info for the radosnamespace %q", poolAndRadosNamespaceName)
	}

	// Initialize the channel for radosNamespace
	// This allows us to track multiple radosNamespace in the same namespace
	radosNamespaceChannelKey := radosNamespaceChannelKeyName(cephBlockPool.Namespace, poolAndRadosNamespaceName)
	_, radosNamespaceContextsExists := r.radosNamespaceContexts[radosNamespaceChannelKey]
	if !radosNamespaceContextsExists {
		internalCtx, internalCancel := context.WithCancel(r.opManagerContext)
		r.radosNamespaceContexts[radosNamespaceChannelKey] = &mirrorHealth{
			internalCtx:    internalCtx,
			internalCancel: internalCancel,
		}
	}
	monitoringSpec := cephv1.NamedPoolSpec{
		Name:     poolAndRadosNamespaceName, // use the name of the blockpool/radosNamespace
		PoolSpec: cephBlockPool.Spec.PoolSpec,
	}
	checker := cephclient.NewMirrorChecker(r.context, r.client, r.clusterInfo, types.NamespacedName{Name: cephBlockPoolRadosNamespace.Name, Namespace: cephBlockPoolRadosNamespace.Namespace}, &monitoringSpec, cephBlockPoolRadosNamespace)

	if cephBlockPoolRadosNamespace.Spec.Mirroring != nil {
		mirroringDisabled := checkBlockPoolMirroring(cephBlockPool)
		if mirroringDisabled {
			return errors.Errorf("mirroring is disabled for block pool %q, cannot enable mirroring for radosnamespace %q", cephBlockPool.Name, poolAndRadosNamespaceName)
		}

		err = cephclient.EnableRBDRadosNamespaceMirroring(r.context, r.clusterInfo, poolAndRadosNamespaceName, cephBlockPoolRadosNamespace.Spec.Mirroring.RemoteNamespace, string(cephBlockPoolRadosNamespace.Spec.Mirroring.Mode))
		if err != nil {
			return errors.Wrap(err, "failed to enable rbd rados namespace mirroring")
		}

		// Schedule snapshots
		err = cephclient.EnableSnapshotSchedules(r.context, r.clusterInfo, poolAndRadosNamespaceName, cephBlockPoolRadosNamespace.Spec.Mirroring.SnapshotSchedules)
		if err != nil {
			return errors.Wrapf(err, "failed to enable snapshot scheduling for rbd rados namespace %q", poolAndRadosNamespaceName)
		}

		// Run the goroutine to update the mirroring status
		// use the monitoring settings from the cephBlockPool CR
		if !cephBlockPool.Spec.StatusCheck.Mirror.Disabled {
			logger.Debugf("starting mirror monitoring for radosnamespace %q", poolAndRadosNamespaceName)
			// Start monitoring of the radosNamespace
			if r.radosNamespaceContexts[radosNamespaceChannelKey].started {
				logger.Debug("radosnamespace monitoring go routine already running!")
			} else {
				r.radosNamespaceContexts[radosNamespaceChannelKey].started = true
				go checker.CheckMirroring(r.radosNamespaceContexts[radosNamespaceChannelKey].internalCtx)
			}
		}
	}

	if cephBlockPoolRadosNamespace.Spec.Mirroring == nil && mirrorInfo.Mode != "disabled" {
		if mirrorInfo.Mode == "image" {
			mirroredPools, err := cephclient.GetMirroredPoolImages(r.context, r.clusterInfo, poolAndRadosNamespaceName)
			if err != nil {
				return errors.Wrapf(err, "failed to list mirrored images for radosnamespace %q", poolAndRadosNamespaceName)
			}

			if len(*mirroredPools.Images) > 0 {
				return errors.Errorf("there are images in the radosnamespace %q. Please manually disable mirroring for each image", poolAndRadosNamespaceName)
			}
		}

		err = cephclient.DisableRBDRadosNamespaceMirroring(r.context, r.clusterInfo, poolAndRadosNamespaceName)
		if err != nil {
			return errors.Wrap(err, "failed to disable rbd rados namespace mirroring")
		}
	}

	if cephBlockPool.Spec.StatusCheck.Mirror.Disabled {
		// Stop monitoring the mirroring status of this radosNamespace
		if radosNamespaceContextsExists && r.radosNamespaceContexts[radosNamespaceChannelKey].started {
			r.cancelMirrorMonitoring(radosNamespaceChannelKey)
			// Reset the MirrorHealthCheckSpec
			checker.UpdateStatusMirroring(nil, nil, nil, "")
		}
	}

	return nil
}

func radosNamespaceChannelKeyName(poolAndRadosNamespaceName, namespace string) string {
	return types.NamespacedName{Namespace: namespace, Name: poolAndRadosNamespaceName}.String()
}

// cancel mirror monitoring. This is a noop if monitoring is not running.
func (r *ReconcileCephBlockPoolRadosNamespace) cancelMirrorMonitoring(channelKey string) {
	_, poolContextExists := r.radosNamespaceContexts[channelKey]
	if poolContextExists {
		// Cancel the context to stop the go routine
		r.radosNamespaceContexts[channelKey].internalCancel()

		// Remove ceph radosNamespace from the map
		delete(r.radosNamespaceContexts, channelKey)
	}
}
