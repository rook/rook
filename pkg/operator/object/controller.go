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

// Package rgw to manage a rook object store.
package object

import (
	"reflect"

	"github.com/coreos/pkg/capnslog"
	opkit "github.com/rook/operator-kit"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha1"
	"github.com/rook/rook/pkg/clusterd"
	rgw "github.com/rook/rook/pkg/operator/object/ceph"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-object")

// ObjectStoreResource represents the object store custom resource
var ObjectStoreResource = opkit.CustomResource{
	Name:    "objectstore",
	Plural:  "objectstores",
	Group:   rookalpha.CustomResourceGroup,
	Version: rookalpha.Version,
	Scope:   apiextensionsv1beta1.NamespaceScoped,
	Kind:    reflect.TypeOf(rookalpha.ObjectStore{}).Name(),
}

// ObjectStoreController represents a controller object for object store custom resources
type ObjectStoreController struct {
	context     *clusterd.Context
	rookImage   string
	hostNetwork bool
	ownerRef    metav1.OwnerReference
}

// NewObjectStoreController create controller for watching object store custom resources created
func NewObjectStoreController(context *clusterd.Context, rookImage string, hostNetwork bool, ownerRef metav1.OwnerReference) *ObjectStoreController {
	return &ObjectStoreController{
		context:     context,
		rookImage:   rookImage,
		hostNetwork: hostNetwork,
		ownerRef:    ownerRef,
	}
}

// StartWatch watches for instances of ObjectStore custom resources and acts on them
func (c *ObjectStoreController) StartWatch(namespace string, stopCh chan struct{}) error {

	resourceHandlerFuncs := cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAdd,
		UpdateFunc: c.onUpdate,
		DeleteFunc: c.onDelete,
	}

	logger.Infof("start watching object store resources in namespace %s", namespace)
	watcher := opkit.NewWatcher(ObjectStoreResource, namespace, resourceHandlerFuncs, c.context.RookClientset.Rook().RESTClient())
	go watcher.Watch(&rookalpha.ObjectStore{}, stopCh)
	return nil
}

func (c *ObjectStoreController) onAdd(obj interface{}) {
	store := obj.(*rookalpha.ObjectStore).DeepCopy()

	err := rgw.CreateStore(c.context, *store, c.rookImage, c.hostNetwork, c.storeOwners(store))
	if err != nil {
		logger.Errorf("failed to create object store %s. %+v", store.Name, err)
	}
}

func (c *ObjectStoreController) onUpdate(oldObj, newObj interface{}) {
	// if the object store spec is modified, update the object store
	oldStore := oldObj.(*rookalpha.ObjectStore).DeepCopy()
	newStore := newObj.(*rookalpha.ObjectStore).DeepCopy()
	if !storeChanged(oldStore.Spec, newStore.Spec) {
		logger.Debugf("object store %s did not change", newStore.Name)
		return
	}

	logger.Infof("applying object store %s changes", newStore.Name)
	err := rgw.UpdateStore(c.context, *newStore, c.rookImage, c.hostNetwork, c.storeOwners(newStore))
	if err != nil {
		logger.Errorf("failed to create (modify) object store %s. %+v", newStore.Name, err)
	}
}

func (c *ObjectStoreController) onDelete(obj interface{}) {
	store := obj.(*rookalpha.ObjectStore).DeepCopy()
	err := rgw.DeleteStore(c.context, *store)
	if err != nil {
		logger.Errorf("failed to delete object store %s. %+v", store.Name, err)
	}
}

func (c *ObjectStoreController) storeOwners(store *rookalpha.ObjectStore) []metav1.OwnerReference {
	// Only set the cluster crd as the owner of the object store resources.
	// If the object store crd is deleted, the operator will explicitly remove the object store resources.
	// If the object store crd still exists when the cluster crd is deleted, this will make sure the object store
	// resources are cleaned up.
	return []metav1.OwnerReference{c.ownerRef}
}

func storeChanged(oldStore, newStore rookalpha.ObjectStoreSpec) bool {
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
