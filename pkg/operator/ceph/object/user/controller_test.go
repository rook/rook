/*
Copyright 2018 The Rook Authors. All rights reserved.

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

// Package objectuser to manage a rook object store.
package objectuser

import (
	"context"
	"testing"

	"github.com/coreos/pkg/capnslog"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/operator/k8sutil"

	"github.com/rook/rook/pkg/clusterd"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	userCreateJSON = `{
	"user_id": "my-user",
	"display_name": "my-user",
	"email": "",
	"suspended": 0,
	"max_buckets": 1000,
	"subusers": [],
	"keys": [
		{
			"user": "my-user",
			"access_key": "EOE7FYCNOBZJ5VFV909G",
			"secret_key": "qmIqpWm8HxCzmynCrD6U6vKWi4hnDBndOnmxXNsV"
		}
	],
	"swift_keys": [],
	"caps": [],
	"op_mask": "read, write, delete",
	"default_placement": "",
	"default_storage_class": "",
	"placement_tags": [],
	"bucket_quota": {
		"enabled": false,
		"check_on_raw": false,
		"max_size": -1,
		"max_size_kb": 0,
		"max_objects": -1
	},
	"user_quota": {
		"enabled": false,
		"check_on_raw": false,
		"max_size": -1,
		"max_size_kb": 0,
		"max_objects": -1
	},
	"temp_url_keys": [],
	"type": "rgw",
	"mfa_ids": []
}`
)

var (
	name      = "my-user"
	namespace = "rook-ceph"
	store     = "my-store"
)

func TestCephObjectStoreUserController(t *testing.T) {
	// Set DEBUG logging
	capnslog.SetGlobalLogLevel(capnslog.DEBUG)

	//
	// TEST 1 SETUP
	//
	// FAILURE because no CephCluster
	//
	// A Pool resource with metadata and spec.
	objectUser := &cephv1.CephObjectStoreUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: cephv1.ObjectStoreUserSpec{
			Store: store,
		},
		TypeMeta: metav1.TypeMeta{
			Kind: "CephObjectStoreUser",
		},
	}
	cephCluster := &cephv1.CephCluster{}

	// Objects to track in the fake client.
	object := []runtime.Object{
		objectUser,
		cephCluster,
	}

	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(debug bool, actionName, command, outfile string, args ...string) (string, error) {
			if args[0] == "status" {
				return `{"pgmap":{"num_pgs":100,"pgs_by_state":[{"state_name":"active+clean","count":100}]}}`, nil
			}
			return "", nil
		},
		MockExecuteCommandWithOutput: func(debug bool, actionName, command string, args ...string) (string, error) {
			if args[0] == "user" {
				return userCreateJSON, nil
			}
			return "", nil
		},
	}
	c := &clusterd.Context{
		Executor:      executor,
		RookClientset: rookclient.NewSimpleClientset()}

	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephObjectStoreUser{})
	s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephCluster{})

	// Create a fake client to mock API calls.
	cl := fake.NewFakeClientWithScheme(s, object...)
	// Create a ReconcileObjectStoreUser object with the scheme and fake client.
	r := &ReconcileObjectStoreUser{client: cl, scheme: s, context: c}

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
	// Create a ReconcileObjectStoreUser object with the scheme and fake client.
	r = &ReconcileObjectStoreUser{client: cl, scheme: s, context: c}
	logger.Info("STARTING PHASE 2")
	res, err = r.Reconcile(req)
	assert.NoError(t, err)
	assert.True(t, res.Requeue)
	logger.Info("PHASE 2 DONE")

	//
	// TEST 3:
	//
	// FAILURE! The CephCluster is ready but NO rgw object
	//
	cephCluster.Status.Phase = k8sutil.ReadyStatus
	// Create a fake client to mock API calls.
	cl = fake.NewFakeClientWithScheme(s, object...)
	// Create a ReconcileObjectStoreUser object with the scheme and fake client.
	r = &ReconcileObjectStoreUser{client: cl, scheme: s, context: c}

	logger.Info("STARTING PHASE 3")
	res, err = r.Reconcile(req)
	assert.NoError(t, err)
	assert.True(t, res.Requeue)
	logger.Info("PHASE 3 DONE")

	//
	// TEST 4:
	//
	// FAILURE! The CephCluster is ready
	// Rgw object exists but NO pod are running
	//
	cephObjectStore := &cephv1.CephObjectStore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      store,
			Namespace: namespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind: "CephObjectStore",
		},
	}
	s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephObjectStore{})
	s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephObjectStoreList{})
	object = append(object, cephObjectStore)

	// Create a fake client to mock API calls.
	cl = fake.NewFakeClientWithScheme(s, object...)
	// Create a ReconcileObjectStoreUser object with the scheme and fake client.
	r = &ReconcileObjectStoreUser{client: cl, scheme: s, context: c}

	logger.Info("STARTING PHASE 4")
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: store, Namespace: namespace}, cephObjectStore)
	assert.NoError(t, err, cephObjectStore)
	res, err = r.Reconcile(req)
	assert.NoError(t, err)
	assert.True(t, res.Requeue)
	logger.Info("PHASE 4 DONE")

	//
	// TEST 5:
	//
	// SUCCESS! The CephCluster is ready
	// Rgw object exists and pods are running
	//
	rgwPod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name:      "rook-ceph-rgw-my-store-a-5fd6fb4489-xv65v",
		Namespace: namespace,
		Labels:    map[string]string{k8sutil.AppAttr: appName, "rgw": "my-store"}}}

	// Get the updated object.
	logger.Info("STARTING PHASE 5")
	err = r.client.Create(context.TODO(), rgwPod)
	assert.NoError(t, err)
	res, err = r.Reconcile(req)
	assert.NoError(t, err)
	assert.False(t, res.Requeue)
	err = r.client.Get(context.TODO(), req.NamespacedName, objectUser)
	assert.Equal(t, "Ready", objectUser.Status.Phase, objectUser)
	logger.Info("PHASE 5 DONE")
}
