/*
Copyright 2021 The Rook Authors. All rights reserved.

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

package mon

import (
	"context"
	"sync"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/stretchr/testify/assert"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	mockNamespace = "test-ns"
)

func createFakeCluster(t *testing.T, cephClusterObj *cephv1.CephCluster) *Cluster {
	ownerInfo := cephclient.NewMinimumOwnerInfoWithOwnerRef()
	scheme := scheme.Scheme
	err := policyv1beta1.AddToScheme(scheme)
	if err != nil {
		assert.Fail(t, "failed to build scheme")
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects().Build()

	c := New(&clusterd.Context{Client: cl}, mockNamespace, cephClusterObj.Spec, ownerInfo, &sync.Mutex{})

	return c
}

func TestReconcileMonPDB(t *testing.T) {
	testCases := []struct {
		name                   string
		cephCluster            *cephv1.CephCluster
		expectedMaxUnAvailable int32
		errorExpected          bool
	}{
		{
			name: "0 mons",
			cephCluster: &cephv1.CephCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "rook", Namespace: mockNamespace},
				Spec: cephv1.ClusterSpec{
					DisruptionManagement: cephv1.DisruptionManagementSpec{
						ManagePodBudgets: true,
					},
				},
			},
			expectedMaxUnAvailable: 0,
			errorExpected:          true,
		},
		{
			name: "3 mons",
			cephCluster: &cephv1.CephCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "rook", Namespace: mockNamespace},
				Spec: cephv1.ClusterSpec{
					Mon: cephv1.MonSpec{
						Count: 3,
					},
					DisruptionManagement: cephv1.DisruptionManagementSpec{
						ManagePodBudgets: true,
					},
				},
			},
			expectedMaxUnAvailable: 1,
			errorExpected:          false,
		},
		{
			name: "5 mons",
			cephCluster: &cephv1.CephCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "rook", Namespace: mockNamespace},
				Spec: cephv1.ClusterSpec{
					Mon: cephv1.MonSpec{
						Count: 5,
					},
					DisruptionManagement: cephv1.DisruptionManagementSpec{
						ManagePodBudgets: true,
					},
				},
			},
			expectedMaxUnAvailable: 1,
			errorExpected:          false,
		},
	}

	for _, tc := range testCases {
		c := createFakeCluster(t, tc.cephCluster)
		err := c.reconcileMonPDB()
		assert.NoError(t, err)
		existingPDB := &policyv1beta1.PodDisruptionBudget{}
		err = c.context.Client.Get(context.TODO(), types.NamespacedName{Name: monPDBName, Namespace: mockNamespace}, existingPDB)
		if tc.errorExpected {
			assert.Error(t, err)
			continue
		}
		assert.NoError(t, err)
		assert.Equalf(t, tc.expectedMaxUnAvailable, int32(existingPDB.Spec.MaxUnavailable.IntValue()), "[%s]: incorrect minAvailable count in pdb", tc.name)

		// reconcile mon PDB again to test update
		err = c.reconcileMonPDB()
		assert.NoError(t, err)
	}
}

func TestAllowMonDrain(t *testing.T) {
	c := createFakeCluster(t, &cephv1.CephCluster{
		Spec: cephv1.ClusterSpec{
			DisruptionManagement: cephv1.DisruptionManagementSpec{
				ManagePodBudgets: true,
			},
		},
	})
	fakeNamespaceName := types.NamespacedName{Namespace: mockNamespace, Name: monPDBName}
	t.Run("allow mon drain", func(t *testing.T) {
		// change MaxUnavailable mon PDB to 1
		err := c.allowMonDrain(fakeNamespaceName)
		assert.NoError(t, err)
		existingPDB := &policyv1beta1.PodDisruptionBudget{}
		err = c.context.Client.Get(context.TODO(), fakeNamespaceName, existingPDB)
		assert.NoError(t, err)
		assert.Equal(t, 1, int(existingPDB.Spec.MaxUnavailable.IntValue()))
	})
}

func TestBlockMonDrain(t *testing.T) {
	c := createFakeCluster(t, &cephv1.CephCluster{
		Spec: cephv1.ClusterSpec{
			DisruptionManagement: cephv1.DisruptionManagementSpec{
				ManagePodBudgets: true,
			},
		},
	})
	fakeNamespaceName := types.NamespacedName{Namespace: mockNamespace, Name: monPDBName}
	t.Run("block mon drain", func(t *testing.T) {
		// change MaxUnavailable mon PDB to 0
		err := c.blockMonDrain(fakeNamespaceName)
		assert.NoError(t, err)
		existingPDB := &policyv1beta1.PodDisruptionBudget{}
		err = c.context.Client.Get(context.TODO(), fakeNamespaceName, existingPDB)
		assert.NoError(t, err)
		assert.Equal(t, 0, int(existingPDB.Spec.MaxUnavailable.IntValue()))
	})
}
