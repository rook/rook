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

// Count returns the number of CRs that may define pools.
func (p *PoolCRs) Count() int {
	return len(p.BlockPools.Items) + len(p.Filesystems.Items) + len(p.ObjectStores.Items)
}

// PoolSpecs returns the specs of all pools defined by the CRs, leaving out unset pool specs such
// as those of an object store using shared pools.
func (p *PoolCRs) PoolSpecs() []cephv1.PoolSpec {
	poolSpecs := make([]cephv1.PoolSpec, 0)
	appendDefined := func(pool cephv1.PoolSpec) {
		if pool.IsReplicated() || pool.IsErasureCoded() {
			poolSpecs = append(poolSpecs, pool)
		}
	}
	for _, cephBlockPool := range p.BlockPools.Items {
		appendDefined(cephBlockPool.Spec.PoolSpec)
	}
	for _, cephFilesystem := range p.Filesystems.Items {
		appendDefined(cephFilesystem.Spec.MetadataPool.PoolSpec)
		for _, pool := range cephFilesystem.Spec.DataPools {
			appendDefined(pool.PoolSpec)
		}
	}
	for _, cephObjectStore := range p.ObjectStores.Items {
		appendDefined(cephObjectStore.Spec.MetadataPool)
		appendDefined(cephObjectStore.Spec.DataPool)
	}
	return poolSpecs
}

// GetMinimumFailureDomain returns the narrowest failure domain of the given pools, among the
// CRUSH levels in CRUSHMapLevelsOrdered. For replicated pools with more than one replica per
// failure domain, this includes their sub-failure domain. Failure domains that are unset or "osd"
// are counted as "host". Pools that are neither replicated nor erasure coded, such as the unset
// pools of an object store using shared pools, are ignored. It returns the default failure domain
// if there are no pools or none of their failure domains match.
func GetMinimumFailureDomain(poolList []cephv1.PoolSpec) string {
	// start with max as the min
	minfailureDomainIndex := len(CRUSHMapLevelsOrdered) - 1
	matched := false

	for _, pool := range poolList {
		for _, poolFailureDomain := range poolFailureDomains(pool) {
			for index, failureDomain := range CRUSHMapLevelsOrdered {
				if index > minfailureDomainIndex {
					// index is higher than the min
					break
				}
				if poolFailureDomain == failureDomain {
					// new min found
					matched = true
					minfailureDomainIndex = index
				}
			}
		}
	}
	if !matched {
		logger.Debugf("could not match failure domain. defaulting to %q", cephv1.DefaultFailureDomain)
		return cephv1.DefaultFailureDomain
	}
	return CRUSHMapLevelsOrdered[minfailureDomainIndex]
}

// poolFailureDomains returns the failure domains that Rook creates the CRUSH rule of the pool with,
// with "osd" counted as "host".
func poolFailureDomains(pool cephv1.PoolSpec) []string {
	if !pool.IsReplicated() && !pool.IsErasureCoded() {
		return nil // the pool is not defined
	}
	withDefault := func(failureDomain string) string {
		switch failureDomain {
		case "":
			return cephv1.DefaultFailureDomain // Rook creates the pool's CRUSH rule with this failure domain
		case "osd":
			return "host" // the narrowest failure domain the PDBs can allow to be disrupted
		}
		return failureDomain
	}
	failureDomains := []string{withDefault(pool.FailureDomain)}
	if pool.IsReplicated() && pool.Replicated.ReplicasPerFailureDomain > 1 {
		// the CRUSH rule chooses the replicas within each failure domain from different
		// sub-failure domains
		failureDomains = append(failureDomains, withDefault(pool.Replicated.SubFailureDomain))
	}
	return failureDomains
}
