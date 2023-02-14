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

package object

import (
	"context"
	"fmt"
	"syscall"
	"testing"
	"time"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

const (
	dashboardAdminCreateJSON = `{
    "user_id": "dashboard-admin",
    "display_name": "dashboard-admin",
    "email": "",
    "suspended": 0,
    "max_buckets": 1000,
    "subusers": [],
    "keys": [
        {
            "user": "dashboard-admin",
            "access_key": "VFKF8SSU9L3L2UR03Z8C",
            "secret_key": "5U4e2MkXHgXstfWkxGZOI6AXDfVUkDDHM7Dwc3mY"
        }
    ],
    "swift_keys": [],
    "caps": [],
    "op_mask": "read, write, delete",
    "system": "true",
    "temp_url_keys": [],
    "type": "rgw",
    "mfa_ids": [],
	"user_quota": {
		"enabled": false,
		"check_on_raw": false,
		"max_size": -1,
		"max_size_kb": 0,
		"max_objects": -1
	}
}`
	//#nosec G101 -- The credentials are just for the unit tests
	access_key = "VFKF8SSU9L3L2UR03Z8C"
	//#nosec G101 -- The credentials are just for the unit tests
	secret_key = "5U4e2MkXHgXstfWkxGZOI6AXDfVUkDDHM7Dwc3mY"
)

func TestReconcileRealm(t *testing.T) {
	executorFunc := func(command string, args ...string) (string, error) {
		idResponse := `{"id":"test-id"}`
		logger.Infof("Execute: %s %v", command, args)
		return idResponse, nil
	}
	executorFuncTimeout := func(timeout time.Duration, command string, args ...string) (string, error) {
		testResponse := `{"id": "test-id"}`
		logger.Infof("Execute: %s %v", command, args)
		return testResponse, nil
	}
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput:         executorFunc,
		MockExecuteCommandWithCombinedOutput: executorFunc,
		MockExecuteCommandWithTimeout:        executorFuncTimeout,
	}

	storeName := "myobject"
	context := &clusterd.Context{Executor: executor}
	objContext := NewContext(context, &client.ClusterInfo{Namespace: "mycluster"}, storeName)
	// create the first realm, marked as default
	store := cephv1.CephObjectStore{}
	err := setMultisite(objContext, &store, nil)
	assert.Nil(t, err)

	// create the second realm, not marked as default
	err = setMultisite(objContext, &store, nil)
	assert.Nil(t, err)
}

func TestDeleteStore(t *testing.T) {
	deleteStore(t, "myobj", `"mystore","myobj"`, false)
	deleteStore(t, "myobj", `"myobj"`, true)
}

