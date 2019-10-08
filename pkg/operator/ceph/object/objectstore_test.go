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
	"fmt"
	"testing"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

func TestCreateRealm(t *testing.T) {
	defaultStore := true
	executorFunc := func(debug bool, actionName string, command string, args ...string) (string, error) {
		idResponse := `{"id":"test-id"}`
		logger.Infof("Execute: %s %v", command, args)
		if args[1] == "get" {
			return "", errors.New("induce a create")
		} else if args[1] == "create" {
			for _, arg := range args {
				if arg == "--default" {
					assert.True(t, defaultStore, "did not expect to find --default in %v", args)
					return idResponse, nil
				}
			}
			assert.False(t, defaultStore, "did not find --default flag in %v", args)
		} else if args[0] == "realm" && args[1] == "list" {
			if defaultStore {
				return "", errors.New("failed to run radosgw-admin: Failed to complete : exit status 2")
			}
			return `{"realms": ["myobj"]}`, nil
		}
		return idResponse, nil
	}
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput:         executorFunc,
		MockExecuteCommandWithCombinedOutput: executorFunc,
	}

	storeName := "myobject"
	context := &clusterd.Context{Executor: executor}
	objContext := NewContext(context, storeName, "mycluster")
	// create the first realm, marked as default
	err := createRealm(objContext, "1.2.3.4", 80)
	assert.Nil(t, err)

	// create the second realm, not marked as default
	defaultStore = false
	err = createRealm(objContext, "2.3.4.5", 80)
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
	executor.MockExecuteCommandWithOutputFile = func(debug bool, actionName, command, outputFile string, args ...string) (string, error) {
		//logger.Infof("command: %s %v", command, args)
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
		return "", errors.Errorf("unexpected ceph command %q", args)
	}
	executorFunc := func(debug bool, actionName, command string, args ...string) (string, error) {
		//logger.Infof("Command: %s %v", command, args)
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
	executor.MockExecuteCommandWithOutput = executorFunc
	executor.MockExecuteCommandWithCombinedOutput = executorFunc
	context := &Context{Context: &clusterd.Context{Executor: executor}, Name: "myobj", ClusterName: "ns"}

	// Delete an object store
	err := deleteRealmAndPools(context, false)
	assert.Nil(t, err)
	expectedPoolsDeleted := 6
	if expectedDeleteRootPool {
		expectedPoolsDeleted++
	}
	assert.Equal(t, expectedPoolsDeleted, poolsDeleted)
	assert.Equal(t, expectedPoolsDeleted, rulesDeleted)
	assert.True(t, realmDeleted)
	assert.True(t, zoneGroupDeleted)
	assert.True(t, zoneDeleted)
	assert.Equal(t, expectedDeleteRootPool, deletedRootPool)
	assert.Equal(t, true, deletedErasureCodeProfile)
}
