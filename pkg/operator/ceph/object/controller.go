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

package object

import (
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/coreos/pkg/capnslog"
	bktclient "github.com/kube-object-storage/lib-bucket-provisioner/pkg/client/clientset/versioned"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	daemonconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	cephconfig "github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-object")

// ObjectStoreResource represents the object store custom resource
var ObjectStoreResource = k8sutil.CustomResource{
	Name:    "cephobjectstore",
	Plural:  "cephobjectstores",
	Group:   cephv1.CustomResourceGroup,
	Version: cephv1.Version,
	Kind:    reflect.TypeOf(cephv1.CephObjectStore{}).Name(),
}

var (
	finalizerName                  = fmt.Sprintf("%s.%s", ObjectStoreResource.Name, ObjectStoreResource.Group)
	objectstoreDeleteRetryInterval = 2 * time.Second
	objectstoreDeleteMaxRetries    = 15
)

// ObjectStoreController represents a controller object for object store custom resources
type ObjectStoreController struct {
	bktclient          bktclient.Interface
	clusterInfo        *daemonconfig.ClusterInfo
	clusterSpec        *cephv1.ClusterSpec
	context            *clusterd.Context
	namespace          string
	rookImage          string
	ownerRef           metav1.OwnerReference
	dataDirHostPath    string
	orchestrationMutex sync.Mutex
}

// NewObjectStoreController create controller for watching object store custom resources created
func NewObjectStoreController(
	clusterInfo *daemonconfig.ClusterInfo,
	context *clusterd.Context,
	namespace string,
	rookImage string,
	clusterSpec *cephv1.ClusterSpec,
	ownerRef metav1.OwnerReference,
	dataDirHostPath string,
) *ObjectStoreController {
	return &ObjectStoreController{
		clusterInfo:     clusterInfo,
		clusterSpec:     clusterSpec,
		context:         context,
		namespace:       namespace,
		rookImage:       rookImage,
		ownerRef:        ownerRef,
		dataDirHostPath: dataDirHostPath,
	}
}

// StartWatch watches for instances of ObjectStore custom resources and acts on them
func (c *ObjectStoreController) StartWatch(namespace string, stopCh chan struct{}) {
	resourceHandlerFuncs := cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAdd,
		UpdateFunc: c.onUpdate,
		DeleteFunc: c.onDelete,
	}

	logger.Infof("start watching object store resources in namespace %s", c.namespace)
	go k8sutil.WatchCR(ObjectStoreResource, c.namespace, resourceHandlerFuncs, c.context.RookClientset.CephV1().RESTClient(), &cephv1.CephObjectStore{}, stopCh)
}

func (c *ObjectStoreController) onAdd(obj interface{}) {
	if c.clusterSpec.External.Enable && c.clusterSpec.CephVersion.Image == "" {
		logger.Warningf("Creating object store for an external ceph cluster is disabled because no Ceph image is specified")
		return
	}

	objectStore, err := getObjectStoreObject(obj)
	if err != nil {
		logger.Errorf("failed to get objectstore object. %v", err)
		return
	}

	c.acquireOrchestrationLock()
	defer c.releaseOrchestrationLock()
	updateCephObjectStoreStatus(objectStore.GetName(), objectStore.GetNamespace(), k8sutil.ProcessingStatus, c.context)

	if c.clusterSpec.External.Enable {
		_, err := controller.ValidateCephVersionsBetweenLocalAndExternalClusters(c.context, c.namespace, c.clusterInfo.CephVersion)
		if err != nil {
			// This handles the case where the operator is running, the external cluster has been upgraded and a CR creation is called
			// If that's a major version upgrade we fail, if it's a minor version, we continue, it's not ideal but not critical
			logger.Errorf("refusing to run new crd. %v", err)
			updateCephObjectStoreStatus(objectStore.GetName(), objectStore.GetNamespace(), k8sutil.FailedStatus, c.context)
			return
		}
	}
	c.createOrUpdateStore(objectStore)
	updateCephObjectStoreStatus(objectStore.GetName(), objectStore.GetNamespace(), k8sutil.ReadyStatus, c.context)
	err = c.addFinalizer(objectStore.GetNamespace(), objectStore.GetName())
	if err != nil {
		logger.Errorf("failed to add finalizer to objectStore crd. %v", err)
		return
	}
}

