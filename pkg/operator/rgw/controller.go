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

Some of the code below came from https://github.com/coreos/etcd-operator
which also has the apache 2.0 license.
*/

// Package rgw to manage a rook object store.
package rgw

import (
	"fmt"

	opkit "github.com/rook/operator-kit"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
)

// ObjectStoreController represents a controller object for object store custom resources
type ObjectStoreController struct {
	context     *clusterd.Context
	scheme      *runtime.Scheme
	versionTag  string
	hostNetwork bool
}

// NewObjectStoreController create controller for watching object store custom resources created
func NewObjectStoreController(context *clusterd.Context, versionTag string, hostNetwork bool) *ObjectStoreController {
	return &ObjectStoreController{
		context:     context,
		versionTag:  versionTag,
		hostNetwork: hostNetwork,
	}
}

// StartWatch watches for instances of ObjectStore custom resources and acts on them
func (c *ObjectStoreController) StartWatch(namespace string, stopCh chan struct{}) error {
	client, scheme, err := opkit.NewHTTPClient(k8sutil.CustomResourceGroup, k8sutil.V1Alpha1, schemeBuilder)
	if err != nil {
		return fmt.Errorf("failed to get a k8s client for watching object store resources: %v", err)
	}
	c.scheme = scheme

	resourceHandlerFuncs := cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAdd,
		UpdateFunc: c.onUpdate,
		DeleteFunc: c.onDelete,
	}
	watcher := opkit.NewWatcher(ObjectStoreResource, namespace, resourceHandlerFuncs, client)
	go watcher.Watch(&ObjectStore{}, stopCh)
	return nil
}

func (c *ObjectStoreController) onAdd(obj interface{}) {
	objectStore := obj.(*ObjectStore)

	// NEVER modify objects from the store. It's a read-only, local cache.
	// Use scheme.Copy() to make a deep copy of original object.
	copyObj, err := c.scheme.Copy(objectStore)
	if err != nil {
		fmt.Printf("ERROR creating a deep copy of object store: %v\n", err)
		return
	}
	objectStoreCopy := copyObj.(*ObjectStore)

	err = objectStoreCopy.Create(c.context, c.versionTag, c.hostNetwork)
	if err != nil {
		logger.Errorf("failed to create object store %s. %+v", objectStore.Name, err)
	}
}

func (c *ObjectStoreController) onUpdate(oldObj, newObj interface{}) {
	// if the object store spec is modified, update the object store
	oldStore := oldObj.(*ObjectStore)
	newStore := newObj.(*ObjectStore)
	if !storeChanged(oldStore.Spec, newStore.Spec) {
		logger.Debugf("object store %s did not change", newStore.Name)
		return
	}

	logger.Infof("applying object store %s changes", newStore.Name)
	err := newStore.Update(c.context, c.versionTag, c.hostNetwork)
	if err != nil {
		logger.Errorf("failed to create (modify) object store %s. %+v", newStore.Name, err)
	}
}

func (c *ObjectStoreController) onDelete(obj interface{}) {
	objectStore := obj.(*ObjectStore)
	err := objectStore.Delete(c.context)
	if err != nil {
		logger.Errorf("failed to delete object store %s. %+v", objectStore.Name, err)
	}
}

func storeChanged(oldStore, newStore ObjectStoreSpec) bool {
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
