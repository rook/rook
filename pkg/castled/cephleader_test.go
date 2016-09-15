package castled

import (
	"log"
	"testing"

	"github.com/quantum/castle/pkg/clusterd/inventory"
	"github.com/quantum/castle/pkg/clusterd"
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
	leader := &cephLeader{mockCeph: true}
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
	addNode := clusterd.NewAddNodeEvent(context, []string{"b"})
	leader.Events() <- addNode

	// wait for the event queue to be empty
	waitForEvents(leader)

	assert.True(t, etcdClient.GetChildDirs("/castle/services/ceph/osd/desired").Equals(util.CreateSet([]string{"a", "b"})))
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
