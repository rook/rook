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
	"strings"

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
	RestoreOSD            bool           // Restore OSD by reparing it with with OSD ID
}

func (m *DeviceOsdMapping) String() string {
	b, _ := json.Marshal(m)
	return string(b)
}

// deviceClass resolves the CRUSH device class for a configured device:
// per-device value first, node/cluster-level default otherwise. Returns "" when
// nothing is configured.
func (a *OsdAgent) deviceClass(d DesiredDevice) string {
	if d.DeviceClass != "" {
		return d.DeviceClass
	}
	return a.defaultDeviceClass()
}

// deviceClassByPath returns the CRUSH device class for an OSD identified by
// any of the given candidate paths (typically the block path plus its udev
// DevLinks), with the following priority:
//
//  1. the device-level configuration from the CR, matched against any candidate
//  2. the global or node-local default
//
// When multiple specs could match, the first configured device wins.
// Returns "" when nothing is configured.
func (a *OsdAgent) deviceClassByPath(paths []string) string {
	for _, d := range a.devices {
		if d.IsFilter || d.IsDevicePathFilter {
			continue
		}
		normSpec := strings.TrimPrefix(d.Name, "/dev/")
		for _, p := range paths {
			if strings.TrimPrefix(p, "/dev/") == normSpec {
				return a.deviceClass(d)
			}
		}
	}
	return a.defaultDeviceClass()
}

// defaultDeviceClass returns the node or cluster-level default CRUSH class.
// PVC-backed prepare jobs read it from ROOK_OSD_CRUSH_DEVICE_CLASS (injected
// by the operator); non-PVC jobs read it from a.storeConfig.DeviceClass.
func (a *OsdAgent) defaultDeviceClass() string {
	if a.pvcBacked {
		return os.Getenv(oposd.CrushDeviceClassVarName)
	}
	return a.storeConfig.DeviceClass
}
