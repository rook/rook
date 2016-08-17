package castled

import (
	"fmt"
	"log"
	"os/exec"
	"path"

	"golang.org/x/net/context"

	etcd "github.com/coreos/etcd/client"
	"github.com/quantum/castle/pkg/cephd"
	"github.com/quantum/castle/pkg/kvstore"
	"github.com/quantum/clusterd/pkg/orchestrator"
)

// Interface implemented by a service that has been elected leader
type monLeader struct {
}

// Load the state of the service from etcd. Typically a service will populate the desired/discovered state and the applied state
// from etcd, then compute the difference and cache it.
// Returns whether the service has updates to be applied.
func (m *monLeader) LoadState(ctx *orchestrator.ClusterContext) (bool, error) {
	return true, nil
}

// Apply the desired state to the cluster. The context provides all the information needed to make changes to the service.
func (m *monLeader) ApplyState(ctx *orchestrator.ClusterContext) error {
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

	if !isClusterInfoSet(cluster) {
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

func startMonitors(cfg Config) (*clusterInfo, []*exec.Cmd, error) {

	cluster, err := createOrGetClusterInfo(cfg.EtcdClient)
	if err != nil {
		return nil, nil, err
	}

	if err := registerMonitors(cfg, cfg.EtcdClient); err != nil {
		return cluster, nil, fmt.Errorf("failed to register monitors: %+v", err)
	}

	// wait for monitor registration to complete for all expected initial monitors
	if err := waitForMonitorRegistration(cfg, cfg.EtcdClient); err != nil {
		return cluster, nil, fmt.Errorf("failed to wait for monitors to register: %+v", err)
	}

	// initialze the file systems for the monitors
	if err := makeMonitorFileSystems(cfg, cluster); err != nil {
		return cluster, nil, fmt.Errorf("failed to make monitor filesystems: %+v", err)
	}

	// run the monitors
	procs, err := runMonitors(cfg)
	if err != nil {
		return cluster, nil, fmt.Errorf("failed to run monitors: %+v", err)
	}

	log.Printf("successfully started monitors")

	// open an admin connection to the cluster
	user := "client.admin"
	adminConn, err := connectToCluster(cfg.ClusterName, user, getMonConfFilePath(cfg.MonNames[0]))
	if err != nil {
		return cluster, procs, err
	}
	defer adminConn.Shutdown()

	// wait for monitors to establish quorum
	if err := waitForMonitorQuorum(adminConn, cfg); err != nil {
		return cluster, procs, fmt.Errorf("failed to wait for monitors to establish quorum: %+v", err)
	}

	log.Printf("monitors formed quorum")
	return cluster, procs, nil
}

// attempt to load any previously created and saved cluster info
func loadClusterInfo(etcdClient etcd.KeysAPI) (*clusterInfo, error) {
	resp, err := etcdClient.Get(context.Background(), path.Join(cephKey, "fsid"), nil)
	if err != nil {
		return handleLoadClusterInfoErr(err)
	}
	fsid := resp.Node.Value

	secretsKey := path.Join(cephKey, "_secrets")

	resp, err = etcdClient.Get(context.Background(), path.Join(secretsKey, "monitor"), nil)
	if err != nil {
		return handleLoadClusterInfoErr(err)
	}
	monSecret := resp.Node.Value

	resp, err = etcdClient.Get(context.Background(), path.Join(secretsKey, "admin"), nil)
	if err != nil {
		return handleLoadClusterInfoErr(err)
	}
	adminSecret := resp.Node.Value

	return &clusterInfo{
		FSID:          fsid,
		MonitorSecret: monSecret,
		AdminSecret:   adminSecret,
	}, nil
}

func handleLoadClusterInfoErr(err error) (*clusterInfo, error) {
	if kvstore.IsEtcdKeyNotFound(err) {
		return &clusterInfo{}, nil
	}

	return nil, err
}

func isClusterInfoSet(c *clusterInfo) bool {
	return c.FSID != "" && c.MonitorSecret != "" && c.AdminSecret != ""
}

// create new cluster info (FSID, shared keys)
func createClusterInfo() (*clusterInfo, error) {
	fsid, err := cephd.NewFsid()
	if err != nil {
		return handleLoadClusterInfoErr(err)
	}

	monSecret, err := cephd.NewSecretKey()
	if err != nil {
		return handleLoadClusterInfoErr(err)
	}

	adminSecret, err := cephd.NewSecretKey()
	if err != nil {
		return handleLoadClusterInfoErr(err)
	}

	return &clusterInfo{
		FSID:          fsid,
		MonitorSecret: monSecret,
		AdminSecret:   adminSecret,
	}, nil
}

// save the given cluster info to the key value store
func saveClusterInfo(c *clusterInfo, etcdClient etcd.KeysAPI) error {
	_, err := etcdClient.Set(context.Background(), path.Join(cephKey, "fsid"), c.FSID, nil)
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
