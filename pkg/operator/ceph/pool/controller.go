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
	"reflect"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	ceph "github.com/rook/rook/pkg/daemon/ceph/client"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	"github.com/rook/rook/pkg/daemon/ceph/model"
	"github.com/rook/rook/pkg/operator/k8sutil"
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
var PoolResource = k8sutil.CustomResource{
	Name:    "cephblockpool",
	Plural:  "cephblockpools",
	Group:   cephv1.CustomResourceGroup,
	Version: cephv1.Version,
	Kind:    reflect.TypeOf(cephv1.CephBlockPool{}).Name(),
}

// PoolController represents a controller object for pool custom resources
type PoolController struct {
	context     *clusterd.Context
	clusterSpec *cephv1.ClusterSpec
}

// NewPoolController create controller for watching pool custom resources created
func NewPoolController(context *clusterd.Context, clusterSpec *cephv1.ClusterSpec) *PoolController {
	return &PoolController{
		context:     context,
		clusterSpec: clusterSpec,
	}
}

// Watch watches for instances of Pool custom resources and acts on them
func (c *PoolController) StartWatch(namespace string, stopCh chan struct{}) error {

	resourceHandlerFuncs := cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAdd,
		UpdateFunc: c.onUpdate,
		DeleteFunc: c.onDelete,
	}

	logger.Infof("start watching pools in namespace %q", namespace)
	go k8sutil.WatchCR(PoolResource, namespace, resourceHandlerFuncs, c.context.RookClientset.CephV1().RESTClient(), &cephv1.CephBlockPool{}, stopCh)
	return nil
}

func (c *PoolController) onAdd(obj interface{}) {
	if c.clusterSpec.External.Enable && c.clusterSpec.CephVersion.Image == "" {
		logger.Warningf("Creating pools for an external ceph cluster is disabled because no Ceph image is specified")
		return
	}

	pool, err := getPoolObject(obj)
	if err != nil {
		logger.Errorf("failed to get pool object. %v", err)
		return
	}
	updateCephBlockPoolStatus(pool.GetName(), pool.GetNamespace(), k8sutil.ProcessingStatus, c.context)
	err = createPool(c.context, pool)
	if err != nil {
		logger.Errorf("failed to create pool %q. %v", pool.ObjectMeta.Name, err)
		updateCephBlockPoolStatus(pool.GetName(), pool.GetNamespace(), k8sutil.FailedStatus, c.context)
		return
	}
	updateCephBlockPoolStatus(pool.GetName(), pool.GetNamespace(), k8sutil.ReadyStatus, c.context)
}

func (c *PoolController) onUpdate(oldObj, newObj interface{}) {
	if c.clusterSpec.External.Enable && c.clusterSpec.CephVersion.Image == "" {
		logger.Warningf("Updating pools for an external ceph cluster is disabled because no Ceph image is specified")
		return
	}

	oldPool, err := getPoolObject(oldObj)
	if err != nil {
		logger.Errorf("failed to get old pool object. %v", err)
		return
	}
	pool, err := getPoolObject(newObj)
	if err != nil {
		logger.Errorf("failed to get new pool object. %v", err)
		return
	}

	if oldPool.Name != pool.Name {
		logger.Errorf("failed to update pool %q. name update not allowed", pool.Name)
		updateCephBlockPoolStatus(pool.GetName(), pool.GetNamespace(), k8sutil.FailedStatus, c.context)
		return
	}
	if pool.Spec.ErasureCoded.CodingChunks != 0 && pool.Spec.ErasureCoded.DataChunks != 0 {
		logger.Errorf("failed to update pool %q. erasurecoded update not allowed", pool.Name)
		updateCephBlockPoolStatus(pool.GetName(), pool.GetNamespace(), k8sutil.FailedStatus, c.context)
		return
	}
	if !poolChanged(oldPool.Spec, pool.Spec) {
		logger.Debugf("pool %q not changed", pool.Name)
		return
	}
	updateCephBlockPoolStatus(pool.GetName(), pool.GetNamespace(), k8sutil.ProcessingStatus, c.context)

	// if the pool is modified, allow the pool to be created if it wasn't already
	logger.Infof("updating pool %q", pool.Name)
	if err := createPool(c.context, pool); err != nil {
		logger.Errorf("failed to create (modify) pool %q. %v", pool.ObjectMeta.Name, err)
		updateCephBlockPoolStatus(pool.GetName(), pool.GetNamespace(), k8sutil.FailedStatus, c.context)
		return
	}
	updateCephBlockPoolStatus(pool.GetName(), pool.GetNamespace(), k8sutil.ReadyStatus, c.context)
}

