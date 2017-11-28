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
	opkit "github.com/rook/operator-kit"
	ceph "github.com/rook/rook/pkg/ceph/client"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/model"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/apimachinery/pkg/runtime"
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
	context *clusterd.Context
	scheme  *runtime.Scheme
}

// NewPoolController create controller for watching pool custom resources created
func NewPoolController(context *clusterd.Context) *PoolController {
	return &PoolController{
		context: context,
	}
}

// Watch watches for instances of Pool custom resources and acts on them
func (c *PoolController) StartWatch(namespace string, stopCh chan struct{}) error {
	client, scheme, err := opkit.NewHTTPClient(k8sutil.CustomResourceGroup, k8sutil.V1Alpha1, schemeBuilder)
	if err != nil {
		return fmt.Errorf("failed to get a k8s client for watching pool resources: %v", err)
	}
	c.scheme = scheme

	resourceHandlerFuncs := cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAdd,
		UpdateFunc: c.onUpdate,
		DeleteFunc: c.onDelete,
	}

	logger.Infof("start watching pool resources in namespace %s", namespace)
	watcher := opkit.NewWatcher(PoolResource, namespace, resourceHandlerFuncs, client)
	go watcher.Watch(&Pool{}, stopCh)
	return nil
}

func (c *PoolController) onAdd(obj interface{}) {
	pool := obj.(*Pool)

	// NEVER modify objects from the store. It's a read-only, local cache.
	// Use scheme.Copy() to make a deep copy of original object.
	copyObj, err := c.scheme.Copy(pool)
	if err != nil {
		logger.Errorf("failed to create a deep copy of pool object: %v\n", err)
		return
	}
	poolCopy := copyObj.(*Pool)

	err = poolCopy.create(c.context)
	if err != nil {
		logger.Errorf("failed to create pool %s. %+v", pool.ObjectMeta.Name, err)
	}
}

func (c *PoolController) onUpdate(oldObj, newObj interface{}) {
	oldPool := oldObj.(*Pool)
	pool := newObj.(*Pool)

	if oldPool.Name != pool.Name {
		logger.Errorf("failed to update pool %s. name update not allowed", pool.Name)
		return
	}
	if pool.Spec.ErasureCoded.CodingChunks != 0 && pool.Spec.ErasureCoded.DataChunks != 0 {
		logger.Errorf("failed to update pool %s. erasurecoded update not allowed", pool.Name)
		return
	}
	if !poolChanged(oldPool.Spec, pool.Spec) {
		logger.Debugf("pool %s not changed", pool.Name)
		return
	}

	// if the pool is modified, allow the pool to be created if it wasn't already
	logger.Infof("updating pool %s", pool.Name)
	if err := pool.create(c.context); err != nil {
		logger.Errorf("failed to create (modify) pool %s. %+v", pool.ObjectMeta.Name, err)
	}
}

func poolChanged(old, new PoolSpec) bool {
	if old.Replicated.Size != new.Replicated.Size {
		logger.Infof("pool replication changed from %d to %d", old.Replicated.Size, new.Replicated.Size)
		return true
	}
	return false
}

func (c *PoolController) onDelete(obj interface{}) {
	pool := obj.(*Pool)
	if err := pool.delete(c.context); err != nil {
		logger.Errorf("failed to delete pool %s. %+v", pool.ObjectMeta.Name, err)
	}
}

// Create the pool
func (p *Pool) create(context *clusterd.Context) error {
	// validate the pool settings
	if err := p.validate(context); err != nil {
		return fmt.Errorf("invalid pool %s arguments. %+v", p.Name, err)
	}

	// create the pool
	logger.Infof("creating pool %s in namespace %s", p.Name, p.Namespace)
	if err := ceph.CreatePoolWithProfile(context, p.Namespace, *p.Spec.ToModel(p.Name), p.Name); err != nil {
		return fmt.Errorf("failed to create pool %s. %+v", p.Name, err)
	}

	logger.Infof("created pool %s", p.Name)
	return nil
}

// Delete the pool
func (p *Pool) delete(context *clusterd.Context) error {

	if err := ceph.DeletePool(context, p.Namespace, p.Name); err != nil {
		return fmt.Errorf("failed to delete pool '%s'. %+v", p.Name, err)
	}

	return nil
}

// Check if the pool exists
func (p *Pool) exists(context *clusterd.Context) (bool, error) {
	pools, err := ceph.GetPools(context, p.Namespace)
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
func (p *Pool) validate(context *clusterd.Context) error {
	if p.Name == "" {
		return fmt.Errorf("missing name")
	}
	if p.Namespace == "" {
		return fmt.Errorf("missing namespace")
	}
	if err := p.Spec.Validate(context, p.Namespace); err != nil {
		return err
	}
	return nil
}

func (p *PoolSpec) ToModel(name string) *model.Pool {
	pool := &model.Pool{Name: name, FailureDomain: p.FailureDomain}
	r := p.replication()
	if r != nil {
		pool.ReplicatedConfig.Size = r.Size
		pool.Type = model.Replicated
	} else {
		ec := p.erasureCode()
		if ec != nil {
			pool.ErasureCodedConfig.CodingChunkCount = ec.CodingChunks
			pool.ErasureCodedConfig.DataChunkCount = ec.DataChunks
			pool.Type = model.ErasureCoded
		}
	}
	return pool
}

func (p *PoolSpec) replication() *ReplicatedSpec {
	if p.Replicated.Size > 0 {
		return &p.Replicated
	}
	return nil
}

func (p *PoolSpec) erasureCode() *ErasureCodedSpec {
	ec := &p.ErasureCoded
	if ec.CodingChunks > 0 || ec.DataChunks > 0 {
		return ec
	}
	return nil
}

func (p *PoolSpec) Validate(context *clusterd.Context, namespace string) error {
	if p.replication() != nil && p.erasureCode() != nil {
		return fmt.Errorf("both replication and erasure code settings cannot be specified")
	}
	if p.replication() == nil && p.erasureCode() == nil {
		return fmt.Errorf("neither replication nor erasure code settings were specified")
	}

	// validate the failure domain if specified
	if p.FailureDomain != "" {
		crush, err := ceph.GetCrushMap(context, namespace)
		if err != nil {
			return fmt.Errorf("failed to get crush map. %+v", err)
		}
		found := false
		for _, t := range crush.Types {
			if t.Name == p.FailureDomain {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("unrecognized failure domain %s", p.FailureDomain)
		}
	}

	return nil
}

func ModelToSpec(pool model.Pool) PoolSpec {
	ec := pool.ErasureCodedConfig
	return PoolSpec{
		FailureDomain: pool.FailureDomain,
		Replicated:    ReplicatedSpec{Size: pool.ReplicatedConfig.Size},
		ErasureCoded:  ErasureCodedSpec{CodingChunks: ec.CodingChunkCount, DataChunks: ec.DataChunkCount, Algorithm: ec.Algorithm},
	}
}
