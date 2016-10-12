package inventory

import (
	"errors"
	"fmt"
	"log"
	"path"
	"strconv"
	"strings"
	"time"

	ctx "golang.org/x/net/context"

	"github.com/quantum/castle/pkg/util"
	"github.com/quantum/castle/pkg/util/exec"

	etcd "github.com/coreos/etcd/client"
)

const (
	HeartbeatKey        = "heartbeat"
	IpAddressKey        = "ipaddress"
	DisksKey            = "disks"
	ProcessorsKey       = "cpu"
	NetworkKey          = "net"
	MemoryKey           = "mem"
	LocationKey         = "location"
	heartbeatTtlSeconds = 60 * 60
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

	log.Printf("Discovered %d nodes", len(nodeInfo.Node.Nodes))

	for _, etcdNode := range nodeInfo.Node.Nodes {
		nodeConfig := &NodeConfig{}
		nodeID := util.GetLeafKeyPath(etcdNode.Key)

		// get all the config information for the current node
		err = loadHardwareConfig(nodeID, nodeConfig, etcdNode)
		if err != nil {
			log.Printf("failed to parse hardware config for node %s, %v", etcdNode, err)
			return nil, err
		}

		ipAddr, err := getIPAddress(nodeID, etcdNode)
		if err != nil {
			return nil, fmt.Errorf("failed to get IP address for node %s. %v", nodeID, err)
		}
		nodeConfig.IPAddress = ipAddr

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
			log.Printf("Node %s has age of %s", node.IPAddress, node.HeartbeatAge.String())
		default:
			return fmt.Errorf("unknown node health key %s", prop.Key)
		}
	}

	return nil
}

// Set the IP address for a node
func SetIPAddress(etcdClient etcd.KeysAPI, nodeId, ipaddress string) error {
	return setConfigProperty(etcdClient, nodeId, IpAddressKey, ipaddress)
}

func SetLocation(etcdClient etcd.KeysAPI, nodeId, location string) error {
	return setConfigProperty(etcdClient, nodeId, LocationKey, location)
}

func setConfigProperty(etcdClient etcd.KeysAPI, nodeId, keyName, val string) error {
	key := path.Join(GetNodeConfigKey(nodeId), keyName)
	_, err := etcdClient.Set(ctx.Background(), key, val, nil)

	return err
}

func getIPAddress(nodeID string, nodeInfo *etcd.Node) (string, error) {
	for _, prop := range nodeInfo.Nodes {
		switch util.GetLeafKeyPath(prop.Key) {
		case "ipaddress":
			return prop.Value, nil
		}
	}

	return "", fmt.Errorf("node %s ip address not found", nodeID)
}

func loadHardwareConfig(nodeId string, nodeConfig *NodeConfig, nodeInfo *etcd.Node) error {
	if nodeInfo == nil || nodeInfo.Nodes == nil {
		return errors.New("hardware info missing")
	}

	for _, nodeConfigRoot := range nodeInfo.Nodes {
		switch util.GetLeafKeyPath(nodeConfigRoot.Key) {
		case DisksKey:
			err := loadDisksConfig(nodeConfig, nodeConfigRoot)
			if err != nil {
				log.Printf("failed to load disk config for node %s, %v", nodeId, err)
				return err
			}
		case ProcessorsKey:
			err := loadProcessorsConfig(nodeConfig, nodeConfigRoot)
			if err != nil {
				log.Printf("failed to load processor config for node %s, %v", nodeId, err)
				return err
			}
		case MemoryKey:
			err := loadMemoryConfig(nodeConfig, nodeConfigRoot)
			if err != nil {
				log.Printf("failed to load memory config for node %s, %v", nodeId, err)
				return err
			}
		case NetworkKey:
			err := loadNetworkConfig(nodeConfig, nodeConfigRoot)
			if err != nil {
				log.Printf("failed to load network config for node %s, %v", nodeId, err)
				return err
			}
		case IpAddressKey:
			err := loadSimpleConfigStringProperty(&(nodeConfig.IPAddress), nodeConfigRoot, "IP Address")
			if err != nil {
				log.Printf("failed to load IP address config for node %s, %v", nodeId, err)
				return err
			}
		case LocationKey:
			err := loadSimpleConfigStringProperty(&(nodeConfig.Location), nodeConfigRoot, "Location")
			if err != nil {
				log.Printf("failed to load location config for node %s, %v", nodeId, err)
				return err
			}
		default:
			log.Printf("unexpected hardware component: %s, skipping...", nodeConfigRoot)
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
		disk, err := GetDiskInfo(diskInfo)
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

// converts a raw key value pair string into a map of key value pairs
// example raw string of `foo="0" bar="1" baz="biz"` is returned as:
// map[string]string{"foo":"0", "bar":"1", "baz":"biz"}
func parseKeyValuePairString(propsRaw string) map[string]string {
	// first split the single raw string on spaces and initialize a map of
	// a length equal to the number of pairs
	props := strings.Split(propsRaw, " ")
	propMap := make(map[string]string, len(props))

	for _, kvpRaw := range props {
		// split each individual key value pair on the equals sign
		kvp := strings.Split(kvpRaw, "=")
		if len(kvp) == 2 {
			// first element is the final key, second element is the final value
			// (don't forget to remove surrounding quotes from the value)
			propMap[kvp[0]] = strings.Replace(kvp[1], `"`, "", -1)
		}
	}

	return propMap
}
