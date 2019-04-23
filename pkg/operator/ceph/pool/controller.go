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

// Package pool to manage a rook pool.
package pool

import (
	"fmt"
	"reflect"

	"github.com/coreos/pkg/capnslog"
	opkit "github.com/rook/operator-kit"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	cephbeta "github.com/rook/rook/pkg/apis/ceph.rook.io/v1beta1"
	"github.com/rook/rook/pkg/clusterd"
	ceph "github.com/rook/rook/pkg/daemon/ceph/client"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	"github.com/rook/rook/pkg/daemon/ceph/model"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

const (
	replicatedType         = "replicated"
	erasureCodeType        = "erasure-coded"
	poolApplicationNameRBD = "rbd"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "op-pool")

// PoolResource represents the Pool custom resource object
var PoolResource = opkit.CustomResource{
	Name:    "cephblockpool",
	Plural:  "cephblockpools",
	Group:   cephv1.CustomResourceGroup,
	Version: cephv1.Version,
	Scope:   apiextensionsv1beta1.NamespaceScoped,
	Kind:    reflect.TypeOf(cephv1.CephBlockPool{}).Name(),
}

var PoolResourceRookLegacy = opkit.CustomResource{
	Name:    "pool",
	Plural:  "pools",
	Group:   cephbeta.CustomResourceGroup,
	Version: cephbeta.Version,
	Scope:   apiextensionsv1beta1.NamespaceScoped,
	Kind:    reflect.TypeOf(cephbeta.Pool{}).Name(),
}

// PoolController represents a controller object for pool custom resources
type PoolController struct {
	context   *clusterd.Context
	namespace string
}

// NewPoolController create controller for watching pool custom resources created
func NewPoolController(context *clusterd.Context, namespace string) *PoolController {
	return &PoolController{
		context:   context,
		namespace: namespace,
	}
}

// Watch watches for instances of Pool custom resources and acts on them
func (c *PoolController) StartWatch(stopCh chan struct{}) error {

	resourceHandlerFuncs := cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAdd,
		UpdateFunc: c.onUpdate,
		DeleteFunc: c.onDelete,
	}

	logger.Infof("start watching pool resources in namespace %s", c.namespace)
	watcher := opkit.NewWatcher(PoolResource, c.namespace, resourceHandlerFuncs, c.context.RookClientset.CephV1().RESTClient())
	go watcher.Watch(&cephv1.CephBlockPool{}, stopCh)

	// watch for events on all legacy types too
	c.watchLegacyPools(c.namespace, stopCh, resourceHandlerFuncs)

	return nil
}

func (c *PoolController) onAdd(obj interface{}) {
	pool, migrationNeeded, err := getPoolObject(obj)
	if err != nil {
		logger.Errorf("failed to get pool object: %+v", err)
		return
	}

	if migrationNeeded {
		if err = c.migratePoolObject(pool, obj); err != nil {
			logger.Errorf("failed to migrate pool %s in namespace %s: %+v", pool.Name, pool.Namespace, err)
		}
		return
	}

	err = createPool(c.context, pool)
	if err != nil {
		logger.Errorf("failed to create pool %s. %+v", pool.ObjectMeta.Name, err)
	}
}

func (c *PoolController) onUpdate(oldObj, newObj interface{}) {
	oldPool, _, err := getPoolObject(oldObj)
	if err != nil {
		logger.Errorf("failed to get old pool object: %+v", err)
		return
	}
	pool, migrationNeeded, err := getPoolObject(newObj)
	if err != nil {
		logger.Errorf("failed to get new pool object: %+v", err)
		return
	}

	if migrationNeeded {
		if err = c.migratePoolObject(pool, newObj); err != nil {
			logger.Errorf("failed to migrate pool %s in namespace %s: %+v", pool.Name, pool.Namespace, err)
		}
		return
	}

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
	if err := createPool(c.context, pool); err != nil {
		logger.Errorf("failed to create (modify) pool %s. %+v", pool.ObjectMeta.Name, err)
	}
}

func (c *PoolController) ParentClusterChanged(cluster cephv1.ClusterSpec, clusterInfo *cephconfig.ClusterInfo) {
	logger.Debugf("No need to update the pool after the parent cluster changed")
}

