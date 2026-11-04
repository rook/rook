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
	"strings"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	oposd "github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	"github.com/rook/rook/pkg/util/log"
	"github.com/rook/rook/pkg/util/sys"
)

var (
	errUnmarshalBluestoreLabel = errors.New("failed to unmarshal bluestore label")
	errOpenEncryptedDevice     = errors.New("failed to open encrypted device")
)

// bluestoreLabel is the part of the `ceph-bluestore-tool show-label` output that ties a device to an OSD.
// An interrupted mkfs can leave osd_uuid set while whoami and ceph_fsid are still missing.
type bluestoreLabel struct {
	OsdUUID  string `json:"osd_uuid"`
	CephFsid string `json:"ceph_fsid"`
	Whoami   string `json:"whoami"`

	// Not used in the code. Logged at debug level to help troubleshoot an incomplete OSD.
	CreatedAt string   `json:"created_at"`
	Ready     string   `json:"ready"`
	Type      string   `json:"type"`
	Locations []string `json:"locations"`
}

func (a *OsdAgent) wipeIncompleteOSDDeviceOnPVC(context *clusterd.Context, existingOSDs map[string]osdInfoBlock) error {
	if !a.pvcBacked {
		return nil
	}
	var disk *sys.LocalDisk
	for _, d := range context.Devices {
		if d.Type == pvcDataTypeDevice {
			disk = d
			break
		}
	}

	// The initial device discovery can report an empty filesystem for a device that has a signature,
	// because lsblk reads from the udev database and it can be out of date. Refresh udev and read the
	// filesystem again before deciding whether this PVC can be reclaimed.
	if disk != nil && disk.Filesystem == "" {
		filesystem, err := getDeviceFileSystem(context, disk.RealPath)
		if err != nil {
			return errors.Wrapf(err, "failed to get filesystem on device %q", disk.RealPath)
		}
		disk.Filesystem = filesystem
	}

	// Only a PVC that a prepare job started on can be reclaimed: mkfs writes the bluestore signature first, and
	// luksFormat the LUKS one. A blank or mounted PVC, or one with another filesystem, is not ours to touch.
	if disk == nil || disk.Mountpoint != "" || (disk.Filesystem != "ceph_bluestore" && disk.Filesystem != "crypto_LUKS") {
		logger.Debugf("skipping incomplete OSD cleanup: PVC data device is missing, mounted, or has no BlueStore or LUKS signature. device: %+v", disk)
		return nil
	}
	// A healthy OSD has the same signature as an incomplete one. If ceph-volume lists an OSD of this cluster on the
	// PVC, the OSD is complete and in use, so stop here before fetching the osdmap or touching the encrypted block.
	// Entries of other clusters, and db or wal entries which have no device path, cannot be that OSD.
	for _, existingOSD := range existingOSDs {
		isOurOSD := existingOSD.CephFsid == a.clusterInfo.FSID
		hasDevicePath := existingOSD.Device != ""
		if !isOurOSD || !hasDevicePath {
			continue
		}
		if osdDisk, _, err := getOSDDiskToBeWiped(context, existingOSD.Device); err == nil && osdDisk == disk {
			logger.Debugf("device %q holds osd.%d of this cluster, nothing to reclaim", disk.RealPath, existingOSD.OsdID)
			return nil
		}
	}

	osdDump, err := client.GetOSDDump(context, a.clusterInfo)
	if err != nil {
		return errors.Wrap(err, "failed to get the osdmap to check the device with an incomplete OSD")
	}

	_ = log.LogAsJSON(logger.Debugf, "OSD dump", osdDump)

	wipe, err := a.releaseIncompleteOSD(context, osdDump, disk)
	if err != nil {
		return errors.Wrapf(err, "failed to release incomplete OSD on device %q", disk.RealPath)
	}
	if !wipe {
		return nil
	}

	// The db and wal PVCs were prepared with the data PVC and carry a label with the same osd_uuid. Left alone, they
	// would be skipped as devices with a filesystem and the OSD would be prepared without them.
	// Wipe the data PVC last so its signature still triggers recovery if another device fails to wipe.
	devicesToWipe := make([]*sys.LocalDisk, 0, len(context.Devices))
	for _, d := range context.Devices {
		if d == disk || d.Filesystem == "" {
			continue
		}
		devicesToWipe = append(devicesToWipe, d)
	}
	devicesToWipe = append(devicesToWipe, disk)
	for _, d := range devicesToWipe {
		logger.Infof("wiping %s device %q so that the OSD can be prepared again", d.Type, d.RealPath)
		if err := ZapDevice(context, d.RealPath); err != nil {
			return errors.Wrapf(err, "failed to wipe %s device %q", d.Type, d.RealPath)
		}
		// Clear the stale filesystem so the device is not filtered out when the available devices are selected.
		d.Filesystem = ""
	}
	return nil
}

// releaseIncompleteOSD reports whether the data PVC may be wiped. For encrypted
// PVCs, it inspects the existing mapper device and closes it only if wiping is allowed.
func (a *OsdAgent) releaseIncompleteOSD(context *clusterd.Context, osdDump *client.OSDDump, disk *sys.LocalDisk) (bool, error) {
	if disk.Filesystem != "crypto_LUKS" {
		return a.canWipeOSDOnDevice(context, osdDump, disk.RealPath, disk.RealPath)
	}

	encryptedBlock, err := openEncryptedPVCWithClusterKey(context, disk)
	if err != nil {
		return false, err
	}

	wipe, err := a.canWipeOSDOnDevice(context, osdDump, disk.RealPath, encryptedBlock)
	if err != nil || !wipe {
		return false, err
	}
	if err := RemoveEncryptedDevice(context, encryptedBlock); err != nil {
		return false, errors.Wrapf(err, "failed to close encrypted block %q for device %q", encryptedBlock, disk.RealPath)
	}

	return true, nil
}

