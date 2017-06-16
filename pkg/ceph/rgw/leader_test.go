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
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"testing"

	etcd "github.com/coreos/etcd/client"
	ctx "golang.org/x/net/context"

	cephtest "github.com/rook/rook/pkg/ceph/test"
	"github.com/rook/rook/pkg/clusterd/inventory"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/rook/rook/pkg/util/proc"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util"

	"github.com/stretchr/testify/assert"
)

func TestRGWConfig(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	nodes := map[string]*inventory.NodeConfig{
		"a": &inventory.NodeConfig{PublicIP: "1.2.3.4"},
		"b": &inventory.NodeConfig{PublicIP: "2.3.4.5"},
	}
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(actionName string, command string, outFileArg string, args ...string) (string, error) {
			if args[0] == "auth" && args[1] == "get-or-create-key" {
				return `{"key":"mykey"}`, nil
			}
			response := `{"keys": [
				{"access_key": "myaccessid", "secret_key": "mybigsecretkey"}
			]}`

			return response, nil
		},
	}
	configDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(configDir)
	context := &clusterd.Context{DirectContext: clusterd.DirectContext{EtcdClient: etcdClient, Inventory: &inventory.Config{Nodes: nodes}},
		ProcMan: proc.New(executor), Executor: executor, ConfigDir: configDir}
	leader := NewLeader()

	// mock a monitor
	cephtest.CreateClusterInfo(etcdClient, configDir, []string{"mymon"})

	// Nothing happens when not in desired state
	err := leader.Configure(context)
	assert.Nil(t, err)
	desired, err := getObjectStoreState(context, false)
	assert.Nil(t, err)
	assert.False(t, desired)
	applied, err := getObjectStoreState(context, true)
	assert.Nil(t, err)
	assert.False(t, applied)

	// Add the object store to desired state
	err = EnableObjectStore(etcdClient)
	assert.Nil(t, err)
	desired, _ = getObjectStoreState(context, false)
	assert.True(t, desired)

	etcdClient.WatcherResponses["/rook/_notify/a/rgw/status"] = "succeeded"
	etcdClient.WatcherResponses["/rook/_notify/b/rgw/status"] = "succeeded"

	// Configure the object store
	err = leader.Configure(context)
	assert.Nil(t, err)
	verifyObjectConfigured(t, context, true)

	// Get the RGW endpoints and verify
	host, ipAddr, found, err := GetRGWEndpoints(etcdClient, context.Inventory)
	assert.Nil(t, err)
	assert.True(t, found)
	assert.Equal(t, "rook-ceph-rgw:53390", host)
	assert.True(t, ipAddr == "1.2.3.4:53390" || ipAddr == "2.3.4.5:53390", fmt.Sprintf("unexpected rgw IP endpoint: %s", ipAddr))

	// Remove the object service
	err = RemoveObjectStore(etcdClient)
	assert.Nil(t, err)
	assert.Equal(t, "", etcdClient.GetValue("/rook/services/ceph/object/applied/admin/id"))
	assert.Equal(t, "", etcdClient.GetValue("/rook/services/ceph/object/applied/admin/_secret"))
	err = leader.Configure(context)
	assert.Nil(t, err)
	verifyObjectConfigured(t, context, false)
}

func verifyObjectConfigured(t *testing.T, context *clusterd.Context, configured bool) {
	desired, err := getObjectStoreState(context, false)
	assert.Nil(t, err)
	assert.Equal(t, configured, desired)
	applied, err := getObjectStoreState(context, true)
	assert.Nil(t, err)
	assert.Equal(t, configured, applied)

	// Check that both nodes are in desired and applied state
	desired, err = getRGWState(context.EtcdClient, "a", false)
	assert.Nil(t, err)
	assert.Equal(t, configured, desired)
	desired, err = getRGWState(context.EtcdClient, "b", false)
	assert.Nil(t, err)
	assert.Equal(t, configured, desired)
	desired, err = getRGWState(context.EtcdClient, "a", true)
	assert.Nil(t, err)
	assert.Equal(t, configured, desired)
	desired, err = getRGWState(context.EtcdClient, "b", true)
	assert.Nil(t, err)
	assert.Equal(t, configured, desired)
}

func TestDefaultDesiredState(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()

	err := EnableObjectStore(etcdClient)
	assert.Nil(t, err)
	assert.Equal(t, "1", etcdClient.GetValue("/rook/services/ceph/object/desired/state"))

	err = RemoveObjectStore(etcdClient)
	assert.Nil(t, err)
	assert.Equal(t, 0, etcdClient.GetChildDirs("/rook/services/ceph/object/desired").Count())
}

func TestMarkApplied(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	context := &clusterd.Context{DirectContext: clusterd.DirectContext{EtcdClient: etcdClient}}

	err := markApplied(context)
	assert.Nil(t, err)

	assert.Equal(t, "1", etcdClient.GetValue("/rook/services/ceph/object/applied/state"))

	err = markUnapplied(context)
	assert.Nil(t, err)
	assert.Equal(t, 0, etcdClient.GetChildDirs("/rook/services/ceph/object").Count())
}

func TestGetDesiredNodes(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	nodes := map[string]*inventory.NodeConfig{}
	context := &clusterd.Context{DirectContext: clusterd.DirectContext{EtcdClient: etcdClient, Inventory: &inventory.Config{Nodes: nodes}}}
	leader := NewLeader()

	// no nodes to select
	desired, err := leader.getDesiredRGWNodes(context, 0)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(desired))

	nodes["a"] = &inventory.NodeConfig{}
	nodes["b"] = &inventory.NodeConfig{}

	// no nodes desired
	desired, err = leader.getDesiredRGWNodes(context, 0)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(desired))

	// select only one node that was already in desired state
	etcdClient.SetValue(path.Join(getRGWNodeKey("a", false), "state"), "1")
	desired, err = leader.getDesiredRGWNodes(context, 1)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(desired))
	assert.Equal(t, "a", desired[0])

	// select both nodes
	desired, err = leader.getDesiredRGWNodes(context, 2)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(desired))

	// fail to select three nodes
	desired, err = leader.getDesiredRGWNodes(context, 3)
	assert.NotNil(t, err)
}

func TestGetRGWEndpointsFailure(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	etcdClient.MockGet = func(context ctx.Context, key string, opts *etcd.GetOptions) (*etcd.Response, error) {
		return nil, fmt.Errorf("mock etcd GET error")
	}

	host, ipAddr, found, err := GetRGWEndpoints(etcdClient, nil)
	assert.Equal(t, "", host)
	assert.Equal(t, "", ipAddr)
	assert.False(t, found)
	assert.NotNil(t, err)
}

func TestGetRGWEndpointsNotFound(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	nodes := map[string]*inventory.NodeConfig{}
	clusterInventory := &inventory.Config{Nodes: nodes}

	host, ipAddr, found, err := GetRGWEndpoints(etcdClient, clusterInventory)
	assert.Equal(t, "", host)
	assert.Equal(t, "", ipAddr)
	assert.False(t, found)
	assert.Nil(t, err)
}