func poolChanged(old, new cephv1.PoolSpec) bool {
	if old.Replicated.Size != new.Replicated.Size {
		logger.Infof("pool replication changed from %d to %d", old.Replicated.Size, new.Replicated.Size)
		return true
	}
	return false
}

func (c *PoolController) onDelete(obj interface{}) {
	pool, migrationNeeded, err := getPoolObject(obj)
	if err != nil {
		logger.Errorf("failed to get pool object: %+v", err)
		return
	}

	if migrationNeeded {
		logger.Infof("ignoring deletion of legacy pool %s in namespace %s", pool.Name, pool.Namespace)
		return
	}

	if err := deletePool(c.context, pool); err != nil {
		logger.Errorf("failed to delete pool %s. %+v", pool.ObjectMeta.Name, err)
	}
}

// Create the pool
func createPool(context *clusterd.Context, p *cephv1.CephBlockPool) error {
	// validate the pool settings
	if err := ValidatePool(context, p); err != nil {
		return fmt.Errorf("invalid pool %s arguments. %+v", p.Name, err)
	}

	// create the pool
	logger.Infof("creating pool %s in namespace %s", p.Name, p.Namespace)
	if err := ceph.CreatePoolWithProfile(context, p.Namespace, *p.Spec.ToModel(p.Name), poolApplicationNameRBD); err != nil {
		return fmt.Errorf("failed to create pool %s. %+v", p.Name, err)
	}

	logger.Infof("created pool %s", p.Name)
	return nil
}

// Delete the pool
func deletePool(context *clusterd.Context, p *cephv1.CephBlockPool) error {

	if err := ceph.DeletePool(context, p.Namespace, p.Name); err != nil {
		return fmt.Errorf("failed to delete pool '%s'. %+v", p.Name, err)
	}

	return nil
}

