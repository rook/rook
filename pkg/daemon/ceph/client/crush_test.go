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

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util"
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
	executor.MockExecuteCommandWithOutputFile = func(debug bool, actionName, command, outputFile string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[1] == "crush" && args[2] == "dump" {
			return testCrushMap, nil
		}
		return "", fmt.Errorf("unexpected ceph command '%v'", args)
	}
	crush, err := GetCrushMap(&clusterd.Context{Executor: executor}, "rook")

	assert.Nil(t, err)
	assert.Equal(t, 11, len(crush.Types))
	assert.Equal(t, 1, len(crush.Devices))
	assert.Equal(t, 4, len(crush.Buckets))
	assert.Equal(t, 2, len(crush.Rules))
}

func TestCrushLocation(t *testing.T) {
	loc := "dc=datacenter1"

	// test that root will get filled in with default/runtime values
	res, err := FormatLocation(loc, "my.node")
	assert.Nil(t, err)
	assert.Equal(t, 3, len(res))
	locSet := util.CreateSet(res)
	assert.True(t, locSet.Contains("root=default"))
	assert.True(t, locSet.Contains("dc=datacenter1"))
	assert.True(t, locSet.Contains("host=my-node"))

	// test that if host name and root are already set they will be honored
	loc = "root=otherRoot,dc=datacenter2,host=node123"
	res, err = FormatLocation(loc, "othernode")
	assert.Nil(t, err)
	assert.Equal(t, 3, len(res))
	locSet = util.CreateSet(res)
	assert.True(t, locSet.Contains("root=otherRoot"))
	assert.True(t, locSet.Contains("dc=datacenter2"))
	assert.True(t, locSet.Contains("host=node123"))

	// test an invalid CRUSH location format
	loc = "root=default,prop:value"
	_, err = FormatLocation(loc, "othernode")
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "is not in a valid format")
}