func deleteStore(t *testing.T, name string, existingStores string, expectedDeleteRootPool bool) {
	realmDeleted := false
	zoneDeleted := false
	zoneGroupDeleted := false
	poolsDeleted := 0
	rulesDeleted := 0
	executor := &exectest.MockExecutor{}
	deletedRootPool := false
	deletedErasureCodeProfile := false
	mockExecutorFuncOutput := func(command string, args ...string) (string, error) {
		if args[0] == "osd" {
			if args[1] == "pool" {
				if args[2] == "get" {
					return `{"pool_id":1}`, nil
				}
				if args[2] == "delete" {
					poolsDeleted++
					if args[3] == rootPool {
						deletedRootPool = true
					}
					return "", nil
				}
			}
			if args[1] == "crush" {
				assert.Equal(t, "rule", args[2])
				assert.Equal(t, "rm", args[3])
				rulesDeleted++
				return "", nil
			}
			if args[1] == "erasure-code-profile" {
				if args[2] == "ls" {
					return `["default","myobj_ecprofile"]`, nil
				}
				if args[2] == "rm" {
					if args[3] == "myobj_ecprofile" {
						deletedErasureCodeProfile = true
					} else {
						assert.Fail(t, fmt.Sprintf("the erasure code profile to be deleted should be myobj_ecprofile. Actual: %s ", args[3]))
					}
					return "", nil
				}
			}
		}
		if args[0] == "realm" {
			if args[1] == "delete" {
				realmDeleted = true
				return "", nil
			}
			if args[1] == "list" {
				return fmt.Sprintf(`{"realms":[%s]}`, existingStores), nil
			}
		}
		if args[0] == "zonegroup" {
			assert.Equal(t, "delete", args[1])
			zoneGroupDeleted = true
			return "", nil
		}
		if args[0] == "zone" {
			assert.Equal(t, "delete", args[1])
			zoneDeleted = true
			return "", nil
		}

		if args[0] == "pool" {
			if args[1] == "stats" {
				emptyPool := "{\"images\":{\"count\":0,\"provisioned_bytes\":0,\"snap_count\":0},\"trash\":{\"count\":1,\"provisioned_bytes\":2048,\"snap_count\":0}}"
				return emptyPool, nil
			}
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}

	executorFuncWithTimeout := func(timeout time.Duration, command string, args ...string) (string, error) {
		return mockExecutorFuncOutput(command, args...)
	}
	executorFunc := func(command string, args ...string) (string, error) {
		return mockExecutorFuncOutput(command, args...)
	}

	executor.MockExecuteCommandWithTimeout = executorFuncWithTimeout
	executor.MockExecuteCommandWithOutput = executorFunc
	executor.MockExecuteCommandWithCombinedOutput = executorFunc
	context := &Context{Context: &clusterd.Context{Executor: executor}, Name: "myobj", clusterInfo: client.AdminTestClusterInfo("mycluster")}

	// Delete an object store without deleting the pools
	spec := cephv1.ObjectStoreSpec{}
	err := deleteRealmAndPools(context, spec)
	assert.Nil(t, err)
	expectedPoolsDeleted := 0
	assert.Equal(t, expectedPoolsDeleted, poolsDeleted)
	assert.Equal(t, expectedPoolsDeleted, rulesDeleted)
	assert.True(t, realmDeleted)
	assert.True(t, zoneGroupDeleted)
	assert.True(t, zoneDeleted)
	assert.Equal(t, false, deletedErasureCodeProfile)

	// Delete an object store with the pools
	spec = cephv1.ObjectStoreSpec{
		MetadataPool: cephv1.PoolSpec{Replicated: cephv1.ReplicatedSpec{Size: 1}},
		DataPool:     cephv1.PoolSpec{Replicated: cephv1.ReplicatedSpec{Size: 1}},
	}
	err = deleteRealmAndPools(context, spec)
	assert.Nil(t, err)
	expectedPoolsDeleted = 7
	if expectedDeleteRootPool {
		expectedPoolsDeleted++
	}
	assert.Equal(t, expectedPoolsDeleted, poolsDeleted)
	assert.Equal(t, expectedDeleteRootPool, deletedRootPool)
	assert.Equal(t, true, deletedErasureCodeProfile)
}

func TestGetObjectBucketProvisioner(t *testing.T) {
	testNamespace := "test-namespace"
	t.Setenv(k8sutil.PodNamespaceEnvVar, testNamespace)

	t.Run("watch single namespace", func(t *testing.T) {
		data := map[string]string{"ROOK_OBC_WATCH_OPERATOR_NAMESPACE": "true"}
		bktprovisioner := GetObjectBucketProvisioner(data, testNamespace)
		assert.Equal(t, fmt.Sprintf("%s.%s", testNamespace, bucketProvisionerName), bktprovisioner)
	})

	t.Run("watch all namespaces", func(t *testing.T) {
		data := map[string]string{"ROOK_OBC_WATCH_OPERATOR_NAMESPACE": "false"}
		bktprovisioner := GetObjectBucketProvisioner(data, testNamespace)
		assert.Equal(t, bucketProvisionerName, bktprovisioner)
	})
}

func TestRGWPGNumVersion(t *testing.T) {
	assert.False(t, rgwRadosPGNumIsNew(cephver.CephVersion{Major: 15, Minor: 2, Extra: 9}))
	assert.False(t, rgwRadosPGNumIsNew(cephver.CephVersion{Major: 16, Minor: 2, Extra: 10}))
	assert.True(t, rgwRadosPGNumIsNew(cephver.CephVersion{Major: 16, Minor: 2, Extra: 11}))
	assert.False(t, rgwRadosPGNumIsNew(cephver.CephVersion{Major: 17, Minor: 2, Extra: 1}))
	assert.True(t, rgwRadosPGNumIsNew(cephver.CephVersion{Major: 17, Minor: 2, Extra: 2}))
	assert.True(t, rgwRadosPGNumIsNew(cephver.CephVersion{Major: 18, Minor: 0, Extra: 0}))
}

func TestCheckDashboardUser(t *testing.T) {
	storeName := "myobject"
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			return "", nil
		},
		MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
			if args[0] == "user" {
				if args[1] == "info" {
					return "no user info saved", nil
				}
			}
			return "", nil
		},
	}
	objContext := NewContext(&clusterd.Context{Executor: executor}, &client.ClusterInfo{
		Namespace:   "mycluster",
		CephVersion: cephver.CephVersion{Major: 15, Minor: 2, Extra: 9},
		Context:     context.TODO(),
	},
		storeName)

	// Scenario 1: No user exists yet
	user, err := getDashboardUser(objContext)
	assert.NoError(t, err)
	assert.NotNil(t, user)
	assert.Nil(t, user.AccessKey)
	assert.Nil(t, user.SecretKey)
	checkdashboard, err := checkDashboardUser(objContext, user)
	assert.NoError(t, err)
	assert.False(t, checkdashboard)

	// Scenario 2: User exists and the current dashboard credentials are the same
	objContext.Context.Executor = &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			if args[0] == "dashboard" {
				if args[1] == "get-rgw-api-access-key" {
					return access_key, nil
				} else if args[1] == "get-rgw-api-secret-key" {
					return secret_key, nil
				}
			}
			return "", nil
		},
		MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
			if args[0] == "user" {
				if args[1] == "info" {
					return dashboardAdminCreateJSON, nil
				}
			}
			return "", nil
		},
	}

	user, err = getDashboardUser(objContext)
	assert.NoError(t, err)
	assert.NotNil(t, user)
	assert.NotNil(t, user.AccessKey)
	assert.NotNil(t, user.SecretKey)

	checkdashboard, err = checkDashboardUser(objContext, user)
	assert.NoError(t, err)
	assert.True(t, checkdashboard)

	// Scenario 3: User exists but dashboard credentials differ from radosgw-admin user info credentials
	objContext.Context.Executor = &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			if args[0] == "dashboard" {
				if args[1] == "get-rgw-api-access-key" {
					return "incorrect", nil
				} else if args[1] == "get-rgw-api-secret-key" {
					return "incorrect", nil
				}
			}
			return "", nil
		},
		MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
			if args[0] == "user" {
				if args[1] == "info" {
					return dashboardAdminCreateJSON, nil
				}
			}
			return "", nil
		},
	}

	user, err = getDashboardUser(objContext)
	assert.NoError(t, err)
	assert.NotNil(t, user)
	assert.NotNil(t, user.AccessKey)
	assert.NotNil(t, user.SecretKey)

	checkdashboard, err = checkDashboardUser(objContext, user)
	assert.NoError(t, err)
	assert.False(t, checkdashboard)
}

