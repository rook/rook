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
	"encoding/json"
	"fmt"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

const emptyApplicationName = `{"":{}}`

var emptyParameters = map[string]string{}

func TestCreateECPoolWithOverwrites(t *testing.T) {
	testCreateECPool(t, true, "", emptyParameters)
}

func TestCreateECPoolWithoutOverwrites(t *testing.T) {
	testCreateECPool(t, false, "", emptyParameters)
}

func TestCreateECPoolWithCompression(t *testing.T) {
	testCreateECPool(t, false, "aggressive", emptyParameters)
	testCreateECPool(t, true, "none", emptyParameters)
}

func TestCreateECPoolWithPgNumMin(t *testing.T) {
	parameters := map[string]string{"pg_num_min": "128"}
	testCreateECPool(t, false, "", parameters)
}

func testCreateECPool(t *testing.T, overwrite bool, compressionMode string, parameters map[string]string) {
	compressionModeCreated := false
	pgNumMinSet, pgNumSet, pgpNumSet := false, false, false
	expectedPgNumMin, expectedPgNumMinSet := parameters["pg_num_min"]
	p := cephv1.NamedPoolSpec{
		Name: "mypool",
		PoolSpec: cephv1.PoolSpec{
			FailureDomain: "host",
			ErasureCoded:  cephv1.ErasureCodedSpec{},
			Parameters:    parameters,
		},
	}
	if compressionMode != "" {
		p.CompressionMode = compressionMode
	}
	executor := &exectest.MockExecutor{}
	context := &clusterd.Context{Executor: executor}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[1] == "pool" {
			if args[2] == "create" {
				assert.Equal(t, "mypool", args[3])
				assert.Equal(t, DefaultPGCount, args[4])
				assert.Equal(t, "erasure", args[5])
				assert.Equal(t, "mypoolprofile", args[6])
				return "", nil
			}
			if args[2] == "get" {
				assert.Equal(t, "mypool", args[3])
				assert.Equal(t, "all", args[4])
				return "{\"pool\":\"mypool\",\"pool_id\":5,\"size\":3,\"min_size\":2,\"pg_num\":8,\"pgp_num\":8,\"crush_rule\":\"mypool\",\"hashpspool\":true,\"nodelete\":false,\"nopgchange\":false,\"nosizechange\":false,\"write_fadvise_dontneed\":false,\"noscrub\":false,\"nodeep-scrub\":false,\"use_gmt_hitset\":true,\"fast_read\":0,\"recovery_priority\":5,\"pg_autoscale_mode\":\"on\",\"pg_autoscale_bias\":4,\"bulk\":false}", nil
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
				if args[4] == "pg_autoscale_mode" {
					assert.True(t, args[5] == "on" || args[5] == "off")
					return "", nil
				}
				if expectedPgNumMinSet {
					if args[4] == "pg_num_min" {
						assert.Equal(t, expectedPgNumMin, args[5])
						pgNumMinSet = true
						return "", nil
					}
					if args[4] == "pg_num" {
						assert.Equal(t, expectedPgNumMin, args[5])
						pgNumSet = true
						return "", nil
					}
					if args[4] == "pgp_num" {
						assert.Equal(t, expectedPgNumMin, args[5])
						pgpNumSet = true
						return "", nil
					}
				}
			}
			if args[2] == "application" {
				if args[3] == "get" {
					return emptyApplicationName, nil
				}
				assert.Equal(t, "enable", args[3])
				assert.Equal(t, "mypool", args[4])
				assert.Equal(t, "myapp", args[5])
				return "", nil
			}
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}

	err := createECPoolForApp(context, AdminTestClusterInfo("mycluster"), "mypoolprofile", p, DefaultPGCount, "myapp", overwrite)
	assert.Nil(t, err)
	if compressionMode != "" {
		assert.True(t, compressionModeCreated)
	} else {
		assert.False(t, compressionModeCreated)
	}

	if expectedPgNumMinSet {
		assert.True(t, pgNumMinSet)
		assert.True(t, pgNumSet)
		assert.True(t, pgpNumSet)
	}
}