// canWipeOSDOnDevice reads the bluestore label of the device and asks ceph whether the OSD it names can be given up.
func (a *OsdAgent) canWipeOSDOnDevice(context *clusterd.Context, osdDump *client.OSDDump, diskPath, device string) (bool, error) {
	label, err := readBluestoreLabel(context, device)
	if err != nil {
		if errors.Is(err, errUnmarshalBluestoreLabel) {
			return false, errors.Wrapf(err, "cannot determine whether device %q is safe to wipe: invalid label JSON", diskPath)
		}
		logger.Infof("device %q has a ceph signature but no readable bluestore label, so it cannot hold an OSD of this cluster. wiping it. %v", diskPath, err)
		return true, nil
	}

	if label.CephFsid == a.clusterInfo.FSID && label.Whoami != "" {
		logger.Infof("device %q holds the complete osd.%s of this cluster, leaving it alone", diskPath, label.Whoami)
		return false, nil
	}
	osdID := osdDump.GetOSDIDByUUID(label.OsdUUID)
	if osdID < 0 {
		logger.Infof("device %q holds an incomplete OSD with uuid %q that is unknown to this cluster. wiping it", diskPath, label.OsdUUID)
		return true, nil
	}

	// The OSD id was created by `ceph osd new` before the preparation was interrupted. The mons only purge it when
	// the OSD never booted, so this is also the check that the device holds no data.
	logger.Infof("device %q holds the incomplete osd.%d (uuid %q) of this cluster. purging the OSD so that the device can be wiped", diskPath, osdID, label.OsdUUID)
	if err := client.PurgeNewOSD(context, a.clusterInfo, osdID); err != nil {
		logger.Warningf("failed to purge new osd.%d on device %q: %v", osdID, diskPath, err)
		return false, nil
	}

	return true, nil
}

// readBluestoreLabel returns the label of the main bluestore device.
func readBluestoreLabel(context *clusterd.Context, device string) (*bluestoreLabel, error) {
	output, err := context.Executor.ExecuteCommandWithOutput("ceph-bluestore-tool", "show-label", "--dev", device)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read the bluestore label of %q", device)
	}

	var labels map[string]bluestoreLabel
	if err := json.Unmarshal([]byte(output), &labels); err != nil {
		return nil, fmt.Errorf("%w for %q: %w", errUnmarshalBluestoreLabel, device, err)
	}
	_ = log.LogAsJSON(logger.Debugf, "OSD bluestore labels", labels)

	label, ok := labels[device]
	if !ok {
		return nil, errors.Errorf("the bluestore label output does not describe labels for %q", device)
	}
	return &label, nil
}

func openEncryptedPVCWithClusterKey(context *clusterd.Context, disk *sys.LocalDisk) (string, error) {
	passphrase := os.Getenv(oposd.CephVolumeEncryptedKeyEnvVarName)
	pvcName := os.Getenv(oposd.PVCNameEnvVarName)
	if passphrase == "" || pvcName == "" {
		return "", errors.Wrapf(errOpenEncryptedDevice, "device %q: encryption key or PVC name is missing", disk.RealPath)
	}
	target := oposd.EncryptionDMName(pvcName, oposd.DmcryptBlockType)
	// close a stale block left behind by an earlier attempt
	if err := RemoveEncryptedDevice(context, target); err != nil {
		logger.Debugf("no stale dm device %q to remove. %v", target, err)
	}
	if err := openEncryptedDevice(context, disk.RealPath, target, passphrase); err != nil {
		return "", fmt.Errorf("%w %q with the key of this cluster: %w", errOpenEncryptedDevice, disk.RealPath, err)
	}
	return oposd.EncryptionDMPath(pvcName, oposd.DmcryptBlockType), nil
}

func getDeviceFileSystem(context *clusterd.Context, devicePath string) (string, error) {
	// lsblk reads filesystem information from the udev database, which may not yet
	// reflect recently added or modified devices. Run udevadm settle before lsblk
	// to wait for pending udev events to finish.
	// See https://man7.org/linux/man-pages/man8/lsblk.8.html

	output, err := context.Executor.ExecuteCommandWithOutput(
		"lsblk", "--nodeps", "--noheadings", "--raw", "--output", "KNAME", devicePath)
	if err != nil {
		return "", fmt.Errorf("failed to get kernel name for %q: %w", devicePath, err)
	}

	kernelName := strings.TrimSpace(output)
	if kernelName == "" {
		return "", fmt.Errorf("empty kernel name for %q", devicePath)
	}

	output, err = context.Executor.ExecuteCommandWithOutput(
		"udevadm", "trigger", "--action=change", "/sys/class/block/"+kernelName)
	if err != nil {
		return "", fmt.Errorf("failed to trigger udev for %q: %s: %w", devicePath, output, err)
	}

	output, err = context.Executor.ExecuteCommandWithOutput("udevadm", "settle")
	if err != nil {
		return "", fmt.Errorf("failed to settle udev for %q: %s: %w", devicePath, output, err)
	}

	output, err = context.Executor.ExecuteCommandWithOutput("lsblk", devicePath,
		"--noheadings", "--nodeps", "--output", "FSTYPE")
	if err != nil {
		logger.Errorf("failed to execute lsblk. output: %s", output)
		return "", err
	}
	logger.Debugf("filesystem on disk %q is: %q", devicePath, output)

	return strings.TrimSpace(output), nil
}
