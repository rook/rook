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
package mds

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

func TestGetSetMDSID(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	nodeID := "a"
	id, err := getMDSID(etcdClient, nodeID)
	assert.NotNil(t, err)

	err = setMDSID(etcdClient, nodeID, "23")
	assert.Nil(t, err)

	id, err = getMDSID(etcdClient, nodeID)
	assert.Nil(t, err)
	assert.Equal(t, "23", id)
}

func TestStartMDS(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	executor := &exectest.MockExecutor{}
	context := &clusterd.Context{
		EtcdClient: etcdClient,
		NodeID:     "123",
		Executor:   executor,
		ProcMan:    proc.New(executor),
		ConfigDir:  "/tmp/mds",
	}
	defer os.RemoveAll(context.ConfigDir)

	cephtest.CreateClusterInfo(etcdClient, []string{context.NodeID})
	factory := &testceph.MockConnectionFactory{Fsid: "f", SecretKey: "k"}
	conn, _ := factory.NewConnWithClusterAndUser("mycluster", "user")
	conn.(*testceph.MockConnection).MockMonCommand = func(buf []byte) (buffer []byte, info string, err error) {
		response := "{\"key\":\"mysecurekey\"}"
		return []byte(response), "", nil
	}

	// nothing to stop without mds in desired state
	agent := NewAgent(factory)
	err := agent.ConfigureLocalService(context)
	assert.Nil(t, err)

	// add the mds to desired state
	fs := &FileSystem{ID: "fs", context: context}
	mds := &mdsInfo{nodeID: context.NodeID, mdsID: "23", fileSystem: fs.ID}
	err = fs.storeMDSState(mds, false)
	assert.Nil(t, err)

	// start the mds
	err = agent.ConfigureLocalService(context)
	assert.Nil(t, err)
	assert.NotNil(t, agent.mdsProc)

	// remove the mds from desired state
	fs.removeMDSState(mds, false)

	// stop the mds
	err = agent.ConfigureLocalService(context)
	assert.Nil(t, err)
	assert.Nil(t, agent.mdsProc)
}
