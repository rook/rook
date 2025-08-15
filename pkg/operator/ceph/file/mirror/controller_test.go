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

// Package mirror to manage a rook filesystem
package mirror

import (
	"context"
	"os"
	"testing"

	"github.com/coreos/pkg/capnslog"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	cephAuthGetOrCreateKey = `{"key":"AQCvzWBeIV9lFRAAninzm+8XFxbSfTiPwoX50g=="}`
)

func TestCephFilesystemMirrorController(t *testing.T) {
	ctx := context.TODO()
	var (
		name      = "my-fs-mirror"
		namespace = "rook-ceph"
	)
	// Set DEBUG logging
	capnslog.SetGlobalLogLevel(capnslog.DEBUG)
	os.Setenv("ROOK_LOG_LEVEL", "DEBUG")

	// Mock cmd reporter
	currentAndDesiredCephVersion = func(ctx context.Context, rookImage string, namespace string, jobName string, ownerInfo *k8sutil.OwnerInfo, context *clusterd.Context, cephClusterSpec *cephv1.ClusterSpec, clusterInfo *client.ClusterInfo) (*version.CephVersion, *version.CephVersion, error) {
		return &version.CephVersion{Major: 16}, &version.CephVersion{Major: 16}, nil
	}

	fsMirror := &cephv1.CephFilesystemMirror{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec:     cephv1.FilesystemMirroringSpec{},
		TypeMeta: controllerTypeMeta,
	}

	// Objects to track in the fake client.
	object := []runtime.Object{
		fsMirror,
	}

	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			if args[0] == "status" {
				return `{"fsid":"c47cac40-9bee-4d52-823b-ccd803ba5bfe","health":{"checks":{},"status":"HEALTH_ERR"},"pgmap":{"num_pgs":100,"pgs_by_state":[{"state_name":"active+clean","count":100}]}}`, nil
			}
			if args[0] == "auth" && args[1] == "get-or-create-key" {
				return cephAuthGetOrCreateKey, nil
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

	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephFilesystemMirror{})
	s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephCluster{})

	// Create a fake client to mock API calls.
	cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()

	// Create a ReconcileFilesystemMirror object with the scheme and fake client.
	r := &ReconcileFilesystemMirror{
		client:  cl,
		scheme:  s,
		context: c,
		opConfig: controller.OperatorConfig{
			OperatorNamespace: namespace,
			Image:             "rook",
			ServiceAccount:    "foo",
		},
		opManagerContext: ctx,
		recorder:         record.NewFakeRecorder(5),
	}
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
	}

	cephCluster := &cephv1.CephCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namespace,
			Namespace: namespace,
		},
		Spec: cephv1.ClusterSpec{
			CephVersion: cephv1.CephVersionSpec{
				Image: "ceph/ceph:16.2.0",
			},
		},
		Status: cephv1.ClusterStatus{
			Phase: "",
			CephStatus: &cephv1.CephStatus{
				Health: "",
			},
		},
	}

	t.Run("error - no ceph cluster", func(t *testing.T) {
		// Mock request to simulate Reconcile() being called on an event for a
		// watched resource .

		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.True(t, res.Requeue)
	})

	t.Run("error - ceph cluster not ready", func(t *testing.T) {
		object = append(object, cephCluster)
		// Create a fake client to mock API calls.
		cl = fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()
		// Create a ReconcileFilesystemMirror object with the scheme and fake client.
		r = &ReconcileFilesystemMirror{
			client:  cl,
			scheme:  s,
			context: c,
			opConfig: controller.OperatorConfig{
				OperatorNamespace: namespace,
				Image:             "rook",
				ServiceAccount:    "foo",
			},
			opManagerContext: ctx,
			recorder:         record.NewFakeRecorder(5),
		}
		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.True(t, res.Requeue)
	})

	t.Run("error - ceph cluster ready but version is too old", func(t *testing.T) {
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

		// Add ready status to the CephCluster
		cephCluster.Status.Phase = k8sutil.ReadyStatus
		cephCluster.Status.CephStatus.Health = "HEALTH_OK"

		// Create a fake client to mock API calls.
		cl = fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()

		// Create a ReconcileFilesystemMirror object with the scheme and fake client.
		r = &ReconcileFilesystemMirror{
			client:  cl,
			scheme:  s,
			context: c,
			opConfig: controller.OperatorConfig{
				OperatorNamespace: namespace,
				Image:             "rook",
				ServiceAccount:    "foo",
			},
			opManagerContext: ctx,
			recorder:         record.NewFakeRecorder(5),
		}
		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.False(t, res.Requeue)
	})

	t.Run("error - cluster is upgrading", func(t *testing.T) {
		r.context.Executor = &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				if args[0] == "status" {
					return `{"fsid":"c47cac40-9bee-4d52-823b-ccd803ba5bfe","health":{"checks":{},"status":"HEALTH_ERR"},"pgmap":{"num_pgs":100,"pgs_by_state":[{"state_name":"active+clean","count":100}]}}`, nil
				}
				if args[0] == "auth" && args[1] == "get-or-create-key" {
					return cephAuthGetOrCreateKey, nil
				}
				return "", nil
			},
		}

		currentAndDesiredCephVersion = func(ctx context.Context, rookImage string, namespace string, jobName string, ownerInfo *k8sutil.OwnerInfo, context *clusterd.Context, cephClusterSpec *cephv1.ClusterSpec, clusterInfo *client.ClusterInfo) (*version.CephVersion, *version.CephVersion, error) {
			return &version.Squid, &version.Reef, nil
		}

		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.True(t, res.Requeue)
	})

	t.Run("success create reef", func(t *testing.T) {
		r.context.Executor = &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				if args[0] == "status" {
					return `{"fsid":"c47cac40-9bee-4d52-823b-ccd803ba5bfe","health":{"checks":{},"status":"HEALTH_ERR"},"pgmap":{"num_pgs":100,"pgs_by_state":[{"state_name":"active+clean","count":100}]}}`, nil
				}
				if args[0] == "auth" && args[1] == "get-or-create-key" {
					return cephAuthGetOrCreateKey, nil
				}
				return "", nil
			},
		}

		currentAndDesiredCephVersion = func(ctx context.Context, rookImage string, namespace string, jobName string, ownerInfo *k8sutil.OwnerInfo, context *clusterd.Context, cephClusterSpec *cephv1.ClusterSpec, clusterInfo *client.ClusterInfo) (*version.CephVersion, *version.CephVersion, error) {
			return &version.Reef, &version.Reef, nil
		}

		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.False(t, res.Requeue)
		err = r.client.Get(context.TODO(), req.NamespacedName, fsMirror)
		assert.NoError(t, err)
		assert.Equal(t, "Ready", fsMirror.Status.Phase, fsMirror)
	})
}

