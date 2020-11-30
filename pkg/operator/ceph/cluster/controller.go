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
	"os"
	"sync"
	"time"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/agent/flexvolume/attachment"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/daemon/ceph/osd/kms"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	"github.com/rook/rook/pkg/operator/ceph/config"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/csi"
	"github.com/rook/rook/pkg/operator/k8sutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
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
	controllerName           = "ceph-cluster-controller"
	enableFlexDriver         = "ROOK_ENABLE_FLEX_DRIVER"
	detectCephVersionTimeout = 15 * time.Minute
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
var objectsToWatch = []runtime.Object{
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
	context                 *clusterd.Context
	volumeAttachment        attachment.Attachment
	rookImage               string
	clusterMap              map[string]*cluster
	operatorConfigCallbacks []func() error
	addClusterCallbacks     []func() error
	csiConfigMutex          *sync.Mutex
	osdChecker              *osd.OSDHealthMonitor
	client                  client.Client
	namespacedName          types.NamespacedName
}

// ReconcileCephCluster reconciles a CephFilesystem object
type ReconcileCephCluster struct {
	client            client.Client
	scheme            *runtime.Scheme
	context           *clusterd.Context
	clusterController *ClusterController
}

// Add creates a new CephCluster Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager, context *clusterd.Context, clusterController *ClusterController) error {
	return add(mgr, newReconciler(mgr, context, clusterController), context)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager, context *clusterd.Context, clusterController *ClusterController) reconcile.Reconciler {
	// Add the cephv1 scheme to the manager scheme so that the controller knows about it
	mgrScheme := mgr.GetScheme()
	if err := cephv1.AddToScheme(mgr.GetScheme()); err != nil {
		panic(err)
	}

	return &ReconcileCephCluster{
		client:            mgr.GetClient(),
		scheme:            mgrScheme,
		context:           context,
		clusterController: clusterController,
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
		watchControllerPredicate(context))
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
	handerFunc, err := opcontroller.ObjectToCRMapper(mgr.GetClient(), &cephv1.CephClusterList{}, mgr.GetScheme())
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
		&handler.EnqueueRequestsFromMapFunc{ToRequests: handerFunc},
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
			&handler.EnqueueRequestsFromMapFunc{ToRequests: handerFunc},
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
func (r *ReconcileCephCluster) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// workaround because the rook logging mechanism is not compatible with the controller-runtime logging interface
	reconcileResponse, err := r.reconcile(request)
	if err != nil {
		logger.Errorf("failed to reconcile. %v", err)
	}

	return reconcileResponse, err
}

func (r *ReconcileCephCluster) reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Pass the client context to the ClusterController
	r.clusterController.client = r.client

	// Used by functions not part of the ClusterController struct but are given the context to execute actions
	r.clusterController.context.Client = r.client

	// Pass object name and namespace
	r.clusterController.namespacedName = request.NamespacedName

	// Fetch the cephCluster instance
	cephCluster := &cephv1.CephCluster{}
	err := r.client.Get(context.TODO(), request.NamespacedName, cephCluster)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("cephCluster resource not found. Ignoring since object must be deleted.")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, errors.Wrap(err, "failed to get cephCluster")
	}

	// Set a finalizer so we can do cleanup before the object goes away
	err = opcontroller.AddFinalizerIfNotPresent(r.client, cephCluster)
	if err != nil {
		return reconcile.Result{}, errors.Wrap(err, "failed to add finalizer")
	}

	// DELETE: the CR was deleted
	if !cephCluster.GetDeletionTimestamp().IsZero() {
		logger.Infof("deleting ceph cluster %q", cephCluster.Name)

		// Start cluster clean up only if cleanupPolicy is applied to the ceph cluster
		stopCleanupCh := make(chan struct{})
		if cephCluster.Spec.CleanupPolicy.HasDataDirCleanPolicy() {
			// Set the deleting status
			updateStatus(r.client, request.NamespacedName, cephv1.ConditionDeleting)

			monSecret, clusterFSID, err := r.clusterController.getCleanUpDetails(cephCluster.Namespace)
			if err != nil {
				return reconcile.Result{}, errors.Wrap(err, "failed to get mon secret, no cleanup")
			}
			cephHosts, err := r.clusterController.getCephHosts(cephCluster.Namespace)
			if err != nil {
				return reconcile.Result{}, errors.Wrapf(err, "failed to find valid ceph hosts in the cluster %q", cephCluster.Namespace)
			}
			go r.clusterController.startClusterCleanUp(stopCleanupCh, cephCluster, cephHosts, monSecret, clusterFSID)
		}

		// Run delete sequence
		response, ok := r.clusterController.requestClusterDelete(cephCluster)
		if !ok {
			// If the cluster cannot be deleted, requeue the request for deletion to see if the conditions
			// will eventually be satisfied such as the volumes being removed
			close(stopCleanupCh)
			return response, nil
		}

		// Remove finalizer
		err = removeFinalizer(r.client, request.NamespacedName)
		if err != nil {
			return reconcile.Result{}, errors.Wrap(err, "failed to remove finalize")
		}

		// Return and do not requeue. Successful deletion.
		return reconcile.Result{}, nil
	}

	// Do reconcile here!
	if err := r.clusterController.onAdd(cephCluster, cephclient.NewOwnerInfo(cephCluster, true, r.scheme)); err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "failed to reconcile cluster %q", cephCluster.Name)
	}

	// Return and do not requeue
	return reconcile.Result{}, nil
}

