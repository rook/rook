package clusterd

import (
	"log"
	"sync/atomic"
	"time"

	etcd "github.com/coreos/etcd/client"

	ctx "golang.org/x/net/context"

	"github.com/quantum/castle/pkg/clusterd/inventory"
	"github.com/quantum/castle/pkg/util"
)

const (
	triggerRefreshDelaySeconds = 5
)

var (
	triggerRefreshInterval = time.Duration(triggerRefreshDelaySeconds) * time.Second
)

type servicesLeader struct {
	leaseName          string
	context            *Context
	triggerRefreshLock int32
	parent             *ClusterMember
	watcherCancel      ctx.CancelFunc
}

func (s *servicesLeader) OnLeadershipAcquired() error {
	return s.onLeadershipAcquiredRefresh(true)
}

func (s *servicesLeader) onLeadershipAcquiredRefresh(refresh bool) error {
	// The leaders should start watching for events
	for _, service := range s.context.Services {
		service.Leader.StartWatchEvents()
	}

	// start watching for membership changes and handling any changes to it
	s.startWatchingDiscoveredNodeChanges()

	var err error
	if refresh {
		// Tell the services to refresh the cluster whenever leadership is acquired
		_, err = s.triggerRefresh()
	}

	return err
}

func (s *servicesLeader) OnLeadershipLost() error {
	s.stopWatchingDiscoveredNodeChanges()

	// Close down each of the leaders watching for events
	for _, service := range s.context.Services {
		service.Leader.Close()
	}

	return nil
}

func (s *servicesLeader) GetLeaseName() string {
	return s.leaseName
}

func (s *servicesLeader) triggerRefresh() (bool, error) {
	return s.triggerEvent(&RefreshEvent{context: copyContext(s.context)}, true)
}

func (s *servicesLeader) triggerNodeAdded(node string) (bool, error) {
	return s.triggerEvent(&AddNodeEvent{context: copyContext(s.context), nodes: []string{node}}, false)
}

func (s *servicesLeader) triggerNodeStale(node string) (bool, error) {
	return s.triggerEvent(&StaleNodeEvent{context: copyContext(s.context), nodes: []string{node}}, false)
}

func (s *servicesLeader) triggerEvent(event LeaderEvent, delay bool) (bool, error) {
	// Only start the orchestration if the leader
	if !s.parent.isLeader {
		return false, nil
	}

	// Update the node inventory for the event
	var err error
	context := event.Context()
	context.Inventory, err = inventory.LoadDiscoveredNodes(context.EtcdClient)
	if err != nil {
		log.Printf("failed to load node info. err=%v", err)
		return false, err
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
			triggerCount = atomic.AddInt32(&s.triggerRefreshLock, 1)
			defer atomic.AddInt32(&s.triggerRefreshLock, -1)
			if triggerCount > 1 {
				log.Printf("refresh already triggered")
				return
			}

			// Wait a few seconds in case multiple machines are coming online at the same time.
			log.Printf("triggering a refresh in %.1fs", triggerRefreshInterval.Seconds())
			<-time.After(triggerRefreshInterval)
		} else {
			triggerCount = atomic.LoadInt32(&s.triggerRefreshLock)
			if triggerCount > 0 {
				log.Printf("refresh already triggered. skipping event %s. %+v", event.Name(), event)
				return
			}
		}

		// Double check that we're still the leader
		if !s.parent.isLeader {
			log.Printf("not leader anymore. skipping event %s. %+v", event.Name(), event)
			return
		}

		// Push the event to each of the services
		for _, service := range s.context.Services {
			serviceChannel := service.Leader.Events()

			// Push the event as long as the buffer is not full
			if len(serviceChannel) < cap(serviceChannel) {
				serviceChannel <- event
			} else {
				log.Printf("dropping event %s for service %s due to full channel", event.Name(), service.Name)
			}
		}
	}()

	return true, nil
}

func (s *servicesLeader) startWatchingDiscoveredNodeChanges() {
	// create an etcd watcher object and initialize a cancellable context for it
	discoveredNodeWatcher := s.context.EtcdClient.Watcher(inventory.DiscoveredNodesKey, &etcd.WatcherOptions{Recursive: true})
	context, cancelFunc := ctx.WithCancel(ctx.Background())
	s.watcherCancel = cancelFunc

	// goroutine to watch for changes in the discovered nodes etcd key
	go func() {
		for {
			resp, err := discoveredNodeWatcher.Next(context)
			if err != nil {
				if err == ctx.Canceled {
					log.Print("discovered nodes change watching cancelled, bailing out...")
					break
				} else {
					log.Printf(
						"discovered nodes change watcher Next returned error, sleeping %d sec before retry: %+v",
						WatchErrorRetrySeconds,
						err)
					<-time.After(time.Duration(WatchErrorRetrySeconds) * time.Second)
					continue
				}
			}

			if resp != nil && resp.Node != nil && resp.Action == CreateAction {
				newNodeID := util.GetLeafKeyPath(resp.Node.Key)
				log.Printf("new node discovered: %s", newNodeID)

				// trigger an orchestration to configure services on the new machine
				s.triggerNodeAdded(newNodeID)
			}
		}
	}()
}

func (s *servicesLeader) stopWatchingDiscoveredNodeChanges() {
	if s.watcherCancel != nil {
		log.Print("calling cancel function for discovered node change watcher...")
		s.watcherCancel()
	}

	s.watcherCancel = nil
}