func TestSetPoolApplication(t *testing.T) {
	poolName := "testpool"
	appName := "testapp"
	setAppName := false
	blankAppName := false
	clusterInfo := AdminTestClusterInfo("mycluster")
	executor := &exectest.MockExecutor{}
	context := &clusterd.Context{Executor: executor}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[1] == "pool" && args[2] == "application" {
			if args[3] == "get" {
				assert.Equal(t, poolName, args[4])
				if blankAppName {
					return emptyApplicationName, nil
				} else {
					return fmt.Sprintf(`{"%s":{}}`, appName), nil
				}
			}
			if args[3] == "enable" {
				setAppName = true
				assert.Equal(t, poolName, args[4])
				assert.Equal(t, appName, args[5])
				return "", nil
			}
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}

	t.Run("set pool application", func(t *testing.T) {
		setAppName = false
		blankAppName = true
		err := givePoolAppTag(context, clusterInfo, poolName, appName)
		assert.NoError(t, err)
		assert.True(t, setAppName)
	})

	t.Run("pool application already set", func(t *testing.T) {
		setAppName = false
		blankAppName = false
		err := givePoolAppTag(context, clusterInfo, poolName, appName)
		assert.NoError(t, err)
		assert.False(t, setAppName)
	})

}

func TestCreateReplicaPoolWithFailureDomain(t *testing.T) {
	testCreateReplicaPool(t, "osd", "mycrushroot", "", "", emptyParameters)
}

func TestCreateReplicaPoolWithDeviceClass(t *testing.T) {
	testCreateReplicaPool(t, "osd", "mycrushroot", "hdd", "", emptyParameters)
}

func TestCreateReplicaPoolWithCompression(t *testing.T) {
	testCreateReplicaPool(t, "osd", "mycrushroot", "hdd", "passive", emptyParameters)
	testCreateReplicaPool(t, "osd", "mycrushroot", "hdd", "force", emptyParameters)
}

func TestCreateReplicaPoolWithPgNumMin(t *testing.T) {
	parameters := map[string]string{"pg_num_min": "128"}
	testCreateReplicaPool(t, "osd", "mycrushroot", "", "", parameters)
}

