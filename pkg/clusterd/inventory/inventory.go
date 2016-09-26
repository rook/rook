package inventory

import (
	"fmt"
	"path"

	etcd "github.com/coreos/etcd/client"
	ctx "golang.org/x/net/context"

	"github.com/quantum/castle/pkg/util"
)

const (
	NodesHealthKey              = "/castle/nodes/health"
	NodesConfigKey              = "/castle/nodes/config"
	TriggerHardwareDetectionKey = "trigger-hardware-detection"
)

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
