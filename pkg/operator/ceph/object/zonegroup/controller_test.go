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

// Package zonegroup to manage a rook object zone group.
package zonegroup

import (
	"context"
	"testing"
	"time"

	"github.com/coreos/pkg/capnslog"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/operator/test"

	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/k8sutil"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var (
	name         = "zonegroup-a"
	realm        = "realm-a"
	namespace    = "rook-ceph"
	realmGetJSON = `{
		"id": "237e6250-5f7d-4b85-9359-8cb2b1848507",
		"name": "realm-a",
		"current_period": "df665ecb-1762-47a9-9c66-f938d251c02a",
		"epoch": 2
	}`
	periodGetJSON = `{
		"id": "3ee7933c-da65-4255-a5a6-f7348aaf2ece",
		"epoch": 1,
		"predecessor_uuid": "",
		"sync_status": [],
		"period_map": {
			"id": "3ee7933c-da65-4255-a5a6-f7348aaf2ece",
			"zonegroups": [],
			"short_zone_ids": []
		},
		"master_zonegroup": "",
		"master_zone": "",
		"period_config": {
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
			}
		},
		"realm_id": "ef13e399-a981-4138-a37c-e2ae05a06532",
		"realm_name": "realm-a",
		"realm_epoch": 1
	}`
	zoneGroupGetJSON = `{
		"id": "fd8ff110-d3fd-49b4-b24f-f6cd3dddfedf",
		"name": "zonegroup-a",
		"api_name": "zonegroup-a",
		"is_master": true,
		"endpoints": [
			":80"
		],
		"hostnames": [],
		"hostnames_s3website": [],
		"master_zone": "6cb39d2c-3005-49da-9be3-c1a92a97d28a",
		"zones": [
			{
				"id": "6cb39d2c-3005-49da-9be3-c1a92a97d28a",
				"name": "zone-group",
				"endpoints": [
					":80"
				],
				"log_meta": "false",
				"log_data": "false",
				"bucket_index_max_shards": 0,
				"read_only": "false",
				"tier_type": "",
				"sync_from_all": "true",
				"sync_from": [],
				"redirect_zone": ""
			}
		],
		"placement_targets": [
			{
				"name": "default-placement",
				"tags": [],
				"storage_classes": [
					"STANDARD"
				]
			}
		],
		"default_placement": "default-placement",
		"realm_id": "237e6250-5f7d-4b85-9359-8cb2b1848507"
	}`
)

