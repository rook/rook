package clusterd

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTriggerRefresh(t *testing.T) {

	_, context, mockLeaseManager, _ := createDefaultDependencies()
	leader := newServicesLeader(context)
	r := newClusterMember(context, mockLeaseManager, leader)
	leader.parent = r
	// Skip the orchestration if not the leader
	triggered := leader.refresher.TriggerRefresh()

	assert.False(t, triggered)

	r.isLeader = true

	// FIX: Use channels instead of sleeps
	triggerRefreshInterval = 250 * time.Millisecond

	// The orchestration is triggered, but multiple triggers will still result in a single orchestrator
	triggered = leader.refresher.TriggerRefresh()
	assert.True(t, triggered)
	triggered = leader.refresher.TriggerRefresh()
	assert.True(t, triggered)
	triggered = leader.refresher.TriggerRefresh()
	assert.True(t, triggered)
	<-time.After(100 * time.Millisecond)
	assert.Equal(t, int32(1), leader.refresher.triggerRefreshLock)

	<-time.After(200 * time.Millisecond)
	assert.Equal(t, int32(0), leader.refresher.triggerRefreshLock)

	triggerRefreshInterval = 0
	triggered = leader.refresher.triggerNodeAdded("abc")
	assert.True(t, triggered)
}
