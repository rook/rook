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
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/config"
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
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestCreatePool(t *testing.T) {
	p := &cephv1.NamedPoolSpec{}
	enabledMetricsApp := false
	enabledMgrApp := false
	clusterInfo := cephclient.AdminTestClusterInfo("mycluster")
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
			logger.Infof("CommandTimeout: %s %v", command, args)
			if command == "rbd" {
				if args[0] == "pool" && args[1] == "init" {
					// assert that `rbd pool init` is only run when application is set to `rbd`
					assert.Equal(t, "rbd", p.Application)
					assert.Equal(t, p.Name, args[2])
					return "{}", nil
				}
			}
			return "", nil
		},
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			logger.Infof("Command: %s %v", command, args)
			if command == "ceph" {
				if args[1] == "erasure-code-profile" {
					return `{"k":"2","m":"1","plugin":"jerasure","technique":"reed_sol_van"}`, nil
				}
				if args[0] == "osd" && args[1] == "pool" && args[2] == "application" {
					if args[3] == "get" {
						return ``, nil
					}
					assert.Equal(t, "enable", args[3])
					if args[5] != "rbd" {
						if args[4] == ".mgr" {
							enabledMgrApp = true
							assert.Equal(t, ".mgr", args[4])
							assert.Equal(t, "mgr", args[5])
						} else {
							fmt.Printf("pool - %v", args)
							assert.Fail(t, fmt.Sprintf("invalid pool %q", args[4]))
						}
					}
				}
			}
			if command == "rbd" {
				if args[0] == "mirror" && args[2] == "info" {
					return "{}", nil
				} else if args[0] == "mirror" && args[2] == "disable" {
					return "", nil
				}
			}
			return "", nil
		},
	}

	context := &clusterd.Context{Executor: executor}

	clusterSpec := &cephv1.ClusterSpec{Storage: cephv1.StorageScopeSpec{Config: map[string]string{cephclient.CrushRootConfigKey: "cluster-crush-root"}}}

	t.Run("replicated pool", func(t *testing.T) {
		p.Name = "replicapool"
		p.Replicated.Size = 1
		p.Replicated.RequireSafeReplicaSize = false
		// reset the application name
		p.Application = ""
		err := createPool(context, clusterInfo, clusterSpec, p)
		assert.Nil(t, err)
		assert.False(t, enabledMetricsApp)
	})

	t.Run("built-in mgr pool", func(t *testing.T) {
		p.Name = ".mgr"
		// reset the application name
		p.Application = ""
		err := createPool(context, clusterInfo, clusterSpec, p)
		assert.Nil(t, err)
		assert.True(t, enabledMgrApp)
	})

	t.Run("ec pool", func(t *testing.T) {
		p.Name = "ecpool"
		p.Replicated.Size = 0
		p.ErasureCoded.CodingChunks = 1
		p.ErasureCoded.DataChunks = 2
		// reset the application name
		p.Application = ""
		err := createPool(context, clusterInfo, clusterSpec, p)
		assert.Nil(t, err)
	})
}

func TestCephPoolName(t *testing.T) {
	t.Run("spec not set", func(t *testing.T) {
		p := cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "metapool"}}
		name := p.ToNamedPoolSpec().Name
		assert.Equal(t, "metapool", name)
	})
	t.Run("same name already set", func(t *testing.T) {
		p := cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "metapool"}, Spec: cephv1.NamedBlockPoolSpec{Name: "metapool"}}
		name := p.ToNamedPoolSpec().Name
		assert.Equal(t, "metapool", name)
	})
	t.Run("override mgr", func(t *testing.T) {
		p := cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "default-mgr"}, Spec: cephv1.NamedBlockPoolSpec{Name: ".mgr"}}
		name := p.ToNamedPoolSpec().Name
		assert.Equal(t, ".mgr", name)
	})
	t.Run("override nfs", func(t *testing.T) {
		p := cephv1.CephBlockPool{ObjectMeta: metav1.ObjectMeta{Name: "default-nfs"}, Spec: cephv1.NamedBlockPoolSpec{Name: ".nfs"}}
		name := p.ToNamedPoolSpec().Name
		assert.Equal(t, ".nfs", name)
	})
}

