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
