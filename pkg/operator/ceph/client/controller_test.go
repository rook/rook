/*
Copyright 2019 The Rook Authors. All rights reserved.

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

package client

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/coreos/pkg/capnslog"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	testop "github.com/rook/rook/pkg/operator/test"
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

func TestValidateClient(t *testing.T) {
	context := &clusterd.Context{Executor: &exectest.MockExecutor{}}

	// must specify caps
	p := cephv1.CephClient{ObjectMeta: metav1.ObjectMeta{Name: "client1", Namespace: "myns"}}
	err := ValidateClient(context, &p)
	assert.NotNil(t, err)

	// must specify name
	p = cephv1.CephClient{ObjectMeta: metav1.ObjectMeta{Namespace: "myns"}}
	err = ValidateClient(context, &p)
	assert.NotNil(t, err)

	// must specify namespace
	p = cephv1.CephClient{ObjectMeta: metav1.ObjectMeta{Name: "client1"}}
	err = ValidateClient(context, &p)
	assert.NotNil(t, err)

	// succeed with caps properly defined
	p = cephv1.CephClient{ObjectMeta: metav1.ObjectMeta{Name: "client1", Namespace: "myns"}}
	p.Spec.Caps = map[string]string{
		"osd": "allow *",
		"mon": "allow *",
		"mds": "allow *",
	}
	err = ValidateClient(context, &p)
	assert.Nil(t, err)
}

func TestGenerateClient(t *testing.T) {
	p := &cephv1.CephClient{ObjectMeta: metav1.ObjectMeta{Name: "client1", Namespace: "myns"},
		Spec: cephv1.ClientSpec{
			Caps: map[string]string{
				"osd": "allow *",
				"mon": "allow rw",
				"mds": "allow rwx",
			},
		},
	}

	client, caps := genClientEntity(p)
	equal := bytes.Compare([]byte(client), []byte("client.client1"))
	var res bool = equal == 0
	assert.True(t, res)
	assert.True(t, strings.Contains(strings.Join(caps, " "), "osd allow *"))
	assert.True(t, strings.Contains(strings.Join(caps, " "), "mon allow rw"))
	assert.True(t, strings.Contains(strings.Join(caps, " "), "mds allow rwx"))

	// Fail if caps are empty
	p2 := &cephv1.CephClient{ObjectMeta: metav1.ObjectMeta{Name: "client2", Namespace: "myns"},
		Spec: cephv1.ClientSpec{
			Caps: map[string]string{
				"osd": "",
				"mon": "",
			},
		},
	}

	client, _ = genClientEntity(p2)
	equal = bytes.Compare([]byte(client), []byte("client.client2"))
	res = equal == 0
	assert.True(t, res)
}

func TestCephClientController(t *testing.T) {
	ctx := context.TODO()
	// Set DEBUG logging
	capnslog.SetGlobalLogLevel(capnslog.DEBUG)
	os.Setenv("ROOK_LOG_LEVEL", "DEBUG")

	//
	// TEST 1 SETUP
	//
	// FAILURE because no CephCluster
	//
	logger.Info("RUN 1")
	var (
		name      = "my-client"
		namespace = "rook-ceph"
	)

	// A Pool resource with metadata and spec.
	cephClient := &cephv1.CephClient{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       types.UID("c47cac40-9bee-4d52-823b-ccd803ba5bfe"),
		},
		Spec: cephv1.ClientSpec{
			Caps: map[string]string{
				"osd": "allow *",
				"mon": "allow *",
			},
		},
		Status: &cephv1.CephClientStatus{
			Phase: "",
		},
	}

	// Objects to track in the fake client.
	object := []runtime.Object{
		cephClient,
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
	s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephClient{}, &cephv1.CephClusterList{})

	// Create a fake client to mock API calls.
	cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()

	// Create a ReconcileCephClient object with the scheme and fake client.
	r := &ReconcileCephClient{
		client:           cl,
		scheme:           s,
		context:          c,
		opManagerContext: ctx,
		recorder:         record.NewFakeRecorder(5),
	}

	// Mock request to simulate Reconcile() being called on an event for a
	// watched resource .
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
	}

	res, err := r.Reconcile(ctx, req)
	assert.NoError(t, err)
	assert.True(t, res.Requeue)

	//
	// TEST 2:
	//
	// FAILURE we have a cluster but it's not ready
	//
	logger.Info("RUN 2")
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

	object = append(object, cephCluster)
	// Create a fake client to mock API calls.
	cl = fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()
	// Create a ReconcileCephClient object with the scheme and fake client.
	r = &ReconcileCephClient{
		client:           cl,
		scheme:           s,
		context:          c,
		opManagerContext: ctx,
		recorder:         record.NewFakeRecorder(5),
	}
	assert.True(t, res.Requeue)

	//
	// TEST 3:
	//
	// SUCCESS! The CephCluster is ready
	//
	logger.Info("RUN 3")
	cephCluster.Status.Phase = cephv1.ConditionReady
	cephCluster.Status.CephStatus.Health = "HEALTH_OK"

	objects := []runtime.Object{
		cephClient,
		cephCluster,
	}
	// Create a fake client to mock API calls.
	cl = fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objects...).Build()
	c.Client = cl

	executor = &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			if args[0] == "status" {
				return `{"fsid":"c47cac40-9bee-4d52-823b-ccd803ba5bfe","health":{"checks":{},"status":"HEALTH_OK"},"pgmap":{"num_pgs":100,"pgs_by_state":[{"state_name":"active+clean","count":100}]}}`, nil
			}
			if args[0] == "auth" && args[1] == "get-or-create-key" {
				return `{"key":"AQCvzWBeIV9lFRAAninzm+8XFxbSfTiPwoX50g=="}`, nil
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
	_, err = c.Clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
	assert.NoError(t, err)

	s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephBlockPoolList{})
	// Create a ReconcileCephClient object with the scheme and fake client.
	r = &ReconcileCephClient{
		client:           cl,
		scheme:           s,
		context:          c,
		opManagerContext: context.TODO(),
		recorder:         record.NewFakeRecorder(5),
	}

	res, err = r.Reconcile(ctx, req)
	assert.NoError(t, err)
	assert.False(t, res.Requeue)

	err = r.client.Get(context.TODO(), req.NamespacedName, cephClient)
	assert.NoError(t, err)
	assert.Equal(t, cephv1.ConditionReady, cephClient.Status.Phase)
	assert.NotEmpty(t, cephClient.Status.Info["secretName"], cephClient.Status.Info)
	cephClientSecret, err := c.Clientset.CoreV1().Secrets(namespace).Get(ctx, cephClient.Status.Info["secretName"], metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotEmpty(t, cephClientSecret.StringData)
}

func TestBuildUpdateStatusInfo(t *testing.T) {
	cephClient := &cephv1.CephClient{
		ObjectMeta: metav1.ObjectMeta{
			Name: "client-ocp",
		},
		Spec: cephv1.ClientSpec{},
	}

	statusInfo := generateStatusInfo(cephClient)
	assert.NotEmpty(t, statusInfo["secretName"])
	assert.Equal(t, "rook-ceph-client-client-ocp", statusInfo["secretName"])
}
