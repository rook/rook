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
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/rook/rook/pkg/util/sys"
	"github.com/stretchr/testify/assert"
)

const udevFSOutput = `
DEVNAME=/dev/sdk
DEVPATH=/devices/platform/host6/session2/target6:0:0/6:0:0:0/block/sdk
DEVTYPE=disk
ID_BUS=scsi
ID_FS_TYPE=ext2
ID_FS_USAGE=filesystem
ID_FS_UUID=f2d38cba-37da-411d-b7ba-9a6696c58174
ID_FS_UUID_ENC=f2d38cba-37da-411d-b7ba-9a6696c58174
ID_FS_VERSION=1.0
ID_MODEL=disk01
ID_MODEL_ENC=disk01\x20\x20\x20\x20\x20\x20\x20\x20\x20\x20
ID_PATH=ip-127.0.0.1:3260-iscsi-iqn.2016-06.world.srv:storage.target01-lun-0
ID_PATH_TAG=ip-127_0_0_1_3260-iscsi-iqn_2016-06_world_srv_storage_target01-lun-0
ID_REVISION=4.0
ID_SCSI=1
ID_SCSI_SERIAL=d27e5d89-8829-468b-90ce-4ef8c02f07fe
ID_SERIAL=36001405d27e5d898829468b90ce4ef8c
ID_SERIAL_SHORT=6001405d27e5d898829468b90ce4ef8c
ID_TARGET_PORT=0
ID_TYPE=disk
ID_VENDOR=LIO-ORG
ID_VENDOR_ENC=LIO-ORG\x20
ID_WWN=0x6001405d27e5d898
ID_WWN_VENDOR_EXTENSION=0x829468b90ce4ef8c
ID_WWN_WITH_EXTENSION=0x6001405d27e5d898829468b90ce4ef8c
MAJOR=8
MINOR=160
SUBSYSTEM=block
TAGS=:systemd:
USEC_INITIALIZED=15981915740802
`

func TestRunDaemon(t *testing.T) {
	configDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(configDir)
	os.MkdirAll(configDir, 0755)

	defBkp := cephconfig.DefaultConfigDir
	cephconfig.DefaultConfigDir = configDir
	defer func() { cephconfig.DefaultConfigDir = defBkp }()

	agent, _, context := createTestAgent(t, "none", configDir, "node5375", &config.StoreConfig{StoreType: config.Bluestore})
	agent.devices[0].IsFilter = true

	agent.pvcBacked = false
	logger.Infof("Agent %+v", agent)
	crushLocation := "root=default host=foo"
	err := Provision(context, agent, crushLocation)
	assert.Nil(t, err)
}

