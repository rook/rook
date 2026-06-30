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
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
)

// cvLVMListEntry is one entry in the output of `ceph-volume lvm list <id>`.
type cvLVMListEntry struct {
	Type   string  `json:"type"`    // "block", "db" or "wal"
	VGName string  `json:"vg_name"` // volume group of the LV
	LVName string  `json:"lv_name"` // logical volume name
	LVUUID string  `json:"lv_uuid"` // LVM uuid of the LV; in lvm mode this is also the dm-crypt mapping name
	Path   string  `json:"path"`    // /dev/<vg>/<lv>
	Tags   osdTags `json:"tags"`    // ceph.* tags, including ceph.encrypted
}

// cephVolumeLVMList runs `ceph-volume lvm list <id>` and returns the entries reported for that osd id
// (empty when the id has no lvm volumes).
func cephVolumeLVMList(context *clusterd.Context, osdID int) ([]cvLVMListEntry, error) {
	result, err := callCephVolume(context, "lvm", "list", strconv.Itoa(osdID), "--format", "json")
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list lvm volumes for osd.%d", osdID)
	}
	var listResult map[string][]cvLVMListEntry
	if err := json.Unmarshal([]byte(result), &listResult); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal ceph-volume lvm list result for osd.%d. %s", osdID, result)
	}
	return listResult[strconv.Itoa(osdID)], nil
}

// CloseEncryptedDevicesForOSD closes the host dm-crypt mappings backing the encrypted OSD with the
// given id. Mappings already closed are treated as success.
func CloseEncryptedDevicesForOSD(context *clusterd.Context, osdID int) error {
	dmNames, err := encryptedDMNamesForOSD(context, osdID)
	if err != nil {
		return errors.Wrapf(err, "failed to resolve dm-crypt mappings for osd.%d", osdID)
	}

	if len(dmNames) == 0 {
		logger.Infof("no dm-crypt mappings found for osd.%d, nothing to close", osdID)
		return nil
	}

	for _, dmName := range dmNames {
		logger.Infof("closing dm-crypt mapping %q for osd.%d", dmName, osdID)
		if err := CloseEncryptedDevice(context, dmName); err != nil {
			if isCryptsetupNotActive(err) {
				logger.Infof("dm-crypt mapping %q for osd.%d is already closed", dmName, osdID)
				continue
			}
			return errors.Wrapf(err, "failed to close dm-crypt mapping %q for osd.%d", dmName, osdID)
		}
	}

	return nil
}

// encryptedDMNamesForOSD returns the host dm-crypt mapping names for the OSD with the given id,
// resolved from `ceph-volume lvm list <id>`. In lvm mode the mapping name is the LV's lv_uuid.
func encryptedDMNamesForOSD(context *clusterd.Context, osdID int) ([]string, error) {
	entries, err := cephVolumeLVMList(context, osdID)
	if err != nil {
		return nil, err
	}

	dmNames := []string{}
	for _, entry := range entries {
		if entry.Tags.Encrypted != "1" {
			continue
		}
		if entry.LVUUID == "" {
			logger.Warningf("ceph-volume lvm list returned an encrypted %q entry for osd.%d with no lv_uuid; skipping", entry.Type, osdID)
			continue
		}
		dmNames = append(dmNames, entry.LVUUID)
	}

	return dmNames, nil
}

// isCryptsetupNotActive reports whether err is cryptsetup's "already closed" failure (exit code 4),
// which is treated as success so closing is idempotent. The message substring is a locale-dependent
// fallback to the exit code.
func isCryptsetupNotActive(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "exit status 4") || strings.Contains(msg, "is not active")
}

