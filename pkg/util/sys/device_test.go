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
package sys

import (
	"fmt"
	"testing"

	"github.com/pkg/errors"
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
	udevPartOutput = `ID_PART_ENTRY_DISK=8:32
ID_PART_ENTRY_NAME=%s
ID_PART_ENTRY_NUMBER=3
ID_PART_ENTRY_OFFSET=3278848
ID_PART_ENTRY_SCHEME=gpt
ID_PART_ENTRY_SIZE=7206879
ID_PART_ENTRY_TYPE=0fc63daf-8483-4772-8e79-3d69d8477de4
ID_PART_ENTRY_UUID=2089640e-bdeb-4fb4-aaec-88e165780b88
ID_PART_TABLE_TYPE=gpt
ID_PART_TABLE_UUID=46242f96-6cf7-4e5d-b4bd-9d046e6ad920
ID_REVISION=4.0
ID_SCSI=1
ID_SCSI_SERIAL=68c0bd28-d4ee-4376-9387-c9f02c53b3f2
ID_SERIAL=3600140568c0bd28d4ee43769387c9f02
ID_SERIAL_SHORT=600140568c0bd28d4ee43769387c9f02
ID_TARGET_PORT=0
ID_TYPE=disk
ID_VENDOR=LIO-ORG
ID_VENDOR_ENC=LIO-ORG\x20
ID_WWN=0x600140568c0bd28d
ID_WWN_VENDOR_EXTENSION=0x4ee43769387c9f02
ID_WWN_WITH_EXTENSION=0x600140568c0bd28d4ee43769387c9f02
MAJOR=8
MINOR=35
PARTN=3
PARTNAME=Linux filesystem
SUBSYSTEM=block
`
)

var (
	lsblkChildOutput = `NAME="ceph--cec981b8--2eca--45cd--bf91--a4472779f2a9-osd--data--428984b7--f94d--40cd--9cb7--1458e1613eab" MAJ:MIN="252:0" RM="0" SIZE="29G" RO="0" TYPE="lvm" MOUNTPOINT=""
NAME="vdb" MAJ:MIN="253:16" RM="0" SIZE="30G" RO="0" TYPE="disk" MOUNTPOINT=""
NAME="vdb1" MAJ:MIN="253:17" RM="0" SIZE="30G" RO="0" TYPE="part" MOUNTPOINT=""`
)

func TestFindUUID(t *testing.T) {
	output := `Disk /dev/sdb: 10485760 sectors, 5.0 GiB
Logical sector size: 512 bytes
Disk identifier (GUID): 31273B25-7B2E-4D31-BAC9-EE77E62EAC71
Partition table holds up to 128 entries
First usable sector is 34, last usable sector is 10485726
Partitions will be aligned on 2048-sector boundaries
Total free space is 20971453 sectors (10.0 GiB)
`
	uuid, err := parseUUID("sdb", output)
	assert.Nil(t, err)
	assert.Equal(t, "31273b25-7b2e-4d31-bac9-ee77e62eac71", uuid)
}

func TestParseFileSystem(t *testing.T) {
	output := udevOutput

	result := parseFS(output)
	assert.Equal(t, "ext2", result)
}

