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
	"path/filepath"
	"strings"
	"testing"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The cvRawList* fixtures are verbatim "ceph-volume raw list <device...>" outputs captured from
// quay.io/ceph/ceph containers against file-backed BlueStore devices (a labeled 10G main device
// with a 4G block.db, an unlabeled 8G device, and a 1KiB device standing in for the
// extended-partition node of an MBR-partitioned OS disk).

// A device holding the OSD's main BlueStore label. Identical output from v19.2.3, v19.2.4 and
// v20.2.2.
const cvRawListSingleOSD = `{
    "4f7b9ffc-ffa3-479e-9153-18fff4f6c806": {
        "ceph_fsid": "8c481195-3476-4884-866b-56356c166303",
        "device": "/work/osd.img",
        "osd_id": 7,
        "osd_uuid": "4f7b9ffc-ffa3-479e-9153-18fff4f6c806",
        "type": "bluestore"
    }
}
`

// A device holding only the OSD's "bluefs db" label: no osd_id, no device. Identical output from
// v19.2.3, v19.2.4 and v20.2.2. On v19.2.4+ the no-argument scan emits such entries too, which
// crashed the previous parser (rook issue 17983).
const cvRawListDBDeviceOnly = `{
    "4f7b9ffc-ffa3-455b-a82f-a2735f32d6aa": {
        "device_db": "/work/db.img",
        "osd_uuid": "4f7b9ffc-ffa3-455b-a82f-a2735f32d6aa"
    }
}
`

// One batched listing of the OSD's main device AND its db device: the report is keyed by
// osd_uuid, so the db entry replaces the main entry and the osd_id is lost even though every
// device was read successfully. Identical output from v19.2.3, v19.2.4 and v20.2.2.
const cvRawListMainAndDBCollapsed = `{
    "4f7b9ffc-ffa3-479e-9153-18fff4f6c806": {
        "device_db": "/work/db.img",
        "osd_uuid": "4f7b9ffc-ffa3-479e-9153-18fff4f6c806"
    }
}
`

// A batched listing that includes a sub-4KiB device. v19.2.3 and v19.2.4 tolerate the unreadable
// device and still report the OSD.
const cvRawListBatchWithTinyDeviceSquid = `{
    "4f7b9ffc-ffa3-479e-9153-18fff4f6c806": {
        "ceph_fsid": "8c481195-3476-4884-866b-56356c166303",
        "device": "/work/osd.img",
        "osd_id": 7,
        "osd_uuid": "4f7b9ffc-ffa3-479e-9153-18fff4f6c806",
        "type": "bluestore"
    }
}
`

// The same batched listing on v20.2.2: the sub-4KiB device aborts the single ceph-bluestore-tool
// process behind the scan (https://tracker.ceph.com/issues/76354) and ceph-volume reports the
// whole scan as empty, exit code 0. Also the verbatim output for any device without a BlueStore
// label on all three versions.
const cvRawListEmpty = `{}
`

// Synthetic: osd id 0 is valid and must not be confused with an entry that has no osd_id at all.
const cvRawListOSDZero = `{
    "162d9e2a-b9e5-4d7c-a3f1-53b0a0a1f6b2": {
        "ceph_fsid": "8c481195-3476-4884-866b-56356c166303",
        "device": "/dev/sdb",
        "osd_id": 0,
        "osd_uuid": "162d9e2a-b9e5-4d7c-a3f1-53b0a0a1f6b2",
        "type": "bluestore"
    }
}
`

const lsblkMixedNode = `/dev/sda  disk
/dev/sda1 part
/dev/sda2 part
/dev/sda5 part
/dev/sdb  disk
/dev/sdc  disk
/dev/loop0 loop
/dev/rbd0 disk
/dev/nbd0 disk
/dev/zram0 disk
/dev/drbd0 disk
/dev/mapper/vg-lv lvm
`

