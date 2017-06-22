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
package clusterd

import (
	"path"
	"testing"

	"github.com/rook/rook/pkg/clusterd/inventory"
	"github.com/rook/rook/pkg/util"
	"github.com/stretchr/testify/assert"
)

func TestLoadDiscoveredNodes(t *testing.T) {
	etcdClient := &util.MockEtcdClient{}
	mockHandler := newTestServiceLeader()
	raised := make(chan bool)
	mockHandler.unhealthyNode = func(nodes map[string]*UnhealthyNode) {
		assert.Equal(t, 1, len(nodes))
		assert.Equal(t, "23", nodes["23"].ID)
		raised <- true
	}

	context := &Context{DirectContext: DirectContext{EtcdClient: etcdClient}}
	context.Services = []*ClusterService{
		&ClusterService{Name: "test", Leader: mockHandler},
	}
	leader := newServicesLeader(context)
	leader.refresher.Start()
	defer leader.refresher.Stop()
	leader.parent = &ClusterMember{isLeader: true}

	etcdClient.SetValue(path.Join(inventory.NodesConfigKey, "23", "publicIp"), "1.2.3.4")
	etcdClient.SetValue(path.Join(inventory.NodesConfigKey, "23", "privateIp"), "10.2.3.4")

	// one unhealthy nodes to discover
	err := leader.discoverUnhealthyNodes()
	<-raised
	assert.Nil(t, err)
}