func (c *ObjectStoreController) onUpdate(oldObj, newObj interface{}) {
	if c.clusterSpec.External.Enable && c.clusterSpec.CephVersion.Image == "" {
		logger.Warningf("Updating object store for an external ceph cluster is disabled because no Ceph image is specified")
		return
	}

	// if the object store spec is modified, update the object store
	oldStore, err := getObjectStoreObject(oldObj)
	if err != nil {
		logger.Errorf("failed to get old objectstore object. %v", err)
		return
	}
	newStore, err := getObjectStoreObject(newObj)
	if err != nil {
		logger.Errorf("failed to get new objectstore object. %v", err)
		return
	}

	// Check if the objectstore is being deleted. This code path is called when a finalizer is specified in the crd.
	// When an objectstore is requested for deletion, K8s will only set the deletion timestamp if there are any finalizers in the list.
	// K8s will only delete the crd and child resources when the finalizers have been removed from the crd.
	if !newStore.DeletionTimestamp.IsZero() {
		logger.Infof("objectstore %q has a deletion timestamp", newStore.Namespace)
		err := c.handleDelete(newStore)
		if err != nil {
			logger.Errorf("failed finalizer for objectstore. %v", err)
			return
		}
		// remove the finalizer from the crd, which indicates to k8s that the resource can safely be deleted
		c.removeFinalizer(newStore)
		return
	} else {
		if !c.hasFinalizer(newStore) {
			err := c.addFinalizer(newStore.GetNamespace(), newStore.GetName())
			if err != nil {
				logger.Errorf("failed to add finalizer to objectStore crd. %v", err)
				return
			}
		}
	}

	if !storeChanged(oldStore.Spec, newStore.Spec) {
		logger.Debugf("object store %q did not change", newStore.Name)
		return
	}

	c.acquireOrchestrationLock()
	defer c.releaseOrchestrationLock()

	updateCephObjectStoreStatus(newStore.GetName(), newStore.GetNamespace(), k8sutil.ProcessingStatus, c.context)
	c.createOrUpdateStore(newStore)
	updateCephObjectStoreStatus(newStore.GetName(), newStore.GetNamespace(), k8sutil.ReadyStatus, c.context)
}

func (c *ObjectStoreController) createOrUpdateStore(objectstore *cephv1.CephObjectStore) {
	logger.Infof("creating object store %q", objectstore.Name)
	cfg := clusterConfig{
		clusterInfo: c.clusterInfo,
		context:     c.context,
		store:       *objectstore,
		rookVersion: c.rookImage,
		clusterSpec: c.clusterSpec,
		ownerRef:    c.storeOwners(objectstore),
		DataPathMap: cephconfig.NewStatelessDaemonDataPathMap(cephconfig.RgwType, objectstore.Name, c.clusterInfo.Name, c.dataDirHostPath),
	}
	if err := cfg.createOrUpdate(); err != nil {
		logger.Errorf("failed to create or update object store %s. %v", objectstore.Name, err)
	}
}

func (c *ObjectStoreController) onDelete(obj interface{}) {
	if c.clusterSpec.External.Enable && c.clusterSpec.CephVersion.Image == "" {
		logger.Warningf("Deleting object store for an external ceph cluster is disabled because no Ceph image is specified")
		return
	}

	objectstore, err := getObjectStoreObject(obj)
	if err != nil {
		logger.Errorf("failed to get objectstore object. %v", err)
		return
	}

	c.acquireOrchestrationLock()
	defer c.releaseOrchestrationLock()

	err = c.handleDelete(objectstore)
	if err != nil {
		logger.Errorf("failed to delete objectstore. %v", err)
	}

	cfg := clusterConfig{context: c.context, store: *objectstore}
	if err = cfg.deleteStore(); err != nil {
		logger.Errorf("failed to delete object store %q. %v", objectstore.Name, err)
	}
}

func (c *ObjectStoreController) handleDelete(objectstore *cephv1.CephObjectStore) error {
	for retryCount := 0; retryCount < objectstoreDeleteMaxRetries; retryCount++ {

		selector := fmt.Sprintf("metadata.namespace=%s", objectstore.Namespace)
		objectBuckets, err := c.bktclient.ObjectbucketV1alpha1().ObjectBuckets().List(metav1.ListOptions{FieldSelector: selector})
		if err != nil {
			return errors.Wrapf(err, "failed to get buckets for objectstore %q in namespace %q", objectstore.Name, objectstore.Namespace)
		}
		if len(objectBuckets.Items) == 0 {
			logger.Infof("no objectbuckets for objectstore %q to clean up.", objectstore.Namespace)
			break
		}

		retryCount++
		if retryCount == objectstoreDeleteMaxRetries {
			bucketNames := make([]string, 0)
			for _, bucket := range objectBuckets.Items {
				bucketNames = append(bucketNames, bucket.Name)
			}
			logger.Warningf(
				"exceeded retry count while waiting for buckets for objectstore %q to be cleaned up. buckets: %+v",
				objectstore.Namespace,
				bucketNames)
			break
		}

		logger.Infof("waiting for buckets in objectstore %q to be cleaned up. Retrying in %q.",
			objectstore.Namespace, objectstoreDeleteRetryInterval.String())
		<-time.After(objectstoreDeleteRetryInterval)
	}
	return nil
}

