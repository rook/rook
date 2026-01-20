/*
Copyright 2025 The Rook Authors. All rights reserved.

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

package nvmeof

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
	"github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	ini "gopkg.in/ini.v1"
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
	name      = "my-nvmeof"
	namespace = "rook-ceph"
)

func TestCephNVMeOFGatewayController(t *testing.T) {
	ctx := context.TODO()
	// Set DEBUG logging
	capnslog.SetGlobalLogLevel(capnslog.DEBUG)
	os.Setenv("ROOK_LOG_LEVEL", "DEBUG")

	// Register operator types with the runtime scheme.
	testScheme := scheme.Scheme
	testScheme.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephNVMeOFGateway{})
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
					if args[0] == "nvme-gw" && args[1] == "create" {
						return "", nil
					}
					if args[0] == "nvme-gw" && args[1] == "show" {
						return "", nil
					}
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

	baseCephNVMeOFGateway := func() *cephv1.CephNVMeOFGateway {
		return &cephv1.CephNVMeOFGateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:       name,
				Namespace:  namespace,
				Finalizers: []string{"cephnvmeofgateway.ceph.rook.io"},
			},
			Spec: cephv1.NVMeOFGatewaySpec{
				Pool:      "nvmeof",
				Group:     "group-a",
				Instances: 1,
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

	newReconcile := func(clusterCtx *clusterd.Context, cl client.WithWatch) *ReconcileCephNVMeOFGateway {
		return &ReconcileCephNVMeOFGateway{client: cl, scheme: testScheme, context: clusterCtx, opManagerContext: ctx, recorder: record.NewFakeRecorder(5)}
	}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
	}

	currentAndDesiredCephVersion = func(ctx context.Context, rookImage string, namespace string, jobName string, ownerInfo *k8sutil.OwnerInfo, context *clusterd.Context, cephClusterSpec *cephv1.ClusterSpec, clusterInfo *cephclient.ClusterInfo) (*version.CephVersion, *version.CephVersion, error) {
		return &version.Squid, &version.Squid, nil
	}

	t.Run("error - no ceph cluster", func(t *testing.T) {
		cCtx := newContext(baseExecutor())
		cl := newControllerClient(baseCephNVMeOFGateway())
		r := newReconcile(cCtx, cl)

		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.Greater(t, res.RequeueAfter, time.Duration(0))
	})

	t.Run("error - ceph cluster not ready", func(t *testing.T) {
		cCtx := newContext(baseExecutor())
		cl := newControllerClient(baseCephNVMeOFGateway(), cephClusterNotReady())
		r := newReconcile(cCtx, cl)

		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.Greater(t, res.RequeueAfter, time.Duration(0))
	})

	t.Run("error - invalid gateway spec", func(t *testing.T) {
		t.Run("instances less than 1 should error", func(t *testing.T) {
			cCtx := newContext(successExecutor(t))
			nvmeof := baseCephNVMeOFGateway()
			nvmeof.Spec.Instances = 0
			cl := newControllerClient(nvmeof, cephClusterReady(cCtx))
			r := newReconcile(cCtx, cl)
			fakeRecorder := record.NewFakeRecorder(5)
			r.recorder = fakeRecorder

			res, err := r.Reconcile(ctx, req)
			assert.Error(t, err)
			assert.Equal(t, res.RequeueAfter, time.Duration(0))
			assert.Contains(t, err.Error(), "invalid configuration")

			assert.Len(t, fakeRecorder.Events, 1)
			event := <-fakeRecorder.Events
			assert.Contains(t, event, "ReconcileFailed")
			assert.Contains(t, event, "invalid configuration")
		})

		t.Run("group empty should error", func(t *testing.T) {
			cCtx := newContext(successExecutor(t))
			nvmeof := baseCephNVMeOFGateway()
			nvmeof.Spec.Group = ""
			cl := newControllerClient(nvmeof, cephClusterReady(cCtx))
			r := newReconcile(cCtx, cl)
			fakeRecorder := record.NewFakeRecorder(5)
			r.recorder = fakeRecorder

			res, err := r.Reconcile(ctx, req)
			assert.Error(t, err)
			assert.Equal(t, res.RequeueAfter, time.Duration(0))
			assert.Contains(t, err.Error(), "invalid configuration")

			assert.Len(t, fakeRecorder.Events, 1)
			event := <-fakeRecorder.Events
			assert.Contains(t, event, "ReconcileFailed")
			assert.Contains(t, event, "invalid configuration")
		})
	})

	assertCephNVMeOFGatewayReady := func(t *testing.T, r *ReconcileCephNVMeOFGateway, names ...string) {
		t.Helper()

		if len(names) == 0 {
			names = []string{name}
		}

		for _, n := range names {
			cephNVMeOFGateway := &cephv1.CephNVMeOFGateway{}
			err := r.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: n}, cephNVMeOFGateway)
			assert.NoError(t, err)
			assert.Equal(t, "Ready", cephNVMeOFGateway.Status.Phase, cephNVMeOFGateway)
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
	}

	t.Run("run one nvmeof gateway", func(t *testing.T) {
		cCtx := newContext(successExecutor(t))
		cl := newControllerClient(baseCephNVMeOFGateway(), cephClusterReady(cCtx))
		r := newReconcile(cCtx, cl)

		t.Run("initial reconcile", func(t *testing.T) {
			res, err := r.Reconcile(ctx, req)
			assert.NoError(t, err)
			assert.Equal(t, res.RequeueAfter, time.Duration(0))
			assertCephNVMeOFGatewayReady(t, r)
			assertResourcesExist(t, cCtx, "rook-ceph-nvmeof-my-nvmeof-a")
		})

		t.Run("double reconcile", func(t *testing.T) {
			res, err := r.Reconcile(ctx, req)
			assert.NoError(t, err)
			assert.Equal(t, res.RequeueAfter, time.Duration(0))
			assertCephNVMeOFGatewayReady(t, r)
			assertResourcesExist(t, cCtx, "rook-ceph-nvmeof-my-nvmeof-a")
		})
	})

	t.Run("run multiple nvmeof gateways", func(t *testing.T) {
		cCtx := newContext(successExecutor(t))
		nvmeof := baseCephNVMeOFGateway()
		nvmeof.Spec.Instances = 3
		cl := newControllerClient(nvmeof, cephClusterReady(cCtx))
		r := newReconcile(cCtx, cl)

		t.Run("initial reconcile", func(t *testing.T) {
			res, err := r.Reconcile(ctx, req)
			assert.NoError(t, err)
			assert.Equal(t, res.RequeueAfter, time.Duration(0))
			assertCephNVMeOFGatewayReady(t, r)
			assertResourcesExist(t, cCtx, "rook-ceph-nvmeof-my-nvmeof-a", "rook-ceph-nvmeof-my-nvmeof-b", "rook-ceph-nvmeof-my-nvmeof-c")
		})

		t.Run("double reconcile", func(t *testing.T) {
			res, err := r.Reconcile(ctx, req)
			assert.NoError(t, err)
			assert.Equal(t, res.RequeueAfter, time.Duration(0))
			assertCephNVMeOFGatewayReady(t, r)
			assertResourcesExist(t, cCtx, "rook-ceph-nvmeof-my-nvmeof-a", "rook-ceph-nvmeof-my-nvmeof-b", "rook-ceph-nvmeof-my-nvmeof-c")
		})
	})

	t.Run("scale down nvmeof gateways", func(t *testing.T) {
		t.Run("scale from 3 to 2 gateways", func(t *testing.T) {
			cCtx := newContext(successExecutor(t))
			nvmeof := baseCephNVMeOFGateway()
			nvmeof.Spec.Instances = 3
			cl := newControllerClient(nvmeof, cephClusterReady(cCtx))
			r := newReconcile(cCtx, cl)

			res, err := r.Reconcile(ctx, req)
			assert.NoError(t, err)
			assert.Equal(t, res.RequeueAfter, time.Duration(0))
			assertCephNVMeOFGatewayReady(t, r)
			assertResourcesExist(t, cCtx, "rook-ceph-nvmeof-my-nvmeof-a", "rook-ceph-nvmeof-my-nvmeof-b", "rook-ceph-nvmeof-my-nvmeof-c")

			err = cl.Get(ctx, client.ObjectKeyFromObject(nvmeof), nvmeof)
			assert.NoError(t, err)
			nvmeof.Spec.Instances = 2
			err = cl.Update(ctx, nvmeof, &client.UpdateOptions{})
			assert.NoError(t, err)

			res, err = r.Reconcile(ctx, req)
			assert.NoError(t, err)
			assert.Equal(t, res.RequeueAfter, time.Duration(0))
			assertCephNVMeOFGatewayReady(t, r)
			assertResourcesExist(t, cCtx, "rook-ceph-nvmeof-my-nvmeof-a", "rook-ceph-nvmeof-my-nvmeof-b")
		})

		t.Run("scale from 3 to 1 gateway", func(t *testing.T) {
			cCtx := newContext(successExecutor(t))
			nvmeof := baseCephNVMeOFGateway()
			nvmeof.Spec.Instances = 3
			cl := newControllerClient(nvmeof, cephClusterReady(cCtx))
			r := newReconcile(cCtx, cl)

			res, err := r.Reconcile(ctx, req)
			assert.NoError(t, err)
			assert.Equal(t, res.RequeueAfter, time.Duration(0))
			assertCephNVMeOFGatewayReady(t, r)
			assertResourcesExist(t, cCtx, "rook-ceph-nvmeof-my-nvmeof-a", "rook-ceph-nvmeof-my-nvmeof-b", "rook-ceph-nvmeof-my-nvmeof-c")

			err = cl.Get(ctx, client.ObjectKeyFromObject(nvmeof), nvmeof)
			assert.NoError(t, err)
			nvmeof.Spec.Instances = 1
			err = cl.Update(ctx, nvmeof, &client.UpdateOptions{})
			assert.NoError(t, err)

			res, err = r.Reconcile(ctx, req)
			assert.NoError(t, err)
			assert.Equal(t, res.RequeueAfter, time.Duration(0))
			assertCephNVMeOFGatewayReady(t, r)
			assertResourcesExist(t, cCtx, "rook-ceph-nvmeof-my-nvmeof-a")
		})
	})

	t.Run("multiple CephNVMeOFGateway clusters", func(t *testing.T) {
		nvmeof1 := baseCephNVMeOFGateway()
		nvmeof1.Spec.Instances = 3
		nvmeof2 := baseCephNVMeOFGateway()
		nvmeof2.Spec.Instances = 2
		nvmeof2.Name = "nvmeof2"

		cCtx := newContext(successExecutor(t))
		cl := newControllerClient(nvmeof1, nvmeof2, cephClusterReady(cCtx))
		r := newReconcile(cCtx, cl)

		req2 := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      "nvmeof2",
				Namespace: namespace,
			},
		}

		t.Run("reconcile first CephNVMeOFGateway cluster", func(t *testing.T) {
			res, err := r.Reconcile(ctx, req)
			assert.NoError(t, err)
			assert.Equal(t, res.RequeueAfter, time.Duration(0))
			assertCephNVMeOFGatewayReady(t, r, "my-nvmeof") // first cluster should be ready
			assertResourcesExist(t, cCtx, "rook-ceph-nvmeof-my-nvmeof-a", "rook-ceph-nvmeof-my-nvmeof-b", "rook-ceph-nvmeof-my-nvmeof-c")
		})

		t.Run("reconcile second CephNVMeOFGateway cluster", func(t *testing.T) {
			res, err := r.Reconcile(ctx, req2)
			assert.NoError(t, err)
			assert.Equal(t, res.RequeueAfter, time.Duration(0))
			assertCephNVMeOFGatewayReady(t, r, "my-nvmeof", "nvmeof2")
			assertResourcesExist(t, cCtx,
				"rook-ceph-nvmeof-my-nvmeof-a", "rook-ceph-nvmeof-my-nvmeof-b", "rook-ceph-nvmeof-my-nvmeof-c",
				"rook-ceph-nvmeof-nvmeof2-a", "rook-ceph-nvmeof-nvmeof2-b",
			)
		})

		t.Run("scale down first CephNVMeOFGateway cluster (3 to 1)", func(t *testing.T) {
			err := cl.Get(ctx, client.ObjectKeyFromObject(nvmeof1), nvmeof1)
			assert.NoError(t, err)
			nvmeof1.Spec.Instances = 1
			err = cl.Update(ctx, nvmeof1, &client.UpdateOptions{})
			assert.NoError(t, err)

			res, err := r.Reconcile(ctx, req)
			assert.NoError(t, err)
			assert.Equal(t, res.RequeueAfter, time.Duration(0))

			assertCephNVMeOFGatewayReady(t, r, "my-nvmeof", "nvmeof2")
			assertResourcesExist(t, cCtx,
				"rook-ceph-nvmeof-my-nvmeof-a",
				"rook-ceph-nvmeof-nvmeof2-a", "rook-ceph-nvmeof-nvmeof2-b",
			)
		})
	})
}

func TestNVMeOFKeyRotation(t *testing.T) {
	ctx := context.TODO()
	var (
		name      = "my-nvmeof"
		namespace = "rook-ceph"
	)
	// Set DEBUG logging
	t.Setenv("ROOK_LOG_LEVEL", "DEBUG")

	// Mock version to avoid deployment updates (filesystem mirror pattern)
	currentAndDesiredCephVersion = func(ctx context.Context, rookImage string, namespace string, jobName string, ownerInfo *k8sutil.OwnerInfo, context *clusterd.Context, cephClusterSpec *cephv1.ClusterSpec, clusterInfo *cephclient.ClusterInfo) (*version.CephVersion, *version.CephVersion, error) {
		return &version.CephVersion{Major: 20, Minor: 2, Extra: 0}, &version.CephVersion{Major: 20, Minor: 2, Extra: 0}, nil
	}

	nvmeof := &cephv1.CephNVMeOFGateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: cephv1.NVMeOFGatewaySpec{
			Pool:      "nvmeof",
			Group:     "group-a",
			Instances: 1,
		},
		TypeMeta: controllerTypeMeta,
	}

	cephCluster := &cephv1.CephCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namespace,
			Namespace: namespace,
		},
		Spec: cephv1.ClusterSpec{
			Security: cephv1.ClusterSecuritySpec{
				CephX: cephv1.ClusterCephxConfig{
					Daemon: cephv1.CephxConfig{},
				},
			},
		},
		Status: cephv1.ClusterStatus{
			Phase: k8sutil.ReadyStatus,
			CephStatus: &cephv1.CephStatus{
				Health: "HEALTH_OK",
			},
		},
	}

	object := []runtime.Object{
		nvmeof,
		cephCluster,
	}

	s := scheme.Scheme
	s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephNVMeOFGateway{})
	s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephCluster{})

	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			if command == "ceph" {
				if args[0] == "status" {
					return `{"fsid":"c47cac40-9bee-4d52-823b-ccd803ba5bfe","health":{"checks":{},"status":"HEALTH_OK"},"pgmap":{"num_pgs":100,"pgs_by_state":[{"state_name":"active+clean","count":100}]}}`, nil
				}
				if args[0] == "nvme-gw" && args[1] == "create" {
					return "", nil
				}
				if args[0] == "nvme-gw" && args[1] == "show" {
					return "", nil
				}
			}
			return "", nil
		},
	}
	clientset := test.New(t, 3)
	c := &clusterd.Context{
		Executor:      executor,
		RookClientset: rookclient.NewSimpleClientset(),
		Clientset:     clientset,
	}

	cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()
	r := &ReconcileCephNVMeOFGateway{client: cl, scheme: s, context: c, opManagerContext: ctx, recorder: record.NewFakeRecorder(5)}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
	}

	t.Run("first reconcile", func(t *testing.T) {
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

		_, err = r.Reconcile(ctx, req)
		assert.NoError(t, err)

		nvmeofResult := cephv1.CephNVMeOFGateway{}
		err = cl.Get(ctx, req.NamespacedName, &nvmeofResult)
		assert.NoError(t, err)
		assert.Equal(t, uint32(1), nvmeofResult.Status.Cephx.Daemon.KeyGeneration)
		assert.Equal(t, "20.2.0-0", nvmeofResult.Status.Cephx.Daemon.KeyCephVersion)
	})

	t.Run("subsequent reconcile - retain cephx status", func(t *testing.T) {
		r := &ReconcileCephNVMeOFGateway{client: cl, scheme: s, context: c, opManagerContext: ctx, recorder: record.NewFakeRecorder(5)}
		_, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		nvmeofResult := cephv1.CephNVMeOFGateway{}
		err = cl.Get(ctx, req.NamespacedName, &nvmeofResult)
		assert.NoError(t, err)
		assert.Equal(t, uint32(1), nvmeofResult.Status.Cephx.Daemon.KeyGeneration)
		assert.Equal(t, "20.2.0-0", nvmeofResult.Status.Cephx.Daemon.KeyCephVersion)
	})

	t.Run("brownfield reconcile - retain unknown cephx status", func(t *testing.T) {
		nvmeofResult := cephv1.CephNVMeOFGateway{}
		err := cl.Get(ctx, req.NamespacedName, &nvmeofResult)
		assert.NoError(t, err)
		nvmeofResult.Status.Cephx.Daemon = cephv1.CephxStatus{}
		err = cl.Update(ctx, &nvmeofResult)
		assert.NoError(t, err)

		_, err = r.Reconcile(ctx, req)
		assert.NoError(t, err)

		err = cl.Get(ctx, req.NamespacedName, &nvmeofResult)
		assert.NoError(t, err)
		assert.Equal(t, cephv1.CephxStatus{}, nvmeofResult.Status.Cephx.Daemon)
	})

	t.Run("rotate key - brownfield unknown status becomes known", func(t *testing.T) {
		cluster := cephv1.CephCluster{}
		err := cl.Get(ctx, types.NamespacedName{Namespace: namespace, Name: namespace}, &cluster)
		assert.NoError(t, err)
		cluster.Spec.Security.CephX.Daemon = cephv1.CephxConfig{
			KeyRotationPolicy: "KeyGeneration",
			KeyGeneration:     2,
		}
		err = cl.Update(ctx, &cluster)
		assert.NoError(t, err)

		_, err = r.Reconcile(ctx, req)
		assert.NoError(t, err)

		nvmeofResult := cephv1.CephNVMeOFGateway{}
		err = cl.Get(ctx, req.NamespacedName, &nvmeofResult)
		assert.NoError(t, err)
		assert.Equal(t, uint32(2), nvmeofResult.Status.Cephx.Daemon.KeyGeneration)
		assert.Equal(t, "20.2.0-0", nvmeofResult.Status.Cephx.Daemon.KeyCephVersion)
	})

	t.Run("brownfield reconcile - no further rotation happens", func(t *testing.T) {
		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.Equal(t, res.RequeueAfter, time.Duration(0))

		nvmeofResult := cephv1.CephNVMeOFGateway{}
		err = cl.Get(ctx, req.NamespacedName, &nvmeofResult)
		assert.NoError(t, err)
		assert.Equal(t, uint32(2), nvmeofResult.Status.Cephx.Daemon.KeyGeneration)
		assert.Equal(t, "20.2.0-0", nvmeofResult.Status.Cephx.Daemon.KeyCephVersion)
	})

	t.Run("rotate key - cephx status updated", func(t *testing.T) {
		cluster := cephv1.CephCluster{}
		err := cl.Get(ctx, types.NamespacedName{Namespace: namespace, Name: namespace}, &cluster)
		assert.NoError(t, err)
		cluster.Spec.Security.CephX.Daemon = cephv1.CephxConfig{
			KeyRotationPolicy: "KeyGeneration",
			KeyGeneration:     3,
		}
		err = cl.Update(ctx, &cluster)
		assert.NoError(t, err)

		_, err = r.Reconcile(ctx, req)
		assert.NoError(t, err)

		nvmeofResult := cephv1.CephNVMeOFGateway{}
		err = cl.Get(ctx, req.NamespacedName, &nvmeofResult)
		assert.NoError(t, err)
		assert.Equal(t, uint32(3), nvmeofResult.Status.Cephx.Daemon.KeyGeneration)
		assert.Equal(t, "20.2.0-0", nvmeofResult.Status.Cephx.Daemon.KeyCephVersion)
	})
}

func TestNVMeOFConfigGeneration(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		config, err := getNVMeOFGatewayConfig("pool-a", "pod-a", "10.0.0.1", "ana-a", nil)
		assert.NoError(t, err)

		cfg, err := ini.Load([]byte(config))
		assert.NoError(t, err)
		assert.Equal(t, "pod-a", cfg.Section("gateway").Key("name").String())
		assert.Equal(t, "ana-a", cfg.Section("gateway").Key("group").String())
		assert.Equal(t, "10.0.0.1", cfg.Section("gateway").Key("addr").String())
		assert.Equal(t, "5500", cfg.Section("gateway").Key("port").String())
		assert.Equal(t, "0.0.0.0", cfg.Section("discovery").Key("addr").String())
		assert.Equal(t, "8009", cfg.Section("discovery").Key("port").String())
		assert.Equal(t, "admin", cfg.Section("ceph").Key("id").String())
		assert.Equal(t, "pool-a", cfg.Section("ceph").Key("pool").String())
		assert.Equal(t, "4096", cfg.Section("spdk").Key("mem_size").String())
		assert.Equal(t, "5499", cfg.Section("monitor").Key("port").String())
	})

	t.Run("overrides and new section", func(t *testing.T) {
		userConfig := map[string]map[string]string{
			"gateway": {
				"port":        "6600",
				"enable_auth": "True",
			},
			"ceph": {
				"pool": "pool-b",
			},
			"custom": {
				"foo": "bar",
			},
		}
		config, err := getNVMeOFGatewayConfig("pool-a", "pod-a", "10.0.0.1", "ana-a", userConfig)
		assert.NoError(t, err)

		cfg, err := ini.Load([]byte(config))
		assert.NoError(t, err)
		assert.Equal(t, "6600", cfg.Section("gateway").Key("port").String())
		assert.Equal(t, "True", cfg.Section("gateway").Key("enable_auth").String())
		assert.Equal(t, "pool-b", cfg.Section("ceph").Key("pool").String())
		assert.Equal(t, "bar", cfg.Section("custom").Key("foo").String())
	})
}

func TestNVMeOFConfigMapGeneration(t *testing.T) {
	nvmeof := &cephv1.CephNVMeOFGateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-nvmeof",
			Namespace: "rook-ceph",
		},
		Spec: cephv1.NVMeOFGatewaySpec{
			Pool:      "pool-a",
			Group:     "group-a",
			Instances: 1,
			NVMeOFConfig: map[string]map[string]string{
				"gateway": {
					"port": "5511",
				},
			},
		},
		TypeMeta: controllerTypeMeta,
	}

	r := &ReconcileCephNVMeOFGateway{}
	configMap, err := r.generateConfigMap(nvmeof, "a")
	assert.NoError(t, err)
	assert.Equal(t, "rook-ceph-nvmeof-my-nvmeof-a-config", configMap.Name)
	assert.Equal(t, "rook-ceph", configMap.Namespace)
	assert.Contains(t, configMap.Data, "config")

	cfg, err := ini.Load([]byte(configMap.Data["config"]))
	assert.NoError(t, err)
	assert.Equal(t, instanceName(nvmeof, "a"), cfg.Section("gateway").Key("name").String())
	assert.Equal(t, "@@POD_IP@@", cfg.Section("gateway").Key("addr").String())
	assert.Equal(t, "group-a", cfg.Section("gateway").Key("group").String())
	assert.Equal(t, "5511", cfg.Section("gateway").Key("port").String())
	assert.Equal(t, "pool-a", cfg.Section("ceph").Key("pool").String())
}
