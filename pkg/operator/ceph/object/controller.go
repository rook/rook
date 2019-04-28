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

	"github.com/coreos/pkg/capnslog"
	opkit "github.com/rook/operator-kit"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	cephbeta "github.com/rook/rook/pkg/apis/ceph.rook.io/v1beta1"
	"github.com/rook/rook/pkg/clusterd"
	daemonconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	cephconfig "github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/pool"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-object")

// ObjectStoreResource represents the object store custom resource
var ObjectStoreResource = opkit.CustomResource{
	Name:    "cephobjectstore",
	Plural:  "cephobjectstores",
	Group:   cephv1.CustomResourceGroup,
	Version: cephv1.Version,
	Scope:   apiextensionsv1beta1.NamespaceScoped,
	Kind:    reflect.TypeOf(cephv1.CephObjectStore{}).Name(),
}

var ObjectStoreResourceRookLegacy = opkit.CustomResource{
	Name:    "objectstore",
	Plural:  "objectstores",
	Group:   cephbeta.CustomResourceGroup,
	Version: cephbeta.Version,
	Scope:   apiextensionsv1beta1.NamespaceScoped,
	Kind:    reflect.TypeOf(cephbeta.ObjectStore{}).Name(),
}

// ObjectStoreController represents a controller object for object store custom resources
type ObjectStoreController struct {
	clusterInfo        *daemonconfig.ClusterInfo
	context            *clusterd.Context
	namespace          string
	rookImage          string
	cephVersion        cephv1.CephVersionSpec
	hostNetwork        bool
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
	cephVersion cephv1.CephVersionSpec,
	hostNetwork bool,
	ownerRef metav1.OwnerReference,
	dataDirHostPath string,
) *ObjectStoreController {
	return &ObjectStoreController{
		clusterInfo:     clusterInfo,
		context:         context,
		namespace:       namespace,
		rookImage:       rookImage,
		cephVersion:     cephVersion,
		hostNetwork:     hostNetwork,
		ownerRef:        ownerRef,
		dataDirHostPath: dataDirHostPath,
	}
}

// StartWatch watches for instances of ObjectStore custom resources and acts on them
func (c *ObjectStoreController) StartWatch(stopCh chan struct{}) error {

	resourceHandlerFuncs := cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAdd,
		UpdateFunc: c.onUpdate,
		DeleteFunc: c.onDelete,
	}

	logger.Infof("start watching object store resources in namespace %s", c.namespace)
	watcher := opkit.NewWatcher(ObjectStoreResource, c.namespace, resourceHandlerFuncs, c.context.RookClientset.CephV1().RESTClient())
	go watcher.Watch(&cephv1.CephObjectStore{}, stopCh)

	// watch for events on all legacy types too
	c.watchLegacyObjectStores(c.namespace, stopCh, resourceHandlerFuncs)

	return nil
}

func (c *ObjectStoreController) onAdd(obj interface{}) {
	objectstore, migrationNeeded, err := getObjectStoreObject(obj)
	if err != nil {
		logger.Errorf("failed to get objectstore object: %+v", err)
		return
	}

	if migrationNeeded {
		if err = c.migrateObjectStoreObject(objectstore, obj); err != nil {
			logger.Errorf("failed to migrate objectstore %s in namespace %s: %+v", objectstore.Name, objectstore.Namespace, err)
		}
		return
	}

	c.acquireOrchestrationLock()
	defer c.releaseOrchestrationLock()

	c.createOrUpdateStore(true, objectstore)
}

func (c *ObjectStoreController) onUpdate(oldObj, newObj interface{}) {
	// if the object store spec is modified, update the object store
	oldStore, _, err := getObjectStoreObject(oldObj)
	if err != nil {
		logger.Errorf("failed to get old objectstore object: %+v", err)
		return
	}
	newStore, migrationNeeded, err := getObjectStoreObject(newObj)
	if err != nil {
		logger.Errorf("failed to get new objectstore object: %+v", err)
		return
	}

	if migrationNeeded {
		if err = c.migrateObjectStoreObject(newStore, newObj); err != nil {
			logger.Errorf("failed to migrate objectstore %s in namespace %s: %+v", newStore.Name, newStore.Namespace, err)
		}
		return
	}

	if !storeChanged(oldStore.Spec, newStore.Spec) {
		logger.Debugf("object store %s did not change", newStore.Name)
		return
	}

	c.acquireOrchestrationLock()
	defer c.releaseOrchestrationLock()

	c.createOrUpdateStore(true, newStore)
}