// preProvisionReplacedOSDs pairs each destroyed slot belonging to this node with a freshly-swapped
// blank device and re-provisions it into the same slot via `ceph-volume ... prepare --osd-id <id>`,
// reusing the surviving DB LV in place when present. Devices consumed here are removed from
// available so the normal provisioning path does not also claim them (which would create a brand-new
// OSD id instead of reusing the destroyed slot).
func (a *OsdAgent) preProvisionReplacedOSDs(context *clusterd.Context, available *DeviceOsdMapping, crushLocation string) error {
	if a.pvcBacked {
		return nil
	}
	// a.migrateOSD is set by the OSD-migration flow, which reprovisions an OSD in place on config
	// changes (e.g. enabling encryption). Skip replacement provisioning when migration owns this prepare job.
	if a.migrateOSD != nil {
		return nil
	}

	// Fetch the osd tree once and derive both the destroyed set and the per-node filter from it.
	tree, err := client.HostTree(context, a.clusterInfo)
	if err != nil {
		return errors.Wrap(err, "failed to get osd tree")
	}

	destroyedOSDIds := tree.GetDestroyedIDs()
	destroyedOSDIds, err = filterDestroyedOSDIdsForNode(tree, destroyedOSDIds, crushLocation)
	if err != nil {
		return errors.Wrap(err, "failed to filter destroyed osd ids for this node")
	}
	if len(destroyedOSDIds) == 0 {
		logger.Debug("no destroyed osd slots belong to this node, skipping replacement provisioning")
		return nil
	}
	// Sort so the slot<->device pairing is reproducible across reconciles and operator restarts.
	sort.Ints(destroyedOSDIds)
	logger.Infof("destroyed osd slots on this node eligible for replacement: %v", destroyedOSDIds)

	// ceph-volume prepare needs the OSD bootstrap keyring to claim the slot via `ceph osd new`.
	// configureCVDevices also creates it but runs later, so ensure it exists here (idempotent).
	if err := createOSDBootstrapKeyring(context, a.clusterInfo, cephConfigDir); err != nil {
		return errors.Wrap(err, "failed to generate osd bootstrap keyring")
	}

	// A device is only offered once it is empty, so an available blank device is the signal that the
	// disk has been swapped.
	blankDevices := availableDataDevices(available)
	for _, osdID := range destroyedOSDIds {
		if len(blankDevices) == 0 {
			logger.Infof("no blank device available yet to replace destroyed osd.%d, waiting for the disk to be swapped", osdID)
			continue
		}
		// select the first blank device from the list:
		deviceName := blankDevices[0]
		blankDevices = blankDevices[1:]

		// Consume the device up front so the normal provisioning path never claims it as a brand-new
		// OSD, whether or not this slot succeeds.
		entry := available.Entries[deviceName]
		delete(available.Entries, deviceName)

		// provision selected blank device into destroyed osdID slot:
		if err := a.provisionReplacedOSD(context, osdID, entry); err != nil {
			// Skip this slot on failure so one bad slot does not block the other slots or the normal
			// provisioning that runs after this. The destroyed slot stays destroyed and is retried on
			// the next reconcile.
			logger.Errorf("failed to provision replacement for destroyed osd.%d on device %q, skipping. %v", osdID, deviceName, err)
			continue
		}
		logger.Infof("provisioned replacement osd.%d on device %q", osdID, deviceName)
	}

	return nil
}

// availableDataDevices returns the names of blank, unassigned data devices, sorted so the
// slot<->device pairing is deterministic across reconciles and operator restarts.
func availableDataDevices(available *DeviceOsdMapping) []string {
	devices := make([]string, 0, len(available.Entries))
	for name, entry := range available.Entries {
		// A non-nil Metadata slice (even if empty) marks a device reserved as a metadata/wal device by
		// getAvailableDevices; Data != unassignedOSDID means a data device already has an OSD on it.
		if entry.Metadata == nil && entry.Data == unassignedOSDID {
			devices = append(devices, name)
		}
	}
	sort.Strings(devices)
	return devices
}

// filterDestroyedOSDIdsForNode narrows the cluster-wide destroyed slots to the ones whose CRUSH host
// bucket matches this node, derived from the prepare job's crush location (`host=<name>`).
func filterDestroyedOSDIdsForNode(tree client.OsdTree, destroyedOSDIds []int, crushLocation string) ([]int, error) {
	if len(destroyedOSDIds) == 0 {
		return destroyedOSDIds, nil
	}
	hostName := crushHostFromLocation(crushLocation)
	if hostName == "" {
		return nil, errors.Errorf("failed to determine crush host from location %q", crushLocation)
	}

	// OSDs are direct children of their host bucket, so the host's children are its osd ids.
	hostOSDs := map[int]struct{}{}
	for _, node := range tree.Nodes {
		if node.Type == "host" && node.Name == hostName {
			for _, id := range node.Children {
				hostOSDs[id] = struct{}{}
			}
			break
		}
	}

	destroyedOsdsOnHost := []int{}
	for _, id := range destroyedOSDIds {
		if _, ok := hostOSDs[id]; ok {
			destroyedOsdsOnHost = append(destroyedOsdsOnHost, id)
		}
	}

	return destroyedOsdsOnHost, nil
}

// crushHostFromLocation extracts the host bucket name from a CRUSH location string of the form
// "root=default host=node-1 ...".
func crushHostFromLocation(crushLocation string) string {
	for _, token := range strings.Fields(crushLocation) {
		if strings.HasPrefix(token, "host=") {
			return strings.TrimPrefix(token, "host=")
		}
	}
	return ""
}