func TestDeletePool(t *testing.T) {
	failOnDelete := false
	clusterInfo := cephclient.AdminTestClusterInfo("mycluster")
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
	p := &cephv1.NamedPoolSpec{Name: "mypool"}
	err := deletePool(context, clusterInfo, p)
	assert.NoError(t, err)

	// succeed even if the pool doesn't exist
	p = &cephv1.NamedPoolSpec{Name: "otherpool"}
	err = deletePool(context, clusterInfo, p)
	assert.Nil(t, err)

	// fail if images/snapshosts exist in the pool
	failOnDelete = true
	p = &cephv1.NamedPoolSpec{Name: "mypool"}
	err = deletePool(context, clusterInfo, p)
	assert.Error(t, err)
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
			Name:       name,
			Namespace:  namespace,
			UID:        types.UID("c47cac40-9bee-4d52-823b-ccd803ba5bfe"),
			Finalizers: []string{"cephblockpool.ceph.rook.io"},
		},
		TypeMeta: metav1.TypeMeta{
			Kind: "CephBlockPool",
		},
		Spec: cephv1.NamedBlockPoolSpec{
			PoolSpec: cephv1.PoolSpec{
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
		recorder:          record.NewFakeRecorder(5),
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
			recorder:          record.NewFakeRecorder(5),
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
				if args[0] == "config" && args[2] == "mgr" && args[3] == "mgr/prometheus/rbd_stats_pools" {
					return "", nil
				}
				if args[0] == "mirror" && args[1] == "pool" && args[2] == "info" {
					return "{}", nil
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
			recorder:          record.NewFakeRecorder(5),
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
		assert.NoError(t, err)
		assert.True(t, res.Requeue)
	})

	t.Setenv("POD_NAME", "test")
	t.Setenv("POD_NAMESPACE", namespace)
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
				if args[0] == "mirror" && args[1] == "pool" && args[2] == "info" {
					return "{}", nil
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
			recorder:          record.NewFakeRecorder(5),
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
			recorder:          record.NewFakeRecorder(5),
		}

		pool.Spec.Mirroring.Peers.SecretNames = []string{peerSecretName}
		err := r.client.Update(context.TODO(), pool)
		assert.NoError(t, err)
		res, err := r.Reconcile(ctx, req)
		// assert reconcile failure because peer token secret was not created
		assert.NoError(t, err)
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
			recorder:          record.NewFakeRecorder(5),
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

func TestDeletionBlocked(t *testing.T) {
	// A Pool resource with metadata and spec.
	pool := &cephv1.CephBlockPool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pool",
			Namespace: "ns",
		},
		TypeMeta: metav1.TypeMeta{
			Kind: "CephBlockPool",
		},
		Status: &cephv1.CephBlockPoolStatus{},
	}
	object := []runtime.Object{pool}
	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	s.AddKnownTypes(cephv1.SchemeGroupVersion,
		&cephv1.CephBlockPoolList{},
		&cephv1.CephBlockPoolRadosNamespaceList{},
		&cephv1.CephBlockPool{},
		&cephv1.CephBlockPoolRadosNamespace{},
	)
	blockImageCount := 0
	rnsImageCount := 0
	cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			logger.Infof("Command: %s %v", command, args)
			if args[0] == "pool" {
				if args[1] == "stats" {
					response := "{\"images\":{\"count\":%d,\"provisioned_bytes\":0,\"snap_count\":0},\"trash\":{\"count\":1,\"provisioned_bytes\":2048,\"snap_count\":0}}"
					if args[4] == "--namespace" {
						return fmt.Sprintf(response, rnsImageCount), nil
					} else {
						return fmt.Sprintf(response, blockImageCount), nil
					}
				}
			}
			return "not implemented", nil
		},
	}
	c := &clusterd.Context{
		Executor:      executor,
		Clientset:     testop.New(t, 1),
		RookClientset: rookclient.NewSimpleClientset(),
	}
	// Create a ReconcileCephBlockPool object with the scheme and fake client.
	r := &ReconcileCephBlockPool{
		client:            cl,
		scheme:            s,
		context:           c,
		blockPoolContexts: make(map[string]*blockPoolHealth),
		opManagerContext:  context.TODO(),
		recorder:          record.NewFakeRecorder(5),
		clusterInfo:       cephclient.AdminTestClusterInfo("mycluster"),
	}
	cephCluster := &cephv1.CephCluster{}
	t.Run("deletion is allowed with no images or rns", func(t *testing.T) {
		err := r.handleDeletionBlocked(pool, cephCluster)
		assert.NoError(t, err)

		result := &cephv1.CephBlockPool{}
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: pool.Name, Namespace: pool.Namespace}, result)
		assert.NoError(t, err)
		assert.Equal(t, pool.Name, result.Name)
		assert.Equal(t, pool.Namespace, result.Namespace)
		assert.Equal(t, 2, len(pool.Status.Conditions))
		assert.Equal(t, "PoolDeletionIsBlocked", string(pool.Status.Conditions[0].Type))
		assert.Equal(t, v1.ConditionFalse, pool.Status.Conditions[0].Status)
		assert.Equal(t, "PoolEmpty", string(pool.Status.Conditions[0].Reason))
		assert.Equal(t, "DeletionIsBlocked", string(pool.Status.Conditions[1].Type))
		assert.Equal(t, v1.ConditionFalse, pool.Status.Conditions[1].Status)
		assert.Equal(t, "ObjectHasNoDependents", string(pool.Status.Conditions[1].Reason))
	})
	t.Run("image prevents deletion", func(t *testing.T) {
		blockImageCount = 1
		err := r.handleDeletionBlocked(pool, cephCluster)
		assert.Error(t, err)

		result := &cephv1.CephBlockPool{}
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: pool.Name, Namespace: pool.Namespace}, result)
		assert.NoError(t, err)
		assert.Equal(t, pool.Name, result.Name)
		assert.Equal(t, pool.Namespace, result.Namespace)
		assert.Equal(t, 2, len(pool.Status.Conditions))
		assert.Equal(t, "PoolDeletionIsBlocked", string(pool.Status.Conditions[0].Type))
		assert.Equal(t, v1.ConditionTrue, pool.Status.Conditions[0].Status)
		assert.Equal(t, "PoolNotEmpty", string(pool.Status.Conditions[0].Reason))
		assert.Equal(t, "DeletionIsBlocked", string(pool.Status.Conditions[1].Type))
		assert.Equal(t, v1.ConditionFalse, pool.Status.Conditions[1].Status)
		assert.Equal(t, "ObjectHasNoDependents", string(pool.Status.Conditions[1].Reason))
	})
	t.Run("rados namespace prevents deletion", func(t *testing.T) {
		radosNamespace := &cephv1.CephBlockPoolRadosNamespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-rns",
				Namespace: pool.Namespace,
			},
			Spec: cephv1.CephBlockPoolRadosNamespaceSpec{
				BlockPoolName: pool.Name,
			},
		}
		_, err := c.RookClientset.CephV1().CephBlockPoolRadosNamespaces(pool.Namespace).Create(context.TODO(), radosNamespace, metav1.CreateOptions{})
		assert.NoError(t, err)

		// A radosnamespaces prevents deletion of the pool
		blockImageCount = 0
		rnsImageCount = 1
		err = r.handleDeletionBlocked(pool, cephCluster)
		assert.Error(t, err)

		result := &cephv1.CephBlockPool{}
		err = r.client.Get(context.TODO(), types.NamespacedName{Name: pool.Name, Namespace: pool.Namespace}, result)
		assert.NoError(t, err)
		assert.Equal(t, pool.Name, result.Name)
		assert.Equal(t, pool.Namespace, result.Namespace)
		assert.Equal(t, 2, len(pool.Status.Conditions))
		assert.Equal(t, "PoolDeletionIsBlocked", string(pool.Status.Conditions[0].Type))
		assert.Equal(t, v1.ConditionTrue, pool.Status.Conditions[0].Status)
		assert.Equal(t, "PoolNotEmpty", string(pool.Status.Conditions[0].Reason))
		assert.Equal(t, "DeletionIsBlocked", string(pool.Status.Conditions[1].Type))
		assert.Equal(t, v1.ConditionTrue, pool.Status.Conditions[1].Status)
		assert.Equal(t, "ObjectHasDependents", string(pool.Status.Conditions[1].Reason))
	})
}

