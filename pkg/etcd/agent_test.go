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
package etcd

import (
	"path"
	"testing"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/etcd/test"
	"github.com/rook/rook/pkg/util"
	"github.com/stretchr/testify/assert"
)

func TestEtcdMgrAgent(t *testing.T) {
	mockContext := test.MockContext{}
	// adding 1.2.3.4 as the first/existing cluster member
	mockContext.AddMembers([]string{"http://1.2.3.4:53379"})
	mockEmbeddedEtcdFactory := test.MockEmbeddedEtcdFactory{}

	// agent2 is the agent on node 2 which is going to create a new embedded etcd
	agent2 := &etcdMgrAgent{context: &mockContext, etcdFactory: &mockEmbeddedEtcdFactory}
	etcdClient2 := util.NewMockEtcdClient()
	context2 := &clusterd.Context{
		DirectContext: clusterd.DirectContext{EtcdClient: etcdClient2, NodeID: "node2"},
	}
	err := agent2.Initialize(context2)
	assert.Equal(t, "etcd", agent2.Name())
	assert.Nil(t, agent2.embeddedEtcd)

	err = agent2.ConfigureLocalService(context2)
	assert.Nil(t, err)

	//set the agent in the desired state
	key := path.Join(etcdmgrKey, clusterd.DesiredKey, context2.NodeID)
	etcdClient2.SetValue(path.Join(key, "ipaddress"), "2.3.4.5")

	err = agent2.ConfigureLocalService(context2)
	assert.Nil(t, err)
	assert.NotNil(t, agent2.embeddedEtcd)
	//remove the desired status
	etcdClient2.DeleteDir(key)
	err = agent2.ConfigureLocalService(context2)
	assert.Nil(t, err)
	assert.Nil(t, agent2.embeddedEtcd)
}
