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
	"log"
	"path"
	"strconv"
	"time"

	ctx "golang.org/x/net/context"

	"github.com/rook/rook/pkg/util"
	"github.com/rook/rook/pkg/util/exec"

	etcd "github.com/coreos/etcd/client"
)

const (
	HeartbeatKey        = "heartbeat"
	disksKey            = "disks"
	processorsKey       = "cpu"
	networkKey          = "net"
	memoryKey           = "mem"
	locationKey         = "location"
	heartbeatTtlSeconds = 60 * 60
	publicIpAddressKey  = "publicIp"
	privateIpAddressKey = "privateIp"
)

var (
	HeartbeatTtlDuration = time.Duration(heartbeatTtlSeconds) * time.Second
)

func DiscoverHardware(nodeID string, etcdClient etcd.KeysAPI, executor exec.Executor) error {
	nodeConfigKey := GetNodeConfigKey(nodeID)
	if err := discoverDisks(nodeConfigKey, etcdClient, executor); err != nil {
		return err
	}

	// TODO: discover more hardware properties

	return nil
}

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

	// verbose: log.Printf("Discovered %d nodes", len(nodeInfo.Node.Nodes))

	for _, etcdNode := range nodeInfo.Node.Nodes {
		nodeConfig := &NodeConfig{}
		nodeID := util.GetLeafKeyPath(etcdNode.Key)

		// get all the config information for the current node
		err = loadHardwareConfig(nodeID, nodeConfig, etcdNode)
		if err != nil {
			log.Printf("failed to parse hardware config for node %s, %v", etcdNode, err)
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
			log.Printf("found health but no config for node %s", nodeID)
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
			// verbose: log.Printf("Node %s has age of %s", node.IPAddress, node.HeartbeatAge.String())
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
			err = loadDisksConfig(nodeConfig, nodeConfigRoot)

		case processorsKey:
			err = loadProcessorsConfig(nodeConfig, nodeConfigRoot)

		case memoryKey:
			err = loadMemoryConfig(nodeConfig, nodeConfigRoot)

		case networkKey:
			err = loadNetworkConfig(nodeConfig, nodeConfigRoot)

		case publicIpAddressKey:
			err = loadSimpleConfigStringProperty(&(nodeConfig.PublicIP), nodeConfigRoot, "Public IP")

		case privateIpAddressKey:
			err = loadSimpleConfigStringProperty(&(nodeConfig.PrivateIP), nodeConfigRoot, "Private IP")

		case locationKey:
			err = loadSimpleConfigStringProperty(&(nodeConfig.Location), nodeConfigRoot, "Location")

		default:
			log.Printf("unexpected hardware component: %s, skipping...", nodeConfigRoot)
		}

		if err != nil {
			log.Printf("failed to load %s config for node %s, %v", key, nodeId, err)
			return err
		}
	}

	return nil
}

func loadDisksConfig(nodeConfig *NodeConfig, disksRootNode *etcd.Node) error {
	numDisks := 0
	if disksRootNode.Nodes != nil {
		numDisks = len(disksRootNode.Nodes)
	}

	nodeConfig.Disks = make([]DiskConfig, numDisks)

	// iterate over all disks from etcd
	for i, diskInfo := range disksRootNode.Nodes {
		disk, err := getDiskInfo(diskInfo)
		if err != nil {
			log.Printf("Failed to get disk. err=%v", err)
			return err
		}

		nodeConfig.Disks[i] = *disk

	}

	return nil
}

