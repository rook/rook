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
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookv1 "github.com/rook/rook/pkg/apis/rook.io/v1"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"

	"github.com/rook/rook/pkg/clusterd"
)

func TestCreateECPoolWithOverwrites(t *testing.T) {
	testCreateECPool(t, true, "")
}

func TestCreateECPoolWithoutOverwrites(t *testing.T) {
	testCreateECPool(t, false, "")
}

func TestCreateECPoolWithCompression(t *testing.T) {
	testCreateECPool(t, false, "aggressive")
	testCreateECPool(t, true, "none")
}

func testCreateECPool(t *testing.T, overwrite bool, compressionMode string) {
	poolName := "mypool"
	compressionModeCreated := false
	p := cephv1.PoolSpec{
		FailureDomain: "host",
		ErasureCoded:  cephv1.ErasureCodedSpec{},
	}
	if compressionMode != "" {
		p.CompressionMode = compressionMode
	}
	executor := &exectest.MockExecutor{}
	context := &clusterd.Context{Executor: executor}
	executor.MockExecuteCommandWithOutputFile = func(command, outputFile string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[1] == "pool" {
			if args[2] == "create" {
				assert.Equal(t, "mypool", args[3])
				assert.Equal(t, "erasure", args[5])
				assert.Equal(t, "mypoolprofile", args[6])
				return "", nil
			}
			if args[2] == "set" {
				assert.Equal(t, "mypool", args[3])
				if args[4] == "allow_ec_overwrites" {
					assert.Equal(t, true, overwrite)
					assert.Equal(t, "true", args[5])
					return "", nil
				}
				if args[4] == "compression_mode" {
					assert.Equal(t, compressionMode, args[5])
					compressionModeCreated = true
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

	err := CreateECPoolForApp(context, AdminClusterInfo("mycluster"), poolName, "mypoolprofile", p, DefaultPGCount, "myapp", overwrite)
	assert.Nil(t, err)
	if compressionMode != "" {
		assert.True(t, compressionModeCreated)
	} else {
		assert.False(t, compressionModeCreated)
	}
}

func TestCreateReplicaPoolWithFailureDomain(t *testing.T) {
	testCreateReplicaPool(t, "osd", "mycrushroot", "", "")
}

func TestCreateReplicaPoolWithDeviceClass(t *testing.T) {
	testCreateReplicaPool(t, "osd", "mycrushroot", "hdd", "")
}

func TestCreateReplicaPoolWithCompression(t *testing.T) {
	testCreateReplicaPool(t, "osd", "mycrushroot", "hdd", "passive")
	testCreateReplicaPool(t, "osd", "mycrushroot", "hdd", "force")
}

func testCreateReplicaPool(t *testing.T, failureDomain, crushRoot, deviceClass, compressionMode string) {
	crushRuleCreated := false
	compressionModeCreated := false
	executor := &exectest.MockExecutor{}
	context := &clusterd.Context{Executor: executor}
	executor.MockExecuteCommandWithOutputFile = func(command, outputFile string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[1] == "pool" {
			if args[2] == "create" {
				assert.Equal(t, "mypool", args[3])
				assert.Equal(t, "replicated", args[5])
				assert.Equal(t, "--size", args[7])
				assert.Equal(t, "12345", args[8])
				return "", nil
			}
			if args[2] == "set" {
				assert.Equal(t, "mypool", args[3])
				if args[4] == "size" {
					assert.Equal(t, "12345", args[5])
				}
				if args[4] == "compression_mode" {
					assert.Equal(t, compressionMode, args[5])
					compressionModeCreated = true
				}
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
				assert.Equal(t, "cluster-crush-root", args[5])
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

	p := cephv1.PoolSpec{
		FailureDomain: failureDomain, CrushRoot: crushRoot, DeviceClass: deviceClass,
		Replicated: cephv1.ReplicatedSpec{Size: 12345},
	}
	if compressionMode != "" {
		p.CompressionMode = compressionMode
	}
	clusterSpec := &cephv1.ClusterSpec{Storage: rookv1.StorageScopeSpec{Config: map[string]string{CrushRootConfigKey: "cluster-crush-root"}}}
	err := CreateReplicatedPoolForApp(context, AdminClusterInfo("mycluster"), clusterSpec, "mypool", p, DefaultPGCount, "myapp")
	assert.Nil(t, err)
	assert.True(t, crushRuleCreated)
	if compressionMode != "" {
		assert.True(t, compressionModeCreated)
	} else {
		assert.False(t, compressionModeCreated)
	}
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
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
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

	clusterInfo := AdminClusterInfo("mycluster")
	stats, err := GetPoolStatistics(context, clusterInfo, "replicapool")
	assert.Nil(t, err)
	assert.True(t, reflect.DeepEqual(stats, &p))

	stats, err = GetPoolStatistics(context, clusterInfo, "rbd")
	assert.NotNil(t, err)
	assert.Nil(t, stats)
}

func TestSetPoolReplicatedSizeProperty(t *testing.T) {
	poolName := "mypool"
	executor := &exectest.MockExecutor{}
	context := &clusterd.Context{Executor: executor}
	executor.MockExecuteCommandWithOutputFile = func(command, outputFile string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)

		if args[2] == "set" {
			assert.Equal(t, poolName, args[3])
			assert.Equal(t, "size", args[4])
			assert.Equal(t, "3", args[5])
			return "", nil
		}

		return "", errors.Errorf("unexpected ceph command %q", args)
	}

	err := SetPoolReplicatedSizeProperty(context, AdminClusterInfo("mycluster"), poolName, "3")
	assert.NoError(t, err)

	// TEST POOL SIZE 1 AND RequireSafeReplicaSize True
	executor.MockExecuteCommandWithOutputFile = func(command, outputFile string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)

		if args[2] == "set" {
			assert.Equal(t, "mypool", args[3])
			assert.Equal(t, "size", args[4])
			assert.Equal(t, "1", args[5])
			assert.Equal(t, "--yes-i-really-mean-it", args[6])
			return "", nil
		}

		return "", errors.Errorf("unexpected ceph command %q", args)
	}

	err = SetPoolReplicatedSizeProperty(context, AdminClusterInfo("mycluster"), poolName, "1")
	assert.NoError(t, err)
}

func TestCreateStretchCrushRule(t *testing.T) {
	testCreateStretchCrushRule(t, true)
	testCreateStretchCrushRule(t, false)
}

func testCreateStretchCrushRule(t *testing.T, alreadyExists bool) {
	executor := &exectest.MockExecutor{}
	context := &clusterd.Context{Executor: executor}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[0] == "osd" {
			if args[1] == "getcrushmap" {
				return "", nil
			}
			if args[1] == "setcrushmap" {
				if alreadyExists {
					return "", errors.New("setcrushmap not expected for already existing crush rule")
				}
				return "", nil
			}
		}
		if command == "crushtool" {
			switch {
			case args[0] == "--decompile" || args[0] == "--compile":
				if alreadyExists {
					return "", errors.New("--compile or --decompile not expected for already existing crush rule")
				}
				return "", nil
			}
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}
	executor.MockExecuteCommandWithOutputFile = func(command, outputFile string, args ...string) (string, error) {
		logger.Infof("Command (file): %s %v", command, args)
		if args[0] == "osd" && args[1] == "crush" && args[2] == "dump" {
			return testCrushMap, nil
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}
	clusterInfo := AdminClusterInfo("mycluster")
	clusterSpec := &cephv1.ClusterSpec{}
	poolSpec := cephv1.PoolSpec{FailureDomain: "rack"}
	ruleName := "testrule"
	if alreadyExists {
		ruleName = "replicated_ruleset"
	}

	err := createTwoStepCrushRule(context, clusterInfo, clusterSpec, ruleName, poolSpec)
	assert.NoError(t, err)
}