func (c *ObjectStoreController) createOrUpdateStore(update bool, objectstore *cephv1.CephObjectStore) {
	action := "create"
	if update {
		action = "update"
	}

	logger.Infof("%s object store %s", action, objectstore.Name)
	cfg := clusterConfig{
		clusterInfo: c.clusterInfo,
		context:     c.context,
		store:       *objectstore,
		rookVersion: c.rookImage,
		cephVersion: c.cephVersion,
		hostNetwork: c.hostNetwork,
		ownerRefs:   c.storeOwners(objectstore),
		DataPathMap: cephconfig.NewStatelessDaemonDataPathMap(cephconfig.RgwType, objectstore.Name, c.clusterInfo.Name, c.dataDirHostPath),
	}
	if err := cfg.createOrUpdate(update); err != nil {
		logger.Errorf("failed to %s object store %s. %+v", action, objectstore.Name, err)
	}
}

func (c *ObjectStoreController) onDelete(obj interface{}) {
	objectstore, migrationNeeded, err := getObjectStoreObject(obj)
	if err != nil {
		logger.Errorf("failed to get objectstore object: %+v", err)
		return
	}

	if migrationNeeded {
		logger.Infof("ignoring deletion of legacy objectstore %s in namespace %s", objectstore.Name, objectstore.Namespace)
		return
	}

	c.acquireOrchestrationLock()
	defer c.releaseOrchestrationLock()

	cfg := clusterConfig{context: c.context, store: *objectstore}
	if err = cfg.deleteStore(); err != nil {
		logger.Errorf("failed to delete object store %s. %+v", objectstore.Name, err)
	}
}

func (c *ObjectStoreController) ParentClusterChanged(cluster cephv1.ClusterSpec, clusterInfo *daemonconfig.ClusterInfo) {
	c.clusterInfo = clusterInfo
	if cluster.CephVersion.Image == c.cephVersion.Image {
		logger.Debugf("No need to update the object store after the parent cluster changed")
		return
	}

	c.acquireOrchestrationLock()
	defer c.releaseOrchestrationLock()

	c.cephVersion = cluster.CephVersion
	objectStores, err := c.context.RookClientset.CephV1().CephObjectStores(c.namespace).List(metav1.ListOptions{})
	if err != nil {
		logger.Errorf("failed to retrieve object stores to update the ceph version. %+v", err)
		return
	}
	for _, store := range objectStores.Items {
		logger.Infof("updating the ceph version for object store %s to %s", store.Name, c.cephVersion.Image)
		c.createOrUpdateStore(true, &store)
		if err != nil {
			logger.Errorf("failed to update object store %s. %+v", store.Name, err)
		} else {
			logger.Infof("updated object store %s to ceph version %s", store.Name, c.cephVersion.Image)
		}
	}
}

