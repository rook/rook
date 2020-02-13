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
	"strings"
	"testing"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/rook/rook/pkg/util/sys"
	"github.com/stretchr/testify/assert"
)

const (
	udevFSOutput = `
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

	cvInventoryOutputAvailable = `
	{
		"available":true,
		"lvs":[

		],
		"rejected_reasons":[
		   ""
		],
		"sys_api":{
		   "size":10737418240.0,
		   "scheduler_mode":"mq-deadline",
		   "rotational":"0",
		   "vendor":"",
		   "human_readable_size":"10.00 GB",
		   "sectors":0,
		   "sas_device_handle":"",
		   "rev":"",
		   "sas_address":"",
		   "locked":0,
		   "sectorsize":"512",
		   "removable":"0",
		   "path":"/dev/sdb",
		   "support_discard":"0",
		   "model":"",
		   "ro":"0",
		   "nr_requests":"64",
		   "partitions":{

		   }
		},
		"path":"/dev/sdb",
		"device_id":""
	 }
	 `

	cvInventoryOutputNotAvailableBluestoreLabel = `
	{
		"available":false,
		"lvs":[

		],
		"rejected_reasons":[
		   "Has BlueStore device label"
		]
	 }
	`

	cvInventoryOutputNotAvailableLocked = `
	{
		"available":false,
		"lvs":[

		],
		"rejected_reasons":[
		   "locked"
		]
	 }
	 `

	cvInventoryOutputNotAvailableSmall = `
	{
		"available":false,
		"lvs":[

		],
		"rejected_reasons":[
			["Insufficient space (<5GB)"]
		]
	 }
	 `
)

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
		} else if command == "ceph-volume" {
			if strings.Index(args[3], "/mnt/set1-0-data-qfhfk") != -1 {
				return cvInventoryOutputNotAvailableBluestoreLabel, nil
			} else if strings.Index(args[3], "sdb") != -1 {
				// sdb is locked
				return cvInventoryOutputNotAvailableLocked, nil
			} else if strings.Index(args[3], "sdc") != -1 {
				// sdc is too small
				return cvInventoryOutputNotAvailableSmall, nil
			}

			return cvInventoryOutputAvailable, nil
		}

		return "", errors.Errorf("unknown command %s %s", command, args)
	}

	context := &clusterd.Context{Executor: executor}
	context.Devices = []*sys.LocalDisk{
		{Name: "sda", DevLinks: "/dev/disk/by-id/scsi-0123 /dev/disk/by-path/pci-0:1:2:3-scsi-1", RealName: "sda"},
		{Name: "sdb", DevLinks: "/dev/disk/by-id/scsi-4567 /dev/disk/by-path/pci-4:5:6:7-scsi-1", RealName: "sdb"},
		{Name: "sdc", DevLinks: "/dev/disk/by-id/scsi-89ab /dev/disk/by-path/pci-8:9:a:b-scsi-1", RealName: "sdc"},
		{Name: "sdd", DevLinks: "/dev/disk/by-id/scsi-cdef /dev/disk/by-path/pci-c:d:e:f-scsi-1", RealName: "sdd"},
		{Name: "sde", DevLinks: "/dev/disk/by-id/sde-0x0000 /dev/disk/by-path/pci-0000:00:18.0-ata-1", RealName: "sde"},
		{Name: "nvme01", DevLinks: "/dev/disk/by-id/nvme-0246 /dev/disk/by-path/pci-0:2:4:6-nvme-1", RealName: "nvme01"},
		{Name: "rda", RealName: "rda"},
		{Name: "rdb", RealName: "rdb"},
		{Name: "/mnt/set1-0-data-qfhfk", RealName: "xvdcy", Type: "data"},
		{Name: "sdt1", RealName: "sdt1", Type: sys.PartType},
	}

	version := cephver.Octopus

	// select all devices, including nvme01 for metadata
	pvcBackedOSD := false
	mapping, err := getAvailableDevices(context, []DesiredDevice{{Name: "all"}}, "nvme01", pvcBackedOSD, version)
	assert.Nil(t, err)
	assert.Equal(t, 7, len(mapping.Entries))
	assert.Equal(t, -1, mapping.Entries["sda"].Data)
	assert.Equal(t, -1, mapping.Entries["sdd"].Data)
	assert.Equal(t, -1, mapping.Entries["rda"].Data)
	assert.Equal(t, -1, mapping.Entries["rdb"].Data)
	assert.Equal(t, -1, mapping.Entries["nvme01"].Data)
	assert.NotNil(t, mapping.Entries["nvme01"].Metadata)
	assert.Equal(t, 0, len(mapping.Entries["nvme01"].Metadata))

	// Partition is skipped
	version = cephver.Nautilus
	mapping, err = getAvailableDevices(context, []DesiredDevice{{Name: "all"}}, "nvme01", pvcBackedOSD, version)
	assert.Nil(t, err)
	assert.Equal(t, 6, len(mapping.Entries))

	// Do not skip partition anymore
	version = cephver.Octopus

	// select no devices both using and not using a filter
	mapping, err = getAvailableDevices(context, nil, "", pvcBackedOSD, version)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(mapping.Entries))

	mapping, err = getAvailableDevices(context, nil, "", pvcBackedOSD, version)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(mapping.Entries))

	// select the sd* devices
	mapping, err = getAvailableDevices(context, []DesiredDevice{{Name: "^sd.$", IsFilter: true}}, "", pvcBackedOSD, version)
	assert.Nil(t, err)
	assert.Equal(t, 3, len(mapping.Entries))
	assert.Equal(t, -1, mapping.Entries["sda"].Data)
	assert.Equal(t, -1, mapping.Entries["sdd"].Data)

	// select an exact device
	mapping, err = getAvailableDevices(context, []DesiredDevice{{Name: "sdd"}}, "", pvcBackedOSD, version)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(mapping.Entries))
	assert.Equal(t, -1, mapping.Entries["sdd"].Data)

	// select all devices except those that have a prefix of "s"
	mapping, err = getAvailableDevices(context, []DesiredDevice{{Name: "^[^s]", IsFilter: true}}, "", pvcBackedOSD, version)
	assert.Nil(t, err)
	assert.Equal(t, 3, len(mapping.Entries))
	assert.Equal(t, -1, mapping.Entries["rda"].Data)
	assert.Equal(t, -1, mapping.Entries["rdb"].Data)
	assert.Equal(t, -1, mapping.Entries["nvme01"].Data)

	// select the sd* devices by path names
	mapping, err = getAvailableDevices(context, []DesiredDevice{{Name: "^/dev/sd.$", IsDevicePathFilter: true}}, "", pvcBackedOSD, version)
	assert.Nil(t, err)
	assert.Equal(t, 3, len(mapping.Entries))
	assert.Equal(t, -1, mapping.Entries["sda"].Data)
	assert.Equal(t, -1, mapping.Entries["sdd"].Data)

	// select the SCSI devices
	mapping, err = getAvailableDevices(context, []DesiredDevice{{Name: "^/dev/disk/by-path/.*-scsi-.*", IsDevicePathFilter: true}}, "", pvcBackedOSD, version)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(mapping.Entries))
	assert.Equal(t, -1, mapping.Entries["sda"].Data)
	assert.Equal(t, -1, mapping.Entries["sdd"].Data)

	// select a device by explicit link
	mapping, err = getAvailableDevices(context, []DesiredDevice{{Name: "/dev/disk/by-id/sde-0x0000"}}, "", pvcBackedOSD, version)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(mapping.Entries))
	assert.Equal(t, -1, mapping.Entries["sde"].Data)

	// test on PVC
	pvcBackedOSD = true
	mapping, err = getAvailableDevices(context, []DesiredDevice{{Name: "all"}}, "", pvcBackedOSD, version)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(mapping.Entries), mapping)
}

func TestGetVolumeGroupName(t *testing.T) {
	validLVPath := "/dev/vgName1/lvName2"
	invalidLVPath1 := "/dev//vgName2"
	invalidLVPath2 := "/dev/"

	vgName := getVolumeGroupName(validLVPath)
	assert.Equal(t, vgName, "vgName1")

	vgName = getVolumeGroupName(invalidLVPath1)
	assert.Equal(t, vgName, "")

	vgName = getVolumeGroupName(invalidLVPath2)
	assert.Equal(t, vgName, "")
}
