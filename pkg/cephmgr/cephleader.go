package cephmgr

import (
	"fmt"
	"log"
	"path"
	"strings"

	ctx "golang.org/x/net/context"

	etcd "github.com/coreos/etcd/client"
	"github.com/coreos/etcd/store"
	"github.com/rook/rook/pkg/cephmgr/client"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/clusterd/inventory"
	"github.com/rook/rook/pkg/util"
)

// Interface implemented by a service that has been elected leader
type cephLeader struct {
	monLeader *monLeader
	cluster   *ClusterInfo
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

func getOSDsToRefresh(e *clusterd.RefreshEvent) *util.Set {
	osds := util.NewSet()
	osds.AddSet(e.NodesAdded)
	osds.AddSet(e.NodesChanged)
	osds.AddSet(e.NodesRemoved)

	// Nothing changed in the event, so refresh osds on all nodes
	if osds.Count() == 0 {
		for nodeID := range e.Context.Inventory.Nodes {
			osds.Add(nodeID)
		}
	}

	return osds
}

func getRefreshMons(e *clusterd.RefreshEvent) bool {
	return true
}

func (c *cephLeader) HandleRefresh(e *clusterd.RefreshEvent) {
	// Listen for events from the orchestrator indicating that a refresh is needed or nodes have been added
	log.Printf("ceph leader received refresh event")

	refreshMon := getRefreshMons(e)
	osdsToRefresh := getOSDsToRefresh(e)

	if refreshMon {
		// Perform a full refresh of the cluster to ensure the monitors are running with quorum
		err := c.configureCephMons(e.Context)
		if err != nil {
			log.Printf("FAILED TO CONFIGURE CEPH MONS. %v", err)
			return
		}
	}

	if osdsToRefresh.Count() > 0 {
		// Configure the OSDs
		err := configureOSDs(e.Context, osdsToRefresh.ToSlice())
		if err != nil {
			log.Printf("FAILED TO CONFIGURE CEPH OSDs. %v", err)
			return
		}
	}

	log.Printf("ceph leader completed refresh")
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
		Name:          "rookcluster",
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
		refresher.TriggerDevicesChanged(nodeID)
	}
}

// Get the node ID from the etcd key to a desired device
// For example: /rook/services/ceph/osd/desired/9b69e58300f9/device/sdb
func extractNodeIDFromDesiredDevice(path string) (string, error) {
	parts := strings.Split(path, "/")
	const nodeIDOffset = 6
	if len(parts) < nodeIDOffset+1 {
		return "", fmt.Errorf("cannot get node ID from %s", path)
	}

	return parts[nodeIDOffset], nil
}
