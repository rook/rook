/*
Copyright 2018 The Rook Authors. All rights reserved.

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

// Package discover to discover unused devices.
package discover

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/rook/rook/pkg/clusterd"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/rook/rook/pkg/util/sys"

	"github.com/stretchr/testify/assert"
)

func TestProbeDevices(t *testing.T) {
	var (
		listDevicesArgs = []string{"--all", "--noheadings", "--list", "--output", "KNAME"}

		// testa: single disk with FS
		// testb1: first partition of disk testb
		listDevicesOutput = "testa\ntestb\ntestb1"

		listDevicesChildArgs    = []string{"--noheadings", "--path", "--list", "--output", "NAME"}
		listDevicesChildOutputs = map[string]string{
			"/dev/testa":  "/dev/testa",
			"/dev/testb":  "/dev/testb\n/dev/testb1",
			"/dev/testb1": "/dev/testb1",
		}

		getDevicePropertiesArgs    = []string{"--bytes", "--nodeps", "--pairs", "--paths", "--output", "SIZE,ROTA,RO,TYPE,PKNAME,NAME,KNAME,MOUNTPOINT,FSTYPE"}
		getDevicePropertiesOutputs = map[string]string{
			"/dev/testa":  `SIZE="249510756352" ROTA="1" RO="0" TYPE="disk" PKNAME=""`,
			"/dev/testb":  `SIZE="3200000196608" ROTA="0" RO="0" TYPE="disk" PKNAME="" NAME="/dev/testb" KNAME="/dev/testb" MOUNTPOINT="" FSTYPE=""`,
			"/dev/testb1": `SIZE="3199999131136" ROTA="0" RO="0" TYPE="part" PKNAME="/dev/testb" NAME="/dev/testb1" KNAME="/dev/testb1" MOUNTPOINT="" FSTYPE=""`,
		}

		getUdevInfoArgs    = []string{"info", "--query=property"}
		getUdevInfoOutputs = map[string]string{
			"/dev/testa": `DEVLINKS=/dev/disk/by-id/scsi-36001405d27e5d898829468b90ce4ef8c /dev/disk/by-id/wwn-0x6001405d27e5d898829468b90ce4ef8c /dev/disk/by-path/ip-127.0.0.1:3260-iscsi-iqn.2016-06.world.srv:storage.target01-lun-0 /dev/disk/by-uuid/f2d38cba-37da-411d-b7ba-9a6696c58174
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
`,
			"/dev/testb1": `DEVPATH=/devices/pci0000:00/0000:00:02.2/0000:03:00.0/host0/target0:2:0/0:2:0:0/block/testb/testb1
	DEVNAME=/dev/testb1
	DEVTYPE=partition
	PARTN=1
	MAJOR=8
	MINOR=1
	SUBSYSTEM=block
	USEC_INITIALIZED=2439447
	SCSI_TPGS=0
	SCSI_TYPE=disk
	SCSI_VENDOR=DELL
	SCSI_VENDOR_ENC=DELL\x20\x20\x20\x20
	SCSI_MODEL=PERC_H710P
	SCSI_MODEL_ENC=PERC\x20H710P\x20\x20\x20\x20\x20\x20
	SCSI_REVISION=3.13
	SCSI_IDENT_SERIAL=00fc8e7907c0a26e26003b99f960f681
	SCSI_IDENT_LUN_NAA_REGEXT=6c81f660f9993b00266ea2c007798efc
	ID_SCSI=1
	ID_VENDOR=DELL
	ID_VENDOR_ENC=DELL\x20\x20\x20\x20
	ID_MODEL=PERC_H710P
	ID_MODEL_ENC=PERC\x20H710P\x20\x20\x20\x20\x20\x20
	ID_REVISION=3.13
	ID_TYPE=disk
	ID_WWN=0x6c81f660f9993b00266ea2c007798efc
	ID_WWN_WITH_EXTENSION=0x6c81f660f9993b00266ea2c007798efc
	ID_BUS=scsi
	ID_SERIAL=36c81f660f9993b00266ea2c007798efc
	ID_SERIAL_SHORT=6c81f660f9993b00266ea2c007798efc
	DM_MULTIPATH_DEVICE_PATH=0
	ID_SCSI_INQUIRY=1
	ID_PATH=pci-0000:03:00.0-scsi-0:2:0:0
	ID_PATH_TAG=pci-0000_03_00_0-scsi-0_2_0_0
	ID_PART_TABLE_UUID=57251e2b-1081-4215-b37d-076ba307c1fa
	ID_PART_TABLE_TYPE=gpt
	ID_PART_ENTRY_SCHEME=gpt
	ID_PART_ENTRY_UUID=a43b1740-4219-4e33-83a5-0fc87ffe39db
	ID_PART_ENTRY_TYPE=21686148-6449-6e6f-744e-656564454649
	ID_PART_ENTRY_NUMBER=1
	ID_PART_ENTRY_OFFSET=2048
	ID_PART_ENTRY_SIZE=2048
	ID_PART_ENTRY_DISK=8:0
	DEVLINKS=/dev/disk/by-path/pci-0000:03:00.0-scsi-0:2:0:0-part1 /dev/disk/by-partuuid/a43b1740-4219-4e33-83a5-0fc87ffe39db /dev/disk/by-id/scsi-36c81f660f9993b00266ea2c007798efc-part1 /dev/disk/by-id/scsi-SDELL_PERC_H710P_00fc8e7907c0a26e26003b99f960f681-part1 /dev/disk/by-id/wwn-0x6c81f660f9993b00266ea2c007798efc-part1
	TAGS=:systemd:
`,
		}
	)
	// set up mock execute so we can verify the partitioning happens on sda
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("RUN Command %s  %v", command, args)
		output := ""
		if reflect.DeepEqual(args, listDevicesArgs) {
			output = listDevicesOutput
		} else if reflect.DeepEqual(args[1:], getDevicePropertiesArgs) {
			output = getDevicePropertiesOutputs[args[0]]
		} else if reflect.DeepEqual(args[:len(args)-1], getUdevInfoArgs) {
			output = getUdevInfoOutputs[args[len(args)-1]]
		} else if reflect.DeepEqual(args[:len(args)-1], listDevicesChildArgs) {
			output = listDevicesChildOutputs[args[len(args)-1]]
		}
		return output, nil
	}

	context := &clusterd.Context{Executor: executor}

	devices, err := probeDevices(context)
	assert.Nil(t, err)

	// the /dev/testb should be filtered out in favor of its partitions
	assert.Equal(t, 2, len(devices))
	assert.Equal(t, "ext2", devices[0].Filesystem)
	assert.Equal(t, false, devices[0].Empty)
	assert.Equal(t, "testb1", devices[1].Name)
	assert.Equal(t, "", devices[1].Filesystem)
	assert.Equal(t, true, devices[1].Empty)
}

func TestMatchUdevMonitorFiltering(t *testing.T) {
	// f <- matching function as configured
	f := func(text string) bool {
		take, err := matchUdevEvent(text, []string{"(?i)add", "(?i)remove"}, []string{"(?i)dm-[0-9]+"})
		assert.NoError(t, err)
		return take
	}

	// add events are emitted
	take := f("KERNEL[1008.734088] add      /devices/pci0000:00/0000:00:07.0/virtio5/block/vdc (block)")
	assert.True(t, take)

	// remove events are emitted
	take = f("KERNEL[1104.287884] remove   /devices/pci0000:00/0000:00:07.0/virtio5/block/vdc (block)")
	assert.True(t, take)

	// change events are ignored
	take = f("KERNEL[1136.069071] change   /devices/pci0000:00/0000:00:02.0/virtio0/block/vda/vda1 (block)")
	assert.False(t, take)

	// add events that match device mapper events are ignored
	take = f("KERNEL[1042.464238] add      /devices/virtual/block/dm-1 (block)")
	assert.False(t, take)
}

func TestDeviceListsEqual(t *testing.T) {
	// empty lists are equal
	assert.True(t, checkDeviceListsEqual(
		[]sys.LocalDisk{},
		[]sys.LocalDisk{},
	))

	// default constructed LocalDisks are equal
	assert.True(t, checkDeviceListsEqual(
		[]sys.LocalDisk{
			{},
		},
		[]sys.LocalDisk{
			{},
		},
	))

	// a disk is removed
	assert.False(t, checkDeviceListsEqual(
		[]sys.LocalDisk{
			{},
		},
		[]sys.LocalDisk{},
	))

	// a disk is added
	assert.False(t, checkDeviceListsEqual(
		[]sys.LocalDisk{},
		[]sys.LocalDisk{
			{},
		},
	))

	// devices with usb keyword are ignored. the lists should be equal
	assert.True(t, checkDeviceListsEqual(
		[]sys.LocalDisk{
			{
				DevLinks: "xyzusbabc",
			},
		},
		[]sys.LocalDisk{},
	))

	// devices with usb keyword are ignored. the lists should be equal
	assert.True(t, checkDeviceListsEqual(
		[]sys.LocalDisk{},
		[]sys.LocalDisk{
			{
				DevLinks: "xyzusbabc",
			},
		},
	))

	// equal if uuid is equal
	assert.True(t, checkDeviceListsEqual(
		[]sys.LocalDisk{
			{
				UUID:   "u2",
				Serial: "xxx",
				Name:   "xxx",
			},
		},
		[]sys.LocalDisk{
			{
				UUID:   "u2",
				Serial: "s2",
				Name:   "n2",
			},
		},
	))

	// equal if serial is equal
	assert.True(t, checkDeviceListsEqual(
		[]sys.LocalDisk{
			{
				UUID:   "xxx",
				Serial: "s2",
				Name:   "xxx",
			},
		},
		[]sys.LocalDisk{
			{
				UUID:   "u2",
				Serial: "s2",
				Name:   "n2",
			},
		},
	))

	// equal if device name is equal
	assert.True(t, checkDeviceListsEqual(
		[]sys.LocalDisk{
			{
				UUID:   "xxx",
				Serial: "xxx",
				Name:   "n2",
			},
		},
		[]sys.LocalDisk{
			{
				UUID:   "u2",
				Serial: "s2",
				Name:   "n2",
			},
		},
	))

	// otherwise, not equal
	assert.False(t, checkDeviceListsEqual(
		[]sys.LocalDisk{
			{
				UUID:   "xxx",
				Serial: "xxx",
				Name:   "xxx",
			},
		},
		[]sys.LocalDisk{
			{
				UUID:   "u2",
				Serial: "s2",
				Name:   "n2",
			},
		},
	))

	// device equality ignores an empty serial
	assert.False(t, checkDeviceListsEqual(
		[]sys.LocalDisk{
			{
				UUID:   "xxx",
				Serial: "",
				Name:   "xxx",
			},
		},
		[]sys.LocalDisk{
			{
				UUID:   "u2",
				Serial: "",
				Name:   "n2",
			},
		},
	))

	// devices are the same, but transition from non-empty to empty. in this
	// case we consider the lists to be non-equal (i.e. of interest to storage
	// providers).
	assert.False(t, checkDeviceListsEqual(
		[]sys.LocalDisk{
			{
				UUID:  "uuid",
				Empty: false,
			},
		},
		[]sys.LocalDisk{
			{
				UUID:  "uuid",
				Empty: true,
			},
		},
	))

	// devices are the same, but transition from empty to non-empty (e.g. the
	// dev is now in use). in this case we consider the lists to be equal (i.e.
	// no interesting change).
	assert.True(t, checkDeviceListsEqual(
		[]sys.LocalDisk{
			{
				UUID:  "uuid",
				Empty: true,
			},
		},
		[]sys.LocalDisk{
			{
				UUID:  "uuid",
				Empty: false,
			},
		},
	))

	// devices are the same, but the partition table is cleared. this would be
	// of interest to storage providers!
	assert.False(t, checkDeviceListsEqual(
		[]sys.LocalDisk{
			{
				UUID: "uuid",
				Partitions: []sys.Partition{
					{},
				},
			},
		},
		[]sys.LocalDisk{
			{
				UUID:       "uuid",
				Partitions: nil,
			},
		},
	))

	// devices are the same, but the partition table has been created. not so
	// interesting.
	assert.True(t, checkDeviceListsEqual(
		[]sys.LocalDisk{
			{
				UUID:       "uuid",
				Partitions: nil,
			},
		},
		[]sys.LocalDisk{
			{
				UUID: "uuid",
				Partitions: []sys.Partition{
					{},
				},
			},
		},
	))
}

func TestGetCephVolumeInventory(t *testing.T) {
	run := 0
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, arg ...string) (string, error) {
			run++
			logger.Infof("run %d command %s", run, command)
			switch {
			case run == 1:
				return `[{"available": true, "rejected_reasons": [], "sys_api": {"scheduler_mode": "noop",
"rotational": "0", "vendor": "ATA", "human_readable_size": "25.00 GB", "sectors": 0, "sas_device_handle": "",
"partitions": {}, "rev": "1.0", "sas_address": "", "locked": 0, "sectorsize": "512", "removable": "0", "path": "/dev/sdb",
"support_discard": "", "model": "VBOX SMART HARDDISK", "ro": "0", "nr_requests": "128", "size": 26843545600.0},
"lvs": [], "path": "/dev/sdb"}, {"available": false, "rejected_reasons": ["locked"], "sys_api": {"scheduler_mode": "noop",
 "rotational": "1", "vendor": "ATA", "human_readable_size": "32.00 GB", "sectors": 0, "sas_device_handle": "",
 "partitions": {"sda2": {"start": "2099200", "holders": ["dm-0", "dm-1"], "sectorsize": 512, "sectors": "65009664",
 "size": "31.00 GB"}, "sda1": {"start": "2048", "holders": [], "sectorsize": 512, "sectors": "2097152", "size": "1024.00 MB"}},
 "rev": "1.0", "sas_address": "", "locked": 1, "sectorsize": "512", "removable": "0", "path": "/dev/sda", "support_discard": "",
 "model": "VBOX HARDDISK", "ro": "0", "nr_requests": "128", "size": 34359738368.0}, "lvs": [{"comment": "not used by ceph", "name": "swap"},
  {"comment": "not used by ceph", "name": "root"}], "path": "/dev/sda"}]
				`, nil
			case run == 2: // No data returned from Ceph Volume
				return ``, nil
			case run == 3: // No devices returned from Ceph Volume
				return `[]`, nil
			case run == 4: // Error executing Ceph Volume
				return ``, fmt.Errorf("unexplainable error")
			case run == 5: // A device without sys_api data
				return `[{"available": true }]`, nil
			}
			return "", nil
		},
	}

	context := &clusterd.Context{Executor: executor}

	dev_sda := `{"path":"/dev/sda","available":false,"rejected_reasons":["locked"],"sys_api":{"scheduler_mode":"noop","rotational":"1","vendor":"ATA","human_readable_size":"32.00 GB","sectors":0,"sas_device_handle":"","partitions":{"sda2":{"start":"2099200","holders":["dm-0","dm-1"],"sectorsize":512,"sectors":"65009664","size":"31.00 GB"},"sda1":{"start":"2048","holders":[],"sectorsize":512,"sectors":"2097152","size":"1024.00 MB"}},"rev":"1.0","sas_address":"","locked":1,"sectorsize":"512","removable":"0","path":"/dev/sda","support_discard":"","model":"VBOX HARDDISK","ro":"0","nr_requests":"128","size":34359738368.0},"lvs":[{"comment":"not used by ceph","name":"swap"},{"comment":"not used by ceph","name":"root"}]}`
	dev_sdb := `{"path":"/dev/sdb","available":true,"rejected_reasons":[],"sys_api":{"scheduler_mode":"noop","rotational":"0","vendor":"ATA","human_readable_size":"25.00 GB","sectors":0,"sas_device_handle":"","partitions":{},"rev":"1.0","sas_address":"","locked":0,"sectorsize":"512","removable":"0","path":"/dev/sdb","support_discard":"","model":"VBOX SMART HARDDISK","ro":"0","nr_requests":"128","size":26843545600.0},"lvs":[]}`

	// Normal execution
	cvdata, err := getCephVolumeInventory(context)

	assert.Nil(t, err)
	assert.Equal(t, len(*cvdata), 2)
	assert.Equal(t, (*cvdata)["/dev/sda"], dev_sda)
	assert.Equal(t, (*cvdata)["/dev/sdb"], dev_sdb)

	// No data returned from Ceph Volume
	cvdata, err = getCephVolumeInventory(context)
	assert.Nil(t, err)
	assert.Equal(t, len(*cvdata), 0)

	// No devices returned from Ceph Volume
	cvdata, err = getCephVolumeInventory(context)
	assert.Nil(t, err)
	assert.Equal(t, len(*cvdata), 0)

	// Error executing Ceph Volume
	cvdata, err = getCephVolumeInventory(context)
	assert.Error(t, err, "unexplainable error")
	assert.Nil(t, cvdata, 0)

	// // A device without sys_api data
	cvdata, err = getCephVolumeInventory(context)
	assert.Nil(t, err)
	assert.Equal(t, len(*cvdata), 1)
}
