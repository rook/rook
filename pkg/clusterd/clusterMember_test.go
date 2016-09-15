package clusterd

import (
	"errors"
	"fmt"
	"log"
	"path"
	"testing"
	"time"

	etcd "github.com/coreos/etcd/client"
	"github.com/quantum/castle/pkg/clusterd/inventory"
	"github.com/quantum/castle/pkg/util"
	"github.com/stretchr/testify/assert"
	ctx "golang.org/x/net/context"
)

// ************************************************************************************************
// Lease interface mock implementation
// ************************************************************************************************
type mockLease struct {
	mockRenew     func(time.Duration) error
	mockMachineID func() string
}

func (r *mockLease) Renew(period time.Duration) error {
	if r.mockRenew != nil {
		return r.mockRenew(period)
	}
	return nil
}
func (*mockLease) Release() error {
	return nil
}
func (r *mockLease) MachineID() string {
	if r.mockMachineID != nil {
		return r.mockMachineID()
	}
	return ""
}
func (*mockLease) Version() int {
	return 0
}
func (*mockLease) Index() uint64 {
	return 0
}
func (*mockLease) TimeRemaining() time.Duration {
	return time.Duration(0)
}

// ************************************************************************************************
// LeaseManager interface mock implementation
// ************************************************************************************************
type mockLeaseManager struct {
	mockGetLease     func(string) (Lease, error)
	mockAcquireLease func(string, string, int, time.Duration) (Lease, error)
}

func (r *mockLeaseManager) GetLease(name string) (Lease, error) {
	if r.mockGetLease != nil {
		return r.mockGetLease(name)
	}
	return nil, nil
}

func (r *mockLeaseManager) AcquireLease(name, machID string, ver int, period time.Duration) (Lease, error) {
	if r.mockAcquireLease != nil {
		return r.mockAcquireLease(name, machID, ver, period)
	}
	return nil, nil
}

func (r *mockLeaseManager) StealLease(name, machID string, ver int, period time.Duration, idx uint64) (Lease, error) {
	return nil, nil
}

// ************************************************************************************************
// Leader interface mock implementation
// ************************************************************************************************
type MockLeader struct {
	IsLeader       bool
	LostLeadership bool
	MembersAdded   int
}

func (l *MockLeader) OnLeadershipAcquired() error {
	l.IsLeader = true
	l.LostLeadership = false
	return nil
}

func (l *MockLeader) OnLeadershipLost() error {
	l.IsLeader = false
	l.LostLeadership = true
	return nil
}

func (l *MockLeader) OnNodeDiscovered(newNodeId string) error {
	l.MembersAdded++
	return nil
}

func (l *MockLeader) GetLeaseName() string {
	return "mock"
}

// ************************************************************************************************
// ************************************************************************************************
//
// unit test functions
//
// ************************************************************************************************
// ************************************************************************************************
func TestTrimMachineID(t *testing.T) {
	testTrimMachineID(t, " 123 		", "123")
	testTrimMachineID(t, " 1234567890", "1234567890")
	testTrimMachineID(t, " 123456789012", "123456789012")
	testTrimMachineID(t, " 1234567890123", "123456789012")
	testTrimMachineID(t, "1234567890123", "123456789012")
	testTrimMachineID(t, "123456789012345678", "123456789012")
}

func testTrimMachineID(t *testing.T, input, expected string) {
	result := trimMachineID(input)
	assert.Equal(t, expected, result)
}

func TestElectLeaderAcquireNil(t *testing.T) {
	_, context, mockLeaseManager, leader := createDefaultDependencies()

	// leader election will fail because AcquireLease returns nil
	clusterMember := newClusterMember(context, mockLeaseManager, leader)
	err := clusterMember.ElectLeader()
	assert.Nil(t, err)
	assert.False(t, clusterMember.isLeader)
}

func TestElectLeaderGetLeaseFails(t *testing.T) {
	_, context, mockLeaseManager, leader := createDefaultDependencies()
	getLeaseError := "get lease failed dude"
	mockLeaseManager.mockGetLease = func(name string) (Lease, error) {
		return nil, errors.New(getLeaseError)
	}

	clusterMember := newClusterMember(context, mockLeaseManager, leader)
	err := clusterMember.ElectLeader()
	assert.Equal(t, getLeaseError, err.Error())
}