// ParentClusterChanged determines wether or not a CR update has been sent
func (c *PoolController) ParentClusterChanged(cluster cephv1.ClusterSpec, clusterInfo *cephconfig.ClusterInfo, isUpgrade bool) {
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
	if c.clusterSpec.External.Enable && c.clusterSpec.CephVersion.Image == "" {
		logger.Warningf("Deleting pools for an external ceph cluster is disabled because no Ceph image is specified")
		return
	}

	pool, err := getPoolObject(obj)
	if err != nil {
		logger.Errorf("failed to get pool object. %v", err)
		return
	}
	if err := deletePool(c.context, pool); err != nil {
		logger.Errorf("failed to delete pool %q. %v", pool.ObjectMeta.Name, err)
	}
}

// Create the pool
func createPool(context *clusterd.Context, p *cephv1.CephBlockPool) error {
	// validate the pool settings
	if err := ValidatePool(context, p); err != nil {
		return errors.Wrapf(err, "invalid pool %q arguments", p.Name)
	}

	// create the pool
	logger.Infof("creating pool %q in namespace %q", p.Name, p.Namespace)
	if err := ceph.CreatePoolWithProfile(context, p.Namespace, *p.Spec.ToModel(p.Name), poolApplicationNameRBD); err != nil {
		return errors.Wrapf(err, "failed to create pool %q", p.Name)
	}

	logger.Infof("created pool %q", p.Name)
	return nil
}

// Delete the pool
func deletePool(context *clusterd.Context, p *cephv1.CephBlockPool) error {

	if err := ceph.DeletePool(context, p.Namespace, p.Name); err != nil {
		return errors.Wrapf(err, "failed to delete pool %q", p.Name)
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
		DeviceClass:   pool.DeviceClass,
		Replicated:    cephv1.ReplicatedSpec{Size: pool.ReplicatedConfig.Size},
		ErasureCoded:  cephv1.ErasureCodedSpec{CodingChunks: ec.CodingChunkCount, DataChunks: ec.DataChunkCount, Algorithm: ec.Algorithm},
	}
}

// Validate the pool arguments
func ValidatePool(context *clusterd.Context, p *cephv1.CephBlockPool) error {
	if p.Name == "" {
		return errors.New("missing name")
	}
	if p.Namespace == "" {
		return errors.New("missing namespace")
	}
	if err := ValidatePoolSpec(context, p.Namespace, &p.Spec); err != nil {
		return err
	}
	return nil
}

// ValidatePoolSpec validates the Ceph block pool spec CR
func ValidatePoolSpec(context *clusterd.Context, namespace string, p *cephv1.PoolSpec) error {
	if p.Replication() != nil && p.ErasureCode() != nil {
		return errors.New("both replication and erasure code settings cannot be specified")
	}

	var crush ceph.CrushMap
	var err error
	if p.FailureDomain != "" || p.CrushRoot != "" {
		crush, err = ceph.GetCrushMap(context, namespace)
		if err != nil {
			return errors.Wrapf(err, "failed to get crush map")
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
			return errors.Errorf("unrecognized failure domain %s", p.FailureDomain)
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
			return errors.Errorf("unrecognized crush root %s", p.CrushRoot)
		}
	}

	return nil
}

func getPoolObject(obj interface{}) (pool *cephv1.CephBlockPool, err error) {
	var ok bool
	pool, ok = obj.(*cephv1.CephBlockPool)
	if ok {
		// the pool object is of the latest type, simply return it
		return pool.DeepCopy(), nil
	}

	return nil, errors.Errorf("not a known pool object %+v", obj)
}

func updateCephBlockPoolStatus(name, namespace, status string, context *clusterd.Context) {
	updatedCephBlockPool, err := context.RookClientset.CephV1().CephBlockPools(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		logger.Errorf("Unable to update the cephBlockPool %s status %v", updatedCephBlockPool.GetName(), err)
		return
	}
	if updatedCephBlockPool.Status == nil {
		updatedCephBlockPool.Status = &cephv1.Status{}
	} else if updatedCephBlockPool.Status.Phase == status {
		return
	}
	updatedCephBlockPool.Status.Phase = status
	_, err = context.RookClientset.CephV1().CephBlockPools(updatedCephBlockPool.Namespace).Update(updatedCephBlockPool)
	if err != nil {
		logger.Errorf("Unable to update the cephBlockPool %s status %v", updatedCephBlockPool.GetName(), err)
		return
	}
}
