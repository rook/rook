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
	"testing"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/clusterd/inventory"
	"github.com/rook/rook/pkg/etcd/test"
	"github.com/rook/rook/pkg/util"
	"github.com/stretchr/testify/assert"
)

// TestEtcdMgrLeaders tests Etcd Cluster's grow scenario
func TestEtcdMgrLeaderGrow(t *testing.T) {
	mockContext := test.MockContext{}
	nodes := make(map[string]*inventory.NodeConfig)
	inv := &inventory.Config{Nodes: nodes}

	// adding 1.2.3.4 as the first/existing cluster member
	nodes["a"] = &inventory.NodeConfig{PrivateIP: "1.2.3.4"}
	mockContext.AddMembers([]string{"http://1.2.3.4:53379"})
	service := etcdMgrLeader{context: &mockContext}

	nodes["b"] = &inventory.NodeConfig{PrivateIP: "2.3.4.5"}
	nodes["c"] = &inventory.NodeConfig{PrivateIP: "3.4.5.6"}

	etcdClient := util.NewMockEtcdClient()

	// mock the agent responses that the deployments were successful to create an instance of embeddedEtcd
	etcdClient.WatcherResponses["/rook/_notify/b/etcd/status"] = "succeeded"
	etcdClient.WatcherResponses["/rook/_notify/c/etcd/status"] = "succeeded"

	refresh := clusterd.NewRefreshEvent()
	refresh.NodesAdded.Add("b")
	refresh.Context = &clusterd.Context{
		DirectContext: clusterd.DirectContext{EtcdClient: etcdClient, Inventory: inv},
	}

	service.HandleRefresh(refresh)

	// there might be a&c or b&c in the desired states
	assert.True(t, etcdClient.GetChildDirs("/rook/services/etcd/desired").Count() == 2)
	assert.Equal(t, etcdClient.GetValue("/rook/services/etcd/desired/b/ipaddress"), "2.3.4.5")
	assert.Equal(t, etcdClient.GetValue("/rook/services/etcd/desired/c/ipaddress"), "3.4.5.6")
}

func TestEtcdMgrLeaderShrink(t *testing.T) {
	mockContext := test.MockContext{}
	nodes := make(map[string]*inventory.NodeConfig)
	inv := &inventory.Config{Nodes: nodes}

	// adding 1.2.3.4 as the first/existing cluster member
	nodes["a"] = &inventory.NodeConfig{PrivateIP: "1.2.3.4"}
	mockContext.AddMembers([]string{"http://1.2.3.4:53379"})
	nodes["b"] = &inventory.NodeConfig{PrivateIP: "2.3.4.5"}
	mockContext.AddMembers([]string{"http://2.3.4.5:53379"})
	nodes["c"] = &inventory.NodeConfig{PrivateIP: "3.4.5.6"}
	mockContext.AddMembers([]string{"http://3.4.5.6:53379"})
	service := etcdMgrLeader{context: &mockContext}

	nodes["a"] = &inventory.NodeConfig{PrivateIP: "1.2.3.4"}
	nodes["b"] = &inventory.NodeConfig{PrivateIP: "2.3.4.5"}
	nodes["c"] = &inventory.NodeConfig{PrivateIP: "3.4.5.6"}

	// mock the agent responses that the deployments were successful to create an instance of embeddedEtcd
	etcdClient := util.NewMockEtcdClient()
	etcdClient.WatcherResponses["/rook/_notify/a/etcd/status"] = "succeeded"
	etcdClient.WatcherResponses["/rook/_notify/c/etcd/status"] = "succeeded"
	etcdClient.WatcherResponses["/rook/services/etcd/desired/a/ipaddress"] = "1.2.3.4"
	etcdClient.WatcherResponses["/rook/services/etcd/desired/b/ipaddress"] = "2.3.4.5"
	etcdClient.WatcherResponses["/rook/services/etcd/desired/c/ipaddress"] = "2.3.4.5"

	unhealthyNode := &clusterd.UnhealthyNode{AgeSeconds: 10, ID: "c"}
	refresh := clusterd.NewRefreshEvent()
	refresh.NodesUnhealthy["c"] = unhealthyNode
	refresh.Context = &clusterd.Context{
		DirectContext: clusterd.DirectContext{EtcdClient: etcdClient, Inventory: inv},
	}

	service.HandleRefresh(refresh)

	// two possibilities: there might be a&c or b&c in the desired states
	assert.True(t, etcdClient.GetChildDirs("/rook/services/etcd/desired").Count() == 0)
}
