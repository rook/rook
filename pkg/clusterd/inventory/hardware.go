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

const (
	DiskType = "disk"
	SSDType  = "ssd"
	PartType = "part"
)

type Config struct {
	Nodes map[string]*NodeConfig `json:"nodes"`
	Local *Hardware              `json:"local"`
}

// Basic config info for a node in the cluster
type NodeConfig struct {
	Disks           []Disk            `json:"disks"`
	Processors      []ProcessorConfig `json:"processors"`
	NetworkAdapters []NetworkConfig   `json:"networkAdapters"`
	Memory          uint64            `json:"memory"`
	PublicIP        string            `json:"publicIp"`
	PrivateIP       string            `json:"privateIp"`
	HeartbeatAge    time.Duration     `json:"heartbeatAge"`
	Location        string            `json:"location"`
}

type Disk struct {
	Available  bool   `json:"available"`
	Type       string `json:"type"`
	Size       uint64 `json:"size"`
	Rotational bool   `json:"rotational"`
}

// Local hardware info
type Hardware struct {
	Disks           []LocalDisk       `json:"disks"`
	Processors      []ProcessorConfig `json:"processors"`
	NetworkAdapters []NetworkConfig   `json:"networkAdapters"`
	Memory          uint64            `json:"memory"`
}

type LocalDisk struct {
	Name        string `json:"name"`
	ID          string `json:"id"`
	UUID        string `json:"uuid"`
	Size        uint64 `json:"size"`
	Rotational  bool   `json:"rotational"`
	Readonly    bool   `json:"readonly"`
	FileSystem  string `json:"fileSystem"`
	MountPoint  string `json:"mountPoint"`
	Type        string `json:"type"`
	Parent      string `json:"parent"`
	HasChildren bool   `json:"hasChildren"`
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
