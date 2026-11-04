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

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/util/log"
	"github.com/rook/rook/pkg/util/sys"
)

// bluestoreLabel is the part of the `ceph-bluestore-tool show-label` output that ties a device to an OSD.
// An interrupted mkfs can leave osd_uuid set while whoami and ceph_fsid are still missing.
type bluestoreLabel struct {
	OsdUUID  string `json:"osd_uuid"`
	CephFsid string `json:"ceph_fsid"`
	Whoami   string `json:"whoami"`
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

	encryptedBlocks, err := GetEncryptedBlocksForBackingDevice(context, disk.RealPath)
	if err != nil {
		return false, errors.Wrapf(err, "failed to get encrypted blocks for device %q", disk.RealPath)
	}
	if len(encryptedBlocks) != 1 {
		return false, errors.Errorf("expected one encrypted block for device %q, found %d", disk.RealPath, len(encryptedBlocks))
	}

	encryptedBlock := encryptedBlocks[0]
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

	safeToWipe, err := client.OsdSafeToDestroy(context, a.clusterInfo, osdID)
	if err != nil {
		return false, errors.Wrapf(err, "failed to check whether osd.%d on device %q is safe to destroy", osdID, diskPath)
	}

	if !safeToWipe {
		logger.Debugf("it is not safeToWipe the osd %d on disk %q", osdID, diskPath)
		return false, nil
	}

	// The OSD id was created by `ceph osd new` before the preparation was interrupted. The mons only purge it when
	// the OSD never booted, so this is also the check that the device holds no data.
	logger.Infof("device %q holds the incomplete osd.%d (uuid %q) of this cluster. purging the OSD so that the device can be wiped", diskPath, osdID, label.OsdUUID)
	if err := client.PurgeNewOSD(context, a.clusterInfo, osdID); err != nil {
		logger.Warningf("leaving device %q alone, osd.%d is not a new OSD that can be purged. %v", diskPath, osdID, err)
		return false, nil
	}

	return true, nil
}

// readBluestoreLabel returns the label of the main bluestore device.
func readBluestoreLabel(context *clusterd.Context, device string) (*bluestoreLabel, error) {
	output, err := context.Executor.ExecuteCommandWithCombinedOutput("ceph-bluestore-tool", "show-label", "--dev", device)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read the bluestore label of %q. %s", device, output)
	}

	logger.Debugf("%v", output)

	var labels map[string]bluestoreLabel
	if err := json.Unmarshal([]byte(output), &labels); err != nil {
		return nil, errors.Wrapf(err, "failed to parse the bluestore label of %q", device)
	}
	label, ok := labels[device]
	if !ok {
		return nil, errors.Errorf("the bluestore label output does not describe labels for %q. %s", device, output)
	}
	return &label, nil
}
