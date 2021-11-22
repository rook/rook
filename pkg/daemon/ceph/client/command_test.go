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
	"strconv"
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
	cmd, args := FinalizeCephCommandArgs(expectedCommand, clusterInfo, args, configDir)
	assert.Exactly(t, expectedCommand, cmd)
	assert.Exactly(t, expectedArgs, args)
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
