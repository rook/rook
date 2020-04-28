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
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	cephAuthGetOrCreateKey = `{"key":"AQCvzWBeIV9lFRAAninzm+8XFxbSfTiPwoX50g=="}`
	dummyVersionsRaw       = `
	{
		"mon": {
			"ceph version 14.2.8 (3a54b2b6d167d4a2a19e003a705696d4fe619afc) nautilus (stable)": 3
		}
	}`
)

func TestCephRBDMirrorController(t *testing.T) {
	var (
		name      = "my-fs"
		namespace = "rook-ceph"
	)
	// Set DEBUG logging
	capnslog.SetGlobalLogLevel(capnslog.DEBUG)
	os.Setenv("ROOK_LOG_LEVEL", "DEBUG")

	//
	// TEST 1 SETUP
	//
	// FAILURE because no CephCluster
	//
	// An rbd-mirror resource with metadata and spec.
	fs := &cephv1.CephRBDMirror{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: cephv1.RBDMirroringSpec{
			Count: 1,
		},
		TypeMeta: controllerTypeMeta,
	}
	cephCluster := &cephv1.CephCluster{}

	// Objects to track in the fake client.
	object := []runtime.Object{
		fs,
	}

	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(command, outfile string, args ...string) (string, error) {
			if args[0] == "status" {
				return `{"fsid":"c47cac40-9bee-4d52-823b-ccd803ba5bfe","health":{"checks":{},"status":"HEALTH_ERR"},"pgmap":{"num_pgs":100,"pgs_by_state":[{"state_name":"active+clean","count":100}]}}`, nil
			}
			if args[0] == "auth" && args[1] == "get-or-create-key" {
				return cephAuthGetOrCreateKey, nil
			}
			if args[0] == "versions" {
				return dummyVersionsRaw, nil
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
	s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephObjectStore{})
	s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephCluster{})

	// Create a fake client to mock API calls.
	cl := fake.NewFakeClientWithScheme(s, object...)
	// Create a ReconcileCephRBDMirror object with the scheme and fake client.
	r := &ReconcileCephRBDMirror{client: cl, scheme: s, context: c}

	// Mock request to simulate Reconcile() being called on an event for a
	// watched resource .
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
	}
	logger.Info("STARTING PHASE 1")
	res, err := r.Reconcile(req)
	assert.NoError(t, err)
	assert.True(t, res.Requeue)
	logger.Info("PHASE 1 DONE")

	//
	// TEST 2:
	//
	// FAILURE we have a cluster but it's not ready
	//
	cephCluster = &cephv1.CephCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namespace,
			Namespace: namespace,
		},
		Status: cephv1.ClusterStatus{
			Phase: "",
		},
	}
	object = append(object, cephCluster)
	// Create a fake client to mock API calls.
	cl = fake.NewFakeClientWithScheme(s, object...)
	// Create a ReconcileCephRBDMirror object with the scheme and fake client.
	r = &ReconcileCephRBDMirror{client: cl, scheme: s, context: c}
	logger.Info("STARTING PHASE 2")
	res, err = r.Reconcile(req)
	assert.NoError(t, err)
	assert.True(t, res.Requeue)
	logger.Info("PHASE 2 DONE")

	//
	// TEST 3:
	//
	// SUCCESS! The CephCluster is ready
	//

	// Mock clusterInfo
	secrets := map[string][]byte{
		"cluster-name": []byte("foo-cluster"),
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

	// Add ready status to the CephCluster
	cephCluster.Status.Phase = k8sutil.ReadyStatus

	// Create a fake client to mock API calls.
	cl = fake.NewFakeClientWithScheme(s, object...)

	executor = &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(command, outfile string, args ...string) (string, error) {
			if args[0] == "status" {
				return `{"fsid":"c47cac40-9bee-4d52-823b-ccd803ba5bfe","health":{"checks":{},"status":"HEALTH_OK"},"pgmap":{"num_pgs":100,"pgs_by_state":[{"state_name":"active+clean","count":100}]}}`, nil
			}
			if args[0] == "auth" && args[1] == "get-or-create-key" {
				return cephAuthGetOrCreateKey, nil
			}
			return "", nil
		},
	}
	c.Executor = executor

	// Create a ReconcileCephRBDMirror object with the scheme and fake client.
	r = &ReconcileCephRBDMirror{client: cl, scheme: s, context: c}

	logger.Info("STARTING PHASE 3")
	res, err = r.Reconcile(req)
	assert.NoError(t, err)
	assert.False(t, res.Requeue)
	err = r.client.Get(context.TODO(), req.NamespacedName, fs)
	assert.NoError(t, err)
	assert.Equal(t, "Ready", fs.Status.Phase, fs)
	logger.Info("PHASE 3 DONE")
}