func testCreateReplicaPool(t *testing.T, failureDomain, crushRoot, deviceClass, compressionMode string, parameters map[string]string) {
	crushRuleCreated := false
	poolCreated := false
	compressionModeCreated := false
	pgNumMinSet, pgNumSet, pgpNumSet := false, false, false
	expectedPgNumMin, expectedPgNumMinSet := parameters["pg_num_min"]
	executor := &exectest.MockExecutor{}
	context := &clusterd.Context{Executor: executor}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[1] == "pool" {
			if args[2] == "create" {
				assert.Equal(t, "mypool", args[3])
				assert.Equal(t, DefaultPGCount, args[4])
				assert.Equal(t, "replicated", args[5])
				assert.Equal(t, "--size", args[7])
				assert.Equal(t, "12345", args[8])
				poolCreated = true
				return "", nil
			}
			if args[2] == "get" {
				assert.Equal(t, "mypool", args[3])
				if !poolCreated {
					return "", errors.New("pool doesn't exist")
				} else {
					return "{\"pool\":\"mypool\",\"pool_id\":5,\"size\":3,\"min_size\":2,\"pg_num\":8,\"pgp_num\":8,\"crush_rule\":\"mypool\",\"hashpspool\":true,\"nodelete\":false,\"nopgchange\":false,\"nosizechange\":false,\"write_fadvise_dontneed\":false,\"noscrub\":false,\"nodeep-scrub\":false,\"use_gmt_hitset\":true,\"fast_read\":0,\"recovery_priority\":5,\"pg_autoscale_mode\":\"on\",\"pg_autoscale_bias\":4,\"bulk\":false}", nil
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
				if expectedPgNumMinSet {
					if args[4] == "pg_num_min" {
						assert.Equal(t, expectedPgNumMin, args[5])
						pgNumMinSet = true
					}
					if args[4] == "pg_num" {
						assert.Equal(t, expectedPgNumMin, args[5])
						pgNumSet = true
					}
					if args[4] == "pgp_num" {
						assert.Equal(t, expectedPgNumMin, args[5])
						pgpNumSet = true
					}
				}
				return "", nil
			}
			if args[2] == "application" {
				if args[3] == "get" {
					return emptyApplicationName, nil
				}
				assert.Equal(t, "enable", args[3])
				assert.Equal(t, "mypool", args[4])
				assert.Equal(t, "myapp", args[5])
				return "", nil
			}
		}
		if args[1] == "crush" {
			assert.Equal(t, "rule", args[2])
			if args[3] == "create-replicated" {
				crushRuleCreated = true
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
			if args[3] == "dump" {
				assert.Equal(t, "mypool", args[4])
				if crushRuleCreated {
					return `{"rule_id":15,"rule_name":"mypool","ruleset":15,"type":1,"min_size":1,"max_size":10,"steps":[{"op":"take","item":-2,"item_name":"default"},{"op":"chooseleaf_firstn","num":0,"type":"` + failureDomain + `"},{"op":"emit"}]}`, nil
				} else {
					return "", errors.New("crush rule doesn't exist")
				}
			}
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}

	p := cephv1.NamedPoolSpec{
		Name: "mypool",
		PoolSpec: cephv1.PoolSpec{
			FailureDomain: failureDomain, CrushRoot: crushRoot, DeviceClass: deviceClass,
			Replicated: cephv1.ReplicatedSpec{Size: 12345},
			Parameters: parameters,
		},
	}
	if compressionMode != "" {
		p.CompressionMode = compressionMode
	}
	clusterSpec := &cephv1.ClusterSpec{Storage: cephv1.StorageScopeSpec{Config: map[string]string{CrushRootConfigKey: "cluster-crush-root"}}}
	err := createReplicatedPoolForApp(context, AdminTestClusterInfo("mycluster"), clusterSpec, p, DefaultPGCount, "myapp")
	assert.Nil(t, err)
	assert.True(t, crushRuleCreated)
	if compressionMode != "" {
		assert.True(t, compressionModeCreated)
	} else {
		assert.False(t, compressionModeCreated)
	}
	if expectedPgNumMinSet {
		assert.True(t, pgNumMinSet)
		assert.True(t, pgNumSet)
		assert.True(t, pgpNumSet)
	}
}

func TestUpdateFailureDomain(t *testing.T) {
	var newCrushRule string
	currentFailureDomain := "rack"
	executor := &exectest.MockExecutor{}
	context := &clusterd.Context{Executor: executor}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[1] == "pool" {
			if args[2] == "get" {
				assert.Equal(t, "mypool", args[3])
				return `{"crush_rule": "test_rule"}`, nil
			}
			if args[2] == "set" {
				assert.Equal(t, "mypool", args[3])
				assert.Equal(t, "crush_rule", args[4])
				newCrushRule = args[5]
				return "", nil
			}
		}
		if args[1] == "crush" {
			if args[2] == "rule" && args[3] == "dump" {
				return fmt.Sprintf(`{"steps": [{"foo":"bar"},{"type":"%s"}]}`, currentFailureDomain), nil
			}
			newCrushRule = "foo"
			return "", nil
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}

	t.Run("no desired failure domain", func(t *testing.T) {
		p := cephv1.NamedPoolSpec{
			Name: "mypool",
			PoolSpec: cephv1.PoolSpec{
				Replicated: cephv1.ReplicatedSpec{Size: 3},
			},
		}
		clusterSpec := &cephv1.ClusterSpec{Storage: cephv1.StorageScopeSpec{}}
		err := ensureFailureDomain(context, AdminTestClusterInfo("mycluster"), clusterSpec, p)
		assert.NoError(t, err)
		assert.Equal(t, "", newCrushRule)
	})

	t.Run("same failure domain", func(t *testing.T) {
		p := cephv1.NamedPoolSpec{
			Name: "mypool",
			PoolSpec: cephv1.PoolSpec{
				FailureDomain: currentFailureDomain,
				Replicated:    cephv1.ReplicatedSpec{Size: 3},
			},
		}
		clusterSpec := &cephv1.ClusterSpec{Storage: cephv1.StorageScopeSpec{}}
		err := ensureFailureDomain(context, AdminTestClusterInfo("mycluster"), clusterSpec, p)
		assert.NoError(t, err)
		assert.Equal(t, "", newCrushRule)
	})

	t.Run("changing failure domain", func(t *testing.T) {
		p := cephv1.NamedPoolSpec{
			Name: "mypool",
			PoolSpec: cephv1.PoolSpec{
				FailureDomain: "zone",
				Replicated:    cephv1.ReplicatedSpec{Size: 3},
			},
		}
		clusterSpec := &cephv1.ClusterSpec{Storage: cephv1.StorageScopeSpec{}}
		err := ensureFailureDomain(context, AdminTestClusterInfo("mycluster"), clusterSpec, p)
		assert.NoError(t, err)
		assert.Equal(t, "mypool_zone", newCrushRule)
	})
}

func TestExtractFailureDomain(t *testing.T) {
	t.Run("complex crush rule skipped", func(t *testing.T) {
		rule := ruleSpec{Steps: []stepSpec{
			{Type: ""},
			{Type: ""},
			{Type: "zone"},
		}}
		failureDomain := extractFailureDomain(rule)
		assert.Equal(t, "", failureDomain)
	})
	t.Run("valid crush rule", func(t *testing.T) {
		rule := ruleSpec{Steps: []stepSpec{
			{Type: ""},
			{Type: "zone"},
		}}
		failureDomain := extractFailureDomain(rule)
		assert.Equal(t, "zone", failureDomain)
	})
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

	clusterInfo := AdminTestClusterInfo("mycluster")
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
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)

		if args[2] == "set" {
			assert.Equal(t, poolName, args[3])
			assert.Equal(t, "size", args[4])
			assert.Equal(t, "3", args[5])
			return "", nil
		}

		return "", errors.Errorf("unexpected ceph command %q", args)
	}

	err := SetPoolReplicatedSizeProperty(context, AdminTestClusterInfo("mycluster"), poolName, "3")
	assert.NoError(t, err)

	// TEST POOL SIZE 1 AND RequireSafeReplicaSize True
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
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

	err = SetPoolReplicatedSizeProperty(context, AdminTestClusterInfo("mycluster"), poolName, "1")
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
		if args[0] == "osd" && args[1] == "crush" && args[2] == "dump" {
			return testCrushMap, nil
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}
	clusterInfo := AdminTestClusterInfo("mycluster")
	clusterSpec := &cephv1.ClusterSpec{}
	poolSpec := cephv1.PoolSpec{FailureDomain: "rack"}
	ruleName := "testrule"
	if alreadyExists {
		ruleName = "replicated_ruleset"
	}

	err := createStretchCrushRule(context, clusterInfo, clusterSpec, ruleName, poolSpec)
	assert.NoError(t, err)
}

