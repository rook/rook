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
package mon

import (
	"fmt"
	"os/exec"
	"path"
	"testing"

	testceph "github.com/rook/rook/pkg/cephmgr/client/test"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util"
	"github.com/rook/rook/pkg/util/exec/test"
	"github.com/rook/rook/pkg/util/proc"
	"github.com/stretchr/testify/assert"
)

func TestMonAgent(t *testing.T) {
	executor := &test.MockExecutor{}

	runCommands := 0
	executor.MockExecuteCommand = func(name string, command string, args ...string) error {
		logger.Infof("RUN %d. %s %+v", runCommands, command, args)
		switch {
		case runCommands == 0:
			assert.Equal(t, "--mkfs", args[3])
		default:
			assert.Fail(t, fmt.Sprintf("unexpected case %d", runCommands))
		}

		runCommands++
		return nil
	}

	startCommands := 0
	executor.MockStartExecuteCommand = func(name string, command string, args ...string) (*exec.Cmd, error) {
		logger.Infof("START %d. %s %+v", startCommands, command, args)
		cmd := &exec.Cmd{Args: append([]string{command}, args...)}
		assert.Equal(t, "daemon", args[0])
		assert.Equal(t, "--type=mon", args[1])
		assert.Equal(t, "--", args[2])
		assert.Equal(t, "--cluster=rookcluster", args[4])
		assert.Equal(t, "--name=mon.mon0", args[5])
		assert.Equal(t, "--mon-data=/tmp/mon0/mon.mon0", args[6])
		assert.Equal(t, "--conf=/tmp/mon0/rookcluster.config", args[7])
		switch {
		case startCommands == 0:
			assert.Equal(t, "--foreground", args[3])
			assert.Equal(t, "--public-addr=1.2.3.4:2345", args[8])
		default:
			return cmd, fmt.Errorf("unexpected case %d", startCommands)
		}
		startCommands++
		return cmd, nil
	}

	etcdClient := util.NewMockEtcdClient()
	context := &clusterd.Context{
		EtcdClient: etcdClient,
		NodeID:     "a",
		ProcMan:    proc.New(executor),
		ConfigDir:  "/tmp",
	}

	factory := &testceph.MockConnectionFactory{Fsid: "f", SecretKey: "k"}
	cluster, err := createOrGetClusterInfo(factory, etcdClient, "")
	assert.Nil(t, err)
	assert.NotNil(t, cluster)

	// nothing expected to be configured because the node is not in the desired state
	agent := &agent{}
	err = agent.ConfigureLocalService(context)
	assert.Nil(t, err)
	assert.Equal(t, 0, runCommands)
	assert.Equal(t, 0, startCommands)
	assert.Nil(t, agent.monProc)

	// set the agent in the desired state
	key := path.Join(CephKey, monitorAgentName, clusterd.DesiredKey, context.NodeID)
	etcdClient.SetValue(path.Join(key, "id"), "mon0")
	etcdClient.SetValue(path.Join(key, "ipaddress"), "1.2.3.4")
	etcdClient.SetValue(path.Join(key, "port"), "2345")

	// now the monitor will be configured
	err = agent.ConfigureLocalService(context)
	assert.Nil(t, err)
	assert.Equal(t, 1, runCommands)
	assert.Equal(t, 1, startCommands)
	assert.NotNil(t, agent.monProc)

	// when the mon is not in desired state, it will be removed
	etcdClient.DeleteDir(key)
	err = agent.ConfigureLocalService(context)
	assert.Nil(t, err)
	assert.Equal(t, 1, runCommands)
	assert.Equal(t, 1, startCommands)
	assert.Nil(t, agent.monProc)
}
