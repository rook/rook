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

// Package mds to manage a rook file system.
package mds

import (
	"fmt"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/kit"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
)

// FilesystemController represents a controller for file system custom resources
type FilesystemController struct {
	context     *clusterd.Context
	scheme      *runtime.Scheme
	versionTag  string
	hostNetwork bool
}

// NewFilesystemController create controller for watching file system custom resources created
func NewFilesystemController(context *clusterd.Context, versionTag string, hostNetwork bool) *FilesystemController {
	return &FilesystemController{
		context:     context,
		versionTag:  versionTag,
		hostNetwork: hostNetwork,
	}
}

// StartWatch watches for instances of Filesystem custom resources and acts on them
func (c *FilesystemController) StartWatch(namespace string, stopCh chan struct{}) error {
	client, scheme, err := kit.NewHTTPClient(k8sutil.CustomResourceGroup, k8sutil.V1Alpha1, schemeBuilder)
	if err != nil {
		return fmt.Errorf("failed to get a k8s client for watching file system resources: %v", err)
	}
	c.scheme = scheme

	resourceHandlerFuncs := cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAdd,
		UpdateFunc: c.onUpdate,
		DeleteFunc: c.onDelete,
	}
	watcher := kit.NewWatcher(FilesystemResource, namespace, resourceHandlerFuncs, client)
	go watcher.Watch(&Filesystem{}, stopCh)
	return nil
}

func (c *FilesystemController) onAdd(obj interface{}) {
	filesystem := obj.(*Filesystem)

	// NEVER modify objects from the store. It's a read-only, local cache.
	// Use scheme.Copy() to make a deep copy of original object.
	copyObj, err := c.scheme.Copy(filesystem)
	if err != nil {
		fmt.Printf("failed to create a deep copy of file system: %v\n", err)
		return
	}
	fsCopy := copyObj.(*Filesystem)

	err = fsCopy.Create(c.context, c.versionTag, c.hostNetwork)
	if err != nil {
		logger.Errorf("failed to create file system %s. %+v", filesystem.Name, err)
	}
}

func (c *FilesystemController) onUpdate(oldObj, newObj interface{}) {
	//oldFilesystem := oldObj.(*Filesystem)
	newFilesystem := newObj.(*Filesystem)

	// if the file system is modified, allow the file system to be created if it wasn't already
	err := newFilesystem.Create(c.context, c.versionTag, c.hostNetwork)
	if err != nil {
		logger.Errorf("failed to create (modify) file system %s. %+v", newFilesystem.Name, err)
	}
}

func (c *FilesystemController) onDelete(obj interface{}) {
	filesystem := obj.(*Filesystem)
	err := filesystem.Delete(c.context)
	if err != nil {
		logger.Errorf("failed to delete file system %s. %+v", filesystem.Name, err)
	}
}
