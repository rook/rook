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

// Package cluster to manage a Ceph cluster.
package cluster

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/daemon/ceph/osd/kms"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/csi"
	"github.com/rook/rook/pkg/operator/ceph/reporting"
	"github.com/rook/rook/pkg/operator/k8sutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apituntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	controllerName = "ceph-cluster-controller"
)

const (
	// DefaultClusterName states the default name of the rook-cluster if not provided.
	DefaultClusterName = "rook-ceph"
	disableHotplugEnv  = "ROOK_DISABLE_DEVICE_HOTPLUG"
)

var (
	logger = capnslog.NewPackageLogger("github.com/rook/rook", controllerName)
	// disallowedHostDirectories directories which are not allowed to be used
	disallowedHostDirectories = []string{"/etc/ceph", "/rook", "/var/log/ceph"}
)

// List of object resources to watch by the controller
var objectsToWatch = []client.Object{
	&appsv1.Deployment{TypeMeta: metav1.TypeMeta{Kind: "Deployment", APIVersion: appsv1.SchemeGroupVersion.String()}},
	&corev1.Service{TypeMeta: metav1.TypeMeta{Kind: "Service", APIVersion: corev1.SchemeGroupVersion.String()}},
	&corev1.Secret{TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: corev1.SchemeGroupVersion.String()}},
	&corev1.ConfigMap{TypeMeta: metav1.TypeMeta{Kind: "ConfigMap", APIVersion: corev1.SchemeGroupVersion.String()}},
}

// ControllerTypeMeta Sets the type meta for the controller main object
var ControllerTypeMeta = metav1.TypeMeta{
	Kind:       opcontroller.ClusterResource.Kind,
	APIVersion: opcontroller.ClusterResource.APIVersion,
}

// ClusterController controls an instance of a Rook cluster
type ClusterController struct {
	context        *clusterd.Context
	rookImage      string
	clusterMap     map[string]*cluster
	csiConfigMutex *sync.Mutex
	osdChecker     *osd.OSDHealthMonitor
	client         client.Client
	namespacedName types.NamespacedName
	recorder       *k8sutil.EventReporter
	OpManagerCtx   context.Context
}

// ReconcileCephCluster reconciles a CephFilesystem object
type ReconcileCephCluster struct {
	client            client.Client
	scheme            *apituntime.Scheme
	context           *clusterd.Context
	clusterController *ClusterController
	opManagerContext  context.Context
}

// Add creates a new CephCluster Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, ctx *clusterd.Context, clusterController *ClusterController, opManagerContext context.Context) error {
	return add(mgr, newReconciler(mgr, ctx, clusterController, opManagerContext), ctx)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, ctx *clusterd.Context, clusterController *ClusterController, opManagerContext context.Context) reconcile.Reconciler {
	// add "rook-" prefix to the controller name to make sure it is clear to all reading the events
	// that they are coming from Rook. The controller name already has context that it is for Ceph
	// and from the cluster controller.
	clusterController.recorder = k8sutil.NewEventReporter(mgr.GetEventRecorderFor("rook-" + controllerName))

	return &ReconcileCephCluster{
		client:            mgr.GetClient(),
		scheme:            mgr.GetScheme(),
		context:           ctx,
		clusterController: clusterController,
		opManagerContext:  opManagerContext,
	}
}

func add(mgr manager.Manager, r reconcile.Reconciler, context *clusterd.Context) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}
	logger.Info("successfully started")

	// Watch for changes on the CephCluster CR object
	err = c.Watch(
		&source.Kind{
			Type: &cephv1.CephCluster{
				TypeMeta: ControllerTypeMeta,
			},
		},
		&handler.EnqueueRequestForObject{},
		watchControllerPredicate())
	if err != nil {
		return err
	}

	// Watch all other resources of the Ceph Cluster
	for _, t := range objectsToWatch {
		err = c.Watch(
			&source.Kind{
				Type: t,
			},
			&handler.EnqueueRequestForOwner{
				IsController: true,
				OwnerType:    &cephv1.CephCluster{},
			},
			opcontroller.WatchPredicateForNonCRDObject(&cephv1.CephCluster{TypeMeta: ControllerTypeMeta}, mgr.GetScheme()))
		if err != nil {
			return err
		}
	}

	// Build Handler function to return the list of ceph clusters
	// This is used by the watchers below
	handlerFunc, err := opcontroller.ObjectToCRMapper(mgr.GetClient(), &cephv1.CephClusterList{}, mgr.GetScheme())
	if err != nil {
		return err
	}

	// Watch for nodes additions and updates
	err = c.Watch(
		&source.Kind{
			Type: &corev1.Node{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Node",
					APIVersion: corev1.SchemeGroupVersion.String(),
				},
			},
		},
		handler.EnqueueRequestsFromMapFunc(handlerFunc),
		predicateForNodeWatcher(mgr.GetClient(), context))
	if err != nil {
		return err
	}

	// Watch for changes on the hotplug config map
	// TODO: to improve, can we run this against the operator namespace only?
	disableVal := os.Getenv(disableHotplugEnv)
	if disableVal != "true" {
		logger.Info("enabling hotplug orchestration")
		err = c.Watch(
			&source.Kind{
				Type: &corev1.ConfigMap{
					TypeMeta: metav1.TypeMeta{
						Kind:       "ConfigMap",
						APIVersion: corev1.SchemeGroupVersion.String(),
					},
				},
			},
			handler.EnqueueRequestsFromMapFunc(handlerFunc),
			predicateForHotPlugCMWatcher(mgr.GetClient()))
		if err != nil {
			return err
		}
	} else {
		logger.Info("hotplug orchestration disabled")
	}

	return nil
}

