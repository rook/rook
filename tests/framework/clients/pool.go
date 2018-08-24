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

package clients

import (
	"fmt"
	"strconv"

	cephv1beta1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1beta1"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//PoolOperation is a wrapper for rook pool operations
type PoolOperation struct {
	k8sh      *utils.K8sHelper
	manifests installer.CephManifests
}

// CreatePoolOperation creates a new pool client
func CreatePoolOperation(k8sh *utils.K8sHelper, manifests installer.CephManifests) *PoolOperation {
	return &PoolOperation{k8sh, manifests}
}

func (p *PoolOperation) Create(name, namespace string, replicas int) (string, error) {
	return p.createOrUpdatePool(name, namespace, "create", replicas)
}

func (p *PoolOperation) Delete(name string, namespace string) error {
	_, err := p.k8sh.ResourceOperation("delete", p.manifests.GetBlockPoolDef(name, namespace, "1"))
	return err
}

func (p *PoolOperation) Update(name, namespace string, replicas int) (string, error) {
	return p.createOrUpdatePool(name, namespace, "apply", replicas)
}

func (p *PoolOperation) createOrUpdatePool(name, namespace, action string, replicas int) (string, error) {
	return p.k8sh.ResourceOperation(action, p.manifests.GetBlockPoolDef(name, namespace, strconv.Itoa(replicas)))
}

func (p *PoolOperation) ListCephPools(namespace string) ([]client.CephStoragePoolSummary, error) {
	context := p.k8sh.MakeContext()
	pools, err := client.ListPoolSummaries(context, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to list pools: %+v", err)
	}
	return pools, nil
}

func (p *PoolOperation) GetCephPoolDetails(namespace, name string) (client.CephStoragePoolDetails, error) {
	context := p.k8sh.MakeContext()
	details, err := client.GetPoolDetails(context, namespace, name)
	if err != nil {
		return client.CephStoragePoolDetails{}, fmt.Errorf("failed to get pool %s details: %+v", name, err)
	}
	return details, nil
}

func (p *PoolOperation) ListPoolCRDs(namespace string) ([]cephv1beta1.Pool, error) {
	pools, err := p.k8sh.RookClientset.CephV1beta1().Pools(namespace).List(metav1.ListOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	return pools.Items, nil
}

func (p *PoolOperation) PoolCRDExists(namespace, name string) (bool, error) {
	_, err := p.k8sh.RookClientset.CephV1beta1().Pools(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (p *PoolOperation) CephPoolExists(namespace, name string) (bool, error) {
	pools, err := p.ListCephPools(namespace)
	if err != nil {
		return false, err
	}
	for _, pool := range pools {
		if pool.Name == name {
			return true, nil
		}
	}
	return false, nil
}

func (p *PoolOperation) CreateStorageClassAndPvc(namespace, poolName, storageClassName, blockName, mode string) (string, error) {
	return p.k8sh.ResourceOperation("create", p.manifests.GetBlockPoolStorageClassAndPvcDef(namespace, poolName, storageClassName, blockName, mode))
}

func (p *PoolOperation) DeleteStorageClass(namespace, poolName, storageClassName string) error {
	_, err := p.k8sh.ResourceOperation("delete", p.manifests.GetBlockPoolStorageClass(namespace, poolName, storageClassName))
	return err
}

func (p *PoolOperation) DeleteStorageClassAndPvc(namespace, poolName, storageClassName, blockName, mode string) error {
	_, err := p.k8sh.ResourceOperation("delete", p.manifests.GetBlockPoolStorageClassAndPvcDef(namespace, poolName, storageClassName, blockName, mode))
	return err
}

func (p *PoolOperation) CreateStorageClass(namespace, poolName, storageClassName string) (string, error) {
	return p.k8sh.ResourceOperation("create", p.manifests.GetBlockPoolStorageClass(namespace, poolName, storageClassName))
}
