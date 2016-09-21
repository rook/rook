package inventory

import (
	"path"
	"strconv"
	"testing"
	"time"

	etcd "github.com/coreos/etcd/client"
	ctx "golang.org/x/net/context"

	"github.com/quantum/castle/pkg/util"
	"github.com/stretchr/testify/assert"
)

func TestLoadDiscoveredNodes(t *testing.T) {
	etcdClient := &util.MockEtcdClient{}

	// Create some test config
	etcdClient.SetValue(path.Join(NodesConfigKey, "23", "ipaddress"), "1.2.3.4")
	etcdClient.SetValue(path.Join(NodesConfigKey, "46", "ipaddress"), "4.5.6.7")

	config, err := LoadDiscoveredNodes(etcdClient)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(config.Nodes))
	assert.Equal(t, "1.2.3.4", config.Nodes["23"].IPAddress)
	assert.Equal(t, "4.5.6.7", config.Nodes["46"].IPAddress)
	assert.Equal(t, time.Hour*24*365, config.Nodes["23"].HeartbeatAge) // no heartbeat has an age of a year

	desiredIpaddress := "9.8.7.6"
	err = SetIPAddress(etcdClient, "23", desiredIpaddress)
	assert.Nil(t, err)

	config, err = LoadDiscoveredNodes(etcdClient)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(config.Nodes))
	assert.Equal(t, "9.8.7.6", config.Nodes["23"].IPAddress)
	assert.Equal(t, "4.5.6.7", config.Nodes["46"].IPAddress)
}

func TestLoadHardwareConfig(t *testing.T) {
	machineId := "df1c87e8266843f2ab822c0d72f584d3"
	etcdClient := util.NewMockEtcdClient()

	// set up the hardware date in etcd
	hardwareKey := path.Join(NodesConfigKey, machineId)
	etcdClient.CreateDir(hardwareKey)

	// setup disk info in etcd
	disksConfig := make([]DiskConfig, 2)
	disksConfig[0] = TestSetDiskInfo(etcdClient, hardwareKey, "MB2CK3F6S5041EPCPJ4T", "sda", "506d4869-29ee-4bfd-bf21-dfd597bd222e",
		10737418240, true, false, "btrfs", "/mnt/abc", Disk, "", true)
	disksConfig[1] = TestSetDiskInfo(etcdClient, hardwareKey, "2B9C7KZN3VBM77PSA63P", "sda2", "506d4869-29ee-4bfd-bf21-dfd597bd222e",
		2097152, false, true, "", "", Part, "sda", false)

	// setup processor info in etcd
	procsKey := path.Join(hardwareKey, ProcessorsKey)
	etcdClient.Set(ctx.Background(), procsKey, "", &etcd.SetOptions{Dir: true})
	procsConfig := make([]ProcessorConfig, 3)
	procsConfig[0] = setProcInfo(etcdClient, procsKey, 0, 0, 1, 0, 1, 1234.56, 64)
	procsConfig[1] = setProcInfo(etcdClient, procsKey, 1, 1, 2, 0, 2, 8000.00, 32)
	procsConfig[2] = setProcInfo(etcdClient, procsKey, 2, 1, 2, 1, 2, 4000.01, 32)

	// setup memory info in etcd
	memKey := path.Join(hardwareKey, MemoryKey)
	etcdClient.Set(ctx.Background(), memKey, "", &etcd.SetOptions{Dir: true})
	memConfig := setMemoryInfo(etcdClient, memKey, 4149252096)

	// set up network info in etcd
	netKey := path.Join(hardwareKey, NetworkKey)
	etcdClient.Set(ctx.Background(), netKey, "", &etcd.SetOptions{Dir: true})
	netsConfig := make([]NetworkConfig, 2)
	netsConfig[0] = setNetInfo(etcdClient, netKey, "eth0", "172.17.42.1/16", "fe80::42:4aff:fefe:13d7/64", 0)
	netsConfig[1] = setNetInfo(etcdClient, netKey, "veth2b6453a", "", "fe80::7c0f:acff:feff:478d/64", 10000)

	// set IP address in etcd
	SetIPAddress(etcdClient, machineId, "10.0.0.43")

	// load the discovered node config
	nodeConfig, err := loadNodeConfig(etcdClient)
	assert.Nil(t, err, "loaded node config error should be nil")
	assert.NotNil(t, nodeConfig, "loaded node config should not be nil")
	assert.Equal(t, 1, len(nodeConfig))

	// verify all the hardware configuration retrieved from etcd
	verifyDiskConfig(t, nodeConfig[machineId], disksConfig)
	verifyProcConfig(t, nodeConfig[machineId], procsConfig)
	verifyMemoryConfig(t, nodeConfig[machineId], memConfig)
	verifyNetworkConfig(t, nodeConfig[machineId], netsConfig)
	assert.Equal(t, "10.0.0.43", nodeConfig[machineId].IPAddress)
}

