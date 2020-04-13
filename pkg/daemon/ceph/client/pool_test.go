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
	"fmt"
	"reflect"
	"strconv"
	"testing"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"

	"github.com/rook/rook/pkg/clusterd"
)

func TestCreateECPoolWithOverwrites(t *testing.T) {
	testCreateECPool(t, true, "", false)
	testCreateECPool(t, true, "", true)
}

func TestCreateECPoolWithoutOverwrites(t *testing.T) {
	testCreateECPool(t, false, "", false)
}

func TestCreateECPoolWithCompression(t *testing.T) {
	testCreateECPool(t, false, "aggressive", false)
	testCreateECPool(t, true, "none", false)
	testCreateECPool(t, true, "none", true)
}

func testCreateECPool(t *testing.T, overwrite bool, compressionMode string, poolExist bool) {
	poolName := "mypool"
	compressionModeCreated := false
	poolCreated := false
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
				poolCreated = true
				assert.Equal(t, poolName, args[3])
				assert.Equal(t, "erasure", args[5])
				assert.Equal(t, fmt.Sprintf("%s_ecprofile", poolName), args[6])
				return "", nil
			}
			if args[2] == "get" {
				assert.Equal(t, "mypool", args[3])
				assert.Equal(t, "all", args[4])
				if poolExist {
					return "", nil
				} else {
					return "", errors.Errorf("pool not found")
				}
			}
			if args[2] == "set" {
				assert.Equal(t, poolName, args[3])
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
				assert.Equal(t, poolName, args[4])
				assert.Equal(t, "myapp", args[5])
				return "", nil
			}
		}
		if args[1] == "erasure-code-profile" {
			if args[2] == "get" {
				return `{"plugin":"myplugin","technique":"t"}`, nil
			}
			if args[2] == "set" {
				return "", nil
			}
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}

	err := CreateECPoolForApp(context, "myns", poolName, p, DefaultPGCount, "myapp", overwrite)
	assert.Nil(t, err)
	if poolExist {
		assert.False(t, poolCreated)
	} else {
		assert.True(t, poolCreated)
	}
	if compressionMode != "" {
		assert.True(t, compressionModeCreated)
	} else {
		assert.False(t, compressionModeCreated)
	}
}

func TestCreateReplicaPool(t *testing.T) {
	testCreateReplicaPool(t, "", "", "", "", DefaultPGCount, "", "", false)
	testCreateReplicaPool(t, "", "", "", "", DefaultPGCount, "", "", true)
}
func TestCreateReplicaPoolWithPgCount(t *testing.T) {
	testCreateReplicaPool(t, "", "", "", "", "8", "", "", false)
	// Does not create pool as it exist but apply pgCount
	testCreateReplicaPool(t, "", "", "", "", "8", "", "", true)
	// Does not create pool as it exist and don't apply pgCount as pgCount already fine
	testCreateReplicaPool(t, "", "", "", "", "8", "", "{\"pg_num_min\":8}", true)
}
func TestCreateReplicaPoolTargetSizeRatio(t *testing.T) {
	testCreateReplicaPool(t, "", "", "", "", DefaultPGCount, "0.2", "", false)
	// Does not create pool as it exist but apply targetSizeRatio
	testCreateReplicaPool(t, "", "", "", "", DefaultPGCount, "0.2", "", true)
	// Does not create pool as it exist and don't apply targetSizeRatio as targetSizeRatio already fine
	testCreateReplicaPool(t, "", "", "", "", DefaultPGCount, "0.2", "{\"target_size_ratio\":0.2}", true)
}
func TestCreateReplicaPoolWithFailureDomain(t *testing.T) {
	testCreateReplicaPool(t, "osd", "mycrushroot", "", "", DefaultPGCount, "", "", false)
}

func TestCreateReplicaPoolWithDeviceClass(t *testing.T) {
	testCreateReplicaPool(t, "osd", "mycrushroot", "hdd", "", DefaultPGCount, "", "", false)
}

