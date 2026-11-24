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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	oposd "github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/rook/rook/pkg/util/sys"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanWipeOSDOnDevice(t *testing.T) {
	for _, tt := range []struct {
		name, label                  string
		readErr, purgeErr            bool
		wantWipe, wantErr, wantPurge bool
	}{
		{name: "unreadable label", readErr: true, wantWipe: true},
		{name: "invalid JSON", label: `{`, wantErr: true},
		{name: "wrong field type", label: `{"/dev/data":{"whoami":0}}`, wantErr: true},
		{name: "missing device label", label: `{"/dev/other":{}}`, wantWipe: true},
		{name: "complete local OSD", label: `{"/dev/data":{"ceph_fsid":"cluster","whoami":"0","osd_uuid":"known"}}`},
		{name: "unknown UUID", label: `{"/dev/data":{"osd_uuid":"unknown"}}`, wantWipe: true},
		{name: "new OSD", label: `{"/dev/data":{"osd_uuid":"known"}}`, wantWipe: true, wantPurge: true},
		{name: "missing whoami", label: `{"/dev/data":{"ceph_fsid":"cluster","osd_uuid":"known"}}`, wantWipe: true, wantPurge: true},
		{name: "purge rejected", label: `{"/dev/data":{"osd_uuid":"known"}}`, purgeErr: true, wantPurge: true},
		{name: "foreign OSD", label: `{"/dev/data":{"ceph_fsid":"other","whoami":"0","osd_uuid":"unknown"}}`, wantWipe: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dump := &client.OSDDump{}
			require.NoError(t, json.Unmarshal([]byte(`{"osds":[{"osd":7,"uuid":"known"}]}`), dump))
			purged := false
			executor := &exectest.MockExecutor{MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				if command == "ceph-bluestore-tool" {
					assert.Equal(t, []string{"show-label", "--dev", "/dev/data"}, args)
					if tt.readErr {
						return "", errors.New("read failed")
					}
					return tt.label, nil
				}
				assert.Equal(t, "ceph", command)
				require.GreaterOrEqual(t, len(args), 4)
				assert.Equal(t, []string{"osd", "purge-new", "osd.7", "--yes-i-really-mean-it"}, args[:4])
				purged = true
				if tt.purgeErr {
					return "", errors.New("OSD is not new")
				}
				return "", nil
			}}
			agent := &OsdAgent{clusterInfo: &client.ClusterInfo{FSID: "cluster", Context: context.Background()}}
			wipe, err := agent.canWipeOSDOnDevice(&clusterd.Context{Executor: executor}, dump, "/dev/data", "/dev/data")
			assert.Equal(t, tt.wantWipe, wipe)
			assert.Equal(t, tt.wantPurge, purged)
			if tt.wantErr {
				assert.ErrorIs(t, err, errUnmarshalBluestoreLabel)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestReadBluestoreLabel(t *testing.T) {
	executor := &exectest.MockExecutor{MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
		return `{"/dev/data":{"osd_uuid":"uuid","ceph_fsid":"cluster","whoami":"0","created_at":"today","ready":"ready","type":"bluestore","locations":["0","1073741824"],"osd_key":"secret"},"/dev/db":{"osd_uuid":"other"}}`, nil
	}}
	label, err := readBluestoreLabel(&clusterd.Context{Executor: executor}, "/dev/data")
	require.NoError(t, err)
	assert.Equal(t, &bluestoreLabel{OsdUUID: "uuid", CephFsid: "cluster", Whoami: "0", CreatedAt: "today", Ready: "ready", Type: "bluestore", Locations: []string{"0", "1073741824"}}, label)
}

func TestGetDeviceFileSystem(t *testing.T) {
	commands := []string{
		"lsblk --nodeps --noheadings --raw --output KNAME /dev/data",
		"udevadm trigger --action=change /sys/class/block/sda",
		"udevadm settle",
		"lsblk /dev/data --noheadings --nodeps --output FSTYPE",
	}
	for _, tt := range []struct {
		name        string
		failAt      int
		emptyKernel bool
	}{
		{"success", -1, false},
		{"kernel lookup failure", 0, false},
		{"empty kernel name", -1, true},
		{"trigger failure", 1, false},
		{"settle failure", 2, false},
		{"filesystem lookup failure", 3, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			failure := errors.New("command failed")
			executor := &exectest.MockExecutor{MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
				i := calls
				calls++
				require.Less(t, i, len(commands))
				assert.Equal(t, commands[i], command+" "+strings.Join(args, " "))
				if i == tt.failAt {
					return "", failure
				}
				if i == 0 {
					if tt.emptyKernel {
						return " \n", nil
					}
					return " sda\n", nil
				}
				if i == 3 {
					return " ceph_bluestore\n", nil
				}
				return "", nil
			}}
			fs, err := getDeviceFileSystem(&clusterd.Context{Executor: executor}, "/dev/data")
			switch {
			case tt.emptyKernel:
				assert.ErrorContains(t, err, "empty kernel name")
				assert.Equal(t, 1, calls)
			case tt.failAt >= 0:
				assert.ErrorIs(t, err, failure)
				assert.Equal(t, tt.failAt+1, calls)
			default:
				assert.NoError(t, err)
				assert.Equal(t, "ceph_bluestore", fs)
				assert.Equal(t, 4, calls)
			}
		})
	}
}

