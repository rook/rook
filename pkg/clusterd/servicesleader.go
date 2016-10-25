package clusterd

import (
	"log"
	"time"

	etcd "github.com/coreos/etcd/client"
	"github.com/coreos/etcd/store"

	ctx "golang.org/x/net/context"

	"github.com/rook/rook/pkg/clusterd/inventory"
	"github.com/rook/rook/pkg/util"
)

var (
	detectUnhealthyNodeInterval = 5 * time.Second
)

type servicesLeader struct {
	leaseName     string
	context       *Context
	isLeader      bool
	parent        *ClusterMember
	watcherCancel ctx.CancelFunc
	refresher     *ClusterRefresher
}

func newServicesLeader(context *Context) *servicesLeader {
	l := &servicesLeader{context: context, refresher: &ClusterRefresher{}, leaseName: LeaderElectionKey}
	l.refresher.leader = l
	return l
}

func (s *servicesLeader) OnLeadershipAcquired() error {
	s.onLeadershipAcquiredRefresh(true)
	return nil
}

func (s *servicesLeader) onLeadershipAcquiredRefresh(refresh bool) {
	s.isLeader = true

	// The leaders should start watching for events
	for _, service := range s.context.Services {
		service.Leader.StartWatchEvents()
	}

	// start watching for membership changes and handling any changes to it
	s.startWatchingClusterChanges()
	s.startWatchingUnhealthyNodes()

	if refresh {
		// Tell the services to refresh the cluster whenever leadership is acquired
		s.refresher.TriggerRefresh()
	}
}

func (s *servicesLeader) OnLeadershipLost() error {
	s.isLeader = false
	s.stopWatchingClusterChanges()

	// Close down each of the leaders watching for events
	for _, service := range s.context.Services {
		service.Leader.Close()
	}

	return nil
}

func (s *servicesLeader) GetLeaseName() string {
	return s.leaseName
}

func (s *servicesLeader) startWatchingUnhealthyNodes() {
	go func() {
		for {
			// look for unhealthy nodes at a regular interval
			<-time.After(detectUnhealthyNodeInterval)
			if !s.isLeader {
				// poor man's cancellation when leadership is lost
				break
			}

			err := s.discoverUnhealthyNodes()
			if err != nil {
				log.Printf("error while discovering unhealthy nodes: %+v", err)
			}
		}
	}()
}

func (s *servicesLeader) discoverUnhealthyNodes() error {
	// load the state of all the nodes in the cluster
	config, err := inventory.LoadDiscoveredNodes(s.context.EtcdClient)
	if err != nil {
		return err
	}

	// look for old heartbeats
	var unhealthyNodes []*UnhealthyNode
	for nodeID, node := range config.Nodes {
		age, unhealthy := IsNodeUnhealthy(node)
		if unhealthy {
			unhealthyNodes = append(unhealthyNodes, &UnhealthyNode{AgeSeconds: age, NodeID: nodeID})
		}
	}

	// if we found unhealthy nodes, raise an event
	if len(unhealthyNodes) > 0 {
		log.Printf("Found %d unhealthy nodes", len(unhealthyNodes))
		s.refresher.triggerNodeUnhealthy(unhealthyNodes)
	}

	return nil
}

func handleNodeAdded(response *etcd.Response, refresher *ClusterRefresher) {
	if response.Action == store.Create {
		newNodeID := util.GetLeafKeyPath(response.Node.Key)
		log.Printf("new node discovered: %s", newNodeID)

		// trigger an orchestration to configure services on the new machine
		refresher.triggerNodeAdded(newNodeID)
	}
}

func (s *servicesLeader) startWatchingClusterChanges() {
	// create an etcd watcher object and initialize a cancellable context for it
	context, cancelFunc := ctx.WithCancel(ctx.Background())
	s.watcherCancel = cancelFunc

	// watch for changes in the discovered nodes etcd key
	nodeRefresh := &RefreshKey{Path: inventory.NodesConfigKey, Triggered: handleNodeAdded}
	go s.watchClusterChange(context, nodeRefresh)

	// watch for changes requested by the service managers
	for _, mgr := range s.context.Services {
		for _, refresh := range mgr.Leader.RefreshKeys() {
			go s.watchClusterChange(context, refresh)
		}
	}
}

func (s *servicesLeader) watchClusterChange(context ctx.Context, refreshKey *RefreshKey) {
	watcher := s.context.EtcdClient.Watcher(refreshKey.Path, &etcd.WatcherOptions{Recursive: true})
	for {
		//log.Printf("watching cluster changes under %s", refreshKey.Path)
		resp, err := watcher.Next(context)
		if err != nil {
			if err == ctx.Canceled {
				log.Printf("%s change watching cancelled, bailing out...", refreshKey.Path)
				break
			} else {
				log.Printf(
					"%s change watcher Next returned error, sleeping %d sec before retry: %+v",
					refreshKey.Path,
					watchErrorRetrySeconds,
					err)
				<-time.After(time.Duration(watchErrorRetrySeconds) * time.Second)
				continue
			}
		}

		if resp != nil && resp.Node != nil {
			refreshKey.Triggered(resp, s.refresher)
		}
	}

}

func (s *servicesLeader) stopWatchingClusterChanges() {
	if s.watcherCancel != nil {
		log.Print("calling cancel function for cluster change watcher...")
		s.watcherCancel()
	}

	s.watcherCancel = nil
}
