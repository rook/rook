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

// Package file to manage a rook filesystem
package rbd

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
	dummyVersionsRaw       = `
	{
		"mon": {
			"ceph version 17.2.1 (0000000000000000000000000000000000) quincy (stable)": 3
		}
	}`
)

func TestCephRBDMirrorController(t *testing.T) {
	ctx := context.TODO()
	var (
		name           = "my-mirror"
		namespace      = "rook-ceph"
		peerSecretName = "peer-secret"
	)
	// Set DEBUG logging
	capnslog.SetGlobalLogLevel(capnslog.DEBUG)
	os.Setenv("ROOK_LOG_LEVEL", "DEBUG")

	currentAndDesiredCephVersion = func(ctx context.Context, rookImage string, namespace string, jobName string, ownerInfo *k8sutil.OwnerInfo, context *clusterd.Context, cephClusterSpec *cephv1.ClusterSpec, clusterInfo *client.ClusterInfo) (*version.CephVersion, *version.CephVersion, error) {
		return &version.Pacific, &version.Pacific, nil
	}

	// An rbd-mirror resource with metadata and spec.
	rbdMirror := &cephv1.CephRBDMirror{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: cephv1.RBDMirroringSpec{
			Count: 1,
		},
		TypeMeta: controllerTypeMeta,
	}

	// Objects to track in the fake client.
	object := []runtime.Object{
		rbdMirror,
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
		Status: cephv1.ClusterStatus{
			Phase: "",
			CephStatus: &cephv1.CephStatus{
				Health: "",
			},
		},
	}

	s := scheme.Scheme

	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			if args[0] == "status" {
				return `{"fsid":"c47cac40-9bee-4d52-823b-ccd803ba5bfe","health":{"checks":{},"status":"HEALTH_ERR"},"pgmap":{"num_pgs":100,"pgs_by_state":[{"state_name":"active+clean","count":100}]}}`, nil
			}
			if args[0] == "auth" && args[1] == "get-or-create-key" {
				return cephAuthGetOrCreateKey, nil
			}
			if args[0] == "versions" {
				return dummyVersionsRaw, nil
			}
			if args[0] == "mirror" && args[1] == "pool" && args[2] == "info" {
				return `{"mode":"image","site_name":"39074576-5884-4ef3-8a4d-8a0c5ed33031","peers":[{"uuid":"4a6983c0-3c9d-40f5-b2a9-2334a4659827","direction":"rx-tx","site_name":"ocs","mirror_uuid":"","client_name":"client.rbd-mirror-peer"}]}`, nil
			}
			if args[0] == "mirror" && args[1] == "pool" && args[2] == "status" {
				return `{"summary":{"health":"WARNING","daemon_health":"OK","image_health":"WARNING","states":{"unknown":1}}}`, nil
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

	t.Run("error - no ceph cluster", func(t *testing.T) {
		// Register operator types with the runtime scheme.
		s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephObjectStore{})
		s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephCluster{})

		// Create a fake client to mock API calls.
		cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()

		// Create a ReconcileCephRBDMirror object with the scheme and fake client.
		r := &ReconcileCephRBDMirror{client: cl, scheme: s, context: c, opManagerContext: ctx, recorder: record.NewFakeRecorder(5)}
		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.True(t, res.Requeue)
	})

	t.Run("error - ceph cluster not ready", func(t *testing.T) {
		object = append(object, cephCluster)
		// Create a fake client to mock API calls.
		cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()
		// Create a ReconcileCephRBDMirror object with the scheme and fake client.
		r := &ReconcileCephRBDMirror{client: cl, scheme: s, context: c, opManagerContext: ctx, recorder: record.NewFakeRecorder(5)}
		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.True(t, res.Requeue)
	})

	t.Run("error - wrong peers configuration", func(t *testing.T) {
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
		cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()

		// Create a ReconcileCephRBDMirror object with the scheme and fake client.
		r := &ReconcileCephRBDMirror{
			client:           cl,
			scheme:           s,
			context:          c,
			peers:            make(map[string]*peerSpec),
			opManagerContext: ctx,
			recorder:         record.NewFakeRecorder(5),
		}

		rbdMirror.Spec.Peers.SecretNames = []string{peerSecretName}
		err = r.client.Update(context.TODO(), rbdMirror)
		assert.NoError(t, err)
		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.True(t, res.Requeue)

	})

	t.Run("success - peers are configured", func(t *testing.T) {
		// Create a fake client to mock API calls.
		cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()
		r := &ReconcileCephRBDMirror{
			client:           cl,
			scheme:           s,
			context:          c,
			peers:            make(map[string]*peerSpec),
			opManagerContext: ctx,
			recorder:         record.NewFakeRecorder(5),
		}
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
		err = r.client.Get(context.TODO(), req.NamespacedName, rbdMirror)
		assert.NoError(t, err)
		assert.Equal(t, "Ready", rbdMirror.Status.Phase, rbdMirror)
	})
}
