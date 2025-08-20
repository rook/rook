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

package client

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/test"
	"github.com/rook/rook/pkg/util/exec"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

func TestFinalizeCephCommandArgs(t *testing.T) {
	RunAllCephCommandsInToolboxPod = ""
	configDir := "/var/lib/rook/rook-ceph"
	expectedCommand := "ceph"
	args := []string{"quorum_status"}
	expectedArgs := []string{
		"quorum_status",
		"--connect-timeout=" + strconv.Itoa(int(exec.CephCommandsTimeout.Seconds())),
		"--cluster=rook",
		"--conf=/var/lib/rook/rook-ceph/rook/rook.config",
		"--name=client.admin",
		"--keyring=/var/lib/rook/rook-ceph/rook/client.admin.keyring",
	}

	clusterInfo := AdminTestClusterInfo("rook")
	cmd, gotArgs := FinalizeCephCommandArgs(expectedCommand, clusterInfo, args, configDir)
	assert.Exactly(t, expectedCommand, cmd)
	assert.Exactly(t, expectedArgs, gotArgs)

	t.Run("keyring override", func(t *testing.T) {
		// run the same test as above, but set the keyring override
		clusterInfo.KeyringFileOverride = "/some/random/file/path.extension"
		// keyring arg should change but nothing else
		expectedArgs[len(expectedArgs)-1] = "--keyring=/some/random/file/path.extension"
		cmd, args := FinalizeCephCommandArgs(expectedCommand, clusterInfo, args, configDir)
		assert.Exactly(t, expectedCommand, cmd)
		assert.Exactly(t, expectedArgs, args)
	})
}

func TestFinalizeRadosGWAdminCommandArgs(t *testing.T) {
	RunAllCephCommandsInToolboxPod = ""
	configDir := "/var/lib/rook/rook-ceph"
	expectedCommand := "radosgw-admin"
	args := []string{
		"realm",
		"create",
		"--default",
		"--rgw-realm=default-rook",
		"--rgw-zonegroup=default-rook",
	}

	expectedArgs := []string{
		"realm",
		"create",
		"--default",
		"--rgw-realm=default-rook",
		"--rgw-zonegroup=default-rook",
		"--cluster=rook",
		"--conf=/var/lib/rook/rook-ceph/rook/rook.config",
		"--name=client.admin",
		"--keyring=/var/lib/rook/rook-ceph/rook/client.admin.keyring",
	}

	clusterInfo := AdminTestClusterInfo("rook")
	cmd, args := FinalizeCephCommandArgs(expectedCommand, clusterInfo, args, configDir)
	assert.Exactly(t, expectedCommand, cmd)
	assert.Exactly(t, expectedArgs, args)
}

func TestFinalizeCephCommandArgsToolBox(t *testing.T) {
	RunAllCephCommandsInToolboxPod = "rook-ceph-tools"
	configDir := "/var/lib/rook/rook-ceph"
	expectedCommand := "ceph"
	args := []string{"health"}
	expectedArgs := []string{
		"exec",
		"-i",
		"rook-ceph-tools",
		"-n",
		"rook",
		"--",
		"timeout",
		"15",
		"ceph",
		"health",
		"--connect-timeout=15",
	}

	clusterInfo := AdminTestClusterInfo("rook")
	exec.CephCommandsTimeout = 15 * time.Second
	cmd, args := FinalizeCephCommandArgs(expectedCommand, clusterInfo, args, configDir)
	assert.Exactly(t, "kubectl", cmd)
	assert.Exactly(t, expectedArgs, args)
	RunAllCephCommandsInToolboxPod = ""
}

