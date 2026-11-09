/*
Copyright 2026 The Rook Authors. All rights reserved.

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
	"os"
	"slices"
	"testing"
	"time"

	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	oposd "github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/rook/rook/pkg/util/sys"
	"github.com/stretchr/testify/assert"
)

func TestOSDUUIDFromMapper(t *testing.T) {
	uuid := "8f3d6a1e-2b7c-4e59-9a0d-1c2b3d4e5f60"
	assert.Equal(t, uuid, osdUUIDFromMapper("/dev/mapper/ceph-"+uuid+"-loop0-block-dmcrypt"))
	assert.Equal(t, uuid, osdUUIDFromMapper("/dev/mapper/ceph-"+uuid+"-nvme0n1p2-block-dmcrypt"))
	assert.Equal(t, "", osdUUIDFromMapper(""))
	assert.Equal(t, "", osdUUIDFromMapper("/dev/mapper/ceph-short-block-dmcrypt"))
	assert.Equal(t, "", osdUUIDFromMapper("/dev/mapper/set1-data-0abcd-block-dmcrypt-and-more-text"))
}

func TestGetEncryptedBlockPathRawPrepare(t *testing.T) {
	op := "Running command: /usr/sbin/cryptsetup --key-size 512 --key-file - --allow-discards luksOpen /dev/loop0 ceph-8f3d6a1e-2b7c-4e59-9a0d-1c2b3d4e5f60-loop0-block-dmcrypt"
	assert.Equal(t, "/dev/mapper/ceph-8f3d6a1e-2b7c-4e59-9a0d-1c2b3d4e5f60-loop0-block-dmcrypt", getEncryptedBlockPath(op, "block-dmcrypt"))
}

func TestOSDUUIDFromLUKS(t *testing.T) {
	assert.Equal(t, "64f279de-45a7-4372-aa11-d52d13db22c6", osdUUIDFromLUKS("LUKS header information\nVersion:       \t2\nLabel:         \t(no label)\nSubsystem:     \tceph_fsid=64f279de-45a7-4372-aa11-d52d13db22c6\n"))
	assert.Equal(t, "", osdUUIDFromLUKS("Label:  \t(no label)\nSubsystem:     \t(no subsystem)\n"))
	assert.Equal(t, "", osdUUIDFromLUKS("Subsystem: ceph_fsid=not-a-uuid\n"))
	// only the subsystem holds the OSD fsid
	assert.Equal(t, "", osdUUIDFromLUKS("Label:         \tceph_fsid=64f279de-45a7-4372-aa11-d52d13db22c6\nSubsystem:     \t(no subsystem)\n"))
}

func TestOpenHostOSDs(t *testing.T) {
	openUUID := "8f3d6a1e-2b7c-4e59-9a0d-1c2b3d4e5f60"
	closedUUID := "64f279de-45a7-4372-aa11-d52d13db22c6"
	purgedUUID := "0b8e7c1a-3d2f-4a5b-8c6d-7e8f9a0b1c2d"
	var configKeys []string
	opened := map[string]string{}
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithCombinedOutput: func(command string, args ...string) (string, error) {
			switch args[1] {
			case "/dev/sdb":
				return "Subsystem:     \tceph_fsid=" + openUUID + "\n", nil
			case "/dev/sdc":
				return "Subsystem:     \tceph_fsid=" + closedUUID + "\n", nil
			case "/dev/sde":
				return "Subsystem:     \tceph_fsid=" + purgedUUID + "\n", nil
			}
			return "Subsystem:     \t(no subsystem)\n", nil
		},
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			configKeys = append(configKeys, args[2])
			return "key-" + args[2], nil
		},
		MockExecuteCommandWithStdin: func(timeout time.Duration, command string, stdin *string, args ...string) error {
			opened[args[len(args)-1]] = *stdin
			return nil
		},
	}
	context := &clusterd.Context{Executor: executor, Devices: []*sys.LocalDisk{
		// an open OSD, also listed as LUKS, as a loop device is
		{Name: "dm-2", RealPath: "/dev/dm-2", Type: sys.CryptType, DevLinks: "/dev/mapper/ceph-" + openUUID + "-sdb-block-dmcrypt"},
		{Name: "sdb", RealPath: "/dev/sdb", Type: sys.DiskType, Filesystem: "crypto_LUKS"},
		// a closed OSD of this cluster
		{Name: "sdc", RealPath: "/dev/sdc", Type: sys.DiskType, Filesystem: "crypto_LUKS"},
		// LUKS that is no raw mode OSD
		{Name: "sdd", RealPath: "/dev/sdd", Type: sys.DiskType, Filesystem: "crypto_LUKS"},
		// a purged OSD whose disk was not wiped
		{Name: "sde", RealPath: "/dev/sde", Type: sys.DiskType, Filesystem: "crypto_LUKS"},
	}}

	openHostOSDs(context, cephclient.AdminTestClusterInfo("ns"), map[string]bool{openUUID: true, closedUUID: true})
	assert.Equal(t, []string{"dm-crypt/osd/" + closedUUID + "/luks"}, configKeys)
	assert.Equal(t, map[string]string{"ceph-" + closedUUID + "-sdc-block-dmcrypt": "key-dm-crypt/osd/" + closedUUID + "/luks"}, opened)
}

func TestRawModeForEncryptedHostOSDs(t *testing.T) {
	encryptedAgent := &OsdAgent{storeConfig: config.StoreConfig{EncryptedDevice: true}}
	device := &DeviceOsdIDEntry{Config: DesiredDevice{Name: "sdb", OSDsPerDevice: 1, EncryptedDevice: true}, DeviceInfo: &sys.LocalDisk{Type: sys.DiskType}}
	rawAllowed := func() bool {
		allowed, err := encryptedAgent.allowRawMode(&clusterd.Context{})
		assert.NoError(t, err)
		return allowed
	}

	assert.False(t, rawAllowed())
	assert.False(t, isSafeToUseRawMode(device))

	t.Setenv(oposd.HostOSDKMSEnvVarName, "true")
	assert.True(t, rawAllowed())
	assert.True(t, isSafeToUseRawMode(device))

	// the other reasons still force lvm mode
	device.Config.OSDsPerDevice = 2
	assert.False(t, isSafeToUseRawMode(device))
	device.Config.OSDsPerDevice = 1
	device.DeviceInfo.Type = sys.LVMType
	assert.False(t, isSafeToUseRawMode(device))

	defer func(onPVC bool) { isOnPVC = onPVC }(isOnPVC)
	isOnPVC = true
	assert.False(t, hostKMSEnabled())
}

func TestEncryptedPartitionGetsMapper(t *testing.T) {
	t.Setenv(oposd.HostOSDKMSEnvVarName, "true")
	uuid := "8f3d6a1e-2b7c-4e59-9a0d-1c2b3d4e5f60"
	var prepareArgs []string
	var keySet bool
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithCombinedOutput: func(command string, args ...string) (string, error) {
			prepareArgs = args
			keySet = os.Getenv(oposd.CephVolumeEncryptedKeyEnvVarName) != ""
			return "Running command: /usr/sbin/cryptsetup --key-size 512 --key-file - --allow-discards luksOpen /dev/sdt1 ceph-" + uuid + "-sdt1-block-dmcrypt", nil
		},
	}
	agent := &OsdAgent{}
	devices := &DeviceOsdMapping{Entries: map[string]*DeviceOsdIDEntry{
		"sdt1": {Data: -1, Config: DesiredDevice{Name: "sdt1", OSDsPerDevice: 1, EncryptedDevice: true}, DeviceInfo: &sys.LocalDisk{Type: sys.PartType}},
	}}
	assert.True(t, isSafeToUseRawMode(devices.Entries["sdt1"]))

	prepared, err := agent.initializeDevicesRawMode(&clusterd.Context{Executor: executor}, devices)
	assert.NoError(t, err)
	assert.True(t, slices.Contains(prepareArgs, encryptedFlag), prepareArgs)
	assert.True(t, slices.Contains(prepareArgs, "/dev/sdt1"), prepareArgs)
	assert.True(t, keySet)
	assert.Empty(t, os.Getenv(oposd.CephVolumeEncryptedKeyEnvVarName))
	assert.Equal(t, []string{"/dev/mapper/ceph-" + uuid + "-sdt1-block-dmcrypt"}, prepared)
}