func TestDashboard(t *testing.T) {
	storeName := "myobject"
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			return "", nil
		},
		MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
			if args[0] == "user" {
				if args[1] == "info" {
					return "no user info saved", nil
				} else if args[1] == "create" {
					return dashboardAdminCreateJSON, nil
				}
			}
			return "", nil
		},
	}
	objContext := NewContext(&clusterd.Context{Executor: executor}, &client.ClusterInfo{
		Namespace:   "mycluster",
		CephVersion: cephver.CephVersion{Major: 15, Minor: 2, Extra: 9},
		Context:     context.TODO(),
	},
		storeName)

	user, err := getDashboardUser(objContext)
	assert.NoError(t, err)
	assert.NotNil(t, user)
	checkdashboard, err := checkDashboardUser(objContext, user)
	assert.NoError(t, err)
	assert.False(t, checkdashboard)
	err = enableRGWDashboard(objContext)
	assert.NoError(t, err)

	executor = &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			return "", nil
		},
		MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
			if args[0] == "user" && args[1] == "info" {
				return dashboardAdminCreateJSON, nil
			}
			return "", nil
		},
	}
	objContext.Context.Executor = executor

	user, err = getDashboardUser(objContext)
	assert.NoError(t, err)
	assert.NotNil(t, user)
	checkdashboard, err = checkDashboardUser(objContext, user)
	assert.NoError(t, err)
	assert.True(t, checkdashboard)
	disableRGWDashboard(objContext)

	objContext = NewContext(&clusterd.Context{Executor: executor}, &client.ClusterInfo{
		Namespace:   "mycluster",
		CephVersion: cephver.CephVersion{Major: 15, Minor: 2, Extra: 10},
		Context:     context.TODO(),
	},
		storeName)
	err = enableRGWDashboard(objContext)
	assert.NoError(t, err)
	checkdashboard, err = checkDashboardUser(objContext, user)
	assert.NoError(t, err)
	assert.True(t, checkdashboard)
	disableRGWDashboard(objContext)
}

// import TestMockExecHelperProcess
func TestMockExecHelperProcess(t *testing.T) {
	exectest.TestMockExecHelperProcess(t)
}