// Reconcile reads that state of the cluster for a CephCluster object and makes changes based on the state read
// and what is in the cephCluster.Spec
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileCephCluster) Reconcile(context context.Context, request reconcile.Request) (reconcile.Result, error) {
	// workaround because the rook logging mechanism is not compatible with the controller-runtime logging interface
	reconcileResponse, cephCluster, err := r.reconcile(request)

	return reporting.ReportReconcileResult(logger, r.clusterController.recorder,
		cephCluster, reconcileResponse, err)
}

func (r *ReconcileCephCluster) reconcile(request reconcile.Request) (reconcile.Result, *cephv1.CephCluster, error) {
	// Pass the client context to the ClusterController
	r.clusterController.client = r.client

	// Used by functions not part of the ClusterController struct but are given the context to execute actions
	r.clusterController.context.Client = r.client

	// Pass object name and namespace
	r.clusterController.namespacedName = request.NamespacedName

	// Fetch the cephCluster instance
	cephCluster := &cephv1.CephCluster{}
	err := r.client.Get(r.opManagerContext, request.NamespacedName, cephCluster)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("cephCluster resource not found. Ignoring since object must be deleted.")
			return reconcile.Result{}, cephCluster, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, cephCluster, errors.Wrap(err, "failed to get cephCluster")
	}

	// Set a finalizer so we can do cleanup before the object goes away
	err = opcontroller.AddFinalizerIfNotPresent(r.client, cephCluster)
	if err != nil {
		return reconcile.Result{}, cephCluster, errors.Wrap(err, "failed to add finalizer")
	}

	// DELETE: the CR was deleted
	if !cephCluster.GetDeletionTimestamp().IsZero() {
		return r.reconcileDelete(cephCluster)
	}

	// Do reconcile here!
	ownerInfo := k8sutil.NewOwnerInfo(cephCluster, r.scheme)
	if err := r.clusterController.reconcileCephCluster(cephCluster, ownerInfo); err != nil {
		return reconcile.Result{}, cephCluster, errors.Wrapf(err, "failed to reconcile cluster %q", cephCluster.Name)
	}

	// Return and do not requeue
	return reconcile.Result{}, cephCluster, nil
}

func (r *ReconcileCephCluster) reconcileDelete(cephCluster *cephv1.CephCluster) (reconcile.Result, *cephv1.CephCluster, error) {
	nsName := r.clusterController.namespacedName
	var err error

	// Set the deleting status
	opcontroller.UpdateClusterCondition(r.context, cephCluster, nsName,
		cephv1.ConditionDeleting, corev1.ConditionTrue, cephv1.ClusterDeletingReason, "Deleting the CephCluster",
		true /* keep all other conditions to be safe */)

	deps, err := CephClusterDependents(r.context, cephCluster.Namespace)
	if err != nil {
		return reconcile.Result{}, cephCluster, err
	}
	if !deps.Empty() {
		err := reporting.ReportDeletionBlockedDueToDependents(logger, r.client, cephCluster, deps)
		return opcontroller.WaitForRequeueIfFinalizerBlocked, cephCluster, err
	}
	reporting.ReportDeletionNotBlockedDueToDependents(logger, r.client, r.clusterController.recorder, cephCluster)

	doCleanup := true

	// Start cluster clean up only if cleanupPolicy is applied to the ceph cluster
	internalCtx := context.Context(r.opManagerContext)
	if cephCluster.Spec.CleanupPolicy.HasDataDirCleanPolicy() && !cephCluster.Spec.External.Enable {
		monSecret, clusterFSID, err := r.clusterController.getCleanUpDetails(cephCluster.Namespace)
		if err != nil {
			logger.Warningf("failed to get mon secret. skip cluster cleanup. remove finalizer. %v", err)
			doCleanup = false
		}

		if doCleanup {
			cephHosts, err := r.clusterController.getCephHosts(cephCluster.Namespace)
			if err != nil {
				return reconcile.Result{}, cephCluster, errors.Wrapf(err, "failed to find valid ceph hosts in the cluster %q", cephCluster.Namespace)
			}
			go r.clusterController.startClusterCleanUp(internalCtx, cephCluster, cephHosts, monSecret, clusterFSID)
		}
	}

	if doCleanup {
		// Run delete sequence
		response, err := r.clusterController.requestClusterDelete(cephCluster)
		if err != nil {
			// If the cluster cannot be deleted, requeue the request for deletion to see if the conditions
			// will eventually be satisfied such as the volumes being removed
			return response, cephCluster, errors.Wrapf(err, "failed to clean up CephCluster %q", nsName.String())
		}
	}

	// Remove finalizer
	err = removeFinalizer(r.client, nsName)
	if err != nil {
		return reconcile.Result{}, cephCluster, errors.Wrap(err, "failed to remove finalizer")
	}

	// Return and do not requeue. Successful deletion.
	return reconcile.Result{}, cephCluster, nil
}

