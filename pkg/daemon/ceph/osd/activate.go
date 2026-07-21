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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
)

const (
	osdDataDirBase        = "/var/lib/ceph/osd/ceph-"
	adminKeyringStorePath = "/etc/ceph/admin-keyring-store/keyring"
)

// scanDeviceExclusionPattern excludes network-backed devices (rbd, nbd, drbd) that can hang in
// uninterruptible sleep when their backing storage is unavailable — opening an rbd device whose
// I/O depends on this OSD being up would deadlock activation — and zram (volatile RAM). Rook
// never provisions OSDs on any of these.
var scanDeviceExclusionPattern = regexp.MustCompile(`^/dev/(rbd|nbd|zram|drbd)`)

// ActivateOSDArgs are the inputs of the activate init container for an OSD on a node.
type ActivateOSDArgs struct {
	ID        string
	UUID      string
	StoreFlag string
	CVMode    string
	BlockPath string
	Encrypted bool
}

// rawListEntry is a single device report from "ceph-volume raw list". Auxiliary entries (e.g.
// "bluefs db" labels of an OSD with a separate metadata device) carry only osd_uuid and
// device_db; OsdID is a pointer so those can be told apart from a valid osd id 0.
type rawListEntry struct {
	OsdID  *int   `json:"osd_id"`
	Device string `json:"device"`
}

// ActivateOSD prepares the OSD data dir of a node-based OSD so the ceph-osd daemon container can
// start: it re-resolves the OSD's current block device if the persisted path went stale after a
// reboot renamed kernel device names, then runs "ceph-volume {lvm,raw} activate".
func ActivateOSD(context *clusterd.Context, args ActivateOSDArgs) error {
	dataDir := osdDataDirBase + args.ID

	if args.Encrypted {
		refreshLockboxKeyring(context, args.UUID, dataDir)
	}

	if args.CVMode == "lvm" {
		return activateLVM(context, args, dataDir)
	}
	return activateRaw(context, args, dataDir)
}

// refreshLockboxKeyring copies the latest lockbox key to the keyring file as it might have been
// rotated. Failures are tolerated: allowing the OSD to attempt to start with the on-disk key
// could avoid a full OSD outage due to mon or system issues.
func refreshLockboxKeyring(context *clusterd.Context, osdUUID, dataDir string) {
	lockboxUser := fmt.Sprintf("client.osd-lockbox.%s", osdUUID)
	monCap := fmt.Sprintf("allow command \"config-key get\" with key=\"dm-crypt/osd/%s/luks\"", osdUUID)
	keyring, err := context.Executor.ExecuteCommandWithOutput("ceph",
		"--name", "client.admin",
		"auth", "get-or-create", lockboxUser, "mon", monCap,
		"--keyring", adminKeyringStorePath)
	if err != nil {
		logger.Warningf("failed to get latest cephx lockbox key for OSD. continuing OSD startup using on-disk key. %v", err)
		return
	}

	logger.Info("got latest cephx lockbox key for OSD successfully. updating on-disk key")
	// ExecuteCommandWithOutput strips the trailing newline; ceph's keyring parser rejects a
	// keyring file that does not end in one, so restore it before writing.
	if err := os.WriteFile(filepath.Join(dataDir, "lockbox.keyring"), []byte(keyring+"\n"), 0o600); err != nil {
		logger.Warningf("failed to update on-disk lockbox key. continuing OSD startup using on-disk key. %v", err)
	}
}

func activateLVM(context *clusterd.Context, args ActivateOSDArgs, dataDir string) error {
	// prevent LVM from trying to scan RBD volumes that may be unable to serve reads without this
	// OSD up
	f, err := os.OpenFile(lvmConfPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return errors.Wrap(err, "failed to open lvm config for the rbd device filter")
	}
	if _, err := f.WriteString("devices { filter = [\"r|/dev/rbd.*|\"] }\n"); err != nil {
		f.Close()
		return errors.Wrap(err, "failed to append the rbd device filter to the lvm config")
	}
	if err := f.Close(); err != nil {
		return errors.Wrap(err, "failed to close the lvm config")
	}

	output, err := context.Executor.ExecuteCommandWithCombinedOutput("ceph-volume", "lvm", "activate", "--no-systemd", args.StoreFlag, args.ID, args.UUID)
	logger.Infof("ceph-volume lvm activate for osd.%s: %s", args.ID, output)
	if err != nil {
		return errors.Wrapf(err, "failed to activate osd.%s with ceph-volume lvm: %s", args.ID, output)
	}

	if args.Encrypted {
		if err := resizeEncryptedLVMDevice(context, args, dataDir); err != nil {
			return err
		}
	}

	return preserveActivationFiles(context, dataDir)
}

