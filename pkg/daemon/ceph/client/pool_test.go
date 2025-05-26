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
	"os/exec"
	"reflect"
	"slices"
	"strconv"
	"testing"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

const emptyApplicationName = `{"":{}}`

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
	compressionModeCreated := false
	p := cephv1.NamedPoolSpec{
		Name: "mypool",
		PoolSpec: cephv1.PoolSpec{
			FailureDomain: "host",
			ErasureCoded:  cephv1.ErasureCodedSpec{},
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

	err := createECPoolForApp(context, AdminTestClusterInfo("mycluster"), "mypoolprofile", p, DefaultPGCount, overwrite)
	assert.Nil(t, err)
	if compressionMode != "" {
		assert.True(t, compressionModeCreated)
	} else {
		assert.False(t, compressionModeCreated)
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
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[1] == "pool" {
			if args[2] == "create" {
				assert.Equal(t, "mypool", args[3])
				assert.Equal(t, "replicated", args[5])
				assert.Equal(t, "--size", args[7])
				assert.Equal(t, "12345", args[8])
				return "", nil
			}
			if args[2] == "get" {
				assert.Equal(t, "mypool", args[3])
				assert.Equal(t, "all", args[4])
				return `{"pool":"replicapool","pool_id":2,"size":1,"min_size":1,"crush_rule":"replicapool_osd"}`, nil
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
			crushRuleCreated = true
			assert.Equal(t, "rule", args[2])
			if args[3] == "dump" {
				assert.Equal(t, "replicapool_osd", args[4])
				return `{"rule_id": 3,"rule_name": "replicapool_osd","type": 1}`, nil
			}
			assert.Equal(t, "create-replicated", args[3])
			assert.Contains(t, args[4], "mypool")
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
				assert.False(t, slices.Contains(args, "hdd"))
			} else {
				assert.Equal(t, deviceClass, args[7])
			}
			return "", nil
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}

	p := cephv1.NamedPoolSpec{
		Name: "mypool",
		PoolSpec: cephv1.PoolSpec{
			FailureDomain: failureDomain, CrushRoot: crushRoot, DeviceClass: deviceClass,
			Replicated: cephv1.ReplicatedSpec{Size: 12345},
		},
	}
	if compressionMode != "" {
		p.CompressionMode = compressionMode
	}
	clusterSpec := &cephv1.ClusterSpec{Storage: cephv1.StorageScopeSpec{Config: map[string]string{CrushRootConfigKey: "cluster-crush-root"}}}
	err := createReplicatedPoolForApp(context, AdminTestClusterInfo("mycluster"), clusterSpec, p, DefaultPGCount)
	assert.Nil(t, err)
	assert.True(t, crushRuleCreated)
	if compressionMode != "" {
		assert.True(t, compressionModeCreated)
	} else {
		assert.False(t, compressionModeCreated)
	}
}

func TestUpdateFailureDomain(t *testing.T) {
	var newCrushRule string
	currentFailureDomain := "rack"
	currentDeviceClass := "default"
	testCrushRuleName := "test_rule"
	cephCommandCalled := false
	executor := &exectest.MockExecutor{}
	context := &clusterd.Context{Executor: executor}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		cephCommandCalled = true
		if args[1] == "pool" {
			if args[2] == "get" {
				assert.Equal(t, "mypool", args[3])
				return fmt.Sprintf(`{"crush_rule": "%s"}`, testCrushRuleName), nil
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
				return fmt.Sprintf(`{"steps": [{"item_name":"%s"},{"type":"%s"}]}`, currentDeviceClass, currentFailureDomain), nil
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
				Replicated:         cephv1.ReplicatedSpec{Size: 3},
				EnableCrushUpdates: true,
			},
		}
		clusterSpec := &cephv1.ClusterSpec{Storage: cephv1.StorageScopeSpec{}}
		err := updatePoolCrushRule(context, AdminTestClusterInfo("mycluster"), clusterSpec, p)
		assert.NoError(t, err)
		assert.Equal(t, "", newCrushRule)
	})

	t.Run("same failure domain", func(t *testing.T) {
		p := cephv1.NamedPoolSpec{
			Name: "mypool",
			PoolSpec: cephv1.PoolSpec{
				FailureDomain:      currentFailureDomain,
				Replicated:         cephv1.ReplicatedSpec{Size: 3},
				EnableCrushUpdates: true,
			},
		}
		testCrushRuleName = "mypool_rack"
		clusterSpec := &cephv1.ClusterSpec{Storage: cephv1.StorageScopeSpec{}}
		err := updatePoolCrushRule(context, AdminTestClusterInfo("mycluster"), clusterSpec, p)
		assert.NoError(t, err)
		assert.Equal(t, "", newCrushRule)
	})

	t.Run("trying to change failure domain without enabling EnableCrushUpdates", func(t *testing.T) {
		p := cephv1.NamedPoolSpec{
			Name: "mypool",
			PoolSpec: cephv1.PoolSpec{
				FailureDomain: "zone",
				Replicated:    cephv1.ReplicatedSpec{Size: 3},
			},
		}
		clusterSpec := &cephv1.ClusterSpec{Storage: cephv1.StorageScopeSpec{}}
		err := updatePoolCrushRule(context, AdminTestClusterInfo("mycluster"), clusterSpec, p)
		assert.NoError(t, err)
		assert.Equal(t, "", newCrushRule)
	})

	t.Run("changing failure domain", func(t *testing.T) {
		p := cephv1.NamedPoolSpec{
			Name: "mypool",
			PoolSpec: cephv1.PoolSpec{
				FailureDomain:      "zone",
				Replicated:         cephv1.ReplicatedSpec{Size: 3},
				EnableCrushUpdates: true,
			},
		}
		clusterSpec := &cephv1.ClusterSpec{Storage: cephv1.StorageScopeSpec{}}
		err := updatePoolCrushRule(context, AdminTestClusterInfo("mycluster"), clusterSpec, p)
		assert.NoError(t, err)
		assert.Equal(t, "mypool_zone", newCrushRule)
	})

	t.Run("stretch cluster skips crush rule update", func(t *testing.T) {
		p := cephv1.NamedPoolSpec{
			Name: "mypool",
			PoolSpec: cephv1.PoolSpec{
				FailureDomain:      "zone",
				Replicated:         cephv1.ReplicatedSpec{Size: 3},
				EnableCrushUpdates: true,
			},
		}
		clusterSpec := &cephv1.ClusterSpec{
			Mon:     cephv1.MonSpec{StretchCluster: &cephv1.StretchClusterSpec{Zones: []cephv1.MonZoneSpec{{Name: "zone1"}, {Name: "zone2"}, {Name: "zone3", Arbiter: true}}}},
			Storage: cephv1.StorageScopeSpec{},
		}
		newCrushRule = ""
		cephCommandCalled = false
		err := updatePoolCrushRule(context, AdminTestClusterInfo("mycluster"), clusterSpec, p)
		assert.NoError(t, err)
		assert.Equal(t, "", newCrushRule)
		assert.False(t, cephCommandCalled)
	})
}