func TestWipeIncompleteOSDDeviceOnPVC(t *testing.T) {
	for _, tt := range []struct {
		name                     string
		nonPVC, missing, mounted bool
		filesystem, refreshed    string
		existing                 map[string]osdInfoBlock
		label                    string
		refreshErr, dumpErr      bool
		failWipe                 string
		wantWiped                []string
		wantErr                  string
		wantDump                 bool
	}{
		{name: "non PVC", nonPVC: true},
		{name: "missing data device", missing: true},
		{name: "mounted", mounted: true, filesystem: "ceph_bluestore"},
		{name: "foreign filesystem", filesystem: "ext4"},
		{name: "blank device"},
		{name: "refresh failure", refreshErr: true, wantErr: "failed to get filesystem"},
		{name: "healthy local OSD", filesystem: "ceph_bluestore", existing: map[string]osdInfoBlock{"osd": {CephFsid: "cluster", Device: "/dev/data"}}},
		{name: "healthy OSD symlink", filesystem: "ceph_bluestore", existing: map[string]osdInfoBlock{"osd": {CephFsid: "cluster", Device: "/dev/disk/by-id/data"}}},
		{name: "OSD dump failure", filesystem: "ceph_bluestore", dumpErr: true, wantDump: true, wantErr: "failed to get the osdmap"},
		{name: "invalid label", filesystem: "ceph_bluestore", label: `{`, wantDump: true, wantErr: "failed to release incomplete OSD"},
		{name: "complete label", filesystem: "ceph_bluestore", label: `{"/dev/data":{"ceph_fsid":"cluster","whoami":"0"}}`, wantDump: true},
		{name: "wipe data last", filesystem: "ceph_bluestore", wantDump: true, wantWiped: []string{"/dev/db", "/dev/wal", "/dev/data"}},
		{name: "refresh stale filesystem", refreshed: "ceph_bluestore", wantDump: true, wantWiped: []string{"/dev/db", "/dev/wal", "/dev/data"}},
		{name: "ignore foreign and metadata entries", filesystem: "ceph_bluestore", existing: map[string]osdInfoBlock{"foreign": {CephFsid: "other", Device: "/dev/data"}, "db": {CephFsid: "cluster"}, "another": {CephFsid: "cluster", Device: "/dev/other"}}, wantDump: true, wantWiped: []string{"/dev/db", "/dev/wal", "/dev/data"}},
		{name: "metadata wipe failure preserves data", filesystem: "ceph_bluestore", failWipe: "/dev/wal", wantDump: true, wantWiped: []string{"/dev/db", "/dev/wal"}, wantErr: "failed to wipe"},
		{name: "data wipe failure preserves signature", filesystem: "ceph_bluestore", failWipe: "/dev/data", wantDump: true, wantWiped: []string{"/dev/db", "/dev/wal", "/dev/data"}, wantErr: "failed to wipe"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			data := &sys.LocalDisk{Type: pvcDataTypeDevice, RealPath: "/dev/data", DevLinks: "/dev/disk/by-id/data", Filesystem: tt.filesystem}
			if tt.mounted {
				data.Mountpoint = "/mounted"
			}
			devices := []*sys.LocalDisk{data, {Type: pvcMetadataTypeDevice, RealPath: "/dev/db", Filesystem: "ceph_bluestore"}, {Type: pvcWalTypeDevice, RealPath: "/dev/wal", Filesystem: "ceph_bluestore"}, {RealPath: "/dev/blank"}}
			if tt.missing {
				devices = devices[1:]
			}
			var wiped []string
			dumped, refreshed := false, false
			executor := &exectest.MockExecutor{}
			executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
				switch command {
				case "lsblk":
					refreshed = true
					if tt.refreshErr {
						return "", errors.New("lsblk failed")
					}
					if slices.Contains(args, "KNAME") {
						return "sda", nil
					}
					return tt.refreshed, nil
				case "udevadm":
					return "", nil
				case "ceph":
					assert.Equal(t, []string{"osd", "dump"}, args[:2])
					dumped = true
					if tt.dumpErr {
						return "", errors.New("dump failed")
					}
					return `{"osds":[]}`, nil
				case "ceph-bluestore-tool":
					if tt.label != "" {
						return tt.label, nil
					}
					return `{"/dev/data":{"osd_uuid":"unknown"}}`, nil
				default:
					t.Fatalf("unexpected command %s %v", command, args)
					return "", nil
				}
			}
			executor.MockExecuteCommandWithCombinedOutput = func(command string, args ...string) (string, error) {
				switch command {
				case "wipefs":
					wiped = append(wiped, args[1])
					if args[1] == tt.failWipe {
						return "", errors.New("wipe failed")
					}
				case "stdbuf", "umount", "ceph-bluestore-tool", "dd":
				default:
					t.Fatalf("unexpected command %s %v", command, args)
				}
				return "", nil
			}
			agent := &OsdAgent{pvcBacked: !tt.nonPVC, clusterInfo: &client.ClusterInfo{FSID: "cluster", Context: context.Background()}}
			err := agent.wipeIncompleteOSDDeviceOnPVC(&clusterd.Context{Executor: executor, Devices: devices}, tt.existing)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.wantDump, dumped)
			assert.Equal(t, !tt.nonPVC && !tt.missing && tt.filesystem == "", refreshed)
			assert.Equal(t, tt.wantWiped, wiped)
			for _, d := range devices {
				expected := "ceph_bluestore"
				if d == data {
					expected = tt.filesystem
					if refreshed && !tt.refreshErr {
						expected = tt.refreshed
					}
				}
				if d.RealPath == "/dev/blank" || (slices.Contains(wiped, d.RealPath) && d.RealPath != tt.failWipe) {
					expected = ""
				}
				assert.Equal(t, expected, d.Filesystem, d.RealPath)
			}
		})
	}
}

