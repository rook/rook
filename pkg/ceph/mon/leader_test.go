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
	"strings"
	"testing"
	"time"

	cephtest "github.com/rook/rook/pkg/ceph/test"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/clusterd/inventory"
	"github.com/rook/rook/pkg/util"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

func TestMonSelection(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	nodes := make(map[string]*inventory.NodeConfig)
	inv := &inventory.Config{Nodes: nodes}
	nodes["a"] = &inventory.NodeConfig{PublicIP: "1.2.3.4"}
	context := &clusterd.Context{
		DirectContext: clusterd.DirectContext{EtcdClient: etcdClient, Inventory: inv},
		ConfigDir:     "/tmp",
	}

	// choose 1 mon from the set of 1 nodes
	chosen, bad, err := chooseMonitorNodes(context)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(chosen))
	assert.Equal(t, 0, len(bad))

	// no new monitors when we have 2 nodes
	nodes["b"] = &inventory.NodeConfig{PublicIP: "2.2.3.4"}
	chosen, bad, err = chooseMonitorNodes(context)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(chosen))
	assert.Equal(t, 0, len(bad))
	assert.Equal(t, "mon0", chosen["a"].Name)

	// add two more monitors when we hit the threshold of 3 nodes
	nodes["c"] = &inventory.NodeConfig{PublicIP: "3.2.3.4"}
	chosen, bad, err = chooseMonitorNodes(context)
	assert.Nil(t, err)
	assert.Equal(t, 3, len(chosen))
	assert.Equal(t, 0, len(bad))
	assert.Equal(t, "mon0", chosen["a"].Name)
	assert.True(t, chosen["b"].Name == "mon1" || chosen["b"].Name == "mon2")
	assert.True(t, chosen["c"].Name == "mon1" || chosen["c"].Name == "mon2")
	assert.NotEqual(t, chosen["b"].Name, chosen["c"].Name)

	// remove a node, then check that with four nodes we only go to three monitors
	etcdClient.DeleteDir("/rook/services/ceph/monitor/desired/c")
	nodes["c"] = &inventory.NodeConfig{PublicIP: "4.2.3.4"}
	nodes["d"] = &inventory.NodeConfig{PublicIP: "5.2.3.4"}
	chosen, bad, err = chooseMonitorNodes(context)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(bad))
	assert.Equal(t, 3, len(chosen))
	assert.Equal(t, "mon0", chosen["a"].Name)
}

func TestMonitorCount(t *testing.T) {
	assert.Equal(t, 0, calculateMonitorCount(0))
	assert.Equal(t, 1, calculateMonitorCount(1))
	assert.Equal(t, 1, calculateMonitorCount(2))
	assert.Equal(t, 3, calculateMonitorCount(3))
	assert.Equal(t, 3, calculateMonitorCount(4))
	assert.Equal(t, 3, calculateMonitorCount(20))
	assert.Equal(t, 5, calculateMonitorCount(21))
	assert.Equal(t, 5, calculateMonitorCount(100))
	assert.Equal(t, 7, calculateMonitorCount(101))
	assert.Equal(t, 7, calculateMonitorCount(1001))
}

func TestMonOnUnhealthyNode(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()

	// mock a monitor
	cephtest.CreateClusterInfo(etcdClient, []string{"a"})

	// the monitor is on the bad node
	badNode := &clusterd.UnhealthyNode{ID: "a"}
	context := &clusterd.Context{DirectContext: clusterd.DirectContext{EtcdClient: etcdClient}, ConfigDir: "/tmp"}
	response, err := monsOnUnhealthyNode(context, []*clusterd.UnhealthyNode{badNode})
	assert.True(t, response)
	assert.Nil(t, err)

	// the monitor is not on another node
	badNode.ID = "b"
	response, err = monsOnUnhealthyNode(context, []*clusterd.UnhealthyNode{badNode})
	assert.False(t, response)
	assert.Nil(t, err)
}

func TestMoveUnhealthyMonitor(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	waitForQuorum := func(context *clusterd.Context, cluster *ClusterInfo) error {
		return nil
	}
	leader := &Leader{waitForQuorum: waitForQuorum}
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(actionName string, command string, args ...string) (string, error) {
			cephtest.CreateClusterInfo(etcdClient, []string{"a", "b", "c"})
			return "mysecret", nil
		},
	}

	nodes := make(map[string]*inventory.NodeConfig)
	inv := &inventory.Config{Nodes: nodes}
	nodes["a"] = &inventory.NodeConfig{PublicIP: "1.2.3.4"}
	nodes["b"] = &inventory.NodeConfig{PublicIP: "2.3.4.5"}
	nodes["c"] = &inventory.NodeConfig{PublicIP: "3.4.5.6"}

	context := &clusterd.Context{
		DirectContext: clusterd.DirectContext{EtcdClient: etcdClient, Inventory: inv},
		ConfigDir:     "/tmp",
		Executor:      executor,
	}

	// mock the agent responses that the deployments were successful to start mons and osds
	etcdClient.WatcherResponses["/rook/_notify/a/monitor/status"] = "succeeded"
	etcdClient.WatcherResponses["/rook/_notify/b/monitor/status"] = "succeeded"
	etcdClient.WatcherResponses["/rook/_notify/c/monitor/status"] = "succeeded"

	// initialize the first three monitors
	err := leader.Configure(context, "mykey")
	assert.Nil(t, err)
	assert.True(t, etcdClient.GetChildDirs("/rook/services/ceph/monitor/desired").Equals(util.CreateSet([]string{"a", "b", "c"})))

	// add a new node and mark node a as unhealthy
	nodes["a"].HeartbeatAge = (UnhealthyHeartbeatAgeSeconds + 1) * time.Second
	nodes["d"] = &inventory.NodeConfig{PublicIP: "4.5.6.7"}
	etcdClient.WatcherResponses["/rook/_notify/d/monitor/status"] = "succeeded"

	err = leader.Configure(context, "mykey")
	assert.Nil(t, err)
	assert.True(t, etcdClient.GetChildDirs("/rook/services/ceph/monitor/desired").Equals(util.CreateSet([]string{"d", "b", "c"})))

	cluster, err := LoadClusterInfo(etcdClient)
	assert.Nil(t, err)
	assert.Equal(t, 3, len(cluster.Monitors))
	mon1 := false
	mon2 := false
	mon3 := false
	for _, mon := range cluster.Monitors {
		if strings.Contains(mon.Endpoint, nodes["a"].PublicIP) {
			assert.Fail(t, "mon a was not removed")
		}
		if strings.Contains(mon.Endpoint, nodes["b"].PublicIP) {
			mon1 = true
		}
		if strings.Contains(mon.Endpoint, nodes["c"].PublicIP) {
			mon2 = true
		}
		if strings.Contains(mon.Endpoint, nodes["d"].PublicIP) {
			mon3 = true
		}
	}

	assert.True(t, mon1)
	assert.True(t, mon2)
	assert.True(t, mon3)
}

