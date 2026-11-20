/*
Copyright 2026 The Rook Authors. All rights reserved.

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

package topology

import (
	"context"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// PoolCRs holds the CRs that define pools in a namespace.
type PoolCRs struct {
	BlockPools   *cephv1.CephBlockPoolList
	Filesystems  *cephv1.CephFilesystemList
	ObjectStores *cephv1.CephObjectStoreList
}

// ListPoolCRs lists the CephBlockPools, CephFilesystems and CephObjectStores in the namespace.
func ListPoolCRs(ctx context.Context, c client.Client, namespace string) (*PoolCRs, error) {
	p := &PoolCRs{
		BlockPools:   &cephv1.CephBlockPoolList{},
		Filesystems:  &cephv1.CephFilesystemList{},
		ObjectStores: &cephv1.CephObjectStoreList{},
	}
	if err := c.List(ctx, p.BlockPools, client.InNamespace(namespace)); err != nil {
		return nil, errors.Wrapf(err, "could not list the CephBlockpools in namespace %q", namespace)
	}
	if err := c.List(ctx, p.Filesystems, client.InNamespace(namespace)); err != nil {
		return nil, errors.Wrapf(err, "could not list the CephFilesystems in namespace %q", namespace)
	}
	if err := c.List(ctx, p.ObjectStores, client.InNamespace(namespace)); err != nil {
		return nil, errors.Wrapf(err, "could not list the CephObjectStores in namespace %q", namespace)
	}
	return p, nil
}

// Count returns the number of CRs that define pools.
func (p *PoolCRs) Count() int {
	return len(p.BlockPools.Items) + len(p.Filesystems.Items) + len(p.ObjectStores.Items)
}

// PoolSpecs returns the specs of all pools defined by the CRs.
func (p *PoolCRs) PoolSpecs() []cephv1.PoolSpec {
	poolSpecs := make([]cephv1.PoolSpec, 0)
	for _, cephBlockPool := range p.BlockPools.Items {
		poolSpecs = append(poolSpecs, cephBlockPool.Spec.PoolSpec)
	}
	for _, cephFilesystem := range p.Filesystems.Items {
		poolSpecs = append(poolSpecs, cephFilesystem.Spec.MetadataPool.PoolSpec)
		for _, pool := range cephFilesystem.Spec.DataPools {
			poolSpecs = append(poolSpecs, pool.PoolSpec)
		}
	}
	for _, cephObjectStore := range p.ObjectStores.Items {
		poolSpecs = append(poolSpecs, cephObjectStore.Spec.MetadataPool)
		poolSpecs = append(poolSpecs, cephObjectStore.Spec.DataPool)
	}
	return poolSpecs
}

// GetMinimumFailureDomain returns the narrowest failure domain of the given pools, among the
// CRUSH levels in CRUSHMapLevelsOrdered. It returns the default failure domain if there are no
// pools or none of their failure domains match.
func GetMinimumFailureDomain(poolList []cephv1.PoolSpec) string {
	if len(poolList) == 0 {
		return cephv1.DefaultFailureDomain
	}

	// start with max as the min
	minfailureDomainIndex := len(CRUSHMapLevelsOrdered) - 1
	matched := false

	for _, pool := range poolList {
		for index, failureDomain := range CRUSHMapLevelsOrdered {
			if index > minfailureDomainIndex {
				// index is higher than the min
				break
			}
			if pool.FailureDomain == failureDomain {
				// new min found
				matched = true
				minfailureDomainIndex = index
			}
		}
	}
	if !matched {
		logger.Debugf("could not match failure domain. defaulting to %q", cephv1.DefaultFailureDomain)
		return cephv1.DefaultFailureDomain
	}
	return CRUSHMapLevelsOrdered[minfailureDomainIndex]
}
