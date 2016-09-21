package inventory

import "time"

type DiskType int

const (
	MemoryTotalSizeKey             = "total"
	NetworkIPv4AddressKey          = "ipv4"
	NetworkIPv6AddressKey          = "ipv6"
	NetworkSpeedKey                = "speed"
	ProcPhysicalIDKey              = "physical-id"
	ProcSiblingsKey                = "siblings"
	ProcCoreIDKey                  = "core-id"
	ProcNumCoresKey                = "cores"
	ProcSpeedKey                   = "speed"
	ProcBitsKey                    = "arch"
	Disk                  DiskType = iota
	Part
)

type Config struct {
	Nodes map[string]*NodeConfig `json:"nodes"`
}

type NodeConfig struct {
	Disks           []DiskConfig      `json:"disks"`
	Processors      []ProcessorConfig `json:"processors"`
	Memory          MemoryConfig      `json:"memory"`
	NetworkAdapters []NetworkConfig   `json:"networkAdapters"`
	IPAddress       string            `json:"ipAddr"`
	HeartbeatAge    time.Duration     `json:heartbeatAge`
}

type DiskConfig struct {
	Serial      string   `json:"serial"`
	Name        string   `json:"name"`
	UUID        string   `json:"uuid"`
	Size        uint64   `json:"size"`
	Rotational  bool     `json:"rotational"`
	Readonly    bool     `json:"readonly"`
	FileSystem  string   `json:"fileSystem"`
	MountPoint  string   `json:"mountPoint"`
	Type        DiskType `json:"type"`
	Parent      string   `json:"parent"`
	HasChildren bool     `json:"hasChildren"`
}

type MemoryConfig struct {
	TotalSize uint64 `json:"totalSize"`
}

type ProcessorConfig struct {
	ID         uint    `json:"id"`
	PhysicalID uint    `json:"physicalId"`
	Siblings   uint    `json:"siblings"`
	CoreID     uint    `json:"coreId"`
	NumCores   uint    `json:"numCores"`
	Speed      float64 `json:"speed"`
	Bits       uint    `json:"bits"`
}

type NetworkConfig struct {
	Name        string `json:"name"`
	IPv4Address string `json:"ipv4"`
	IPv6Address string `json:"ipv6"`
	Speed       uint64 `json:"speed"`
}