func TestMaxMonID(t *testing.T) {

	// the empty list returns -1
	mons := map[string]*CephMonitorConfig{}
	max, err := getMaxMonitorID(mons)
	assert.Nil(t, err)
	assert.Equal(t, -1, max)

	testBadMonID(t, "")
	testBadMonID(t, "m")
	testBadMonID(t, "m1")
	testBadMonID(t, "m100")
	testBadMonID(t, "1mon")
	testBadMonID(t, "badmon")
	testBadMonID(t, "mons1")

	testGoodMonID(t, "mon0", 0)
	testGoodMonID(t, "mon10", 10)
	testGoodMonID(t, "mon123", 123)
}

func testBadMonID(t *testing.T, name string) {
	mons := map[string]*CephMonitorConfig{}
	mons["a"] = &CephMonitorConfig{Name: name}
	_, err := getMaxMonitorID(mons)
	assert.NotNil(t, err, fmt.Sprintf("bad mon=%s", name))
}

func testGoodMonID(t *testing.T, name string, expected int) {
	mons := map[string]*CephMonitorConfig{}
	mons["a"] = &CephMonitorConfig{Name: name}
	actual, err := getMaxMonitorID(mons)
	assert.Nil(t, err)
	assert.Equal(t, expected, actual)
}

func TestUnhealthyMon(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	nodes := make(map[string]*inventory.NodeConfig)
	inv := &inventory.Config{Nodes: nodes}
	nodes["a"] = &inventory.NodeConfig{PublicIP: "1.2.3.4", HeartbeatAge: UnhealthyHeartbeatAgeSeconds * time.Second}
	nodes["b"] = &inventory.NodeConfig{PublicIP: "2.2.3.4"}
	nodes["c"] = &inventory.NodeConfig{PublicIP: "3.2.3.4"}
	nodes["d"] = &inventory.NodeConfig{PublicIP: "4.2.3.4"}
	context := &clusterd.Context{
		DirectContext: clusterd.DirectContext{EtcdClient: etcdClient, Inventory: inv},
		ConfigDir:     "/tmp",
	}

	// choose 3 mons from the set of 4 nodes, but don't choose the unhealthy node 'a'
	chosen, bad, err := chooseMonitorNodes(context)
	assert.Nil(t, err)
	assert.Equal(t, 3, len(chosen))
	_, found := chosen["a"]
	assert.False(t, found)
	bmonName := chosen["b"].Name
	cmonName := chosen["c"].Name
	desiredMons := etcdClient.GetChildDirs("/rook/services/ceph/monitor/desired")
	assert.True(t, desiredMons.Contains("b"))
	assert.True(t, desiredMons.Contains("c"))
	assert.True(t, desiredMons.Contains("d"))
	assert.False(t, desiredMons.Contains("a"))

	// now we expect to add a and remove b from the desired state
	nodes["a"].HeartbeatAge = 0
	nodes["b"].HeartbeatAge = UnhealthyHeartbeatAgeSeconds * time.Second
	chosen, bad, err = chooseMonitorNodes(context)
	assert.Nil(t, err)
	assert.Equal(t, 3, len(chosen))
	assert.Equal(t, 1, len(bad))
	assert.Equal(t, "mon3", chosen["a"].Name)
	assert.Equal(t, bmonName, bad["b"].Name)

	desiredMons = etcdClient.GetChildDirs("/rook/services/ceph/monitor/desired")
	assert.True(t, desiredMons.Contains("a"))
	assert.True(t, desiredMons.Contains("c"))
	assert.True(t, desiredMons.Contains("d"))
	assert.False(t, desiredMons.Contains("b"))

	// fail choosing the nodes if we don't have enough healthy nodes to move.
	// 'b' is still unhealthy and 'c' now becomes unhealthy
	nodes["c"].HeartbeatAge = UnhealthyHeartbeatAgeSeconds * time.Second
	chosen, bad, err = chooseMonitorNodes(context)
	assert.NotNil(t, err)
	assert.Equal(t, 1, len(bad))
	assert.Equal(t, cmonName, bad["c"].Name)
}
