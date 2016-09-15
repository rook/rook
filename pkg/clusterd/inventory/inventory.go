package inventory

import (
	"path"

	etcd "github.com/coreos/etcd/client"
	ctx "golang.org/x/net/context"

	"github.com/quantum/castle/pkg/util"
)

const (
	DiscoveredNodesHealthKey    = "/castle/nodes/health"
	DiscoveredNodesKey          = "/castle/nodes/config"
	TriggerHardwareDetectionKey = "trigger-hardware-detection"
)

func LoadDiscoveredNodes(etcdClient etcd.KeysAPI) (*Config, error) {

	// Get the discovered state of the infrastructure
	discovered, err := loadDiscoveredConfig(etcdClient)
	if err != nil {
		return nil, err
	}

	return &Config{Nodes: discovered.Nodes}, nil
}

func TriggerClusterHardwareDetection(etcdClient etcd.KeysAPI) {
	// for each member of the cluster, trigger hardware detection
	members, err := util.GetDirChildKeys(etcdClient, DiscoveredNodesKey)
	if err != nil {
		return
	}

	for member := range members.Iter() {
		hardwareTriggerKey := path.Join(DiscoveredNodesKey, member, TriggerHardwareDetectionKey)
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
func loadDiscoveredConfig(etcdClient etcd.KeysAPI) (*Config, error) {
	return loadConfig(etcdClient)
}

// Get the cluster configuration from etcd
func loadConfig(etcdClient etcd.KeysAPI) (*Config, error) {
	nodes, err := loadNodeConfig(etcdClient)
	if err != nil {
		return nil, err
	}

	return &Config{
		Nodes: nodes,
	}, nil
}