func Test_createMultisite(t *testing.T) {
	// control the return values from calling get/create/update on resources
	type commandReturns struct {
		realmExists             bool
		zoneGroupExists         bool
		zoneExists              bool
		failCreateRealm         bool
		failCreateZoneGroup     bool
		failCreateZone          bool
		failCommitConfigChanges bool
	}

	// control whether we should expect certain 'get' calls
	type expectCommands struct {
		getRealm            bool
		createRealm         bool
		getZoneGroup        bool
		createZoneGroup     bool
		getZone             bool
		createZone          bool
		commitConfigChanges bool
	}

	// vars used for testing if calls were made
	var (
		calledGetRealm            = false
		calledGetZoneGroup        = false
		calledGetZone             = false
		calledCreateRealm         = false
		calledCreateZoneGroup     = false
		calledCreateZone          = false
		calledCommitConfigChanges = false
	)

	commitConfigChangesOrig := commitConfigChanges
	defer func() { commitConfigChanges = commitConfigChangesOrig }()

	enoentIfNotExist := func(resourceExists bool) (string, error) {
		if !resourceExists {
			return "", exectest.MockExecCommandReturns(t, "", "", int(syscall.ENOENT))
		}
		return "{}", nil // get wants json, and {} is the most basic json
	}

	errorIfFail := func(shouldFail bool) (string, error) {
		if shouldFail {
			return "", exectest.MockExecCommandReturns(t, "", "basic error", 1)
		}
		return "", nil
	}

	setupTest := func(env commandReturns) *exectest.MockExecutor {
		// reset output testing vars
		calledGetRealm = false
		calledCreateRealm = false
		calledGetZoneGroup = false
		calledCreateZoneGroup = false
		calledGetZone = false
		calledCreateZone = false
		calledCommitConfigChanges = false

		commitConfigChanges = func(c *Context) error {
			calledCommitConfigChanges = true
			if env.failCommitConfigChanges {
				return errors.New("fake error from CommitConfigChanges")
			}
			return nil
		}

		return &exectest.MockExecutor{
			MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, arg ...string) (string, error) {
				if command == "radosgw-admin" {
					switch arg[0] {
					case "realm":
						switch arg[1] {
						case "get":
							calledGetRealm = true
							return enoentIfNotExist(env.realmExists)
						case "create":
							calledCreateRealm = true
							return errorIfFail(env.failCreateRealm)
						}
					case "zonegroup":
						switch arg[1] {
						case "get":
							calledGetZoneGroup = true
							return enoentIfNotExist(env.zoneGroupExists)
						case "create":
							calledCreateZoneGroup = true
							return errorIfFail(env.failCreateZoneGroup)
						}
					case "zone":
						switch arg[1] {
						case "get":
							calledGetZone = true
							return enoentIfNotExist(env.zoneExists)
						case "create":
							calledCreateZone = true
							return errorIfFail(env.failCreateZone)
						}
					}
				}
				t.Fatalf("unhandled command: %s %v", command, arg)
				panic("unhandled command")
			},
		}
	}

	expectNoErr := false // want no error
	expectErr := true    // want an error

	tests := []struct {
		name           string
		commandReturns commandReturns
		expectCommands expectCommands
		wantErr        bool
	}{
		{"create realm, zonegroup, and zone; commit config",
			commandReturns{
				// nothing exists, and all should succeed
			},
			expectCommands{
				getRealm:            true,
				createRealm:         true,
				getZoneGroup:        true,
				createZoneGroup:     true,
				getZone:             true,
				createZone:          true,
				commitConfigChanges: true,
			},
			expectNoErr},
		{"fail creating realm",
			commandReturns{
				failCreateRealm: true,
			},
			expectCommands{
				getRealm:    true,
				createRealm: true,
				// when we fail to create realm, we should not continue
			},
			expectErr},
		{"fail creating zonegroup",
			commandReturns{
				failCreateZoneGroup: true,
			},
			expectCommands{
				getRealm:        true,
				createRealm:     true,
				getZoneGroup:    true,
				createZoneGroup: true,
				// when we fail to create zonegroup, we should not continue
			},
			expectErr},
		{"fail creating zone",
			commandReturns{
				failCreateZone: true,
			},
			expectCommands{
				getRealm:        true,
				createRealm:     true,
				getZoneGroup:    true,
				createZoneGroup: true,
				getZone:         true,
				createZone:      true,
				// when we fail to create zone, we should not continue
			},
			expectErr},
		{"fail commit config",
			commandReturns{
				failCommitConfigChanges: true,
			},
			expectCommands{
				getRealm:            true,
				createRealm:         true,
				getZoneGroup:        true,
				createZoneGroup:     true,
				getZone:             true,
				createZone:          true,
				commitConfigChanges: true,
			},
			expectErr},
		{"realm exists; create zonegroup and zone; commit config",
			commandReturns{
				realmExists: true,
			},
			expectCommands{
				getRealm:            true,
				createRealm:         false,
				getZoneGroup:        true,
				createZoneGroup:     true,
				getZone:             true,
				createZone:          true,
				commitConfigChanges: true,
			},
			expectNoErr},
		{"realm and zonegroup exist; create zone; commit config",
			commandReturns{
				realmExists:     true,
				zoneGroupExists: true,
			},
			expectCommands{
				getRealm:            true,
				createRealm:         false,
				getZoneGroup:        true,
				createZoneGroup:     false,
				getZone:             true,
				createZone:          true,
				commitConfigChanges: true,
			},
			expectNoErr},
		{"realm, zonegroup, and zone exist; commit config",
			commandReturns{
				realmExists:     true,
				zoneGroupExists: true,
				zoneExists:      true,
			},
			expectCommands{
				getRealm:            true,
				createRealm:         false,
				getZoneGroup:        true,
				createZoneGroup:     false,
				getZone:             true,
				createZone:          false,
				commitConfigChanges: true,
			},
			expectNoErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := setupTest(tt.commandReturns)
			ctx := &clusterd.Context{
				Executor: executor,
			}
			objContext := NewContext(ctx, &client.ClusterInfo{Namespace: "my-cluster"}, "my-store")

			// assumption: endpointArg is sufficiently tested by integration tests
			err := createMultisite(objContext, "")
			assert.Equal(t, tt.expectCommands.getRealm, calledGetRealm)
			assert.Equal(t, tt.expectCommands.createRealm, calledCreateRealm)
			assert.Equal(t, tt.expectCommands.getZoneGroup, calledGetZoneGroup)
			assert.Equal(t, tt.expectCommands.createZoneGroup, calledCreateZoneGroup)
			assert.Equal(t, tt.expectCommands.getZone, calledGetZone)
			assert.Equal(t, tt.expectCommands.createZone, calledCreateZone)
			assert.Equal(t, tt.expectCommands.commitConfigChanges, calledCommitConfigChanges)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetRealmKeySecret(t *testing.T) {
	ns := "my-ns"
	realmName := "my-realm"
	ctx := context.TODO()

	t.Run("secret exists", func(t *testing.T) {
		secret := &v1.Secret{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Secret",
				APIVersion: v1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ns,
				Name:      realmName + "-keys",
			},
			// should not care about data presence just to get the secret
		}

		c := &clusterd.Context{
			Clientset: k8sfake.NewSimpleClientset(secret),
		}

		secret, err := GetRealmKeySecret(ctx, c, types.NamespacedName{Namespace: ns, Name: realmName})
		assert.NoError(t, err)
		assert.NotNil(t, secret)
	})

	t.Run("secret doesn't exist", func(t *testing.T) {
		c := &clusterd.Context{
			Clientset: k8sfake.NewSimpleClientset(),
		}

		secret, err := GetRealmKeySecret(ctx, c, types.NamespacedName{Namespace: ns, Name: realmName})
		assert.Error(t, err)
		assert.Nil(t, secret)
	})
}

func TestGetRealmKeyArgsFromSecret(t *testing.T) {
	ns := "my-ns"
	realmName := "my-realm"
	realmNsName := types.NamespacedName{Namespace: ns, Name: realmName}

	baseSecret := &v1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      realmName + "-keys",
		},
		Data: map[string][]byte{},
	}

	t.Run("all secret data exists", func(t *testing.T) {
		s := baseSecret.DeepCopy()
		s.Data["access-key"] = []byte("my-access-key")
		s.Data["secret-key"] = []byte("my-secret-key")

		access, secret, err := GetRealmKeyArgsFromSecret(s, realmNsName)
		assert.NoError(t, err)
		assert.Equal(t, "--access-key=my-access-key", access)
		assert.Equal(t, "--secret-key=my-secret-key", secret)
	})

	t.Run("access-key missing", func(t *testing.T) {
		s := baseSecret.DeepCopy()
		// missing s.Data["access-key"]
		s.Data["secret-key"] = []byte("my-secret-key")

		access, secret, err := GetRealmKeyArgsFromSecret(s, realmNsName)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decode CephObjectRealm \"my-ns/my-realm\" access key from secret")
		assert.Equal(t, "", access)
		assert.Equal(t, "", secret)
	})

	t.Run("secret-key missing", func(t *testing.T) {
		s := baseSecret.DeepCopy()
		s.Data["access-key"] = []byte("my-access-key")
		// missing s.Data["secret-key"]

		access, secret, err := GetRealmKeyArgsFromSecret(s, realmNsName)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decode CephObjectRealm \"my-ns/my-realm\" secret key from secret")
		assert.Equal(t, "", access)
		assert.Equal(t, "", secret)
	})
}