// NewClusterController create controller for watching cluster custom resources created
func NewClusterController(context *clusterd.Context, rookImage string, volumeAttachment attachment.Attachment, operatorConfigCallbacks []func() error, addClusterCallbacks []func() error) *ClusterController {
	return &ClusterController{
		context:                 context,
		volumeAttachment:        volumeAttachment,
		rookImage:               rookImage,
		clusterMap:              make(map[string]*cluster),
		operatorConfigCallbacks: operatorConfigCallbacks,
		addClusterCallbacks:     addClusterCallbacks,
		csiConfigMutex:          &sync.Mutex{},
	}
}

func (c *ClusterController) onAdd(clusterObj *cephv1.CephCluster, ownerInfo *cephclient.OwnerInfo) error {
	if clusterObj.Spec.CleanupPolicy.HasDataDirCleanPolicy() {
		logger.Infof("skipping orchestration for cluster object %q in namespace %q because its cleanup policy is set", clusterObj.Name, clusterObj.Namespace)
		return nil
	}

	cluster, ok := c.clusterMap[clusterObj.Namespace]
	if !ok {
		// It's a new cluster so let's populate the struct
		cluster = newCluster(clusterObj, c.context, c.csiConfigMutex, ownerInfo)
	}

	// Note that this lock is held through the callback process, as this creates CSI resources, but we must lock in
	// this scope as the clusterMap is authoritative on cluster count and thus involved in the check for CSI resource
	// deletion. If we ever add additional callback functions, we should tighten this lock.
	c.csiConfigMutex.Lock()
	c.clusterMap[cluster.Namespace] = cluster
	logger.Infof("reconciling ceph cluster in namespace %q", cluster.Namespace)

	for _, callback := range c.addClusterCallbacks {
		if err := callback(); err != nil {
			logger.Errorf("%v", err)
		}
	}
	c.csiConfigMutex.Unlock()

	// Start the main ceph cluster orchestration
	return c.initializeCluster(cluster, clusterObj)
}