// ParentClusterChanged determines wether or not a CR update has been sent
func (c *ObjectStoreController) ParentClusterChanged(cluster cephv1.ClusterSpec, clusterInfo *daemonconfig.ClusterInfo, isUpgrade bool) {
	c.clusterInfo = clusterInfo
	if !isUpgrade {
		logger.Debugf("No need to update the object store after the parent cluster changed")
		return
	}

	logger.Infof("waiting for the orchestration lock to update the object store")
	c.acquireOrchestrationLock()
	defer c.releaseOrchestrationLock()

	c.clusterSpec.CephVersion = cluster.CephVersion
	objectStores, err := c.context.RookClientset.CephV1().CephObjectStores(c.namespace).List(metav1.ListOptions{})
	if err != nil {
		logger.Errorf("failed to retrieve object stores to update the ceph version. %v", err)
		return
	}
	for _, store := range objectStores.Items {
		logger.Infof("updating the ceph version for object store %q to %q", store.Name, c.clusterSpec.CephVersion.Image)
		c.createOrUpdateStore(&store)
		if err != nil {
			logger.Errorf("failed to update object store %q. %v", store.Name, err)
		} else {
			logger.Infof("updated object store %q to ceph version %q", store.Name, c.clusterSpec.CephVersion.Image)
		}
	}
}

func (c *ObjectStoreController) storeOwners(store *cephv1.CephObjectStore) metav1.OwnerReference {
	// Set the object store CR as the owner
	return metav1.OwnerReference{
		APIVersion: fmt.Sprintf("%s/%s", ObjectStoreResource.Group, ObjectStoreResource.Version),
		Kind:       ObjectStoreResource.Kind,
		Name:       store.Name,
		UID:        store.UID,
	}
}

func storeChanged(oldStore, newStore cephv1.ObjectStoreSpec) bool {
	if oldStore.DataPool.Replicated.Size != newStore.DataPool.Replicated.Size {
		logger.Infof("data pool replication changed from %d to %d", oldStore.DataPool.Replicated.Size, newStore.DataPool.Replicated.Size)
		return true
	}
	if oldStore.MetadataPool.Replicated.Size != newStore.MetadataPool.Replicated.Size {
		logger.Infof("metadata pool replication changed from %d to %d", oldStore.MetadataPool.Replicated.Size, newStore.MetadataPool.Replicated.Size)
		return true
	}
	if oldStore.Gateway.Instances != newStore.Gateway.Instances {
		logger.Infof("RGW instances changed from %d to %d", oldStore.Gateway.Instances, newStore.Gateway.Instances)
		return true
	}
	if oldStore.Gateway.Port != newStore.Gateway.Port {
		logger.Infof("Port changed from %d to %d", oldStore.Gateway.Port, newStore.Gateway.Port)
		return true
	}
	if oldStore.Gateway.SecurePort != newStore.Gateway.SecurePort {
		logger.Infof("SecurePort changed from %d to %d", oldStore.Gateway.SecurePort, newStore.Gateway.SecurePort)
		return true
	}
	if oldStore.Gateway.AllNodes != newStore.Gateway.AllNodes {
		logger.Infof("AllNodes changed from %t to %t", oldStore.Gateway.AllNodes, newStore.Gateway.AllNodes)
		return true
	}
	if oldStore.Gateway.SSLCertificateRef != newStore.Gateway.SSLCertificateRef {
		logger.Infof("SSLCertificateRef changed from %s to %s", oldStore.Gateway.SSLCertificateRef, newStore.Gateway.SSLCertificateRef)
		return true
	}
	return false
}

func getObjectStoreObject(obj interface{}) (objectstore *cephv1.CephObjectStore, err error) {
	var ok bool
	objectstore, ok = obj.(*cephv1.CephObjectStore)
	if ok {
		// the objectstore object is of the latest type, simply return it
		return objectstore.DeepCopy(), nil
	}

	return nil, errors.Errorf("not a known objectstore object %+v", obj)
}

