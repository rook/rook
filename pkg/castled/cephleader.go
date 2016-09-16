package castled

import (
	"fmt"
	"log"
	"path"

	ctx "golang.org/x/net/context"

	etcd "github.com/coreos/etcd/client"
	"github.com/quantum/castle/pkg/cephclient"
	"github.com/quantum/castle/pkg/clusterd"
)

// Interface implemented by a service that has been elected leader
type cephLeader struct {
	cluster *clusterInfo
	events  chan clusterd.LeaderEvent
	factory cephclient.ConnectionFactory
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
		if _, ok := e.(*clusterd.RefreshEvent); ok {
			// Perform a full refresh of the cluster to ensure the monitors and OSDs are running
			c.configureCephServices(e.Context())

		} else if nodeAdded, ok := e.(*clusterd.AddNodeEvent); ok {
			// When a node is added simply start OSDs on the node
			configureOSDs(e.Context(), nodeAdded.Nodes())

		} else if _, ok := e.(*clusterd.StaleNodeEvent); ok {
			// TODO: Move a monitor to another node and/or declare OSDs dead
		}
		log.Printf("ceph leader completed event %s", e.Name())
	}
}

// Apply the desired state to the cluster. The context provides all the information needed to make changes to the service.
func (c *cephLeader) configureCephServices(context *clusterd.Context) error {

	// Create or get the basic cluster info
	var err error
	c.cluster, err = createOrGetClusterInfo(c.factory, context.EtcdClient)
	if err != nil {
		return err
	}

	// Select the monitors, instruct them to start, and wait for quorum
	err = createMonitors(c.factory, context, c.cluster)
	if err != nil {
		return err
	}

	// Convert the node IDs to a simple slice
	nodes := make([]string, len(context.Inventory.Nodes))
	i := 0
	for node := range context.Inventory.Nodes {
		nodes[i] = node
		i++
	}

	// Configure the OSDs
	err = configureOSDs(context, nodes)
	if err != nil {
		return err
	}

	return nil
}

func createOrGetClusterInfo(factory cephclient.ConnectionFactory, etcdClient etcd.KeysAPI) (*clusterInfo, error) {
	// load any existing cluster info that may have previously been created
	cluster, err := loadClusterInfo(etcdClient)
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
func createClusterInfo(factory cephclient.ConnectionFactory) (*clusterInfo, error) {
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

	return &clusterInfo{
		FSID:          fsid,
		MonitorSecret: monSecret,
		AdminSecret:   adminSecret,
		Name:          "castlecluster",
	}, nil
}

// save the given cluster info to the key value store
func saveClusterInfo(c *clusterInfo, etcdClient etcd.KeysAPI) error {
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