func TestFSMirrorKeyRotation(t *testing.T) {
	ctx := context.TODO()
	var (
		name      = "my-fs-mirror"
		namespace = "rook-ceph"
	)
	// Set DEBUG logging
	t.Setenv("ROOK_LOG_LEVEL", "DEBUG")

	currentAndDesiredCephVersion = func(ctx context.Context, rookImage, namespace, jobName string, ownerInfo *k8sutil.OwnerInfo, context *clusterd.Context, cephClusterSpec *cephv1.ClusterSpec, clusterInfo *client.ClusterInfo) (*version.CephVersion, *version.CephVersion, error) {
		rotationSupportedVer := version.CephVersion{Major: 20, Minor: 2, Extra: 0}
		return &rotationSupportedVer, &rotationSupportedVer, nil
	}

	fsMirror := &cephv1.CephFilesystemMirror{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec:     cephv1.FilesystemMirroringSpec{},
		TypeMeta: controllerTypeMeta,
	}

	// Mock request to simulate Reconcile() being called on an event for a
	// watched resource .
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
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

	// Objects to track in the fake client.
	object := []runtime.Object{
		fsMirror,
		cephCluster,
	}

	s := scheme.Scheme

	mirrorDaemonRotatedKey := `{"key":"AQCvzWBeIV9lFRAAninzm+8XFxbSfTiPwoX50g=="}`
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			if args[0] == "status" {
				return `{"fsid":"c47cac40-9bee-4d52-823b-ccd803ba5bfe","health":{"checks":{},"status":"HEALTH_ERR"},"pgmap":{"num_pgs":100,"pgs_by_state":[{"state_name":"active+clean","count":100}]}}`, nil
			}
			if args[0] == "auth" && args[1] == "get-or-create-key" {
				return cephAuthGetOrCreateKey, nil
			}
			if args[0] == "auth" && args[1] == "rotate" {
				t.Logf("rotating key and returning: %s", mirrorDaemonRotatedKey)
				return mirrorDaemonRotatedKey, nil
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

	// Register operator types with the runtime scheme.
	s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephRBDMirror{})
	s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephCluster{})

	// Create a fake client to mock API calls.
	cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()
	r := &ReconcileFilesystemMirror{client: cl, scheme: s, context: c, opManagerContext: ctx, recorder: record.NewFakeRecorder(5)}

	t.Run("first reconcile", func(t *testing.T) {
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

		_, err = r.Reconcile(ctx, req)
		assert.NoError(t, err)

		fsMirror := cephv1.CephFilesystemMirror{}
		err = cl.Get(ctx, req.NamespacedName, &fsMirror)
		assert.NoError(t, err)
		assert.Equal(t, uint32(1), fsMirror.Status.Cephx.Daemon.KeyGeneration)
		assert.Equal(t, "20.2.0-0", fsMirror.Status.Cephx.Daemon.KeyCephVersion)
	})

	t.Run("subsequent reconcile - retain cephx status", func(t *testing.T) {
		r := &ReconcileFilesystemMirror{client: cl, scheme: s, context: c, opManagerContext: ctx, recorder: record.NewFakeRecorder(5)}
		_, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		fsMirror := cephv1.CephFilesystemMirror{}
		err = cl.Get(ctx, req.NamespacedName, &fsMirror)
		assert.NoError(t, err)
		assert.Equal(t, uint32(1), fsMirror.Status.Cephx.Daemon.KeyGeneration)
		assert.Equal(t, "20.2.0-0", fsMirror.Status.Cephx.Daemon.KeyCephVersion)
	})

	t.Run("brownfield reconcile - retain unknown cephx status", func(t *testing.T) {
		fsMirror := cephv1.CephFilesystemMirror{}
		err := cl.Get(ctx, req.NamespacedName, &fsMirror)
		assert.NoError(t, err)
		fsMirror.Status.Cephx.Daemon = cephv1.CephxStatus{}
		err = cl.Update(ctx, &fsMirror)
		assert.NoError(t, err)

		_, err = r.Reconcile(ctx, req)
		assert.NoError(t, err)

		err = cl.Get(ctx, req.NamespacedName, &fsMirror)
		assert.NoError(t, err)
		assert.Equal(t, cephv1.CephxStatus{}, fsMirror.Status.Cephx.Daemon)
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

		mirrorDaemonRotatedKey = `[{"key":"BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB=="}]`

		_, err = r.Reconcile(ctx, req)
		assert.NoError(t, err)

		fsMirror := cephv1.CephFilesystemMirror{}
		err = cl.Get(ctx, req.NamespacedName, &fsMirror)
		assert.NoError(t, err)
		assert.Equal(t, uint32(2), fsMirror.Status.Cephx.Daemon.KeyGeneration)
		assert.Equal(t, "20.2.0-0", fsMirror.Status.Cephx.Daemon.KeyCephVersion)

		secret, err := clientset.CoreV1().Secrets(namespace).Get(ctx, "rook-ceph-fs-mirror-keyring", metav1.GetOptions{})
		assert.NoError(t, err)
		assert.Contains(t, secret.StringData["keyring"], "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB==")
	})

	t.Run("brownfield reconcile - no further rotation happens", func(t *testing.T) {
		// not expecting any rotation. So `ceph auth rotate` should not run and secret should not be updated
		mirrorDaemonRotatedKey = `[{"key":"CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC=="}]`

		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.False(t, res.Requeue)

		fsMirror := cephv1.CephFilesystemMirror{}
		err = cl.Get(ctx, req.NamespacedName, &fsMirror)
		assert.NoError(t, err)
		assert.Equal(t, uint32(2), fsMirror.Status.Cephx.Daemon.KeyGeneration)
		assert.Equal(t, "20.2.0-0", fsMirror.Status.Cephx.Daemon.KeyCephVersion)

		secret, err := clientset.CoreV1().Secrets(namespace).Get(ctx, "rook-ceph-fs-mirror-keyring", metav1.GetOptions{})
		assert.NoError(t, err)
		assert.NotContains(t, secret.StringData["keyring"], "CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC==")
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

		mirrorDaemonRotatedKey = `[{"key":"CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC=="}]`

		_, err = r.Reconcile(ctx, req)
		assert.NoError(t, err)

		fsMirror := cephv1.CephFilesystemMirror{}
		err = cl.Get(ctx, req.NamespacedName, &fsMirror)
		assert.NoError(t, err)
		assert.Equal(t, uint32(3), fsMirror.Status.Cephx.Daemon.KeyGeneration)
		assert.Equal(t, "20.2.0-0", fsMirror.Status.Cephx.Daemon.KeyCephVersion)

		secret, err := clientset.CoreV1().Secrets(namespace).Get(ctx, "rook-ceph-fs-mirror-keyring", metav1.GetOptions{})
		assert.NoError(t, err)
		assert.Contains(t, secret.StringData["keyring"], "CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC==")
	})
}
