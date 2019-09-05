/*
Copyright 2019 The Rook Authors. All rights reserved.

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

package config

import (
	"fmt"
	"strings"
	"testing"

	rookceph "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	testop "github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

func TestMonStore_Set(t *testing.T) {
	executor := &exectest.MockExecutor{}
	ctx := &clusterd.Context{
		Clientset: testop.New(1),
		Executor:  executor,
	}

	// create a mock command runner which creates a simple string of the command it ran, and allow
	// us to cause it to return an error when it detects a keyword.
	execedCmd := ""
	execInjectErr := false
	executor.MockExecuteCommandWithOutputFile =
		func(debug bool, actionName string, command string, outfile string, args ...string) (string, error) {
			execedCmd = command + " " + strings.Join(args, " ")
			if execInjectErr {
				return "output from cmd with error", fmt.Errorf("mocked error")
			}
			return "", nil
		}

	monStore := GetMonStore(ctx, "ns")

	stringPointer := func(s string) *string {
		return &s
	}

	// setting with spaces converts to underscores
	e := monStore.Set("global", "debug ms", stringPointer("10"))
	assert.NoError(t, e)
	assert.Contains(t, execedCmd, "config set global debug_ms 10")

	// setting with dashes converts to underscores
	e = monStore.Set("osd.0", "debug-osd", stringPointer("20"))
	assert.NoError(t, e)
	assert.Contains(t, execedCmd, " config set osd.0 debug_osd 20 ")

	// setting with underscores stays the same
	e = monStore.Set("mds.*", "debug_mds", stringPointer("15"))
	assert.NoError(t, e)
	assert.Contains(t, execedCmd, " config set mds.* debug_mds 15 ")

	// errors returned as expected
	execInjectErr = true
	e = monStore.Set("mon.*", "unknown_setting", stringPointer("10"))
	assert.Error(t, e)
	assert.Contains(t, execedCmd, " config set mon.* unknown_setting 10 ")

	// unset when nil
	execInjectErr = false
	e = monStore.Set("global", "debug_ms", nil)
	assert.NoError(t, e)
	assert.Contains(t, execedCmd, " config rm global debug_ms")
}

func TestMonStore_SetAll(t *testing.T) {
	executor := &exectest.MockExecutor{}
	ctx := &clusterd.Context{
		Clientset: testop.New(1),
		Executor:  executor,
	}

	// create a mock command runner which creates a simple string of the command it ran, and allow
	// us to cause it to return an error when it detects a keyword.
	execedCmds := []string{}
	execInjectErrOnKeyword := "donotinjectanerror"
	executor.MockExecuteCommandWithOutputFile =
		func(debug bool, actionName string, command string, outfile string, args ...string) (string, error) {
			execedCmd := command + " " + strings.Join(args, " ")
			execedCmds = append(execedCmds, execedCmd)
			k := execInjectErrOnKeyword
			if strings.Contains(execedCmd, k) {
				return "output from cmd with error on keyword: " + k, fmt.Errorf("mocked error on keyword: " + k)
			}
			return "", nil
		}

	monStore := GetMonStore(ctx, "ns")

	var cfgOverrides rookceph.ConfigOverridesSpec

	cfgOverrides = []rookceph.ConfigOverride{
		configOverride("global", "debug ms", "10"), // setting w/ spaces converts to underscores
		configOverride("osd.0", "debug-osd", "20"), // setting w/ dashes converts to underscores
		configOverride("mds.*", "debug_mds", "15"), // setting w/ underscores remains the same
	}

	// commands w/ no error
	e := monStore.SetAll(cfgOverrides)
	assert.NoError(t, e)
	assert.Len(t, execedCmds, 3)
	assert.Contains(t, execedCmds[0], " global debug_ms 10 ")
	assert.Contains(t, execedCmds[1], " osd.0 debug_osd 20 ")
	assert.Contains(t, execedCmds[2], " mds.* debug_mds 15 ")

	// commands w/ one error
	// keep cfgOverrides from last test
	execInjectErrOnKeyword = "debug_osd"
	execedCmds = execedCmds[:0] // empty execedCmds slice
	e = monStore.SetAll(cfgOverrides)
	assert.Error(t, e)
	// Rook should not return error before trying to set all config overrides
	assert.Len(t, execedCmds, 3)

	// all commands return error
	// keep cfgOverrides
	execInjectErrOnKeyword = "debug"
	execedCmds = execedCmds[:0]
	e = monStore.SetAll(cfgOverrides)
	assert.Error(t, e)
	assert.Len(t, execedCmds, 3)
}