func TestElectLeaderHeartbeatFails(t *testing.T) {
	etcdClient, context, mockLeaseManager, leader := createDefaultDependencies()
	etcdClient.MockSet = func(c ctx.Context, key, value string, opts *etcd.SetOptions) (*etcd.Response, error) {
		return nil, etcd.Error{Code: 999, Message: "mock etcd failure"}
	}

	// GetLease should return a lease indicating a different machine is the leader so the rest of ElectLeader
	// function is smooth sailing with no errors
	mockLeaseManager.mockGetLease = getLeaseNotLeader

	clusterMember := newClusterMember(context, mockLeaseManager, leader)

	// the heartbeat will run into an etcd error that it can't recover from, but since we'll just try
	// to heartbeat again later, and no other errors occur in ElectLeader, no error should be surfaced
	err := clusterMember.ElectLeader()
	assert.Nil(t, err)
}

func TestElectLeaderHeartbeatSucceeds(t *testing.T) {
	etcdClient, context, mockLeaseManager, leader := createDefaultDependencies()
	mockLeaseManager.mockGetLease = getLeaseNotLeader

	var actualKey string
	var actualTtl uint64
	etcdClient.MockSet = func(c ctx.Context, key, value string, opts *etcd.SetOptions) (*etcd.Response, error) {
		actualKey = key
		if opts != nil {
			actualTtl = uint64(opts.TTL.Seconds())
		}
		return nil, nil
	}

	// allow heartbeating to succeed then verify the heartbeat key was set in etcd
	clusterMember := newClusterMember(context, mockLeaseManager, leader)
	err := clusterMember.ElectLeader()

	assert.Nil(t, err)
	assert.Equal(t, fmt.Sprintf(inventory.DiscoveredNodesHealthKey+"/%s/heartbeat", context.NodeID), actualKey)
	assert.Equal(t, uint64(HeartbeatTtlSeconds), actualTtl)
}

func TestElectLeaderRenewal(t *testing.T) {
	_, context, mockLeaseManager, leader := createDefaultDependencies()

	renewCalled := false

	// GetLease should return a lease indicating we are already the leader so a renewal will be performed
	mockLeaseManager.mockGetLease = func(name string) (Lease, error) {
		existingLease := &mockLease{
			mockMachineID: func() string { return context.NodeID },
			mockRenew: func(period time.Duration) error {
				renewCalled = true
				return nil
			},
		}
		return existingLease, nil
	}

	clusterMember := newClusterMember(context, mockLeaseManager, leader)

	// elect a leader, since we're already the leader we expect Renew to be called and no error to surface
	err := clusterMember.ElectLeader()
	assert.Nil(t, err)
	assert.True(t, renewCalled)
	assert.True(t, clusterMember.isLeader)
}

func TestElectLeaderAcquireLease(t *testing.T) {
	etcdClient, context, mockLeaseManager, leader := createDefaultDependencies()
	mockLeaseManager.mockAcquireLease = acquireLeaseSuccessfully

	// once we're the leader, we'll ask for the cluster membership.  just return our machine ID since this is a
	// single machine cluster
	machineIds := []string{context.NodeID}
	setupGetMachineIds(etcdClient, machineIds)

	// try to elect a leader, we should win and aqcuire the lease
	clusterMember := newClusterMember(context, mockLeaseManager, leader)
	err := clusterMember.ElectLeader()

	assert.True(t, clusterMember.isLeader)
	assert.Nil(t, err)
}

func TestElectLeaderLoseLease(t *testing.T) {
	// setup the cluster member to first acquire leadership of the cluster
	clusterMember, mockLeaseManager, _ := setupAndRunAcquireLeaseScenario(t)

	// now that we've acquired leadership, perform another leader election where we lose it
	mockLeaseManager.mockGetLease = getLeaseNotLeader
	err := clusterMember.ElectLeader()

	// we should no longer think we are the leader, and we should have cleaned up our resources
	assert.Nil(t, err)
	assert.False(t, clusterMember.isLeader)
}

func TestElectLeaderLoseDueToGetLeaseFailure(t *testing.T) {
	// setup the cluster member to first acquire leadership of the cluster
	clusterMember, mockLeaseManager, _ := setupAndRunAcquireLeaseScenario(t)

	// now that we've acquired leadership, perform another leader election where we run into
	// an etcd error getting the current leader.  since we can't reliably tell if we still
	// have the lease, we should clean up.
	getLeaseError := "get lease failed, etcd is down or something, i dunno"
	mockLeaseManager.mockGetLease = func(name string) (Lease, error) {
		return nil, errors.New(getLeaseError)
	}
	err := clusterMember.ElectLeader()

	assert.Equal(t, getLeaseError, err.Error())
	assert.False(t, clusterMember.isLeader)
}

