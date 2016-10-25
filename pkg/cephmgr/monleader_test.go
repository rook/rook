package cephmgr

import (
	"fmt"
	"path"
	"testing"
	"time"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/clusterd/inventory"
	"github.com/rook/rook/pkg/util"
	"github.com/stretchr/testify/assert"
)

func TestMonSelection(t *testing.T) {
	etcdClient := util.NewMockEtcdClient()
	nodes := make(map[string]*inventory.NodeConfig)
	inv := &inventory.Config{Nodes: nodes}
	nodes["a"] = &inventory.NodeConfig{IPAddress: "1.2.3.4"}
	context := &clusterd.Context{
		EtcdClient: etcdClient,
		Inventory:  inv,
		ConfigDir:  "/tmp",
	}

	// choose 1 mon from the set of 1 nodes
	chosen, bad, err := chooseMonitorNodes(context)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(chosen))
	assert.Equal(t, 0, len(bad))

	// no new monitors when we have 2 nodes
	nodes["b"] = &inventory.NodeConfig{IPAddress: "2.2.3.4"}
	chosen, bad, err = chooseMonitorNodes(context)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(chosen))
	assert.Equal(t, 0, len(bad))
	assert.Equal(t, "mon0", chosen["a"].Name)

	// add two more monitors when we hit the threshold of 3 nodes
	nodes["c"] = &inventory.NodeConfig{IPAddress: "3.2.3.4"}
	chosen, bad, err = chooseMonitorNodes(context)
	assert.Nil(t, err)
	assert.Equal(t, 3, len(chosen))
	assert.Equal(t, 0, len(bad))
	assert.Equal(t, "mon0", chosen["a"].Name)
	assert.True(t, chosen["b"].Name == "mon1" || chosen["b"].Name == "mon2")
	assert.True(t, chosen["c"].Name == "mon1" || chosen["c"].Name == "mon2")
	assert.NotEqual(t, chosen["b"].Name, chosen["c"].Name)

	// remove a node, then check that with four nodes we only go to three monitors
	etcdClient.DeleteDir("/castle/services/ceph/monitor/desired/c")
	nodes["c"] = &inventory.NodeConfig{IPAddress: "4.2.3.4"}
	nodes["d"] = &inventory.NodeConfig{IPAddress: "5.2.3.4"}
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
	createTestClusterInfo(etcdClient, []string{"a"})

	// the monitor is on the bad node
	badNode := &clusterd.UnhealthyNode{NodeID: "a"}
	context := &clusterd.Context{EtcdClient: etcdClient, ConfigDir: "/tmp"}
	response, err := monsOnUnhealthyNode(context, []*clusterd.UnhealthyNode{badNode})
	assert.True(t, response)
	assert.Nil(t, err)

	// the monitor is not on another node
	badNode.NodeID = "b"
	response, err = monsOnUnhealthyNode(context, []*clusterd.UnhealthyNode{badNode})
	assert.False(t, response)
	assert.Nil(t, err)
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
	nodes["a"] = &inventory.NodeConfig{IPAddress: "1.2.3.4", HeartbeatAge: unhealthyMonHeatbeatAgeSeconds * time.Second}
	nodes["b"] = &inventory.NodeConfig{IPAddress: "2.2.3.4"}
	nodes["c"] = &inventory.NodeConfig{IPAddress: "3.2.3.4"}
	nodes["d"] = &inventory.NodeConfig{IPAddress: "4.2.3.4"}
	context := &clusterd.Context{
		EtcdClient: etcdClient,
		Inventory:  inv,
		ConfigDir:  "/tmp",
	}

	// choose 3 mons from the set of 4 nodes, but don't choose the unhealthy node 'a'
	chosen, bad, err := chooseMonitorNodes(context)
	assert.Nil(t, err)
	assert.Equal(t, 3, len(chosen))
	_, found := chosen["a"]
	assert.False(t, found)
	bmonName := chosen["b"].Name
	cmonName := chosen["c"].Name
	desiredMons := etcdClient.GetChildDirs("/castle/services/ceph/monitor/desired")
	assert.True(t, desiredMons.Contains("b"))
	assert.True(t, desiredMons.Contains("c"))
	assert.True(t, desiredMons.Contains("d"))
	assert.False(t, desiredMons.Contains("a"))

	// now we expect to add a and remove b from the desired state
	nodes["a"].HeartbeatAge = 0
	nodes["b"].HeartbeatAge = unhealthyMonHeatbeatAgeSeconds * time.Second
	chosen, bad, err = chooseMonitorNodes(context)
	assert.Nil(t, err)
	assert.Equal(t, 3, len(chosen))
	assert.Equal(t, 1, len(bad))
	assert.Equal(t, "mon3", chosen["a"].Name)
	assert.Equal(t, bmonName, bad["b"].Name)

	desiredMons = etcdClient.GetChildDirs("/castle/services/ceph/monitor/desired")
	assert.True(t, desiredMons.Contains("a"))
	assert.True(t, desiredMons.Contains("c"))
	assert.True(t, desiredMons.Contains("d"))
	assert.False(t, desiredMons.Contains("b"))

	// fail choosing the nodes if we don't have enough healthy nodes to move.
	// 'b' is still unhealthy and 'c' now becomes unhealthy
	nodes["c"].HeartbeatAge = unhealthyMonHeatbeatAgeSeconds * time.Second
	chosen, bad, err = chooseMonitorNodes(context)
	assert.NotNil(t, err)
	assert.Equal(t, 1, len(bad))
	assert.Equal(t, cmonName, bad["c"].Name)
}

func createTestClusterInfo(etcdClient *util.MockEtcdClient, mons []string) {
	key := "/castle/services/ceph"
	etcdClient.SetValue(path.Join(key, "fsid"), "12345")
	etcdClient.SetValue(path.Join(key, "name"), "default")
	etcdClient.SetValue(path.Join(key, "_secrets/monitor"), "foo")
	etcdClient.SetValue(path.Join(key, "_secrets/admin"), "bar")

	base := "/castle/services/ceph/monitor/desired"
	for i, mon := range mons {
		etcdClient.SetValue(path.Join(base, mon, "id"), fmt.Sprintf("mon%d", i))
		etcdClient.SetValue(path.Join(base, mon, "ipaddress"), fmt.Sprintf("1.2.3.%d", i))
		etcdClient.SetValue(path.Join(base, mon, "port"), "4321")
	}
}
