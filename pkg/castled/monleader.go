package castled

import (
	"errors"
	"fmt"
	"log"
	"path"
	"strconv"
	"strings"
	"time"

	ctx "golang.org/x/net/context"

	etcd "github.com/coreos/etcd/client"
	"github.com/quantum/castle/pkg/cephd"
	"github.com/quantum/clusterd/pkg/orchestrator"
	"github.com/quantum/clusterd/pkg/store"
)

// Interface implemented by a service that has been elected leader
type monLeader struct {
	cluster     *clusterInfo
	privateIPv4 string
	devices     []string
	forceFormat bool
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
	m.cluster, err = createOrGetClusterInfo(context.EtcdClient)
	if err != nil {
		return err
	}

	// Select the monitors, instruct them to start, and wait for quorum
	err = m.createMonitors(context)
	if err != nil {
		return err
	}

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

// attempt to load any previously created and saved cluster info
func loadClusterInfo(etcdClient etcd.KeysAPI) (*clusterInfo, error) {
	resp, err := etcdClient.Get(ctx.Background(), path.Join(cephKey, "fsid"), nil)
	if err != nil {
		if store.IsEtcdKeyNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	fsid := resp.Node.Value

	resp, err = etcdClient.Get(ctx.Background(), path.Join(cephKey, "name"), nil)
	if err != nil {
		return nil, err
	}
	name := resp.Node.Value

	secretsKey := path.Join(cephKey, "_secrets")

	resp, err = etcdClient.Get(ctx.Background(), path.Join(secretsKey, "monitor"), nil)
	if err != nil {
		return nil, err
	}
	monSecret := resp.Node.Value

	resp, err = etcdClient.Get(ctx.Background(), path.Join(secretsKey, "admin"), nil)
	if err != nil {
		return nil, err
	}
	adminSecret := resp.Node.Value

	cluster := &clusterInfo{
		FSID:          fsid,
		MonitorSecret: monSecret,
		AdminSecret:   adminSecret,
		Name:          name,
	}

	// Get the monitors that have been applied in a previous orchestration
	cluster.Monitors, err = getChosenMonitors(etcdClient)

	return cluster, nil
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

// Create the ceph monitors
// Must be idempotent
func (m *monLeader) createMonitors(context *orchestrator.ClusterContext) error {

	// Choose the nodes where the monitors will run
	monitors, err := m.chooseMonitorNodes(context)
	if err != nil {
		log.Printf("failed to choose monitors. err=%s", err.Error())
		return err
	}

	// Trigger the monitors to start on each node
	monNodes := []string{}
	for mon := range monitors {
		monNodes = append(monNodes, mon)
	}
	err = orchestrator.TriggerAgentsAndWaitForCompletion(context.EtcdClient, monNodes, monitorKey, len(monNodes))
	if err != nil {
		return err
	}

	// Wait for quorum
	err = m.waitForQuorum()
	if err != nil {
		return err
	}

	return nil
}

func getChosenMonitors(etcdClient etcd.KeysAPI) (map[string]*CephMonitorConfig, error) {
	monitors := make(map[string]*CephMonitorConfig)
	monKey := path.Join(cephKey, "monitors")
	previousMonitors, err := etcdClient.Get(ctx.Background(), monKey, &etcd.GetOptions{Recursive: true})
	if err != nil {
		if store.IsEtcdKeyNotFound(err) {
			return monitors, nil
		}
		return nil, err
	}
	if previousMonitors == nil || previousMonitors.Node == nil {
		return monitors, nil
	}

	// Load the previously selected monitors
	log.Printf("Loading previously selected monitors")
	for _, node := range previousMonitors.Node.Nodes {
		nodeID := store.GetLeafKeyPath(node.Key)
		mon := &CephMonitorConfig{}
		ipaddress := ""
		port := ""
		for _, monSettings := range node.Nodes {
			if strings.HasSuffix(monSettings.Key, "/id") {
				mon.Name = monSettings.Value
			} else if strings.HasSuffix(monSettings.Key, "/ipaddress") {
				ipaddress = monSettings.Value
			} else if strings.HasSuffix(monSettings.Key, "/port") {
				port = monSettings.Value
			}
		}

		if mon.Name == "" || ipaddress == "" || port == "" {
			return nil, errors.New("missing monitor id or ip address or port")
		}

		mon.Endpoint = fmt.Sprintf("%s:%s", ipaddress, port)

		monitors[nodeID] = mon
	}

	return monitors, nil
}

func (m *monLeader) chooseMonitorNodes(context *orchestrator.ClusterContext) (map[string]*CephMonitorConfig, error) {
	monitors, err := getChosenMonitors(context.EtcdClient)
	if err != nil {
		return nil, err
	}

	if len(monitors) > 0 {
		// TODO: Support adding and removing monitors
		return monitors, nil
	}

	// Choose new monitor nodes
	nodeCount := len(context.Inventory.Nodes)
	if nodeCount == 0 {
		return nil, errors.New("cannot create cluster with 0 nodes")
	}

	monitorCount := calculateMonitorCount(nodeCount)
	log.Printf("Selecting %d new monitors from %d discovered nodes", monitorCount, nodeCount)

	// Select nodes and assign them a monitor ID
	monitorNum := 0
	var settings = make(map[string]string)
	for nodeID, node := range context.Inventory.Nodes {
		ipaddress, err := getDesiredNodeIPAddress(context, nodeID)
		if err != nil {
			log.Printf("failed to discover desired ip address for node %s. %v", nodeID, err)
			return nil, err
		}

		// Store the monitor id and connection info
		monitorID := strconv.FormatInt(int64(monitorNum), 10)
		port := "6790"
		settings[path.Join(nodeID, "id")] = monitorID
		settings[path.Join(nodeID, "ipaddress")] = ipaddress
		settings[path.Join(nodeID, "port")] = port

		monitor := &CephMonitorConfig{Name: monitorID, Endpoint: fmt.Sprintf("%s:%s", node.IPAddress, port)}
		monitors[nodeID] = monitor

		monitorNum++
	}

	monKey := path.Join(cephKey, "monitors")
	err = orchestrator.StoreEtcdProperties(context.EtcdClient, monKey, settings)
	if err != nil {
		log.Printf("failed to save monitor ids. err=%v", err)
		return nil, err
	}

	return monitors, nil
}

func getDesiredNodeIPAddress(context *orchestrator.ClusterContext, nodeID string) (string, error) {
	key := path.Join(orchestrator.DesiredNodesKey, nodeID, PrivateIPv4Value)
	resp, err := context.EtcdClient.Get(ctx.Background(), key, nil)
	if err != nil {
		return "", err
	}

	return resp.Node.Value, nil
}

// Calculate the number of monitors that should be deployed
func calculateMonitorCount(nodeCount int) int {
	if nodeCount > 100 {
		return 7
	} else if nodeCount > 20 {
		return 5
	} else if nodeCount > 2 {
		return 3
	} else {
		return 1
	}
}

func (m *monLeader) waitForQuorum() error {

	// open an admin connection to the clufster
	user := "client.admin"
	config, err := getCephConnectionConfig(m.cluster)
	if err != nil {
		return err
	}

	adminConn, err := connectToCluster(m.cluster.Name, user, config)
	if err != nil {
		return err
	}
	defer adminConn.Shutdown()

	// wait for monitors to establish quorum
	retryCount := 0
	retryMax := 20
	sleepTime := 5
	for {
		retryCount++
		if retryCount > retryMax {
			return fmt.Errorf("exceeded max retry count waiting for monitors to reach quorum")
		}

		if retryCount > 1 {
			// only sleep after the first time
			<-time.After(time.Duration(sleepTime) * time.Second)
		}

		// get the mon_status response that contains info about all monitors in the mon map and
		// their quorum status
		monStatusResp, err := getMonStatus(adminConn)
		if err != nil {
			log.Printf("failed to get mon_status, err: %+v", err)
			continue
		}

		// check if each of the initial monitors is in quorum
		allInQuorum := true
		for _, im := range m.cluster.Monitors {
			// first get the initial monitors corresponding mon map entry
			var monMapEntry *MonMapEntry
			for i := range monStatusResp.MonMap.Mons {
				if im.Name == monStatusResp.MonMap.Mons[i].Name {
					monMapEntry = &monStatusResp.MonMap.Mons[i]
					break
				}
			}

			if monMapEntry == nil {
				// found an initial monitor that is not in the mon map, bail out of this retry
				log.Printf("failed to find initial monitor %s in mon map", im.Name)
				allInQuorum = false
				break
			}

			// using the current initial monitor's mon map entry, check to see if it's in the quorum list
			// (a list of monitor rank values)
			inQuorumList := false
			for _, q := range monStatusResp.Quorum {
				if monMapEntry.Rank == q {
					inQuorumList = true
					break
				}
			}

			if !inQuorumList {
				// found an initial monitor that is not in quorum, bail out of this retry
				log.Printf("initial monitor %s is not in quorum list", im.Name)
				allInQuorum = false
				break
			}
		}

		if allInQuorum {
			log.Printf("all initial monitors are in quorum")
			break
		}
	}

	log.Printf("Ceph monitors formed quorum")
	return nil
}
