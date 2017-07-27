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
	"sync"
	"time"

	etcd "github.com/coreos/etcd/client"

	"github.com/rook/rook/pkg/clusterd/inventory"
	"github.com/rook/rook/pkg/util"
)

const (
	refreshDelaySeconds    = 5
	periodicRefreshMinutes = 10
)

var (
	refreshDelayInterval    = time.Second * refreshDelaySeconds
	periodicRefreshInterval = time.Minute * periodicRefreshMinutes
)

// RefreshEvent type
type RefreshEvent struct {
	Context        *Context
	NodesAdded     *util.Set
	NodesRemoved   *util.Set
	NodesChanged   *util.Set
	NodesUnhealthy map[string]*UnhealthyNode
}

// UnhealthyNode type
type UnhealthyNode struct {
	ID         string
	AgeSeconds int
}

// RefreshKey type
type RefreshKey struct {
	Path      string
	Triggered func(response *etcd.Response, refresher *ClusterRefresher)
}

// ClusterRefresher type
type ClusterRefresher struct {
	nextEvent    *RefreshEvent
	leader       *servicesLeader
	refreshMutex sync.RWMutex
	changes      bool
	closed       chan (bool)
	triggered    chan (bool)
}

// Create a new cluster refresher
func NewClusterRefresher() *ClusterRefresher {
	return &ClusterRefresher{
		nextEvent: NewRefreshEvent(),
		closed:    make(chan bool),
		triggered: make(chan bool),
	}
}

// Create a new refresh event
func NewRefreshEvent() *RefreshEvent {
	return &RefreshEvent{
		NodesAdded:     util.NewSet(),
		NodesRemoved:   util.NewSet(),
		NodesChanged:   util.NewSet(),
		NodesUnhealthy: make(map[string]*UnhealthyNode),
	}
}

// Start consuming refresh events
func (c *ClusterRefresher) Start() {
	go c.eventLoop()

	// wait for the event loop to start
	<-time.After(time.Millisecond)
}

// Stop consuming refresh events
func (c *ClusterRefresher) Stop() {
	c.closed <- true
}

// Trigger a general refresh of the cluster
func (c *ClusterRefresher) TriggerRefresh() bool {
	c.refreshMutex.Lock()
	c.changes = true
	c.refreshMutex.Unlock()

	return c.triggerRefresh()
}

// Trigger an event for a device being added or removed to a node
func (c *ClusterRefresher) TriggerDevicesChanged(nodeID string) bool {
	c.refreshMutex.Lock()
	c.changes = true
	c.nextEvent.NodesChanged.Add(nodeID)
	c.refreshMutex.Unlock()

	return c.triggerRefresh()
}

// Trigger an event for a node being added to the cluster
func (c *ClusterRefresher) triggerNodeAdded(nodeID string) bool {
	c.refreshMutex.Lock()
	c.changes = true
	c.nextEvent.NodesAdded.Add(nodeID)
	c.refreshMutex.Unlock()

	return c.triggerRefresh()
}

// Trigger an event for detecting unhealthy nodes
func (c *ClusterRefresher) triggerNodeUnhealthy(nodes []*UnhealthyNode) bool {
	c.refreshMutex.Lock()
	c.changes = true
	for _, node := range nodes {
		c.nextEvent.NodesUnhealthy[node.ID] = node
	}
	c.refreshMutex.Unlock()

	return c.triggerRefresh()
}

// Produce a refresh event.
// The pattern is a non-blocking producer with a blocking consumer.
func (c *ClusterRefresher) triggerRefresh() bool {

	// Only start the orchestration if currently elected leader
	if !c.leader.parent.isLeader {
		return false
	}

	// Trigger the refresh, but only if it is not busy.
	// If the orchestrator is already busy with a refresh, the new refresh
	// will be handled in processEvent() immediately after the first orchestration completes.
	select {
	case c.triggered <- true:
		logger.Debugf("The refresh was idle when triggered")
		return true
	default:
		logger.Debugf("The refresh was busy when triggered")
		return true
	}
}

// Consume the refresh events
func (c *ClusterRefresher) eventLoop() {
	for {
		select {
		case <-c.closed:
			logger.Debugf("refresh event loop closed")
			return
		case <-c.triggered:
			c.processEvent()
		case <-time.After(periodicRefreshInterval):
			// periodically check if any refresh is needed
			if c.refreshNeeded() {
				c.processEvent()
			}
		}
	}
}

// Process an individual refresh. If another refresh is triggered after this one has already started,
// process the new refresh immediately after the first one completes.
func (c *ClusterRefresher) processEvent() {
	// Wait a few seconds in case multiple machines are coming online at the same time.
	logger.Infof("triggering a refresh in %.1fs", refreshDelayInterval.Seconds())
	<-time.After(refreshDelayInterval)

	// Double check that we're still the leader
	if !c.leader.parent.isLeader {
		logger.Infof("not leader anymore. skipping refresh.")
		return
	}

	// Process each of the refresh events until no more are raised during the current refresh
	for {
		c.refreshMutex.Lock()
		c.changes = false
		event := c.nextEvent
		c.nextEvent = NewRefreshEvent()
		c.refreshMutex.Unlock()

		// Update the node inventory for the event
		var err error
		event.Context = copyContext(c.leader.context)
		event.Context.Inventory, err = inventory.LoadDiscoveredNodes(event.Context.EtcdClient)
		if err != nil {
			logger.Errorf("failed to load node info. err=%v", err)
			return
		}

		// Sequentially update the services. We may consider running them in parallel in the future.
		for _, s := range event.Context.Services {
			s.Leader.HandleRefresh(event)
		}

		// stop refreshing if no more refresh events were raised during this refresh
		if !c.refreshNeeded() {
			logger.Infof("Done with the orchestration. Waiting for the next event signal.")
			break
		}

		// There is a small window where a refresh event could be raised and we will miss processing
		// the refresh since we are not listening on the triggered channel yet. This will be caught
		// by the periodic check for refreshNeeded() in the consumer select statement.
		logger.Infof("triggering events that were queued during an active orchestration")
	}
}

// check if a refresh is needed
func (c *ClusterRefresher) refreshNeeded() bool {
	c.refreshMutex.Lock()
	defer c.refreshMutex.Unlock()
	return c.changes
}

func copyContext(c *Context) *Context {
	return &Context{
		DirectContext: DirectContext{
			Services:   c.Services,
			NodeID:     c.NodeID,
			EtcdClient: c.EtcdClient,
			Inventory:  c.Inventory,
		},
		Executor:           c.Executor,
		ProcMan:            c.ProcMan,
		ConfigDir:          c.ConfigDir,
		LogLevel:           c.LogLevel,
		ConfigFileOverride: c.ConfigFileOverride,
	}
}
