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
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	clienttest "github.com/rook/rook/pkg/daemon/ceph/client/test"
	"github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	policyv1 "k8s.io/api/policy/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	mockNamespace = "test-ns"
)

// createMonQuorumResponse creates a mock quorum status response with specified mons and quorum
func createMonQuorumResponse(monNames []string, quorumRanks []int) string {
	resp := cephclient.MonStatusResponse{Quorum: quorumRanks}
	resp.MonMap.Mons = []cephclient.MonMapEntry{}
	for i, name := range monNames {
		resp.MonMap.Mons = append(resp.MonMap.Mons, cephclient.MonMapEntry{
			Name:    name,
			Rank:    i,
			Address: fmt.Sprintf("1.2.3.%d", i+1),
		})
	}
	serialized, _ := json.Marshal(resp)
	return string(serialized)
}

// createFakeClusterWithExecutor creates a fake cluster with optional executor for mocking ceph commands
func createFakeClusterWithExecutor(t *testing.T, cephClusterObj *cephv1.CephCluster, k8sVersion string, executor *exectest.MockExecutor) *Cluster {
	ctx := context.TODO()
	ownerInfo := cephclient.NewMinimumOwnerInfoWithOwnerRef()
	scheme := scheme.Scheme
	err := policyv1.AddToScheme(scheme)
	assert.NoError(t, err)

	cl := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects().Build()
	clientset := test.New(t, 3)

	clusterdContext := &clusterd.Context{
		Client:    cl,
		Clientset: clientset,
		Executor:  executor,
	}

	c := New(ctx, clusterdContext, mockNamespace, cephClusterObj.Spec, ownerInfo)
	c.ClusterInfo = clienttest.CreateTestClusterInfo(int(cephClusterObj.Spec.Mon.Count))
	test.SetFakeKubernetesVersion(clientset, k8sVersion)
	return c
}

