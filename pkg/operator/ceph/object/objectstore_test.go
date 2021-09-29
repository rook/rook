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
	"os"
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
	access_key = "VFKF8SSU9L3L2UR03Z8C"
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
	err := setMultisite(objContext, &store, "1.2.3.4")
	assert.Nil(t, err)

	// create the second realm, not marked as default
	err = setMultisite(objContext, &store, "2.3.4.5")
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
	context := &Context{Context: &clusterd.Context{Executor: executor}, Name: "myobj", clusterInfo: client.AdminClusterInfo("mycluster")}

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
	expectedPoolsDeleted = 6
	if expectedDeleteRootPool {
		expectedPoolsDeleted++
	}
	assert.Equal(t, expectedPoolsDeleted, poolsDeleted)
	assert.Equal(t, expectedDeleteRootPool, deletedRootPool)
	assert.Equal(t, true, deletedErasureCodeProfile)
}

func TestGetObjectBucketProvisioner(t *testing.T) {
	testNamespace := "test-namespace"
	os.Setenv(k8sutil.PodNamespaceEnvVar, testNamespace)

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

func TestDashboard(t *testing.T) {
	storeName := "myobject"
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			return "", nil
		},
		MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, args ...string) (string, error) {
			if args[0] == "user" {
				return dashboardAdminCreateJSON, nil
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

	checkdashboard, err := checkDashboardUser(objContext)
	assert.NoError(t, err)
	assert.False(t, checkdashboard)
	err = enableRGWDashboard(objContext)
	assert.Nil(t, err)
	executor = &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			if args[0] == "dashboard" && args[1] == "get-rgw-api-access-key" {
				return access_key, nil
			}
			return "", nil
		},
	}
	objContext.Context.Executor = executor
	checkdashboard, err = checkDashboardUser(objContext)
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
	assert.Nil(t, err)
	executor = &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			if args[0] == "dashboard" && args[1] == "get-rgw-api-access-key" {
				return access_key, nil
			}
			return "", nil
		},
	}
	objContext.Context.Executor = executor
	checkdashboard, err = checkDashboardUser(objContext)
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
		realmExists         bool
		zoneGroupExists     bool
		zoneExists          bool
		periodExists        bool
		failCreateRealm     bool
		failCreateZoneGroup bool
		failCreateZone      bool
		failUpdatePeriod    bool
	}

	// control whether we should expect certain 'get' calls
	type expectCommands struct {
		getRealm        bool
		createRealm     bool
		getZoneGroup    bool
		createZoneGroup bool
		getZone         bool
		createZone      bool
		getPeriod       bool
		updatePeriod    bool
	}

	// vars used for testing if calls were made
	var (
		calledGetRealm        = false
		calledGetZoneGroup    = false
		calledGetZone         = false
		calledGetPeriod       = false
		calledCreateRealm     = false
		calledCreateZoneGroup = false
		calledCreateZone      = false
		calledUpdatePeriod    = false
	)

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
		calledGetPeriod = false
		calledUpdatePeriod = false

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
					case "period":
						switch arg[1] {
						case "get":
							calledGetPeriod = true
							return enoentIfNotExist(env.periodExists)
						case "update":
							calledUpdatePeriod = true
							return errorIfFail(env.failUpdatePeriod)
						}
					}
				}
				t.Fatalf("unhandled command: %s %v", command, arg)
				return "", nil
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
		{"create realm, zonegroup, and zone; period update",
			commandReturns{
				// nothing exists, and all should succeed
			},
			expectCommands{
				getRealm:        true,
				createRealm:     true,
				getZoneGroup:    true,
				createZoneGroup: true,
				getZone:         true,
				createZone:      true,
				getPeriod:       true,
				updatePeriod:    true,
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
		{"fail period update",
			commandReturns{
				failUpdatePeriod: true,
			},
			expectCommands{
				getRealm:        true,
				createRealm:     true,
				getZoneGroup:    true,
				createZoneGroup: true,
				getZone:         true,
				createZone:      true,
				getPeriod:       true,
				updatePeriod:    true,
			},
			expectErr},
		{"realm exists; create zonegroup and zone; period update",
			commandReturns{
				realmExists: true,
			},
			expectCommands{
				getRealm:        true,
				createRealm:     false,
				getZoneGroup:    true,
				createZoneGroup: true,
				getZone:         true,
				createZone:      true,
				getPeriod:       true,
				updatePeriod:    true,
			},
			expectNoErr},
		{"realm and zonegroup exist; create zone; period update",
			commandReturns{
				realmExists:     true,
				zoneGroupExists: true,
			},
			expectCommands{
				getRealm:        true,
				createRealm:     false,
				getZoneGroup:    true,
				createZoneGroup: false,
				getZone:         true,
				createZone:      true,
				getPeriod:       true,
				updatePeriod:    true,
			},
			expectNoErr},
		{"realm, zonegroup, and zone exist; period update",
			commandReturns{
				realmExists:     true,
				zoneGroupExists: true,
				zoneExists:      true,
			},
			expectCommands{
				getRealm:        true,
				createRealm:     false,
				getZoneGroup:    true,
				createZoneGroup: false,
				getZone:         true,
				createZone:      false,
				getPeriod:       true,
				updatePeriod:    true,
			},
			expectNoErr},
		{"realm, zonegroup, zone, and period all exist",
			commandReturns{
				realmExists:     true,
				zoneGroupExists: true,
				zoneExists:      true,
				periodExists:    true,
			},
			expectCommands{
				getRealm:        true,
				createRealm:     false,
				getZoneGroup:    true,
				createZoneGroup: false,
				getZone:         true,
				createZone:      false,
				getPeriod:       true,
				updatePeriod:    false,
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
			assert.Equal(t, tt.expectCommands.getPeriod, calledGetPeriod)
			assert.Equal(t, tt.expectCommands.updatePeriod, calledUpdatePeriod)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
