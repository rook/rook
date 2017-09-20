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
