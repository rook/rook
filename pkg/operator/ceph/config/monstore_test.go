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
	"bytes"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	testop "github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	executor.MockExecuteCommandWithTimeout = func(timeout time.Duration, command string, args ...string) (string, error) {
		execedCmd = command + " " + strings.Join(args, " ")
		if execInjectErr {
			return "output from cmd with error", errors.New("mocked error")
		}
		return "", nil
	}

	monStore := GetMonStore(ctx, client.AdminTestClusterInfo("mycluster"))

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
	executor.MockExecuteCommandWithTimeout = func(timeout time.Duration, command string, args ...string) (string, error) {
		execedCmd = command + " " + strings.Join(args, " ")
		if execInjectErr {
			return "output from cmd with error", errors.New("mocked error")
		}
		return "", nil
	}

	monStore := GetMonStore(ctx, client.AdminTestClusterInfo("mycluster"))

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
	executor.MockExecuteCommandWithTimeout = func(timeout time.Duration, command string, args ...string) (string, error) {
		execedCmd = command + " " + strings.Join(args, " ")
		if execInjectErr {
			return "output from cmd with error", errors.New("mocked error")
		}
		return execReturn, nil
	}

	monStore := GetMonStore(ctx, client.AdminTestClusterInfo("mycluster"))

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
	executor.MockExecuteCommandWithTimeout = func(timeout time.Duration, command string, args ...string) (string, error) {
		execedCmd = command + " " + strings.Join(args, " ")
		return execReturn, nil
	}

	monStore := GetMonStore(ctx, client.AdminTestClusterInfo("mycluster"))

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
	appliedSettings := false
	executor.MockExecuteCommandWithTimeout = func(timeout time.Duration, command string, args ...string) (string, error) {
		appliedSettings = true
		return "", nil
	}

	monStore := GetMonStore(ctx, client.AdminTestClusterInfo("mycluster"))

	cfgOverrides := map[string]string{
		"debug ms":  "10", // setting w/ spaces converts to underscores
		"debug-osd": "20", // setting w/ dashes converts to underscores
		"debug_mds": "15", // setting w/ underscores remains the same
	}

	// commands w/ no error
	keys, e := monStore.setAll("osd.0", cfgOverrides)
	// no keys written since it's mocked
	assert.Equal(t, 0, len(keys))
	assert.NoError(t, e)
	assert.True(t, appliedSettings)
}

func TestMonStore_SetAllRejectsLineBreaksAndBrackets(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{"newline in key", "#\n[global]\nmon_max_pg_per_osd", "s3cr3t"},
		{"carriage return in key", "#\r[global]\rmon_max_pg_per_osd", "s3cr3t"},
		{"newline in value", "rgw_keystone_url", "s3cr3t\n[global]\nmon_max_pg_per_osd = 1000"},
		{"carriage return in value", "rgw_keystone_url", "s3cr3t\r[global]\rmon_max_pg_per_osd = 1000"},
		{"opening bracket in key", "[global] mon_max_pg_per_osd", "s3cr3t"},
		{"closing bracket in key", "mon_max_pg_per_osd]", "s3cr3t"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logBuf := &bytes.Buffer{}
			capnslog.SetFormatter(capnslog.NewPrettyFormatter(logBuf, false))
			capnslog.SetGlobalLogLevel(capnslog.TRACE)
			t.Cleanup(func() {
				capnslog.SetFormatter(capnslog.NewDefaultFormatter(os.Stderr))
				capnslog.SetGlobalLogLevel(capnslog.INFO)
			})

			executor := &exectest.MockExecutor{}
			ranCommand := false
			executor.MockExecuteCommandWithTimeout = func(timeout time.Duration, command string, args ...string) (string, error) {
				ranCommand = true
				return "", nil
			}
			ctx := &clusterd.Context{
				Clientset: testop.New(t, 1),
				Executor:  executor,
				ConfigDir: t.TempDir(),
			}
			monStore := GetMonStore(ctx, client.AdminTestClusterInfo("mycluster"))

			err := monStore.SetAll("client.rgw.my.store.a", map[string]string{
				"rgw_zone": "zone",
				tt.key:     tt.value,
			})
			require.Error(t, err)
			assert.ErrorContains(t, err, strconv.Quote(tt.key))
			assert.NotContains(t, err.Error(), "s3cr3t", "the error must not include the value")
			assert.False(t, ranCommand, "no setting may be applied when one is rejected")
			assert.Regexp(t, ` E \| op-config: .*`+regexp.QuoteMeta(strconv.Quote(tt.key)), logBuf.String(),
				"the rejection must be logged at ERROR")
			assert.NotContains(t, logBuf.String(), "s3cr3t", "the log must not include the value")
		})
	}
}

func TestMonStore_SetAllAcceptsBracketsInValue(t *testing.T) {
	executor := &exectest.MockExecutor{}
	var assimilated string
	var readErr error
	executor.MockExecuteCommandWithTimeout = func(timeout time.Duration, command string, args ...string) (string, error) {
		if i := slices.Index(args, "-i"); i >= 0 && i+1 < len(args) {
			var b []byte
			b, readErr = os.ReadFile(args[i+1])
			assimilated = string(b)
		}
		return "", nil
	}
	ctx := &clusterd.Context{
		Clientset: testop.New(t, 1),
		Executor:  executor,
		ConfigDir: t.TempDir(),
	}
	monStore := GetMonStore(ctx, client.AdminTestClusterInfo("mycluster"))

	err := monStore.SetAll("client.rgw.my.store.a", map[string]string{
		"rgw_frontends": "beast endpoint=[::]:8080",
	})
	assert.NoError(t, err)
	require.NoError(t, readErr)
	assert.Equal(t, "[client.rgw.my.store.a]\nrgw_frontends = beast endpoint=[::]:8080\n", assimilated)
}

func TestFilterSettingsMap(t *testing.T) {
	tests := []struct {
		name     string
		input    CephConfigOptionsMap
		expected CephConfigOptionsMap
	}{
		{
			name: "no filtered config options",
			input: map[string]map[string]string{
				"global": {
					"osd_pool_default_size": "1",
				},
			},
			expected: map[string]map[string]string{
				"global": {
					"osd_pool_default_size": "1",
				},
			},
		},
		{
			name: "filtered config options",
			input: map[string]map[string]string{
				"global": {
					"mon_host":              "192.168.1.100:687",
					"osd_pool_default_size": "1",
					"bluefs_buffered_io":    "false",
				},
			},
			expected: map[string]map[string]string{
				"global": {
					"osd_pool_default_size": "1",
					"bluefs_buffered_io":    "false",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, filterSettingsMap(tt.input))
		})
	}
}