// NewClusterController create controller for watching cluster custom resources created
func NewClusterController(context *clusterd.Context, rookImage string) *ClusterController {
	return &ClusterController{
		context:        context,
		rookImage:      rookImage,
		clusterMap:     make(map[string]*cluster),
		csiConfigMutex: &sync.Mutex{},
	}
}

func (c *ClusterController) reconcileCephCluster(clusterObj *cephv1.CephCluster, ownerInfo *k8sutil.OwnerInfo) error {
	if clusterObj.Spec.CleanupPolicy.HasDataDirCleanPolicy() {
		logger.Infof("skipping orchestration for cluster object %q in namespace %q because its cleanup policy is set", clusterObj.Name, clusterObj.Namespace)
		return nil
	}

	cluster, ok := c.clusterMap[clusterObj.Namespace]
	if !ok {
		// It's a new cluster so let's populate the struct
		cluster = newCluster(clusterObj, c.context, c.csiConfigMutex, ownerInfo)
	}
	cluster.namespacedName = c.namespacedName

	// Pass down the client to interact with Kubernetes objects
	// This will be used later down by spec code to create objects like deployment, services etc
	cluster.context.Client = c.client

	// Set the spec
	cluster.Spec = &clusterObj.Spec

	c.clusterMap[cluster.Namespace] = cluster
	logger.Infof("reconciling ceph cluster in namespace %q", cluster.Namespace)

	// Start the main ceph cluster orchestration
	return c.initializeCluster(cluster)
}

func (c *ClusterController) requestClusterDelete(cluster *cephv1.CephCluster) (reconcile.Result, error) {
	nsName := fmt.Sprintf("%s/%s", cluster.Namespace, cluster.Name)

	if existing, ok := c.clusterMap[cluster.Namespace]; ok && existing.namespacedName.Name != cluster.Name {
		logger.Errorf("skipping deletion of CephCluster %q. CephCluster CR %q already exists in this namespace. only one cluster cr per namespace is supported.",
			nsName, existing.namespacedName.Name)
		return reconcile.Result{}, nil // do not requeue the delete
	}

	logger.Infof("cleaning up CephCluster %q", nsName)

	if cluster, ok := c.clusterMap[cluster.Namespace]; ok {
		// We used to stop the bucket controller here but when we get a DELETE event for the CephCluster
		// we will reload the CRD manager anyway so the bucket controller go routine will be stopped
		// since the op manager context is cancelled.
		// close the goroutines watching the health of the cluster (mons, osds, ceph status)
		for _, daemon := range monitorDaemonList {
			if monitoring, ok := cluster.monitoringRoutines[daemon]; ok && monitoring.internalCtx.Err() == nil { // if the context hasn't been cancelled
				cluster.monitoringRoutines[daemon].internalCancel()
			}
		}
	}

	if cluster.Spec.CleanupPolicy.AllowUninstallWithVolumes {
		logger.Info("skipping check for existing PVs as allowUninstallWithVolumes is set to true")
	} else {
		err := c.checkIfVolumesExist(cluster)
		if err != nil {
			return opcontroller.WaitForRequeueIfFinalizerBlocked, errors.Wrapf(err, "failed to check if volumes exist for CephCluster in namespace %q", cluster.Namespace)
		}
	}

	if cluster.Spec.External.Enable {
		purgeExternalCluster(c.context.Clientset, cluster.Namespace)
	} else if cluster.Spec.Storage.IsOnPVCEncrypted() && cluster.Spec.Security.KeyManagementService.IsEnabled() {
		// If the StorageClass retain policy of an encrypted cluster with KMS is Delete we also delete the keys
		// Delete keys from KMS
		err := c.deleteOSDEncryptionKeyFromKMS(cluster)
		if err != nil {
			logger.Errorf("failed to delete osd encryption keys for CephCluster %q from kms; deletion will continue. %v", nsName, err)
		}
	}

	if cluster, ok := c.clusterMap[cluster.Namespace]; ok {
		delete(c.clusterMap, cluster.Namespace)
	}

	return reconcile.Result{}, nil
}

