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
	"testing"

	"github.com/rook/rook/pkg/cephmgr/client/test"
	"github.com/rook/rook/pkg/cephmgr/mon"
	cephtest "github.com/rook/rook/pkg/cephmgr/test"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/clusterd/inventory"
	"github.com/rook/rook/pkg/util"

	"os"

	"github.com/stretchr/testify/assert"
)

func TestDefaultDesiredState(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	context := &clusterd.Context{EtcdClient: etcdClient}

	err := EnableFileSystem(context)
	assert.Nil(t, err)
	assert.Equal(t, defaultPoolName, etcdClient.GetValue("/rook/services/ceph/fs/desired/rookfs/pool"))

	err = RemoveFileSystem(context)
	assert.Nil(t, err)
	assert.Equal(t, 0, etcdClient.GetChildDirs("/rook/services/ceph/fs/desired").Count())
}

func TestMarkApplied(t *testing.T) {
	leader := &Leader{}
	etcdClient := util.NewMockEtcdClient()
	context := &clusterd.Context{EtcdClient: etcdClient}

	fileSystems, err := leader.loadFileSystems(context, true, nil)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(fileSystems))

	fs := NewFS(context, nil, "myfs", "mypool")

	err = fs.markApplied()
	assert.Nil(t, err)

	assert.Equal(t, "mypool", etcdClient.GetValue("/rook/services/ceph/fs/applied/myfs/pool"))

	fileSystems, err = leader.loadFileSystems(context, true, nil)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(fileSystems))
	assert.Equal(t, "myfs", fileSystems["myfs"].ID)
	assert.Equal(t, "mypool", fileSystems["myfs"].Pool)
}

func TestAddFileToDesired(t *testing.T) {
	leader := &Leader{}
	etcdClient := util.NewMockEtcdClient()
	context := &clusterd.Context{EtcdClient: etcdClient}

	fileSystems, err := leader.loadFileSystems(context, false, nil)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(fileSystems))

	fs := NewFS(context, nil, "myfs", "yourpool")
	err = fs.AddToDesiredState()
	assert.Nil(t, err)

	assert.Equal(t, "yourpool", etcdClient.GetValue("/rook/services/ceph/fs/desired/myfs/pool"))

	fileSystems, err = leader.loadFileSystems(context, false, nil)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(fileSystems))
	assert.Equal(t, "myfs", fileSystems["myfs"].ID)
	assert.Equal(t, "yourpool", fileSystems["myfs"].Pool)
}

func TestAddRemoveFileSystem(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	nodes := make(map[string]*inventory.NodeConfig)
	inv := &inventory.Config{Nodes: nodes}

	context := &clusterd.Context{EtcdClient: etcdClient, Inventory: inv, ConfigDir: "/tmp/file"}
	defer os.RemoveAll(context.ConfigDir)

	fs := NewFS(context, nil, "myfs", "yourpool")
	err := fs.AddToDesiredState()
	assert.Nil(t, err)

	cephtest.CreateClusterInfo(etcdClient, []string{"a"})

	factory := &test.MockConnectionFactory{}
	_, err = mon.CreateClusterInfo(factory, "secret")
	assert.Nil(t, err)

	// fail when there are no nodes
	leader := &Leader{}
	err = leader.Configure(context, factory)
	assert.NotNil(t, err)

	// add a couple nodes to choose from
	nodes["a"] = &inventory.NodeConfig{PublicIP: "1.2.3.4"}
	nodes["b"] = &inventory.NodeConfig{PublicIP: "2.2.3.4"}
	etcdClient.WatcherResponses["/rook/_notify/a/mds/status"] = "succeeded"
	etcdClient.WatcherResponses["/rook/_notify/b/mds/status"] = "succeeded"

	// succeed when there are nodes
	err = leader.Configure(context, factory)
	assert.Nil(t, err)
	assert.Equal(t, "1", etcdClient.GetValue("/rook/services/ceph/mds/desired/node/a/id"))
	assert.Equal(t, "1", etcdClient.GetValue("/rook/services/ceph/mds/applied/node/a/id"))

	// remove the file system
	err = fs.DeleteFromDesiredState()
	assert.Nil(t, err)
	assert.Equal(t, 0, etcdClient.GetChildDirs("/rook/services/ceph/mds/desired/node").Count())
	assert.Equal(t, "1", etcdClient.GetValue("/rook/services/ceph/mds/applied/node/a/id"))

	err = leader.Configure(context, factory)
	assert.Nil(t, err)
	assert.Equal(t, 0, etcdClient.GetChildDirs("/rook/services/ceph/mds/desired/node").Count())
	assert.Equal(t, 0, etcdClient.GetChildDirs("/rook/services/ceph/mds/applied/node").Count())
}
