package clusterd

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTriggerRefresh(t *testing.T) {

	_, context, mockLeaseManager, _ := createDefaultDependencies()
	leader := newServicesLeader(context)
	leader.refresher.Start()
	defer leader.refresher.Stop()
	r := newClusterMember(context, mockLeaseManager, leader)
	leader.parent = r
	// Skip the orchestration if not the leader
	triggered := leader.refresher.TriggerRefresh()

	assert.False(t, triggered)

	r.isLeader = true

	// FIX: Use channels instead of sleeps
	refreshDelayInterval = 250 * time.Millisecond

	// The orchestration is triggered, but multiple triggers will still result in a single orchestrator
	triggered = leader.refresher.TriggerRefresh()
	assert.True(t, triggered)
	triggered = leader.refresher.TriggerRefresh()
	assert.True(t, triggered)
	triggered = leader.refresher.TriggerRefresh()
	assert.True(t, triggered)
	<-time.After(100 * time.Millisecond)
	assert.True(t, leader.refresher.changes)

	<-time.After(200 * time.Millisecond)
	assert.False(t, leader.refresher.changes)

	refreshDelayInterval = 0
	triggered = leader.refresher.triggerNodeAdded("abc")
	assert.True(t, triggered)
}
