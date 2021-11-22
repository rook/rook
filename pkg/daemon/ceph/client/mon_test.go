/*
Copyright 2018 The Rook Authors. All rights reserved.

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
	"time"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util/exec"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

func TestCephArgs(t *testing.T) {
	// cluster a under /etc
	args := []string{}
	clusterInfo := AdminTestClusterInfo("a")
	exec.CephCommandsTimeout = 15 * time.Second
	command, args := FinalizeCephCommandArgs(CephTool, clusterInfo, args, "/etc")
	assert.Equal(t, CephTool, command)
	assert.Equal(t, 5, len(args))
	assert.Equal(t, "--connect-timeout=15", args[0])
	assert.Equal(t, "--cluster=a", args[1])
	assert.Equal(t, "--conf=/etc/a/a.config", args[2])
	assert.Equal(t, "--name=client.admin", args[3])
	assert.Equal(t, "--keyring=/etc/a/client.admin.keyring", args[4])

	RunAllCephCommandsInToolboxPod = "rook-ceph-tools"
	args = []string{}
	command, args = FinalizeCephCommandArgs(CephTool, clusterInfo, args, "/etc")
	assert.Equal(t, Kubectl, command)
	assert.Equal(t, 10, len(args), fmt.Sprintf("%+v", args))
	assert.Equal(t, "exec", args[0])
	assert.Equal(t, "-i", args[1])
	assert.Equal(t, "rook-ceph-tools", args[2])
	assert.Equal(t, "-n", args[3])
	assert.Equal(t, clusterInfo.Namespace, args[4])
	assert.Equal(t, "--", args[5])
	assert.Equal(t, CephTool, args[8])
	assert.Equal(t, "--connect-timeout=15", args[9])
	RunAllCephCommandsInToolboxPod = ""

	// cluster under /var/lib/rook
	args = []string{"myarg"}
	command, args = FinalizeCephCommandArgs(RBDTool, clusterInfo, args, "/var/lib/rook")
	assert.Equal(t, RBDTool, command)
	assert.Equal(t, 5, len(args))
	assert.Equal(t, "myarg", args[0])
	assert.Equal(t, "--cluster="+clusterInfo.Namespace, args[1])
	assert.Equal(t, "--conf=/var/lib/rook/a/a.config", args[2])
	assert.Equal(t, "--name=client.admin", args[3])
	assert.Equal(t, "--keyring=/var/lib/rook/a/client.admin.keyring", args[4])
}

func TestStretchElectionStrategy(t *testing.T) {
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[0] == "mon" && args[1] == "set" && args[2] == "election_strategy" {
			assert.Equal(t, "connectivity", args[3])
			return "", nil
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}
	context := &clusterd.Context{Executor: executor}
	clusterInfo := AdminTestClusterInfo("mycluster")

	err := EnableStretchElectionStrategy(context, clusterInfo)
	assert.NoError(t, err)
}

func TestStretchClusterMonTiebreaker(t *testing.T) {
	monName := "a"
	failureDomain := "rack"
	setTiebreaker := false
	enabledStretch := false
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		switch {
		case args[0] == "mon" && args[1] == "enable_stretch_mode":
			enabledStretch = true
			assert.Equal(t, monName, args[2])
			assert.Equal(t, defaultStretchCrushRuleName, args[3])
			assert.Equal(t, failureDomain, args[4])
			return "", nil
		case args[0] == "mon" && args[1] == "set_new_tiebreaker":
			setTiebreaker = true
			assert.Equal(t, monName, args[2])
			return "", nil
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}
	context := &clusterd.Context{Executor: executor}
	clusterInfo := AdminTestClusterInfo("mycluster")

	err := SetMonStretchTiebreaker(context, clusterInfo, monName, failureDomain)
	assert.NoError(t, err)
	assert.True(t, enabledStretch)
	assert.False(t, setTiebreaker)
	enabledStretch = false

	err = SetNewTiebreaker(context, clusterInfo, monName)
	assert.NoError(t, err)
	assert.True(t, setTiebreaker)
	assert.False(t, enabledStretch)
}

func TestMonDump(t *testing.T) {
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		switch {
		case args[0] == "mon" && args[1] == "dump":
			return `{"epoch":3,"fsid":"6a31a264-9090-4048-8d95-4b8c3cde909d","modified":"2020-12-09T18:13:36.346150Z","created":"2020-12-09T18:13:13.014270Z","min_mon_release":15,"min_mon_release_name":"octopus",
		"features":{"persistent":["kraken","luminous","mimic","osdmap-prune","nautilus","octopus"],"optional":[]},
		"election_strategy":1,"mons":[
		{"rank":0,"name":"a","crush_location":"{zone=a}","public_addrs":{"addrvec":[{"type":"v2","addr":"10.109.80.104:3300","nonce":0},{"type":"v1","addr":"10.109.80.104:6789","nonce":0}]},"addr":"10.109.80.104:6789/0","public_addr":"10.109.80.104:6789/0","priority":0,"weight":0},
		{"rank":1,"name":"b","crush_location":"{zone=b}","public_addrs":{"addrvec":[{"type":"v2","addr":"10.107.12.199:3300","nonce":0},{"type":"v1","addr":"10.107.12.199:6789","nonce":0}]},"addr":"10.107.12.199:6789/0","public_addr":"10.107.12.199:6789/0","priority":0,"weight":0},
		{"rank":2,"name":"c","crush_location":"{zone=c}","public_addrs":{"addrvec":[{"type":"v2","addr":"10.107.5.207:3300","nonce":0},{"type":"v1","addr":"10.107.5.207:6789","nonce":0}]},"addr":"10.107.5.207:6789/0","public_addr":"10.107.5.207:6789/0","priority":0,"weight":0}],
		"quorum":[0,1,2]}`, nil
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}
	context := &clusterd.Context{Executor: executor}
	clusterInfo := AdminTestClusterInfo("mycluster")

	dump, err := GetMonDump(context, clusterInfo)
	assert.NoError(t, err)
	assert.Equal(t, 1, dump.ElectionStrategy)
	assert.Equal(t, "{zone=a}", dump.Mons[0].CrushLocation)
	assert.Equal(t, "a", dump.Mons[0].Name)
	assert.Equal(t, 0, dump.Mons[0].Rank)
	assert.Equal(t, "b", dump.Mons[1].Name)
	assert.Equal(t, 1, dump.Mons[1].Rank)
	assert.Equal(t, 3, len(dump.Mons))
	assert.Equal(t, 3, len(dump.Quorum))
}