func TestExtractPoolDetails(t *testing.T) {
	t.Run("complex crush rule skipped", func(t *testing.T) {
		rule := ruleSpec{Steps: []stepSpec{
			{Type: ""},
			{Type: ""},
			{Type: "zone"},
		}}
		failureDomain, _ := extractPoolDetails(rule)
		assert.Equal(t, "", failureDomain)
	})
	t.Run("valid crush rule", func(t *testing.T) {
		rule := ruleSpec{Steps: []stepSpec{
			{Type: ""},
			{Type: "zone", ItemName: "ssd"},
		}}
		failureDomain, deviceClass := extractPoolDetails(rule)
		assert.Equal(t, "zone", failureDomain)
		assert.Equal(t, "ssd", deviceClass)
	})

	t.Run("valid crush rule with crushroot combined", func(t *testing.T) {
		rule := ruleSpec{Steps: []stepSpec{
			{Type: ""},
			{Type: "zone", ItemName: "default~ssd"},
		}}
		failureDomain, deviceClass := extractPoolDetails(rule)
		assert.Equal(t, "zone", failureDomain)
		assert.Equal(t, "ssd", deviceClass)
	})
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
			switch args[0] {
			case "--decompile", "--compile":
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
			uPoolSize := uint(poolSize) // nolint:gosec // G115 : we know it is not too big
			assert.Equal(t, uPoolSize, poolSpec.Replicated.Size)
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
	err := createReplicatedPoolForApp(context, AdminTestClusterInfo("mycluster"), clusterSpec, poolSpec, DefaultPGCount)
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
			switch args[0] {
			case "--decompile", "--compile":
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
