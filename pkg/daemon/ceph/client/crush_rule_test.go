/*
Copyright 2020 The Rook Authors. All rights reserved.

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
	"testing"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

func TestBuildStretchClusterCrushRule(t *testing.T) {
	var crushMap CrushMap
	err := json.Unmarshal([]byte(testCrushMap), &crushMap)
	assert.NoError(t, err)

	pool := &cephv1.PoolSpec{
		FailureDomain: "datacenter",
		CrushRoot:     cephv1.DefaultCRUSHRoot,
		Replicated: cephv1.ReplicatedSpec{
			ReplicasPerFailureDomain: 2,
		},
	}

	rule := buildTwoStepCrushRule(crushMap, "stretched", *pool)
	assert.Equal(t, 2, rule.ID)
}

func TestBuildCrushSteps(t *testing.T) {
	pool := &cephv1.PoolSpec{
		FailureDomain: "datacenter",
		CrushRoot:     cephv1.DefaultCRUSHRoot,
		Replicated: cephv1.ReplicatedSpec{
			ReplicasPerFailureDomain: 2,
		},
	}
	steps := buildTwoStepCrushSteps(*pool)
	assert.Equal(t, 4, len(steps))
	assert.Equal(t, cephv1.DefaultCRUSHRoot, steps[0].ItemName)
	assert.Equal(t, "datacenter", steps[1].Type)
	assert.Equal(t, uint(2), steps[2].Number)
}

func TestCompileCRUSHMap(t *testing.T) {
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if command == "crushtool" && args[0] == "--compile" && args[1] == "/tmp/063990228" && args[2] == "--outfn" && args[3] == "/tmp/063990228.compiled" {
			return "3", nil
		}
		return "", errors.Errorf("unexpected ceph command '%v'", args)
	}

	err := compileCRUSHMap(&clusterd.Context{Executor: executor}, "/tmp/063990228")
	assert.Nil(t, err)
}

func TestDecompileCRUSHMap(t *testing.T) {
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if command == "crushtool" && args[0] == "--decompile" && args[1] == "/tmp/063990228" && args[2] == "--outfn" && args[3] == "/tmp/063990228.decompiled" {
			return "3", nil
		}
		return "", errors.Errorf("unexpected ceph command '%v'", args)
	}

	err := decompileCRUSHMap(&clusterd.Context{Executor: executor}, "/tmp/063990228")
	assert.Nil(t, err)
}

func TestInjectCRUSHMapMap(t *testing.T) {
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[0] == "osd" && args[1] == "setcrushmap" && args[2] == "--in-file" && args[3] == "/tmp/063990228.compiled" {
			return "3", nil
		}
		return "", errors.Errorf("unexpected ceph command '%v'", args)
	}

	err := injectCRUSHMap(&clusterd.Context{Executor: executor}, AdminTestClusterInfo("mycluster"), "/tmp/063990228.compiled")
	assert.Nil(t, err)
}

func TestSetCRUSHMapMap(t *testing.T) {
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[0] == "osd" && args[1] == "crush" && args[2] == "set" && args[3] == "/tmp/063990228.compiled" {
			return "3", nil
		}
		return "", errors.Errorf("unexpected ceph command '%v'", args)
	}

	err := setCRUSHMap(&clusterd.Context{Executor: executor}, AdminTestClusterInfo("mycluster"), "/tmp/063990228.compiled")
	assert.Nil(t, err)
}

func Test_generateRuleID(t *testing.T) {
	tests := []struct {
		name string
		args []ruleSpec
		want int
	}{
		{"ordered rules", []ruleSpec{{ID: 1}, {ID: 2}, {ID: 3}}, 4},
		{"unordered rules", []ruleSpec{{ID: 1}, {ID: 3}, {ID: 2}, {ID: 5}}, 6},
		{"unordered rules", []ruleSpec{{ID: 1}, {ID: 3}, {ID: 2}}, 4},
		{"ordered rules", []ruleSpec{{ID: 1}, {ID: 3}}, 4},
		{"ordered rules", []ruleSpec{{ID: 1}}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := generateRuleID(tt.args); got != tt.want {
				t.Errorf("generateRuleID() = %v, want %v", got, tt.want)
			}
		})
	}
}
