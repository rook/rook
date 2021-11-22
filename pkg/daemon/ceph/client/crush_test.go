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
	"testing"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

const testCrushMap = `{
    "devices": [
        {
            "id": 0,
            "name": "osd.0",
            "class": "hdd"
        }
    ],
    "types": [
        {
            "type_id": 0,
            "name": "osd"
        },
        {
            "type_id": 1,
            "name": "host"
        },
        {
            "type_id": 2,
            "name": "chassis"
        },
        {
            "type_id": 3,
            "name": "rack"
        },
        {
            "type_id": 4,
            "name": "row"
        },
        {
            "type_id": 5,
            "name": "pdu"
        },
        {
            "type_id": 6,
            "name": "pod"
        },
        {
            "type_id": 7,
            "name": "room"
        },
        {
            "type_id": 8,
            "name": "datacenter"
        },
        {
            "type_id": 9,
            "name": "region"
        },
        {
            "type_id": 10,
            "name": "root"
        }
    ],
    "buckets": [
        {
            "id": -1,
            "name": "default",
            "type_id": 10,
            "type_name": "root",
            "weight": 1028,
            "alg": "straw",
            "hash": "rjenkins1",
            "items": [
                {
                    "id": -3,
                    "weight": 1028,
                    "pos": 0
                }
            ]
        },
        {
            "id": -2,
            "name": "default~hdd",
            "type_id": 10,
            "type_name": "root",
            "weight": 1028,
            "alg": "straw",
            "hash": "rjenkins1",
            "items": [
                {
                    "id": -4,
                    "weight": 1028,
                    "pos": 0
                }
            ]
        },
        {
            "id": -3,
            "name": "minikube",
            "type_id": 1,
            "type_name": "host",
            "weight": 1028,
            "alg": "straw",
            "hash": "rjenkins1",
            "items": [
                {
                    "id": 0,
                    "weight": 1028,
                    "pos": 0
                }
            ]
        },
        {
            "id": -4,
            "name": "minikube~hdd",
            "type_id": 1,
            "type_name": "host",
            "weight": 1028,
            "alg": "straw",
            "hash": "rjenkins1",
            "items": [
                {
                    "id": 0,
                    "weight": 1028,
                    "pos": 0
                }
            ]
        }
    ],
    "rules": [
        {
            "rule_id": 0,
            "rule_name": "replicated_ruleset",
            "ruleset": 0,
            "type": 1,
            "min_size": 1,
            "max_size": 10,
            "steps": [
                {
                    "op": "take",
                    "item": -1,
                    "item_name": "default"
                },
                {
                    "op": "chooseleaf_firstn",
                    "num": 0,
                    "type": "host"
                },
                {
                    "op": "emit"
                }
            ]
        },
        {
            "rule_id": 1,
            "rule_name": "hybrid_ruleset",
            "ruleset": 1,
            "type": 1,
            "min_size": 1,
            "max_size": 10,
            "steps": [
                {
                    "op": "take",
                    "item": -2,
                    "item_name": "default~hdd"
                },
                {
                    "op": "chooseleaf_firstn",
                    "num": 1,
                    "type": "host"
                },
                {
                    "op": "emit"
                },
                {
                    "op": "take",
                    "item": -2,
                    "item_name": "default~ssd"
                },
                {
                    "op": "chooseleaf_firstn",
                    "num": 0,
                    "type": "host"
                },
                {
                    "op": "emit"
                }
            ]
        },
        {
            "rule_id": 1,
            "rule_name": "my-store.rgw.buckets.data",
            "ruleset": 1,
            "type": 3,
            "min_size": 3,
            "max_size": 3,
            "steps": [
                {
                    "op": "set_chooseleaf_tries",
                    "num": 5
                },
                {
                    "op": "set_choose_tries",
                    "num": 100
                },
                {
                    "op": "take",
                    "item": -1,
                    "item_name": "default"
                },
                {
                    "op": "chooseleaf_indep",
                    "num": 0,
                    "type": "host"
                },
                {
                    "op": "emit"
                }
            ]
        }
    ],
    "tunables": {
        "choose_local_tries": 0,
        "choose_local_fallback_tries": 0,
        "choose_total_tries": 50,
        "chooseleaf_descend_once": 1,
        "chooseleaf_vary_r": 1,
        "chooseleaf_stable": 0,
        "straw_calc_version": 1,
        "allowed_bucket_algs": 22,
        "profile": "firefly",
        "optimal_tunables": 0,
        "legacy_tunables": 0,
        "minimum_required_version": "firefly",
        "require_feature_tunables": 1,
        "require_feature_tunables2": 1,
        "has_v2_rules": 1,
        "require_feature_tunables3": 1,
        "has_v3_rules": 0,
        "has_v4_buckets": 0,
        "require_feature_tunables5": 0,
        "has_v5_rules": 0
    },
    "choose_args": {}
}
`