// provisionReplacedOSD runs `ceph-volume ... prepare --osd-id <id>` to bring the destroyed slot back
// on the supplied blank data device.
func (a *OsdAgent) provisionReplacedOSD(context *clusterd.Context, osdID int, entry *DeviceOsdIDEntry) error {
	dataDevice := path.Join("/dev", entry.DeviceInfo.Name)

	// Recover the surviving DB LV from the destroyed OSD itself, never from the spec; empty means the
	// OSD had no separate metadata device.
	dbLV, err := a.recoverDBLVForOSDFromHost(context, osdID)
	if err != nil {
		return errors.Wrapf(err, "failed to recover db lv for osd.%d", osdID)
	}

	// Raw mode is forced off by encryption, osdsPerDevice > 1, or a separate metadata device.
	allowRaw, err := a.allowRawMode(context)
	if err != nil {
		return errors.Wrap(err, "failed to determine ceph-volume mode")
	}
	useRaw := allowRaw && isSafeToUseRawMode(entry) && dbLV == ""

	baseCommand := "stdbuf"
	// Match callCephVolume's log path so the failure log read below is the one this command wrote.
	logPath := "/tmp/ceph-log"
	if err := os.MkdirAll(logPath, 0o700); err != nil {
		return errors.Wrapf(err, "failed to create dir %q", logPath)
	}

	if !useRaw {
		if err := lvmPreReq(context); err != nil {
			return errors.Wrap(err, "failed to run lvm prerequisites")
		}
	}

	args := a.buildReplacementPrepareArgs(osdID, dataDevice, dbLV, entry, useRaw, logPath)

	logger.Infof("provisioning replacement osd.%d: %s %v", osdID, baseCommand, args)
	op, err := context.Executor.ExecuteCommandWithCombinedOutput(baseCommand, args...)
	cvLog := readCVLogContent(path.Join(logPath, "ceph-volume.log"))
	if err != nil {
		if cvLog != "" {
			logger.Errorf("%s", cvLog)
		}
		return errors.Wrapf(err, "failed to run ceph-volume prepare for replacement osd.%d. %s", osdID, op)
	}
	if cvLog != "" {
		logger.Infof("%s", cvLog)
	}
	logger.Infof("%v", op)

	return nil
}

// buildReplacementPrepareArgs assembles the ceph-volume prepare invocation (the args after the
// "stdbuf" base command) for re-provisioning a destroyed slot into the given data device. The layout
// is selected by useRaw and the presence of a recovered DB LV:
//   - raw single-disk:               raw prepare --osd-id <id> --data <disk> [--crush-device-class]
//   - lvm single-disk:               lvm prepare --osd-id <id> --data <disk> [--crush-device-class]
//   - lvm shared-metadata (plain):   lvm prepare --osd-id <id> --data <disk> --block.db <vg/lv> [--crush-device-class]
//   - lvm shared-metadata (crypt):   lvm prepare --osd-id <id> --data <disk> --block.db <vg/lv> --dmcrypt [--crush-device-class]
//
// `lvm prepare` (not `lvm batch`) is required: batch reallocates the DB and refuses a surviving
// sibling LV, whereas prepare reuses the recovered DB LV in place.
func (a *OsdAgent) buildReplacementPrepareArgs(osdID int, dataDevice, dbLV string, entry *DeviceOsdIDEntry, useRaw bool, logPath string) []string {
	storeFlag := a.storeConfig.GetStoreFlag()
	cvMode := "lvm"
	if useRaw {
		cvMode = "raw"
	}

	args := []string{"-oL", cephVolumeCmd, "--log-path", logPath, cvMode, "prepare", storeFlag, "--osd-id", strconv.Itoa(osdID), dataFlag, dataDevice}

	if !useRaw {
		// Raw mode never has a separate DB device or encryption (those force lvm mode).
		if dbLV != "" {
			args = append(args, blockDBFlag, dbLV)
		}
		if a.storeConfig.EncryptedDevice {
			args = append(args, encryptedFlag)
		}
	}

	return a.appendDeviceClassArg(entry, args)
}

// recoverDBLVForOSDFromHost reads the destroyed OSD's surviving DB logical volume via `ceph-volume
// lvm list <id>` and returns it as a "vg/lv" reference suitable for `--block.db`. It returns an empty
// string when the OSD has no separate metadata device.
func (a *OsdAgent) recoverDBLVForOSDFromHost(context *clusterd.Context, osdID int) (string, error) {
	entries, err := cephVolumeLVMList(context, osdID)
	if err != nil {
		return "", err
	}

	for _, entry := range entries {
		if entry.Type != "db" {
			continue
		}
		if entry.VGName != "" && entry.LVName != "" {
			return fmt.Sprintf("%s/%s", entry.VGName, entry.LVName), nil
		}
		if entry.Path != "" {
			return entry.Path, nil
		}
	}

	return "", nil
}