func TestCreatePoolWithReplicasPerFailureDomain(t *testing.T) {
	// This test goes via the path of explicit compile/decompile CRUSH map; ignored if 'crushtool' is not installed
	// on local build machine
	if hasCrushtool() {
		testCreatePoolWithReplicasPerFailureDomain(t, "host", "mycrushroot", "hdd")
		testCreatePoolWithReplicasPerFailureDomain(t, "rack", "mycrushroot", "ssd")
	}
}

func testCreatePoolWithReplicasPerFailureDomain(t *testing.T, failureDomain, crushRoot, deviceClass string) {
	poolRuleCreated := false
	poolRuleSet := false
	poolAppEnable := false
	poolSpec := cephv1.NamedPoolSpec{
		Name: "mypool-with-two-step-clush-rule",
		PoolSpec: cephv1.PoolSpec{
			FailureDomain: failureDomain,
			CrushRoot:     crushRoot,
			DeviceClass:   deviceClass,
			Replicated: cephv1.ReplicatedSpec{
				Size:                     12345678,
				ReplicasPerFailureDomain: 2,
			},
		},
	}

	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		assert.Equal(t, command, "ceph")
		assert.Equal(t, args[0], "osd")
		if len(args) >= 3 && args[1] == "crush" && args[2] == "dump" {
			return testCrushMap, nil
		}
		if len(args) >= 3 && args[1] == "pool" && args[2] == "create" {
			// Currently, CRUSH-rule name equals pool's name
			assert.GreaterOrEqual(t, len(args), 7)
			assert.Equal(t, args[3], poolSpec.Name)
			assert.Equal(t, args[5], "replicated")
			crushRuleName := args[6]
			assert.Equal(t, crushRuleName, poolSpec.Name)
			poolRuleCreated = true
			return "", nil
		}
		if len(args) >= 3 && args[1] == "pool" && args[2] == "set" {
			crushRuleName := args[3]
			assert.Equal(t, crushRuleName, poolSpec.Name)
			assert.Equal(t, args[4], "size")
			poolSize, err := strconv.Atoi(args[5])
			assert.NoError(t, err)
			assert.Equal(t, uint(poolSize), poolSpec.Replicated.Size)
			poolRuleSet = true
			return "", nil
		}
		if len(args) >= 4 && args[1] == "pool" && args[2] == "application" {
			if args[3] == "get" {
				return emptyApplicationName, nil
			}

			crushRuleName := args[4]
			assert.Equal(t, crushRuleName, poolSpec.Name)
			poolAppEnable = true
			return "", nil
		}
		if len(args) >= 4 && args[1] == "crush" && args[2] == "rule" && args[3] == "create-replicated" {
			crushRuleName := args[4]
			assert.Equal(t, crushRuleName, poolSpec.Name)
			deviceClassName := args[7]
			assert.Equal(t, deviceClassName, deviceClass)
			poolRuleCreated = true
			return "", nil
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}
	context := &clusterd.Context{Executor: executor}
	clusterSpec := &cephv1.ClusterSpec{Storage: cephv1.StorageScopeSpec{Config: map[string]string{CrushRootConfigKey: "cluster-crush-root"}}}
	err := createReplicatedPoolForApp(context, AdminTestClusterInfo("mycluster"), clusterSpec, poolSpec, DefaultPGCount, "myapp")
	assert.Nil(t, err)
	assert.True(t, poolRuleCreated)
	assert.True(t, poolRuleSet)
	assert.True(t, poolAppEnable)
}

