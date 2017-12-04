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
package rgw

import (
	"fmt"
	"testing"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/model"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

func TestCreateRealm(t *testing.T) {
	defaultStore := true
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithCombinedOutput: func(debug bool, actionName string, command string, args ...string) (string, error) {
			idResponse := `{"id":"test-id"}`
			logger.Infof("Execute: %s %v", command, args)
			if args[1] == "get" {
				return "", fmt.Errorf("induce a create")
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
					return "", fmt.Errorf("failed to run radosgw-admin: Failed to complete : exit status 2")
				} else {
					return `{"realms": ["myobj"]}`, nil
				}
			}
			return idResponse, nil
		},
	}

	store := model.ObjectStore{Name: "myobject", Gateway: model.Gateway{Port: 123}}
	context := &clusterd.Context{Executor: executor}
	objContext := NewContext(context, store.Name, "mycluster")
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
		}
		return "", fmt.Errorf("unexpected ceph command '%v'", args)
	}
	executor.MockExecuteCommandWithCombinedOutput = func(debug bool, actionName, command string, args ...string) (string, error) {
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
		return "", fmt.Errorf("unexpected ceph command '%v'", args)
	}
	context := &Context{context: &clusterd.Context{Executor: executor}, Name: "myobj", ClusterName: "ns"}

	// Delete an object store
	err := DeleteObjectStore(context)
	assert.Nil(t, err)
	expectedPoolsDeleted := 5
	if expectedDeleteRootPool {
		expectedPoolsDeleted++
	}
	assert.Equal(t, expectedPoolsDeleted, poolsDeleted)
	assert.Equal(t, expectedPoolsDeleted, rulesDeleted)
	assert.True(t, realmDeleted)
	assert.True(t, zoneGroupDeleted)
	assert.True(t, zoneDeleted)
	assert.Equal(t, expectedDeleteRootPool, deletedRootPool)
}
