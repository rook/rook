package clusterd

import (
	"log"
	"sync/atomic"
	"time"

	etcd "github.com/coreos/etcd/client"

	"github.com/rook/rook/pkg/clusterd/inventory"
)

const (
	triggerRefreshDelaySeconds = 5
)

var (
	triggerRefreshInterval = triggerRefreshDelaySeconds * time.Second
)

type RefreshKey struct {
	Path      string
	Triggered func(response *etcd.Response, refresher *ClusterRefresher)
}

type ClusterRefresher struct {
	leader             *servicesLeader
	triggerRefreshLock int32
}

func (c *ClusterRefresher) TriggerRefresh() bool {
	return c.triggerEvent(&RefreshEvent{context: copyContext(c.leader.context)}, true)
}

func (c *ClusterRefresher) TriggerNodeRefresh(nodeID string) bool {
	return c.triggerEvent(&RefreshEvent{context: copyContext(c.leader.context), nodeID: nodeID}, true)
}

func (c *ClusterRefresher) triggerNodeAdded(nodeID string) bool {
	return c.triggerEvent(&AddNodeEvent{context: copyContext(c.leader.context), nodeID: nodeID}, false)
}

func (c *ClusterRefresher) triggerNodeUnhealthy(nodes []*UnhealthyNode) bool {
	return c.triggerEvent(&UnhealthyNodeEvent{context: copyContext(c.leader.context), nodes: nodes}, false)
}

func (c *ClusterRefresher) triggerEvent(event LeaderEvent, delay bool) bool {
	// Only start the orchestration if the leader
	if !c.leader.parent.isLeader {
		return false
	}

	// Avoid blocking the calling thread. No need to prevent multiple threads from entering this
	// go routine since the orchestrator already prevents multiple orchestrations.
	go func() {
		// If the event is to be delayed, only allow the refresh to be triggered once. Other events
		// will need to be implicitly handled during the refresh.
		// For example, if a new node is added, the refresh should notice the new node and
		// a node added event will not be triggered separately.
		// If a new node is added outside the full refresh cycle, the node added event will be raised immediately.
		var triggerCount int32
		if delay {
			triggerCount = atomic.AddInt32(&c.triggerRefreshLock, 1)
			defer atomic.AddInt32(&c.triggerRefreshLock, -1)
			if triggerCount > 1 {
				log.Printf("refresh already triggered")
				return
			}

			// Wait a few seconds in case multiple machines are coming online at the same time.
			log.Printf("triggering a refresh in %.1fs", triggerRefreshInterval.Seconds())
			<-time.After(triggerRefreshInterval)
		} else {
			triggerCount = atomic.LoadInt32(&c.triggerRefreshLock)
			if triggerCount > 0 {
				log.Printf("refresh already triggered. skipping event %s. %+v", event.Name(), event)
				return
			}
		}

		// Double check that we're still the leader
		if !c.leader.parent.isLeader {
			log.Printf("not leader anymore. skipping event %s. %+v", event.Name(), event)
			return
		}

		// Update the node inventory for the event
		var err error
		context := event.Context()
		context.Inventory, err = inventory.LoadDiscoveredNodes(context.EtcdClient)
		if err != nil {
			log.Printf("failed to load node info. err=%v", err)
			return
		}

		// Push the event to each of the services
		for _, service := range c.leader.context.Services {
			serviceChannel := service.Leader.Events()

			// Push the event as long as the buffer is not full
			if len(serviceChannel) < cap(serviceChannel) {
				serviceChannel <- event
			} else {
				log.Printf("dropping event %s for service %s due to full channel", event.Name(), service.Name)
			}
		}
	}()

	return true
}