func TestGetDataDirs(t *testing.T) {
	configDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(configDir)
	context := &clusterd.Context{ConfigDir: configDir}
	defer os.RemoveAll(context.ConfigDir)
	os.MkdirAll(context.ConfigDir, 0755)

	kv := mockKVStore()
	nodeName := "node6046"

	// user has specified devices to use, no dirs should be returned
	dirMap, err := getDataDirs(context, kv, "", true, nodeName)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(dirMap))

	// user has no devices specified, should NO LONGER return default dir
	dirMap, err = getDataDirs(context, kv, "", false, nodeName)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(dirMap))

	// user has no devices specified but does specify dirs, those should be returned
	dirMap, err = getDataDirs(context, kv, "/rook/dir1", false, nodeName)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(dirMap))
	assert.Equal(t, unassignedOSDID, dirMap["/rook/dir1"])

	// user has devices specified and also specifies dirs, those should be returned
	dirMap, err = getDataDirs(context, kv, "/rook/dir1", true, nodeName)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(dirMap))
	assert.Equal(t, unassignedOSDID, dirMap["/rook/dir1"])

	// simulate an OSD ID being assigned to the dir
	dirMap["/rook/dir1"] = 1
	// save the directory config
	err = config.SaveOSDDirMap(kv, nodeName, dirMap)
	assert.Nil(t, err)

	// user has specified devices and also a new directory to use.  it should be added to the dir map
	dirMap, err = getDataDirs(context, kv, "/rook/dir1,/tmp/mydir", true, nodeName)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(dirMap))
	assert.Equal(t, 1, dirMap["/rook/dir1"])
	assert.Equal(t, unassignedOSDID, dirMap["/tmp/mydir"])

	// simulate that the user's dir got an OSD by assigning it an ID
	dirMap["/tmp/mydir"] = 23
	err = config.SaveOSDDirMap(kv, nodeName, dirMap)
	assert.Nil(t, err)

	// user is still specifying the 2 directories, we should get back their IDs
	dirMap, err = getDataDirs(context, kv, "/rook/dir1,/tmp/mydir", true, nodeName)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(dirMap))
	assert.Equal(t, 1, dirMap["/rook/dir1"])
	assert.Equal(t, 23, dirMap["/tmp/mydir"])

	// user is now only specifying 1 of the dirs, the other 1 should be returned as removed
	dirMap, err = getDataDirs(context, kv, "/rook/dir1", true, nodeName)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(dirMap))
	assert.Equal(t, 1, dirMap["/rook/dir1"])

	// clear the dir map and simulate the scenario where an OSD has been created in the default dir
	kv.ClearStore(config.GetConfigStoreName(nodeName))
	osdID := 9802
	dirMap = map[string]int{context.ConfigDir: osdID}
	err = config.SaveOSDDirMap(kv, nodeName, dirMap)
	assert.Nil(t, err)

	// when an OSD has been created in the default dir, no dirs are specified, and no devices are specified,
	// the default dir should still be in use (it should not come back as removed!)
	dirMap, err = getDataDirs(context, kv, "", false, nodeName)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(dirMap))
	assert.Equal(t, osdID, dirMap[context.ConfigDir])
}

