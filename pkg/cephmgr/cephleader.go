package cephmgr

import (
	"fmt"
	"log"
	"path"
	"strings"

	ctx "golang.org/x/net/context"

	etcd "github.com/coreos/etcd/client"
	"github.com/coreos/etcd/store"
	"github.com/quantum/castle/pkg/cephmgr/client"
	"github.com/quantum/castle/pkg/clusterd"
	"github.com/quantum/castle/pkg/clusterd/inventory"
)

// Interface implemented by a service that has been elected leader
type cephLeader struct {
	monLeader *monLeader
	cluster   *ClusterInfo
	events    chan clusterd.LeaderEvent
	factory   client.ConnectionFactory
}

func newCephLeader(factory client.ConnectionFactory) *cephLeader {
	return &cephLeader{factory: factory, monLeader: newMonLeader()}
}

func (c *cephLeader) RefreshKeys() []*clusterd.RefreshKey {
	// when devices are added or removed we will want to trigger an orchestration
	deviceChange := &clusterd.RefreshKey{
		Path:      path.Join(cephKey, osdAgentName, desiredKey),
		Triggered: handleDeviceChanged,
	}
	return []*clusterd.RefreshKey{deviceChange}
}

func (c *cephLeader) StartWatchEvents() {
	if c.events != nil {
		close(c.events)
	}
	c.events = make(chan clusterd.LeaderEvent, 10)
	go c.handleOrchestratorEvents()
}

func (c *cephLeader) Events() chan clusterd.LeaderEvent {
	return c.events
}

func (c *cephLeader) Close() error {
	close(c.events)
	c.events = nil
	return nil
}

func (c *cephLeader) handleOrchestratorEvents() {
	// Listen for events from the orchestrator indicating that a refresh is needed or nodes have been added
	for e := range c.events {
		log.Printf("ceph leader received event %s", e.Name())

		var osdsToRefresh []string
		refreshMon := false
		if r, ok := e.(*clusterd.RefreshEvent); ok {
			if r.NodeID() != "" {
				// refresh a single node, which is currently only for adding and removing devices
				osdsToRefresh = []string{r.NodeID()}
			} else {
				// refresh the whole cluster
				refreshMon = true
				osdsToRefresh = getSlice(e.Context().Inventory.Nodes)
			}

		} else if nodeAdded, ok := e.(*clusterd.AddNodeEvent); ok {
			osdsToRefresh = []string{nodeAdded.Node()}

		} else if unhealthyEvent, ok := e.(*clusterd.UnhealthyNodeEvent); ok {
			var err error
			refreshMon, err = monsOnUnhealthyNode(e.Context(), unhealthyEvent.Nodes())
			if err != nil {
				log.Printf("failed to handle unhealthy nodes. %v", err)
			}

		} else {
			// if we don't recognize the event we will skip the refresh
		}

		if refreshMon {
			// Perform a full refresh of the cluster to ensure the monitors are running with quorum
			err := c.configureCephMons(e.Context())
			if err != nil {
				log.Printf("FAILED TO CONFIGURE CEPH MONS. %v", err)
				continue
			}
		}

		if osdsToRefresh != nil {
			// Configure the OSDs
			err := configureOSDs(e.Context(), osdsToRefresh)
			if err != nil {
				log.Printf("FAILED TO CONFIGURE CEPH OSDs. %v", err)
				continue
			}
		}

		log.Printf("ceph leader completed event %s", e.Name())
	}
}

// Apply the desired state to the cluster. The context provides all the information needed to make changes to the service.
func (c *cephLeader) configureCephMons(context *clusterd.Context) error {

	// Create or get the basic cluster info
	var err error
	c.cluster, err = createOrGetClusterInfo(c.factory, context.EtcdClient)
	if err != nil {
		return err
	}

	// Select the monitors, instruct them to start, and wait for quorum
	return c.monLeader.configureMonitors(c.factory, context, c.cluster)
}

func createOrGetClusterInfo(factory client.ConnectionFactory, etcdClient etcd.KeysAPI) (*ClusterInfo, error) {
	// load any existing cluster info that may have previously been created
	cluster, err := LoadClusterInfo(etcdClient)
	if err != nil {
		return nil, fmt.Errorf("failed to load cluster info: %+v", err)
	}

	if cluster == nil {
		// the cluster info is not yet set, go ahead and set it now
		cluster, err = createClusterInfo(factory)
		if err != nil {
			return nil, fmt.Errorf("failed to create cluster info: %+v", err)
		}

		log.Printf("Created new cluster info: %+v", cluster)
		err = saveClusterInfo(cluster, etcdClient)
		if err != nil {
			return nil, fmt.Errorf("failed to save new cluster info: %+v", err)
		}
	} else {
		// the cluster has already been created
		log.Printf("Cluster already exists: %+v", cluster)
	}

	return cluster, nil
}

// create new cluster info (FSID, shared keys)
func createClusterInfo(factory client.ConnectionFactory) (*ClusterInfo, error) {
	fsid, err := factory.NewFsid()
	if err != nil {
		return nil, err
	}

	monSecret, err := factory.NewSecretKey()
	if err != nil {
		return nil, err
	}

	adminSecret, err := factory.NewSecretKey()
	if err != nil {
		return nil, err
	}

	return &ClusterInfo{
		FSID:          fsid,
		MonitorSecret: monSecret,
		AdminSecret:   adminSecret,
		Name:          "castlecluster",
	}, nil
}

// save the given cluster info to the key value store
func saveClusterInfo(c *ClusterInfo, etcdClient etcd.KeysAPI) error {
	_, err := etcdClient.Set(ctx.Background(), path.Join(cephKey, "fsid"), c.FSID, nil)
	if err != nil {
		return err
	}

	_, err = etcdClient.Set(ctx.Background(), path.Join(cephKey, "name"), c.Name, nil)
	if err != nil {
		return err
	}

	secretsKey := path.Join(cephKey, "_secrets")

	_, err = etcdClient.Set(ctx.Background(), path.Join(secretsKey, "monitor"), c.MonitorSecret, nil)
	if err != nil {
		return err
	}

	_, err = etcdClient.Set(ctx.Background(), path.Join(secretsKey, "admin"), c.AdminSecret, nil)
	if err != nil {
		return err
	}

	return nil
}

func getSlice(nodeMap map[string]*inventory.NodeConfig) []string {

	// Convert the node IDs to a simple slice
	nodes := make([]string, len(nodeMap))
	i := 0
	for node := range nodeMap {
		nodes[i] = node
		i++
	}

	return nodes
}

func handleDeviceChanged(response *etcd.Response, refresher *clusterd.ClusterRefresher) {
	if response.Action == store.Create || response.Action == store.Delete {
		nodeID, err := extractNodeIDFromDesiredDevice(response.Node.Key)
		if err != nil {
			log.Printf("ignored device changed event. %v", err)
			return
		}

		log.Printf("device changed: %s", nodeID)

		// trigger an orchestration to add or remove the device
		refresher.TriggerNodeRefresh(nodeID)
	}
}

// Get the node ID from the etcd key to a desired device
// For example: /castle/services/ceph/osd/desired/9b69e58300f9/device/sdb
func extractNodeIDFromDesiredDevice(path string) (string, error) {
	parts := strings.Split(path, "/")
	const nodeIDOffset = 6
	if len(parts) < nodeIDOffset+1 {
		return "", fmt.Errorf("cannot get node ID from %s", path)
	}

	return parts[nodeIDOffset], nil
}