func TestOSDDeviceFromListing(t *testing.T) {
	listings := map[string]string{}
	var listErr error
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, arg ...string) (string, error) {
			require.Equal(t, "ceph-volume", command)
			require.Equal(t, []string{"raw", "list"}, arg[:2])
			if listErr != nil {
				return "", listErr
			}
			return listings[arg[2]], nil
		},
	}
	context := &clusterd.Context{Executor: executor}

	t.Run("main device entry matches", func(t *testing.T) {
		listings["/dev/sdb"] = cvRawListSingleOSD
		device, found := osdDeviceFromListing(context, 7, "/dev/sdb")
		assert.True(t, found)
		assert.Equal(t, "/work/osd.img", device)
	})

	t.Run("wrong osd id does not match", func(t *testing.T) {
		listings["/dev/sdb"] = cvRawListSingleOSD
		_, found := osdDeviceFromListing(context, 3, "/dev/sdb")
		assert.False(t, found)
	})

	t.Run("db-only entry without osd_id is skipped", func(t *testing.T) {
		listings["/dev/sdd"] = cvRawListDBDeviceOnly
		_, found := osdDeviceFromListing(context, 7, "/dev/sdd")
		assert.False(t, found)
	})

	t.Run("batched main plus db listing loses the osd id", func(t *testing.T) {
		// The uuid-keyed report of a batched listing of an OSD's main and db devices retains
		// only the id-less db entry, which is why the fallback must list devices one at a time.
		listings["/dev/sdb"] = cvRawListMainAndDBCollapsed
		_, found := osdDeviceFromListing(context, 7, "/dev/sdb")
		assert.False(t, found)
	})

	t.Run("entry without osd_id does not match osd id 0", func(t *testing.T) {
		listings["/dev/sdd"] = cvRawListDBDeviceOnly
		_, found := osdDeviceFromListing(context, 0, "/dev/sdd")
		assert.False(t, found)
	})

	t.Run("osd id 0 matches", func(t *testing.T) {
		listings["/dev/sdb"] = cvRawListOSDZero
		device, found := osdDeviceFromListing(context, 0, "/dev/sdb")
		assert.True(t, found)
		assert.Equal(t, "/dev/sdb", device)
	})

	t.Run("empty report does not match", func(t *testing.T) {
		listings["/dev/sda"] = cvRawListEmpty
		_, found := osdDeviceFromListing(context, 7, "/dev/sda")
		assert.False(t, found)
	})

	t.Run("listing failure does not match", func(t *testing.T) {
		listErr = errors.New("exit status 1")
		defer func() { listErr = nil }()
		_, found := osdDeviceFromListing(context, 7, "/dev/sdb")
		assert.False(t, found)
	})

	t.Run("unparsable report does not match", func(t *testing.T) {
		listings["/dev/sdb"] = "Traceback (most recent call last):"
		_, found := osdDeviceFromListing(context, 7, "/dev/sdb")
		assert.False(t, found)
	})
}

func TestScanDeviceList(t *testing.T) {
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, arg ...string) (string, error) {
			require.Equal(t, "lsblk", command)
			return lsblkMixedNode, nil
		},
	}
	devices, err := scanDeviceList(&clusterd.Context{Executor: executor})
	require.NoError(t, err)
	assert.Equal(t, []string{"/dev/sda", "/dev/sda1", "/dev/sda2", "/dev/sda5", "/dev/sdb", "/dev/sdc"}, devices)
}

