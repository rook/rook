package cephmgr

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"path"
	"strconv"
	"strings"
	"time"

	ctx "golang.org/x/net/context"

	etcd "github.com/coreos/etcd/client"
	"github.com/quantum/castle/pkg/cephmgr/client"
	"github.com/quantum/castle/pkg/clusterd"
	"github.com/quantum/castle/pkg/clusterd/inventory"
	"github.com/quantum/castle/pkg/util"
)

const (
	monitorKey                     = "monitor"
	unhealthyMonHeatbeatAgeSeconds = 10
)

type monLeader struct {
	waitForQuorum func(factory client.ConnectionFactory, context *clusterd.Context, cluster *ClusterInfo) error
}

func newMonLeader() *monLeader {
	return &monLeader{waitForQuorum: waitForQuorum}
}

// Create the ceph monitors
// Must be idempotent
func (m *monLeader) configureMonitors(factory client.ConnectionFactory, context *clusterd.Context, cluster *ClusterInfo) error {
	log.Printf("Creating monitors with %d nodes available", len(context.Inventory.Nodes))

	// choose the nodes where the monitors will run
	var err error
	var monitorsToRemove map[string]*CephMonitorConfig
	cluster.Monitors, monitorsToRemove, err = chooseMonitorNodes(context)
	if err != nil {
		log.Printf("failed to choose monitors. err=%s", err.Error())
		return err
	}

	// trigger the monitors to start on each node
	err = clusterd.TriggerAgentsAndWaitForCompletion(context.EtcdClient, monIDs(cluster.Monitors), monitorAgentName, len(cluster.Monitors))
	if err != nil {
		return err
	}

	// wait for quorum
	err = m.waitForQuorum(factory, context, cluster)
	if err != nil {
		return err
	}

	if len(monitorsToRemove) == 0 {
		// no monitors to remove
		return nil
	}

	// notify the quorum to remove the bad members
	err = removeMonitorsFromQuorum(factory, context, cluster, monitorsToRemove)
	if err != nil {
		log.Printf("failed to remove monitors from quorum. %v", err)
	}

	return nil
}

func removeMonitorsFromQuorum(factory client.ConnectionFactory, context *clusterd.Context, cluster *ClusterInfo, monitors map[string]*CephMonitorConfig) error {
	// trigger the monitors to remove, but don't wait for a response very long since they are likely down
	waitSeconds := 10
	err := clusterd.TriggerAgentsAndWait(context.EtcdClient, monIDs(monitors), monitorAgentName, 0, waitSeconds)
	if err != nil {
		return fmt.Errorf("failed to trigger removal of unhealthy monitors. %v", err)
	}

	// open an admin connection to the cluster
	conn, err := ConnectToClusterAsAdmin(factory, cluster)
	if err != nil {
		return err
	}
	defer conn.Shutdown()

	log.Printf("removing %d monitors from quorum", len(monitors))
	for monID, mon := range monitors {
		log.Printf("removing monitor %s (%v)", monID, mon)

		command, err := json.Marshal(map[string]interface{}{
			"prefix": "mon remove",
			"format": "json",
			"name":   mon.Name,
		})
		if err != nil {
			return fmt.Errorf("FAILED to remove monitor %s (%+v). %v", monID, mon, err)
		}

		_, _, err = conn.MonCommand(command)
		if err != nil {
			return fmt.Errorf("mon remove failed: %+v", err)
		}

		log.Printf("removed monitor %s from node %s", mon.Name, monID)
	}

	return nil
}

// extract the nodeIDs from the mon map
func monIDs(mons map[string]*CephMonitorConfig) []string {
	nodes := []string{}
	for mon := range mons {
		nodes = append(nodes, mon)
	}
	return nodes
}

func monsOnUnhealthyNode(context *clusterd.Context, nodes []*clusterd.UnhealthyNode) (bool, error) {
	cluster, err := LoadClusterInfo(context.EtcdClient)
	if err != nil {
		return false, fmt.Errorf("failed to load cluster info: %+v", err)
	}

	for _, node := range nodes {
		if isMonitor(cluster, node.NodeID) {
			return true, nil
		}
	}

	return false, nil
}

func isMonitor(cluster *ClusterInfo, nodeID string) bool {
	for mon := range cluster.Monitors {
		if mon == nodeID {
			return true
		}
	}

	return false
}