func TestIsAnyRadosNamespaceMirrored(t *testing.T) {
	pool := "test"
	object := []runtime.Object{}
	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()
	executor := &exectest.MockExecutor{}
	c := &clusterd.Context{
		Executor:      executor,
		Clientset:     testop.New(t, 1),
		RookClientset: rookclient.NewSimpleClientset(),
	}
	// Create a ReconcileCephBlockPool object with the scheme and fake client.
	r := &ReconcileCephBlockPool{
		client:            cl,
		scheme:            s,
		context:           c,
		blockPoolContexts: make(map[string]*blockPoolHealth),
		opManagerContext:  context.TODO(),
		recorder:          record.NewFakeRecorder(5),
		clusterInfo:       cephclient.AdminTestClusterInfo("mycluster"),
	}

	t.Run("rados namespace mirroring enabled", func(t *testing.T) {
		r.context.Executor = &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				if args[0] == "namespace" {
					assert.Equal(t, pool, args[2])
					return `[{"name":"abc"},{"name":"abc1"},{"name":"abc3"}]`, nil
				}
				if args[0] == "mirror" && args[1] == "pool" && args[2] == "info" {
					return "{}", nil
				}
				return "", nil
			},
		}
		enabled, err := r.isAnyRadosNamespaceMirrored(pool)
		assert.NoError(t, err)
		assert.True(t, enabled)
	})

	t.Run("rados namespace mirroring disabled", func(t *testing.T) {
		r.context.Executor = &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				if args[0] == "namespace" {
					assert.Equal(t, pool, args[2])
					return `[]`, nil
				}
				if args[0] == "mirror" && args[1] == "pool" && args[2] == "info" {
					return "{}", nil
				}
				return "", nil
			},
		}
		enabled, err := r.isAnyRadosNamespaceMirrored(pool)
		assert.NoError(t, err)
		assert.False(t, enabled)
	})
}

