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
	"fmt"
	"path"

	etcd "github.com/coreos/etcd/client"
	ctx "golang.org/x/net/context"

	"github.com/rook/rook/pkg/util"
	"github.com/rook/rook/pkg/util/exec"
)

const (
	NodesHealthKey              = "/rook/nodes/health"
	NodesConfigKey              = "/rook/nodes/config"
	TriggerHardwareDetectionKey = "trigger-hardware-detection"
)

// Discover all the hardware properties for this node.
// Store the important properties in etcd and more detailed info for the local node in the context.
func DiscoverHardwareAndStore(etcdClient etcd.KeysAPI, executor exec.Executor, nodeID string) (*Hardware, error) {
	hardware, err := DiscoverHardware(executor)
	if err != nil {
		return nil, fmt.Errorf("failed to discover hw. %+v", err)
	}

	if err = storeDevices(etcdClient, nodeID, hardware.Disks); err != nil {
		return nil, fmt.Errorf("failed to store disks in etcd. %+v", err)
	}

	if err = storeMemory(etcdClient, nodeID, hardware.Memory); err != nil {
		return nil, fmt.Errorf("failed to store system memory in etcd. %+v", err)
	}

	return hardware, nil
}

// Discover all the hardware properties for this node.
func DiscoverHardware(executor exec.Executor) (*Hardware, error) {
	devices, err := discoverDevices(executor)
	if err != nil {
		return nil, fmt.Errorf("failed to discover devices. %+v", err)
	}

	mem := getSystemMemory()

	return &Hardware{Disks: devices, Memory: mem}, nil
}

func LoadDiscoveredNodes(etcdClient etcd.KeysAPI) (*Config, error) {

	// Get the discovered state of the infrastructure
	nodes, err := loadNodes(etcdClient)
	if err != nil {
		return nil, err
	}

	return &Config{Nodes: nodes}, nil
}

func TriggerClusterHardwareDetection(etcdClient etcd.KeysAPI) {
	// for each member of the cluster, trigger hardware detection
	members, err := util.GetDirChildKeys(etcdClient, NodesConfigKey)
	if err != nil {
		return
	}

	for member := range members.Iter() {
		hardwareTriggerKey := path.Join(GetNodeConfigKey(member), TriggerHardwareDetectionKey)
		etcdClient.Set(ctx.Background(), hardwareTriggerKey, "1", nil)
	}
}

// Helper to create a Config with a set of node IDs
func CreateConfig(nodeIDs []string) *Config {
	config := &Config{Nodes: make(map[string]*NodeConfig)}
	for _, nodeID := range nodeIDs {
		config.Nodes[nodeID] = &NodeConfig{}
	}
	return config
}

// Helper to get the set of node IDs
func GetNodeIDSet(c *Config) *util.Set {
	set := util.NewSet()
	for nodeId := range c.Nodes {
		set.Add(nodeId)
	}

	return set
}

// Get the cluster configuration from etcd
func loadNodes(etcdClient etcd.KeysAPI) (map[string]*NodeConfig, error) {
	nodes, err := loadNodeConfig(etcdClient)
	if err != nil {
		return nil, fmt.Errorf("failed to load node config. %v", err)
	}

	err = loadNodeHealth(etcdClient, nodes)
	if err != nil {
		return nil, fmt.Errorf("failed to load node health. %v", err)
	}

	return nodes, nil
}
