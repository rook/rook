package cephmgr

import (
	"log"
	"strings"
	"testing"
	"time"

	"github.com/quantum/castle/pkg/cephmgr/client"
	testceph "github.com/quantum/castle/pkg/cephmgr/client/test"
	"github.com/quantum/castle/pkg/clusterd"
	"github.com/quantum/castle/pkg/clusterd/inventory"
	"github.com/quantum/castle/pkg/util"
	"github.com/stretchr/testify/assert"
)

// ************************************************************************************************
// ************************************************************************************************
//
// unit test functions
//
// ********************************"****************************************************************
// ************************************************************************************************
func TestCephLeaders(t *testing.T) {
	factory := &testceph.MockConnectionFactory{Fsid: "myfsid", SecretKey: "mykey"}
	leader := newCephLeader(factory)

	leader.StartWatchEvents()
	defer leader.Close()

	nodes := make(map[string]*inventory.NodeConfig)
	inv := &inventory.Config{Nodes: nodes}
	nodes["a"] = &inventory.NodeConfig{IPAddress: "1.2.3.4"}

	etcdClient := util.NewMockEtcdClient()
	context := &clusterd.Context{
		EtcdClient: etcdClient,
		Inventory:  inv,
	}

	// mock the agent responses that the deployments were successful to start mons and osds
	etcdClient.WatcherResponses["/castle/_notify/a/monitor/status"] = "succeeded"
	etcdClient.WatcherResponses["/castle/_notify/b/monitor/status"] = "succeeded"
	etcdClient.WatcherResponses["/castle/_notify/a/osd/status"] = "succeeded"
	etcdClient.WatcherResponses["/castle/_notify/b/osd/status"] = "succeeded"

	// trigger a refresh event
	refresh := clusterd.NewRefreshEvent(context)
	leader.Events() <- refresh

	// wait for the event queue to be empty
	waitForEvents(leader)

	assert.True(t, etcdClient.GetChildDirs("/castle/services/ceph/osd/desired").Equals(util.CreateSet([]string{"a"})))
	assert.Equal(t, "mon0", etcdClient.GetValue("/castle/services/ceph/monitor/desired/a/id"))
	assert.Equal(t, "1.2.3.4", etcdClient.GetValue("/castle/services/ceph/monitor/desired/a/ipaddress"))
	assert.Equal(t, "6790", etcdClient.GetValue("/castle/services/ceph/monitor/desired/a/port"))

	// trigger an add node event
	nodes["b"] = &inventory.NodeConfig{IPAddress: "2.3.4.5"}
	addNode := clusterd.NewAddNodeEvent(context, "b")
	leader.Events() <- addNode

	// wait for the event queue to be empty
	waitForEvents(leader)

	assert.True(t, etcdClient.GetChildDirs("/castle/services/ceph/osd/desired").Equals(util.CreateSet([]string{"a", "b"})))
	assert.Equal(t, "myfsid", etcdClient.GetValue("/castle/services/ceph/fsid"))
	assert.Equal(t, "mykey", etcdClient.GetValue("/castle/services/ceph/_secrets/admin"))
}

func waitForEvents(leader *cephLeader) {
	// add a placeholder event to the queue. When it is dequeued we know the rest of the events have completed.
	e := newNonEvent()
	leader.Events() <- e

	// wait for the Name() method to be called on the nonevent, which means it was dequeued
	log.Printf("waiting for event queue to empty")
	<-e.signaled
	log.Printf("event queue is empty")
}

// Empty event for testing
type nonEvent struct {
	signaled   chan bool
	nameCalled bool
}

func newNonEvent() *nonEvent {
	return &nonEvent{signaled: make(chan bool)}
}

func (e *nonEvent) Name() string {
	if !e.nameCalled {
		e.nameCalled = true
		e.signaled <- true
	}
	return "nonevent"
}
func (e *nonEvent) Context() *clusterd.Context {
	return nil
}

func TestMoveUnhealthyMonitor(t *testing.T) {
	factory := &testceph.MockConnectionFactory{Fsid: "myfsid", SecretKey: "mykey"}
	leader := newCephLeader(factory)
	etcdClient := util.NewMockEtcdClient()
	leader.monLeader.waitForQuorum = func(factory client.ConnectionFactory, context *clusterd.Context, cluster *ClusterInfo) error {
		return nil
	}

	nodes := make(map[string]*inventory.NodeConfig)
	inv := &inventory.Config{Nodes: nodes}
	nodes["a"] = &inventory.NodeConfig{IPAddress: "1.2.3.4"}
	nodes["b"] = &inventory.NodeConfig{IPAddress: "2.3.4.5"}
	nodes["c"] = &inventory.NodeConfig{IPAddress: "3.4.5.6"}

	context := &clusterd.Context{
		EtcdClient: etcdClient,
		Inventory:  inv,
	}

	// mock the agent responses that the deployments were successful to start mons and osds
	etcdClient.WatcherResponses["/castle/_notify/a/monitor/status"] = "succeeded"
	etcdClient.WatcherResponses["/castle/_notify/b/monitor/status"] = "succeeded"
	etcdClient.WatcherResponses["/castle/_notify/c/monitor/status"] = "succeeded"

	// initialize the first three monitors
	err := leader.configureCephMons(context)
	assert.Nil(t, err)
	assert.True(t, etcdClient.GetChildDirs("/castle/services/ceph/monitor/desired").Equals(util.CreateSet([]string{"a", "b", "c"})))

	// add a new node and mark node a as unhealthy
	nodes["a"].HeartbeatAge = (unhealthyMonHeatbeatAgeSeconds + 1) * time.Second
	nodes["d"] = &inventory.NodeConfig{IPAddress: "4.5.6.7"}
	etcdClient.WatcherResponses["/castle/_notify/d/monitor/status"] = "succeeded"

	err = leader.configureCephMons(context)
	assert.Nil(t, err)
	assert.True(t, etcdClient.GetChildDirs("/castle/services/ceph/monitor/desired").Equals(util.CreateSet([]string{"d", "b", "c"})))

	cluster, err := LoadClusterInfo(etcdClient)
	assert.Nil(t, err)
	assert.Equal(t, 3, len(cluster.Monitors))
	mon1 := false
	mon2 := false
	mon3 := false
	for _, mon := range cluster.Monitors {
		if strings.Contains(mon.Endpoint, nodes["a"].IPAddress) {
			assert.Fail(t, "mon a was not removed")
		}
		if strings.Contains(mon.Endpoint, nodes["b"].IPAddress) {
			mon1 = true
		}
		if strings.Contains(mon.Endpoint, nodes["c"].IPAddress) {
			mon2 = true
		}
		if strings.Contains(mon.Endpoint, nodes["d"].IPAddress) {
			mon3 = true
		}
	}

	assert.True(t, mon1)
	assert.True(t, mon2)
	assert.True(t, mon3)
}

func TestExtractDesiredDeviceNode(t *testing.T) {
	node, err := extractNodeIDFromDesiredDevice("/castle/services/ceph/osd/desired/abc/device/sdb")
	assert.Nil(t, err)
	assert.Equal(t, "abc", node)

	node, err = extractNodeIDFromDesiredDevice("/castle/services/ceph/osd/desired")
	assert.NotNil(t, err)
	assert.Equal(t, "", node)
}