func TestCreateReplicaPoolWithCompression(t *testing.T) {
	testCreateReplicaPool(t, "osd", "mycrushroot", "hdd", "passive", DefaultPGCount, "", "", false)
	testCreateReplicaPool(t, "osd", "mycrushroot", "hdd", "force", DefaultPGCount, "", "", false)
	testCreateReplicaPool(t, "osd", "mycrushroot", "hdd", "force", DefaultPGCount, "", "", true)
}

func testCreateReplicaPool(t *testing.T, failureDomain, crushRoot, deviceClass, compressionMode, pgCount, targetSizeRatio, poolConfig string, poolExist bool) {
	crushRuleCreated := false
	poolCreated := false
	compressionModeCreated := false
	pgNumMinSet := false
	targetSizeRatioSet := false
	firstGetPoolCall := true
	executor := &exectest.MockExecutor{}
	context := &clusterd.Context{Executor: executor}
	executor.MockExecuteCommandWithOutputFile = func(command, outputFile string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[1] == "pool" {
			if args[2] == "create" {
				poolCreated = true
				assert.Equal(t, "mypool", args[3])
				assert.Equal(t, "replicated", args[5])
				assert.Equal(t, "--size", args[7])
				assert.Equal(t, "12345", args[8])
				return "", nil
			}
			if args[2] == "get" {
				assert.Equal(t, "mypool", args[3])
				assert.Equal(t, "all", args[4])
				if poolExist || !firstGetPoolCall {
					return poolConfig, nil
				} else {
					firstGetPoolCall = false
					return "", errors.Errorf("pool not found")
				}
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
				if args[4] == "pg_num_min" {
					assert.Equal(t, pgCount, args[5])
					pgNumMinSet = true
				}
				if args[4] == "target_size_ratio" {
					assert.Equal(t, targetSizeRatio, args[5])
					targetSizeRatioSet = true
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

	p := cephv1.PoolSpec{
		FailureDomain: failureDomain, CrushRoot: crushRoot, DeviceClass: deviceClass,
		Replicated: cephv1.ReplicatedSpec{Size: 12345},
	}
	if compressionMode != "" {
		p.CompressionMode = compressionMode
	}
	if targetSizeRatio != "" {
		fTargetSizeRatio, _ := strconv.ParseFloat(targetSizeRatio, 64)
		p.Replicated.TargetSizeRatio = fTargetSizeRatio
	}
	err := CreateReplicatedPoolForApp(context, "myns", "mypool", p, pgCount, "myapp")
	poolDetails, err := GetPoolDetails(context, "myns", "mypool")
	assert.Nil(t, err)
	if poolExist {
		assert.False(t, crushRuleCreated)
		assert.False(t, poolCreated)
	} else {
		assert.True(t, crushRuleCreated)
		assert.True(t, poolCreated)
	}
	if compressionMode != "" {
		assert.True(t, compressionModeCreated)
	} else {
		assert.False(t, compressionModeCreated)
	}
	if (pgCount != DefaultPGCount) && (pgCount != strconv.FormatUint(uint64(poolDetails.PgNumMin), 10)) {
		assert.True(t, pgNumMinSet)
	} else {
		assert.False(t, pgNumMinSet)
	}
	if (targetSizeRatio != "") && (targetSizeRatio != strconv.FormatFloat(poolDetails.TargetSizeRatio, 'f', -1, 64)) {
		assert.True(t, targetSizeRatioSet)
	} else {
		assert.False(t, targetSizeRatioSet)
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

	stats, err := GetPoolStatistics(context, "replicapool", "cluster")
	assert.Nil(t, err)
	assert.True(t, reflect.DeepEqual(stats, &p))

	stats, err = GetPoolStatistics(context, "rbd", "cluster")
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

	err := SetPoolReplicatedSizeProperty(context, "myns", poolName, "3")
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

	err = SetPoolReplicatedSizeProperty(context, "myns", poolName, "1")
	assert.NoError(t, err)
}