func (c *ObjectStoreController) storeOwners(store *cephv1.CephObjectStore) []metav1.OwnerReference {
	// Only set the cluster crd as the owner of the object store resources.
	// If the object store crd is deleted, the operator will explicitly remove the object store resources.
	// If the object store crd still exists when the cluster crd is deleted, this will make sure the object store
	// resources are cleaned up.
	return []metav1.OwnerReference{c.ownerRef}
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

func (c *ObjectStoreController) watchLegacyObjectStores(namespace string, stopCh chan struct{}, resourceHandlerFuncs cache.ResourceEventHandlerFuncs) {
	// watch for objectstore.rook.io/v1alpha1 events if the CRD exists
	if _, err := c.context.RookClientset.CephV1beta1().ObjectStores(namespace).List(metav1.ListOptions{}); err != nil {
		logger.Infof("skipping watching for legacy rook objectstore events (legacy objectstore CRD probably doesn't exist): %+v", err)
	} else {
		logger.Infof("start watching legacy rook objectstores in all namespaces")
		watcherLegacy := opkit.NewWatcher(ObjectStoreResourceRookLegacy, namespace, resourceHandlerFuncs, c.context.RookClientset.CephV1beta1().RESTClient())
		go watcherLegacy.Watch(&cephbeta.ObjectStore{}, stopCh)
	}
}

func getObjectStoreObject(obj interface{}) (objectstore *cephv1.CephObjectStore, migrationNeeded bool, err error) {
	var ok bool
	objectstore, ok = obj.(*cephv1.CephObjectStore)
	if ok {
		// the objectstore object is of the latest type, simply return it
		return objectstore.DeepCopy(), false, nil
	}

	// type assertion to current objectstore type failed, try instead asserting to the legacy objectstore types
	// then convert it to the current objectstore type
	objectStoreRookLegacy, ok := obj.(*cephbeta.ObjectStore)
	if ok {
		return convertRookLegacyObjectStore(objectStoreRookLegacy.DeepCopy()), true, nil
	}

	return nil, false, fmt.Errorf("not a known objectstore object: %+v", obj)
}

func (c *ObjectStoreController) migrateObjectStoreObject(objectstoreToMigrate *cephv1.CephObjectStore, legacyObj interface{}) error {
	logger.Infof("migrating legacy objectstore %s in namespace %s", objectstoreToMigrate.Name, objectstoreToMigrate.Namespace)

	_, err := c.context.RookClientset.CephV1().CephObjectStores(objectstoreToMigrate.Namespace).Get(objectstoreToMigrate.Name, metav1.GetOptions{})
	if err == nil {
		// objectstore of current type with same name/namespace already exists, don't overwrite it
		logger.Warningf("objectstore object %s in namespace %s already exists, will not overwrite with migrated legacy objectstore.",
			objectstoreToMigrate.Name, objectstoreToMigrate.Namespace)
	} else {
		if !errors.IsNotFound(err) {
			return err
		}

		// objectstore of current type does not already exist, create it now to complete the migration
		_, err = c.context.RookClientset.CephV1().CephObjectStores(objectstoreToMigrate.Namespace).Create(objectstoreToMigrate)
		if err != nil {
			return err
		}

		logger.Infof("completed migration of legacy objectstore %s in namespace %s", objectstoreToMigrate.Name, objectstoreToMigrate.Namespace)
	}

	// delete the legacy objectstore instance, it should not be used anymore now that a migrated instance of the current type exists
	deletePropagation := metav1.DeletePropagationOrphan
	if _, ok := legacyObj.(*cephbeta.ObjectStore); ok {
		logger.Infof("deleting legacy rook objectstore %s in namespace %s", objectstoreToMigrate.Name, objectstoreToMigrate.Namespace)
		return c.context.RookClientset.CephV1beta1().ObjectStores(objectstoreToMigrate.Namespace).Delete(
			objectstoreToMigrate.Name, &metav1.DeleteOptions{PropagationPolicy: &deletePropagation})
	}

	return fmt.Errorf("not a known objectstore object: %+v", legacyObj)
}

func convertRookLegacyObjectStore(legacyObjectStore *cephbeta.ObjectStore) *cephv1.CephObjectStore {
	if legacyObjectStore == nil {
		return nil
	}

	legacySpec := legacyObjectStore.Spec

	objectStore := &cephv1.CephObjectStore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      legacyObjectStore.Name,
			Namespace: legacyObjectStore.Namespace,
		},
		Spec: cephv1.ObjectStoreSpec{
			MetadataPool: pool.ConvertRookLegacyPoolSpec(legacySpec.MetadataPool),
			DataPool:     pool.ConvertRookLegacyPoolSpec(legacySpec.DataPool),
			Gateway: cephv1.GatewaySpec{
				Port:              legacySpec.Gateway.Port,
				SecurePort:        legacySpec.Gateway.SecurePort,
				Instances:         legacySpec.Gateway.Instances,
				AllNodes:          legacySpec.Gateway.AllNodes,
				SSLCertificateRef: legacySpec.Gateway.SSLCertificateRef,
				Placement:         legacySpec.Gateway.Placement,
				Resources:         legacySpec.Gateway.Resources,
			},
		},
	}

	return objectStore
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
