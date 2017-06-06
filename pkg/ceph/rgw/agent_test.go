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
package rgw

import (
	"io/ioutil"
	"os"
	"testing"

	cephtest "github.com/rook/rook/pkg/ceph/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/rook/rook/pkg/util/proc"
	"github.com/stretchr/testify/assert"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util"
)

func TestStartRGW(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(actionName string, command string, args ...string) (string, error) {
			return "{\"key\":\"mysecurekey\"}", nil
		},
	}
	configDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(configDir)
	context := &clusterd.Context{
		DirectContext: clusterd.DirectContext{EtcdClient: etcdClient, NodeID: "123"},
		Executor:      executor,
		ProcMan:       proc.New(executor),
		ConfigDir:     configDir,
	}

	cephtest.CreateClusterInfo(etcdClient, configDir, []string{context.NodeID})

	// nothing to stop without rgw in desired state
	agent := NewAgent()
	err := agent.ConfigureLocalService(context)
	assert.Nil(t, err)
	assert.Nil(t, agent.rgwProc)

	// add the rgw to desired state
	err = setRGWState(context.EtcdClient, context.NodeID, false)
	etcdClient.SetValue("/rook/services/ceph/object/desired/keyring", "1234")
	assert.Nil(t, err)

	// start the rgw
	err = agent.ConfigureLocalService(context)
	assert.Nil(t, err)
	assert.NotNil(t, agent.rgwProc)

	// remove the rgw from desired state
	removeRGWState(context.EtcdClient, context.NodeID, false)

	// stop the rgw
	err = agent.ConfigureLocalService(context)
	assert.Nil(t, err)
	assert.Nil(t, agent.rgwProc)
}
