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
	"reflect"
	"strings"
	"testing"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	testop "github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

func TestMonStore_Set(t *testing.T) {
	executor := &exectest.MockExecutor{}
	clientset := testop.New(t, 1)
	ctx := &clusterd.Context{
		Clientset: clientset,
		Executor:  executor,
	}

	// create a mock command runner which creates a simple string of the command it ran, and allow
	// us to cause it to return an error when it detects a keyword.
	execedCmd := ""
	execInjectErr := false
	executor.MockExecuteCommandWithOutputFile =
		func(command string, outfile string, args ...string) (string, error) {
			execedCmd = command + " " + strings.Join(args, " ")
			if execInjectErr {
				return "output from cmd with error", errors.New("mocked error")
			}
			return "", nil
		}

	monStore := GetMonStore(ctx, "ns")

	// setting with spaces converts to underscores
	e := monStore.Set("global", "debug ms", "10")
	assert.NoError(t, e)
	assert.Contains(t, execedCmd, "config set global debug_ms 10")

	// setting with dashes converts to underscores
	e = monStore.Set("osd.0", "debug-osd", "20")
	assert.NoError(t, e)
	assert.Contains(t, execedCmd, " config set osd.0 debug_osd 20 ")

	// setting with underscores stays the same
	e = monStore.Set("mds.*", "debug_mds", "15")
	assert.NoError(t, e)
	assert.Contains(t, execedCmd, " config set mds.* debug_mds 15 ")

	// errors returned as expected
	execInjectErr = true
	e = monStore.Set("mon.*", "unknown_setting", "10")
	assert.Error(t, e)
	assert.Contains(t, execedCmd, " config set mon.* unknown_setting 10 ")
}

func TestMonStore_Delete(t *testing.T) {
	executor := &exectest.MockExecutor{}
	clientset := testop.New(t, 1)
	ctx := &clusterd.Context{
		Clientset: clientset,
		Executor:  executor,
	}

	// create a mock command runner which creates a simple string of the command it ran, and allow
	// us to cause it to return an error when it detects a keyword.
	execedCmd := ""
	execInjectErr := false
	executor.MockExecuteCommandWithOutputFile =
		func(command string, outfile string, args ...string) (string, error) {
			execedCmd = command + " " + strings.Join(args, " ")
			if execInjectErr {
				return "output from cmd with error", errors.New("mocked error")
			}
			return "", nil
		}

	monStore := GetMonStore(ctx, "ns")

	// ceph config rm called as expected
	e := monStore.Delete("global", "debug ms")
	assert.NoError(t, e)
	assert.Contains(t, execedCmd, "config rm global debug_ms")

	// errors returned as expected
	execInjectErr = true
	e = monStore.Delete("mon.*", "unknown_setting")
	assert.Error(t, e)
	assert.Contains(t, execedCmd, " config rm mon.* unknown_setting ")
}

func TestMonStore_GetDaemon(t *testing.T) {
	executor := &exectest.MockExecutor{}
	clientset := testop.New(t, 1)
	ctx := &clusterd.Context{
		Clientset: clientset,
		Executor:  executor,
	}

	// create a mock command runner which creates a simple string of the command it ran, and allow
	// us to cause it to return an error when it detects a keyword and to return a specific string
	execedCmd := ""
	execReturn := "{\"rbd_default_features\":{\"value\":\"3\",\"section\":\"global\",\"mask\":{}," +
		"\"can_update_at_runtime\":true}," +
		"\"rgw_enable_usage_log\":{\"value\":\"true\",\"section\":\"client.rgw.test.a\",\"mask\":{}," +
		"\"can_update_at_runtime\":true}}"
	execInjectErr := false
	executor.MockExecuteCommandWithOutputFile =
		func(command string, outfile string, args ...string) (string, error) {
			execedCmd = command + " " + strings.Join(args, " ")
			if execInjectErr {
				return "output from cmd with error", errors.New("mocked error")
			}
			return execReturn, nil
		}

	monStore := GetMonStore(ctx, "ns")

	// ceph config get called as expected
	options, e := monStore.GetDaemon("client.rgw.test.a")
	assert.NoError(t, e)
	assert.Contains(t, execedCmd, "ceph config get client.rgw.test.a")
	assert.True(t, reflect.DeepEqual(options, []Option{{"client.rgw.test.a", "rgw_enable_usage_log", "true"}}))

	// json parse exception return as expected
	execReturn = "bad json output"
	_, e = monStore.GetDaemon("client.rgw.test.a")
	assert.Error(t, e)
	assert.Contains(t, e.Error(), "failed to parse json config for daemon \"client.rgw.test.a\". json: "+
		"bad json output")

	// errors returned as expected
	execInjectErr = true
	_, e = monStore.GetDaemon("mon.*")
	assert.Error(t, e)
	assert.Contains(t, execedCmd, " config get mon.* ")
}

