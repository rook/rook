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
	poolList := []cephv1.PoolSpec{
		{FailureDomain: "region"},
		{FailureDomain: "zone"},
	}

	assert.Equal(t, "zone", GetMinimumFailureDomain(poolList))

	poolList = []cephv1.PoolSpec{
		{FailureDomain: "region"},
		{FailureDomain: "zone"},
		{FailureDomain: "host"},
	}

	assert.Equal(t, "host", GetMinimumFailureDomain(poolList))

	poolList = []cephv1.PoolSpec{
		{FailureDomain: "region"},
	}

	assert.Equal(t, "region", GetMinimumFailureDomain(poolList))

	// test default
	poolList = []cephv1.PoolSpec{
		{FailureDomain: "aaa"},
		{FailureDomain: "bbb"},
		{FailureDomain: "ccc"},
	}

	assert.Equal(t, "host", GetMinimumFailureDomain(poolList))
}

func TestListPoolCRs(t *testing.T) {
	ns := "rook-ceph"
	objects := []runtime.Object{
		&cephv1.CephBlockPool{
			ObjectMeta: metav1.ObjectMeta{Name: "pool", Namespace: ns},
			Spec:       cephv1.NamedBlockPoolSpec{PoolSpec: cephv1.PoolSpec{FailureDomain: "rack"}},
		},
		&cephv1.CephBlockPool{
			ObjectMeta: metav1.ObjectMeta{Name: "other-ns-pool", Namespace: "other"},
			Spec:       cephv1.NamedBlockPoolSpec{PoolSpec: cephv1.PoolSpec{FailureDomain: "osd"}},
		},
		&cephv1.CephFilesystem{
			ObjectMeta: metav1.ObjectMeta{Name: "fs", Namespace: ns},
			Spec: cephv1.FilesystemSpec{
				MetadataPool: cephv1.NamedPoolSpec{PoolSpec: cephv1.PoolSpec{FailureDomain: "zone"}},
				DataPools: []cephv1.NamedPoolSpec{
					{PoolSpec: cephv1.PoolSpec{FailureDomain: "zone"}},
					{PoolSpec: cephv1.PoolSpec{FailureDomain: "region"}},
				},
			},
		},
		&cephv1.CephObjectStore{
			ObjectMeta: metav1.ObjectMeta{Name: "store", Namespace: ns},
			Spec: cephv1.ObjectStoreSpec{
				MetadataPool: cephv1.PoolSpec{FailureDomain: "row"},
				DataPool:     cephv1.PoolSpec{FailureDomain: "datacenter"},
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
