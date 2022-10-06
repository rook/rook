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

// Package nfs to manage a rook ceph nfs
package nfs

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/coreos/pkg/capnslog"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	testopk8s "github.com/rook/rook/pkg/operator/k8sutil/test"
	"github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var (
	name                      = "my-nfs"
	namespace                 = "rook-ceph"
	nfsCephAuthGetOrCreateKey = `{"key":"AQCvzWBeIV9lFRAAninzm+8XFxbSfTiPwoX50g=="}`
)

func TestCephNFSController(t *testing.T) {
	ctx := context.TODO()
	// Set DEBUG logging
	capnslog.SetGlobalLogLevel(capnslog.DEBUG)
	os.Setenv("ROOK_LOG_LEVEL", "DEBUG")

	// Register operator types with the runtime scheme.
	testScheme := scheme.Scheme
	testScheme.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephNFS{})
	testScheme.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephCluster{})

	baseExecutor := func() *exectest.MockExecutor {
		return &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				logger.Infof("mock execute: %s %v", command, args)
				if command == "ceph" && args[0] == "status" {
					return `{"fsid":"c47cac40-9bee-4d52-823b-ccd803ba5bfe","health":{"checks":{},"status":"HEALTH_ERR"},"pgmap":{"num_pgs":100,"pgs_by_state":[{"state_name":"active+clean","count":100}]}}`, nil
				}
				panic(fmt.Sprintf("unhandled command %s %v", command, args))
			},
		}
	}

	successExecutor := func(t *testing.T) *exectest.MockExecutor {
		t.Helper()

		return &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				logger.Infof("mock execute: %s %v", command, args)
				if command == "ceph" {
					if args[0] == "status" {
						return `{"fsid":"c47cac40-9bee-4d52-823b-ccd803ba5bfe","health":{"checks":{},"status":"HEALTH_OK"},"pgmap":{"num_pgs":100,"pgs_by_state":[{"state_name":"active+clean","count":100}]}}`, nil
					}
					if args[0] == "auth" && args[1] == "get-or-create-key" {
						return nfsCephAuthGetOrCreateKey, nil
					}
					if args[0] == "osd" && args[1] == "pool" && args[2] == "create" {
						return "", nil
					}
					if args[0] == "osd" && args[1] == "crush" && args[2] == "rule" {
						return "", nil
					}
					if args[0] == "osd" && args[1] == "pool" && args[2] == "application" {
						return "", nil
					}
				}
				panic(fmt.Sprintf("unhandled command %s %v", command, args))
			},
			MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
				logger.Infof("mock execute: %s %v", command, args)
				if command == "ganesha-rados-grace" {
					if args[4] == "add" {
						return "", nil
					}
					if args[4] == "remove" {
						return "", nil
					}
				}
				if command == "rados" {
					subc := args[4]
					switch subc {
					case "stat", "lock", "unlock":
						return "", nil
					}
					assert.Condition(t, func() bool {
						return stringInSlice("conf-nfs.my-nfs", args) ||
							stringInSlice("conf-nfs.nfs2", args) ||
							stringInSlice("kerberos", args)
					})
					return "", nil
				}
				panic(fmt.Sprintf("unhandled command %s %v", command, args))
			},
		}
	}

	newContext := func(executor *exectest.MockExecutor) *clusterd.Context {
		clientset := test.New(t, 3)
		return &clusterd.Context{
			Executor:      executor,
			RookClientset: rookclient.NewSimpleClientset(),
			Clientset:     clientset,
		}
	}

	baseCephNFS := func() *cephv1.CephNFS {
		return &cephv1.CephNFS{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: cephv1.NFSGaneshaSpec{
				RADOS: cephv1.GaneshaRADOSSpec{
					Pool:      "foo",
					Namespace: namespace,
				},
				Server: cephv1.GaneshaServerSpec{
					Active: 1,
				},
			},
			TypeMeta: controllerTypeMeta,
		}
	}

	cephClusterNotReady := func() *cephv1.CephCluster {
		return &cephv1.CephCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      namespace,
				Namespace: namespace,
			},
			Status: cephv1.ClusterStatus{
				Phase: "",
				CephStatus: &cephv1.CephStatus{
					Health: "",
				},
			},
		}
	}

	cephClusterReady := func(clusterCtx *clusterd.Context) *cephv1.CephCluster {
		cephCluster := cephClusterNotReady()
		cephCluster.Status.Phase = k8sutil.ReadyStatus
		cephCluster.Status.CephStatus.Health = "HEALTH_OK"

		// Create mock clusterInfo secret
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
		_, err := clusterCtx.Clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
		if err != nil {
			panic(fmt.Sprintf("should be no error: %v", err))
		}

		return cephCluster
	}

	newControllerClient := func(objects ...runtime.Object) client.WithWatch {
		return fake.NewClientBuilder().WithScheme(testScheme).WithRuntimeObjects(objects...).Build()
	}

	newReconcile := func(clusterCtx *clusterd.Context, cl client.WithWatch) *ReconcileCephNFS {
		// Create a ReconcileCephNFS object with the scheme and fake client.
		return &ReconcileCephNFS{client: cl, scheme: testScheme, context: clusterCtx, opManagerContext: ctx, recorder: record.NewFakeRecorder(5)}
	}

	// Mock request to simulate Reconcile() being called on an event for a
	// watched resource .
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
	}

	currentAndDesiredCephVersion = func(ctx context.Context, rookImage string, namespace string, jobName string, ownerInfo *k8sutil.OwnerInfo, context *clusterd.Context, cephClusterSpec *cephv1.ClusterSpec, clusterInfo *cephclient.ClusterInfo) (*version.CephVersion, *version.CephVersion, error) {
		return &version.Quincy, &version.Quincy, nil
	}

	t.Run("error - no ceph cluster", func(t *testing.T) {
		cCtx := newContext(baseExecutor())
		cl := newControllerClient(baseCephNFS())
		r := newReconcile(cCtx, cl)

		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.True(t, res.Requeue)
	})

	t.Run("error - ceph cluster not ready", func(t *testing.T) {
		cCtx := newContext(baseExecutor())
		cl := newControllerClient(baseCephNFS(), cephClusterNotReady())
		r := newReconcile(cCtx, cl)

		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.True(t, res.Requeue)
	})

	t.Run("error - security spec invalid", func(t *testing.T) {
		t.Run("security.sssd empty should error", func(t *testing.T) {
			cCtx := newContext(baseExecutor())
			nfs := baseCephNFS()
			nfs.Spec.Security = &cephv1.NFSSecuritySpec{
				SSSD: &cephv1.SSSDSpec{},
			}
			cl := newControllerClient(nfs, cephClusterNotReady())
			r := newReconcile(cCtx, cl)
			fakeRecorder := record.NewFakeRecorder(5)
			r.recorder = fakeRecorder

			res, err := r.Reconcile(ctx, req)
			assert.NoError(t, err)
			assert.True(t, res.Requeue)

			assert.Len(t, fakeRecorder.Events, 1)
			event := <-fakeRecorder.Events
			assert.Contains(t, event, // verify the security spec calls the Validate() method
				"System Security Services Daemon (SSSD) is enabled, but no runtime option is specified")
		})
	})

	assertCephNFSReady := func(t *testing.T, r *ReconcileCephNFS, names ...string) {
		t.Helper()

		if len(names) == 0 {
			// default to checking just the base CephNFS cluster
			names = []string{name}
		}

		for _, n := range names {
			cephNFS := &cephv1.CephNFS{}
			err := r.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: n}, cephNFS)
			assert.NoError(t, err)
			assert.Equal(t, "Ready", cephNFS.Status.Phase, cephNFS)
		}
	}

	assertResourcesExist := func(t *testing.T, cCtx *clusterd.Context, names ...string) {
		t.Helper()

		depNames := []string{}
		deps, err := cCtx.Clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
		assert.NoError(t, err)
		for _, dep := range deps.Items {
			depNames = append(depNames, dep.Name)
		}
		assert.ElementsMatch(t, names, depNames)

		svcNames := []string{}
		svcs, err := cCtx.Clientset.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
		assert.NoError(t, err)
		for _, dep := range svcs.Items {
			svcNames = append(svcNames, dep.Name)
		}
		assert.ElementsMatch(t, names, svcNames)

		cmNames := []string{}
		cms, err := cCtx.Clientset.CoreV1().ConfigMaps(namespace).List(ctx, metav1.ListOptions{})
		assert.NoError(t, err)
		for _, dep := range cms.Items {
			cmNames = append(cmNames, dep.Name)
		}
		assert.ElementsMatch(t, names, cmNames)
	}

	t.Run("run one nfs server", func(t *testing.T) {
		cCtx := newContext(successExecutor(t))
		cl := newControllerClient(baseCephNFS(), cephClusterReady(cCtx))
		r := newReconcile(cCtx, cl)

		t.Run("initial reconcile", func(t *testing.T) {
			res, err := r.Reconcile(ctx, req)
			assert.NoError(t, err)
			assert.False(t, res.Requeue)
			assertCephNFSReady(t, r)
			assertResourcesExist(t, cCtx, "rook-ceph-nfs-my-nfs-a")
		})

		t.Run("double reconcile", func(t *testing.T) {
			var deploymentsUpdated *[]*apps.Deployment
			updateDeploymentAndWait, deploymentsUpdated = testopk8s.UpdateDeploymentAndWaitStub()

			res, err := r.Reconcile(ctx, req)
			assert.NoError(t, err)
			assert.False(t, res.Requeue)
			assertCephNFSReady(t, r)
			assertResourcesExist(t, cCtx, "rook-ceph-nfs-my-nfs-a")
			assert.Len(t, *deploymentsUpdated, 1)
			assert.Equal(t, "rook-ceph-nfs-my-nfs-a", (*deploymentsUpdated)[0].Name)
		})
	})

	t.Run("run multiple nfs servers", func(t *testing.T) {
		cCtx := newContext(successExecutor(t))
		nfs := baseCephNFS()
		nfs.Spec.Server.Active = 3
		cl := newControllerClient(nfs, cephClusterReady(cCtx))
		r := newReconcile(cCtx, cl)

		t.Run("initial reconcile", func(t *testing.T) {
			res, err := r.Reconcile(ctx, req)
			assert.NoError(t, err)
			assert.False(t, res.Requeue)
			assertCephNFSReady(t, r)
			assertResourcesExist(t, cCtx, "rook-ceph-nfs-my-nfs-a", "rook-ceph-nfs-my-nfs-b", "rook-ceph-nfs-my-nfs-c")
		})

		t.Run("double reconcile", func(t *testing.T) {
			var deploymentsUpdated *[]*apps.Deployment
			updateDeploymentAndWait, deploymentsUpdated = testopk8s.UpdateDeploymentAndWaitStub()

			res, err := r.Reconcile(ctx, req)
			assert.NoError(t, err)
			assert.False(t, res.Requeue)
			assertCephNFSReady(t, r)
			assertResourcesExist(t, cCtx, "rook-ceph-nfs-my-nfs-a", "rook-ceph-nfs-my-nfs-b", "rook-ceph-nfs-my-nfs-c")
			assert.Len(t, *deploymentsUpdated, 3)
			assert.Equal(t, "rook-ceph-nfs-my-nfs-a", (*deploymentsUpdated)[0].Name)
			assert.Equal(t, "rook-ceph-nfs-my-nfs-b", (*deploymentsUpdated)[1].Name)
			assert.Equal(t, "rook-ceph-nfs-my-nfs-c", (*deploymentsUpdated)[2].Name)
		})
	})

	t.Run("scale down nfs servers", func(t *testing.T) {
		t.Run("scale from 3 to 2 servers", func(t *testing.T) {
			cCtx := newContext(successExecutor(t))
			nfs := baseCephNFS()
			nfs.Spec.Server.Active = 3
			cl := newControllerClient(nfs, cephClusterReady(cCtx))
			r := newReconcile(cCtx, cl)

			var deploymentsUpdated *[]*apps.Deployment
			updateDeploymentAndWait, deploymentsUpdated = testopk8s.UpdateDeploymentAndWaitStub()

			res, err := r.Reconcile(ctx, req)
			assert.NoError(t, err)
			assert.False(t, res.Requeue)
			assertCephNFSReady(t, r)
			assertResourcesExist(t, cCtx, "rook-ceph-nfs-my-nfs-a", "rook-ceph-nfs-my-nfs-b", "rook-ceph-nfs-my-nfs-c")

			err = cl.Get(ctx, client.ObjectKeyFromObject(nfs), nfs)
			assert.NoError(t, err)
			nfs.Spec.Server.Active = 2
			err = cl.Update(ctx, nfs, &client.UpdateOptions{})
			assert.NoError(t, err)

			res, err = r.Reconcile(ctx, req)
			assert.NoError(t, err)
			assert.False(t, res.Requeue)
			assertCephNFSReady(t, r)
			assertResourcesExist(t, cCtx, "rook-ceph-nfs-my-nfs-a", "rook-ceph-nfs-my-nfs-b")
			assert.Len(t, *deploymentsUpdated, 2)
			assert.Equal(t, "rook-ceph-nfs-my-nfs-a", (*deploymentsUpdated)[0].Name)
			assert.Equal(t, "rook-ceph-nfs-my-nfs-b", (*deploymentsUpdated)[1].Name)
		})

		t.Run("scale from 3 to 1 servers", func(t *testing.T) {
			cCtx := newContext(successExecutor(t))
			nfs := baseCephNFS()
			nfs.Spec.Server.Active = 3
			cl := newControllerClient(nfs, cephClusterReady(cCtx))
			r := newReconcile(cCtx, cl)

			var deploymentsUpdated *[]*apps.Deployment
			updateDeploymentAndWait, deploymentsUpdated = testopk8s.UpdateDeploymentAndWaitStub()

			res, err := r.Reconcile(ctx, req)
			assert.NoError(t, err)
			assert.False(t, res.Requeue)
			assertCephNFSReady(t, r)
			assertResourcesExist(t, cCtx, "rook-ceph-nfs-my-nfs-a", "rook-ceph-nfs-my-nfs-b", "rook-ceph-nfs-my-nfs-c")

			err = cl.Get(ctx, client.ObjectKeyFromObject(nfs), nfs)
			assert.NoError(t, err)
			nfs.Spec.Server.Active = 1
			err = cl.Update(ctx, nfs, &client.UpdateOptions{})
			assert.NoError(t, err)

			res, err = r.Reconcile(ctx, req)
			assert.NoError(t, err)
			assert.False(t, res.Requeue)
			assertCephNFSReady(t, r)
			assertResourcesExist(t, cCtx, "rook-ceph-nfs-my-nfs-a")
			assert.Len(t, *deploymentsUpdated, 1)
			assert.Equal(t, "rook-ceph-nfs-my-nfs-a", (*deploymentsUpdated)[0].Name)
		})
	})

	t.Run("multiple CephNFS clusters", func(t *testing.T) {
		// nfs1 - same config as other tests, 3 active
		nfs1 := baseCephNFS()
		nfs1.Spec.Server.Active = 3
		// nfs2 - change name to "nfs2", 2 active
		nfs2 := baseCephNFS()
		nfs2.Spec.Server.Active = 2
		nfs2.Name = "nfs2"

		cCtx := newContext(successExecutor(t))
		cl := newControllerClient(nfs1, nfs2, cephClusterReady(cCtx))
		r := newReconcile(cCtx, cl)

		req2 := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "nfs2",
				Namespace: namespace,
			},
		}

		t.Run("reconcile first CephNFS cluster", func(t *testing.T) {
			res, err := r.Reconcile(ctx, req)
			assert.NoError(t, err)
			assert.False(t, res.Requeue)
			assertCephNFSReady(t, r, "my-nfs") // first cluster should be ready
			assertResourcesExist(t, cCtx, "rook-ceph-nfs-my-nfs-a", "rook-ceph-nfs-my-nfs-b", "rook-ceph-nfs-my-nfs-c")
		})

		t.Run("reconcile second CephNFS cluster", func(t *testing.T) {
			res, err := r.Reconcile(ctx, req2)
			assert.NoError(t, err)
			assert.False(t, res.Requeue)
			assertCephNFSReady(t, r, "my-nfs", "nfs2") // both clusters should be ready
			// resources from first and second cluster should both exist
			assertResourcesExist(t, cCtx,
				"rook-ceph-nfs-my-nfs-a", "rook-ceph-nfs-my-nfs-b", "rook-ceph-nfs-my-nfs-c",
				"rook-ceph-nfs-nfs2-a", "rook-ceph-nfs-nfs2-b",
			)
		})

		t.Run("scale down first CephNFS cluster (3 to 1)", func(t *testing.T) {
			err := cl.Get(ctx, client.ObjectKeyFromObject(nfs1), nfs1)
			assert.NoError(t, err)
			nfs1.Spec.Server.Active = 1
			err = cl.Update(ctx, nfs1, &client.UpdateOptions{})
			assert.NoError(t, err)

			var deploymentsUpdated *[]*apps.Deployment
			updateDeploymentAndWait, deploymentsUpdated = testopk8s.UpdateDeploymentAndWaitStub()

			res, err := r.Reconcile(ctx, req)
			assert.NoError(t, err)
			assert.False(t, res.Requeue)

			assertCephNFSReady(t, r, "my-nfs", "nfs2")
			// one resource set should exist from first cluster, all should still exist for second cluster
			assertResourcesExist(t, cCtx,
				"rook-ceph-nfs-my-nfs-a",
				"rook-ceph-nfs-nfs2-a", "rook-ceph-nfs-nfs2-b",
			)
			assert.Len(t, *deploymentsUpdated, 1)
			assert.Equal(t, "rook-ceph-nfs-my-nfs-a", (*deploymentsUpdated)[0].Name)
		})
	})
}

func TestGetGaneshaConfigObject(t *testing.T) {
	cephNFS := &cephv1.CephNFS{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	expectedName := "conf-nfs.my-nfs"

	res := getGaneshaConfigObject(cephNFS)
	logger.Infof("Config Object for Pacific is %s", res)
	assert.Equal(t, expectedName, res)
}

func stringInSlice(str string, slice []string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}