func TestMonStore_DeleteDaemon(t *testing.T) {
	executor := &exectest.MockExecutor{}
	clientset := testop.New(t, 1)
	ctx := &clusterd.Context{
		Clientset: clientset,
		Executor:  executor,
	}

	// create a mock command runner which creates a simple string of the command it ran, and allow
	// us to cause it to return an error when it detects a keyword and to return a specific string
	execedCmd := ""
	execReturn := "{\"rbd_default_features\":{\"value\":\"3\",\"section\":\"global\",\"mask\":{}," +
		"\"can_update_at_runtime\":true}," +
		"\"rgw_enable_usage_log\":{\"value\":\"true\",\"section\":\"client.rgw.test.a\",\"mask\":{}," +
		"\"can_update_at_runtime\":true}}"
	executor.MockExecuteCommandWithOutputFile =
		func(command string, outfile string, args ...string) (string, error) {
			execedCmd = command + " " + strings.Join(args, " ")
			return execReturn, nil
		}

	monStore := GetMonStore(ctx, "ns")

	// ceph config rm rgw_enable_usage_log called as expected
	e := monStore.DeleteDaemon("client.rgw.test.a")
	assert.NoError(t, e)
	assert.Contains(t, execedCmd, "ceph config rm client.rgw.test.a rgw_enable_usage_log")
}

func TestMonStore_SetAll(t *testing.T) {
	clientset := testop.New(t, 1)
	executor := &exectest.MockExecutor{}
	ctx := &clusterd.Context{
		Clientset: clientset,
		Executor:  executor,
	}

	// create a mock command runner which creates a simple string of the command it ran, and allow
	// us to cause it to return an error when it detects a keyword.
	execedCmds := []string{}
	execInjectErrOnKeyword := "donotinjectanerror"
	executor.MockExecuteCommandWithOutputFile =
		func(command string, outfile string, args ...string) (string, error) {
			execedCmd := command + " " + strings.Join(args, " ")
			execedCmds = append(execedCmds, execedCmd)
			k := execInjectErrOnKeyword
			if strings.Contains(execedCmd, k) {
				return "output from cmd with error on keyword: " + k, errors.Errorf("mocked error on keyword: " + k)
			}
			return "", nil
		}

	monStore := GetMonStore(ctx, "ns")

	cfgOverrides := []Option{
		configOverride("global", "debug ms", "10"), // setting w/ spaces converts to underscores
		configOverride("osd.0", "debug-osd", "20"), // setting w/ dashes converts to underscores
		configOverride("mds.*", "debug_mds", "15"), // setting w/ underscores remains the same
	}

	// commands w/ no error
	e := monStore.SetAll(cfgOverrides...)
	assert.NoError(t, e)
	assert.Len(t, execedCmds, 3)
	assert.Contains(t, execedCmds[0], " global debug_ms 10 ")
	assert.Contains(t, execedCmds[1], " osd.0 debug_osd 20 ")
	assert.Contains(t, execedCmds[2], " mds.* debug_mds 15 ")

	// commands w/ one error
	// keep cfgOverrides from last test
	execInjectErrOnKeyword = "debug_osd"
	execedCmds = execedCmds[:0] // empty execedCmds slice
	e = monStore.SetAll(cfgOverrides...)
	assert.Error(t, e)
	// Rook should not return error before trying to set all config overrides
	assert.Len(t, execedCmds, 3)

	// all commands return error
	// keep cfgOverrides
	execInjectErrOnKeyword = "debug"
	execedCmds = execedCmds[:0]
	e = monStore.SetAll(cfgOverrides...)
	assert.Error(t, e)
	assert.Len(t, execedCmds, 3)
}
