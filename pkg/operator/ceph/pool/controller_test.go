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
	"os"
	"testing"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	testop "github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestCreatePool(t *testing.T) {
	clusterInfo := client.AdminClusterInfo("mycluster")
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
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

	clusterSpec := &cephv1.ClusterSpec{Storage: cephv1.StorageScopeSpec{Config: map[string]string{cephclient.CrushRootConfigKey: "cluster-crush-root"}}}
	err := createPool(context, clusterInfo, clusterSpec, p)
	assert.Nil(t, err)

	// succeed with EC
	p.Spec.Replicated.Size = 0
	p.Spec.ErasureCoded.CodingChunks = 1
	p.Spec.ErasureCoded.DataChunks = 2
	err = createPool(context, clusterInfo, clusterSpec, p)
	assert.Nil(t, err)
}

func TestDeletePool(t *testing.T) {
	failOnDelete := false
	clusterInfo := cephclient.AdminClusterInfo("mycluster")
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			emptyPool := "{\"images\":{\"count\":0,\"provisioned_bytes\":0,\"snap_count\":0},\"trash\":{\"count\":1,\"provisioned_bytes\":2048,\"snap_count\":0}}"
			p := "{\"images\":{\"count\":1,\"provisioned_bytes\":1024,\"snap_count\":0},\"trash\":{\"count\":1,\"provisioned_bytes\":2048,\"snap_count\":0}}"
			logger.Infof("Command: %s %v", command, args)
			if command == "ceph" && args[1] == "lspools" {
				return `[{"poolnum":1,"poolname":"mypool"}]`, nil
			} else if command == "ceph" && args[1] == "pool" && args[2] == "get" {
				return `{"pool": "mypool","pool_id": 1,"size":1}`, nil
			} else if command == "ceph" && args[1] == "pool" && args[2] == "delete" {
				return "", nil
			} else if args[0] == "pool" {
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
	ctx := context.TODO()
	// Set DEBUG logging
	capnslog.SetGlobalLogLevel(capnslog.DEBUG)
	os.Setenv("ROOK_LOG_LEVEL", "DEBUG")
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
			UID:       types.UID("c47cac40-9bee-4d52-823b-ccd803ba5bfe"),
		},
		Spec: cephv1.PoolSpec{
			Replicated: cephv1.ReplicatedSpec{
				Size: replicas,
			},
			Mirroring: cephv1.MirroringSpec{
				Peers: &cephv1.MirroringPeerSpec{},
			},
			StatusCheck: cephv1.MirrorHealthCheckSpec{
				Mirror: cephv1.HealthCheckSpec{
					Disabled: true,
				},
			},
		},
		Status: &cephv1.CephBlockPoolStatus{
			Phase: "",
		},
	}

	// Objects to track in the fake client.
	object := []runtime.Object{
		pool,
	}

	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			if args[0] == "status" {
				return `{"fsid":"c47cac40-9bee-4d52-823b-ccd803ba5bfe","health":{"checks":{},"status":"HEALTH_ERR"},"pgmap":{"num_pgs":100,"pgs_by_state":[{"state_name":"active+clean","count":100}]}}`, nil
			}

			return "", nil
		},
	}
	c := &clusterd.Context{
		Executor:      executor,
		Clientset:     testop.New(t, 1),
		RookClientset: rookclient.NewSimpleClientset(),
	}

	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	s.AddKnownTypes(cephv1.SchemeGroupVersion, pool, &cephv1.CephClusterList{})

	// Create a fake client to mock API calls.
	cl := fake.NewClientBuilder().WithRuntimeObjects(object...).Build()

	// Create a ReconcileCephBlockPool object with the scheme and fake client.
	r := &ReconcileCephBlockPool{
		client:            cl,
		scheme:            s,
		context:           c,
		blockPoolContexts: make(map[string]*blockPoolHealth),
		opManagerContext:  context.TODO(),
	}

	// Mock request to simulate Reconcile() being called on an event for a
	// watched resource .
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
	}

	cephCluster := &cephv1.CephCluster{}

	t.Run("failure no CephCluster", func(t *testing.T) {
		// Create pool for updateCephBlockPoolStatus()
		_, err := c.RookClientset.CephV1().CephBlockPools(namespace).Create(ctx, pool, metav1.CreateOptions{})
		assert.NoError(t, err)
		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.True(t, res.Requeue)
	})

	t.Run("failure CephCluster not ready", func(t *testing.T) {
		cephCluster = &cephv1.CephCluster{
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
		_, err := c.RookClientset.CephV1().CephClusters(namespace).Create(ctx, cephCluster, metav1.CreateOptions{})
		assert.NoError(t, err)

		object = append(object, cephCluster)
		// Create a fake client to mock API calls.
		cl = fake.NewClientBuilder().WithRuntimeObjects(object...).Build()
		// Create a ReconcileCephBlockPool object with the scheme and fake client.
		r = &ReconcileCephBlockPool{
			client:            cl,
			scheme:            s,
			context:           c,
			blockPoolContexts: make(map[string]*blockPoolHealth),
			opManagerContext:  context.TODO(),
		}

		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.True(t, res.Requeue)
	})

	t.Run("failure CephCluster is ready", func(t *testing.T) {
		cephCluster.Status.Phase = cephv1.ConditionReady
		cephCluster.Status.CephStatus.Health = "HEALTH_OK"
		objects := []runtime.Object{
			pool,
			cephCluster,
		}
		// Create a fake client to mock API calls.
		cl = fake.NewClientBuilder().WithRuntimeObjects(objects...).Build()
		c.Client = cl

		executor = &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
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
		_, err := c.Clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
		assert.NoError(t, err)

		s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephBlockPoolList{})
		// Create a ReconcileCephBlockPool object with the scheme and fake client.
		r = &ReconcileCephBlockPool{
			client:            cl,
			scheme:            s,
			context:           c,
			blockPoolContexts: make(map[string]*blockPoolHealth),
			opManagerContext:  context.TODO(),
		}
		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.False(t, res.Requeue)

		err = r.client.Get(context.TODO(), req.NamespacedName, pool)
		assert.NoError(t, err)
		assert.Equal(t, cephv1.ConditionReady, pool.Status.Phase)
	})

	t.Run("failure no mirror mode", func(t *testing.T) {
		pool.Spec.Mirroring.Enabled = true
		err := r.client.Update(context.TODO(), pool)
		assert.NoError(t, err)
		res, err := r.Reconcile(ctx, req)
		assert.Error(t, err)
		assert.True(t, res.Requeue)
	})

	os.Setenv("POD_NAME", "test")
	defer os.Setenv("POD_NAME", "")
	os.Setenv("POD_NAMESPACE", namespace)
	defer os.Setenv("POD_NAMESPACE", "")
	p := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind: "ReplicaSet",
					Name: "testReplicaSet",
				},
			},
		},
	}
	// Create fake pod
	_, err := r.context.Clientset.CoreV1().Pods(namespace).Create(context.TODO(), p, metav1.CreateOptions{})
	assert.NoError(t, err)

	replicaSet := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testReplicaSet",
			Namespace: namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind: "Deployment",
				},
			},
		},
	}

	// Create fake replicaset
	_, err = r.context.Clientset.AppsV1().ReplicaSets(namespace).Create(context.TODO(), replicaSet, metav1.CreateOptions{})
	assert.NoError(t, err)

	t.Run("success - mirroring set", func(t *testing.T) {
		executor = &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				if args[0] == "mirror" && args[1] == "pool" && args[2] == "peer" && args[3] == "bootstrap" && args[4] == "create" {
					return `eyJmc2lkIjoiYzZiMDg3ZjItNzgyOS00ZGJiLWJjZmMtNTNkYzM0ZTBiMzVkIiwiY2xpZW50X2lkIjoicmJkLW1pcnJvci1wZWVyIiwia2V5IjoiQVFBV1lsWmZVQ1Q2RGhBQVBtVnAwbGtubDA5YVZWS3lyRVV1NEE9PSIsIm1vbl9ob3N0IjoiW3YyOjE5Mi4xNjguMTExLjEwOjMzMDAsdjE6MTkyLjE2OC4xMTEuMTA6Njc4OV0sW3YyOjE5Mi4xNjguMTExLjEyOjMzMDAsdjE6MTkyLjE2OC4xMTEuMTI6Njc4OV0sW3YyOjE5Mi4xNjguMTExLjExOjMzMDAsdjE6MTkyLjE2OC4xMTEuMTE6Njc4OV0ifQ==`, nil
				}
				return "", nil
			},
		}
		c.Executor = executor
		r = &ReconcileCephBlockPool{
			client:            cl,
			scheme:            s,
			context:           c,
			blockPoolContexts: make(map[string]*blockPoolHealth),
			opManagerContext:  context.TODO(),
		}

		pool.Spec.Mirroring.Mode = "image"
		pool.Spec.Mirroring.Peers.SecretNames = []string{}
		err = r.client.Update(context.TODO(), pool)
		assert.NoError(t, err)
		for i := 0; i < 5; i++ {
			res, err := r.Reconcile(ctx, req)
			assert.NoError(t, err)
			assert.False(t, res.Requeue)
			err = r.client.Get(context.TODO(), req.NamespacedName, pool)
			assert.NoError(t, err)
			assert.Equal(t, cephv1.ConditionReady, pool.Status.Phase)
			if _, ok := pool.Status.Info[opcontroller.RBDMirrorBootstrapPeerSecretName]; ok {
				break
			}
			logger.Infof("FIX: trying again to update the mirroring status")
		}
		assert.NotEmpty(t, pool.Status.Info[opcontroller.RBDMirrorBootstrapPeerSecretName], pool.Status.Info)

		// fetch the secret
		myPeerSecret, err := c.Clientset.CoreV1().Secrets(namespace).Get(ctx, pool.Status.Info[opcontroller.RBDMirrorBootstrapPeerSecretName], metav1.GetOptions{})
		assert.NoError(t, err)
		if myPeerSecret != nil {
			assert.NotEmpty(t, myPeerSecret.Data["token"], myPeerSecret.Data)
			assert.NotEmpty(t, myPeerSecret.Data["pool"])
		}
	})

	peerSecretName := "peer-secret"
	t.Run("failure - import peer token but was not created", func(t *testing.T) {
		// Create a fake client to mock API calls.
		cl = fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()

		// Create a ReconcileCephBlockPool object with the scheme and fake client.
		r = &ReconcileCephBlockPool{
			client:            cl,
			scheme:            s,
			context:           c,
			blockPoolContexts: make(map[string]*blockPoolHealth),
			opManagerContext:  context.TODO(),
		}

		pool.Spec.Mirroring.Peers.SecretNames = []string{peerSecretName}
		err := r.client.Update(context.TODO(), pool)
		assert.NoError(t, err)
		res, err := r.Reconcile(ctx, req)
		// assert reconcile failure because peer token secert was not created
		assert.Error(t, err)
		assert.True(t, res.Requeue)
	})

	t.Run("success - import peer token but was created", func(t *testing.T) {
		bootstrapPeerToken := `eyJmc2lkIjoiYzZiMDg3ZjItNzgyOS00ZGJiLWJjZmMtNTNkYzM0ZTBiMzVkIiwiY2xpZW50X2lkIjoicmJkLW1pcnJvci1wZWVyIiwia2V5IjoiQVFBV1lsWmZVQ1Q2RGhBQVBtVnAwbGtubDA5YVZWS3lyRVV1NEE9PSIsIm1vbl9ob3N0IjoiW3YyOjE5Mi4xNjguMTExLjEwOjMzMDAsdjE6MTkyLjE2OC4xMTEuMTA6Njc4OV0sW3YyOjE5Mi4xNjguMTExLjEyOjMzMDAsdjE6MTkyLjE2OC4xMTEuMTI6Njc4OV0sW3YyOjE5Mi4xNjguMTExLjExOjMzMDAsdjE6MTkyLjE2OC4xMTEuMTE6Njc4OV0ifQ==` //nolint:gosec // This is just a var name, not a real token
		peerSecret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      peerSecretName,
				Namespace: namespace,
			},
			Data: map[string][]byte{"token": []byte(bootstrapPeerToken), "pool": []byte("goo")},
			Type: k8sutil.RookType,
		}
		_, err := c.Clientset.CoreV1().Secrets(namespace).Create(ctx, peerSecret, metav1.CreateOptions{})
		assert.NoError(t, err)
		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.False(t, res.Requeue)
		err = r.client.Get(context.TODO(), req.NamespacedName, pool)
		assert.NoError(t, err)
	})

	t.Run("failure - mirroring disabled", func(t *testing.T) {
		r = &ReconcileCephBlockPool{
			client:            cl,
			scheme:            s,
			context:           c,
			blockPoolContexts: make(map[string]*blockPoolHealth),
			opManagerContext:  context.TODO(),
		}
		pool.Spec.Mirroring.Enabled = false
		pool.Spec.Mirroring.Mode = "image"
		err := r.client.Update(context.TODO(), pool)
		assert.NoError(t, err)
		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.False(t, res.Requeue)
		err = r.client.Get(context.TODO(), req.NamespacedName, pool)
		assert.NoError(t, err)
		assert.Equal(t, cephv1.ConditionReady, pool.Status.Phase)
		assert.Nil(t, pool.Status.MirroringStatus)
	})
}