func TestReleaseIncompleteEncryptedOSD(t *testing.T) {
	for _, tt := range []struct {
		name                                                     string
		missingKey, missingPVC, openErr, staleCloseErr, closeErr bool
		label                                                    string
		wantWipe                                                 bool
		wantErr                                                  string
		wantCalls                                                []string
	}{
		{name: "missing key", missingKey: true, wantErr: "encryption key or PVC name is missing"},
		{name: "missing PVC", missingPVC: true, wantErr: "encryption key or PVC name is missing"},
		{name: "open failure", openErr: true, wantErr: "failed to open encrypted device", wantCalls: []string{"remove", "open"}},
		{name: "complete OSD keeps mapper", label: `{"ceph_fsid":"cluster","whoami":"0"}`, wantCalls: []string{"remove", "open", "label"}},
		{name: "invalid label keeps mapper", label: `{`, wantErr: "invalid label JSON", wantCalls: []string{"remove", "open", "label"}},
		{name: "incomplete OSD closes mapper", wantWipe: true, wantCalls: []string{"remove", "open", "label", "close"}},
		{name: "stale mapper removal failure is ignored", staleCloseErr: true, wantWipe: true, wantCalls: []string{"remove", "open", "label", "close"}},
		{name: "close failure prevents wipe", closeErr: true, wantErr: "failed to close encrypted block", wantCalls: []string{"remove", "open", "label", "close"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			key, pvc := "test-key", "test-pvc"
			if tt.missingKey {
				key = ""
			}
			if tt.missingPVC {
				pvc = ""
			}
			t.Setenv(oposd.CephVolumeEncryptedKeyEnvVarName, key)
			t.Setenv(oposd.PVCNameEnvVarName, pvc)
			target := oposd.EncryptionDMName(pvc, oposd.DmcryptBlockType)
			mapper := oposd.EncryptionDMPath(pvc, oposd.DmcryptBlockType)
			var calls []string
			executor := &exectest.MockExecutor{}
			// MockExecutor requires this callback to dispatch ExecuteCommandWithStdin.
			executor.MockExecuteCommand = func(command string, args ...string) error { t.Fatalf("unexpected command %s", command); return nil }
			executor.MockExecuteCommandWithStdin = func(_ time.Duration, command string, stdin *string, args ...string) error {
				calls = append(calls, "open")
				assert.Equal(t, "cryptsetup", command)
				assert.Equal(t, []string{"luksOpen", "--verbose", "--allow-discards", "/dev/data", target}, args)
				assert.Equal(t, key, *stdin)
				if tt.openErr {
					return errors.New("open failed")
				}
				return nil
			}
			executor.MockExecuteCommandWithTimeout = func(_ time.Duration, command string, args ...string) (string, error) {
				assert.Equal(t, "dmsetup", command)
				if len(calls) == 0 {
					calls = append(calls, "remove")
					assert.Equal(t, []string{"remove", "--force", target}, args)
					if tt.staleCloseErr {
						return "", errors.New("no stale mapper")
					}
				} else {
					calls = append(calls, "close")
					assert.Equal(t, []string{"remove", "--force", mapper}, args)
					if tt.closeErr {
						return "", errors.New("close failed")
					}
				}
				return "", nil
			}
			executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
				calls = append(calls, "label")
				assert.Equal(t, "ceph-bluestore-tool", command)
				assert.Equal(t, []string{"show-label", "--dev", mapper}, args)
				label := tt.label
				if label == "" {
					label = `{"osd_uuid":"unknown"}`
				}
				return fmt.Sprintf(`{%q:%s}`, mapper, label), nil
			}
			agent := &OsdAgent{clusterInfo: &client.ClusterInfo{FSID: "cluster", Context: context.Background()}}
			wipe, err := agent.releaseIncompleteOSD(&clusterd.Context{Executor: executor}, &client.OSDDump{}, &sys.LocalDisk{RealPath: "/dev/data", Filesystem: "crypto_LUKS"})
			assert.Equal(t, tt.wantWipe, wipe)
			assert.Equal(t, tt.wantCalls, calls)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
			if tt.missingKey || tt.missingPVC || tt.openErr {
				assert.ErrorIs(t, err, errOpenEncryptedDevice)
			}
		})
	}
}