func setProcInfo(etcdClient *util.MockEtcdClient, procsKey string, procId uint, physicalId uint, siblings uint,
	coreID uint, numCores uint, speed float64, bits uint) ProcessorConfig {

	procKey := path.Join(procsKey, strconv.FormatUint(uint64(procId), 10))
	etcdClient.Set(ctx.Background(), procKey, "", &etcd.SetOptions{Dir: true})

	etcdClient.Set(ctx.Background(), path.Join(procKey, ProcPhysicalIDKey), strconv.FormatUint(uint64(physicalId), 10), nil)
	etcdClient.Set(ctx.Background(), path.Join(procKey, ProcSiblingsKey), strconv.FormatUint(uint64(siblings), 10), nil)
	etcdClient.Set(ctx.Background(), path.Join(procKey, ProcCoreIDKey), strconv.FormatUint(uint64(coreID), 10), nil)
	etcdClient.Set(ctx.Background(), path.Join(procKey, ProcNumCoresKey), strconv.FormatUint(uint64(numCores), 10), nil)
	etcdClient.Set(ctx.Background(), path.Join(procKey, ProcSpeedKey), strconv.FormatFloat(speed, 'f', 2, 64), nil)
	etcdClient.Set(ctx.Background(), path.Join(procKey, ProcBitsKey), strconv.FormatUint(uint64(bits), 10), nil)

	return ProcessorConfig{
		ID:         procId,
		PhysicalID: physicalId,
		Siblings:   siblings,
		CoreID:     coreID,
		NumCores:   numCores,
		Speed:      speed,
		Bits:       bits,
	}
}

func setMemoryInfo(etcdClient *util.MockEtcdClient, memKey string, totalMem uint64) MemoryConfig {
	etcdClient.Set(ctx.Background(), path.Join(memKey, MemoryTotalSizeKey), strconv.FormatUint(totalMem, 10), nil)
	return MemoryConfig{TotalSize: totalMem}
}

func setNetInfo(etcdClient *util.MockEtcdClient, netsKey string, name string, ipv4Addr string, ipv6Addr string, speed uint64) NetworkConfig {
	netKey := path.Join(netsKey, name)
	etcdClient.Set(ctx.Background(), netKey, "", &etcd.SetOptions{Dir: true})

	etcdClient.Set(ctx.Background(), path.Join(netKey, NetworkIPv4AddressKey), ipv4Addr, nil)
	etcdClient.Set(ctx.Background(), path.Join(netKey, NetworkIPv6AddressKey), ipv6Addr, nil)
	speedStr := ""
	if speed > 0 {
		speedStr = strconv.FormatUint(speed, 10)
	}
	etcdClient.Set(ctx.Background(), path.Join(netKey, NetworkSpeedKey), speedStr, nil)

	return NetworkConfig{
		Name:        name,
		IPv4Address: ipv4Addr,
		IPv6Address: ipv6Addr,
		Speed:       speed,
	}
}

func verifyDiskConfig(t *testing.T, nodeConfig *NodeConfig, expectedDisksConfig []DiskConfig) {
	assert.Equal(t, len(expectedDisksConfig), len(nodeConfig.Disks))

	for _, expectedDisk := range expectedDisksConfig {
		var matchingActual DiskConfig
		for _, actualDisk := range nodeConfig.Disks {
			if actualDisk.Serial == expectedDisk.Serial {
				matchingActual = actualDisk
				break
			}
		}

		assert.NotNil(t, matchingActual, "missing actual disk %s", expectedDisk.Serial)
		assert.Equal(t, expectedDisk, matchingActual)
	}
}

func verifyProcConfig(t *testing.T, nodeConfig *NodeConfig, expectedProcsConfig []ProcessorConfig) {
	assert.Equal(t, len(expectedProcsConfig), len(nodeConfig.Processors))

	for _, expectedProc := range expectedProcsConfig {
		var matchingActual ProcessorConfig
		for _, actualProc := range nodeConfig.Processors {
			if actualProc.ID == expectedProc.ID {
				matchingActual = actualProc
				break
			}
		}

		assert.NotNil(t, matchingActual, "missing actual proc %d", expectedProc.ID)
		assert.Equal(t, expectedProc, matchingActual)
	}
}

func verifyMemoryConfig(t *testing.T, nodeConfig *NodeConfig, expectedMemConfig MemoryConfig) {
	assert.NotNil(t, nodeConfig.Memory)
	assert.Equal(t, expectedMemConfig, nodeConfig.Memory)
}

func verifyNetworkConfig(t *testing.T, nodeConfig *NodeConfig, expectedNetsConfig []NetworkConfig) {
	assert.Equal(t, len(expectedNetsConfig), len(nodeConfig.NetworkAdapters))

	for _, expectedNet := range expectedNetsConfig {
		var matchingActual NetworkConfig
		for _, actualNet := range nodeConfig.NetworkAdapters {
			if actualNet.Name == expectedNet.Name {
				matchingActual = actualNet
				break
			}
		}

		assert.NotNil(t, matchingActual, "missing actual network adapter %s", expectedNet.Name)
		assert.Equal(t, expectedNet, matchingActual)
	}
}

func TestGetSimpleDiskPropertiesFromSerial(t *testing.T) {
	nodeID := "df1c87e8266843f2ab822c0d72f584d3"
	etcdClient := &util.MockEtcdClient{}
	hardwareKey := path.Join(NodesConfigKey, nodeID)
	etcdClient.Set(ctx.Background(), hardwareKey, "", &etcd.SetOptions{Dir: true})
	TestSetDiskInfo(etcdClient, hardwareKey, "MB2CK3F6S5041EPCPJ4T", "sda", "506d4869-29ee-4bfd-bf21-dfd597bd222e",
		10737418240, true, false, "btrfs", "/mnt/abc", Disk, "", true)

	diskNode, _ := etcdClient.Get(ctx.Background(), path.Join(hardwareKey, "disks", "MB2CK3F6S5041EPCPJ4T"), nil)
	disk, err := GetDiskInfo(diskNode.Node)
	assert.Nil(t, err)

	assert.Equal(t, "sda", disk.Name)
	assert.Equal(t, "506d4869-29ee-4bfd-bf21-dfd597bd222e", disk.UUID)
}
