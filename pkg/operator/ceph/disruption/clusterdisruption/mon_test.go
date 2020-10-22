/*
Copyright 2020 The Rook Authors. All rights reserved.

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

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/stretchr/testify/assert"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func createFakeReconcileClusterDisruption(t *testing.T, obj ...runtime.Object) *ReconcileClusterDisruption {
	scheme := scheme.Scheme
	err := policyv1beta1.AddToScheme(scheme)
	if err != nil {
		assert.Fail(t, "failed to build scheme")
	}
	client := fake.NewFakeClientWithScheme(scheme, obj...)

	return &ReconcileClusterDisruption{
		client: client,
		scheme: scheme,
	}
}

func TestReconcileMonPDB(t *testing.T) {
	r := createFakeReconcileClusterDisruption(t, &cephv1.CephCluster{})
	testCases := []struct {
		label                string
		cephCluster          *cephv1.CephCluster
		expectedMinAvailable int32
		errorExpected        bool
	}{
		{
			label: "case 1: 0 mons",
			cephCluster: &cephv1.CephCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "rook", Namespace: "rook-ceph"},
			},
			expectedMinAvailable: 0,
			errorExpected:        true,
		},
		{
			label: "case 2: 3 mons",
			cephCluster: &cephv1.CephCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "rook", Namespace: "rook-ceph"},
				Spec: cephv1.ClusterSpec{
					Mon: cephv1.MonSpec{
						Count: 3,
					},
				},
			},
			expectedMinAvailable: 2,
			errorExpected:        false,
		},
		{
			label: "case 3: 5 mons",
			cephCluster: &cephv1.CephCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "rook", Namespace: "rook-ceph"},
				Spec: cephv1.ClusterSpec{
					Mon: cephv1.MonSpec{
						Count: 5,
					},
				},
			},
			expectedMinAvailable: 3,
			errorExpected:        false,
		},
	}

	for _, tc := range testCases {
		err := r.reconcileMonPDB(tc.cephCluster)
		assert.NoError(t, err)
		existingPDB := &policyv1beta1.PodDisruptionBudget{}
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: pdbName, Namespace: tc.cephCluster.Namespace}, existingPDB)
		if tc.errorExpected {
			assert.Error(t, err)
			continue
		}
		assert.NoError(t, err)
		assert.Equalf(t, tc.expectedMinAvailable, int32(existingPDB.Spec.MinAvailable.IntValue()), "[%s]: incorrect minAvailable count in pdb", tc.label)
	}
}
