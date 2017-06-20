/*
Copyright 2016 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package mon

import (
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"

	ctx "golang.org/x/net/context"

	etcd "github.com/coreos/etcd/client"
	"github.com/rook/rook/pkg/ceph/client"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/clusterd/inventory"
	"github.com/rook/rook/pkg/util"
)

const (
	monitorKey                   = "monitor"
	UnhealthyHeartbeatAgeSeconds = 10
)

type Leader struct {
	waitForQuorum func(context *clusterd.Context, cluster *ClusterInfo) error
}

func NewLeader() *Leader {
	return &Leader{waitForQuorum: WaitForQuorum}
}

// Apply the desired state to the cluster. The context provides all the information needed to make changes to the service.
// Create the ceph monitors
// Must be idempotent
func (m *Leader) Configure(context *clusterd.Context, adminSecret string) error {

	// Create or get the basic cluster info
	var err error
	cluster, err := createOrGetClusterInfo(context, adminSecret)
	if err != nil {
		return err
	}

	logger.Infof("Creating monitors with %d nodes available", len(context.Inventory.Nodes))

	// choose the nodes where the monitors will run
	var monitorsToRemove map[string]*CephMonitorConfig
	cluster.Monitors, monitorsToRemove, err = chooseMonitorNodes(context)
	if err != nil {
		logger.Errorf("failed to choose monitors. err=%s", err.Error())
		return err
	}

	// trigger the monitors to start on each node
	err = clusterd.TriggerAgentsAndWaitForCompletion(context.EtcdClient, monIDs(cluster.Monitors), monitorAgentName, len(cluster.Monitors))
	if err != nil {
		return err
	}

	// write the latest config to the config dir
	if err := GenerateAdminConnectionConfig(context, cluster); err != nil {
		return fmt.Errorf("failed to write connection config for new mons. %+v", err)
	}

	// wait for quorum
	err = m.waitForQuorum(context, cluster)
	if err != nil {
		return err
	}

	err = createInitialCrushMap(context, cluster)
	if err != nil {
		return err
	}

	if len(monitorsToRemove) == 0 {
		// no monitors to remove
		return nil
	}

	// notify the quorum to remove the bad members
	err = removeMonitorsFromQuorum(context, cluster, monitorsToRemove)
	if err != nil {
		logger.Warningf("failed to remove monitors from quorum. %v", err)
	}

	return nil
}

func removeMonitorsFromQuorum(context *clusterd.Context, cluster *ClusterInfo, monitors map[string]*CephMonitorConfig) error {
	// trigger the monitors to remove, but don't wait for a response very long since they are likely down
	waitSeconds := 10
	err := clusterd.TriggerAgentsAndWait(context.EtcdClient, monIDs(monitors), monitorAgentName, 0, waitSeconds)
	if err != nil {
		return fmt.Errorf("failed to trigger removal of unhealthy monitors. %v", err)
	}

	logger.Infof("removing %d monitors from quorum", len(monitors))
	for _, mon := range monitors {
		if err := RemoveMonitorFromQuorum(context, cluster.Name, mon.Name); err != nil {
			return err
		}
	}

	return nil
}

func RemoveMonitorFromQuorum(context *clusterd.Context, clusterName, name string) error {
	logger.Debugf("removing monitor %s", name)
	args := []string{"mon", "remove", name}
	_, err := client.ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return fmt.Errorf("mon %s remove failed: %+v", name, err)
	}

	logger.Infof("removed monitor %s", name)
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
		if isMonitor(cluster, node.ID) {
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
	monKey := path.Join(CephKey, monitorKey, clusterd.DesiredKey)
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
	logger.Infof("Monitor state. current=%d, desired=%d, unhealthy=%d, toAdd=%d", len(monitors), desiredMonitors, len(monitorsToRemove), newMons)
	if newMons <= 0 {
		logger.Debugf("No need for new monitors")
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
			logger.Debugf("skipping node %s that is %s", nodeID, mon.Name)
			continue
		}

		node, ok := context.Inventory.Nodes[nodeID]
		if !ok || node.PublicIP == "" {
			logger.Errorf("failed to discover desired ip address for node %s. %v", nodeID, err)
			return nil, nil, err
		}
		if !isNodeHealthyForMon(node) {
			logger.Infof("skipping selection of unhealthy node %s as a monitor (age=%s)", nodeID, node.HeartbeatAge)
			continue
		}

		// Store the monitor id and connection info
		port := "6790"
		monitorID := fmt.Sprintf("mon%d", nextMonID)
		settings[path.Join(nodeID, "id")] = monitorID
		settings[path.Join(nodeID, "ipaddress")] = node.PublicIP
		settings[path.Join(nodeID, "port")] = port

		monitor := &CephMonitorConfig{Name: monitorID, Endpoint: fmt.Sprintf("%s:%s", node.PublicIP, port)}
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
	monKey := path.Join(CephKey, monitorKey, clusterd.DesiredKey)
	err = util.StoreEtcdProperties(context.EtcdClient, monKey, settings)
	if err != nil {
		logger.Errorf("failed to save monitor ids. err=%v", err)
		return nil, nil, err
	}

	// remove the unhealthy monitors from the desired state
	for monID, monitor := range monitorsToRemove {
		logger.Infof("removing monitor %s (%+v) from desired state", monID, monitor)
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
	return int(node.HeartbeatAge.Seconds()) < UnhealthyHeartbeatAgeSeconds
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

func WaitForQuorum(context *clusterd.Context, clusterInfo *ClusterInfo) error {
	var mons []string
	for _, mon := range clusterInfo.Monitors {
		mons = append(mons, mon.Name)
	}
	return WaitForQuorumWithMons(context, clusterInfo.Name, mons)
}

func WaitForQuorumWithMons(context *clusterd.Context, clusterName string, mons []string) error {
	logger.Infof("waiting for mon quorum")

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
		monStatusResp, err := client.GetMonStatus(context, clusterName)
		if err != nil {
			logger.Debugf("failed to get mon_status, err: %+v", err)
			continue
		}

		// check if each of the initial monitors is in quorum
		allInQuorum := true
		for _, name := range mons {
			// first get the initial monitors corresponding mon map entry
			var monMapEntry *client.MonMapEntry
			for i := range monStatusResp.MonMap.Mons {
				if name == monStatusResp.MonMap.Mons[i].Name {
					monMapEntry = &monStatusResp.MonMap.Mons[i]
					break
				}
			}

			if monMapEntry == nil {
				// found an initial monitor that is not in the mon map, bail out of this retry
				logger.Warningf("failed to find initial monitor %s in mon map", name)
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
				logger.Warningf("initial monitor %s is not in quorum list", name)
				allInQuorum = false
				break
			}
		}

		if allInQuorum {
			logger.Debugf("all initial monitors are in quorum")
			break
		}
	}

	logger.Infof("Ceph monitors formed quorum")
	return nil
}

func createInitialCrushMap(context *clusterd.Context, cluster *ClusterInfo) error {
	createCrushMap := false

	// check to see if the crush map has already been initialized
	key := path.Join(CephKey, "crushMapInitialized")
	resp, err := context.EtcdClient.Get(ctx.Background(), key, nil)
	if err != nil {
		if !util.IsEtcdKeyNotFound(err) {
			return err
		}
		createCrushMap = true
	} else {
		if resp.Node.Value != "1" {
			createCrushMap = true
		}
	}

	if !createCrushMap {
		// no need to create the crushmap, bail out
		return nil
	}

	logger.Info("creating initial crushmap")
	out, err := client.CreateDefaultCrushMap(context, cluster.Name)
	if err != nil {
		return fmt.Errorf("failed to create initial crushmap: %+v. output: %s", err, out)
	}

	logger.Info("created initial crushmap")

	// save the fact that we've created the initial crush map so we don't do it again
	if _, err := context.EtcdClient.Set(ctx.Background(), key, "1", nil); err != nil {
		return err
	}

	return nil
}
