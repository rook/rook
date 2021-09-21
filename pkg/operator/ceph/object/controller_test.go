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

// Package rgw to manage a rook object store.
package object

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
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
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	realmListJSON = `{
		"default_info": "237e6250-5f7d-4b85-9359-8cb2b1848507",
		"realms": [
			"my-store"
		]
	}`
	realmGetJSON = `{
		"id": "237e6250-5f7d-4b85-9359-8cb2b1848507",
		"name": "my-store",
		"current_period": "df665ecb-1762-47a9-9c66-f938d251c02a",
		"epoch": 2
	}`
	zoneGroupGetJSON = `{
		"id": "fd8ff110-d3fd-49b4-b24f-f6cd3dddfedf",
		"name": "my-store",
		"api_name": "my-store",
		"is_master": "true",
		"endpoints": [
			":80"
		],
		"hostnames": [],
		"hostnames_s3website": [],
		"master_zone": "6cb39d2c-3005-49da-9be3-c1a92a97d28a",
		"zones": [
			{
				"id": "6cb39d2c-3005-49da-9be3-c1a92a97d28a",
				"name": "my-store",
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
	zoneGetJSON = `{
		"id": "6cb39d2c-3005-49da-9be3-c1a92a97d28a",
		"name": "my-store",
		"domain_root": "my-store.rgw.meta:root",
		"control_pool": "my-store.rgw.control",
		"gc_pool": "my-store.rgw.log:gc",
		"lc_pool": "my-store.rgw.log:lc",
		"log_pool": "my-store.rgw.log",
		"intent_log_pool": "my-store.rgw.log:intent",
		"usage_log_pool": "my-store.rgw.log:usage",
		"reshard_pool": "my-store.rgw.log:reshard",
		"user_keys_pool": "my-store.rgw.meta:users.keys",
		"user_email_pool": "my-store.rgw.meta:users.email",
		"user_swift_pool": "my-store.rgw.meta:users.swift",
		"user_uid_pool": "my-store.rgw.meta:users.uid",
		"otp_pool": "my-store.rgw.otp",
		"system_key": {
			"access_key": "",
			"secret_key": ""
		},
		"placement_pools": [
			{
				"key": "default-placement",
				"val": {
					"index_pool": "my-store.rgw.buckets.index",
					"storage_classes": {
						"STANDARD": {
							"data_pool": "my-store.rgw.buckets.data"
						}
					},
					"data_extra_pool": "my-store.rgw.buckets.non-ec",
					"index_type": 0
				}
			}
		],
		"metadata_heap": "",
		"realm_id": ""
	}`
	rgwCephAuthGetOrCreateKey = `{"key":"AQCvzWBeIV9lFRAAninzm+8XFxbSfTiPwoX50g=="}`
	dummyVersionsRaw          = `
	{
		"mon": {
			"ceph version 14.2.8 (3a54b2b6d167d4a2a19e003a705696d4fe619afc) nautilus (stable)": 3
		}
	}`
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
	realmListMultisiteJSON = `{
		"default_info": "237e6250-5f7d-4b85-9359-8cb2b1848507",
		"realms": [
			"realm-a"
		]
	}`
	realmGetMultisiteJSON = `{
		"id": "237e6250-5f7d-4b85-9359-8cb2b1848507",
		"name": "realm-a",
		"current_period": "df665ecb-1762-47a9-9c66-f938d251c02a",
		"epoch": 2
	}`
	zoneGroupGetMultisiteJSON = `{
		"id": "fd8ff110-d3fd-49b4-b24f-f6cd3dddfedf",
		"name": "zonegroup-a",
		"api_name": "zonegroup-a",
		"is_master": "true",
		"endpoints": [
			":80"
		],
		"hostnames": [],
		"hostnames_s3website": [],
		"master_zone": "6cb39d2c-3005-49da-9be3-c1a92a97d28a",
		"zones": [
			{
				"id": "6cb39d2c-3005-49da-9be3-c1a92a97d28a",
				"name": "zone-a",
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
	zoneGetMultisiteJSON = `{
		"id": "6cb39d2c-3005-49da-9be3-c1a92a97d28a",
		"name": "zone-a",
		"domain_root": "my-store.rgw.meta:root",
		"control_pool": "my-store.rgw.control",
		"gc_pool": "my-store.rgw.log:gc",
		"lc_pool": "my-store.rgw.log:lc",
		"log_pool": "my-store.rgw.log",
		"intent_log_pool": "my-store.rgw.log:intent",
		"usage_log_pool": "my-store.rgw.log:usage",
		"reshard_pool": "my-store.rgw.log:reshard",
		"user_keys_pool": "my-store.rgw.meta:users.keys",
		"user_email_pool": "my-store.rgw.meta:users.email",
		"user_swift_pool": "my-store.rgw.meta:users.swift",
		"user_uid_pool": "my-store.rgw.meta:users.uid",
		"otp_pool": "my-store.rgw.otp",
		"system_key": {
			"access_key": "",
			"secret_key": ""
		},
		"placement_pools": [
			{
				"key": "default-placement",
				"val": {
					"index_pool": "my-store.rgw.buckets.index",
					"storage_classes": {
						"STANDARD": {
							"data_pool": "my-store.rgw.buckets.data"
						}
					},
					"data_extra_pool": "my-store.rgw.buckets.non-ec",
					"index_type": 0
				}
			}
		],
		"metadata_heap": "",
		"realm_id": ""
	}`
)

var (
	name      = "my-user"
	namespace = "rook-ceph"
	store     = "my-store"
)

func TestCephObjectStoreController(t *testing.T) {
	ctx := context.TODO()
	// Set DEBUG logging
	capnslog.SetGlobalLogLevel(capnslog.DEBUG)
	os.Setenv("ROOK_LOG_LEVEL", "DEBUG")

	setupNewEnvironment := func(additionalObjects ...runtime.Object) *ReconcileCephObjectStore {
		// A Pool resource with metadata and spec.
		objectStore := &cephv1.CephObjectStore{
			ObjectMeta: metav1.ObjectMeta{
				Name:      store,
				Namespace: namespace,
			},
			Spec:     cephv1.ObjectStoreSpec{},
			TypeMeta: controllerTypeMeta,
		}
		objectStore.Spec.Gateway.Port = 80

		// Objects to track in the fake client.
		objects := []runtime.Object{
			objectStore,
		}

		for i := range additionalObjects {
			objects = append(objects, additionalObjects[i])
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
		s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephObjectStore{})
		s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephCluster{})

		// Create a fake client to mock API calls.
		cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objects...).Build()
		// Create a ReconcileCephObjectStore object with the scheme and fake client.
		r := &ReconcileCephObjectStore{
			client:              cl,
			scheme:              s,
			context:             c,
			objectStoreChannels: make(map[string]*objectStoreHealth),
			recorder:            k8sutil.NewEventReporter(record.NewFakeRecorder(5)),
		}

		return r
	}

	// Mock request to simulate Reconcile() being called on an event for a
	// watched resource .
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      store,
			Namespace: namespace,
		},
	}

	t.Run("error - no ceph cluster", func(t *testing.T) {
		r := setupNewEnvironment()

		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.True(t, res.Requeue)
	})

	t.Run("error - ceph cluster not ready", func(t *testing.T) {
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

		r := setupNewEnvironment(cephCluster)

		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.True(t, res.Requeue)
	})

	// set up an environment that has a ready ceph cluster, and return the reconciler for it
	setupEnvironmentWithReadyCephCluster := func() *ReconcileCephObjectStore {
		cephCluster := &cephv1.CephCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      namespace,
				Namespace: namespace,
			},
			Status: cephv1.ClusterStatus{
				Phase: k8sutil.ReadyStatus,
				CephStatus: &cephv1.CephStatus{
					Health: "HEALTH_OK",
				},
			},
		}

		r := setupNewEnvironment(cephCluster)

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
		_, err := r.context.Clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
		assert.NoError(t, err)

		// Override executor with the new ceph status and more content
		executor := &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				if args[0] == "status" {
					return `{"fsid":"c47cac40-9bee-4d52-823b-ccd803ba5bfe","health":{"checks":{},"status":"HEALTH_OK"},"pgmap":{"num_pgs":100,"pgs_by_state":[{"state_name":"active+clean","count":100}]}}`, nil
				}
				if args[0] == "auth" && args[1] == "get-or-create-key" {
					return rgwCephAuthGetOrCreateKey, nil
				}
				if args[0] == "versions" {
					return dummyVersionsRaw, nil
				}
				if args[0] == "osd" && args[1] == "lspools" {
					// ceph actually outputs this all on one line, but this parses the same
					return `[
						{"poolnum":1,"poolname":"replicapool"},
						{"poolnum":2,"poolname":"device_health_metrics"},
						{"poolnum":3,"poolname":".rgw.root"},
						{"poolnum":4,"poolname":"my-store.rgw.buckets.index"},
						{"poolnum":5,"poolname":"my-store.rgw.buckets.non-ec"},
						{"poolnum":6,"poolname":"my-store.rgw.log"},
						{"poolnum":7,"poolname":"my-store.rgw.control"},
						{"poolnum":8,"poolname":"my-store.rgw.meta"},
						{"poolnum":9,"poolname":"my-store.rgw.buckets.data"}
					]`, nil
				}
				return "", nil
			},
			MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
				if args[0] == "realm" && args[1] == "list" {
					return realmListJSON, nil
				}
				if args[0] == "realm" && args[1] == "get" {
					return realmGetJSON, nil
				}
				if args[0] == "zonegroup" && args[1] == "get" {
					return zoneGroupGetJSON, nil
				}
				if args[0] == "zone" && args[1] == "get" {
					return zoneGetJSON, nil
				}
				if args[0] == "user" {
					return userCreateJSON, nil
				}
				return "", nil
			},
		}
		r.context.Executor = executor

		return r
	}

	t.Run("error - failed to start health checker", func(t *testing.T) {
		r := setupEnvironmentWithReadyCephCluster()

		// cause a failure when creating the admin ops api for the health check
		origHTTPClientFunc := genObjectStoreHTTPClientFunc
		genObjectStoreHTTPClientFunc = func(objContext *Context, spec *cephv1.ObjectStoreSpec) (client *http.Client, tlsCert []byte, err error) {
			return nil, []byte{}, errors.New("induced error creating admin ops API connection")
		}
		defer func() { genObjectStoreHTTPClientFunc = origHTTPClientFunc }()

		_, err := r.Reconcile(ctx, req)
		assert.Error(t, err)
		// we don't actually care if Requeue is true if there is an error assert.True(t, res.Requeue)
		assert.Contains(t, err.Error(), "failed to start rgw health checker")
		assert.Contains(t, err.Error(), "induced error creating admin ops API connection")
	})

	t.Run("success - object store is running", func(t *testing.T) {
		r := setupEnvironmentWithReadyCephCluster()

		res, err := r.Reconcile(ctx, req)
		assert.NoError(t, err)
		assert.False(t, res.Requeue)

		objectStore := &cephv1.CephObjectStore{}
		err = r.client.Get(context.TODO(), req.NamespacedName, objectStore)
		assert.NoError(t, err)
		assert.Equal(t, cephv1.ConditionProgressing, objectStore.Status.Phase, objectStore)
		assert.NotEmpty(t, objectStore.Status.Info["endpoint"], objectStore)
		assert.Equal(t, "http://rook-ceph-rgw-my-store.rook-ceph.svc:80", objectStore.Status.Info["endpoint"], objectStore)
	})
}