func TestCreateHybridCrushRule(t *testing.T) {
	testCreateHybridCrushRule(t, true)
	testCreateHybridCrushRule(t, false)
}

func testCreateHybridCrushRule(t *testing.T, alreadyExists bool) {
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
		if args[0] == "osd" && args[1] == "crush" && args[2] == "dump" {
			return testCrushMap, nil
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}
	clusterInfo := AdminTestClusterInfo("mycluster")
	clusterSpec := &cephv1.ClusterSpec{}
	poolSpec := cephv1.PoolSpec{
		FailureDomain: "rack",
		Replicated: cephv1.ReplicatedSpec{
			HybridStorage: &cephv1.HybridStorageSpec{
				PrimaryDeviceClass:   "ssd",
				SecondaryDeviceClass: "hdd",
			},
		},
	}
	ruleName := "testrule"
	if alreadyExists {
		ruleName = "hybrid_ruleset"
	}

	err := createHybridCrushRule(context, clusterInfo, clusterSpec, ruleName, poolSpec)
	assert.NoError(t, err)
}

func hasCrushtool() bool {
	_, err := exec.LookPath("crushtool")
	return err == nil
}

func TestSetPgNumMinMaxPropertiesDoesNothing(t *testing.T) {
	got, _ := testSetPgNumMinMaxProperties(map[string]string{}, &CephStoragePoolDetails{
		PgNum:           8,
		PgpNum:          8,
		PgAutoScaleMode: "on",
	})
	wanted := []string{}

	if !reflect.DeepEqual(got, wanted) {
		t.Errorf("got %v want %v", got, wanted)
	}
}