func TestAvailableDevices(t *testing.T) {
	executor := &exectest.MockExecutor{}
	// set up a mock function to return "rook owned" partitions on the device and it does not have a filesystem
	executor.MockExecuteCommandWithOutput = func(debug bool, name string, command string, args ...string) (string, error) {
		logger.Infof("OUTPUT for %s. %s %+v", name, command, args)

		if command == "lsblk" {
			if strings.Index(name, "sdb") != -1 {
				// /dev/sdb has a partition
				return `NAME="sdb" SIZE="65" TYPE="disk" PKNAME=""
NAME="sdb1" SIZE="30" TYPE="part" PKNAME="sdb"`, nil
			}
			return "", nil
		} else if command == "blkid" {
			if strings.Index(name, "sdb1") != -1 {
				// partition sdb1 has a label MY-PART
				return "MY-PART", nil
			}
		} else if command == "udevadm" {
			if strings.Index(name, "sdc") != -1 {
				// /dev/sdc has a file system
				return udevFSOutput, nil
			}
			return "", nil
		}

		return "", errors.Errorf("unknown command %s %s", command, args)
	}

	context := &clusterd.Context{Executor: executor}
	context.Devices = []*sys.LocalDisk{
		{Name: "sda", DevLinks: "/dev/disk/by-id/scsi-0123 /dev/disk/by-path/pci-0:1:2:3-scsi-1"},
		{Name: "sdb", DevLinks: "/dev/disk/by-id/scsi-4567 /dev/disk/by-path/pci-4:5:6:7-scsi-1"},
		{Name: "sdc", DevLinks: "/dev/disk/by-id/scsi-89ab /dev/disk/by-path/pci-8:9:a:b-scsi-1"},
		{Name: "sdd", DevLinks: "/dev/disk/by-id/scsi-cdef /dev/disk/by-path/pci-c:d:e:f-scsi-1"},
		{Name: "nvme01", DevLinks: "/dev/disk/by-id/nvme-0246 /dev/disk/by-path/pci-0:2:4:6-nvme-1"},
		{Name: "rda"},
		{Name: "rdb"},
	}

	// select all devices, including nvme01 for metadata
	pvcBackedOSD := false
	mapping, err := getAvailableDevices(context, []DesiredDevice{{Name: "all"}}, "nvme01", pvcBackedOSD)
	assert.Nil(t, err)
	assert.Equal(t, 5, len(mapping.Entries))
	assert.Equal(t, -1, mapping.Entries["sda"].Data)
	assert.Equal(t, -1, mapping.Entries["sdd"].Data)
	assert.Equal(t, -1, mapping.Entries["rda"].Data)
	assert.Equal(t, -1, mapping.Entries["rdb"].Data)
	assert.Equal(t, -1, mapping.Entries["nvme01"].Data)
	assert.NotNil(t, mapping.Entries["nvme01"].Metadata)
	assert.Equal(t, 0, len(mapping.Entries["nvme01"].Metadata))

	// select no devices both using and not using a filter
	mapping, err = getAvailableDevices(context, nil, "", pvcBackedOSD)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(mapping.Entries))

	mapping, err = getAvailableDevices(context, nil, "", pvcBackedOSD)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(mapping.Entries))

	// select the sd* devices
	mapping, err = getAvailableDevices(context, []DesiredDevice{{Name: "^sd.$", IsFilter: true}}, "", pvcBackedOSD)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(mapping.Entries))
	assert.Equal(t, -1, mapping.Entries["sda"].Data)
	assert.Equal(t, -1, mapping.Entries["sdd"].Data)

	// select an exact device
	mapping, err = getAvailableDevices(context, []DesiredDevice{{Name: "sdd"}}, "", pvcBackedOSD)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(mapping.Entries))
	assert.Equal(t, -1, mapping.Entries["sdd"].Data)

	// select all devices except those that have a prefix of "s"
	mapping, err = getAvailableDevices(context, []DesiredDevice{{Name: "^[^s]", IsFilter: true}}, "", pvcBackedOSD)
	assert.Nil(t, err)
	assert.Equal(t, 3, len(mapping.Entries))
	assert.Equal(t, -1, mapping.Entries["rda"].Data)
	assert.Equal(t, -1, mapping.Entries["rdb"].Data)
	assert.Equal(t, -1, mapping.Entries["nvme01"].Data)

	// select the sd* devices by path names
	mapping, err = getAvailableDevices(context, []DesiredDevice{{Name: "^/dev/sd.$", IsDevicePathFilter: true}}, "", pvcBackedOSD)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(mapping.Entries))
	assert.Equal(t, -1, mapping.Entries["sda"].Data)
	assert.Equal(t, -1, mapping.Entries["sdd"].Data)

	// select the SCSI devices
	mapping, err = getAvailableDevices(context, []DesiredDevice{{Name: "^/dev/disk/by-path/.*-scsi-.*", IsDevicePathFilter: true}}, "", pvcBackedOSD)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(mapping.Entries))
	assert.Equal(t, -1, mapping.Entries["sda"].Data)
	assert.Equal(t, -1, mapping.Entries["sdd"].Data)
}

func TestGetVolumeGroupName(t *testing.T) {
	validLVPath := "/dev/vgName1/lvName2"
	invalidLVPath1 := "/dev//vgName2"
	invalidLVPath2 := "/dev/"

	vgName, err := getVolumeGroupName(validLVPath)
	assert.Nil(t, err)
	assert.Equal(t, vgName, "vgName1")

	vgName, err = getVolumeGroupName(invalidLVPath1)
	assert.NotNil(t, err)
	assert.Equal(t, vgName, "")

	vgName, err = getVolumeGroupName(invalidLVPath2)
	assert.NotNil(t, err)
	assert.Equal(t, vgName, "")
}