func TestElectLeaderLoseDueToAcquireLeaseNil(t *testing.T) {
	// setup the cluster member to first acquire leadership of the cluster
	clusterMember, mockLeaseManager, _ := setupAndRunAcquireLeaseScenario(t)

	// now that we've acquired leadership, perform another leader election where AcquireLease returns
	// nil.  since we can't reliably tell if we still have the lease, we should clean up.
	mockLeaseManager.mockAcquireLease = func(name, machID string, ver int, period time.Duration) (Lease, error) {
		return nil, nil
	}
	err := clusterMember.ElectLeader()

	assert.Nil(t, err)
	assert.False(t, clusterMember.isLeader)
}

func TestElectLeaderLoseDueToAcquireLeaseError(t *testing.T) {
	// setup the cluster member to first acquire leadership of the cluster
	clusterMember, mockLeaseManager, _ := setupAndRunAcquireLeaseScenario(t)

	// now that we've acquired leadership, perform another leader election where we run into
	// an etcd error acquiring the lease again.  since we can't reliably tell if we still
	// have the lease, we should clean up.
	acquireLeaseError := "acquire lease failed, etcd is down or something, i dunno"
	mockLeaseManager.mockAcquireLease = func(name, machID string, ver int, period time.Duration) (Lease, error) {
		return nil, errors.New(acquireLeaseError)
	}
	err := clusterMember.ElectLeader()

	assert.Equal(t, acquireLeaseError, err.Error())
	assert.False(t, clusterMember.isLeader)
}

func TestElectLeaderLoseDueToRenewLeaseError(t *testing.T) {
	// setup the cluster member to first acquire leadership of the cluster
	clusterMember, mockLeaseManager, machineId := setupAndRunAcquireLeaseScenario(t)

	// now that we've acquired leadership, perform another leader election where we run into
	// an etcd error renewing the lease.  since we can't reliably tell if we still
	// have the lease, we should clean up.
	renewLeaseError := "renew lease failed, etcd is down or something, i dunno"
	mockLeaseManager.mockGetLease = func(name string) (Lease, error) {
		existingLease := &mockLease{
			mockMachineID: func() string { return machineId },
			mockRenew: func(period time.Duration) error {
				return errors.New(renewLeaseError)
			},
		}
		return existingLease, nil
	}

	err := clusterMember.ElectLeader()

	assert.Equal(t, renewLeaseError, err.Error())
	assert.False(t, clusterMember.isLeader)
}

func TestTriggerRefresh(t *testing.T) {

	_, context, mockLeaseManager, _ := createDefaultDependencies()
	leader := &servicesLeader{context: context}
	r := newClusterMember(context, mockLeaseManager, leader)
	leader.parent = r
	// Skip the orchestration if not the leader
	triggered, err := leader.triggerRefresh()
	assert.False(t, triggered)
	assert.Nil(t, err)

	r.isLeader = true
	// FIX: Use channels instead of sleeps
	triggerRefreshInterval = 250 * time.Millisecond

	// The orchestration is triggered, but multiple triggers will still result in a single orchestrator
	triggered, _ = leader.triggerRefresh()
	assert.True(t, triggered)
	triggered, _ = leader.triggerRefresh()
	assert.True(t, triggered)
	triggered, _ = leader.triggerRefresh()
	assert.True(t, triggered)
	<-time.After(100 * time.Millisecond)
	assert.Equal(t, int32(1), leader.triggerRefreshLock)

	<-time.After(200 * time.Millisecond)
	assert.Equal(t, int32(0), leader.triggerRefreshLock)
}

