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
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	daemonconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	opkit "github.com/rook/rook/pkg/operator-kit"
	cephconfig "github.com/rook/rook/pkg/operator/ceph/config"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
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

// ObjectStoreController represents a controller object for object store custom resources
type ObjectStoreController struct {
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
func (c *ObjectStoreController) StartWatch(namespace string, stopCh chan struct{}) error {
	resourceHandlerFuncs := cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAdd,
		UpdateFunc: c.onUpdate,
		DeleteFunc: c.onDelete,
	}

	logger.Infof("start watching object store resources in namespace %s", c.namespace)
	watcher := opkit.NewWatcher(ObjectStoreResource, c.namespace, resourceHandlerFuncs, c.context.RookClientset.CephV1().RESTClient())
	go watcher.Watch(&cephv1.CephObjectStore{}, stopCh)
	return nil
}

func (c *ObjectStoreController) onAdd(obj interface{}) {
	objectstore, err := getObjectStoreObject(obj)
	if err != nil {
		logger.Errorf("failed to get objectstore object: %+v", err)
		return
	}

	c.acquireOrchestrationLock()
	defer c.releaseOrchestrationLock()

	c.createOrUpdateStore(objectstore)
}

func (c *ObjectStoreController) onUpdate(oldObj, newObj interface{}) {
	// if the object store spec is modified, update the object store
	oldStore, err := getObjectStoreObject(oldObj)
	if err != nil {
		logger.Errorf("failed to get old objectstore object: %+v", err)
		return
	}
	newStore, err := getObjectStoreObject(newObj)
	if err != nil {
		logger.Errorf("failed to get new objectstore object: %+v", err)
		return
	}

	if !storeChanged(oldStore.Spec, newStore.Spec) {
		logger.Debugf("object store %s did not change", newStore.Name)
		return
	}

	c.acquireOrchestrationLock()
	defer c.releaseOrchestrationLock()

	c.createOrUpdateStore(newStore)
}

func (c *ObjectStoreController) createOrUpdateStore(objectstore *cephv1.CephObjectStore) {
	logger.Infof("creating object store %s", objectstore.Name)
	cfg := clusterConfig{
		clusterInfo: c.clusterInfo,
		context:     c.context,
		store:       *objectstore,
		rookVersion: c.rookImage,
		clusterSpec: c.clusterSpec,
		ownerRefs:   c.storeOwners(objectstore),
		DataPathMap: cephconfig.NewStatelessDaemonDataPathMap(cephconfig.RgwType, objectstore.Name, c.clusterInfo.Name, c.dataDirHostPath),
	}
	if err := cfg.createOrUpdate(); err != nil {
		logger.Errorf("failed to create or update object store %s. %+v", objectstore.Name, err)
	}
}

func (c *ObjectStoreController) onDelete(obj interface{}) {
	objectstore, err := getObjectStoreObject(obj)
	if err != nil {
		logger.Errorf("failed to get objectstore object: %+v", err)
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
	if cluster.CephVersion.Image == c.clusterSpec.CephVersion.Image {
		logger.Debugf("No need to update the object store after the parent cluster changed")
		return
	}

	c.acquireOrchestrationLock()
	defer c.releaseOrchestrationLock()

	c.clusterSpec.CephVersion = cluster.CephVersion
	objectStores, err := c.context.RookClientset.CephV1().CephObjectStores(c.namespace).List(metav1.ListOptions{})
	if err != nil {
		logger.Errorf("failed to retrieve object stores to update the ceph version. %+v", err)
		return
	}
	for _, store := range objectStores.Items {
		logger.Infof("updating the ceph version for object store %s to %s", store.Name, c.clusterSpec.CephVersion.Image)
		c.createOrUpdateStore(&store)
		if err != nil {
			logger.Errorf("failed to update object store %s. %+v", store.Name, err)
		} else {
			logger.Infof("updated object store %s to ceph version %s", store.Name, c.clusterSpec.CephVersion.Image)
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

func getObjectStoreObject(obj interface{}) (objectstore *cephv1.CephObjectStore, err error) {
	var ok bool
	objectstore, ok = obj.(*cephv1.CephObjectStore)
	if ok {
		// the objectstore object is of the latest type, simply return it
		return objectstore.DeepCopy(), nil
	}

	return nil, fmt.Errorf("not a known objectstore object: %+v", obj)
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
