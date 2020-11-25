/*
Copyright 2016 The Rook Authors. All rights reserved.

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

package pool

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/k8sutil"
	testop "github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"github.com/tevino/abool"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestValidatePool(t *testing.T) {
	context := &clusterd.Context{Executor: &exectest.MockExecutor{}}
	clusterInfo := &cephclient.ClusterInfo{Namespace: "myns"}

	// not specifying some replication or EC settings is fine
	p := cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: clusterInfo.Namespace}}
	err := ValidatePool(context, clusterInfo, &p)
	assert.Nil(t, err)

	// must specify name
	p = cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Namespace: clusterInfo.Namespace}}
	err = ValidatePool(context, clusterInfo, &p)
	assert.NotNil(t, err)

	// must specify namespace
	p = cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool"}}
	err = ValidatePool(context, clusterInfo, &p)
	assert.NotNil(t, err)

	// must not specify both replication and EC settings
	p = cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: clusterInfo.Namespace}}
	p.Spec.Replicated.Size = 1
	p.Spec.Replicated.RequireSafeReplicaSize = false
	p.Spec.ErasureCoded.CodingChunks = 2
	p.Spec.ErasureCoded.DataChunks = 3
	err = ValidatePool(context, clusterInfo, &p)
	assert.NotNil(t, err)

	// succeed with replication settings
	p = cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: clusterInfo.Namespace}}
	p.Spec.Replicated.Size = 1
	p.Spec.Replicated.RequireSafeReplicaSize = false
	err = ValidatePool(context, clusterInfo, &p)
	assert.Nil(t, err)

	// size is 1 and RequireSafeReplicaSize is true
	p = cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: clusterInfo.Namespace}}
	p.Spec.Replicated.Size = 1
	p.Spec.Replicated.RequireSafeReplicaSize = true
	err = ValidatePool(context, clusterInfo, &p)
	assert.Error(t, err)

	// succeed with ec settings
	p = cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: clusterInfo.Namespace}}
	p.Spec.ErasureCoded.CodingChunks = 1
	p.Spec.ErasureCoded.DataChunks = 2
	err = ValidatePool(context, clusterInfo, &p)
	assert.Nil(t, err)

	// Tests with various compression modes
	// succeed with compression mode "none"
	p = cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: clusterInfo.Namespace}}
	p.Spec.Replicated.Size = 1
	p.Spec.Replicated.RequireSafeReplicaSize = false
	p.Spec.CompressionMode = "none"
	err = ValidatePool(context, clusterInfo, &p)
	assert.Nil(t, err)

	// succeed with compression mode "aggressive"
	p = cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: clusterInfo.Namespace}}
	p.Spec.Replicated.Size = 1
	p.Spec.Replicated.RequireSafeReplicaSize = false
	p.Spec.CompressionMode = "aggressive"
	err = ValidatePool(context, clusterInfo, &p)
	assert.Nil(t, err)

	// fail with compression mode "unsupported"
	p = cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: clusterInfo.Namespace}}
	p.Spec.Replicated.Size = 1
	p.Spec.Replicated.RequireSafeReplicaSize = false
	p.Spec.CompressionMode = "unsupported"
	err = ValidatePool(context, clusterInfo, &p)
	assert.Error(t, err)

	// succeed with ec pool and valid compression mode
	p = cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: clusterInfo.Namespace}}
	p.Spec.ErasureCoded.CodingChunks = 1
	p.Spec.ErasureCoded.DataChunks = 2
	p.Spec.CompressionMode = "passive"
	err = ValidatePool(context, clusterInfo, &p)
	assert.Nil(t, err)
}

func TestValidateCrushProperties(t *testing.T) {
	executor := &exectest.MockExecutor{}
	context := &clusterd.Context{Executor: executor}
	clusterInfo := &cephclient.ClusterInfo{Namespace: "myns"}
	executor.MockExecuteCommandWithOutputFile = func(command, outputFile string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[1] == "crush" && args[2] == "dump" {
			return `{"types":[{"type_id": 0,"name": "osd"}],"buckets":[{"id": -1,"name":"default"},{"id": -2,"name":"good"}]}`, nil
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}

	// succeed with a failure domain that exists
	p := &cephv1.CephBlockPool{
		ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: clusterInfo.Namespace},
		Spec: cephv1.PoolSpec{
			Replicated:    cephv1.ReplicatedSpec{Size: 1, RequireSafeReplicaSize: false},
			FailureDomain: "osd",
		},
	}
	err := ValidatePool(context, clusterInfo, p)
	assert.Nil(t, err)

	// fail with a failure domain that doesn't exist
	p.Spec.FailureDomain = "doesntexist"
	err = ValidatePool(context, clusterInfo, p)
	assert.NotNil(t, err)

	// fail with a crush root that doesn't exist
	p.Spec.FailureDomain = "osd"
	p.Spec.CrushRoot = "bad"
	err = ValidatePool(context, clusterInfo, p)
	assert.NotNil(t, err)

	// fail with a crush root that does exist
	p.Spec.CrushRoot = "good"
	err = ValidatePool(context, clusterInfo, p)
	assert.Nil(t, err)
}

func TestCreatePool(t *testing.T) {
	clusterInfo := &cephclient.ClusterInfo{Namespace: "myns"}
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(command, outfile string, args ...string) (string, error) {
			logger.Infof("Command: %s %v", command, args)
			if command == "ceph" && args[1] == "erasure-code-profile" {
				return `{"k":"2","m":"1","plugin":"jerasure","technique":"reed_sol_van"}`, nil
			}
			return "", nil
		},
	}
	context := &clusterd.Context{Executor: executor}

	p := &cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: clusterInfo.Namespace}}
	p.Spec.Replicated.Size = 1
	p.Spec.Replicated.RequireSafeReplicaSize = false

	err := createPool(context, clusterInfo, p)
	assert.Nil(t, err)

	// succeed with EC
	p.Spec.Replicated.Size = 0
	p.Spec.ErasureCoded.CodingChunks = 1
	p.Spec.ErasureCoded.DataChunks = 2
	err = createPool(context, clusterInfo, p)
	assert.Nil(t, err)
}

func TestDeletePool(t *testing.T) {
	failOnDelete := false
	clusterInfo := &cephclient.ClusterInfo{Namespace: "myns"}
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(command, outfile string, args ...string) (string, error) {
			if command == "ceph" && args[1] == "lspools" {
				return `[{"poolnum":1,"poolname":"mypool"}]`, nil
			} else if command == "ceph" && args[1] == "pool" && args[2] == "get" {
				return `{"pool": "mypool","pool_id": 1,"size":1}`, nil
			}

			return "", nil
		},
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			emptyPool := "{\"images\":{\"count\":0,\"provisioned_bytes\":0,\"snap_count\":0},\"trash\":{\"count\":1,\"provisioned_bytes\":2048,\"snap_count\":0}}"
			p := "{\"images\":{\"count\":1,\"provisioned_bytes\":1024,\"snap_count\":0},\"trash\":{\"count\":1,\"provisioned_bytes\":2048,\"snap_count\":0}}"
			logger.Infof("Command: %s %v", command, args)

			if args[0] == "pool" {
				if args[1] == "stats" {
					if !failOnDelete {
						return emptyPool, nil
					}

					return p, nil

				}
				return "", errors.Errorf("rbd: error opening pool %q: (2) No such file or directory", args[3])

			}
			return "", errors.Errorf("unexpected rbd command %q", args)
		},
	}
	context := &clusterd.Context{Executor: executor}

	// delete a pool that exists
	p := &cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: clusterInfo.Namespace}}
	err := deletePool(context, clusterInfo, p)
	assert.Nil(t, err)

	// succeed even if the pool doesn't exist
	p = &cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "otherpool", Namespace: clusterInfo.Namespace}}
	err = deletePool(context, clusterInfo, p)
	assert.Nil(t, err)

	// fail if images/snapshosts exist in the pool
	failOnDelete = true
	p = &cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: clusterInfo.Namespace}}
	err = deletePool(context, clusterInfo, p)
	assert.NotNil(t, err)
}

// TestCephBlockPoolController runs ReconcileCephBlockPool.Reconcile() against a
// fake client that tracks a CephBlockPool object.
func TestCephBlockPoolController(t *testing.T) {
	//
	// TEST 1 SETUP
	//
	// FAILURE because no CephCluster
	//
	var (
		name           = "my-pool"
		namespace      = "rook-ceph"
		replicas  uint = 3
	)

	// A Pool resource with metadata and spec.
	pool := &cephv1.CephBlockPool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: cephv1.PoolSpec{
			Replicated: cephv1.ReplicatedSpec{
				Size: replicas,
			},
		},
		Status: &cephv1.Status{
			Phase: "",
		},
	}

	// Objects to track in the fake client.
	object := []runtime.Object{
		pool,
	}

	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(command, outfile string, args ...string) (string, error) {
			if args[0] == "status" {
				return `{"fsid":"c47cac40-9bee-4d52-823b-ccd803ba5bfe","health":{"checks":{},"status":"HEALTH_ERR"},"pgmap":{"num_pgs":100,"pgs_by_state":[{"state_name":"active+clean","count":100}]}}`, nil
			}

			return "", nil
		},
	}
	c := &clusterd.Context{
		Executor:                   executor,
		Clientset:                  testop.New(t, 1),
		RookClientset:              rookclient.NewSimpleClientset(),
		RequestCancelOrchestration: abool.New(),
	}

	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	s.AddKnownTypes(cephv1.SchemeGroupVersion, pool, &cephv1.CephClusterList{})

	// Create a fake client to mock API calls.
	cl := fake.NewFakeClient(object...)
	// Create a ReconcileCephBlockPool object with the scheme and fake client.
	r := &ReconcileCephBlockPool{client: cl, scheme: s, context: c}

	// Mock request to simulate Reconcile() being called on an event for a
	// watched resource .
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
	}

	// Create pool for updateCephBlockPoolStatus()
	_, err := c.RookClientset.CephV1().CephBlockPools(namespace).Create(pool)
	assert.NoError(t, err)
	res, err := r.Reconcile(req)
	assert.NoError(t, err)
	assert.True(t, res.Requeue)

	//
	// TEST 2:
	//
	// FAILURE we have a cluster but it's not ready
	//
	cephCluster := &cephv1.CephCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namespace,
			Namespace: namespace,
		},
		Status: cephv1.ClusterStatus{
			Phase: "",
			CephVersion: &cephv1.ClusterVersion{
				Version: "14.2.9-0",
			},
			CephStatus: &cephv1.CephStatus{
				Health: "",
			},
		},
	}

	s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephCluster{}, &cephv1.CephClusterList{})

	// Create CephCluster for updateCephBlockPoolStatus()
	_, err = c.RookClientset.CephV1().CephClusters(namespace).Create(cephCluster)
	assert.NoError(t, err)

	object = append(object, cephCluster)
	// Create a fake client to mock API calls.
	cl = fake.NewFakeClient(object...)
	// Create a ReconcileCephBlockPool object with the scheme and fake client.
	r = &ReconcileCephBlockPool{client: cl, scheme: s, context: c}

	assert.True(t, res.Requeue)

	//
	// TEST 3:
	//
	// SUCCESS! The CephCluster is ready
	//
	cephCluster.Status.Phase = k8sutil.ReadyStatus
	cephCluster.Status.CephStatus.Health = "HEALTH_OK"

	objects := []runtime.Object{
		pool,
		cephCluster,
	}
	// Create a fake client to mock API calls.
	cl = fake.NewFakeClient(objects...)
	c.Client = cl

	executor = &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(command, outfile string, args ...string) (string, error) {
			if args[0] == "status" {
				return `{"fsid":"c47cac40-9bee-4d52-823b-ccd803ba5bfe","health":{"checks":{},"status":"HEALTH_OK"},"pgmap":{"num_pgs":100,"pgs_by_state":[{"state_name":"active+clean","count":100}]}}`, nil
			}
			if args[0] == "config" && args[2] == "mgr." && args[3] == "mgr/prometheus/rbd_stats_pools" {
				return "", nil
			}

			return "", nil
		},
	}
	c.Executor = executor

	// Mock clusterInfo
	secrets := map[string][]byte{
		"fsid":         []byte(name),
		"mon-secret":   []byte("monsecret"),
		"admin-secret": []byte("adminsecret"),
	}
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rook-ceph-mon",
			Namespace: namespace,
		},
		Data: secrets,
		Type: k8sutil.RookType,
	}
	_, err = c.Clientset.CoreV1().Secrets(namespace).Create(secret)
	assert.NoError(t, err)

	s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephBlockPoolList{})
	// Create a ReconcileCephBlockPool object with the scheme and fake client.
	r = &ReconcileCephBlockPool{client: cl, scheme: s, context: c}

	res, err = r.Reconcile(req)
	assert.NoError(t, err)
	assert.False(t, res.Requeue)

	err = r.client.Get(context.TODO(), req.NamespacedName, pool)
	assert.NoError(t, err)
	assert.Equal(t, "Ready", pool.Status.Phase)
}

func TestConfigureRBDStats(t *testing.T) {
	var (
		s         = runtime.NewScheme()
		context   = &clusterd.Context{}
		namespace = "rook-ceph"
	)

	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(command, outfile string, args ...string) (string, error) {
			logger.Infof("Command: %s %v", command, args)
			switch {
			case args[0] == "config" && args[1] == "set" && args[2] == "mgr." && args[3] == "mgr/prometheus/rbd_stats_pools" && args[4] != "":
				return "", nil
			case args[0] == "config" && args[1] == "get" && args[2] == "mgr." && args[3] == "mgr/prometheus/rbd_stats_pools":
				return "", nil
			case args[0] == "config" && args[1] == "rm" && args[2] == "mgr." && args[3] == "mgr/prometheus/rbd_stats_pools":
				return "", nil
			}
			return "", errors.Errorf("unexpected arguments %q", args)
		},
	}

	context.Executor = executor
	context.Client = fake.NewFakeClientWithScheme(s)
	clusterInfo := &cephclient.ClusterInfo{Namespace: namespace}

	// Case 1: CephBlockPoolList is not registered in scheme.
	// So, an error is expected as List() operation would fail.
	err := configureRBDStats(context, clusterInfo)
	assert.NotNil(t, err)

	s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephBlockPoolList{})
	// Case 2: CephBlockPoolList is registered in schema.
	// So, no error is expected.
	err = configureRBDStats(context, clusterInfo)
	assert.Nil(t, err)

	s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephBlockPool{})
	// A Pool resource with metadata and spec.
	poolWithRBDStatsDisabled := &cephv1.CephBlockPool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-pool-without-rbd-stats",
			Namespace: namespace,
		},
		Spec: cephv1.PoolSpec{
			Replicated: cephv1.ReplicatedSpec{
				Size: 3,
			},
		},
	}

	// Case 3: One CephBlockPool with EnableRBDStats:false (default).
	objects := []runtime.Object{
		poolWithRBDStatsDisabled,
	}
	context.Client = fake.NewFakeClientWithScheme(s, objects...)
	err = configureRBDStats(context, clusterInfo)
	assert.Nil(t, err)

	// Case 4: Two CephBlockPools with EnableRBDStats:false & EnableRBDStats:true.
	poolWithRBDStatsEnabled := poolWithRBDStatsDisabled.DeepCopy()
	poolWithRBDStatsEnabled.Name = "my-pool-with-rbd-stats"
	poolWithRBDStatsEnabled.Spec.EnableRBDStats = true
	objects = append(objects, poolWithRBDStatsEnabled)
	context.Client = fake.NewFakeClientWithScheme(s, objects...)
	err = configureRBDStats(context, clusterInfo)
	assert.Nil(t, err)

	// Case 5: Two CephBlockPools with EnableRBDStats:false & EnableRBDStats:true.
	// MgrSetConfig returns an error
	context.Executor = &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(command, outfile string, args ...string) (string, error) {
			logger.Infof("Command: %s %v", command, args)
			return "", errors.Errorf("mock error to simulate failure of MgrSetConfig() function")
		},
	}
	err = configureRBDStats(context, clusterInfo)
	assert.NotNil(t, err)
}