func TestElectLeaderLoseLocallyButPersistInEtcd(t *testing.T) {
	// setup the cluster member to first acquire leadership of the cluster
	clusterMember, mockLeaseManager, machineId := setupAndRunAcquireLeaseScenario(t)

	// now that we've acquired leadership, perform another leader election where we run into
	// an etcd error getting the current leader.  since we can't reliably tell if we still
	// have the lease, we should clean up.
	getLeaseError := "get lease failed because etcd is down temporarily"
	mockLeaseManager.mockGetLease = func(name string) (Lease, error) {
		return nil, errors.New(getLeaseError)
	}
	err := clusterMember.ElectLeader()
	assert.Equal(t, getLeaseError, err.Error())
	assert.False(t, clusterMember.isLeader)

	// at this point, the only person who thinks we've lost leadership is ourselves locally.
	// because etcd was down for a bit, we cleaned up our leadership to be safe.
	// now, simulate etcd coming back online and still having us as the leader.  We should
	// respond by regsitering for RPCs etc just like normal acquiring of leadership, even
	// though we only lost it locally, and not in etcd.
	renewCalled := false
	mockLeaseManager.mockGetLease = func(name string) (Lease, error) {
		// GetLease should say that we're the current leader still
		existingLease := &mockLease{
			mockMachineID: func() string { return machineId },
			mockRenew: func(period time.Duration) error {
				renewCalled = true
				return nil
			},
		}
		return existingLease, nil
	}

	// run the leader election and verify the results
	err = clusterMember.ElectLeader()
	assert.Nil(t, err)
	assert.True(t, clusterMember.isLeader)
	assert.True(t, renewCalled)
}

func TestMembershipChangeWatchingStartStop(t *testing.T) {
	etcdClient, context, mockLeaseManager, _ := createDefaultDependencies()
	mockLeaseManager.mockAcquireLease = acquireLeaseSuccessfully
	machineIds := []string{context.NodeID}
	setupGetMachineIds(etcdClient, machineIds)

	// try to elect a leader, we should win and aqcuire the lease
	leader := &servicesLeader{context: context}
	clusterMember := newClusterMember(context, mockLeaseManager, leader)
	leader.parent = clusterMember
	err := clusterMember.ElectLeader()
	assert.True(t, leader.parent.isLeader)
	assert.Nil(t, err)

	// watching should have started now that we're leader and a watcher cancel function should have been created
	assert.NotNil(t, leader.watcherCancel)

	// now lose leadership, we should call the cancel function and nil it out
	mockLeaseManager.mockAcquireLease = func(name, machID string, ver int, period time.Duration) (Lease, error) {
		return nil, nil
	}
	clusterMember.ElectLeader()

	assert.Nil(t, leader.watcherCancel)
}

func TestSimpleMembershipChangeWatching(t *testing.T) {
	etcdClient, context, mockLeaseManager, _ := createDefaultDependencies()
	mockLeaseManager.mockAcquireLease = acquireLeaseSuccessfully

	testService := newTestServiceLeader()
	context.Services = []*ClusterService{&ClusterService{Name: "test", Leader: testService}}
	nodesAdded := 0
	nodeAddedChannel := make(chan bool)
	testService.nodeAdded = func(nodeID string) {
		nodesAdded++
		nodeAddedChannel <- true
	}

	machineIds := []string{context.NodeID}
	setupGetMachineIds(etcdClient, machineIds)

	// set up a mock watcher that the cluster leader will use
	newMemberChannel := make(chan string)
	membershipWatcher := &util.MockWatcher{
		MockNext: func(c ctx.Context) (*etcd.Response, error) {
			// wait for the test to send a new member ID to the channel, then return an etcd response
			// to caller of the watcher simulating the new machine has joined the cluster
			newMemberId := <-newMemberChannel
			return &etcd.Response{Action: CreateAction, Node: &etcd.Node{Key: path.Join(inventory.DiscoveredNodesKey, newMemberId)}}, nil
		},
	}
	etcdClient.MockWatcher = func(key string, opts *etcd.WatcherOptions) etcd.Watcher { return membershipWatcher }

	// try to elect a leader, we should win and aqcuire the lease
	leader := &servicesLeader{context: context}
	clusterMember := newClusterMember(context, mockLeaseManager, leader)
	clusterMember.isLeader = true
	leader.parent = clusterMember
	err := leader.onLeadershipAcquiredRefresh(false)
	defer leader.OnLeadershipLost()
	assert.Nil(t, err)

	// now that we're the leader, simulate a new member joining the cluster by triggering the etcd watcher
	newMachineId := "1234567890"
	newMemberChannel <- newMachineId

	log.Printf("Waiting for node to be added")
	<-nodeAddedChannel

	// the cluster leader should have added the new node
	assert.Equal(t, 1, nodesAdded)
}