func TestReconcileMonPDB(t *testing.T) {
	testCases := []struct {
		name                   string
		cephCluster            *cephv1.CephCluster
		quorumResponse         string
		expectedMaxUnAvailable int32
		shouldCreatePDB        bool
		errorExpected          bool
	}{
		{
			name: "managePodBudgets disabled",
			cephCluster: &cephv1.CephCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "rook", Namespace: mockNamespace},
				Spec: cephv1.ClusterSpec{
					Mon: cephv1.MonSpec{
						Count: 3,
					},
					DisruptionManagement: cephv1.DisruptionManagementSpec{
						ManagePodBudgets: false,
					},
				},
			},
			quorumResponse:         clienttest.MonInQuorumResponse(),
			expectedMaxUnAvailable: 0,
			shouldCreatePDB:        false,
			errorExpected:          false,
		},
		{
			name: "mon count is 1",
			cephCluster: &cephv1.CephCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "rook", Namespace: mockNamespace},
				Spec: cephv1.ClusterSpec{
					Mon: cephv1.MonSpec{
						Count: 1,
					},
					DisruptionManagement: cephv1.DisruptionManagementSpec{
						ManagePodBudgets: true,
					},
				},
			},
			quorumResponse:         createMonQuorumResponse([]string{"a"}, []int{0}),
			expectedMaxUnAvailable: 0,
			shouldCreatePDB:        false,
			errorExpected:          false,
		},
		{
			name: "mon count is 2",
			cephCluster: &cephv1.CephCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "rook", Namespace: mockNamespace},
				Spec: cephv1.ClusterSpec{
					Mon: cephv1.MonSpec{
						Count: 2,
					},
					DisruptionManagement: cephv1.DisruptionManagementSpec{
						ManagePodBudgets: true,
					},
				},
			},
			quorumResponse:         createMonQuorumResponse([]string{"a", "b"}, []int{0, 1}),
			expectedMaxUnAvailable: 0,
			shouldCreatePDB:        false,
			errorExpected:          false,
		},
		{
			name: "3 mons - all in quorum",
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
			quorumResponse:         createMonQuorumResponse([]string{"a", "b", "c"}, []int{0, 1, 2}),
			expectedMaxUnAvailable: 1,
			shouldCreatePDB:        true,
			errorExpected:          false,
		},
		{
			name: "3 mons - 1 mon down",
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
			quorumResponse:         createMonQuorumResponse([]string{"a", "b", "c"}, []int{0, 1}),
			expectedMaxUnAvailable: 0, // maxUnavailable=1, downMonCount=1, allowedDown=1-1=0
			shouldCreatePDB:        true,
			errorExpected:          false,
		},
		{
			name: "5 mons - all in quorum",
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
			quorumResponse:         createMonQuorumResponse([]string{"a", "b", "c", "d", "e"}, []int{0, 1, 2, 3, 4}),
			expectedMaxUnAvailable: 2,
			shouldCreatePDB:        true,
			errorExpected:          false,
		},
		{
			name: "5 mons - 1 mon down",
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
			quorumResponse:         createMonQuorumResponse([]string{"a", "b", "c", "d", "e"}, []int{0, 1, 2, 3}),
			expectedMaxUnAvailable: 1, // maxUnavailable=2, downMonCount=1, allowedDown=2-1=1
			shouldCreatePDB:        true,
			errorExpected:          false,
		},
		{
			name: "5 mons - 2 mons down",
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
			quorumResponse:         createMonQuorumResponse([]string{"a", "b", "c", "d", "e"}, []int{0, 1, 2}),
			expectedMaxUnAvailable: 0, // maxUnavailable=2, downMonCount=2, allowedDown=2-2=0
			shouldCreatePDB:        true,
			errorExpected:          false,
		},
		{
			name: "5 mons - 3 mons down (edge case)",
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
			quorumResponse:         createMonQuorumResponse([]string{"a", "b", "c", "d", "e"}, []int{0, 1}),
			expectedMaxUnAvailable: 0, // maxUnavailable=2, downMonCount=3, allowedDown=2-3=-1, clamped to 0
			shouldCreatePDB:        true,
			errorExpected:          false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			executor := &exectest.MockExecutor{
				MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
					if strings.Contains(command, "quorum_status") || (len(args) > 0 && args[0] == "quorum_status") {
						return tc.quorumResponse, nil
					}
					return "", nil
				},
			}

			c := createFakeClusterWithExecutor(t, tc.cephCluster, "v1.21.0", executor)
			quorumStatus, err := c.reconcileMonPDB()
			if tc.errorExpected {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			existingPDB := &policyv1.PodDisruptionBudget{}
			err = c.context.Client.Get(context.TODO(), types.NamespacedName{Name: monPDBName, Namespace: mockNamespace}, existingPDB)

			if !tc.shouldCreatePDB {
				assert.Nil(t, quorumStatus)
				assert.True(t, kerrors.IsNotFound(err), "PDB should not exist for test case: %s", tc.name)
				return
			}
			assert.NotNil(t, quorumStatus)

			require.NoError(t, err, "Failed to get PDB for test case: %s", tc.name)
			// nolint:gosec // G115 no overflow expected in the test
			assert.Equalf(t, tc.expectedMaxUnAvailable, int32(existingPDB.Spec.MaxUnavailable.IntValue()),
				"[%s]: incorrect maxUnavailable count in pdb", tc.name)

			// reconcile mon PDB again to test update
			quorumStatus, err = c.reconcileMonPDB()
			assert.NoError(t, err)
			assert.NotNil(t, quorumStatus)

			err = c.context.Client.Delete(context.TODO(), existingPDB)
			assert.NoError(t, err)
		})
	}
}

func TestGetMaxUnavailableMonPodCount(t *testing.T) {
	testCases := []struct {
		name        string
		monCount    int
		expectedMax int32
	}{
		{
			name:        "1 mon",
			monCount:    1,
			expectedMax: 1,
		},
		{
			name:        "3 mons",
			monCount:    3,
			expectedMax: 1,
		},
		{
			name:        "5 mons",
			monCount:    5,
			expectedMax: 2,
		},
		{
			name:        "7 mons",
			monCount:    7,
			expectedMax: 2,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c := &Cluster{
				spec: cephv1.ClusterSpec{
					Mon: cephv1.MonSpec{
						Count: tc.monCount,
					},
				},
				Namespace: mockNamespace,
			}
			actual := c.getMaxUnavailableMonPodCount()
			assert.Equal(t, tc.expectedMax, actual, "getMaxUnavailableMonPodCount returned wrong value for %d mons", tc.monCount)
		})
	}
}