func TestGetCrushMap(t *testing.T) {
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[1] == "crush" && args[2] == "dump" {
			return testCrushMap, nil
		}
		return "", errors.Errorf("unexpected ceph command '%v'", args)
	}
	crush, err := GetCrushMap(&clusterd.Context{Executor: executor}, AdminTestClusterInfo("mycluster"))

	assert.Nil(t, err)
	assert.Equal(t, 11, len(crush.Types))
	assert.Equal(t, 1, len(crush.Devices))
	assert.Equal(t, 4, len(crush.Buckets))
	assert.Equal(t, 3, len(crush.Rules))
}

func TestGetOSDOnHost(t *testing.T) {
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[1] == "crush" && args[2] == "ls" {
			return "[\"osd.2\",\"osd.0\",\"osd.1\"]", nil
		}
		return "", errors.Errorf("unexpected ceph command '%v'", args)
	}

	_, err := GetOSDOnHost(&clusterd.Context{Executor: executor}, AdminTestClusterInfo("mycluster"), "my-host")
	assert.Nil(t, err)
}

func TestCrushName(t *testing.T) {
	// each is slightly different than the last
	crushNames := []string{
		"www.zxyz.com",
		"www.abcd.com",
		"ip-10-0-132-84.us-east-2.compute.internal",
		"ip-10-0-132-85.us-east-2.compute.internal",
		"worker1",
		"worker2",
		"master1",
		"master2",
		"us-east-2b",
		"us-east-2c",
		"us-east-1",
		"us-east-2",
		"ip-10-0-175-140",
		"ip-10-0-175-141",
	}

	for i, crushName := range crushNames {
		normalizedCrushName := NormalizeCrushName(crushName)
		fmt.Printf("crushName: %s, normalizedCrushName: %s\n", crushName, normalizedCrushName)
		assert.True(t, IsNormalizedCrushNameEqual(crushName, normalizedCrushName))
		assert.True(t, IsNormalizedCrushNameEqual(crushName, crushName))
		assert.True(t, IsNormalizedCrushNameEqual(normalizedCrushName, normalizedCrushName))
		if i > 0 {
			// slightly different crush name
			differentCrushName := crushNames[i-1]
			differentNormalizedCrushName := NormalizeCrushName(differentCrushName)
			assert.False(t, IsNormalizedCrushNameEqual(crushName, differentNormalizedCrushName))
			assert.False(t, IsNormalizedCrushNameEqual(crushName, differentCrushName))
			assert.False(t, IsNormalizedCrushNameEqual(normalizedCrushName, differentNormalizedCrushName))
		}
	}
}

func TestBuildCompiledDecompileCRUSHFileName(t *testing.T) {
	assert.Equal(t, "/tmp/06399022.decompiled", buildDecompileCRUSHFileName("/tmp/06399022"))
	assert.Equal(t, "/tmp/06399022.compiled", buildCompileCRUSHFileName("/tmp/06399022"))
}
