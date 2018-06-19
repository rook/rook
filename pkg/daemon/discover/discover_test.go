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
	"testing"

	"github.com/rook/rook/pkg/clusterd"
	exectest "github.com/rook/rook/pkg/util/exec/test"

	"github.com/stretchr/testify/assert"
)

const (
	udevOutput = `DEVLINKS=/dev/disk/by-id/scsi-36001405d27e5d898829468b90ce4ef8c /dev/disk/by-id/wwn-0x6001405d27e5d898829468b90ce4ef8c /dev/disk/by-path/ip-127.0.0.1:3260-iscsi-iqn.2016-06.world.srv:storage.target01-lun-0 /dev/disk/by-uuid/f2d38cba-37da-411d-b7ba-9a6696c58174
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
	sgdiskOutput = `Disk /dev/sdb: 20971520 sectors, 10.0 GiB
Logical sector size: 512 bytes
Disk identifier (GUID): 819C2F95-7015-438F-A624-D40DBA2C2069
Partition table holds up to 128 entries
`
)

func TestProbeDevices(t *testing.T) {
	// set up mock execute so we can verify the partitioning happens on sda
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(debug bool, name string, command string, args ...string) (string, error) {
		logger.Infof("RUN Command for '%s'. %s arg %+v", name, command, args)
		output := ""
		switch name {
		case "lsblk all":
			output = "testa"
		case "lsblk /dev/testa":
			output = `SIZE="249510756352" ROTA="1" RO="0" TYPE="disk" PKNAME=""`
		case "get filesystem type for testa":
			output = udevOutput
		case "get parent for device testa":
			output = `       testa
testa    testa1
testa    testa2
testa2   centos_host13-root
testa2   centos_host13-swap
testa2   centos_host13-home
`
		case "get disk testa uuid":
			output = sgdiskOutput

		case "get disk testa fs serial":
			output = udevOutput
		}

		return output, nil
	}

	context := &clusterd.Context{Executor: executor}

	devices, err := probeDevices(context)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(devices))
	assert.Equal(t, "ext2", devices[0].Filesystem)

}