// Check if the pool exists
func poolExists(context *clusterd.Context, p *cephv1.CephBlockPool) (bool, error) {
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

func ModelToSpec(pool model.Pool) cephv1.PoolSpec {
	ec := pool.ErasureCodedConfig
	return cephv1.PoolSpec{
		FailureDomain: pool.FailureDomain,
		CrushRoot:     pool.CrushRoot,
		Replicated:    cephv1.ReplicatedSpec{Size: pool.ReplicatedConfig.Size},
		ErasureCoded:  cephv1.ErasureCodedSpec{CodingChunks: ec.CodingChunkCount, DataChunks: ec.DataChunkCount, Algorithm: ec.Algorithm},
	}
}

// Validate the pool arguments
func ValidatePool(context *clusterd.Context, p *cephv1.CephBlockPool) error {
	if p.Name == "" {
		return fmt.Errorf("missing name")
	}
	if p.Namespace == "" {
		return fmt.Errorf("missing namespace")
	}
	if err := ValidatePoolSpec(context, p.Namespace, &p.Spec); err != nil {
		return err
	}
	return nil
}

func ValidatePoolSpec(context *clusterd.Context, namespace string, p *cephv1.PoolSpec) error {
	if p.Replication() != nil && p.ErasureCode() != nil {
		return fmt.Errorf("both replication and erasure code settings cannot be specified")
	}
	if p.Replication() == nil && p.ErasureCode() == nil {
		return fmt.Errorf("neither replication nor erasure code settings were specified")
	}

	var crush ceph.CrushMap
	var err error
	if p.FailureDomain != "" || p.CrushRoot != "" {
		crush, err = ceph.GetCrushMap(context, namespace)
		if err != nil {
			return fmt.Errorf("failed to get crush map. %+v", err)
		}
	}

	// validate the failure domain if specified
	if p.FailureDomain != "" {
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

	// validate the crush root if specified
	if p.CrushRoot != "" {
		found := false
		for _, t := range crush.Buckets {
			if t.Name == p.CrushRoot {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("unrecognized crush root %s", p.CrushRoot)
		}
	}

	return nil
}

func (c *PoolController) watchLegacyPools(namespace string, stopCh chan struct{}, resourceHandlerFuncs cache.ResourceEventHandlerFuncs) {
	// watch for pool.rook.io/v1alpha1 events if the CRD exists
	if _, err := c.context.RookClientset.CephV1beta1().Pools(namespace).List(metav1.ListOptions{}); err != nil {
		logger.Infof("skipping watching for legacy rook pool events (legacy pool CRD probably doesn't exist): %+v", err)
	} else {
		logger.Infof("start watching legacy rook pools in all namespaces")
		watcherLegacy := opkit.NewWatcher(PoolResourceRookLegacy, namespace, resourceHandlerFuncs, c.context.RookClientset.CephV1beta1().RESTClient())
		go watcherLegacy.Watch(&cephbeta.Pool{}, stopCh)
	}
}

func getPoolObject(obj interface{}) (pool *cephv1.CephBlockPool, migrationNeeded bool, err error) {
	var ok bool
	pool, ok = obj.(*cephv1.CephBlockPool)
	if ok {
		// the pool object is of the latest type, simply return it
		return pool.DeepCopy(), false, nil
	}

	// type assertion to current pool type failed, try instead asserting to the legacy pool types
	// then convert it to the current pool type
	poolRookLegacy, ok := obj.(*cephbeta.Pool)
	if ok {
		return convertRookLegacyPool(poolRookLegacy.DeepCopy()), true, nil
	}

	return nil, false, fmt.Errorf("not a known pool object: %+v", obj)
}

func (c *PoolController) migratePoolObject(poolToMigrate *cephv1.CephBlockPool, legacyObj interface{}) error {
	logger.Infof("migrating legacy pool %s in namespace %s", poolToMigrate.Name, poolToMigrate.Namespace)

	_, err := c.context.RookClientset.CephV1().CephBlockPools(poolToMigrate.Namespace).Get(poolToMigrate.Name, metav1.GetOptions{})
	if err == nil {
		// pool of current type with same name/namespace already exists, don't overwrite it
		logger.Warningf("pool object %s in namespace %s already exists, will not overwrite with migrated legacy pool.",
			poolToMigrate.Name, poolToMigrate.Namespace)
	} else {
		if !errors.IsNotFound(err) {
			return err
		}

		// pool of current type does not already exist, create it now to complete the migration
		_, err = c.context.RookClientset.CephV1().CephBlockPools(poolToMigrate.Namespace).Create(poolToMigrate)
		if err != nil {
			return err
		}

		logger.Infof("completed migration of legacy pool %s in namespace %s", poolToMigrate.Name, poolToMigrate.Namespace)
	}

	// delete the legacy pool instance, it should not be used anymore now that a migrated instance of the current type exists
	deletePropagation := metav1.DeletePropagationOrphan
	if _, ok := legacyObj.(*cephbeta.Pool); ok {
		logger.Infof("deleting legacy rook pool %s in namespace %s", poolToMigrate.Name, poolToMigrate.Namespace)
		return c.context.RookClientset.CephV1beta1().Pools(poolToMigrate.Namespace).Delete(
			poolToMigrate.Name, &metav1.DeleteOptions{PropagationPolicy: &deletePropagation})
	}

	return fmt.Errorf("not a known pool object: %+v", legacyObj)
}

func ConvertRookLegacyPoolSpec(legacySpec cephbeta.PoolSpec) cephv1.PoolSpec {
	return cephv1.PoolSpec{
		FailureDomain: legacySpec.FailureDomain,
		CrushRoot:     legacySpec.CrushRoot,
		Replicated: cephv1.ReplicatedSpec{
			Size: legacySpec.Replicated.Size,
		},
		ErasureCoded: cephv1.ErasureCodedSpec{
			DataChunks:   legacySpec.ErasureCoded.DataChunks,
			CodingChunks: legacySpec.ErasureCoded.CodingChunks,
			Algorithm:    legacySpec.ErasureCoded.Algorithm,
		},
	}
}

func convertRookLegacyPool(legacyPool *cephbeta.Pool) *cephv1.CephBlockPool {
	if legacyPool == nil {
		return nil
	}

	pool := &cephv1.CephBlockPool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      legacyPool.Name,
			Namespace: legacyPool.Namespace,
		},
		Spec: ConvertRookLegacyPoolSpec(legacyPool.Spec),
	}

	return pool
}
