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
	"testing"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/operator/k8sutil"

	"github.com/rook/rook/pkg/clusterd"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestValidatePool(t *testing.T) {
	context := &clusterd.Context{Executor: &exectest.MockExecutor{}}

	// not specifying some replication or EC settings is fine
	p := cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: "myns"}}
	err := ValidatePool(context, &p)
	assert.Nil(t, err)

	// must specify name
	p = cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Namespace: "myns"}}
	err = ValidatePool(context, &p)
	assert.NotNil(t, err)

	// must specify namespace
	p = cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool"}}
	err = ValidatePool(context, &p)
	assert.NotNil(t, err)

	// must not specify both replication and EC settings
	p = cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: "myns"}}
	p.Spec.Replicated.Size = 1
	p.Spec.Replicated.RequireSafeReplicaSize = false
	p.Spec.ErasureCoded.CodingChunks = 2
	p.Spec.ErasureCoded.DataChunks = 3
	err = ValidatePool(context, &p)
	assert.NotNil(t, err)

	// succeed with replication settings
	p = cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: "myns"}}
	p.Spec.Replicated.Size = 1
	p.Spec.Replicated.RequireSafeReplicaSize = false
	err = ValidatePool(context, &p)
	assert.Nil(t, err)

	// size is 1 and RequireSafeReplicaSize is true
	p = cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: "myns"}}
	p.Spec.Replicated.Size = 1
	p.Spec.Replicated.RequireSafeReplicaSize = true
	err = ValidatePool(context, &p)
	assert.Error(t, err)

	// succeed with ec settings
	p = cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: "myns"}}
	p.Spec.ErasureCoded.CodingChunks = 1
	p.Spec.ErasureCoded.DataChunks = 2
	err = ValidatePool(context, &p)
	assert.Nil(t, err)

}

func TestValidateCrushProperties(t *testing.T) {
	executor := &exectest.MockExecutor{}
	context := &clusterd.Context{Executor: executor}
	executor.MockExecuteCommandWithOutputFile = func(debug bool, actionName, command, outputFile string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[1] == "crush" && args[2] == "dump" {
			return `{"types":[{"type_id": 0,"name": "osd"}],"buckets":[{"id": -1,"name":"default"},{"id": -2,"name":"good"}]}`, nil
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}

	// succeed with a failure domain that exists
	p := &cephv1.CephBlockPool{
		ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: "myns"},
		Spec: cephv1.PoolSpec{
			Replicated:    cephv1.ReplicatedSpec{Size: 1, RequireSafeReplicaSize: false},
			FailureDomain: "osd",
		},
	}
	err := ValidatePool(context, p)
	assert.Nil(t, err)

	// fail with a failure domain that doesn't exist
	p.Spec.FailureDomain = "doesntexist"
	err = ValidatePool(context, p)
	assert.NotNil(t, err)

	// fail with a crush root that doesn't exist
	p.Spec.FailureDomain = "osd"
	p.Spec.CrushRoot = "bad"
	err = ValidatePool(context, p)
	assert.NotNil(t, err)

	// fail with a crush root that does exist
	p.Spec.CrushRoot = "good"
	err = ValidatePool(context, p)
	assert.Nil(t, err)
}

func TestCreatePool(t *testing.T) {
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(debug bool, actionName, command, outfile string, args ...string) (string, error) {
			if command == "ceph" && args[1] == "erasure-code-profile" {
				return `{"k":"2","m":"1","plugin":"jerasure","technique":"reed_sol_van"}`, nil
			}
			return "", nil
		},
	}
	context := &clusterd.Context{Executor: executor}

	p := &cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: "myns"}}
	p.Spec.Replicated.Size = 1
	p.Spec.Replicated.RequireSafeReplicaSize = false

	err := createPool(context, p)
	assert.Nil(t, err)

	// succeed with EC
	p.Spec.Replicated.Size = 0
	err = createPool(context, p)
	assert.Nil(t, err)
}

func TestDeletePool(t *testing.T) {
	failOnDelete := false
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(debug bool, actionName, command, outfile string, args ...string) (string, error) {
			if command == "ceph" && args[1] == "lspools" {
				return `[{"poolnum":1,"poolname":"mypool"}]`, nil
			} else if command == "ceph" && args[1] == "pool" && args[2] == "get" {
				return `{"pool": "mypool","pool_id": 1,"size":1}`, nil
			}

			return "", nil
		},
		MockExecuteCommandWithOutput: func(debug bool, actionName string, command string, args ...string) (string, error) {
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
	p := &cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: "myns"}}
	err := deletePool(context, p)
	assert.Nil(t, err)

	// succeed even if the pool doesn't exist
	p = &cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "otherpool", Namespace: "myns"}}
	err = deletePool(context, p)
	assert.Nil(t, err)

	// fail if images/snapshosts exist in the pool
	failOnDelete = true
	p = &cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "mypool", Namespace: "myns"}}
	err = deletePool(context, p)
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
		MockExecuteCommandWithOutputFile: func(debug bool, actionName, command, outfile string, args ...string) (string, error) {
			if args[0] == "status" {
				return `{"pgmap":{"num_pgs":100,"pgs_by_state":[{"state_name":"active+clean","count":100}]}}`, nil
			}

			return "", nil
		},
	}
	context := &clusterd.Context{
		Executor:      executor,
		RookClientset: rookclient.NewSimpleClientset()}

	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	s.AddKnownTypes(cephv1.SchemeGroupVersion, pool)

	// Create a fake client to mock API calls.
	cl := fake.NewFakeClient(object...)
	// Create a ReconcileCephBlockPool object with the scheme and fake client.
	r := &ReconcileCephBlockPool{client: cl, scheme: s, context: context}

	// Mock request to simulate Reconcile() being called on an event for a
	// watched resource .
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
	}

	// Create pool for updateCephBlockPoolStatus()
	_, err := context.RookClientset.CephV1().CephBlockPools(namespace).Create(pool)
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
		},
	}
	s.AddKnownTypes(cephv1.SchemeGroupVersion, cephCluster)

	// Create CephCluster for updateCephBlockPoolStatus()
	_, err = context.RookClientset.CephV1().CephClusters(namespace).Create(cephCluster)
	assert.NoError(t, err)

	object = append(object, cephCluster)
	// Create a fake client to mock API calls.
	cl = fake.NewFakeClient(object...)
	// Create a ReconcileCephBlockPool object with the scheme and fake client.
	r = &ReconcileCephBlockPool{client: cl, scheme: s, context: context}

	assert.True(t, res.Requeue)

	//
	// TEST 3:
	//
	// SUCCESS! The CephCluster is ready
	//
	cephCluster.Status.Phase = k8sutil.ReadyStatus
	objects := []runtime.Object{
		pool,
		cephCluster,
	}
	// Create a fake client to mock API calls.
	cl = fake.NewFakeClient(objects...)
	// Create a ReconcileCephBlockPool object with the scheme and fake client.
	r = &ReconcileCephBlockPool{client: cl, scheme: s, context: context}

	res, err = r.Reconcile(req)
	assert.NoError(t, err)
	assert.False(t, res.Requeue)

	// Get the updated CephBlockPool object.
	pool, err = context.RookClientset.CephV1().CephBlockPools(namespace).Get(name, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Equal(t, "Ready", pool.Status.Phase)
}
