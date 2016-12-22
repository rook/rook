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
package inventory

import (
	"errors"
	"fmt"
	"path"
	"time"

	ctx "golang.org/x/net/context"

	"github.com/rook/rook/pkg/util"

	"encoding/json"

	etcd "github.com/coreos/etcd/client"
)

const (
	HeartbeatKey        = "heartbeat"
	disksKey            = "disks"
	processorsKey       = "cpu"
	networkKey          = "network"
	memoryKey           = "memory"
	locationKey         = "location"
	heartbeatTtlSeconds = 60 * 60
	publicIpAddressKey  = "publicIp"
	privateIpAddressKey = "privateIp"
)

var (
	HeartbeatTtlDuration = time.Duration(heartbeatTtlSeconds) * time.Second
)

// gets the key under which all node hardware/config will be stored
func GetNodeConfigKey(nodeID string) string {
	return path.Join(NodesConfigKey, nodeID)
}

// Load all the nodes' infrastructure configuration
func loadNodeConfig(etcdClient etcd.KeysAPI) (map[string]*NodeConfig, error) {
	nodesConfig := make(map[string]*NodeConfig)

	// Load the node configuration keys
	nodeInfo, err := etcdClient.Get(ctx.Background(), NodesConfigKey, &etcd.GetOptions{Recursive: true})
	if err != nil && !util.IsEtcdKeyNotFound(err) {
		return nil, fmt.Errorf("failed to get the node config. %v", err)
	}
	if nodeInfo == nil || nodeInfo.Node == nil {
		return nodesConfig, nil
	}

	logger.Tracef("Discovered %d nodes", len(nodeInfo.Node.Nodes))

	for _, etcdNode := range nodeInfo.Node.Nodes {
		nodeConfig := &NodeConfig{}
		nodeID := util.GetLeafKeyPath(etcdNode.Key)

		// get all the config information for the current node
		err = loadHardwareConfig(nodeID, nodeConfig, etcdNode)
		if err != nil {
			logger.Errorf("failed to parse hardware config for node %s, %v", etcdNode, err)
			return nil, err
		}

		nodesConfig[nodeID] = nodeConfig
	}

	return nodesConfig, nil
}

func loadNodeHealth(etcdClient etcd.KeysAPI, nodes map[string]*NodeConfig) error {
	// set the default health info on all the nodes
	for _, node := range nodes {
		// If no heartbeat is found, set the age to one year
		node.HeartbeatAge = time.Hour * 24 * 365
	}

	// Load the node configuration keys
	healthInfo, err := etcdClient.Get(ctx.Background(), NodesHealthKey, &etcd.GetOptions{Recursive: true})
	if err != nil && !util.IsEtcdKeyNotFound(err) {
		return fmt.Errorf("failed to get the node health key. %v", err)
	}
	if healthInfo == nil || healthInfo.Node == nil {
		// no node health found
		return nil
	}

	for _, health := range healthInfo.Node.Nodes {
		nodeID := util.GetLeafKeyPath(health.Key)

		var nodeConfig *NodeConfig
		var ok bool
		if nodeConfig, ok = nodes[nodeID]; !ok {
			logger.Warningf("found health but no config for node %s", nodeID)
			continue
		}

		err := loadSingleNodeHealth(nodeConfig, health)
		if err != nil {
			return fmt.Errorf("failed to load health for node %s. %v", nodeID, err)
		}
	}

	return nil
}

func loadSingleNodeHealth(node *NodeConfig, health *etcd.Node) error {
	for _, prop := range health.Nodes {
		switch util.GetLeafKeyPath(prop.Key) {
		case HeartbeatKey:
			node.HeartbeatAge = HeartbeatTtlDuration - (time.Duration(prop.TTL) * time.Second)
			logger.Tracef("Node %s has age of %s", node.PrivateIP, node.HeartbeatAge.String())
		default:
			return fmt.Errorf("unknown node health key %s", prop.Key)
		}
	}

	return nil
}

// Set the IP address for a node
func SetIPAddress(etcdClient etcd.KeysAPI, nodeId, publicIP, privateIP string) error {
	err := setConfigProperty(etcdClient, nodeId, publicIpAddressKey, publicIP)
	if err != nil {
		return err
	}
	return setConfigProperty(etcdClient, nodeId, privateIpAddressKey, privateIP)
}

func SetLocation(etcdClient etcd.KeysAPI, nodeId, location string) error {
	return setConfigProperty(etcdClient, nodeId, locationKey, location)
}

func setConfigProperty(etcdClient etcd.KeysAPI, nodeId, keyName, val string) error {
	key := path.Join(GetNodeConfigKey(nodeId), keyName)
	_, err := etcdClient.Set(ctx.Background(), key, val, nil)

	return err
}

func loadHardwareConfig(nodeId string, nodeConfig *NodeConfig, nodeInfo *etcd.Node) error {
	if nodeInfo == nil || nodeInfo.Nodes == nil {
		return errors.New("hardware info missing")
	}

	for _, nodeConfigRoot := range nodeInfo.Nodes {
		key := util.GetLeafKeyPath(nodeConfigRoot.Key)
		var err error
		switch key {
		case disksKey:
			err = loadDisksConfig(nodeConfig, nodeConfigRoot.Value)

		case processorsKey:
			err = loadProcessorsConfig(nodeConfig, nodeConfigRoot.Value)

		case networkKey:
			err = loadNetworkConfig(nodeConfig, nodeConfigRoot.Value)

		case memoryKey:
			err = loadMemoryConfig(nodeConfig, nodeConfigRoot.Value)

		case publicIpAddressKey:
			nodeConfig.PublicIP = nodeConfigRoot.Value

		case privateIpAddressKey:
			nodeConfig.PrivateIP = nodeConfigRoot.Value

		case locationKey:
			nodeConfig.Location = nodeConfigRoot.Value

		default:
			logger.Warningf("unexpected hardware component: %s, skipping...", nodeConfigRoot)
		}

		if err != nil {
			logger.Errorf("failed to load %s config for node %s, %v", key, nodeId, err)
			return err
		}
	}

	return nil
}

func loadProcessorsConfig(nodeConfig *NodeConfig, rawProcs string) error {
	var processors []ProcessorConfig
	if err := json.Unmarshal([]byte(rawProcs), &processors); err != nil {
		return fmt.Errorf("failed to deserialize processors. %+v", err)
	}
	nodeConfig.Processors = processors
	return nil
}

func loadNetworkConfig(nodeConfig *NodeConfig, rawAdapters string) error {
	var adapters []NetworkConfig
	if err := json.Unmarshal([]byte(rawAdapters), &adapters); err != nil {
		return fmt.Errorf("failed to deserialize network adapters. %+v", err)
	}
	nodeConfig.NetworkAdapters = adapters
	return nil
}

func storeNetworkConfig(etcdClient etcd.KeysAPI, nodeID string, config []NetworkConfig) error {
	output, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal network config. %+v", err)
	}

	key := path.Join(NodesConfigKey, nodeID, networkKey)
	_, err = etcdClient.Set(ctx.Background(), key, string(output), nil)
	if err != nil {
		return fmt.Errorf("failed to store network config in etcd. %+v", err)
	}

	return nil
}

func storeProcessorConfig(etcdClient etcd.KeysAPI, nodeID string, config []ProcessorConfig) error {
	output, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal processor config. %+v", err)
	}

	key := path.Join(NodesConfigKey, nodeID, processorsKey)
	_, err = etcdClient.Set(ctx.Background(), key, string(output), nil)
	if err != nil {
		return fmt.Errorf("failed to store processor config in etcd. %+v", err)
	}

	return nil
}