func TestConfigureRBDStats(t *testing.T) {
	var (
		s         = runtime.NewScheme()
		context   = &clusterd.Context{}
		namespace = "rook-ceph"
	)

	mockedPools := ""
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
			logger.Infof("Command: %s %v", command, args)
			if args[0] == "config" && args[2] == "mgr" && args[3] == "mgr/prometheus/rbd_stats_pools" {
				if args[1] == "set" {
					mockedPools = args[4]
					return "", nil
				}
				if args[1] == "get" {
					return mockedPools, nil
				}
				if args[1] == "rm" {
					mockedPools = ""
					return "", nil
				}
			}
			return "", errors.Errorf("unexpected arguments %q", args)
		},
	}

	context.Executor = executor
	context.Client = fake.NewClientBuilder().WithScheme(s).Build()

	clusterInfo := cephclient.AdminTestClusterInfo(namespace)

	// Case 1: CephBlockPoolList is not registered in scheme.
	// So, an error is expected as List() operation would fail.
	err := configureRBDStats(context, clusterInfo, "")
	assert.NotNil(t, err)

	s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephBlockPoolList{})
	// Case 2: CephBlockPoolList is registered in schema.
	// So, no error is expected.
	err = configureRBDStats(context, clusterInfo, "")
	assert.Nil(t, err)

	s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephBlockPool{})
	// A Pool resource with metadata and spec.
	poolWithRBDStatsDisabled := &cephv1.CephBlockPool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-pool-without-rbd-stats",
			Namespace: namespace,
		},
		Spec: cephv1.NamedBlockPoolSpec{
			PoolSpec: cephv1.PoolSpec{
				Replicated: cephv1.ReplicatedSpec{
					Size: 3,
				},
			},
		},
	}

	// Case 3: One CephBlockPool with EnableRBDStats:false (default).
	objects := []runtime.Object{
		poolWithRBDStatsDisabled,
	}
	context.Client = fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objects...).Build()
	err = configureRBDStats(context, clusterInfo, "")
	assert.Nil(t, err)

	// Case 4: Two CephBlockPools with EnableRBDStats:false & EnableRBDStats:true.
	poolWithRBDStatsEnabled := poolWithRBDStatsDisabled.DeepCopy()
	poolWithRBDStatsEnabled.Name = "my-pool-with-rbd-stats"
	poolWithRBDStatsEnabled.Spec.EnableRBDStats = true
	objects = append(objects, poolWithRBDStatsEnabled)
	context.Client = fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objects...).Build()
	err = configureRBDStats(context, clusterInfo, "")
	assert.Nil(t, err)

	// Case 5: Two external pools(non CephBlockPools) with rbd_stats_pools config already set
	monStore := config.GetMonStore(context, clusterInfo)
	e := monStore.Set("mgr", "mgr/prometheus/rbd_stats_pools", "pool1,pool2")
	assert.Nil(t, e)
	e = configureRBDStats(context, clusterInfo, "")
	assert.Nil(t, e)

	rbdStatsPools, err := monStore.Get("mgr", "mgr/prometheus/rbd_stats_pools")
	assert.Nil(t, err)
	assert.Equal(t, "my-pool-with-rbd-stats,pool1,pool2", rbdStatsPools)

	// Case 6: Deleted CephBlockPool should be excluded from config
	err = configureRBDStats(context, clusterInfo, "my-pool-with-rbd-stats")
	assert.Nil(t, err)

	rbdStatsPools, err = monStore.Get("mgr", "mgr/prometheus/rbd_stats_pools")
	assert.Nil(t, err)
	assert.Equal(t, "pool1,pool2", rbdStatsPools)

	// Case 7: Duplicate entries should be removed from config
	e = monStore.Set("mgr", "mgr/prometheus/rbd_stats_pools", "pool1,pool2,pool1")
	assert.Nil(t, e)
	err = configureRBDStats(context, clusterInfo, "")
	assert.Nil(t, err)

	rbdStatsPools, err = monStore.Get("mgr", "mgr/prometheus/rbd_stats_pools")
	assert.Nil(t, err)
	assert.Equal(t, "my-pool-with-rbd-stats,pool1,pool2", rbdStatsPools)

	// Case 8: Two CephBlockPools with EnableRBDStats:false & EnableRBDStats:true.
	// SetConfig returns an error
	context.Executor = &exectest.MockExecutor{
		MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
			logger.Infof("Command: %s %v", command, args)
			return "", errors.New("mock error to simulate failure of mon store Set() function")
		},
	}
	err = configureRBDStats(context, clusterInfo, "")
	assert.NotNil(t, err)
}

