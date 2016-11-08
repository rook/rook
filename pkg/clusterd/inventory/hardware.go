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
	PublicIP        string            `json:"publicIp"`
	PrivateIP       string            `json:"privateIp"`
	HeartbeatAge    time.Duration     `json:"heartbeatAge"`
	Location        string            `json:"location"`
}

type DiskConfig struct {
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