func TestNewRBDCommand(t *testing.T) {
	args := []string{"create", "--size", "1G", "myvol"}

	t.Run("rbd command with no multus", func(t *testing.T) {
		clusterInfo := AdminTestClusterInfo("rook")
		executor := &exectest.MockExecutor{}
		executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
			switch {
			case command == "rbd" && args[0] == "create":
				assert.Len(t, args, 8)
				return "success", nil
			}
			return "", errors.Errorf("unexpected ceph command %q", args)
		}
		context := &clusterd.Context{Executor: executor}
		cmd := NewRBDCommand(context, clusterInfo, args)
		assert.False(t, cmd.RemoteExecution)
		output, err := cmd.Run()
		assert.NoError(t, err)
		assert.Equal(t, "success", string(output))
	})
	t.Run("rbd command with multus", func(t *testing.T) {
		clusterInfo := AdminTestClusterInfo("rook")
		clusterInfo.NetworkSpec.Provider = "multus"
		executor := &exectest.MockExecutor{}
		context := &clusterd.Context{Executor: executor, RemoteExecutor: exec.RemotePodCommandExecutor{ClientSet: test.New(t, 3)}}
		cmd := NewRBDCommand(context, clusterInfo, args)
		assert.True(t, cmd.RemoteExecution)
		_, err := cmd.Run()
		assert.Error(t, err)
		assert.Len(t, cmd.args, 4)
		// This is not the best but it shows we go through the right codepath
		assert.Contains(t, err.Error(), "no pods found with selector \"rook-ceph-mgr\"")
	})

	t.Run("context canceled nothing to run", func(t *testing.T) {
		clusterInfo := AdminTestClusterInfo("rook")
		ctx, cancel := context.WithCancel(context.TODO())
		clusterInfo.Context = ctx
		cancel()
		executor := &exectest.MockExecutor{}
		context := &clusterd.Context{Executor: executor, RemoteExecutor: exec.RemotePodCommandExecutor{ClientSet: test.New(t, 3)}}
		cmd := NewRBDCommand(context, clusterInfo, args)
		_, err := cmd.Run()
		assert.Error(t, err)
		// This is not the best but it shows we go through the right codepath
		assert.EqualError(t, err, "context canceled")
	})
}

func TestNewGaneshaRadosGraceCommand(t *testing.T) {
	anyArgContains := func(substr string, args []string) bool {
		for _, arg := range args {
			if strings.Contains(arg, substr) {
				return true
			}
		}
		return false
	}

	args := []string{"--pool", ".nfs", "--ns", "my-nfs", "add", "node"}

	timeout := time.Second

	ctx := func() *clusterd.Context {
		return &clusterd.Context{
			Executor: &exectest.MockExecutor{
				MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, arg ...string) (string, error) {
					t.Logf("command: %s %v", command, arg)
					assert.Equal(t, time.Second, timeout)

					assert.Equal(t, "ganesha-rados-grace", command)
					assert.Equal(t, []string{"--pool", ".nfs", "--ns", "my-nfs", "add", "node"}, arg[0:6])

					// ganesha-rados-grace's conf flag is --cephconf
					assert.True(t, anyArgContains("--cephconf=", arg))
					// ganesha-rados-grace accepts NO standard flags
					assert.False(t, anyArgContains("--cluster", arg))
					assert.False(t, anyArgContains("--conf", arg))
					assert.False(t, anyArgContains("--name", arg))
					assert.False(t, anyArgContains("--keyring", arg))
					assert.False(t, anyArgContains("--format", arg))

					return "", nil
				},
			},
		}
	}

	clusterInfo := func() *ClusterInfo {
		return &ClusterInfo{
			Namespace: "rook-ceph",
			Context:   context.Background(),
		}
	}

	t.Run("normal call", func(t *testing.T) {
		cmd := NewGaneshaRadosGraceCommand(ctx(), clusterInfo(), args)
		assert.Equal(t, false, cmd.JsonOutput)

		_, err := cmd.RunWithTimeout(timeout)
		assert.NoError(t, err)
	})

	t.Run("error call", func(t *testing.T) {
		ctx := ctx()
		ctx.Executor = &exectest.MockExecutor{
			MockExecuteCommandWithTimeout: func(timeout time.Duration, command string, arg ...string) (string, error) {
				return "", fmt.Errorf("induced error")
			},
		}

		cmd := NewGaneshaRadosGraceCommand(ctx, clusterInfo(), args)
		_, err := cmd.RunWithTimeout(timeout)
		assert.Error(t, err)
	})
}