func TestGetRealmKeyArgs(t *testing.T) {
	ns := "my-ns"
	realmName := "my-realm"
	ctx := context.TODO()

	baseSecret := &v1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      realmName + "-keys",
		},
		Data: map[string][]byte{},
	}

	// No need to test every case since this is a combination of GetRealmKeySecret and
	// GetRealmKeyArgsFromSecret and those are both thoroughly unit tested. Just check the success
	// case and cases where either sub-function fails.

	t.Run("secret exists with all data", func(t *testing.T) {
		s := baseSecret.DeepCopy()
		s.Data["access-key"] = []byte("my-access-key")
		s.Data["secret-key"] = []byte("my-secret-key")

		c := &clusterd.Context{
			Clientset: k8sfake.NewSimpleClientset(s),
		}

		access, secret, err := GetRealmKeyArgs(ctx, c, realmName, ns)
		assert.NoError(t, err)
		assert.Equal(t, "--access-key=my-access-key", access)
		assert.Equal(t, "--secret-key=my-secret-key", secret)
	})

	t.Run("secret doesn't exist", func(t *testing.T) {
		c := &clusterd.Context{
			Clientset: k8sfake.NewSimpleClientset(),
		}

		access, secret, err := GetRealmKeyArgs(ctx, c, realmName, ns)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get CephObjectRealm \"my-ns/my-realm\" keys secret")
		assert.Equal(t, "", access)
		assert.Equal(t, "", secret)
	})

	t.Run("secret exists but is missing data", func(t *testing.T) {
		s := baseSecret.DeepCopy()
		// missing all data

		c := &clusterd.Context{
			Clientset: k8sfake.NewSimpleClientset(s),
		}

		access, secret, err := GetRealmKeyArgs(ctx, c, realmName, ns)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decode CephObjectRealm \"my-ns/my-realm\"")
		assert.Equal(t, "", access)
		assert.Equal(t, "", secret)
	})
}

