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
	"os"
	"testing"

	testceph "github.com/rook/rook/pkg/cephmgr/client/test"
	cephtest "github.com/rook/rook/pkg/cephmgr/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/rook/rook/pkg/util/proc"
	"github.com/stretchr/testify/assert"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util"
)

func TestStartRGW(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	executor := &exectest.MockExecutor{}
	context := &clusterd.Context{
		EtcdClient: etcdClient,
		NodeID:     "123",
		Executor:   executor,
		ProcMan:    proc.New(executor),
		ConfigDir:  "/tmp/rgw",
	}
	defer os.RemoveAll(context.ConfigDir)

	cephtest.CreateClusterInfo(etcdClient, []string{context.NodeID})
	factory := &testceph.MockConnectionFactory{Fsid: "f", SecretKey: "k"}
	conn, _ := factory.NewConnWithClusterAndUser("mycluster", "user")
	conn.(*testceph.MockConnection).MockMonCommand = func(buf []byte) (buffer []byte, info string, err error) {
		response := "{\"key\":\"mysecurekey\"}"
		return []byte(response), "", nil
	}

	// nothing to stop without rgw in desired state
	agent := NewAgent(factory)
	err := agent.ConfigureLocalService(context)
	assert.Nil(t, err)
	assert.Nil(t, agent.rgwProc)

	// add the rgw to desired state
	err = setRGWState(context.EtcdClient, context.NodeID, false)
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
