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

import (
	"encoding/json"
	"path"
	"testing"

	"github.com/rook/rook/pkg/util"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/rook/rook/pkg/util/sys"
	"github.com/stretchr/testify/assert"
)

func TestSerializeClusterDisks(t *testing.T) {
	nodeID := "df1c87e8266843f2ab822c0d72f584d3"
	etcdClient := &util.MockEtcdClient{}
	d1 := &LocalDisk{Name: "sda", UUID: "u1", Size: 23, Rotational: true, Readonly: false,
		FileSystem: "btrfs", MountPoint: "/mnt/abc", Type: sys.DiskType, HasChildren: true}
	d1.Empty = getDeviceEmpty(d1)
	d2 := &LocalDisk{Name: "sdb", UUID: "u2", Size: 24, Rotational: true, Readonly: false,
		Type: sys.DiskType, HasChildren: true}
	d2.Empty = getDeviceEmpty(d2)

	err := storeDevices(etcdClient, nodeID, []*LocalDisk{d1, d2})
	assert.Nil(t, err)

	key := path.Join(NodesConfigKey, nodeID, disksKey)
	rawDisk := etcdClient.GetValue(key)
	var disks []*Disk
	err = json.Unmarshal([]byte(rawDisk), &disks)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(disks))
	assert.False(t, disks[0].Empty)
	assert.True(t, disks[0].Rotational)
	assert.Equal(t, d1.Size, disks[0].Size)
	assert.Equal(t, d1.Type, disks[0].Type)

	assert.True(t, disks[1].Empty)
	assert.True(t, disks[1].Rotational)
	assert.Equal(t, d2.Size, disks[1].Size)
	assert.Equal(t, d2.Type, disks[1].Type)
}

func TestAvailableDisks(t *testing.T) {

	// no disks discovered for a node is an error
	disks := GetAvailableDevices([]*LocalDisk{})
	assert.Equal(t, 0, len(disks))

	// no available disks because of the formatting
	d1 := &LocalDisk{Name: "sda", UUID: "myuuid1", Size: 123, Rotational: true, Readonly: false, FileSystem: "btrfs", MountPoint: "/mnt/abc", Type: sys.DiskType, HasChildren: true}
	disks = GetAvailableDevices([]*LocalDisk{d1})
	assert.Equal(t, 0, len(disks))

	// multiple available disks
	d2 := &LocalDisk{Name: "sdb", UUID: "myuuid2", Size: 123, Rotational: true, Readonly: false, Type: sys.DiskType, HasChildren: true}
	d3 := &LocalDisk{Name: "sdc", UUID: "myuuid3", Size: 123, Rotational: true, Readonly: false, Type: sys.DiskType, HasChildren: true}
	disks = GetAvailableDevices([]*LocalDisk{d1, d2, d3})

	assert.Equal(t, 2, len(disks))
	assert.Equal(t, "sdb", disks[0])
	assert.Equal(t, "sdc", disks[1])

	// partitions don't result in more available devices
	d4 := &LocalDisk{Name: "sdb1", UUID: "myuuid4", Size: 123, Rotational: true, Readonly: false, Type: sys.PartType, HasChildren: true}
	d5 := &LocalDisk{Name: "sdb2", UUID: "myuuid5", Size: 123, Rotational: true, Readonly: false, Type: sys.PartType, HasChildren: true}
	disks = GetAvailableDevices([]*LocalDisk{d1, d2, d3, d4, d5})
	assert.Equal(t, 2, len(disks))
	assert.Equal(t, "sdb", disks[0])
	assert.Equal(t, "sdc", disks[1])
}

func TestDiscoverDevices(t *testing.T) {
	executor := &exectest.MockExecutor{
		MockExecuteCommand: func(debug bool, actionName string, command string, arg ...string) error {
			logger.Infof("mock execute. %s. %s", actionName, command)
			return nil
		},
		MockExecuteCommandWithOutput: func(debug bool, actionName string, command string, arg ...string) (string, error) {
			logger.Infof("mock execute with output. %s. %s", actionName, command)
			return "", nil
		},
	}
	devices, err := discoverDevices(executor)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(devices))
}

func TestIgnoreDevice(t *testing.T) {
	cases := map[string]bool{
		"rbd0":    true,
		"rbd2":    true,
		"rbd9913": true,
		"rbd32p1": true,
		"rbd0a2":  false,
		"rbd":     false,
		"arbd0":   false,
		"rbd0x":   false,
	}
	for dev, expected := range cases {
		assert.Equal(t, expected, ignoreDevice(dev), dev)
	}
}