// resizeEncryptedLVMDevice resizes the full stack of an encrypted LVM OSD: PV → LV → LUKS. All
// operations are no-ops if the underlying device hasn't grown.
func resizeEncryptedLVMDevice(context *clusterd.Context, args ActivateOSDArgs, dataDir string) error {
	vgName := filepath.Base(filepath.Dir(args.BlockPath))

	pvName, err := context.Executor.ExecuteCommandWithOutput("pvs", "--noheadings", "-o", "pv_name", "-S", "vg_name="+vgName)
	if err != nil {
		return errors.Wrapf(err, "failed to look up the physical volume of volume group %q: %s", vgName, pvName)
	}
	if err := context.Executor.ExecuteCommand("pvresize", strings.TrimSpace(pvName)); err != nil {
		return errors.Wrapf(err, "failed to resize the physical volume of volume group %q", vgName)
	}

	// With osd_per_device > 1, multiple LVs share the same VG. An absolute target
	// (total/count) divides space equally and is a no-op on restarts.
	osdCount, err := vgsIntValue(context, vgName, "lv_count")
	if err != nil {
		return err
	}
	totalExtents, err := vgsIntValue(context, vgName, "vg_extent_count")
	if err != nil {
		return err
	}
	if err := context.Executor.ExecuteCommand("lvextend", "-l", strconv.Itoa(totalExtents/osdCount), args.BlockPath); err != nil {
		logger.Debugf("ignoring lvextend of %q, the logical volume likely already spans its share of the volume group. %v", args.BlockPath, err)
	}

	blockTarget, err := os.Readlink(filepath.Join(dataDir, "block"))
	if err != nil {
		return errors.Wrapf(err, "failed to resolve the encrypted block of osd.%s", args.ID)
	}
	dmName := filepath.Base(blockTarget)

	keyFile, err := os.CreateTemp("", ".luks_key")
	if err != nil {
		return errors.Wrap(err, "failed to create a temporary luks key file")
	}
	defer os.Remove(keyFile.Name())

	lockboxUser := fmt.Sprintf("client.osd-lockbox.%s", args.UUID)
	key, err := context.Executor.ExecuteCommandWithOutput("ceph",
		"--cluster", "ceph",
		"--name", lockboxUser,
		"--keyring", filepath.Join(dataDir, "lockbox.keyring"),
		"config-key", "get", fmt.Sprintf("dm-crypt/osd/%s/luks", args.UUID))
	if err != nil {
		keyFile.Close()
		return errors.Wrapf(err, "failed to get the luks key of osd.%s", args.ID)
	}
	if _, err := keyFile.WriteString(key); err != nil {
		keyFile.Close()
		return errors.Wrap(err, "failed to write the luks key file")
	}
	if err := keyFile.Close(); err != nil {
		return errors.Wrap(err, "failed to close the luks key file")
	}

	if err := context.Executor.ExecuteCommand("cryptsetup", "--verbose", "resize", dmName, "--key-file", keyFile.Name()); err != nil {
		return errors.Wrapf(err, "failed to resize the encrypted device %q", dmName)
	}
	return nil
}

func vgsIntValue(context *clusterd.Context, vgName, field string) (int, error) {
	out, err := context.Executor.ExecuteCommandWithOutput("vgs", "--noheadings", "--nosuffix", "-o", field, vgName)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to read %q of volume group %q: %s", field, vgName, out)
	}
	value, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return 0, errors.Wrapf(err, "failed to parse %q of volume group %q", field, vgName)
	}
	return value, nil
}

// preserveActivationFiles copies the OSD data dir aside, unmounts the tmpfs that
// "ceph-volume lvm activate" mounted on it, and copies the content back. When the init container
// exits the tmpfs goes away and its content with it, which would leave the empty dir shared with
// the osd container empty.
func preserveActivationFiles(context *clusterd.Context, dataDir string) error {
	tmpDir, err := os.MkdirTemp("", "activate")
	if err != nil {
		return errors.Wrap(err, "failed to create a temporary activation dir")
	}
	defer os.RemoveAll(tmpDir)

	if err := copyDirEntries(context, dataDir, tmpDir); err != nil {
		return err
	}
	if err := context.Executor.ExecuteCommand("umount", dataDir); err != nil {
		return errors.Wrapf(err, "failed to unmount the activation tmpfs from %q", dataDir)
	}
	if err := copyDirEntries(context, tmpDir, dataDir); err != nil {
		return err
	}
	if err := context.Executor.ExecuteCommand("chown", "--verbose", "--recursive", "ceph:ceph", dataDir); err != nil {
		return errors.Wrapf(err, "failed to chown %q", dataDir)
	}
	return nil
}

func copyDirEntries(context *clusterd.Context, srcDir, destDir string) error {
	entries, err := filepath.Glob(filepath.Join(srcDir, "*"))
	if err != nil {
		return errors.Wrapf(err, "failed to list %q", srcDir)
	}
	if len(entries) == 0 {
		return nil
	}
	args := append([]string{"--verbose", "--no-dereference"}, entries...)
	args = append(args, destDir+"/")
	if err := context.Executor.ExecuteCommand("cp", args...); err != nil {
		return errors.Wrapf(err, "failed to copy the content of %q to %q", srcDir, destDir)
	}
	return nil
}

