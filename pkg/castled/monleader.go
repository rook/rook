package castled

import (
	"fmt"
	"log"
	"path"

	"golang.org/x/net/context"

	etcd "github.com/coreos/etcd/client"
	"github.com/quantum/castle/pkg/cephd"
	"github.com/quantum/clusterd/pkg/orchestrator"
)

// Interface implemented by a service that has been elected leader
type monLeader struct {
	cluster     clusterInfo
	privateIPv4 string
	devices     []string
	forceFormat bool
	etcdClient  etcd.KeysAPI
	monNames    []string
}

// Load the state of the service from etcd. Typically a service will populate the desired/discovered state and the applied state
// from etcd, then compute the difference and cache it.
// Returns whether the service has updates to be applied.
func (m *monLeader) LoadState(context *orchestrator.ClusterContext) (bool, error) {
	return true, nil
}

// Apply the desired state to the cluster. The context provides all the information needed to make changes to the service.
func (m *monLeader) ApplyState(context *orchestrator.ClusterContext) error {

	// Create or get the basic cluster info
	var err error
	m.cluster, err = createOrGetClusterInfo(m.etcdClient)
	if err != nil {
		return nil, nil, err
	}

	// TODO: Get existing monitors
	// TODO: Select new monitors
	// TODO: Send instructions to monitors

	err = m.waitForQuorum()
	if err != nil {
		return nil, err
	}

	log.Printf("Applied state for the ceph monitor")
	return nil
}

// Get the changed state for the service
func (m *monLeader) GetChangedState() interface{} {
	return nil
}

func createOrGetClusterInfo(etcdClient etcd.KeysAPI) (*clusterInfo, error) {
	// load any existing cluster info that may have previously been created
	cluster, err := loadClusterInfo(etcdClient)
	if err != nil {
		return nil, fmt.Errorf("failed to load cluster info: %+v", err)
	}

	if cluster == nil {
		// the cluster info is not yet set, go ahead and set it now
		cluster, err = createClusterInfo()
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

func (m *monLeader) waitForQuorum() error {

	// open an admin connection to the cluster
	user := "client.admin"
	adminConn, err := connectToCluster(cluster.Name, user, getCephConnectionConfig(m.cluster))
	if err != nil {
		return err
	}
	defer adminConn.Shutdown()

	// wait for monitors to establish quorum
	if err := m.waitForMonitorQuorum(adminConn); err != nil {
		return fmt.Errorf("failed to wait for monitors to establish quorum: %+v", err)
	}

	log.Printf("monitors formed quorum")
	return nil
}

// attempt to load any previously created and saved cluster info
func loadClusterInfo(etcdClient etcd.KeysAPI) (*clusterInfo, error) {
	resp, err := etcdClient.Get(context.Background(), path.Join(cephKey, "fsid"), nil)
	if err != nil {
		return nil, err
	}
	fsid := resp.Node.Value

	resp, err = etcdClient.Get(context.Background(), path.Join(cephKey, "name"), nil)
	if err != nil {
		return nil, err
	}
	name := resp.Node.Value

	secretsKey := path.Join(cephKey, "_secrets")

	resp, err = etcdClient.Get(context.Background(), path.Join(secretsKey, "monitor"), nil)
	if err != nil {
		return nil, err
	}
	monSecret := resp.Node.Value

	resp, err = etcdClient.Get(context.Background(), path.Join(secretsKey, "admin"), nil)
	if err != nil {
		return nil, err
	}
	adminSecret := resp.Node.Value

	return &clusterInfo{
		FSID:          fsid,
		MonitorSecret: monSecret,
		AdminSecret:   adminSecret,
		Name:          name,
	}, nil
}

// create new cluster info (FSID, shared keys)
func createClusterInfo() (*clusterInfo, error) {
	fsid, err := cephd.NewFsid()
	if err != nil {
		return nil, err
	}

	monSecret, err := cephd.NewSecretKey()
	if err != nil {
		return nil, err
	}

	adminSecret, err := cephd.NewSecretKey()
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
	_, err := etcdClient.Set(context.Background(), path.Join(cephKey, "fsid"), c.FSID, nil)
	if err != nil {
		return err
	}

	_, err = etcdClient.Set(context.Background(), path.Join(cephKey, "name"), c.Name, nil)
	if err != nil {
		return err
	}

	secretsKey := path.Join(cephKey, "_secrets")

	_, err = etcdClient.Set(context.Background(), path.Join(secretsKey, "monitor"), c.MonitorSecret, nil)
	if err != nil {
		return err
	}

	_, err = etcdClient.Set(context.Background(), path.Join(secretsKey, "admin"), c.AdminSecret, nil)
	if err != nil {
		return err
	}

	return nil
}

// opens a connection to the cluster that can be used for management operations
func connectToCluster(clusterName, user, confFilePath string) (*cephd.Conn, error) {
	log.Printf("connecting to ceph cluster %s with user %s", clusterName, user)

	conn, err := cephd.NewConnWithClusterAndUser(clusterName, user)
	if err != nil {
		return nil, fmt.Errorf("failed to create rados connection for cluster %s and user %s: %+v", clusterName, user, err)
	}

	if err = conn.ReadConfigFile(confFilePath); err != nil {
		return nil, fmt.Errorf("failed to read config file for cluster %s: %+v", clusterName, err)
	}

	if err = conn.Connect(); err != nil {
		return nil, fmt.Errorf("failed to connect to cluster %s: %+v", clusterName, err)
	}

	return conn, nil
}