func TestSetPgNumMinMaxPropertiesDontChangeExistingMinMaxSettings(t *testing.T) {
	pgNumMin := 8
	pgNumMax := 32
	got, _ := testSetPgNumMinMaxProperties(map[string]string{}, &CephStoragePoolDetails{
		PgNum:           8,
		PgpNum:          8,
		PgNumMin:        &pgNumMin,
		PgNumMax:        &pgNumMax,
		PgAutoScaleMode: "on",
	})
	wanted := []string{}

	if !reflect.DeepEqual(got, wanted) {
		t.Errorf("got %v want %v", got, wanted)
	}
}

func TestSetPgNumMinLowerThanPgNum(t *testing.T) {
	got, _ := testSetPgNumMinMaxProperties(map[string]string{"pg_num_min": "4"}, &CephStoragePoolDetails{
		PgNum:           8,
		PgpNum:          8,
		PgAutoScaleMode: "on",
	})
	wanted := []string{
		"osd pool set mypool pg_num_min 4",
	}

	if !reflect.DeepEqual(got, wanted) {
		t.Errorf("got %v want %v", got, wanted)
	}
}

func TestSetPgNumMinHigherThanPgNumWithAutoScalingOn(t *testing.T) {
	got, _ := testSetPgNumMinMaxProperties(map[string]string{"pg_num_min": "16"}, &CephStoragePoolDetails{
		PgNum:           8,
		PgpNum:          8,
		PgAutoScaleMode: "on",
	})
	wanted := []string{
		"osd pool set mypool pg_autoscale_mode off",
		"osd pool set mypool pg_num 16",
		"osd pool set mypool pgp_num 16",
		"osd pool set mypool pg_num_min 16",
		"osd pool set mypool pg_autoscale_mode on",
	}

	if !reflect.DeepEqual(got, wanted) {
		t.Errorf("got %v want %v", got, wanted)
	}
}

func TestSetPgNumMinHigherThanPgNumWithAutoScalingOff(t *testing.T) {
	got, _ := testSetPgNumMinMaxProperties(map[string]string{"pg_num_min": "16"}, &CephStoragePoolDetails{
		PgNum:           8,
		PgpNum:          8,
		PgAutoScaleMode: "off",
	})
	wanted := []string{
		"osd pool set mypool pg_num 16",
		"osd pool set mypool pgp_num 16",
		"osd pool set mypool pg_num_min 16",
	}

	if !reflect.DeepEqual(got, wanted) {
		t.Errorf("got %v want %v", got, wanted)
	}
}

func TestSetPgNumMaxHigherThanPgNum(t *testing.T) {
	got, _ := testSetPgNumMinMaxProperties(map[string]string{"pg_num_max": "16"}, &CephStoragePoolDetails{
		PgNum:           8,
		PgpNum:          8,
		PgAutoScaleMode: "on",
	})
	wanted := []string{
		"osd pool set mypool pg_num_max 16",
	}

	if !reflect.DeepEqual(got, wanted) {
		t.Errorf("got %v want %v", got, wanted)
	}
}

