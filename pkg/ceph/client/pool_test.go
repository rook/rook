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

Some of the code below came from https://github.com/digitalocean/ceph_exporter
which has the same license.
*/
package client

import (
	"fmt"
	"testing"

	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"

	"github.com/rook/rook/pkg/clusterd"
)

func TestCreateECPool(t *testing.T) {
	p := CephStoragePoolDetails{Name: "mypool", Size: 12345, ErasureCodeProfile: "myecprofile", FailureDomain: "host"}
	executor := &exectest.MockExecutor{}
	context := &clusterd.Context{Executor: executor}
	executor.MockExecuteCommandWithOutputFile = func(debug bool, actionName, command, outputFile string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[1] == "pool" {
			if args[2] == "create" {
				assert.Equal(t, "mypool", args[3])
				assert.Equal(t, "erasure", args[5])
				assert.Equal(t, p.ErasureCodeProfile, args[6])
				return "", nil
			}
			if args[2] == "set" {
				assert.Equal(t, "mypool", args[3])
				assert.Equal(t, "size", args[4])
				assert.Equal(t, "12345", args[5])
				return "", nil
			}
			if args[2] == "application" {
				assert.Equal(t, "enable", args[3])
				assert.Equal(t, "mypool", args[4])
				assert.Equal(t, "myapp", args[5])
				return "", nil
			}
		}
		return "", fmt.Errorf("unexpected ceph command '%v'", args)
	}

	err := CreatePoolForApp(context, "myns", p, "myapp")
	assert.Nil(t, err)
}

func TestCreateReplicaPool(t *testing.T) {
	testCreateReplicaPool(t, "")
}
func TestCreateReplicaPoolWithFailureDomain(t *testing.T) {
	testCreateReplicaPool(t, "osd")
}

func testCreateReplicaPool(t *testing.T, failureDomain string) {
	crushRuleCreated := false
	executor := &exectest.MockExecutor{}
	context := &clusterd.Context{Executor: executor}
	executor.MockExecuteCommandWithOutputFile = func(debug bool, actionName, command, outputFile string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[1] == "pool" {
			if args[2] == "create" {
				assert.Equal(t, "mypool", args[3])
				assert.Equal(t, "replicated", args[5])
				return "", nil
			}
			if args[2] == "set" {
				assert.Equal(t, "mypool", args[3])
				assert.Equal(t, "size", args[4])
				assert.Equal(t, "12345", args[5])
				return "", nil
			}
			if args[2] == "application" {
				assert.Equal(t, "enable", args[3])
				assert.Equal(t, "mypool", args[4])
				assert.Equal(t, "myapp", args[5])
				return "", nil
			}
		}
		if args[1] == "crush" {
			crushRuleCreated = true
			assert.NotEqual(t, "", failureDomain)
			assert.Equal(t, "rule", args[2])
			assert.Equal(t, "create-simple", args[3])
			assert.Equal(t, "mypool", args[4])
			assert.Equal(t, "default", args[5])
			assert.Equal(t, failureDomain, args[6])
			return "", nil
		}
		return "", fmt.Errorf("unexpected ceph command '%v'", args)
	}

	p := CephStoragePoolDetails{Name: "mypool", Size: 12345, FailureDomain: failureDomain}
	err := CreatePoolForApp(context, "myns", p, "myapp")
	assert.Nil(t, err)
	if failureDomain == "" {
		assert.False(t, crushRuleCreated)
	} else {
		assert.True(t, crushRuleCreated)
	}
}