func TestCephObjectStoreControllerMultisite(t *testing.T) {
	ctx := context.TODO()
	capnslog.SetGlobalLogLevel(capnslog.DEBUG)
	os.Setenv("ROOK_LOG_LEVEL", "DEBUG")

	zoneName := "zone-a"
	zoneGroupName := "zonegroup-a"
	realmName := "realm-a"

	metadataPool := cephv1.PoolSpec{}
	dataPool := cephv1.PoolSpec{}

	cephCluster := &cephv1.CephCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namespace,
			Namespace: namespace,
		},
		Status: cephv1.ClusterStatus{
			Phase: k8sutil.ReadyStatus,
			CephStatus: &cephv1.CephStatus{
				Health: "HEALTH_OK",
			},
		},
	}

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

	objectZone := &cephv1.CephObjectZone{
		ObjectMeta: metav1.ObjectMeta{
			Name:      zoneName,
			Namespace: namespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind: "CephObjectZone",
		},
		Spec: cephv1.ObjectZoneSpec{
			ZoneGroup:    zoneGroupName,
			MetadataPool: metadataPool,
			DataPool:     dataPool,
		},
	}

	objectZoneGroup := &cephv1.CephObjectZoneGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      zoneGroupName,
			Namespace: namespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind: "CephObjectZoneGroup",
		},
		Spec: cephv1.ObjectZoneGroupSpec{},
	}

	objectZoneGroup.Spec.Realm = realmName

	objectRealm := &cephv1.CephObjectRealm{
		ObjectMeta: metav1.ObjectMeta{
			Name:      realmName,
			Namespace: namespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind: "CephObjectRealm",
		},
		Spec: cephv1.ObjectRealmSpec{},
	}

	objectStore := &cephv1.CephObjectStore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      store,
			Namespace: namespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind: "CephObjectStore",
		},
		Spec: cephv1.ObjectStoreSpec{},
	}

	objectStore.Spec.Zone.Name = zoneName
	objectStore.Spec.Gateway.Port = 80

	object := []runtime.Object{
		objectZone,
		objectStore,
		objectZoneGroup,
		objectRealm,
		cephCluster,
	}

	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			if args[0] == "status" {
				return `{"fsid":"c47cac40-9bee-4d52-823b-ccd803ba5bfe","health":{"checks":{},"status":"HEALTH_OK"},"pgmap":{"num_pgs":100,"pgs_by_state":[{"state_name":"active+clean","count":100}]}}`, nil
			}
			if args[0] == "auth" && args[1] == "get-or-create-key" {
				return rgwCephAuthGetOrCreateKey, nil
			}
			if args[0] == "versions" {
				return dummyVersionsRaw, nil
			}
			return "", nil
		},
		MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
			if args[0] == "realm" && args[1] == "list" {
				return realmListMultisiteJSON, nil
			}
			if args[0] == "realm" && args[1] == "get" {
				return realmGetMultisiteJSON, nil
			}
			if args[0] == "zonegroup" && args[1] == "get" {
				return zoneGroupGetMultisiteJSON, nil
			}
			if args[0] == "zone" && args[1] == "get" {
				return zoneGetMultisiteJSON, nil
			}
			if args[0] == "user" && args[1] == "create" {
				return userCreateJSON, nil
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
	s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephObjectZone{}, &cephv1.CephObjectZoneList{}, &cephv1.CephCluster{}, &cephv1.CephClusterList{}, &cephv1.CephObjectStore{}, &cephv1.CephObjectStoreList{})

	// Create a fake client to mock API calls.
	cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()

	r := &ReconcileCephObjectStore{
		client:              cl,
		scheme:              s,
		context:             c,
		objectStoreChannels: make(map[string]*objectStoreHealth),
		recorder:            k8sutil.NewEventReporter(record.NewFakeRecorder(5)),
	}

	_, err := r.context.Clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
	assert.NoError(t, err)

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      store,
			Namespace: namespace,
		},
	}

	res, err := r.Reconcile(ctx, req)
	assert.NoError(t, err)
	assert.False(t, res.Requeue)
	err = r.client.Get(context.TODO(), req.NamespacedName, objectStore)
	assert.NoError(t, err)
}