func TestMembershipChangeWatchFiltering(t *testing.T) {
	// none of the following etcd keys/actions should result in a new cluster member being detected
	testMembershipChangeWatchFilteringHelper(t, "/castle/resources/", SetAction)
	testMembershipChangeWatchFilteringHelper(t, "/castle/resources/discovered/nodes", SetAction)
	testMembershipChangeWatchFilteringHelper(t, inventory.DiscoveredNodesKey+"/123", "update")
	testMembershipChangeWatchFilteringHelper(t, inventory.DiscoveredNodesKey+"/123/foo", "update")
}

func testMembershipChangeWatchFilteringHelper(t *testing.T, key string, action string) {
	etcdClient, context, mockLeaseManager, _ := createDefaultDependencies()
	mockLeaseManager.mockAcquireLease = acquireLeaseSuccessfully

	machineIds := []string{context.NodeID}
	setupGetMachineIds(etcdClient, machineIds)

	// set up a mock watcher that will return the key passed to this func
	watcherTriggered := make(chan bool)
	nextCalled := make(chan bool)
	membershipWatcher := &util.MockWatcher{
		MockNext: func(c ctx.Context) (*etcd.Response, error) {
			// let anyone listening to this channel that we've been called
			nextCalled <- true

			// wait for an external source to trigger the watch/next, then return the changed etcd key
			<-watcherTriggered
			return &etcd.Response{Action: action, Node: &etcd.Node{Key: key, Dir: true}}, nil
		},
	}
	etcdClient.MockWatcher = func(key string, opts *etcd.WatcherOptions) etcd.Watcher { return membershipWatcher }

	// try to elect a leader, we should win and aqcuire the lease
	leader := &servicesLeader{context: context}
	clusterMember := newClusterMember(context, mockLeaseManager, leader)
	leader.parent = clusterMember
	err := clusterMember.ElectLeader()
	assert.True(t, clusterMember.isLeader)
	assert.Nil(t, err)

	// wait for the Watcher.Next function to be called, he'll be waiting for an etcd key change then
	<-nextCalled

	// trigger the watcher Next function to return, allowing the cluster leader to handle the etcd change
	// (and hopefully filter it out)
	watcherTriggered <- true

	// wait for Watcher.Next to be called again, meaning the cluster leader is done processing/filtering the last change
	// only once that is done can we do any assertions
	<-nextCalled
	assert.True(t, clusterMember.isLeader)
}

// this test is to ensure that a cluster member listens to changes in etcd that trigger a hardware detection
func TestHardwareDetectionTrigger(t *testing.T) {
	etcdClient, context, mockLeaseManager, leader := createDefaultDependencies()
	key := path.Join(inventory.DiscoveredNodesKey, context.NodeID, "trigger-hardware-detection")
	nextCount := 0
	hardwareWatcher := &util.MockWatcher{
		MockNext: func(c ctx.Context) (*etcd.Response, error) {
			nextCount++
			if nextCount <= 2 {
				// the first/second time, return a "set" on the given trigger hardware detection key
				return &etcd.Response{Action: SetAction, Node: &etcd.Node{Key: key}}, nil
			}

			// every other time, return cancelled so the wait for hardware change notifications loop breaks
			return nil, ctx.Canceled
		},
	}
	etcdClient.MockWatcher = func(key string, opts *etcd.WatcherOptions) etcd.Watcher { return hardwareWatcher }

	// keep track of the etcd deletions so we can verify each hardware detection cleaned up the trigger key
	var deletedKeys []string
	etcdClient.MockDelete = func(c ctx.Context, deletedKey string, opts *etcd.DeleteOptions) (*etcd.Response, error) {
		deletedKeys = append(deletedKeys, deletedKey)
		return nil, nil
	}

	// create a cluster member
	clusterMember := newClusterMember(context, mockLeaseManager, leader)

	// wait for hardware changes, which should get triggered twice
	clusterMember.WaitForHardwareChangeNotifications()

	// verify that the hardware detection trigger key was deleted each time too
	assert.Equal(t, 2, len(deletedKeys))
	for _, deletedKey := range deletedKeys {
		assert.Equal(t, key, deletedKey)
	}
}

