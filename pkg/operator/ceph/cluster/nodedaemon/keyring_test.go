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

package nodedaemon

import (
	"context"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCephCrashCollectorKeyringCaps(t *testing.T) {
	caps := cephCrashCollectorKeyringCaps()
	assert.Equal(t, caps, []string{"mon", "allow profile crash", "mgr", "allow rw"})
}

func TestExporterKeyringCaps(t *testing.T) {
	caps := createExporterKeyringCaps()
	assert.Equal(t, caps, []string{"mon", "allow profile ceph-exporter", "mgr", "allow r", "osd", "allow r", "mds", "allow r"})
}

func TestCreateCrashCollectorKeyring(t *testing.T) {
	clusterContext := &clusterd.Context{}
	ctx := context.TODO()
	clusterInfo := &cephclient.ClusterInfo{
		Context:     ctx,
		Namespace:   "rook-ceph",
		CephVersion: cephver.Reef,
	}

	clusterInfo.SetName("mycluster")
	clusterInfo.OwnerInfo = cephclient.NewMinimumOwnerInfo(t)

	// create a sample ceph cluster at add to fake controller
	status := keyring.UninitializedCephxStatus()
	cephCluster := &cephv1.CephCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mycluster",
			Namespace: "rook-ceph",
		},
		Spec: cephv1.ClusterSpec{
			Security: cephv1.ClusterSecuritySpec{
				CephX: cephv1.ClusterCephxConfig{
					Daemon: cephv1.CephxConfig{
						KeyRotationPolicy: "KeyGeneration",
						KeyGeneration:     1,
					},
				},
			},
		},
		Status: cephv1.ClusterStatus{
			Phase: "",
			CephVersion: &cephv1.ClusterVersion{
				Version: "14.2.9-0",
			},
			CephStatus: &cephv1.CephStatus{
				Health: "",
			},
			Cephx: &cephv1.ClusterCephxStatus{
				CrashCollector: &status,
			},
		},
	}

	objects := []runtime.Object{
		cephCluster,
	}
	s := runtime.NewScheme()
	s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephCluster{}, &cephv1.CephClusterList{})

	cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objects...).Build()
	clusterContext.Client = cl

	rotatedKeyJson := `[{"key":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=="}]`
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			if args[0] == "auth" && args[1] == "get-or-create-key" {
				return `{"key":"AQCvzWBeIV9lFRAAninzm+8XFxbSfTiPwoX50g=="}`, nil
			}
			if args[0] == "auth" && args[1] == "rotate" {
				t.Logf("rotating key and returning: %s", rotatedKeyJson)
				return rotatedKeyJson, nil
			}

			return "", nil
		},
	}
	clusterContext.Executor = executor

	k := keyring.GetSecretStore(clusterContext, clusterInfo, clusterInfo.OwnerInfo)
	key1, err := createCrashCollectorKeyring(k, clusterContext, clusterInfo)
	assert.NoError(t, err)
	assert.Equal(t, "AQCvzWBeIV9lFRAAninzm+8XFxbSfTiPwoX50g==", key1)

	// do key rotation
	err = clusterContext.Client.Get(ctx, clusterInfo.NamespacedName(), cephCluster)
	assert.NoError(t, err)
	cephCluster.Spec.Security.CephX.Daemon.KeyGeneration = 2
	err = cl.Update(ctx, cephCluster)
	assert.NoError(t, err)
	clusterInfo.CephVersion = cephver.CephVersion{Major: 20, Minor: 2, Extra: 0}
	key2, err := createCrashCollectorKeyring(k, clusterContext, clusterInfo)
	assert.NoError(t, err)
	assert.Equal(t, "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==", key2)

	// verify that the cephx status is updated
	// get the cluster again to ensure we have the latest status
	updatedCluster := &cephv1.CephCluster{}
	err = clusterContext.Client.Get(ctx, clusterInfo.NamespacedName(), updatedCluster)
	assert.NoError(t, err)
	assert.Equal(t, uint32(2), updatedCluster.Status.Cephx.CrashCollector.KeyGeneration)
}