func TestGenerateStatsPoolList(t *testing.T) {
	tests := []struct {
		name               string
		existingStatsPools []string
		rookStatsPools     []string
		removePools        []string
		expectedOutput     string
	}{
		// Basic cases
		{
			name:               "Empty lists",
			existingStatsPools: []string{},
			rookStatsPools:     []string{},
			removePools:        []string{},
			expectedOutput:     "",
		},
		{
			name:               "Single-item lists, no removal",
			existingStatsPools: []string{"p1"},
			rookStatsPools:     []string{"p2"},
			removePools:        []string{},
			expectedOutput:     "p1,p2",
		},
		// Overlap and duplicates
		{
			name:               "Overlapping pools, some to remove",
			existingStatsPools: []string{"p1", "p2", "p3"},
			rookStatsPools:     []string{"p2", "p4", "p5"},
			removePools:        []string{"p1", "p5"},
			expectedOutput:     "p2,p3,p4",
		},
		{
			name:               "Non-overlapping lists",
			existingStatsPools: []string{"p1", "p2"},
			rookStatsPools:     []string{"p3", "p4"},
			removePools:        []string{},
			expectedOutput:     "p1,p2,p3,p4",
		},
		// All pools removed
		{
			name:               "All pools removed",
			existingStatsPools: []string{"p1", "p2"},
			rookStatsPools:     []string{"p2", "p3"},
			removePools:        []string{"p1", "p2", "p3"},
			expectedOutput:     "",
		},
		// Mixed scenarios with edge cases
		{
			name:               "Only removed pools",
			existingStatsPools: []string{"p1", "p2"},
			rookStatsPools:     []string{"p2", "p3"},
			removePools:        []string{"p1", "p2", "p3", "p4"},
			expectedOutput:     "",
		},
		{
			name:               "Duplicate pools across lists",
			existingStatsPools: []string{"p1", "p2", "p1"},
			rookStatsPools:     []string{"p2", "p3", "p3"},
			removePools:        []string{},
			expectedOutput:     "p1,p2,p3",
		},
		{
			name:               "Empty string in pools",
			existingStatsPools: []string{"p1", ""},
			rookStatsPools:     []string{"p2", ""},
			removePools:        []string{""},
			expectedOutput:     "p1,p2",
		},
		{
			name:               "Empty string in remove pools",
			existingStatsPools: []string{"p1", "p2"},
			rookStatsPools:     []string{"p3", "p4"},
			removePools:        []string{""},
			expectedOutput:     "p1,p2,p3,p4",
		},
		{
			name:               "All lists empty strings",
			existingStatsPools: []string{""},
			rookStatsPools:     []string{""},
			removePools:        []string{""},
			expectedOutput:     "",
		},
		// Larger cases
		{
			name:               "Large unique pool list",
			existingStatsPools: []string{"p1", "p2", "p3", "p4"},
			rookStatsPools:     []string{"p5", "p6", "p7", "p8"},
			removePools:        []string{"p1", "p8"},
			expectedOutput:     "p2,p3,p4,p5,p6,p7",
		},
		{
			name:               "Large list with many duplicates",
			existingStatsPools: []string{"p1", "p2", "p3", "p2", "p1"},
			rookStatsPools:     []string{"p2", "p3", "p3", "p4", "p1"},
			removePools:        []string{"p4"},
			expectedOutput:     "p1,p2,p3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateStatsPoolList(tt.existingStatsPools, tt.rookStatsPools, tt.removePools)
			assert.Equal(t, tt.expectedOutput, result)
		})
	}
}