func TestUpdateZoneEndpointList(t *testing.T) {
	type args struct {
		zones            []zoneType
		zoneEndpointList []string
		zoneName         string
	}
	tests := []struct {
		name    string
		args    args
		want    bool
		wantErr bool
	}{
		{"all the fields are empty",
			args{zones: []zoneType{}, zoneEndpointList: []string{}, zoneName: ""},
			false, true,
		},
		{"zoneName is empty",
			args{
				zones:            []zoneType{{Name: "zone-1", Endpoints: []string{"http://rgw-endpoint"}}},
				zoneEndpointList: []string{"http://rgw-endpoint"},
				zoneName:         "",
			},
			false, true,
		},
		{"new endpoint list is same existing containing single zone",
			args{
				zones:            []zoneType{{Name: "zone-1", Endpoints: []string{"http://rgw-endpoint-1"}}},
				zoneEndpointList: []string{"http://rgw-endpoint-1"},
				zoneName:         "zone-1",
			},
			false, false,
		},
		{"new endpoint list to existing list is empty containing single zone",
			args{
				zones:            []zoneType{{Name: "zone-1", Endpoints: []string{}}},
				zoneEndpointList: []string{"http://rgw-endpoint-1"},
				zoneName:         "zone-1",
			},
			true, false,
		},
		{"deleting endpoints from existing list containing single zone",
			args{
				zones:            []zoneType{{Name: "zone-1", Endpoints: []string{"http://rgw-endpoint-1"}}},
				zoneEndpointList: []string{},
				zoneName:         "zone-1",
			},
			true, false,
		},
		{"zone not listed in zonegroup containing single zone",
			args{zones: []zoneType{{Name: "zone-1", Endpoints: []string{"http://rgw-endpoint-1"}}},
				zoneEndpointList: []string{"http://rgw-endpoint-2"},
				zoneName:         "zone-2",
			},
			false, false,
		},
		{"new endpoint list is different from existing list containing single zone",
			args{
				zones:            []zoneType{{Name: "zone-1", Endpoints: []string{"http://rgw-endpoint-1"}}},
				zoneEndpointList: []string{"http://rgw-endpoint-2"},
				zoneName:         "zone-1",
			},
			true, false,
		},
		{"new endpoint list  has multiple entries is different from existing listed containing single zone",
			args{
				zones:            []zoneType{{Name: "zone-1", Endpoints: []string{"http://rgw-endpoint-1"}}},
				zoneEndpointList: []string{"http://rgw-endpoint-1", "http://rgw-endpoint-2"},
				zoneName:         "zone-1",
			},
			true, false,
		},
		{"new endpoint list removed one endpoint from existing list containing single zone",
			args{
				zones:            []zoneType{{Name: "zone-1", Endpoints: []string{"http://rgw-endpoint-1", "http://rgw-endpoint-2"}}},
				zoneEndpointList: []string{"http://rgw-endpoint-1"},
				zoneName:         "zone-1",
			},
			true, false,
		},
		{"new endpoint list is different from existing listed containing single zone",
			args{
				zones:            []zoneType{{Name: "zone-1", Endpoints: []string{"http://rgw-endpoint-1", "http://rgw-endpoint-2"}}},
				zoneEndpointList: []string{"http://rgw-endpoint-3"},
				zoneName:         "zone-1",
			},
			true, false,
		},
		{"new endpoint list is different from existing list but contains one similar endpoint containing single zone",
			args{
				zones:            []zoneType{{Name: "zone-1", Endpoints: []string{"http://rgw-endpoint-1", "http://rgw-endpoint-2"}}},
				zoneEndpointList: []string{"http://rgw-endpoint-2", "http://rgw-endpoint-3"},
				zoneName:         "zone-1",
			},
			true, false,
		},
		{"new endpoint list contains multiple different endpoints from existing list containing single zone",
			args{
				zones:            []zoneType{{Name: "zone-1", Endpoints: []string{"http://rgw-endpoint-1", "http://rgw-endpoint-2"}}},
				zoneEndpointList: []string{"http://rgw-endpoint-3", "http://rgw-endpoint-4"},
				zoneName:         "zone-1",
			},
			true, false,
		},
		{"deleting endpoint list containing multiple zone",
			args{
				zones: []zoneType{
					{Name: "zone-1", Endpoints: []string{"http://rgw-endpoint-12"}},
					{Name: "zone-2", Endpoints: []string{"http://rgw-endpoint-21"}},
				},
				zoneEndpointList: []string{},
				zoneName:         "zone-2",
			},
			true, false,
		},
		{"adding new endpoint list to empty containing multiple zone",
			args{
				zones: []zoneType{
					{Name: "zone-1", Endpoints: []string{"http://rgw-endpoint-11"}},
					{Name: "zone-2", Endpoints: []string{}},
				},
				zoneEndpointList: []string{"http://rgw-endpoint-21", "http://rgw-endpoint-22"},
				zoneName:         "zone-2",
			},
			true, false,
		},
		{"zone not listed containing multiple zone",
			args{
				zones: []zoneType{
					{Name: "zone-1", Endpoints: []string{"http://rgw-endpoint-11", "http://rgw-endpoint-22"}},
					{Name: "zone-2", Endpoints: []string{"http://rgw-endpoint-21"}},
				},
				zoneEndpointList: []string{"http://rgw-endpoint-11"},
				zoneName:         "zone-3",
			},
			false, false,
		},
		{"new endpoint list have one new entry than existing list containing multiple zone",
			args{
				zones: []zoneType{
					{Name: "zone-1", Endpoints: []string{"http://rgw-endpoint-11", "http://rgw-endpoint-12"}},
					{Name: "zone-2", Endpoints: []string{"http://rgw-endpoint-21"}},
				},
				zoneEndpointList: []string{"http://rgw-endpoint-11", "http://rgw-endpoint-12", "http://rgw-endpoint-13"},
				zoneName:         "zone-1",
			},
			true, false,
		},
		{"new endpoint list same as existing list containing multiple zone",
			args{
				zones: []zoneType{
					{Name: "zone-1", Endpoints: []string{"http://rgw-endpoint-11", "http://rgw-endpoint-12"}},
					{Name: "zone-2", Endpoints: []string{"http://rgw-endpoint-21"}},
				},
				zoneEndpointList: []string{"http://rgw-endpoint-12", "http://rgw-endpoint-11"},
				zoneName:         "zone-1",
			},
			false, false,
		},
		{"new endpoint list is different from existing list containing multiple zone",
			args{
				zones: []zoneType{
					{Name: "zone-1", Endpoints: []string{"http://rgw-endpoint-11", "http://rgw-endpoint-12"}},
					{Name: "zone-2", Endpoints: []string{"http://rgw-endpoint-21", "http://rgw-endpoint-22"}},
				},
				zoneEndpointList: []string{"http://rgw-endpoint-3", "http://rgw-endpoint-4"},
				zoneName:         "zone-2",
			},
			true, false,
		},
		{"new endpoint list have duplicate entries, containing multiple zone",
			args{
				zones: []zoneType{
					{Name: "zone-1", Endpoints: []string{"http://rgw-endpoint-11", "http://rgw-endpoint-12"}},
					{Name: "zone-2", Endpoints: []string{"http://rgw-endpoint-21", "http://rgw-endpoint-22"}},
				},
				zoneEndpointList: []string{"http://rgw-endpoint-21", "http://rgw-endpoint-21"},
				zoneName:         "zone-2",
			},
			true, false,
		},
		{"existing endpoint list have duplicate entries, containing multiple zone",
			args{
				zones: []zoneType{
					{Name: "zone-1", Endpoints: []string{"http://rgw-endpoint-11", "http://rgw-endpoint-12"}},
					{Name: "zone-2", Endpoints: []string{"http://rgw-endpoint-21", "http://rgw-endpoint-21"}},
				},
				zoneEndpointList: []string{"http://rgw-endpoint-3", "http://rgw-endpoint-4"},
				zoneName:         "zone-2",
			},
			true, false,
		},
		{"both list have duplicate entries, containing multiple zone",
			args{
				zones: []zoneType{
					{Name: "zone-1", Endpoints: []string{"http://rgw-endpoint-11", "http://rgw-endpoint-12"}},
					{Name: "zone-2", Endpoints: []string{"http://rgw-endpoint-21", "http://rgw-endpoint-21"}},
				},
				zoneEndpointList: []string{"http://rgw-endpoint-22", "http://rgw-endpoint-22"},
				zoneName:         "zone-2",
			},
			true, false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ShouldUpdateZoneEndpointList(tt.args.zones, tt.args.zoneEndpointList, tt.args.zoneName)
			if (err != nil) != tt.wantErr {
				t.Errorf("maxSizeToInt64() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("UpdateZoneEndpointList() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestListsAreEqual(t *testing.T) {
	type args struct {
		listA []string
		listB []string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{"lists are empty",
			args{listA: []string{}, listB: []string{}},
			true,
		},
		{"first list is empty",
			args{listA: []string{"a"}, listB: []string{}},
			false,
		},
		{"second list is empty",
			args{listA: []string{}, listB: []string{"a"}},
			false,
		},
		{"lists are equal with single entry",
			args{listA: []string{"a"}, listB: []string{"a"}},
			true,
		},
		{"lists are equal with multiple entries",
			args{listA: []string{"a", "b"}, listB: []string{"a", "b"}},
			true,
		},
		{"lists have different entries with same length",
			args{listA: []string{"a", "b"}, listB: []string{"c", "d"}},
			false,
		},
		{"lists have similar entries with different length",
			args{listA: []string{"a", "b"}, listB: []string{"a"}},
			false,
		},
		{"lists have some similar entries with same length",
			args{listA: []string{"a", "b"}, listB: []string{"c", "a"}},
			false,
		},
		{"lists have similar entries with same length but order different",
			args{listA: []string{"a", "b"}, listB: []string{"b", "a"}},
			true,
		},
		{"lists have similar entries but contains duplicate",
			args{listA: []string{"a", "b", "b"}, listB: []string{"b", "a", "b"}},
			true,
		},
		{"lists have similar entries but contains duplicate in first",
			args{listA: []string{"a", "b", "b"}, listB: []string{"a", "b"}},
			false,
		},
		{"lists have all similar entries but length is different",
			args{listA: []string{"b", "b", "b"}, listB: []string{"b", "b"}},
			false,
		},
		{"lists have different entries but contains duplicate in first",
			args{listA: []string{"a", "b", "b"}, listB: []string{"c", "d"}},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := listsAreEqual(tt.args.listA, tt.args.listB); got != tt.want {
				t.Errorf("UpdateZoneEndpointList() = %v, want %v", got, tt.want)
			}
		})
	}
}
