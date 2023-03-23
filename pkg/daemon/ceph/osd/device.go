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

package osd

import (
	"encoding/json"
	"os"

	oposd "github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	"github.com/rook/rook/pkg/util/sys"
)

const (
	bootstrapOSDKeyringTemplate = `
[client.bootstrap-osd]
	key = %s
	caps mon = "allow profile bootstrap-osd"
`
)

// Device is a device
type Device struct {
	Name   string `json:"name"`
	NodeID string `json:"nodeId"`
	Dir    bool   `json:"bool"`
}

// DesiredDevice keeps track of the desired settings for a device
type DesiredDevice struct {
	Name               string
	OSDsPerDevice      int
	MetadataDevice     string
	DatabaseSizeMB     int
	DeviceClass        string
	InitialWeight      string
	IsFilter           bool
	IsDevicePathFilter bool
}

// DeviceOsdMapping represents the mapping of an OSD on disk
type DeviceOsdMapping struct {
	Entries map[string]*DeviceOsdIDEntry // device name to OSD ID mapping entry
}

// DeviceOsdIDEntry represents the details of an OSD
type DeviceOsdIDEntry struct {
	Data                  int           // OSD ID that has data stored here
	Metadata              []int         // OSD IDs (multiple) that have metadata stored here
	Config                DesiredDevice // Device specific config options
	PersistentDevicePaths []string
	DeviceInfo            *sys.LocalDisk // low-level info about the device
}

func (m *DeviceOsdMapping) String() string {
	b, _ := json.Marshal(m)
	return string(b)
}

func (d *DesiredDevice) UpdateDeviceClass(agent *OsdAgent, device *sys.LocalDisk) {
	// Rook sets the storage class of a device with the following priority.
	//
	// 1. the device-level configuration
	// 2. the global or node-local configuration
	// 3. the default value estimated from sysfs.
	if d.DeviceClass != "" {
		return
	}

	if agent.pvcBacked {
		crushDeviceClass := os.Getenv(oposd.CrushDeviceClassVarName)
		if crushDeviceClass != "" {
			d.DeviceClass = crushDeviceClass
			return
		}
	} else {
		if agent.storeConfig.DeviceClass != "" {
			d.DeviceClass = agent.storeConfig.DeviceClass
			return
		}
	}

	d.DeviceClass = sys.GetDiskDeviceClass(device)
}