func TestFindOSDDevice(t *testing.T) {
	t.Run("stale path poisons the batch on v20 but the per-device scan finds the osd", func(t *testing.T) {
		// The persisted path no longer holds the OSD, and one scanned device (the 1KiB
		// extended-partition node /dev/sda2) makes ceph-volume report {} for itself, exactly as
		// the whole batched scan failed in rook issue 17992.
		var listed []string
		executor := &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, arg ...string) (string, error) {
				if command == "lsblk" {
					return lsblkMixedNode, nil
				}
				require.Equal(t, "ceph-volume", command)
				device := arg[2]
				listed = append(listed, device)
				if device == "/dev/sdb" {
					return cvRawListSingleOSD, nil
				}
				return cvRawListEmpty, nil
			},
		}
		device, err := findOSDDevice(&clusterd.Context{Executor: executor}, "7", "/dev/oldname")
		require.NoError(t, err)
		assert.Equal(t, "/work/osd.img", device)
		assert.Equal(t, []string{"/dev/oldname", "/dev/sda", "/dev/sda1", "/dev/sda2", "/dev/sda5", "/dev/sdb"}, listed)
	})

	t.Run("a device whose listing fails does not end the scan", func(t *testing.T) {
		executor := &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, arg ...string) (string, error) {
				if command == "lsblk" {
					return lsblkMixedNode, nil
				}
				switch arg[2] {
				case "/dev/sdc":
					return cvRawListBatchWithTinyDeviceSquid, nil
				case "/dev/sda2":
					return "", errors.New("exit status 1")
				}
				return cvRawListEmpty, nil
			},
		}
		device, err := findOSDDevice(&clusterd.Context{Executor: executor}, "7", "")
		require.NoError(t, err)
		assert.Equal(t, "/work/osd.img", device)
	})

	t.Run("no device reports the osd", func(t *testing.T) {
		executor := &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, arg ...string) (string, error) {
				if command == "lsblk" {
					return lsblkMixedNode, nil
				}
				return cvRawListEmpty, nil
			},
		}
		_, err := findOSDDevice(&clusterd.Context{Executor: executor}, "7", "/dev/oldname")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no device found with OSD ID 7")
	})

	t.Run("empty scan list fails", func(t *testing.T) {
		executor := &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, arg ...string) (string, error) {
				if command == "lsblk" {
					return "/dev/rbd0 disk\n", nil
				}
				return cvRawListEmpty, nil
			},
		}
		_, err := findOSDDevice(&clusterd.Context{Executor: executor}, "7", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no devices to scan")
	})

	t.Run("invalid osd id fails", func(t *testing.T) {
		_, err := findOSDDevice(&clusterd.Context{Executor: &exectest.MockExecutor{}}, "not-a-number", "")
		require.Error(t, err)
	})
}

func TestActivateRaw(t *testing.T) {
	dataDir := t.TempDir()
	require.NoError(t, os.Symlink("/dev/oldname", filepath.Join(dataDir, "block")))

	var activateArgs []string
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, arg ...string) (string, error) {
			if command == "lsblk" {
				return lsblkMixedNode, nil
			}
			if arg[2] == "/dev/sdb" {
				return cvRawListSingleOSD, nil
			}
			return cvRawListEmpty, nil
		},
		MockExecuteCommandWithCombinedOutput: func(command string, arg ...string) (string, error) {
			activateArgs = append([]string{command}, arg...)
			return "", nil
		},
	}

	args := ActivateOSDArgs{ID: "7", UUID: "4f7b9ffc-ffa3-479e-9153-18fff4f6c806", CVMode: "raw", BlockPath: "/dev/oldname"}
	err := activateRaw(&clusterd.Context{Executor: executor}, args, dataDir)
	require.NoError(t, err)

	assert.Equal(t, []string{"ceph-volume", "raw", "activate", "--device", "/work/osd.img", "--no-systemd", "--no-tmpfs"}, activateArgs)

	_, err = os.Lstat(filepath.Join(dataDir, "block"))
	assert.True(t, os.IsNotExist(err), "the stale block symlink must be removed")
}

func TestRemoveStaleBlockSymlink(t *testing.T) {
	t.Run("matching symlink is kept", func(t *testing.T) {
		dataDir := t.TempDir()
		require.NoError(t, os.Symlink("/dev/sdb", filepath.Join(dataDir, "block")))
		removeStaleBlockSymlink(dataDir, "/dev/sdb")
		_, err := os.Lstat(filepath.Join(dataDir, "block"))
		assert.NoError(t, err)
	})

	t.Run("regular file is kept", func(t *testing.T) {
		dataDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dataDir, "block"), []byte("x"), 0o600))
		removeStaleBlockSymlink(dataDir, "/dev/sdb")
		_, err := os.Lstat(filepath.Join(dataDir, "block"))
		assert.NoError(t, err)
	})

	t.Run("missing block path is tolerated", func(t *testing.T) {
		removeStaleBlockSymlink(t.TempDir(), "/dev/sdb")
	})
}

