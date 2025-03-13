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
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	mockNamespace = "test-ns"
)

func createFakeCluster(t *testing.T, cephClusterObj *cephv1.CephCluster, k8sVersion string) *Cluster {
	ctx := context.TODO()
	ownerInfo := cephclient.NewMinimumOwnerInfoWithOwnerRef()
	scheme := scheme.Scheme
	err := policyv1.AddToScheme(scheme)
	assert.NoError(t, err)

	cl := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects().Build()
	clientset := test.New(t, 3)
	c := New(ctx, &clusterd.Context{Client: cl, Clientset: clientset}, mockNamespace, cephClusterObj.Spec, ownerInfo)
	test.SetFakeKubernetesVersion(clientset, k8sVersion)
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
			expectedMaxUnAvailable: 2,
			errorExpected:          false,
		},
	}

	for _, tc := range testCases {

		// check for PDBV1 version
		c := createFakeCluster(t, tc.cephCluster, "v1.21.0")
		err := c.reconcileMonPDB()
		assert.NoError(t, err)
		existingPDBV1 := &policyv1.PodDisruptionBudget{}
		err = c.context.Client.Get(context.TODO(), types.NamespacedName{Name: monPDBName, Namespace: mockNamespace}, existingPDBV1)
		if tc.errorExpected {
			assert.Error(t, err)
			continue
		}
		assert.NoError(t, err)
		// nolint:gosec // G115 no overflow expected in the test
		assert.Equalf(t, tc.expectedMaxUnAvailable, int32(existingPDBV1.Spec.MaxUnavailable.IntValue()), "[%s]: incorrect minAvailable count in pdb", tc.name)

		// reconcile mon PDB again to test update
		err = c.reconcileMonPDB()
		assert.NoError(t, err)
	}
}

func TestAllowMonDrain(t *testing.T) {
	fakeNamespaceName := types.NamespacedName{Namespace: mockNamespace, Name: monPDBName}
	// check for PDBV1 version
	c := createFakeCluster(t, &cephv1.CephCluster{
		Spec: cephv1.ClusterSpec{
			DisruptionManagement: cephv1.DisruptionManagementSpec{
				ManagePodBudgets: true,
			},
		},
	}, "v1.21.0")
	t.Run("allow mon drain for K8s version v1.21.0", func(t *testing.T) {
		// change MaxUnavailable mon PDB to 1
		err := c.allowMonDrain(fakeNamespaceName)
		assert.NoError(t, err)
		existingPDBV1 := &policyv1.PodDisruptionBudget{}
		err = c.context.Client.Get(context.TODO(), fakeNamespaceName, existingPDBV1)
		assert.NoError(t, err)
		assert.Equal(t, 1, int(existingPDBV1.Spec.MaxUnavailable.IntValue()))
	})
}

func TestBlockMonDrain(t *testing.T) {
	fakeNamespaceName := types.NamespacedName{Namespace: mockNamespace, Name: monPDBName}
	// check for PDBV1 version
	c := createFakeCluster(t, &cephv1.CephCluster{
		Spec: cephv1.ClusterSpec{
			DisruptionManagement: cephv1.DisruptionManagementSpec{
				ManagePodBudgets: true,
			},
		},
	}, "v1.21.0")
	t.Run("block mon drain for K8s version v1.21.0", func(t *testing.T) {
		// change MaxUnavailable mon PDB to 0
		err := c.blockMonDrain(fakeNamespaceName)
		assert.NoError(t, err)
		existingPDBV1 := &policyv1.PodDisruptionBudget{}
		err = c.context.Client.Get(context.TODO(), fakeNamespaceName, existingPDBV1)
		assert.NoError(t, err)
		assert.Equal(t, 0, int(existingPDBV1.Spec.MaxUnavailable.IntValue()))
	})
}