/* FIX: This test is not applicable until we are discovering something again
func TestHardwareDiscoveryLocking(t *testing.T) {
	_, context, mockLeaseManager, leader := createDefaultDependencies()
	clusterMember := newClusterMember(context, mockLeaseManager, leader)

	discoveryComplete := make(chan bool, 2)
	funcComplete := make(chan bool, 2)

	// this test will ensure hardware discovery is not reentrant (prevents more than 1 hardware discovery at a time)
	// we will launch two hardware discoveries at the same time in their own goroutines, and ensure only one actually
	// makes the disocvery occur.  the other should bail out.

	execCount := 0
	executor := &util.MockExecutor{}
	context.Executor = executor
	executor.MockExecuteCommand = func(name string, command string, args ...string) error {
		// block until the loser discovery bails out
		<-discoveryComplete
		execCount++
		return nil
	}

	// define a function that attempts discovery and reports when complete
	discoveryFunc := func() {
		// attempt hardware discovery
		clusterMember.discoverHardware()

		// signal that discovery is done
		discoveryComplete <- true

		// signal that the entire goroutine is done
		funcComplete <- true
	}

	// launch both disoveries at the same time
	go discoveryFunc()
	go discoveryFunc()

	// wait till both goroutines are done
	<-funcComplete
	<-funcComplete

	assert.Equal(t, 1, execCount)
}*/

// ************************************************************************************************
// ************************************************************************************************
//
// helper functions
//
// ************************************************************************************************
// ************************************************************************************************
func createDefaultDependencies() (*util.MockEtcdClient, *Context, *mockLeaseManager, *MockLeader) {
	triggerRefreshInterval = time.Millisecond
	mockLeaseManager := &mockLeaseManager{}
	etcdClient := util.NewMockEtcdClient()
	machineId := "8e8f532fe96dcae6b1ce335822e5b03c"
	context := &Context{EtcdClient: etcdClient, NodeID: machineId}
	return etcdClient, context, mockLeaseManager, &MockLeader{}
}

// implementation for GetLease where another machine in the cluster is currently the leader
func getLeaseNotLeader(name string) (Lease, error) {
	existingLease := &mockLease{mockMachineID: func() string { return "not the same machine ID" }}
	return existingLease, nil
}

// implementation of the acquire lease function that succeds and passes back a mock lease indicating the caller
// has acquired leadership of the cluster
func acquireLeaseSuccessfully(name, machID string, ver int, period time.Duration) (Lease, error) {
	return &mockLease{mockMachineID: func() string { return machID }}, nil
}

func setupGetMachineIds(etcdClient *util.MockEtcdClient, machineIds []string) {
	etcdClient.CreateDirs(inventory.DiscoveredNodesKey, util.CreateSet(machineIds))
}

func setupAndRunAcquireLeaseScenario(t *testing.T) (*ClusterMember, *mockLeaseManager, string) {
	etcdClient, context, mockLeaseManager, leader := createDefaultDependencies()
	mockLeaseManager.mockAcquireLease = acquireLeaseSuccessfully
	machineIds := []string{context.NodeID}
	setupGetMachineIds(etcdClient, machineIds)

	// acquire leadership
	clusterMember := newClusterMember(context, mockLeaseManager, leader)
	err := clusterMember.ElectLeader()
	assert.Nil(t, err)
	assert.True(t, clusterMember.isLeader)

	return clusterMember, mockLeaseManager, context.NodeID
}

// ************************************************************************************************
//
// test leader
//
// ************************************************************************************************
type testServiceLeader struct {
	events    chan LeaderEvent
	nodeAdded func(nodeID string)
	refresh   func()
}

func newTestServiceLeader() *testServiceLeader {
	return &testServiceLeader{}
}

func (t *testServiceLeader) StartWatchEvents() {
	if t.events != nil {
		close(t.events)
	}
	t.events = make(chan LeaderEvent, 1)
	go t.handleOrchestratorEvents()
}

func (t *testServiceLeader) Events() chan LeaderEvent {
	return t.events
}

func (t *testServiceLeader) Close() error {
	close(t.events)

	t.events = nil
	return nil
}

func (t *testServiceLeader) handleOrchestratorEvents() {
	for e := range t.events {
		log.Printf("Handling test event %s. %+v", e.Name(), e)
		if _, ok := e.(*RefreshEvent); ok {
			if t.refresh != nil {
				t.refresh()
			}

		} else if nodeAdded, ok := e.(*AddNodeEvent); ok {
			if t.nodeAdded != nil {
				t.nodeAdded(nodeAdded.Nodes()[0])
			}

		} else if _, ok := e.(*StaleNodeEvent); ok {
			// TODO
		}
	}
}