func (c *ClusterController) requestClusterDelete(cluster *cephv1.CephCluster) (reconcile.Result, bool) {
	config.ConditionExport(c.context, c.namespacedName, cephv1.ConditionDeleting, v1.ConditionTrue, "ClusterDeleting", "Cluster is deleting")

	if existing, ok := c.clusterMap[cluster.Namespace]; ok && existing.crdName != cluster.Name {
		logger.Errorf("skipping deletion of cluster cr %q in namespace %q. cluster CR %q already exists in this namespace. only one cluster cr per namespace is supported.",
			cluster.Name, cluster.Namespace, existing.crdName)
		return reconcile.Result{}, true
	}

	logger.Infof("delete event for cluster %q in namespace %q", cluster.Name, cluster.Namespace)

	if cluster, ok := c.clusterMap[cluster.Namespace]; ok {
		// if not already stopped, stop clientcontroller and bucketController
		if !cluster.closedStopCh {
			close(cluster.stopCh)
			cluster.closedStopCh = true
		}

		// close the goroutines watching the health of the cluster (mons, osds, ceph status)
		for _, daemon := range monitorDaemonList {
			if monitoring, ok := cluster.monitoringChannels[daemon]; ok && monitoring.monitoringRunning {
				close(cluster.monitoringChannels[daemon].stopChan)
				cluster.monitoringChannels[daemon].monitoringRunning = false
			}
		}
	}

	if cluster.Spec.CleanupPolicy.AllowUninstallWithVolumes {
		logger.Info("skipping check for existing PVs as allowUninstallWithVolumes is set to true")
	} else {
		err := c.checkIfVolumesExist(cluster)
		if err != nil {
			config.ConditionExport(c.context, c.namespacedName, cephv1.ConditionDeleting, v1.ConditionTrue, "ClusterDeleting", "Failed to delete cluster")
			logger.Errorf("failed to check if volumes exist. %v", err)
			return opcontroller.WaitForRequeueIfFinalizerBlocked, false
		}
	}

	// Only valid when the cluster is not external
	if cluster.Spec.External.Enable {
		purgeExternalCluster(c.context.Clientset, cluster.Namespace)
		return reconcile.Result{}, true
	}

	// If the StorageClass retain policy of an encrypted cluster with KMS is Delete we also delete the keys
	if cluster.Spec.Storage.IsOnPVCEncrypted() && cluster.Spec.Security.KeyManagementService.IsEnabled() {
		// Delete keys from KMS
		err := c.deleteOSDEncryptionKeyFromKMS(cluster)
		if err != nil {
			logger.Errorf("failed to delete osd encryption keys from kms. %v", err)
			return reconcile.Result{}, true
		}
	}

	if cluster, ok := c.clusterMap[cluster.Namespace]; ok {
		delete(c.clusterMap, cluster.Namespace)
	}

	return reconcile.Result{}, true
}

func (c *ClusterController) checkIfVolumesExist(cluster *cephv1.CephCluster) error {
	if csi.CSIEnabled() {
		err := c.csiVolumesAllowForDeletion(cluster)
		if err != nil {
			return err
		}
	}
	flexDriverEnabled := os.Getenv(enableFlexDriver) != "false"
	if !flexDriverEnabled {
		logger.Debugf("Flex driver disabled, skipping check for volume attachments for cluster %q", cluster.Namespace)
		return nil
	}
	return c.flexVolumesAllowForDeletion(cluster)
}

func (c *ClusterController) flexVolumesAllowForDeletion(cluster *cephv1.CephCluster) error {
	operatorNamespace := os.Getenv(k8sutil.PodNamespaceEnvVar)
	vols, err := c.volumeAttachment.List(operatorNamespace)
	if err != nil {
		return errors.Wrapf(err, "failed to get volume attachments for operator namespace %q", operatorNamespace)
	}

	// find volume attachments in the deleted cluster
	attachmentsExist := false
AttachmentLoop:
	for _, vol := range vols.Items {
		for _, a := range vol.Attachments {
			if a.ClusterName == cluster.Namespace {
				// there is still an outstanding volume attachment in the cluster that is being deleted.
				attachmentsExist = true
				break AttachmentLoop
			}
		}
	}

	if !attachmentsExist {
		logger.Infof("no volume attachments for cluster %q to clean up.", cluster.Namespace)
		return nil
	}

	return errors.Errorf("waiting for volume attachments in cluster %q to be cleaned up.", cluster.Namespace)
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
	ctx := context.TODO()
	pv, err := c.context.Clientset.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
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

// updateStatus updates an object with a given status
func updateStatus(client client.Client, name types.NamespacedName, status cephv1.ConditionType) {
	cephCluster := &cephv1.CephCluster{}
	err := client.Get(context.TODO(), name, cephCluster)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("CephCluster resource not found. Ignoring since object must be deleted.")
			return
		}
		logger.Warningf("failed to retrieve ceph cluster %q to update status to %q. %v", name, status, err)
		return
	}

	cephCluster.Status.Phase = status
	if err := opcontroller.UpdateStatus(client, cephCluster); err != nil {
		logger.Errorf("failed to set ceph cluster %q status to %q. %v", cephCluster.Name, status, err)
		return
	}
	logger.Debugf("ceph cluster %q status updated to %q", name, status)
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
	osdPVCs, err := osd.GetExistingOSDPVCs(c.context, currentCluster.Namespace)
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
