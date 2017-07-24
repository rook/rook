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

// Package pool to manage a rook pool.
package pool

import (
	"fmt"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/operator/api"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/kit"
	rookclient "github.com/rook/rook/pkg/rook/client"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

const (
	customResourceName       = "pool"
	customResourceNamePlural = "pools"
	replicatedType           = "replicated"
	erasureCodeType          = "erasure-coded"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-pool")

// PoolController represents a controller object for pool custom resources
type PoolController struct {
	clientset  kubernetes.Interface
	rookClient rookclient.RookRestClient
	scheme     *runtime.Scheme
}

// NewPoolController create controller for watching pool custom resources created
func NewPoolController(clientset kubernetes.Interface) (*PoolController, error) {
	return &PoolController{
		clientset: clientset,
	}, nil

}

// Watch watches for instances of Pool custom resources and acts on them
func (c *PoolController) StartWatch(namespace string, stopCh chan struct{}) error {
	client, scheme, err := kit.NewHTTPClient(k8sutil.CustomResourceGroup, k8sutil.V1Alpha1, schemeBuilder)
	if err != nil {
		return fmt.Errorf("failed to get a k8s client for watching pool resources: %v", err)
	}
	c.scheme = scheme

	rclient, err := api.GetRookClient(namespace, c.clientset)
	if err != nil {
		return fmt.Errorf("Failed to get rook client: %v", err)
	}
	c.rookClient = rclient

	resourceHandlerFuncs := cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAdd,
		UpdateFunc: c.onUpdate,
		DeleteFunc: c.onDelete,
	}
	watcher := kit.NewWatcher(PoolResource, namespace, resourceHandlerFuncs, client)
	go watcher.Watch(&Pool{}, stopCh)
	return nil
}

func (c *PoolController) onAdd(obj interface{}) {
	pool := obj.(*Pool)

	// NEVER modify objects from the store. It's a read-only, local cache.
	// Use scheme.Copy() to make a deep copy of original object.
	copyObj, err := c.scheme.Copy(pool)
	if err != nil {
		fmt.Printf("ERROR creating a deep copy of pool object: %v\n", err)
		return
	}
	poolCopy := copyObj.(*Pool)

	err = poolCopy.create(c.rookClient)
	if err != nil {
		logger.Errorf("failed to create pool %s. %+v", pool.ObjectMeta.Name, err)
	}
}

func (c *PoolController) onUpdate(oldObj, newObj interface{}) {
	//oldPool := oldObj.(*Pool)
	newPool := newObj.(*Pool)

	// if the pool is modified, allow the pool to be created if it wasn't already
	err := newPool.create(c.rookClient)
	if err != nil {
		logger.Errorf("failed to create (modify) pool %s. %+v", newPool.ObjectMeta.Name, err)
	}
}

func (c *PoolController) onDelete(obj interface{}) {
	pool := obj.(*Pool)
	err := pool.delete(c.rookClient)
	if err != nil {
		logger.Errorf("failed to delete pool %s. %+v", pool.ObjectMeta.Name, err)
	}
}

// Create the pool
func (p *Pool) create(rclient rookclient.RookRestClient) error {
	// validate the pool settings
	if err := p.validate(); err != nil {
		return fmt.Errorf("invalid pool %s arguments. %+v", p.Name, err)
	}

	// check if the pool already exists
	exists, err := p.exists(rclient)
	if err == nil && exists {
		logger.Infof("pool %s already exists in namespace %s ", p.Name, p.Namespace)
		return nil
	}

	// create the pool
	pool := model.Pool{Name: p.Name}

	r := p.replication()
	if r != nil {
		logger.Infof("creating pool %s in namespace %s with replicas %d", p.Name, p.Namespace, r.Size)
		pool.ReplicationConfig.Size = r.Size
		pool.Type = model.Replicated
	} else {
		ec := p.erasureCode()
		logger.Infof("creating pool %s in namespace %s. coding chunks = %d, data chunks = %d", p.Name, p.Namespace, ec.CodingChunks, ec.DataChunks)
		pool.ErasureCodedConfig.CodingChunkCount = ec.CodingChunks
		pool.ErasureCodedConfig.DataChunkCount = ec.DataChunks
		pool.Type = model.ErasureCoded
	}

	info, err := rclient.CreatePool(pool)
	if err != nil {
		return fmt.Errorf("failed to create pool %s. %+v", p.Name, err)
	}

	logger.Infof("created pool %s. %s", p.Name, info)
	return nil
}

// Delete the pool
func (p *Pool) delete(rclient rookclient.RookRestClient) error {
	// check if the pool  exists
	exists, err := p.exists(rclient)
	if err == nil && !exists {
		return nil
	}

	logger.Infof("TODO: delete pool %s from namespace %s", p.Name, p.Namespace)
	//return p.client.DeletePool(p.PoolSpec.Name)
	return nil
}

// Check if the pool exists
func (p *Pool) exists(rclient rookclient.RookRestClient) (bool, error) {
	pools, err := rclient.GetPools()
	if err != nil {
		return false, err
	}
	for _, pool := range pools {
		if pool.Name == p.Name {
			return true, nil
		}
	}
	return false, nil
}

// Validate the pool arguments
func (p *Pool) validate() error {
	if p.Name == "" {
		return fmt.Errorf("missing name")
	}
	if p.Namespace == "" {
		return fmt.Errorf("missing namespace")
	}
	if p.replication() != nil && p.erasureCode() != nil {
		return fmt.Errorf("both replication and erasure code settings cannot be specified")
	}
	if p.replication() == nil && p.erasureCode() == nil {
		return fmt.Errorf("neither replication nor erasure code settings were specified")
	}
	return nil
}

func (p *Pool) replication() *ReplicationSpec {
	if p.Spec.Replication.Size > 0 {
		return &p.Spec.Replication
	}
	return nil
}

func (p *Pool) erasureCode() *ErasureCodeSpec {
	ec := &p.Spec.ErasureCoding
	if ec.CodingChunks > 0 || ec.DataChunks > 0 {
		return ec
	}
	return nil
}
