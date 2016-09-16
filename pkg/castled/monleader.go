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
	"github.com/quantum/castle/pkg/cephclient"
	"github.com/quantum/castle/pkg/clusterd"
	"github.com/quantum/castle/pkg/util"
)

const (
	monitorKey = "monitor"
)

// Create the ceph monitors
// Must be idempotent
func configureMonitors(factory cephclient.ConnectionFactory, context *clusterd.Context, cluster *clusterInfo) error {
	log.Printf("Creating monitors with %d nodes available", len(context.Inventory.Nodes))

	// Choose the nodes where the monitors will run
	var err error
	cluster.Monitors, err = chooseMonitorNodes(context)
	if err != nil {
		log.Printf("failed to choose monitors. err=%s", err.Error())
		return err
	}

	// Trigger the monitors to start on each node
	monNodes := []string{}
	for mon := range cluster.Monitors {
		monNodes = append(monNodes, mon)
	}
	err = clusterd.TriggerAgentsAndWaitForCompletion(context.EtcdClient, monNodes, monitorAgentName, len(monNodes))
	if err != nil {
		return err
	}

	// Wait for quorum
	err = waitForQuorum(factory, context, cluster)
	if err != nil {
		return err
	}

	return nil
}

func getChosenMonitors(etcdClient etcd.KeysAPI) (map[string]*CephMonitorConfig, error) {
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

	// Load the previously selected monitors
	log.Printf("Loading previously selected monitors")
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

func chooseMonitorNodes(context *clusterd.Context) (map[string]*CephMonitorConfig, error) {
	nodeCount := len(context.Inventory.Nodes)
	if nodeCount == 0 {
		return nil, errors.New("cannot create cluster with 0 nodes")
	}

	// calculate how many monitors are desired
	desiredMonitors := calculateMonitorCount(nodeCount)

	// get the monitors that have already been chosen
	monitors, err := getChosenMonitors(context.EtcdClient)
	if err != nil {
		return nil, err
	}

	newMons := desiredMonitors - len(monitors)
	if newMons <= 0 {
		log.Printf("Already have %d monitors on %d discovered nodes", len(monitors), nodeCount)
		return monitors, nil
	}

	log.Printf("Selecting %d new monitors (%d existing) from %d discovered nodes", newMons, len(monitors), nodeCount)

	// Select nodes and assign them a monitor ID
	nextMonID, err := getMaxMonitorID(monitors)
	if err != nil {
		return monitors, fmt.Errorf("cannot config monitors. %v", err)
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
			return nil, err
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

	monKey := path.Join(cephKey, monitorKey, desiredKey)
	err = util.StoreEtcdProperties(context.EtcdClient, monKey, settings)
	if err != nil {
		log.Printf("failed to save monitor ids. err=%v", err)
		return nil, err
	}

	return monitors, nil
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

func waitForQuorum(factory cephclient.ConnectionFactory, context *clusterd.Context, cluster *clusterInfo) error {

	// open an admin connection to the clufster
	adminConn, err := connectToClusterAsAdmin(factory, cluster)
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
