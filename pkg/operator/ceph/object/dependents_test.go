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

package object

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/util/exec"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	zoneA                      = "zone-a"
	zoneB                      = "zone-b"
	zoneGroupGetSingleZoneJSON = `{
		"id": "fd8ff110-d3fd-49b4-b24f-f6cd3dddfedf",
		"name": "zonegroup-a",
		"api_name": "zonegroup-a",
		"is_master": true,
		"endpoints": [
			"http://rook-ceph-rgw-store-a.rook-ceph.svc:80"
        ],
		"hostnames": [],
		"hostnames_s3website": [],
		"master_zone": "6cb39d2c-3005-49da-9be3-c1a92a97d28a",
		"zones": [
			{
				"id": "6cb39d2c-3005-49da-9be3-c1a92a97d28a",
				"name": "zone-a",
				"endpoints": [
					"http://rook-ceph-rgw-store-a.rook-ceph.svc:80"
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
	zoneGroupGetMultipleZoneJSON = `{
		"id": "fd8ff110-d3fd-49b4-b24f-f6cd3dddfedf",
		"name": "zonegroup-a",
		"api_name": "zonegroup-a",
		"is_master": true,
		"endpoints": [
			"http://rook-ceph-rgw-store-a.rook-ceph.svc:80"
        ],
		"hostnames": [],
		"hostnames_s3website": [],
		"master_zone": "6cb39d2c-3005-49da-9be3-c1a92a97d28a",
		"zones": [
			{
				"id": "6cb39d2c-3005-49da-9be3-c1a92a97d28a",
				"name": "zone-a",
				"endpoints": [
					"http://rook-ceph-rgw-store-a.rook-ceph.svc:80"
				],
				"log_meta": "false",
				"log_data": "false",
				"bucket_index_max_shards": 0,
				"read_only": "false",
				"tier_type": "",
				"sync_from_all": "true",
				"sync_from": [],
				"redirect_zone": ""
			},
			{
				"id": "6cb39d2c-3005-49da-9be3-c1a92a97d28b",
				"name": "zone-b",
				"endpoints": [
					"http://rook-ceph-rgw-store-b.rook-ceph.svc:80"
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
	zoneGetZoneAJSON = `{
               "id": "6cb39d2c-3005-49da-9be3-c1a92a97d28a",
               "name": "zone-a",
               "domain_root": "zone-a.rgw.meta:root",
               "control_pool": "zone-a.rgw.control",
               "gc_pool": "zone-a.rgw.log:gc",
               "lc_pool": "zone-a.rgw.log:lc",
               "log_pool": "zone-a.rgw.log",
               "intent_log_pool": "zone-a.rgw.log:intent",
               "usage_log_pool": "zone-a.rgw.log:usage",
               "reshard_pool": "zone-a.rgw.log:reshard",
               "user_keys_pool": "zone-a.rgw.meta:users.keys",
               "user_email_pool": "zone-a.rgw.meta:users.email",
               "user_swift_pool": "zone-a.rgw.meta:users.swift",
               "user_uid_pool": "zone-a.rgw.meta:users.uid",
               "otp_pool": "zone-a.rgw.otp",
               "system_key": {
                       "access_key": "",
                       "secret_key": ""
               },
               "placement_pools": [
                       {
                               "key": "default-placement",
                               "val": {
                                       "index_pool": "zone-a.rgw.buckets.index",
                                       "storage_classes": {
                                               "STANDARD": {
                                                       "data_pool": "zone-a.rgw.buckets.data"
                                               }
                                       },
                                       "data_extra_pool": "zone-a.rgw.buckets.non-ec",
                                       "index_type": 0
                               }
                       }
               ],
               "metadata_heap": "",
               "realm_id": ""
       }`
	zoneGetZoneBJSON = `{
               "id": "6cb39d2c-3005-49da-9be3-c1a92a97d28b",
               "name": "zone-b",
               "domain_root": "zone-b.rgw.meta:root",
               "control_pool": "zone-b.rgw.control",
               "gc_pool": "zone-b.rgw.log:gc",
               "lc_pool": "zone-b.rgw.log:lc",
               "log_pool": "zone-b.rgw.log",
               "intent_log_pool": "zone-b.rgw.log:intent",
               "usage_log_pool": "zone-b.rgw.log:usage",
               "reshard_pool": "zone-b.rgw.log:reshard",
               "user_keys_pool": "zone-b.rgw.meta:users.keys",
               "user_email_pool": "zone-b.rgw.meta:users.email",
               "user_swift_pool": "zone-b.rgw.meta:users.swift",
               "user_uid_pool": "zone-b.rgw.meta:users.uid",
               "otp_pool": "zone-b.rgw.otp",
               "system_key": {
                       "access_key": "",
                       "secret_key": ""
               },
               "placement_pools": [
                       {
                               "key": "default-placement",
                               "val": {
                                       "index_pool": "zone-b.rgw.buckets.index",
                                       "storage_classes": {
                                               "STANDARD": {
                                                       "data_pool": "zone-b.rgw.buckets.data"
                                               }
                                       },
                                       "data_extra_pool": "zone-b.rgw.buckets.non-ec",
                                       "index_type": 0
                               }
                       }
               ],
               "metadata_heap": "",
               "realm_id": ""
       }`
)

func TestCephObjectStoreDependents(t *testing.T) {
	scheme := runtime.NewScheme()
	assert.NoError(t, cephv1.AddToScheme(scheme))
	ns := "test-ceph-object-store-dependents"
	var c *clusterd.Context
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			logger.Infof("Command: %s %v", command, args)
			if args[0] == "osd" {
				if args[1] == "lspools" {
					pools := []*cephclient.CephStoragePoolSummary{}
					output, err := json.Marshal(pools)
					assert.Nil(t, err)
					return string(output), nil
				}
			}
			return "", errors.Errorf("unexpected ceph command %q", args)
		},
	}

	newClusterdCtx := func(executor exec.Executor, objects ...runtime.Object) *clusterd.Context {
		return &clusterd.Context{
			RookClientset: rookclient.NewSimpleClientset(),
			Executor:      executor,
		}
	}

	pools := []*cephclient.CephStoragePoolSummary{
		{Name: "my-store.rgw.control"},
		{Name: "my-store.rgw.meta"},
		{Name: "my-store.rgw.log"},
		{Name: "my-store.rgw.buckets.non-ec"},
		{Name: "my-store.rgw.buckets.data"},
		{Name: ".rgw.root"},
		{Name: "my-store.rgw.buckets.index"},
		{Name: "my-store.rgw.otp"},
	}
	zoneBPools := []*cephclient.CephStoragePoolSummary{
		{Name: "zone-b.rgw.control"},
		{Name: "zone-b.rgw.meta"},
		{Name: "zone-b.rgw.log"},
		{Name: "zone-b.rgw.buckets.non-ec"},
		{Name: "zone-b.rgw.buckets.data"},
		{Name: ".rgw.root"},
		{Name: "zone-b.rgw.buckets.index"},
		{Name: "zone-b.rgw.otp"},
	}
	zoneAPools := []*cephclient.CephStoragePoolSummary{
		{Name: "zone-a.rgw.control"},
		{Name: "zone-a.rgw.meta"},
		{Name: "zone-a.rgw.log"},
		{Name: "zone-a.rgw.buckets.non-ec"},
		{Name: "zone-a.rgw.buckets.data"},
		{Name: ".rgw.root"},
		{Name: "zone-a.rgw.buckets.index"},
		{Name: "zone-a.rgw.otp"},
	}
	// Mock HTTP call
	mockClient := func(bucket string) *MockClient {
		return &MockClient{
			MockDo: func(req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodGet && req.URL.Path == "rook-ceph-rgw-my-store.mycluster.svc/admin/bucket" {
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(bytes.NewReader([]byte(bucket))),
					}, nil
				}
				return nil, fmt.Errorf("unexpected request: %q. method %q. path %q", req.URL.RawQuery, req.Method, req.URL.Path)
			},
		}
	}

	setMultisiteContext := func(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo, store *cephv1.CephObjectStore) *Context {
		return &Context{Context: context, Name: store.Name, clusterInfo: clusterInfo, Realm: "realm-a", ZoneGroup: "zonegroup-a", Zone: store.Spec.Zone.Name}
	}
	clusterInfo := cephclient.AdminTestClusterInfo(ns)
	// Create objectmeta with the given name in our test namespace
	meta := func(name string) v1.ObjectMeta {
		return v1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		}
	}

	store := &cephv1.CephObjectStore{
		ObjectMeta: v1.ObjectMeta{
			Name:      "my-store",
			Namespace: ns,
		},
		TypeMeta: v1.TypeMeta{
			Kind: "CephObjectStore",
		},
		Spec: cephv1.ObjectStoreSpec{
			Gateway: cephv1.GatewaySpec{Port: 80},
		},
	}
	storeZoneB := &cephv1.CephObjectStore{
		ObjectMeta: v1.ObjectMeta{
			Name:      "zone-b-store",
			Namespace: ns,
		},
		TypeMeta: v1.TypeMeta{
			Kind: "CephObjectStore",
		},
		Spec: cephv1.ObjectStoreSpec{
			Gateway: cephv1.GatewaySpec{Port: 80},
			Zone:    cephv1.ZoneSpec{Name: zoneB},
		},
	}
	storeZoneA := &cephv1.CephObjectStore{
		ObjectMeta: v1.ObjectMeta{
			Name:      "zone-a-store",
			Namespace: ns,
		},
		TypeMeta: v1.TypeMeta{
			Kind: "CephObjectStore",
		},
		Spec: cephv1.ObjectStoreSpec{
			Gateway: cephv1.GatewaySpec{Port: 80},
			Zone:    cephv1.ZoneSpec{Name: zoneA},
		},
	}

	t.Run("missing pools so skipping", func(t *testing.T) {
		c = newClusterdCtx(executor)
		deps, err := CephObjectStoreDependents(c, clusterInfo, store, NewContext(c, clusterInfo, store.Name), &AdminOpsContext{})
		assert.NoError(t, err)
		assert.True(t, deps.Empty())
	})

	t.Run("no objectstore users and no buckets", func(t *testing.T) {
		executor = &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				logger.Infof("Command: %s %v", command, args)
				if args[0] == "osd" {
					if args[1] == "lspools" {
						output, err := json.Marshal(pools)
						assert.Nil(t, err)
						return string(output), nil
					}
				}
				return "", errors.Errorf("unexpected ceph command %q", args)
			},
			MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
				if command == "radosgw-admin" && args[0] == "user" {
					return userCreateJSON, nil
				}
				return "", errors.Errorf("no such command %v %v", command, args)
			},
		}
		c = newClusterdCtx(executor)
		client, err := admin.New("rook-ceph-rgw-my-store.mycluster.svc", "53S6B9S809NUP19IJ2K3", "1bXPegzsGClvoGAiJdHQD1uOW2sQBLAZM9j9VtXR", mockClient(`[]`))
		assert.NoError(t, err)
		deps, err := CephObjectStoreDependents(c, clusterInfo, store, NewContext(c, clusterInfo, store.Name), &AdminOpsContext{AdminOpsClient: client})
		assert.NoError(t, err)
		assert.True(t, deps.Empty())
	})

	t.Run("one objectstore users but wrong store and no buckets", func(t *testing.T) {
		executor = &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				logger.Infof("Command: %s %v", command, args)
				if args[0] == "osd" {
					if args[1] == "lspools" {
						output, err := json.Marshal(pools)
						assert.Nil(t, err)
						return string(output), nil
					}
				}
				return "", errors.Errorf("unexpected ceph command %q", args)
			},
			MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
				if command == "radosgw-admin" && args[0] == "user" {
					return userCreateJSON, nil
				}
				return "", errors.Errorf("no such command %v %v", command, args)
			},
		}
		c = newClusterdCtx(executor, &cephv1.CephObjectStoreUser{ObjectMeta: meta("u1")})
		_, err := c.RookClientset.CephV1().CephObjectStoreUsers(clusterInfo.Namespace).Create(context.TODO(), &cephv1.CephObjectStoreUser{ObjectMeta: meta("u1")}, v1.CreateOptions{})
		assert.NoError(t, err)
		client, err := admin.New("rook-ceph-rgw-my-store.mycluster.svc", "53S6B9S809NUP19IJ2K3", "1bXPegzsGClvoGAiJdHQD1uOW2sQBLAZM9j9VtXR", mockClient(`[]`))
		assert.NoError(t, err)
		deps, err := CephObjectStoreDependents(c, clusterInfo, store, NewContext(c, clusterInfo, store.Name), &AdminOpsContext{AdminOpsClient: client})
		assert.NoError(t, err)
		assert.True(t, deps.Empty())
	})

	t.Run("one objectstore users and no buckets", func(t *testing.T) {
		executor = &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				logger.Infof("Command: %s %v", command, args)
				if args[0] == "osd" {
					if args[1] == "lspools" {
						output, err := json.Marshal(pools)
						assert.Nil(t, err)
						return string(output), nil
					}
				}
				return "", errors.Errorf("unexpected ceph command %q", args)
			},
			MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
				if command == "radosgw-admin" && args[0] == "user" {
					return userCreateJSON, nil
				}
				return "", errors.Errorf("no such command %v %v", command, args)
			},
		}
		c = newClusterdCtx(executor, &cephv1.CephObjectStoreUser{ObjectMeta: meta("u1")})
		_, err := c.RookClientset.CephV1().CephObjectStoreUsers(clusterInfo.Namespace).Create(context.TODO(), &cephv1.CephObjectStoreUser{ObjectMeta: meta("u1"), Spec: cephv1.ObjectStoreUserSpec{Store: "my-store"}}, v1.CreateOptions{})
		assert.NoError(t, err)
		client, err := admin.New("rook-ceph-rgw-my-store.mycluster.svc", "53S6B9S809NUP19IJ2K3", "1bXPegzsGClvoGAiJdHQD1uOW2sQBLAZM9j9VtXR", mockClient(`[]`))
		assert.NoError(t, err)
		deps, err := CephObjectStoreDependents(c, clusterInfo, store, NewContext(c, clusterInfo, store.Name), &AdminOpsContext{AdminOpsClient: client})
		assert.NoError(t, err)
		assert.False(t, deps.Empty())
		assert.ElementsMatch(t, []string{"u1"}, deps.OfKind("CephObjectStoreUsers"))
	})

	t.Run("store belong to secondary zone with no objectstore users and no buckets", func(t *testing.T) {
		executor = &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				logger.Infof("Command: %s %v", command, args)
				if args[0] == "osd" {
					if args[1] == "lspools" {
						output, err := json.Marshal(zoneBPools)
						assert.Nil(t, err)
						return string(output), nil
					}
				}
				return "", errors.Errorf("unexpected ceph command %q", args)
			},
			MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
				if command == "radosgw-admin" && args[0] == "user" {
					return userCreateJSON, nil
				}
				if command == "radosgw-admin" && args[0] == "zonegroup" && args[1] == "get" {
					return zoneGroupGetMultipleZoneJSON, nil
				}
				if command == "radosgw-admin" && args[0] == "zone" && args[1] == "get" {
					return zoneGetZoneBJSON, nil
				}
				return "", errors.Errorf("no such command %v %v", command, args)
			},
		}
		c = newClusterdCtx(executor)
		client, err := admin.New("rook-ceph-rgw-my-store.mycluster.svc", "53S6B9S809NUP19IJ2K3", "1bXPegzsGClvoGAiJdHQD1uOW2sQBLAZM9j9VtXR", mockClient(`[]`))
		assert.NoError(t, err)
		store.Spec.Zone.Name = zoneB
		deps, err := CephObjectStoreDependents(c, clusterInfo, storeZoneB, setMultisiteContext(c, clusterInfo, storeZoneB), &AdminOpsContext{AdminOpsClient: client})
		assert.NoError(t, err)
		assert.True(t, deps.Empty())
	})

	t.Run("store belong to secondary zone with one objectstore users but wrong store and no buckets", func(t *testing.T) {
		executor = &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				logger.Infof("Command: %s %v", command, args)
				if args[0] == "osd" {
					if args[1] == "lspools" {
						output, err := json.Marshal(zoneBPools)
						assert.Nil(t, err)
						return string(output), nil
					}
				}
				return "", errors.Errorf("unexpected ceph command %q", args)
			},
			MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
				if command == "radosgw-admin" && args[0] == "user" {
					return userCreateJSON, nil
				}
				if command == "radosgw-admin" && args[0] == "zonegroup" && args[1] == "get" {
					return zoneGroupGetMultipleZoneJSON, nil
				}
				if command == "radosgw-admin" && args[0] == "zone" && args[1] == "get" {
					return zoneGetZoneBJSON, nil
				}
				return "", errors.Errorf("no such command %v %v", command, args)
			},
		}
		c = newClusterdCtx(executor, &cephv1.CephObjectStoreUser{ObjectMeta: meta("u1")})
		_, err := c.RookClientset.CephV1().CephObjectStoreUsers(clusterInfo.Namespace).Create(context.TODO(), &cephv1.CephObjectStoreUser{ObjectMeta: meta("u1")}, v1.CreateOptions{})
		assert.NoError(t, err)
		client, err := admin.New("rook-ceph-rgw-my-store.mycluster.svc", "53S6B9S809NUP19IJ2K3", "1bXPegzsGClvoGAiJdHQD1uOW2sQBLAZM9j9VtXR", mockClient(`[]`))
		assert.NoError(t, err)
		deps, err := CephObjectStoreDependents(c, clusterInfo, storeZoneB, setMultisiteContext(c, clusterInfo, storeZoneB), &AdminOpsContext{AdminOpsClient: client})
		assert.NoError(t, err)
		assert.True(t, deps.Empty())
	})

	t.Run("store belong to secondary zone with one objectstore users and no buckets", func(t *testing.T) {
		executor = &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				logger.Infof("Command: %s %v", command, args)
				if args[0] == "osd" {
					if args[1] == "lspools" {
						output, err := json.Marshal(zoneBPools)
						assert.Nil(t, err)
						return string(output), nil
					}
				}
				return "", errors.Errorf("unexpected ceph command %q", args)
			},
			MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
				if command == "radosgw-admin" && args[0] == "user" {
					return userCreateJSON, nil
				}
				if command == "radosgw-admin" && args[0] == "zonegroup" && args[1] == "get" {
					return zoneGroupGetMultipleZoneJSON, nil
				}
				if command == "radosgw-admin" && args[0] == "zone" && args[1] == "get" {
					return zoneGetZoneBJSON, nil
				}
				return "", errors.Errorf("no such command %v %v", command, args)
			},
		}
		c = newClusterdCtx(executor, &cephv1.CephObjectStoreUser{ObjectMeta: meta("u1")})
		_, err := c.RookClientset.CephV1().CephObjectStoreUsers(clusterInfo.Namespace).Create(context.TODO(), &cephv1.CephObjectStoreUser{ObjectMeta: meta("u1"), Spec: cephv1.ObjectStoreUserSpec{Store: storeZoneB.Name}}, v1.CreateOptions{})
		assert.NoError(t, err)
		client, err := admin.New("rook-ceph-rgw-my-store.mycluster.svc", "53S6B9S809NUP19IJ2K3", "1bXPegzsGClvoGAiJdHQD1uOW2sQBLAZM9j9VtXR", mockClient(`[]`))
		assert.NoError(t, err)
		deps, err := CephObjectStoreDependents(c, clusterInfo, storeZoneB, setMultisiteContext(c, clusterInfo, storeZoneB), &AdminOpsContext{AdminOpsClient: client})
		assert.NoError(t, err)
		assert.True(t, deps.Empty())
	})

	t.Run("store belong to secondary zone with no objectstore users and one bucket", func(t *testing.T) {
		executor = &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				logger.Infof("Command: %s %v", command, args)
				if args[0] == "osd" {
					if args[1] == "lspools" {
						output, err := json.Marshal(zoneBPools)
						assert.Nil(t, err)
						return string(output), nil
					}
				}
				return "", errors.Errorf("unexpected ceph command %q", args)
			},
			MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
				if command == "radosgw-admin" && args[0] == "user" {
					return userCreateJSON, nil
				}
				if command == "radosgw-admin" && args[0] == "zonegroup" && args[1] == "get" {
					return zoneGroupGetMultipleZoneJSON, nil
				}
				if command == "radosgw-admin" && args[0] == "zone" && args[1] == "get" {
					return zoneGetZoneBJSON, nil
				}
				return "", errors.Errorf("no such command %v %v", command, args)
			},
		}
		c = newClusterdCtx(executor)
		client, err := admin.New("rook-ceph-rgw-my-store.mycluster.svc", "53S6B9S809NUP19IJ2K3", "1bXPegzsGClvoGAiJdHQD1uOW2sQBLAZM9j9VtXR", mockClient(`["my-bucket"]`))
		assert.NoError(t, err)
		deps, err := CephObjectStoreDependents(c, clusterInfo, storeZoneB, setMultisiteContext(c, clusterInfo, storeZoneB), &AdminOpsContext{AdminOpsClient: client})
		assert.NoError(t, err)
		assert.True(t, deps.Empty())
	})

	t.Run("store belong to master zone with no objectstore users, no buckets and no peers", func(t *testing.T) {
		executor = &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				logger.Infof("Command: %s %v", command, args)
				if args[0] == "osd" {
					if args[1] == "lspools" {
						output, err := json.Marshal(zoneAPools)
						assert.Nil(t, err)
						return string(output), nil
					}
				}
				return "", errors.Errorf("unexpected ceph command %q", args)
			},
			MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
				if command == "radosgw-admin" && args[0] == "user" {
					return userCreateJSON, nil
				}
				if command == "radosgw-admin" && args[0] == "zonegroup" && args[1] == "get" {
					return zoneGroupGetSingleZoneJSON, nil
				}
				if command == "radosgw-admin" && args[0] == "zone" && args[1] == "get" {
					return zoneGetZoneAJSON, nil
				}
				return "", errors.Errorf("no such command %v %v", command, args)
			},
		}
		c = newClusterdCtx(executor)
		client, err := admin.New("rook-ceph-rgw-my-store.mycluster.svc", "53S6B9S809NUP19IJ2K3", "1bXPegzsGClvoGAiJdHQD1uOW2sQBLAZM9j9VtXR", mockClient(`[]`))
		assert.NoError(t, err)
		deps, err := CephObjectStoreDependents(c, clusterInfo, storeZoneA, setMultisiteContext(c, clusterInfo, storeZoneA), &AdminOpsContext{AdminOpsClient: client})
		assert.NoError(t, err)
		assert.True(t, deps.Empty())
	})

	t.Run("store belong to master zone with one objectstore users but wrong store and no buckets and no peers", func(t *testing.T) {
		executor = &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				logger.Infof("Command: %s %v", command, args)
				if args[0] == "osd" {
					if args[1] == "lspools" {
						output, err := json.Marshal(zoneAPools)
						assert.Nil(t, err)
						return string(output), nil
					}
				}
				return "", errors.Errorf("unexpected ceph command %q", args)
			},
			MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
				if command == "radosgw-admin" && args[0] == "user" {
					return userCreateJSON, nil
				}
				if command == "radosgw-admin" && args[0] == "zonegroup" && args[1] == "get" {
					return zoneGroupGetSingleZoneJSON, nil
				}
				if command == "radosgw-admin" && args[0] == "zone" && args[1] == "get" {
					return zoneGetZoneAJSON, nil
				}
				return "", errors.Errorf("no such command %v %v", command, args)
			},
		}
		c = newClusterdCtx(executor, &cephv1.CephObjectStoreUser{ObjectMeta: meta("u1")})
		_, err := c.RookClientset.CephV1().CephObjectStoreUsers(clusterInfo.Namespace).Create(context.TODO(), &cephv1.CephObjectStoreUser{ObjectMeta: meta("u1")}, v1.CreateOptions{})
		assert.NoError(t, err)
		client, err := admin.New("rook-ceph-rgw-my-store.mycluster.svc", "53S6B9S809NUP19IJ2K3", "1bXPegzsGClvoGAiJdHQD1uOW2sQBLAZM9j9VtXR", mockClient(`[]`))
		assert.NoError(t, err)
		deps, err := CephObjectStoreDependents(c, clusterInfo, storeZoneA, setMultisiteContext(c, clusterInfo, storeZoneA), &AdminOpsContext{AdminOpsClient: client})
		assert.NoError(t, err)
		assert.True(t, deps.Empty())
	})

	t.Run("store belong to master zone with one objectstore users and no buckets and no peers", func(t *testing.T) {
		executor = &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				logger.Infof("Command: %s %v", command, args)
				if args[0] == "osd" {
					if args[1] == "lspools" {
						output, err := json.Marshal(zoneAPools)
						assert.Nil(t, err)
						return string(output), nil
					}
				}
				return "", errors.Errorf("unexpected ceph command %q", args)
			},
			MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
				if command == "radosgw-admin" && args[0] == "user" {
					return userCreateJSON, nil
				}
				if command == "radosgw-admin" && args[0] == "zonegroup" && args[1] == "get" {
					return zoneGroupGetSingleZoneJSON, nil
				}
				if command == "radosgw-admin" && args[0] == "zone" && args[1] == "get" {
					return zoneGetZoneAJSON, nil
				}
				return "", errors.Errorf("no such command %v %v", command, args)
			},
		}
		c = newClusterdCtx(executor, &cephv1.CephObjectStoreUser{ObjectMeta: meta("u1")})
		_, err := c.RookClientset.CephV1().CephObjectStoreUsers(clusterInfo.Namespace).Create(context.TODO(), &cephv1.CephObjectStoreUser{ObjectMeta: meta("u1"), Spec: cephv1.ObjectStoreUserSpec{Store: storeZoneA.Name}}, v1.CreateOptions{})
		assert.NoError(t, err)
		client, err := admin.New("rook-ceph-rgw-my-store.mycluster.svc", "53S6B9S809NUP19IJ2K3", "1bXPegzsGClvoGAiJdHQD1uOW2sQBLAZM9j9VtXR", mockClient(`[]`))
		assert.NoError(t, err)
		deps, err := CephObjectStoreDependents(c, clusterInfo, storeZoneA, setMultisiteContext(c, clusterInfo, storeZoneA), &AdminOpsContext{AdminOpsClient: client})
		assert.NoError(t, err)
		assert.False(t, deps.Empty())
		assert.ElementsMatch(t, []string{"u1"}, deps.OfKind("CephObjectStoreUsers"))
	})

	t.Run("store belong to master zone with no objectstore users and one bucket and no peer", func(t *testing.T) {
		executor = &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				logger.Infof("Command: %s %v", command, args)
				if args[0] == "osd" {
					if args[1] == "lspools" {
						output, err := json.Marshal(zoneAPools)
						assert.Nil(t, err)
						return string(output), nil
					}
				}
				return "", errors.Errorf("unexpected ceph command %q", args)
			},
			MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
				if command == "radosgw-admin" && args[0] == "user" {
					return userCreateJSON, nil
				}
				if command == "radosgw-admin" && args[0] == "zonegroup" && args[1] == "get" {
					return zoneGroupGetSingleZoneJSON, nil
				}
				if command == "radosgw-admin" && args[0] == "zone" && args[1] == "get" {
					return zoneGetZoneAJSON, nil
				}
				return "", errors.Errorf("no such command %v %v", command, args)
			},
		}
		c = newClusterdCtx(executor)
		client, err := admin.New("rook-ceph-rgw-my-store.mycluster.svc", "53S6B9S809NUP19IJ2K3", "1bXPegzsGClvoGAiJdHQD1uOW2sQBLAZM9j9VtXR", mockClient(`["my-bucket"]`))
		assert.NoError(t, err)
		deps, err := CephObjectStoreDependents(c, clusterInfo, storeZoneA, setMultisiteContext(c, clusterInfo, storeZoneA), &AdminOpsContext{AdminOpsClient: client})
		assert.NoError(t, err)
		assert.False(t, deps.Empty())
		assert.ElementsMatch(t, []string{"my-bucket"}, deps.OfKind("buckets in the object store (could be from ObjectBucketClaims or COSI Buckets)"), deps)
	})

	t.Run("store belong to master zone with no objectstore users, no buckets and one peer", func(t *testing.T) {
		executor = &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				logger.Infof("Command: %s %v", command, args)
				if args[0] == "osd" {
					if args[1] == "lspools" {
						output, err := json.Marshal(zoneAPools)
						assert.Nil(t, err)
						return string(output), nil
					}
				}
				return "", errors.Errorf("unexpected ceph command %q", args)
			},
			MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
				if command == "radosgw-admin" && args[0] == "user" {
					return userCreateJSON, nil
				}
				if command == "radosgw-admin" && args[0] == "zonegroup" && args[1] == "get" {
					return zoneGroupGetMultipleZoneJSON, nil
				}
				if command == "radosgw-admin" && args[0] == "zone" && args[1] == "get" {
					return zoneGetZoneAJSON, nil
				}
				return "", errors.Errorf("no such command %v %v", command, args)
			},
		}
		c = newClusterdCtx(executor)
		client, err := admin.New("rook-ceph-rgw-my-store.mycluster.svc", "53S6B9S809NUP19IJ2K3", "1bXPegzsGClvoGAiJdHQD1uOW2sQBLAZM9j9VtXR", mockClient(`[]`))
		assert.NoError(t, err)
		deps, err := CephObjectStoreDependents(c, clusterInfo, storeZoneA, setMultisiteContext(c, clusterInfo, storeZoneA), &AdminOpsContext{AdminOpsClient: client})
		assert.Error(t, err)
		assert.False(t, deps.Empty())
		assert.ElementsMatch(t, []string{"zone-b"}, deps.OfKind(zoneIsMasterWithPeersDependentType))
	})

	t.Run("store belong to master zone with one objectstore users but wrong store and no buckets and one peer", func(t *testing.T) {
		executor = &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				logger.Infof("Command: %s %v", command, args)
				if args[0] == "osd" {
					if args[1] == "lspools" {
						output, err := json.Marshal(zoneAPools)
						assert.Nil(t, err)
						return string(output), nil
					}
				}
				return "", errors.Errorf("unexpected ceph command %q", args)
			},
			MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
				if command == "radosgw-admin" && args[0] == "user" {
					return userCreateJSON, nil
				}
				if command == "radosgw-admin" && args[0] == "zonegroup" && args[1] == "get" {
					return zoneGroupGetMultipleZoneJSON, nil
				}
				if command == "radosgw-admin" && args[0] == "zone" && args[1] == "get" {
					return zoneGetZoneAJSON, nil
				}
				return "", errors.Errorf("no such command %v %v", command, args)
			},
		}
		c = newClusterdCtx(executor, &cephv1.CephObjectStoreUser{ObjectMeta: meta("u1")})
		_, err := c.RookClientset.CephV1().CephObjectStoreUsers(clusterInfo.Namespace).Create(context.TODO(), &cephv1.CephObjectStoreUser{ObjectMeta: meta("u1")}, v1.CreateOptions{})
		assert.NoError(t, err)
		client, err := admin.New("rook-ceph-rgw-my-store.mycluster.svc", "53S6B9S809NUP19IJ2K3", "1bXPegzsGClvoGAiJdHQD1uOW2sQBLAZM9j9VtXR", mockClient(`[]`))
		assert.NoError(t, err)
		deps, err := CephObjectStoreDependents(c, clusterInfo, storeZoneA, setMultisiteContext(c, clusterInfo, storeZoneA), &AdminOpsContext{AdminOpsClient: client})
		assert.Error(t, err)
		assert.False(t, deps.Empty())
		assert.ElementsMatch(t, []string{"zone-b"}, deps.OfKind(zoneIsMasterWithPeersDependentType))
	})

	t.Run("store belong to master zone with one objectstore users and no buckets and one peer", func(t *testing.T) {
		executor = &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				logger.Infof("Command: %s %v", command, args)
				if args[0] == "osd" {
					if args[1] == "lspools" {
						output, err := json.Marshal(zoneAPools)
						assert.Nil(t, err)
						return string(output), nil
					}
				}
				return "", errors.Errorf("unexpected ceph command %q", args)
			},
			MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
				if command == "radosgw-admin" && args[0] == "user" {
					return userCreateJSON, nil
				}
				if command == "radosgw-admin" && args[0] == "zonegroup" && args[1] == "get" {
					return zoneGroupGetMultipleZoneJSON, nil
				}
				if command == "radosgw-admin" && args[0] == "zone" && args[1] == "get" {
					return zoneGetZoneAJSON, nil
				}
				return "", errors.Errorf("no such command %v %v", command, args)
			},
		}
		c = newClusterdCtx(executor, &cephv1.CephObjectStoreUser{ObjectMeta: meta("u1")})
		_, err := c.RookClientset.CephV1().CephObjectStoreUsers(clusterInfo.Namespace).Create(context.TODO(), &cephv1.CephObjectStoreUser{ObjectMeta: meta("u1"), Spec: cephv1.ObjectStoreUserSpec{Store: storeZoneA.Name}}, v1.CreateOptions{})
		assert.NoError(t, err)
		client, err := admin.New("rook-ceph-rgw-my-store.mycluster.svc", "53S6B9S809NUP19IJ2K3", "1bXPegzsGClvoGAiJdHQD1uOW2sQBLAZM9j9VtXR", mockClient(`[]`))
		assert.NoError(t, err)
		deps, err := CephObjectStoreDependents(c, clusterInfo, storeZoneA, setMultisiteContext(c, clusterInfo, storeZoneA), &AdminOpsContext{AdminOpsClient: client})
		assert.Error(t, err)
		assert.False(t, deps.Empty())
		assert.ElementsMatch(t, []string{"zone-b"}, deps.OfKind(zoneIsMasterWithPeersDependentType))
	})

	t.Run("store belong to master zone with no objectstore users and one bucket and one peer", func(t *testing.T) {
		executor = &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				logger.Infof("Command: %s %v", command, args)
				if args[0] == "osd" {
					if args[1] == "lspools" {
						output, err := json.Marshal(zoneAPools)
						assert.Nil(t, err)
						return string(output), nil
					}
				}
				return "", errors.Errorf("unexpected ceph command %q", args)
			},
			MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
				if command == "radosgw-admin" && args[0] == "user" {
					return userCreateJSON, nil
				}
				if command == "radosgw-admin" && args[0] == "zonegroup" && args[1] == "get" {
					return zoneGroupGetMultipleZoneJSON, nil
				}
				if command == "radosgw-admin" && args[0] == "zone" && args[1] == "get" {
					return zoneGetZoneAJSON, nil
				}
				return "", errors.Errorf("no such command %v %v", command, args)
			},
		}
		c = newClusterdCtx(executor)
		client, err := admin.New("rook-ceph-rgw-my-store.mycluster.svc", "53S6B9S809NUP19IJ2K3", "1bXPegzsGClvoGAiJdHQD1uOW2sQBLAZM9j9VtXR", mockClient(`["my-bucket"]`))
		assert.NoError(t, err)
		deps, err := CephObjectStoreDependents(c, clusterInfo, storeZoneA, setMultisiteContext(c, clusterInfo, storeZoneA), &AdminOpsContext{AdminOpsClient: client})
		assert.Error(t, err)
		assert.False(t, deps.Empty())
		assert.ElementsMatch(t, []string{"zone-b"}, deps.OfKind(zoneIsMasterWithPeersDependentType))
	})
}
