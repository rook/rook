package inventory

import (
	"errors"
	"fmt"
	"log"
	"path"
	"strconv"

	ctx "golang.org/x/net/context"

	"github.com/quantum/castle/pkg/util"

	etcd "github.com/coreos/etcd/client"
)

// The IP address of a node is stored in etcd. However, in the event that the cluster is cleaned up
// and the etcd values are wiped out, the ip address is not set again until the agent launches again.
// The agent will not launch again unless killed by the developer, therefore we cache the IP address
// so the next configuration can succeed.
var fallbackIPaddress string

const (
	IpAddressKey  = "%s/%s/ipaddress"
	DisksKey      = "disks"
	ProcessorsKey = "cpu"
	NetworkKey    = "net"
	MemoryKey     = "mem"
)

// Load all the nodes' infrastructure configuration
func loadNodeConfig(etcdClient etcd.KeysAPI) (map[string]*NodeConfig, error) {

	// Load the discovered nodes
	nodes, err := util.GetDirChildKeys(etcdClient, DiscoveredNodesKey)
	log.Printf("Discovered %d nodes", nodes.Count())
	if err != nil {
		log.Printf("failed to get the node ids. err=%s", err.Error())
		return nil, err
	}

	nodesConfig := make(map[string]*NodeConfig)
	for node := range nodes.Iter() {
		nodeConfig := &NodeConfig{}

		// get all the config information for the current node
		configKey := path.Join(DiscoveredNodesKey, node)
		nodeInfo, err := etcdClient.Get(ctx.Background(), configKey, &etcd.GetOptions{Recursive: true})
		if err != nil {
			if util.IsEtcdKeyNotFound(err) {
				log.Printf("skipping node %s with no hardware discovered", node)
				continue
			}
			log.Printf("failed to get hardware info from etcd for node %s, %v", node, err)
		} else {
			err = loadHardwareConfig(node, nodeConfig, nodeInfo)
			if err != nil {
				log.Printf("failed to parse hardware config for node %s, %v", node, err)
				return nil, err
			}
		}

		ipAddr, err := GetIpAddress(etcdClient, node)
		if err != nil {
			log.Printf("failed to get IP address for node %s, %+v", node, err)
			return nil, err
		}
		nodeConfig.IPAddress = ipAddr

		nodesConfig[node] = nodeConfig
	}

	return nodesConfig, nil
}

// Get the IP address for a node
func GetIpAddress(etcdClient etcd.KeysAPI, nodeId string) (string, error) {
	key := fmt.Sprintf(IpAddressKey, DiscoveredNodesKey, nodeId)
	val, err := etcdClient.Get(ctx.Background(), key, nil)
	if err != nil {
		if util.IsEtcdKeyNotFound(err) {
			return fallbackIPaddress, nil
		}
		return "", err
	}

	return val.Node.Value, nil
}

// Set the IP address for a node
func SetIpAddress(etcdClient etcd.KeysAPI, nodeId, ipaddress string) error {
	key := fmt.Sprintf(IpAddressKey, DiscoveredNodesKey, nodeId)
	_, err := etcdClient.Set(ctx.Background(), key, ipaddress, nil)
	fallbackIPaddress = ipaddress

	return err
}

func loadHardwareConfig(nodeId string, nodeConfig *NodeConfig, nodeInfo *etcd.Response) error {
	if nodeInfo == nil || nodeInfo.Node == nil {
		return errors.New("hardware info missing")
	}

	for _, nodeConfigRoot := range nodeInfo.Node.Nodes {
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
