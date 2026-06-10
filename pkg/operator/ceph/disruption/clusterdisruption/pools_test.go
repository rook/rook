/*
Copyright 2019 The Rook Authors. All rights reserved.

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

package clusterdisruption

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/rook/rook/pkg/operator/ceph/disruption/controllerconfig"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGetMinimumFailureDomain(t *testing.T) {
	poolList := []cephv1.PoolSpec{
		{FailureDomain: "region"},
		{FailureDomain: "zone"},
	}

	assert.Equal(t, "zone", getMinimumFailureDomain(poolList))

	poolList = []cephv1.PoolSpec{
		{FailureDomain: "region"},
		{FailureDomain: "zone"},
		{FailureDomain: "host"},
	}

	assert.Equal(t, "host", getMinimumFailureDomain(poolList))

	// test default
	poolList = []cephv1.PoolSpec{
		{FailureDomain: "aaa"},
		{FailureDomain: "bbb"},
		{FailureDomain: "ccc"},
	}

	assert.Equal(t, "host", getMinimumFailureDomain(poolList))
}

func TestReconcileCephObjectStorePDB(t *testing.T) {
	ns := "rook-ceph"
	storeName := "my-store"
	pdbName := "rook-ceph-rgw-" + storeName

	s := scheme.Scheme
	err := policyv1.AddToScheme(s)
	assert.NoError(t, err)

	t.Run("scale down from 2 to 1 deletes stale PDB", func(t *testing.T) {
		existingPDB := &policyv1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{Name: pdbName, Namespace: ns},
			Spec: policyv1.PodDisruptionBudgetSpec{
				MinAvailable: &intstr.IntOrString{IntVal: 1},
			},
		}
		client := fake.NewClientBuilder().WithScheme(s).WithObjects(existingPDB).Build()
		r := &ReconcileClusterDisruption{
			client:  client,
			scheme:  s,
			context: &controllerconfig.Context{OpManagerContext: context.TODO()},
		}

		objectStoreList := &cephv1.CephObjectStoreList{
			Items: []cephv1.CephObjectStore{
				{
					ObjectMeta: metav1.ObjectMeta{Name: storeName, Namespace: ns},
					Spec:       cephv1.ObjectStoreSpec{Gateway: cephv1.GatewaySpec{Instances: 1}},
				},
			},
		}

		err := r.reconcileCephObjectStore(objectStoreList)
		assert.NoError(t, err)

		pdb := &policyv1.PodDisruptionBudget{}
		err = client.Get(context.TODO(), types.NamespacedName{Name: pdbName, Namespace: ns}, pdb)
		assert.Error(t, err, "PDB should be deleted")
	})

	t.Run("scale down with no existing PDB does not error", func(t *testing.T) {
		client := fake.NewClientBuilder().WithScheme(s).Build()
		r := &ReconcileClusterDisruption{
			client:  client,
			scheme:  s,
			context: &controllerconfig.Context{OpManagerContext: context.TODO()},
		}

		objectStoreList := &cephv1.CephObjectStoreList{
			Items: []cephv1.CephObjectStore{
				{
					ObjectMeta: metav1.ObjectMeta{Name: storeName, Namespace: ns},
					Spec:       cephv1.ObjectStoreSpec{Gateway: cephv1.GatewaySpec{Instances: 1}},
				},
			},
		}

		err := r.reconcileCephObjectStore(objectStoreList)
		assert.NoError(t, err)
	})

	t.Run("2 instances creates PDB", func(t *testing.T) {
		client := fake.NewClientBuilder().WithScheme(s).Build()
		r := &ReconcileClusterDisruption{
			client:  client,
			scheme:  s,
			context: &controllerconfig.Context{OpManagerContext: context.TODO()},
		}

		objectStoreList := &cephv1.CephObjectStoreList{
			Items: []cephv1.CephObjectStore{
				{
					ObjectMeta: metav1.ObjectMeta{Name: storeName, Namespace: ns},
					Spec:       cephv1.ObjectStoreSpec{Gateway: cephv1.GatewaySpec{Instances: 2}},
				},
			},
		}

		err := r.reconcileCephObjectStore(objectStoreList)
		assert.NoError(t, err)

		pdb := &policyv1.PodDisruptionBudget{}
		err = client.Get(context.TODO(), types.NamespacedName{Name: pdbName, Namespace: ns}, pdb)
		assert.NoError(t, err)
		assert.Equal(t, int32(1), pdb.Spec.MinAvailable.IntVal)
	})
}

func TestReconcileCephFilesystemPDB(t *testing.T) {
	ns := "rook-ceph"
	fsName := "my-fs"
	pdbName := "rook-ceph-mds-" + fsName

	s := scheme.Scheme
	err := policyv1.AddToScheme(s)
	assert.NoError(t, err)

	t.Run("scale down from 2 to 1 active with no standby deletes stale PDB", func(t *testing.T) {
		existingPDB := &policyv1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{Name: pdbName, Namespace: ns},
			Spec: policyv1.PodDisruptionBudgetSpec{
				MinAvailable: &intstr.IntOrString{IntVal: 1},
			},
		}
		client := fake.NewClientBuilder().WithScheme(s).WithObjects(existingPDB).Build()
		r := &ReconcileClusterDisruption{
			client:  client,
			scheme:  s,
			context: &controllerconfig.Context{OpManagerContext: context.TODO()},
		}

		fsList := &cephv1.CephFilesystemList{
			Items: []cephv1.CephFilesystem{
				{
					ObjectMeta: metav1.ObjectMeta{Name: fsName, Namespace: ns},
					Spec: cephv1.FilesystemSpec{
						MetadataServer: cephv1.MetadataServerSpec{ActiveCount: 1, ActiveStandby: false},
					},
				},
			},
		}

		err := r.reconcileCephFilesystem(fsList)
		assert.NoError(t, err)

		pdb := &policyv1.PodDisruptionBudget{}
		err = client.Get(context.TODO(), types.NamespacedName{Name: pdbName, Namespace: ns}, pdb)
		assert.Error(t, err, "PDB should be deleted")
	})

	t.Run("scale down with no existing PDB does not error", func(t *testing.T) {
		client := fake.NewClientBuilder().WithScheme(s).Build()
		r := &ReconcileClusterDisruption{
			client:  client,
			scheme:  s,
			context: &controllerconfig.Context{OpManagerContext: context.TODO()},
		}

		fsList := &cephv1.CephFilesystemList{
			Items: []cephv1.CephFilesystem{
				{
					ObjectMeta: metav1.ObjectMeta{Name: fsName, Namespace: ns},
					Spec: cephv1.FilesystemSpec{
						MetadataServer: cephv1.MetadataServerSpec{ActiveCount: 1, ActiveStandby: false},
					},
				},
			},
		}

		err := r.reconcileCephFilesystem(fsList)
		assert.NoError(t, err)
	})

	t.Run("1 active with standby creates PDB", func(t *testing.T) {
		client := fake.NewClientBuilder().WithScheme(s).Build()
		r := &ReconcileClusterDisruption{
			client:  client,
			scheme:  s,
			context: &controllerconfig.Context{OpManagerContext: context.TODO()},
		}

		fsList := &cephv1.CephFilesystemList{
			Items: []cephv1.CephFilesystem{
				{
					ObjectMeta: metav1.ObjectMeta{Name: fsName, Namespace: ns},
					Spec: cephv1.FilesystemSpec{
						MetadataServer: cephv1.MetadataServerSpec{ActiveCount: 1, ActiveStandby: true},
					},
				},
			},
		}

		err := r.reconcileCephFilesystem(fsList)
		assert.NoError(t, err)

		pdb := &policyv1.PodDisruptionBudget{}
		err = client.Get(context.TODO(), types.NamespacedName{Name: pdbName, Namespace: ns}, pdb)
		assert.NoError(t, err)
		assert.Equal(t, int32(1), pdb.Spec.MinAvailable.IntVal)
	})
}