func activateRaw(context *clusterd.Context, args ActivateOSDArgs, dataDir string) error {
	device, err := findOSDDevice(context, args.ID, args.BlockPath)
	if err != nil {
		return err
	}

	// If a kernel device name change happens and the block device file in the OSD directory
	// becomes missing, this OSD fails to start continuously. Remove the stale symlink so
	// "ceph-volume raw activate" recreates it against the current device.
	removeStaleBlockSymlink(dataDir, device)

	// ceph-volume raw mode only supports bluestore so we don't need to pass a store flag
	output, err := context.Executor.ExecuteCommandWithCombinedOutput("ceph-volume", "raw", "activate", "--device", device, "--no-systemd", "--no-tmpfs")
	logger.Infof("ceph-volume raw activate for osd.%s: %s", args.ID, output)
	if err != nil {
		return errors.Wrapf(err, "failed to activate osd.%s with ceph-volume raw: %s", args.ID, output)
	}
	return nil
}

// findOSDDevice resolves the block device currently holding this OSD. "ceph-volume raw list"
// (which the osd-prepare job uses to report OSDs on nodes) returns user-friendly device names
// which can change when systems reboot, so when the persisted path no longer reports the OSD,
// every candidate block device is listed until the OSD id is found.
func findOSDDevice(context *clusterd.Context, osdID, blockPath string) (string, error) {
	wantID, err := strconv.Atoi(osdID)
	if err != nil {
		return "", errors.Wrapf(err, "invalid OSD id %q", osdID)
	}

	if blockPath != "" {
		if device, found := osdDeviceFromListing(context, wantID, blockPath); found {
			return device, nil
		}
		logger.Infof("osd.%d not reported on %q, the device may be renamed. scanning all devices", wantID, blockPath)
	}

	devices, err := scanDeviceList(context)
	if err != nil {
		return "", err
	}
	if len(devices) == 0 {
		return "", errors.Errorf("no devices to scan for OSD %d", wantID)
	}

	// List the devices one at a time: ceph-volume folds a multi-device argument list into a
	// single ceph-bluestore-tool call and reports {} for the entire scan when that one process
	// fails, so a single unreadable device would otherwise hide every OSD from the scan. On
	// Ceph v20.2.x, sub-4KiB devices (e.g. the 1KiB extended-partition node of an
	// MBR-partitioned OS disk) crash the batched call exactly that way
	// (https://tracker.ceph.com/issues/76354).
	for _, device := range devices {
		if found, ok := osdDeviceFromListing(context, wantID, device); ok {
			return found, nil
		}
	}
	return "", errors.Errorf("no device found with OSD ID %d", wantID)
}

// osdDeviceFromListing reports the device on which "ceph-volume raw list <device>" sees the
// wanted OSD id. Listing failures and unusable report entries only disqualify the given device,
// never the overall scan.
func osdDeviceFromListing(context *clusterd.Context, wantID int, device string) (string, bool) {
	output, err := context.Executor.ExecuteCommandWithOutput("ceph-volume", "raw", "list", device)
	if err != nil {
		// the failure output carries ceph-volume's stderr, which is the only diagnostic of why a
		// device could not be listed during a rename fallback
		logger.Infof("failed to list %q: %s. %v", device, output, err)
		return "", false
	}
	logger.Debugf("listing of %q: %s", device, output)

	var entries map[string]rawListEntry
	if err := json.Unmarshal([]byte(output), &entries); err != nil {
		logger.Infof("failed to parse the listing of %q: %s. %v", device, output, err)
		return "", false
	}

	for _, entry := range entries {
		if entry.OsdID != nil && *entry.OsdID == wantID && entry.Device != "" {
			logger.Infof("found device: %s", entry.Device)
			return entry.Device, true
		}
	}
	return "", false
}

// scanDeviceList enumerates the whole disks and partitions to scan in the rename fallback,
// excluding the device types Rook never provisions OSDs on (see scanDeviceExclusionPattern).
func scanDeviceList(context *clusterd.Context) ([]string, error) {
	output, err := context.Executor.ExecuteCommandWithOutput("lsblk", "--noheadings", "--paths", "--list", "--output", "NAME,TYPE")
	if err != nil {
		return nil, errors.Wrap(err, "failed to list the block devices to scan")
	}

	var devices []string
	for line := range strings.SplitSeq(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || (fields[1] != "disk" && fields[1] != "part") {
			continue
		}
		if scanDeviceExclusionPattern.MatchString(fields[0]) {
			continue
		}
		devices = append(devices, fields[0])
	}
	return devices, nil
}

func removeStaleBlockSymlink(dataDir, device string) {
	blockPath := filepath.Join(dataDir, "block")
	info, err := os.Lstat(blockPath)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return
	}
	target, err := os.Readlink(blockPath)
	if err != nil || target == device {
		return
	}
	logger.Infof("removing stale block symlink %q pointing to %q instead of %q", blockPath, target, device)
	if err := os.Remove(blockPath); err != nil {
		logger.Warningf("failed to remove the stale block symlink %q. %v", blockPath, err)
	}
}