func (c *ObjectStoreController) acquireOrchestrationLock() {
	logger.Debugf("Acquiring lock for object store orchestration")
	c.orchestrationMutex.Lock()
	logger.Debugf("Acquired lock for object store orchestration")
}

func (c *ObjectStoreController) releaseOrchestrationLock() {
	c.orchestrationMutex.Unlock()
	logger.Debugf("Released lock for object store orchestration")
}

func updateCephObjectStoreStatus(name, namespace, status string, context *clusterd.Context) {
	updatedCephObjectStore, err := context.RookClientset.CephV1().CephObjectStores(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		logger.Errorf("Unable to update the cephObjectStore %s status %v", updatedCephObjectStore.GetName(), err)
		return
	}
	if updatedCephObjectStore.Status == nil {
		updatedCephObjectStore.Status = &cephv1.Status{}
	} else if updatedCephObjectStore.Status.Phase == status {
		return
	}
	updatedCephObjectStore.Status.Phase = status
	_, err = context.RookClientset.CephV1().CephObjectStores(updatedCephObjectStore.Namespace).Update(updatedCephObjectStore)
	if err != nil {
		logger.Errorf("Unable to update the cephObjectStore %s status %v", updatedCephObjectStore.GetName(), err)
		return
	}
}

// ************************************************************************************************
// Finalizer functions
// ************************************************************************************************
// Returns a bool indicating whether the finalizer (cephobjectstore.ceph.rook.io) is defined on the objectstore CRD
func (c *ObjectStoreController) hasFinalizer(objectstore *cephv1.CephObjectStore) bool {
	for _, finalizer := range objectstore.Finalizers {
		if finalizer == finalizerName {
			return true
		}
	}
	return false
}

func (c *ObjectStoreController) addFinalizer(namespace, name string) error {

	// get the latest objectstore object since we probably updated it before we got to this point (e.g. by updating its status)
	objectstore, err := c.context.RookClientset.CephV1().CephObjectStores(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to find objectstore %q", name)
	}

	if c.hasFinalizer(objectstore) {
		return nil
	}

	// adding finalizer to the objectstore crd
	objectstore.Finalizers = append(objectstore.Finalizers, finalizerName)

	// update the crd
	_, err = c.context.RookClientset.CephV1().CephObjectStores(objectstore.Namespace).Update(objectstore)
	if err != nil {
		return errors.Wrapf(err, "failed to add finalizer to objectstore %q", name)
	}

	logger.Infof("added finalizer to objectstore %q", objectstore.Name)
	return nil
}

func (c *ObjectStoreController) removeFinalizer(obj interface{}) {
	// first determine what type/version of cluster we are dealing with

	os, ok := obj.(*cephv1.CephObjectStore)
	if !ok {
		logger.Warningf("cannot remove finalizer from object that is not an objectstore. %+v", obj)
		return
	}

	// update the crd to remove the finalizer for good. retry several times in case of intermittent failures.
	maxRetries := 5
	retrySeconds := 5 * time.Second
	for i := 0; i < maxRetries; i++ {
		// Get the latest objectstore instead of using the same instance in case it has been changed
		objectstore, err := c.context.RookClientset.CephV1().CephObjectStores(os.Namespace).Get(os.Name, metav1.GetOptions{})
		if err != nil {
			logger.Errorf("failed to remove finalizer. failed to get cluster. %v", err)
			return
		}
		objectMeta := &objectstore.ObjectMeta

		// remove the finalizer from the slice if it exists
		found := false
		for i, finalizer := range objectMeta.Finalizers {
			if finalizer == finalizerName {
				objectMeta.Finalizers = append(objectMeta.Finalizers[:i], objectMeta.Finalizers[i+1:]...)
				found = true
				break
			}
		}
		if !found {
			logger.Infof("finalizer %q not found in the objectstore crd %q", finalizerName, objectMeta.Name)
			return
		}

		_, err = c.context.RookClientset.CephV1().CephObjectStores(objectstore.Namespace).Update(objectstore)
		if err != nil {
			logger.Errorf("failed to remove finalizer %q from objectstore %q. %v", finalizerName, objectMeta.Name, err)
			time.Sleep(retrySeconds)
			continue
		}
		logger.Infof("removed finalizer %q from objectstore %q", finalizerName, objectMeta.Name)
		return
	}

	logger.Warningf("giving up from removing the %q objectstore finalizer", finalizerName)
}