func (c *ClusterController) checkIfVolumesExist(cluster *cephv1.CephCluster) error {
	if csi.CSIEnabled() {
		err := c.csiVolumesAllowForDeletion(cluster)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *ClusterController) csiVolumesAllowForDeletion(cluster *cephv1.CephCluster) error {
	drivers := []string{csi.CephFSDriverName, csi.RBDDriverName}

	logger.Infof("checking any PVC created by drivers %q and %q with clusterID %q", csi.CephFSDriverName, csi.RBDDriverName, cluster.Namespace)
	// check any PV is created in this cluster
	attachmentsExist, err := c.checkPVPresentInCluster(drivers, cluster.Namespace)
	if err != nil {
		return errors.Wrapf(err, "failed to list PersistentVolumes")
	}
	// no PVC created in this cluster
	if !attachmentsExist {
		logger.Infof("no volume attachments for cluster %q", cluster.Namespace)
		return nil
	}

	return errors.Errorf("waiting for csi volume attachments in cluster %q to be cleaned up", cluster.Namespace)
}

func (c *ClusterController) checkPVPresentInCluster(drivers []string, clusterID string) (bool, error) {
	pv, err := c.context.Clientset.CoreV1().PersistentVolumes().List(c.OpManagerCtx, metav1.ListOptions{})
	if err != nil {
		return false, errors.Wrapf(err, "failed to list PV")
	}

	for _, p := range pv.Items {
		if p.Spec.CSI == nil {
			logger.Errorf("Spec.CSI is nil for PV %q", p.Name)
			continue
		}
		if p.Spec.CSI.VolumeAttributes["clusterID"] == clusterID {
			//check PV is created by drivers deployed by rook
			for _, d := range drivers {
				if d == p.Spec.CSI.Driver {
					return true, nil
				}
			}

		}
	}
	return false, nil
}

// removeFinalizer removes a finalizer
func removeFinalizer(client client.Client, name types.NamespacedName) error {
	cephCluster := &cephv1.CephCluster{}
	err := client.Get(context.TODO(), name, cephCluster)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("CephCluster resource not found. Ignoring since object must be deleted.")
			return nil
		}
		return errors.Wrapf(err, "failed to retrieve ceph cluster %q to remove finalizer", name.Name)
	}

	err = opcontroller.RemoveFinalizer(client, cephCluster)
	if err != nil {
		return errors.Wrap(err, "failed to remove finalizer")
	}

	return nil
}

func (c *ClusterController) deleteOSDEncryptionKeyFromKMS(currentCluster *cephv1.CephCluster) error {
	// If the operator was stopped and we enter this code, the map is empty
	if _, ok := c.clusterMap[currentCluster.Namespace]; !ok {
		c.clusterMap[currentCluster.Namespace] = &cluster{ClusterInfo: &cephclient.ClusterInfo{Namespace: currentCluster.Namespace}}
	}

	// Fetch PVCs
	osdPVCs, _, err := osd.GetExistingPVCs(c.context, currentCluster.Namespace)
	if err != nil {
		return errors.Wrap(err, "failed to list osd pvc")
	}

	// Initialize the KMS code
	kmsConfig := kms.NewConfig(c.context, &currentCluster.Spec, c.clusterMap[currentCluster.Namespace].ClusterInfo)

	// If token auth is used by the KMS we set it as an env variable
	if currentCluster.Spec.Security.KeyManagementService.IsTokenAuthEnabled() {
		err := kms.SetTokenToEnvVar(c.context, currentCluster.Spec.Security.KeyManagementService.TokenSecretName, kmsConfig.Provider, currentCluster.Namespace)
		if err != nil {
			return errors.Wrapf(err, "failed to fetch kms token secret %q", currentCluster.Spec.Security.KeyManagementService.TokenSecretName)
		}
	}

	// Delete each PV KEK
	for _, osdPVC := range osdPVCs {
		// Generate and store the encrypted key in whatever KMS is configured
		err = kmsConfig.DeleteSecret(osdPVC.Name)
		if err != nil {
			logger.Errorf("failed to delete secret. %v", err)
			continue
		}
	}

	return nil
}
