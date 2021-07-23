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
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	"github.com/ceph/go-ceph/rgw/admin"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/util/exec"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
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
					pools := []*client.CephStoragePoolSummary{}
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
			DynamicClientset: dynamicfake.NewSimpleDynamicClient(scheme, objects...),
			RookClientset:    rookclient.NewSimpleClientset(),
			Executor:         executor,
		}
	}

	pools := []*client.CephStoragePoolSummary{
		{Name: "my-store.rgw.control"},
		{Name: "my-store.rgw.meta"},
		{Name: "my-store.rgw.log"},
		{Name: "my-store.rgw.buckets.non-ec"},
		{Name: "my-store.rgw.buckets.data"},
		{Name: ".rgw.root"},
		{Name: "my-store.rgw.buckets.index"},
	}

	// Mock HTTP call
	mockClient := func(bucket string) *MockClient {
		return &MockClient{
			MockDo: func(req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodGet && req.URL.Path == "rook-ceph-rgw-my-store.mycluster.svc/admin/bucket" {
					return &http.Response{
						StatusCode: 200,
						Body:       ioutil.NopCloser(bytes.NewReader([]byte(bucket))),
					}, nil
				}
				return nil, fmt.Errorf("unexpected request: %q. method %q. path %q", req.URL.RawQuery, req.Method, req.URL.Path)
			},
		}
	}

	clusterInfo := client.AdminClusterInfo(ns)
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
		assert.ElementsMatch(t, []string{"u1"}, deps.OfPluralKind("CephObjectStoreUsers"))
	})

	t.Run("no objectstore users and buckets", func(t *testing.T) {
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
		client, err := admin.New("rook-ceph-rgw-my-store.mycluster.svc", "53S6B9S809NUP19IJ2K3", "1bXPegzsGClvoGAiJdHQD1uOW2sQBLAZM9j9VtXR", mockClient(`["my-bucket"]`))
		assert.NoError(t, err)
		deps, err := CephObjectStoreDependents(c, clusterInfo, store, NewContext(c, clusterInfo, store.Name), &AdminOpsContext{AdminOpsClient: client})
		assert.NoError(t, err)
		assert.False(t, deps.Empty())
		assert.ElementsMatch(t, []string{"my-bucket"}, deps.OfPluralKind("buckets in the object store (could be from ObjectBucketClaims or COSI Buckets)"), deps)
	})
}