func TestSetPgNumMaxLowerThanPgNumWithAutoScalingOn(t *testing.T) {
	got, _ := testSetPgNumMinMaxProperties(map[string]string{"pg_num_max": "4"}, &CephStoragePoolDetails{
		PgNum:           8,
		PgpNum:          8,
		PgAutoScaleMode: "on",
	})
	wanted := []string{
		"osd pool set mypool pg_autoscale_mode off",
		"osd pool set mypool pg_num 4",
		"osd pool set mypool pgp_num 4",
		"osd pool set mypool pg_num_max 4",
		"osd pool set mypool pg_autoscale_mode on",
	}

	if !reflect.DeepEqual(got, wanted) {
		t.Errorf("got %v want %v", got, wanted)
	}
}

func TestSetPgNumMaxLowerThanPgNumWithAutoScalingOff(t *testing.T) {
	got, _ := testSetPgNumMinMaxProperties(map[string]string{"pg_num_max": "4"}, &CephStoragePoolDetails{
		PgNum:           8,
		PgpNum:          8,
		PgAutoScaleMode: "off",
	})
	wanted := []string{
		"osd pool set mypool pg_num 4",
		"osd pool set mypool pgp_num 4",
		"osd pool set mypool pg_num_max 4",
	}

	if !reflect.DeepEqual(got, wanted) {
		t.Errorf("got %v want %v", got, wanted)
	}
}

func TestSetPgNumMinMaxWithAutoScalingOn(t *testing.T) {
	got, _ := testSetPgNumMinMaxProperties(map[string]string{"pg_num_min": "16", "pg_num_max": "32"}, &CephStoragePoolDetails{
		PgNum:           8,
		PgpNum:          8,
		PgAutoScaleMode: "on",
	})
	wanted := []string{
		"osd pool set mypool pg_autoscale_mode off",
		"osd pool set mypool pg_num 16",
		"osd pool set mypool pgp_num 16",
		"osd pool set mypool pg_num_min 16",
		"osd pool set mypool pg_num_max 32",
		"osd pool set mypool pg_autoscale_mode on",
	}

	if !reflect.DeepEqual(got, wanted) {
		t.Errorf("got %v want %v", got, wanted)
	}
}

func TestSetPgNumMinMaxPropertiesCheckConsistency(t *testing.T) {
	got, err := testSetPgNumMinMaxProperties(map[string]string{"pg_num_min": "32", "pg_num_max": "16"}, &CephStoragePoolDetails{
		PgNum:           8,
		PgpNum:          8,
		PgAutoScaleMode: "on",
	})
	wanted := []string{}

	if err == nil {
		t.Errorf("SetPgNumMinMaxProperties shouldn't have return an error")
	}

	wantedError := "pg_num_min (32) can't be greater than pg_num_max (16) for pool \"mypool\""
	if wantedError != err.Error() {
		t.Errorf("got %v want %v", err.Error(), wantedError)
	}

	if !reflect.DeepEqual(got, wanted) {
		t.Errorf("got %v want %v", got, wanted)
	}
}

func testSetPgNumMinMaxProperties(poolParameters map[string]string, poolState *CephStoragePoolDetails) ([]string, error) {
	executor := &exectest.MockExecutor{}
	context := &clusterd.Context{Executor: executor}
	p := cephv1.NamedPoolSpec{
		Name: "mypool",
		PoolSpec: cephv1.PoolSpec{
			Parameters: poolParameters,
		},
	}
	calls := make([]string, 0)
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		if len(args) > 4 {
			if args[1] == "pool" && args[2] == "get" {
				data, err := json.Marshal(poolState)
				if err != nil {
					return "", err
				}
				return string(data), nil
			}
		}
		calls = append(calls, strings.Join(args[0:6], " "))
		return "", nil
	}
	err := setPgNumMinMaxProperties(context, AdminTestClusterInfo("mycluster"), p)
	return calls, err
}
