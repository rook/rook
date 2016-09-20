package castled

import (
	"fmt"
	"testing"

	"github.com/quantum/castle/pkg/clusterd"
	"github.com/quantum/castle/pkg/clusterd/inventory"
	"github.com/quantum/castle/pkg/util"
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
	}

	// choose 1 mon from the set of 1 nodes
	chosen, err := chooseMonitorNodes(context)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(chosen))

	// no new monitors when we have 2 nodes
	nodes["b"] = &inventory.NodeConfig{IPAddress: "2.2.3.4"}
	chosen, err = chooseMonitorNodes(context)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(chosen))
	assert.Equal(t, "mon0", chosen["a"].Name)

	// add two more monitors when we hit the threshold of 3 nodes
	nodes["c"] = &inventory.NodeConfig{IPAddress: "3.2.3.4"}
	chosen, err = chooseMonitorNodes(context)
	assert.Nil(t, err)
	assert.Equal(t, 3, len(chosen))
	assert.Equal(t, "mon0", chosen["a"].Name)
	assert.True(t, chosen["b"].Name == "mon1" || chosen["b"].Name == "mon2")
	assert.True(t, chosen["c"].Name == "mon1" || chosen["c"].Name == "mon2")
	assert.NotEqual(t, chosen["b"].Name, chosen["c"].Name)

	// remove a node, then check that with four nodes we only go to three monitors
	etcdClient.DeleteDir("/castle/services/ceph/monitor/desired/c")
	nodes["c"] = &inventory.NodeConfig{IPAddress: "4.2.3.4"}
	nodes["d"] = &inventory.NodeConfig{IPAddress: "5.2.3.4"}
	chosen, err = chooseMonitorNodes(context)
	assert.Nil(t, err)
	assert.Equal(t, 3, len(chosen))
	assert.Equal(t, "mon0", chosen["a"].Name)

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