func TestActivateLVM(t *testing.T) {
	lvmConf, err := os.CreateTemp(t.TempDir(), "lvm.conf")
	require.NoError(t, err)
	require.NoError(t, lvmConf.Close())
	origLVMConfPath := lvmConfPath
	lvmConfPath = lvmConf.Name()
	defer func() { lvmConfPath = origLVMConfPath }()

	dataDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dataDir, "keyring"), []byte("k"), 0o600))
	require.NoError(t, os.Symlink("/dev/vg/lv", filepath.Join(dataDir, "block")))

	var commands [][]string
	var activateArgs []string
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithCombinedOutput: func(command string, arg ...string) (string, error) {
			activateArgs = append([]string{command}, arg...)
			return "", nil
		},
		MockExecuteCommand: func(command string, arg ...string) error {
			commands = append(commands, append([]string{command}, arg...))
			return nil
		},
	}

	args := ActivateOSDArgs{ID: "3", UUID: "some-uuid", StoreFlag: "--bluestore", CVMode: "lvm"}
	require.NoError(t, activateLVM(&clusterd.Context{Executor: executor}, args, dataDir))

	conf, err := os.ReadFile(lvmConfPath)
	require.NoError(t, err)
	assert.Contains(t, string(conf), `filter = ["r|/dev/rbd.*|"]`)

	assert.Equal(t, []string{"ceph-volume", "lvm", "activate", "--no-systemd", "--bluestore", "3", "some-uuid"}, activateArgs)
	require.Len(t, commands, 3)
	assert.Equal(t, "cp", commands[0][0])
	assert.Contains(t, commands[0], filepath.Join(dataDir, "block"))
	assert.Equal(t, []string{"umount", dataDir}, commands[1])
	assert.Equal(t, []string{"chown", "--verbose", "--recursive", "ceph:ceph", dataDir}, commands[2])
}

func TestResizeEncryptedLVMDevice(t *testing.T) {
	dataDir := t.TempDir()
	require.NoError(t, os.Symlink("/dev/mapper/rook-lv-block-dmcrypt", filepath.Join(dataDir, "block")))

	var commands [][]string
	executor := &exectest.MockExecutor{
		MockExecuteCommand: func(command string, arg ...string) error {
			commands = append(commands, append([]string{command}, arg...))
			return nil
		},
		MockExecuteCommandWithOutput: func(command string, arg ...string) (string, error) {
			joined := command + " " + strings.Join(arg, " ")
			switch {
			case command == "pvs":
				return "  /dev/sdb\n", nil
			case command == "vgs" && strings.Contains(joined, "lv_count"):
				return "  2\n", nil
			case command == "vgs" && strings.Contains(joined, "vg_extent_count"):
				return "  100\n", nil
			case command == "ceph":
				return "luks-secret-key", nil
			}
			return "", errors.Errorf("unexpected command %q", joined)
		},
	}

	args := ActivateOSDArgs{ID: "3", UUID: "some-uuid", BlockPath: "/dev/rook-vg/rook-lv", Encrypted: true}
	require.NoError(t, resizeEncryptedLVMDevice(&clusterd.Context{Executor: executor}, args, dataDir))

	require.Len(t, commands, 3)
	assert.Equal(t, []string{"pvresize", "/dev/sdb"}, commands[0])
	assert.Equal(t, []string{"lvextend", "-l", "50", "/dev/rook-vg/rook-lv"}, commands[1])
	assert.Equal(t, "cryptsetup", commands[2][0])
	assert.Contains(t, commands[2], "rook-lv-block-dmcrypt")
}

func TestRefreshLockboxKeyring(t *testing.T) {
	t.Run("rotated key is written", func(t *testing.T) {
		dataDir := t.TempDir()
		executor := &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, arg ...string) (string, error) {
				require.Equal(t, "ceph", command)
				assert.Contains(t, arg, "client.osd-lockbox.some-uuid")
				return "[client.osd-lockbox.some-uuid]\nkey = secret\n", nil
			},
		}
		refreshLockboxKeyring(&clusterd.Context{Executor: executor}, "some-uuid", dataDir)
		content, err := os.ReadFile(filepath.Join(dataDir, "lockbox.keyring"))
		require.NoError(t, err)
		assert.Contains(t, string(content), "key = secret")
		// ceph's keyring parser rejects a file without a trailing newline
		assert.True(t, strings.HasSuffix(string(content), "\n"), "the lockbox keyring must end with a newline")
	})

	t.Run("failure keeps the on-disk key", func(t *testing.T) {
		dataDir := t.TempDir()
		executor := &exectest.MockExecutor{
			MockExecuteCommandWithOutput: func(command string, arg ...string) (string, error) {
				return "", errors.New("mons are down")
			},
		}
		refreshLockboxKeyring(&clusterd.Context{Executor: executor}, "some-uuid", dataDir)
		_, err := os.Stat(filepath.Join(dataDir, "lockbox.keyring"))
		assert.True(t, os.IsNotExist(err))
	})
}
