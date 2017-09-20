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

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/kit"
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
func NewObjectStoreController(context *clusterd.Context, versionTag string, hostNetwork bool) (*ObjectStoreController, error) {
	return &ObjectStoreController{
		context:     context,
		versionTag:  versionTag,
		hostNetwork: hostNetwork,
	}, nil

}

// StartWatch watches for instances of ObjectStore custom resources and acts on them
func (c *ObjectStoreController) StartWatch(namespace string, stopCh chan struct{}) error {
	client, scheme, err := kit.NewHTTPClient(k8sutil.CustomResourceGroup, k8sutil.V1Alpha1, schemeBuilder)
	if err != nil {
		return fmt.Errorf("failed to get a k8s client for watching object store resources: %v", err)
	}
	c.scheme = scheme

	resourceHandlerFuncs := cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAdd,
		UpdateFunc: c.onUpdate,
		DeleteFunc: c.onDelete,
	}
	watcher := kit.NewWatcher(ObjectStoreResource, namespace, resourceHandlerFuncs, client)
	go watcher.Watch(&Objectstore{}, stopCh)
	return nil
}

func (c *ObjectStoreController) onAdd(obj interface{}) {
	objectStore := obj.(*Objectstore)

	// NEVER modify objects from the store. It's a read-only, local cache.
	// Use scheme.Copy() to make a deep copy of original object.
	copyObj, err := c.scheme.Copy(objectStore)
	if err != nil {
		fmt.Printf("ERROR creating a deep copy of object store: %v\n", err)
		return
	}
	objectStoreCopy := copyObj.(*Objectstore)

	err = objectStoreCopy.Create(c.context, c.versionTag, c.hostNetwork)
	if err != nil {
		logger.Errorf("failed to create object store %s. %+v", objectStore.Name, err)
	}
}

func (c *ObjectStoreController) onUpdate(oldObj, newObj interface{}) {
	//oldObjectStore := oldObj.(*ObjectStore)
	newObjectStore := newObj.(*Objectstore)

	// if the object store is modified, allow the object store to be created if it wasn't already
	err := newObjectStore.Update(c.context, c.versionTag, c.hostNetwork)
	if err != nil {
		logger.Errorf("failed to create (modify) object store %s. %+v", newObjectStore.Name, err)
	}
}

func (c *ObjectStoreController) onDelete(obj interface{}) {
	objectStore := obj.(*Objectstore)
	err := objectStore.Delete(c.context)
	if err != nil {
		logger.Errorf("failed to delete object store %s. %+v", objectStore.Name, err)
	}
}