func loadProcessorsConfig(nodeConfig *NodeConfig, procsRootNode *etcd.Node) error {
	numProcs := 0
	if procsRootNode.Nodes != nil {
		numProcs = len(procsRootNode.Nodes)
	}

	nodeConfig.Processors = make([]ProcessorConfig, numProcs)

	// iterate over all processors from etcd
	for i, procInfo := range procsRootNode.Nodes {
		proc := ProcessorConfig{}
		if procID, err := strconv.ParseUint(util.GetLeafKeyPath(procInfo.Key), 10, 32); err != nil {
			return err
		} else {
			proc.ID = uint(procID)
		}

		// iterate over all properties of the processor
		for _, procProperty := range procInfo.Nodes {
			procPropertyName := util.GetLeafKeyPath(procProperty.Key)
			switch procPropertyName {
			case ProcPhysicalIDKey:
				if phsyicalId, err := strconv.ParseUint(procProperty.Value, 10, 32); err != nil {
					return err
				} else {
					proc.PhysicalID = uint(phsyicalId)
				}
			case ProcSiblingsKey:
				if siblings, err := strconv.ParseUint(procProperty.Value, 10, 32); err != nil {
					return err
				} else {
					proc.Siblings = uint(siblings)
				}
			case ProcCoreIDKey:
				if coreId, err := strconv.ParseUint(procProperty.Value, 10, 32); err != nil {
					return err
				} else {
					proc.CoreID = uint(coreId)
				}
			case ProcNumCoresKey:
				if numCores, err := strconv.ParseUint(procProperty.Value, 10, 32); err != nil {
					return err
				} else {
					proc.NumCores = uint(numCores)
				}
			case ProcSpeedKey:
				if speed, err := strconv.ParseFloat(procProperty.Value, 64); err != nil {
					return err
				} else {
					proc.Speed = speed
				}
			case ProcBitsKey:
				if numBits, err := strconv.ParseUint(procProperty.Value, 10, 32); err != nil {
					return err
				} else {
					proc.Bits = uint(numBits)
				}
			default:
				log.Printf("unknown processor property key %s, skipping", procPropertyName)
			}
		}

		nodeConfig.Processors[i] = proc
	}

	return nil
}

func loadMemoryConfig(nodeConfig *NodeConfig, memoryRootNode *etcd.Node) error {
	mem := MemoryConfig{}
	for _, memProperty := range memoryRootNode.Nodes {
		memPropertyName := util.GetLeafKeyPath(memProperty.Key)
		switch memPropertyName {
		case MemoryTotalSizeKey:
			if size, err := strconv.ParseUint(memProperty.Value, 10, 64); err != nil {
				return err
			} else {
				mem.TotalSize = size
			}
		default:
			log.Printf("unknown memory property key %s, skipping", memPropertyName)
		}
	}

	nodeConfig.Memory = mem
	return nil
}

func loadNetworkConfig(nodeConfig *NodeConfig, networkRootNode *etcd.Node) error {
	numNics := 0
	if networkRootNode.Nodes != nil {
		numNics = len(networkRootNode.Nodes)
	}

	nodeConfig.NetworkAdapters = make([]NetworkConfig, numNics)

	// iterate over all network adapters from etcd
	for i, netInfo := range networkRootNode.Nodes {
		net := NetworkConfig{}
		net.Name = util.GetLeafKeyPath(netInfo.Key)

		// iterate over all properties of the network adapter
		for _, netProperty := range netInfo.Nodes {
			netPropertyName := util.GetLeafKeyPath(netProperty.Key)
			switch netPropertyName {
			case NetworkIPv4AddressKey:
				net.IPv4Address = netProperty.Value
			case NetworkIPv6AddressKey:
				net.IPv6Address = netProperty.Value
			case NetworkSpeedKey:
				if netProperty.Value == "" {
					net.Speed = 0
				} else if speed, err := strconv.ParseUint(netProperty.Value, 10, 64); err != nil {
					return err
				} else {
					net.Speed = speed
				}
			default:
				log.Printf("unknown network adapter property key %s, skipping", netPropertyName)
			}
		}

		nodeConfig.NetworkAdapters[i] = net
	}

	return nil
}

func loadSimpleConfigStringProperty(cfgField *string, cfgNode *etcd.Node, propName string) error {
	if cfgNode.Dir {
		return fmt.Errorf("%s node '%s' is a directory, but it's expected to be a key", propName, cfgNode.Key)
	}
	*cfgField = cfgNode.Value
	return nil
}
