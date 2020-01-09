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
package client

import (
	"reflect"
	"testing"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/daemon/ceph/model"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"

	"github.com/rook/rook/pkg/clusterd"
)

func TestCreateECPoolWithOverwrites(t *testing.T) {
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
				if args[4] == "allow_ec_overwrites" {
					assert.Equal(t, "mypool", args[3])
					assert.Equal(t, "true", args[5])
					return "", nil
				}
			}
			if args[2] == "application" {
				assert.Equal(t, "enable", args[3])
				assert.Equal(t, "mypool", args[4])
				assert.Equal(t, "myapp", args[5])
				return "", nil
			}
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}

	err := CreateECPoolForApp(context, "myns", p, "myapp", true, model.ErasureCodedPoolConfig{DataChunkCount: 1})
	assert.Nil(t, err)
}

func TestCreateECPoolWithoutOverwrites(t *testing.T) {
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
				assert.Equal(t, "1", args[4])
				return "", nil
			}
			if args[2] == "application" {
				assert.Equal(t, "enable", args[3])
				assert.Equal(t, "mypool", args[4])
				assert.Equal(t, "myapp", args[5])
				return "", nil
			}
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}

	err := CreateECPoolForApp(context, "myns", p, "myapp", false, model.ErasureCodedPoolConfig{DataChunkCount: 1})
	assert.Nil(t, err)
}

func TestCreateReplicaPool(t *testing.T) {
	testCreateReplicaPool(t, "", "", "")
}
func TestCreateReplicaPoolWithFailureDomain(t *testing.T) {
	testCreateReplicaPool(t, "osd", "mycrushroot", "")
}

func TestCreateReplicaPoolWithDeviceClass(t *testing.T) {
	testCreateReplicaPool(t, "osd", "mycrushroot", "hdd")
}

func testCreateReplicaPool(t *testing.T, failureDomain, crushRoot, deviceClass string) {
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
			assert.Equal(t, "rule", args[2])
			assert.Equal(t, "create-replicated", args[3])
			assert.Equal(t, "mypool", args[4])
			if crushRoot == "" {
				assert.Equal(t, "default", args[5])
			} else {
				assert.Equal(t, crushRoot, args[5])
			}
			if failureDomain == "" {
				assert.Equal(t, "host", args[6])
			} else {
				assert.Equal(t, failureDomain, args[6])
			}
			if deviceClass == "" {
				assert.False(t, testIsStringInSlice("hdd", args))
			} else {
				assert.Equal(t, deviceClass, args[7])
			}
			return "", nil
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}

	p := CephStoragePoolDetails{Name: "mypool", Size: 12345, FailureDomain: failureDomain, CrushRoot: crushRoot, DeviceClass: deviceClass}
	err := CreateReplicatedPoolForApp(context, "myns", p, "myapp")
	assert.Nil(t, err)
	assert.True(t, crushRuleCreated)
}

func testIsStringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

func TestGetPoolStatistics(t *testing.T) {
	p := PoolStatistics{}
	p.Images.Count = 1
	p.Images.ProvisionedBytes = 1024
	p.Images.SnapCount = 1
	p.Trash.Count = 1
	p.Trash.ProvisionedBytes = 2048
	p.Trash.SnapCount = 0
	executor := &exectest.MockExecutor{}
	context := &clusterd.Context{Executor: executor}
	executor.MockExecuteCommandWithOutput = func(debug bool, actionName string, command string, args ...string) (string, error) {
		a := "{\"images\":{\"count\":1,\"provisioned_bytes\":1024,\"snap_count\":1},\"trash\":{\"count\":1,\"provisioned_bytes\":2048,\"snap_count\":0}}"
		logger.Infof("Command: %s %v", command, args)

		if args[0] == "pool" {
			if args[1] == "stats" {
				if args[2] == "replicapool" {
					return a, nil
				}
				return "", errors.Errorf("rbd:error opening pool '%s': (2) No such file or directory", args[3])

			}
		}
		return "", errors.Errorf("unexpected rbd command %q", args)
	}

	stats, err := GetPoolStatistics(context, "replicapool", "cluster")
	assert.Nil(t, err)
	assert.True(t, reflect.DeepEqual(stats, &p))

	stats, err = GetPoolStatistics(context, "rbd", "cluster")
	assert.NotNil(t, err)
	assert.Nil(t, stats)
}