func GetDesiredMonitors(etcdClient etcd.KeysAPI) (map[string]*CephMonitorConfig, error) {
	// query the desired monitors from etcd
	monitors := make(map[string]*CephMonitorConfig)
	monKey := path.Join(cephKey, monitorKey, desiredKey)
	previousMonitors, err := etcdClient.Get(ctx.Background(), monKey, &etcd.GetOptions{Recursive: true})
	if err != nil {
		if util.IsEtcdKeyNotFound(err) {
			return monitors, nil
		}
		return nil, err
	}
	if previousMonitors == nil || previousMonitors.Node == nil {
		return monitors, nil
	}

	// parse the monitor info from etcd
	for _, node := range previousMonitors.Node.Nodes {
		nodeID := util.GetLeafKeyPath(node.Key)
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

func chooseMonitorNodes(context *clusterd.Context) (map[string]*CephMonitorConfig, map[string]*CephMonitorConfig, error) {
	nodeCount := len(context.Inventory.Nodes)

	// calculate how many monitors are desired
	desiredMonitors := calculateMonitorCount(nodeCount)

	// get the monitors that have already been chosen
	monitors, err := GetDesiredMonitors(context.EtcdClient)
	if err != nil {
		return nil, nil, err
	}

	// get the unhealthy monitors
	monitorsToRemove := getUnhealthyMonitors(context, monitors)

	newMons := desiredMonitors + len(monitorsToRemove) - len(monitors)
	log.Printf("Monitor state. current=%d, desired=%d, unhealthy=%d, toAdd=%d", len(monitors), desiredMonitors, len(monitorsToRemove), newMons)
	if newMons <= 0 {
		log.Printf("No need for new monitors")
		return monitors, monitorsToRemove, nil
	}

	// Select nodes and assign them a monitor ID
	nextMonID, err := getMaxMonitorID(monitors)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get max monitor id. %v", err)
	}

	// increment the id because we were actually given the max known id above and we need the next desired id
	nextMonID++

	// iterate through the monitors to find the new candidates
	var settings = make(map[string]string)
	addedMons := 0
	for nodeID := range context.Inventory.Nodes {
		// skip the node if already in the list of monitors
		if mon, ok := monitors[nodeID]; ok {
			log.Printf("skipping node %s that is %s", nodeID, mon.Name)
			continue
		}

		node, ok := context.Inventory.Nodes[nodeID]
		if !ok || node.IPAddress == "" {
			log.Printf("failed to discover desired ip address for node %s. %v", nodeID, err)
			return nil, nil, err
		}
		if !isNodeHealthyForMon(node) {
			log.Printf("skipping selection of unhealthy node %s as a monitor (age=%s)", nodeID, node.HeartbeatAge)
			continue
		}

		// Store the monitor id and connection info
		port := "6790"
		monitorID := fmt.Sprintf("mon%d", nextMonID)
		settings[path.Join(nodeID, "id")] = monitorID
		settings[path.Join(nodeID, "ipaddress")] = node.IPAddress
		settings[path.Join(nodeID, "port")] = port

		monitor := &CephMonitorConfig{Name: monitorID, Endpoint: fmt.Sprintf("%s:%s", node.IPAddress, port)}
		monitors[nodeID] = monitor

		nextMonID++
		addedMons++

		// break if we have enough monitors now
		if addedMons == newMons {
			break
		}
	}

	if addedMons != newMons {
		return monitors, monitorsToRemove, fmt.Errorf("only added %d/%d expected new monitors. aborting monitor deployment.", addedMons, newMons)
	}

	// FIX: Use an etcd3 transaction to add the monitor keys and remove unhealhty monitors

	// store the properties for the new monitors
	monKey := path.Join(cephKey, monitorKey, desiredKey)
	err = util.StoreEtcdProperties(context.EtcdClient, monKey, settings)
	if err != nil {
		log.Printf("failed to save monitor ids. err=%v", err)
		return nil, nil, err
	}

	// remove the unhealthy monitors from the desired state
	for monID, monitor := range monitorsToRemove {
		log.Printf("removing monitor %s (%+v) from desired state", monID, monitor)
		key := path.Join(monKey, monID)
		_, err = context.EtcdClient.Delete(ctx.Background(), key, &etcd.DeleteOptions{Dir: true, Recursive: true})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to remove monitor %s from desired state. %v", monID, err)
		}
		delete(monitors, monID)
	}

	return monitors, monitorsToRemove, nil
}

func isNodeHealthyForMon(node *inventory.NodeConfig) bool {
	return int(node.HeartbeatAge.Seconds()) < unhealthyMonHeatbeatAgeSeconds
}

func getUnhealthyMonitors(context *clusterd.Context, monitors map[string]*CephMonitorConfig) map[string]*CephMonitorConfig {
	unhealthyMons := make(map[string]*CephMonitorConfig)
	for monID, mon := range monitors {
		node, ok := context.Inventory.Nodes[monID]

		// if the monitor is not found in the inventory or else it has an unhealthy heartbeat, add it to the list
		if !ok || !isNodeHealthyForMon(node) {
			unhealthyMons[monID] = mon
		}
	}

	return unhealthyMons
}

// get the highest monitor ID from the list of montors
func getMaxMonitorID(monitors map[string]*CephMonitorConfig) (int, error) {
	maxMonitorID := -1
	for _, mon := range monitors {
		// monitors are expected to have a name of "mon" with an integer suffix
		if len(mon.Name) < 4 || mon.Name[0:3] != "mon" {
			return 0, fmt.Errorf("invalid monitor id %s", mon.Name)
		}

		substr := mon.Name[3:]
		id, err := strconv.ParseInt(substr, 10, 32)
		if err != nil {
			return 0, fmt.Errorf("bad monitor id %s. %v", mon.Name, err)
		}

		if int(id) > maxMonitorID {
			maxMonitorID = int(id)
		}
	}

	return maxMonitorID, nil
}

// Calculate the number of monitors that should be deployed
func calculateMonitorCount(nodeCount int) int {
	switch {
	case nodeCount > 100:
		return 7
	case nodeCount > 20:
		return 5
	case nodeCount > 2:
		return 3
	case nodeCount > 0:
		return 1
	default:
		return 0
	}
}

func waitForQuorum(factory client.ConnectionFactory, context *clusterd.Context, cluster *ClusterInfo) error {

	// open an admin connection to the cluster
	adminConn, err := ConnectToClusterAsAdmin(factory, cluster)
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
		monStatusResp, err := GetMonStatus(adminConn)
		if err != nil {
			log.Printf("failed to get mon_status, err: %+v", err)
			continue
		}

		// check if each of the initial monitors is in quorum
		allInQuorum := true
		for _, im := range cluster.Monitors {
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