func TestCephObjectZoneGroupController(t *testing.T) {
	ctx := context.TODO()
	capnslog.SetGlobalLogLevel(capnslog.DEBUG)
	//
	// TEST 1 SETUP
	//
	// FAILURE: because no CephCluster
	//
	// A Pool resource with metadata and spec.
	objectZoneGroup := &cephv1.CephObjectZoneGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind: "CephObjectZoneGroup",
		},
		Spec: cephv1.ObjectZoneGroupSpec{
			Realm: realm,
		},
	}

	// Objects to track in the fake client.
	object := []runtime.Object{
		objectZoneGroup,
	}

	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			if args[0] == "status" {
				return `{"fsid":"c47cac40-9bee-4d52-823b-ccd803ba5bfe","health":{"checks":{},"status":"HEALTH_ERR"},"pgmap":{"num_pgs":100,"pgs_by_state":[{"state_name":"active+clean","count":100}]}}`, nil
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
	s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephObjectZoneGroup{}, &cephv1.CephObjectZoneGroupList{})

	// Create a fake client to mock API calls.
	cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()

	// Create a ReconcileObjectZoneGroup object with the scheme and fake client.
	clusterInfo := cephclient.AdminTestClusterInfo("rook")

	r := &ReconcileObjectZoneGroup{client: cl, scheme: s, context: c, clusterInfo: clusterInfo}

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
	// FAILURE: we have a cluster but it's not ready
	//
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

	object = []runtime.Object{
		objectZoneGroup,
		cephCluster,
	}

	s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephObjectZoneGroup{}, &cephv1.CephObjectZoneGroupList{}, &cephv1.CephCluster{}, &cephv1.CephClusterList{})
	cl = fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()

	// Create a ReconcileObjectZoneGroup object with the scheme and fake client.
	r = &ReconcileObjectZoneGroup{client: cl, scheme: r.scheme, context: r.context}
	res, err = r.Reconcile(ctx, req)
	assert.NoError(t, err)
	assert.True(t, res.Requeue)

	//
	// TEST 3:
	//
	// Failure: The CephCluster is ready but no ObjectRealm has been created
	//

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
	_, err = r.context.Clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Add ready status to the CephCluster
	cephCluster.Status.Phase = k8sutil.ReadyStatus
	cephCluster.Status.CephStatus.Health = "HEALTH_OK"

	// Create a fake client to mock API calls.
	cl = fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()

	executor = &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			if args[0] == "status" {
				return `{"fsid":"c47cac40-9bee-4d52-823b-ccd803ba5bfe","health":{"checks":{},"status":"HEALTH_OK"},"pgmap":{"num_pgs":100,"pgs_by_state":[{"state_name":"active+clean","count":100}]}}`, nil
			}
			return "", nil
		},
		MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
			if args[0] == "realm" && args[1] == "get" {
				return realmGetJSON, nil
			}
			if args[0] == "zonegroup" && args[1] == "get" {
				return zoneGroupGetJSON, nil
			}
			return "", nil
		},
	}
	r.context.Executor = executor

	// Create a ReconcileObjectZoneGroup
	r = &ReconcileObjectZoneGroup{client: cl, scheme: r.scheme, context: r.context}

	res, err = r.Reconcile(ctx, req)
	assert.Error(t, err)
	assert.True(t, res.Requeue)

	//
	// TEST 4:
	//
	// Success: The CephCluster is ready and ObjectRealm has been created
	//

	objectRealm := &cephv1.CephObjectRealm{
		ObjectMeta: metav1.ObjectMeta{
			Name:      realm,
			Namespace: namespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind: "CephObjectRealm",
		},
		Spec: cephv1.ObjectRealmSpec{},
	}

	// Add ready status to the CephCluster
	cephCluster.Status.Phase = k8sutil.ReadyStatus
	cephCluster.Status.CephStatus.Health = "HEALTH_OK"

	// Objects to track in the fake client.
	object = []runtime.Object{
		objectZoneGroup,
		objectRealm,
		cephCluster,
	}

	executor = &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			if args[0] == "status" {
				return `{"fsid":"c47cac40-9bee-4d52-823b-ccd803ba5bfe","health":{"checks":{},"status":"HEALTH_ERR"},"pgmap":{"num_pgs":100,"pgs_by_state":[{"state_name":"active+clean","count":100}]}}`, nil
			}
			return "", nil
		},
		MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
			if args[0] == "zonegroup" && args[1] == "get" {
				return zoneGroupGetJSON, nil
			}
			if args[0] == "period" && args[1] == "get" {
				return periodGetJSON, nil
			}
			if args[0] == "realm" && args[1] == "get" {
				return realmGetJSON, nil
			}
			return "", nil
		},
	}

	clientset = test.New(t, 3)
	c = &clusterd.Context{
		Executor:      executor,
		RookClientset: rookclient.NewSimpleClientset(),
		Clientset:     clientset,
	}

	_, err = c.Clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
	assert.NoError(t, err)

	s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephObjectZoneGroup{}, &cephv1.CephObjectZoneGroupList{}, &cephv1.CephCluster{}, &cephv1.CephClusterList{}, &cephv1.CephObjectRealm{}, &cephv1.CephObjectRealmList{})

	cl = fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()

	r = &ReconcileObjectZoneGroup{client: cl, scheme: s, context: c, clusterInfo: clusterInfo}

	err = r.client.Get(context.TODO(), types.NamespacedName{Name: realm, Namespace: namespace}, objectRealm)
	assert.NoError(t, err, objectRealm)

	req = reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
	}

	res, err = r.Reconcile(ctx, req)
	assert.NoError(t, err)
	assert.False(t, res.Requeue)
	err = r.client.Get(context.TODO(), req.NamespacedName, objectZoneGroup)
	assert.NoError(t, err)
}
