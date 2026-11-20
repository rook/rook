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
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGetMinimumFailureDomain(t *testing.T) {
	replicated := func(failureDomain string) cephv1.PoolSpec {
		return cephv1.PoolSpec{FailureDomain: failureDomain, Replicated: cephv1.ReplicatedSpec{Size: 3}}
	}
	erasureCoded := func(failureDomain string) cephv1.PoolSpec {
		return cephv1.PoolSpec{FailureDomain: failureDomain, ErasureCoded: cephv1.ErasureCodedSpec{DataChunks: 2, CodingChunks: 1}}
	}

	withSubFailureDomain := func(failureDomain, subFailureDomain string, replicasPerFailureDomain uint) cephv1.PoolSpec {
		pool := replicated(failureDomain)
		pool.Replicated.Size = 4
		pool.Replicated.SubFailureDomain = subFailureDomain
		pool.Replicated.ReplicasPerFailureDomain = replicasPerFailureDomain
		return pool
	}

	withSubFailureDomainEC := func(failureDomain, subFailureDomain string, replicasPerFailureDomain uint) cephv1.PoolSpec {
		pool := erasureCoded(failureDomain)
		pool.Replicated.SubFailureDomain = subFailureDomain
		pool.Replicated.ReplicasPerFailureDomain = replicasPerFailureDomain
		return pool
	}

	tests := []struct {
		name  string
		pools []cephv1.PoolSpec
		want  string
	}{
		{"no pools", nil, "host"},
		{"zone is narrower than region", []cephv1.PoolSpec{replicated("region"), replicated("zone")}, "zone"},
		{"host is the narrowest", []cephv1.PoolSpec{replicated("region"), replicated("zone"), replicated("host")}, "host"},
		{"single pool", []cephv1.PoolSpec{replicated("region")}, "region"},
		{"erasure coded pool", []cephv1.PoolSpec{replicated("region"), erasureCoded("rack")}, "rack"},
		{"unknown failure domains", []cephv1.PoolSpec{replicated("aaa"), replicated("bbb")}, "host"},
		{"empty failure domain is host", []cephv1.PoolSpec{replicated("zone"), replicated("")}, "host"},
		{"osd failure domain is host", []cephv1.PoolSpec{replicated("zone"), replicated("osd")}, "host"},
		{"undefined pools are ignored", []cephv1.PoolSpec{replicated("zone"), {}}, "zone"},
		{"sub-failure domain", []cephv1.PoolSpec{withSubFailureDomain("region", "rack", 2)}, "rack"},
		{"empty sub-failure domain is host", []cephv1.PoolSpec{withSubFailureDomain("zone", "", 2)}, "host"},
		{"sub-failure domain needs more than one replica per failure domain", []cephv1.PoolSpec{withSubFailureDomain("zone", "rack", 1)}, "zone"},
		{"sub-failure domain of erasure coded pool is ignored", []cephv1.PoolSpec{withSubFailureDomainEC("zone", "rack", 2)}, "zone"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, GetMinimumFailureDomain(tt.pools))
		})
	}
}

func TestListPoolCRs(t *testing.T) {
	ns := "rook-ceph"
	objects := []runtime.Object{
		&cephv1.CephBlockPool{
			ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: ns},
			Spec:       cephv1.NamedBlockPoolSpec{PoolSpec: cephv1.PoolSpec{Replicated: cephv1.ReplicatedSpec{Size: 3}, FailureDomain: "rack"}},
		},
		&cephv1.CephBlockPool{
			ObjectMeta: metav1.ObjectMeta{Name: "other-ns-pool", Namespace: "other"},
			Spec:       cephv1.NamedBlockPoolSpec{PoolSpec: cephv1.PoolSpec{Replicated: cephv1.ReplicatedSpec{Size: 3}, FailureDomain: "osd"}},
		},
		&cephv1.CephFilesystem{
			ObjectMeta: metav1.ObjectMeta{Name: "fs", Namespace: ns},
			Spec: cephv1.FilesystemSpec{
				MetadataPool: cephv1.NamedPoolSpec{PoolSpec: cephv1.PoolSpec{Replicated: cephv1.ReplicatedSpec{Size: 3}, FailureDomain: "zone"}},
				DataPools: []cephv1.NamedPoolSpec{
					{PoolSpec: cephv1.PoolSpec{Replicated: cephv1.ReplicatedSpec{Size: 3}, FailureDomain: "zone"}},
					{PoolSpec: cephv1.PoolSpec{Replicated: cephv1.ReplicatedSpec{Size: 3}, FailureDomain: "region"}},
				},
			},
		},
		&cephv1.CephObjectStore{
			ObjectMeta: metav1.ObjectMeta{Name: "store", Namespace: ns},
			Spec: cephv1.ObjectStoreSpec{
				MetadataPool: cephv1.PoolSpec{Replicated: cephv1.ReplicatedSpec{Size: 3}, FailureDomain: "row"},
				DataPool:     cephv1.PoolSpec{Replicated: cephv1.ReplicatedSpec{Size: 3}, FailureDomain: "datacenter"},
			},
		},
	}
	s := scheme.Scheme
	c := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objects...).Build()

	poolCRs, err := ListPoolCRs(context.TODO(), c, ns)
	assert.NoError(t, err)
	assert.Equal(t, 3, poolCRs.Count())
	failureDomains := []string{}
	for _, spec := range poolCRs.PoolSpecs() {
		failureDomains = append(failureDomains, spec.FailureDomain)
	}
	assert.ElementsMatch(t, []string{"rack", "zone", "zone", "region", "row", "datacenter"}, failureDomains)
	assert.Equal(t, "rack", GetMinimumFailureDomain(poolCRs.PoolSpecs()))
}