func TestGetPartitions(t *testing.T) {
	run := 0
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, arg ...string) (string, error) {
			run++
			logger.Infof("run %d command %s", run, command)
			switch {
			case run == 1:
				return `NAME="sdc" SIZE="100000" TYPE="disk" PKNAME=""`, nil
			case run == 2:
				return `NAME="sdb" SIZE="65" TYPE="disk" PKNAME=""
NAME="sdb2" SIZE="10" TYPE="part" PKNAME="sdb"
NAME="sdb3" SIZE="20" TYPE="part" PKNAME="sdb"
NAME="sdb1" SIZE="30" TYPE="part" PKNAME="sdb"`, nil
			case run == 3:
				return fmt.Sprintf(udevPartOutput, "ROOK-OSD0-DB"), nil
			case run == 4:
				return fmt.Sprintf(udevPartOutput, "ROOK-OSD0-BLOCK"), nil
			case run == 5:
				return fmt.Sprintf(udevPartOutput, "ROOK-OSD0-WAL"), nil
			case run == 6:
				return `NAME="sda" SIZE="19818086400" TYPE="disk" PKNAME=""
NAME="sda4" SIZE="1073741824" TYPE="part" PKNAME="sda"
NAME="sda2" SIZE="2097152" TYPE="part" PKNAME="sda"
NAME="sda9" SIZE="17328766976" TYPE="part" PKNAME="sda"
NAME="sda7" SIZE="67108864" TYPE="part" PKNAME="sda"
NAME="sda3" SIZE="1073741824" TYPE="part" PKNAME="sda"
NAME="usr" SIZE="1065345024" TYPE="crypt" PKNAME="sda3"
NAME="sda1" SIZE="134217728" TYPE="part" PKNAME="sda"
NAME="sda6" SIZE="134217728" TYPE="part" PKNAME="sda"`, nil
			case run == 14:
				return `NAME="dm-0" SIZE="100000" TYPE="lvm" PKNAME=""
NAME="ceph--89fa04fa--b93a--4874--9364--c95be3ec01c6-osd--data--70847bdb--2ec1--4874--98ba--d87d4860a70d" SIZE="31138512896" TYPE="lvm" PKNAME=""`, nil
			}
			return "", nil
		},
	}

	partitions, unused, err := GetDevicePartitions("sdc", executor)
	assert.Nil(t, err)
	assert.Equal(t, uint64(100000), unused)
	assert.Equal(t, 0, len(partitions))

	partitions, unused, err = GetDevicePartitions("sdb", executor)
	assert.Nil(t, err)
	assert.Equal(t, uint64(5), unused)
	assert.Equal(t, 3, len(partitions))
	assert.Equal(t, uint64(10), partitions[0].Size)
	assert.Equal(t, "ROOK-OSD0-DB", partitions[0].Label)
	assert.Equal(t, "sdb2", partitions[0].Name)

	partitions, unused, err = GetDevicePartitions("sda", executor)
	assert.Nil(t, err)
	assert.Equal(t, uint64(0x400000), unused)
	assert.Equal(t, 7, len(partitions))

	partitions, _, err = GetDevicePartitions("dm-0", executor)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(partitions))

	partitions, _, err = GetDevicePartitions("sdx", executor)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(partitions))
}

func TestParseUdevInfo(t *testing.T) {
	m := parseUdevInfo(udevOutput)
	assert.Equal(t, m["ID_FS_TYPE"], "ext2")
}

func TestListDevicesChildListDevicesChild(t *testing.T) {
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, arg ...string) (string, error) {
			logger.Infof("command %s", command)
			return lsblkChildOutput, nil
		},
	}

	device := "/dev/vdb"
	child, err := ListDevicesChild(executor, device)
	assert.NoError(t, err)
	assert.Equal(t, 3, len(child))
}

func TestPartitionTableType(t *testing.T) {
	type execOut struct {
		str string
		err error
	}
	type testCase struct {
		name    string
		mockOut execOut
		expect  string
		// basic check to see if some text exists in the error itself; "" means do not want error
		expectErrText string
	}
	tests := []testCase{
		{name: "whole disk",
			// example /dev/sda on ubuntu vm
			mockOut: execOut{str: `BYT;
/dev/sda:137GB:scsi:512:512:msdos:ATA VBOX HARDDISK:;
1:1049kB:512MB:511MB:ext4::boot;
2:512MB:2560MB:2048MB:linux-swap(v1)::;
3:2560MB:137GB:135GB:ext4::;`, err: nil},
			expect: "msdos", expectErrText: ""},
		{name: "loop partition",
			// example /dev/sda2 from above
			mockOut: execOut{str: `BYT;
/dev/sda2:2048MB:unknown:512:512:loop:Unknown:;
1:0.00B:2048MB:2048MB:linux-swap(v1)::;`, err: nil},
			expect: "loop", expectErrText: ""},
		{name: "atari partition",
			// example from manually-created atari partition
			mockOut: execOut{str: `BYT;
/dev/sdb1:1509kB:unknown:512:512:atari:Unknown:;`, err: nil},
			expect: "atari", expectErrText: ""},
		{name: "gpt partition",
			mockOut: execOut{str: `BYT;
/dev/sdd2:5369MB:unknown:512:512:gpt:Unknown:;`, err: nil},
			expect: "gpt", expectErrText: ""},
		{name: "unknown partition",
			// parted returns an error when it finds an unknown partition type
			mockOut: execOut{str: `BYT;
/dev/sdc1:10.7GB:unknown:512:512:unknown:Unknown:;`, err: fmt.Errorf("fake partition type unknown error")},
			expect: "unknown", expectErrText: ""},
		{name: "too few fields",
			mockOut: execOut{str: `BYT;
/dev/sdc1:Unknown:;`, err: nil /* assume this case would have no error */},
			expect: "", expectErrText: "fields"},
		{name: "only one line of output",
			mockOut: execOut{str: `BYT;`, err: fmt.Errorf("fake error where parted doesn't output part info")},
			expect:  "", expectErrText: "lines"},
		{name: "no lines of output",
			mockOut: execOut{str: ``, err: fmt.Errorf("fake error where parted outputs nothing to stdout")},
			expect:  "", expectErrText: "lines"},
		{name: "no semicolon output",
			mockOut: execOut{str: `BYT;
there:is:not:a:semicolon:after:this:line:`, err: nil /* assume this case would have no error */},
			expect: "", expectErrText: "semicolon"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// set up executor with test definition's mock output
			executor := &exectest.MockExecutor{
				MockExecuteCommandWithOutput: func(command string, arg ...string) (string, error) {
					t.Log("command:", command, arg)
					assert.Equal(t, "/dev/sdb1", arg[len(arg)-2]) // 2nd to last arg should be /dev/sdb1
					return test.mockOut.str, test.mockOut.err
				},
			}
			// always query with sdb1 so above can verify the query is "/dev/<input>"
			got, err := PartitionTableType(executor, "sdb1")
			if test.expectErrText != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), test.expectErrText)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, test.expect, got)
		})
	}
}