func TestConfigureRBDStats(t *testing.T) {
	var (
		s         = runtime.NewScheme()
		context   = &clusterd.Context{}
		namespace = "rook-ceph"
	)

	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			logger.Infof("Command: %s %v", command, args)
			if args[0] == "config" && args[2] == "mgr." && args[3] == "mgr/prometheus/rbd_stats_pools" {
				if args[1] == "set" && args[4] != "" {
					return "", nil
				}
				if args[1] == "get" {
					return "", nil
				}
				if args[1] == "rm" {
					return "", nil
				}
			}
			return "", errors.Errorf("unexpected arguments %q", args)
		},
	}

	context.Executor = executor
	context.Client = fake.NewClientBuilder().WithScheme(s).Build()

	clusterInfo := cephclient.AdminClusterInfo(namespace)

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
	context.Client = fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objects...).Build()
	err = configureRBDStats(context, clusterInfo)
	assert.Nil(t, err)

	// Case 4: Two CephBlockPools with EnableRBDStats:false & EnableRBDStats:true.
	poolWithRBDStatsEnabled := poolWithRBDStatsDisabled.DeepCopy()
	poolWithRBDStatsEnabled.Name = "my-pool-with-rbd-stats"
	poolWithRBDStatsEnabled.Spec.EnableRBDStats = true
	objects = append(objects, poolWithRBDStatsEnabled)
	context.Client = fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objects...).Build()
	err = configureRBDStats(context, clusterInfo)
	assert.Nil(t, err)

	// Case 5: Two CephBlockPools with EnableRBDStats:false & EnableRBDStats:true.
	// SetConfig returns an error
	context.Executor = &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			logger.Infof("Command: %s %v", command, args)
			return "", errors.New("mock error to simulate failure of mon store Set() function")
		},
	}
	err = configureRBDStats(context, clusterInfo)
	assert.NotNil(t, err)
}