func Test_isDeviceAvailable(t *testing.T) {
	SkipDeviceOpenForUnitTests = true // don't try to open the disks (will fail on most systems)

	type execOut struct {
		str string
		err error
	}
	type testCase struct {
		name                string
		mockOut             execOut
		expectAvail         bool
		expectRejectReasons string
		expectErr           bool
	}

	tests := []testCase{
		{name: "available disk",
			mockOut:     execOut{str: "0 0 10737418240 disk", err: nil},
			expectAvail: true, expectRejectReasons: "[]", expectErr: false},
		{name: "available partition",
			mockOut:     execOut{str: "0 0 10737418240 part", err: nil},
			expectAvail: true, expectRejectReasons: "[]", expectErr: false},
		{name: "removable disk not available",
			mockOut:     execOut{str: "1 0 10737418240 disk", err: nil},
			expectAvail: false, expectRejectReasons: "[removable]", expectErr: false},
		{name: "readonly partition not available",
			mockOut:     execOut{str: "0 1 10737418240 part", err: nil},
			expectAvail: false, expectRejectReasons: "[read-only]", expectErr: false},
		{name: "size < 5GB not available",
			mockOut:     execOut{str: "0 0 5368709119 disk", err: nil},
			expectAvail: false, expectRejectReasons: "[insufficient space (< 5GB)]", expectErr: false},
		{name: "size == 5GB is available",
			mockOut:     execOut{str: "0 0 5368709120 part", err: nil},
			expectAvail: true, expectRejectReasons: "[]", expectErr: false},
		{name: "size > 5GB is available",
			mockOut:     execOut{str: "0 0 5368709121 part", err: nil},
			expectAvail: true, expectRejectReasons: "[]", expectErr: false},
		{name: "lvm unavailable",
			mockOut:     execOut{str: "0 0 5368709120 lvm", err: nil},
			expectAvail: false, expectRejectReasons: `[device type "lvm" is not acceptable; should be raw device ("disk") or partition ("part")]`, expectErr: false},
		{name: "err during lvm exec",
			mockOut:     execOut{str: "0 0 5368709120 disk", err: errors.Errorf("fake err")},
			expectAvail: false, expectRejectReasons: "", expectErr: true},
		{name: "multiple reject reasons for disk",
			mockOut:     execOut{str: "1 1 5368709119 disk"},
			expectAvail: false, expectRejectReasons: `[removable, read-only, insufficient space (< 5GB)]`, expectErr: false},
		{name: "multiple reject reasons for partition",
			mockOut:     execOut{str: "1 1 5368709119 part"},
			expectAvail: false, expectRejectReasons: `[removable, read-only, insufficient space (< 5GB)]`, expectErr: false},
		{name: "multiple reject reasons for lvm",
			mockOut:     execOut{str: "1 1 5368709119 lvm"},
			expectAvail: false, expectRejectReasons: `[removable, read-only, insufficient space (< 5GB), device type "lvm" is not acceptable; should be raw device ("disk") or partition ("part")]`, expectErr: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// set up executor with test definition's mock output
			executor := &exectest.MockExecutor{
				MockExecuteCommandWithOutput: func(command string, arg ...string) (string, error) {
					t.Log("command:", command, arg)
					assert.Equal(t, "lsblk", command)
					assert.Equal(t, "/dev/sdb1", arg[0])
					assert.Equal(t, "RM,RO,SIZE,TYPE", arg[len(arg)-1])
					return test.mockOut.str, test.mockOut.err
				},
			}
			// always query with sdb1 so above can verify the query is "/dev/<input>"
			avail, rejectReasons, err := isDeviceAvailable(executor, "/dev/sdb1")
			if test.expectErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, test.expectAvail, avail)
			assert.Equal(t, test.expectRejectReasons, rejectReasons)
		})
	}
}
