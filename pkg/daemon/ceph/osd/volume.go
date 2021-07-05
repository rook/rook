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

package osd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	oposd "github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/util/display"
	"github.com/rook/rook/pkg/util/sys"
)

const (
	osdsPerDeviceFlag    = "--osds-per-device"
	crushDeviceClassFlag = "--crush-device-class"
	encryptedFlag        = "--dmcrypt"
	databaseSizeFlag     = "--block-db-size"
	dbDeviceFlag         = "--db-devices"
	cephVolumeCmd        = "ceph-volume"
	cephVolumeMinDBSize  = 1024 // 1GB
)

// These are not constants because they are used by the tests
var (
	cephConfigDir = "/var/lib/ceph"
	cephLogDir    = "/var/log/ceph"
	lvmConfPath   = "/etc/lvm/lvm.conf"
	cvLogDir      = ""
	// The "ceph-volume raw" command is available since Ceph 14.2.8 as well as partition support in ceph-volume
	cephVolumeRawModeMinCephVersion = cephver.CephVersion{Major: 14, Minor: 2, Extra: 8}
	// The Ceph Nautilus to include a retry to acquire device lock
	cephFlockFixNautilusMinCephVersion = cephver.CephVersion{Major: 14, Minor: 2, Extra: 14}
	// The Ceph Octopus to include a retry to acquire device lock
	cephFlockFixOctopusMinCephVersion = cephver.CephVersion{Major: 15, Minor: 2, Extra: 9}
	isEncrypted                       = os.Getenv(oposd.EncryptedDeviceEnvVarName) == "true"
	isOnPVC                           = os.Getenv(oposd.PVCBackedOSDVarName) == "true"
)

type osdInfoBlock struct {
	CephFsid string `json:"ceph_fsid"`
	Device   string `json:"device"`
	OsdID    int    `json:"osd_id"`
	OsdUUID  string `json:"osd_uuid"`
	Type     string `json:"type"`
}

type osdInfo struct {
	Name string  `json:"name"`
	Path string  `json:"path"`
	Tags osdTags `json:"tags"`
	// "block" for bluestore
	Type string `json:"type"`
}

type osdTags struct {
	OSDFSID          string `json:"ceph.osd_fsid"`
	Encrypted        string `json:"ceph.encrypted"`
	ClusterFSID      string `json:"ceph.cluster_fsid"`
	CrushDeviceClass string `json:"ceph.crush_device_class"`
}

type cephVolReport struct {
	Changed bool      `json:"changed"`
	Vg      cephVolVg `json:"vg"`
}

type cephVolVg struct {
	Devices string `json:"devices"`
}

type cephVolReportV2 struct {
	BlockDB      string `json:"block_db"`
	Encryption   string `json:"encryption"`
	Data         string `json:"data"`
	DatabaseSize string `json:"data_size"`
	BlockDbSize  string `json:"block_db_size"`
}

func isNewStyledLvmBatch(version cephver.CephVersion) bool {
	if version.IsNautilus() && version.IsAtLeast(cephver.CephVersion{Major: 14, Minor: 2, Extra: 13}) {
		return true
	}

	if version.IsOctopus() && version.IsAtLeast(cephver.CephVersion{Major: 15, Minor: 2, Extra: 8}) {
		return true
	}

	if version.IsAtLeastPacific() {
		return true
	}

	return false
}

func (a *OsdAgent) configureCVDevices(context *clusterd.Context, devices *DeviceOsdMapping) ([]oposd.OSDInfo, error) {
	var osds []oposd.OSDInfo
	var lvmOsds []oposd.OSDInfo
	var rawOsds []oposd.OSDInfo
	var lvBackedPV bool
	var block, lvPath, metadataBlock, walBlock string
	var err error

	// Idempotency check, if the device list is empty devices have been prepared already
	// In this case, just return the OSDInfo via a 'ceph-volume lvm|raw list' call
	if len(devices.Entries) == 0 {
		logger.Info("no new devices to configure. returning devices already configured with ceph-volume.")

		if a.pvcBacked {
			// So many things changed and it's good to remember this commit and its logic
			// See: https://github.com/rook/rook/commit/8ea693a74011c587970dfc28a3d9efe2ef329159
			skipLVRelease := true

			// For LV mode
			lvPath = getDeviceLVPath(context, fmt.Sprintf("/mnt/%s", a.nodeName))
			lvBackedPV, err := sys.IsLV(fmt.Sprintf("/mnt/%s", a.nodeName), context.Executor)
			if err != nil {
				return nil, errors.Wrap(err, "failed to check device type")
			}

			// List THE existing OSD configured with ceph-volume lvm mode
			lvmOsds, err = GetCephVolumeLVMOSDs(context, a.clusterInfo, a.clusterInfo.FSID, lvPath, skipLVRelease, lvBackedPV)
			if err != nil {
				logger.Infof("failed to get device already provisioned by ceph-volume lvm. %v", err)
			}
			osds = append(osds, lvmOsds...)
			if len(osds) > 0 {
				// "ceph-volume raw list" lists the existing OSD even if it is configured with lvm mode, so escape here to avoid dupe.
				return osds, nil
			}

			// List THE existing OSD configured with ceph-volume raw mode
			if a.clusterInfo.CephVersion.IsAtLeast(cephVolumeRawModeMinCephVersion) {
				// For block mode
				block = fmt.Sprintf("/mnt/%s", a.nodeName)

				// This is hard to determine a potential metadata device here
				// Also, I don't think (leseb) this code we have run in this condition
				// I tried several things:
				//    * evict a node, osd moves, the prepare job was never relaunched ever because we check for the osd deployment and skip the prepare
				//    * restarted the operator, again the prepare job was not re-run
				//
				// I'm leaving this code with an empty metadata device for now
				metadataBlock, walBlock = "", ""

				rawOsds, err = GetCephVolumeRawOSDs(context, a.clusterInfo, a.clusterInfo.FSID, block, metadataBlock, walBlock, lvBackedPV, false)
				if err != nil {
					logger.Infof("failed to get device already provisioned by ceph-volume raw. %v", err)
				}
				osds = append(osds, rawOsds...)
			}

			return osds, nil
		}

		// List existing OSD(s) configured with ceph-volume lvm mode
		lvmOsds, err = GetCephVolumeLVMOSDs(context, a.clusterInfo, a.clusterInfo.FSID, lvPath, false, false)
		if err != nil {
			logger.Infof("failed to get devices already provisioned by ceph-volume. %v", err)
		}
		osds = append(osds, lvmOsds...)

		// List existing OSD(s) configured with ceph-volume raw mode
		rawOsds, err = GetCephVolumeRawOSDs(context, a.clusterInfo, a.clusterInfo.FSID, block, "", "", false, false)
		if err != nil {
			logger.Infof("failed to get device already provisioned by ceph-volume raw. %v", err)
		}
		osds = appendOSDInfo(osds, rawOsds)

		return osds, nil
	}

	// Create OSD bootstrap keyring
	err = createOSDBootstrapKeyring(context, a.clusterInfo, cephConfigDir)
	if err != nil {
		return nil, errors.Wrap(err, "failed to generate osd keyring")
	}

	// Check if the PVC is an LVM block device (certain StorageClass do this)
	if a.pvcBacked {
		for _, device := range devices.Entries {
			dev := device.Config.Name
			lvBackedPV, err = sys.IsLV(dev, context.Executor)
			if err != nil {
				return nil, errors.Wrap(err, "failed to check device type")
			}
			break
		}
	}

	// Should we use ceph-volume raw mode?
	useRawMode, err := a.useRawMode(context, a.pvcBacked)
	if err != nil {
		return nil, errors.Wrap(err, "failed to determine which ceph-volume mode to use")
	}

	// If not raw mode we must execute a few LVM prerequisites
	if !useRawMode {
		err = lvmPreReq(context, a.pvcBacked, lvBackedPV)
		if err != nil {
			return nil, errors.Wrap(err, "failed to run lvm prerequisites")
		}
	}

	// If running on OSD on PVC
	if a.pvcBacked {
		if block, metadataBlock, walBlock, err = a.initializeBlockPVC(context, devices, lvBackedPV); err != nil {
			return nil, errors.Wrap(err, "failed to initialize devices on PVC")
		}
	} else {
		err := a.initializeDevices(context, devices, useRawMode)
		if err != nil {
			return nil, errors.Wrap(err, "failed to initialize osd")
		}
	}

	// List OSD configured with ceph-volume lvm mode
	lvmOsds, err = GetCephVolumeLVMOSDs(context, a.clusterInfo, a.clusterInfo.FSID, block, false, lvBackedPV)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get devices already provisioned by ceph-volume lvm")
	}
	osds = append(osds, lvmOsds...)

	// List THE configured OSD with ceph-volume raw mode
	// When the block is encrypted we need to list against the encrypted device mapper
	if !isEncrypted {
		block = fmt.Sprintf("/mnt/%s", a.nodeName)
	}
	// List ALL OSDs when not running on PVC
	if !a.pvcBacked {
		block = ""
	}
	rawOsds, err = GetCephVolumeRawOSDs(context, a.clusterInfo, a.clusterInfo.FSID, block, metadataBlock, walBlock, lvBackedPV, false)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get devices already provisioned by ceph-volume raw")
	}
	osds = appendOSDInfo(osds, rawOsds)

	return osds, err
}

func (a *OsdAgent) initializeBlockPVC(context *clusterd.Context, devices *DeviceOsdMapping, lvBackedPV bool) (string, string, string, error) {
	// we need to return the block if raw mode is used and the lv if lvm mode
	baseCommand := "stdbuf"
	var baseArgs []string

	cephVolumeMode := "lvm"
	if a.clusterInfo.CephVersion.IsAtLeast(cephVolumeRawModeMinCephVersion) {
		cephVolumeMode = "raw"
	} else if lvBackedPV {
		return "", "", "", errors.New("OSD on LV-backed PVC requires new Ceph to use raw mode")
	}

	// Create a specific log directory so that each prepare command will have its own log
	// Only do this if nothing is present so that we don't override existing logs
	cvLogDir = path.Join(cephLogDir, a.nodeName)
	err := os.MkdirAll(cvLogDir, 0750)
	if err != nil {
		logger.Errorf("failed to create ceph-volume log directory %q, continue with default %q. %v", cvLogDir, cephLogDir, err)
		baseArgs = []string{"-oL", cephVolumeCmd, cephVolumeMode, "prepare", "--bluestore"}
	} else {
		// Always force Bluestore!
		baseArgs = []string{"-oL", cephVolumeCmd, "--log-path", cvLogDir, cephVolumeMode, "prepare", "--bluestore"}
	}

	var metadataArg, walArg []string
	var metadataDev, walDev bool
	var blockPath, metadataBlockPath, walBlockPath string

	// Problem: map is an unordered collection
	// therefore the iteration order of a map is not guaranteed to be the same every time you iterate over it.
	// So we could first get the metadata device and then the main block in a scenario where a metadata PVC is present
	for name, device := range devices.Entries {
		// If this is the metadata device there is nothing to do
		// it'll be used in one of the iterations
		if name == "metadata" || name == "wal" {
			logger.Debugf("device %q is a metadata or wal device, skipping this iteration it will be used in the next one", device.Config.Name)
			// Don't do this device
			continue
		}

		// When running on PVC, the prepare job has a single OSD only so 1 disk
		// However we can present a metadata device so we need to consume it
		// This will make the devices.Entries larger than usual
		if _, ok := devices.Entries["metadata"]; ok {
			metadataDev = true
			metadataArg = append(metadataArg, []string{"--block.db",
				devices.Entries["metadata"].Config.Name,
			}...)

			metadataBlockPath = devices.Entries["metadata"].Config.Name
		}

		if _, ok := devices.Entries["wal"]; ok {
			walDev = true
			walArg = append(walArg, []string{"--block.wal",
				devices.Entries["wal"].Config.Name,
			}...)

			walBlockPath = devices.Entries["wal"].Config.Name
		}

		if device.Data == -1 {
			logger.Infof("configuring new device %q", device.Config.Name)
			var err error
			var deviceArg string

			deviceArg = device.Config.Name
			logger.Info("devlink names:")
			for _, devlink := range device.PersistentDevicePaths {
				logger.Info(devlink)
				if strings.HasPrefix(devlink, "/dev/mapper") {
					deviceArg = devlink
				}
			}

			immediateExecuteArgs := append(baseArgs, []string{
				"--data",
				deviceArg,
			}...)

			crushDeviceClass := os.Getenv(oposd.CrushDeviceClassVarName)
			if crushDeviceClass != "" {
				immediateExecuteArgs = append(immediateExecuteArgs, []string{crushDeviceClassFlag, crushDeviceClass}...)
			}

			if isEncrypted {
				immediateExecuteArgs = append(immediateExecuteArgs, encryptedFlag)
			}

			// Add the cli argument for the metadata device
			if metadataDev {
				immediateExecuteArgs = append(immediateExecuteArgs, metadataArg...)
			}

			// Add the cli argument for the wal device
			if walDev {
				immediateExecuteArgs = append(immediateExecuteArgs, walArg...)
			}

			// execute ceph-volume with the device
			op, err := context.Executor.ExecuteCommandWithCombinedOutput(baseCommand, immediateExecuteArgs...)
			if err != nil {
				cvLogFilePath := path.Join(cvLogDir, "ceph-volume.log")

				// Print c-v log before exiting
				cvLog := readCVLogContent(cvLogFilePath)
				if cvLog != "" {
					logger.Errorf("%s", cvLog)
				}

				// Return failure
				return "", "", "", errors.Wrapf(err, "failed to run ceph-volume. %s. debug logs below:\n%s", op, cvLog)
			}
			logger.Infof("%v", op)
			// if raw mode is used or PV on LV, let's return the path of the device
			if cephVolumeMode == "raw" && !isEncrypted {
				blockPath = deviceArg
			} else if cephVolumeMode == "raw" && isEncrypted {
				blockPath = getEncryptedBlockPath(op, oposd.DmcryptBlockType)
				if blockPath == "" {
					return "", "", "", errors.New("failed to get encrypted block path from ceph-volume lvm prepare output")
				}
				if metadataDev {
					metadataBlockPath = getEncryptedBlockPath(op, oposd.DmcryptMetadataType)
					if metadataBlockPath == "" {
						return "", "", "", errors.New("failed to get encrypted block.db path from ceph-volume lvm prepare output")
					}
				}
				if walDev {
					walBlockPath = getEncryptedBlockPath(op, oposd.DmcryptWalType)
					if walBlockPath == "" {
						return "", "", "", errors.New("failed to get encrypted block.wal path from ceph-volume lvm prepare output")
					}
				}
			} else {
				blockPath = getLVPath(op)
				if blockPath == "" {
					return "", "", "", errors.New("failed to get lv path from ceph-volume lvm prepare output")
				}
			}
		} else {
			logger.Infof("skipping device %q with osd %d already configured", device.Config.Name, device.Data)
		}
	}

	return blockPath, metadataBlockPath, walBlockPath, nil
}

func getLVPath(op string) string {
	tmp := sys.Grep(op, "Volume group")
	vgtmp := strings.Split(tmp, "\"")

	tmp = sys.Grep(op, "Logical volume")
	lvtmp := strings.Split(tmp, "\"")

	if len(vgtmp) >= 2 && len(lvtmp) >= 2 {
		if sys.Grep(vgtmp[1], "ceph") != "" && sys.Grep(lvtmp[1], "osd-block") != "" {
			return fmt.Sprintf("/dev/%s/%s", vgtmp[1], lvtmp[1])
		}
	}
	return ""
}

func getEncryptedBlockPath(op, blockType string) string {
	re := regexp.MustCompile("(?m)^.*luksOpen.*$")
	matches := re.FindAllString(op, -1)

	for _, line := range matches {
		lineSlice := strings.Fields(line)
		for _, word := range lineSlice {
			if strings.Contains(word, blockType) {
				return fmt.Sprintf("/dev/mapper/%s", word)
			}
		}
	}

	return ""
}

// UpdateLVMConfig updates the lvm.conf file
func UpdateLVMConfig(context *clusterd.Context, onPVC, lvBackedPV bool) error {

	input, err := ioutil.ReadFile(lvmConfPath)
	if err != nil {
		return errors.Wrapf(err, "failed to read lvm config file %q", lvmConfPath)
	}

	output := bytes.Replace(input, []byte("udev_sync = 1"), []byte("udev_sync = 0"), 1)
	output = bytes.Replace(output, []byte("allow_changes_with_duplicate_pvs = 0"), []byte("allow_changes_with_duplicate_pvs = 1"), 1)
	output = bytes.Replace(output, []byte("udev_rules = 1"), []byte("udev_rules = 0"), 1)
	output = bytes.Replace(output, []byte("use_lvmetad = 1"), []byte("use_lvmetad = 0"), 1)
	output = bytes.Replace(output, []byte("obtain_device_list_from_udev = 1"), []byte("obtain_device_list_from_udev = 0"), 1)

	// When running on PVC
	if onPVC {
		output = bytes.Replace(output, []byte(`scan = [ "/dev" ]`), []byte(`scan = [ "/dev", "/mnt" ]`), 1)
		// Only filter blocks in /mnt, when running on PVC we copy the PVC claim path to /mnt
		// And reject everything else
		// We have 2 different regex depending on the version of LVM present in the container...
		// Since https://github.com/lvmteam/lvm2/commit/08396b4bce45fb8311979250623f04ec0ddb628c#diff-13c602a6258e57ce666a240e67c44f38
		// the content changed, so depending which version is installled one of the two replace will work
		if lvBackedPV {
			// ceph-volume calls lvs to locate given "vg/lv", so allow "/dev" here. However, ignore loopback devices
			output = bytes.Replace(output, []byte(`# filter = [ "a|.*/|" ]`), []byte(`filter = [ "a|^/mnt/.*|", "r|^/dev/loop.*|", "a|^/dev/.*|", "r|.*|" ]`), 1)
			output = bytes.Replace(output, []byte(`# filter = [ "a|.*|" ]`), []byte(`filter = [ "a|^/mnt/.*|", "r|^/dev/loop.*|", "a|^/dev/.*|", "r|.*|" ]`), 1)
		} else {
			output = bytes.Replace(output, []byte(`# filter = [ "a|.*/|" ]`), []byte(`filter = [ "a|^/mnt/.*|", "r|.*|" ]`), 1)
			output = bytes.Replace(output, []byte(`# filter = [ "a|.*|" ]`), []byte(`filter = [ "a|^/mnt/.*|", "r|.*|" ]`), 1)
		}
	}

	if err = ioutil.WriteFile(lvmConfPath, output, 0600); err != nil {
		return errors.Wrapf(err, "failed to update lvm config file %q", lvmConfPath)
	}

	logger.Infof("Successfully updated lvm config file %q", lvmConfPath)
	return nil
}

func (a *OsdAgent) useRawMode(context *clusterd.Context, pvcBacked bool) (bool, error) {
	if pvcBacked {
		return a.clusterInfo.CephVersion.IsAtLeast(cephVolumeRawModeMinCephVersion), nil
	}

	var useRawMode bool
	// Can we safely use ceph-volume raw mode in the non-PVC case?
	// On non-PVC we see a race between systemd-udev and the osd process to acquire the lock on the device
	if a.clusterInfo.CephVersion.IsNautilus() && a.clusterInfo.CephVersion.IsAtLeast(cephFlockFixNautilusMinCephVersion) {
		logger.Debugf("will use raw mode since cluster version is at least %v", cephFlockFixNautilusMinCephVersion)
		useRawMode = true
	}

	if a.clusterInfo.CephVersion.IsOctopus() && a.clusterInfo.CephVersion.IsAtLeast(cephFlockFixOctopusMinCephVersion) {
		logger.Debugf("will use raw mode since cluster version is at least %v", cephFlockFixOctopusMinCephVersion)
		useRawMode = true
	}

	if a.clusterInfo.CephVersion.IsAtLeastPacific() {
		logger.Debug("will use raw mode since cluster version is at least pacific")
		useRawMode = true
	}

	// ceph-volume raw mode does not support encryption yet
	if a.storeConfig.EncryptedDevice {
		logger.Debug("won't use raw mode since encryption is enabled")
		useRawMode = false
	}

	// ceph-volume raw mode does not support more than one OSD per disk
	osdsPerDeviceCountString := sanitizeOSDsPerDevice(a.storeConfig.OSDsPerDevice)
	osdsPerDeviceCount, err := strconv.Atoi(osdsPerDeviceCountString)
	if err != nil {
		return false, errors.Wrapf(err, "failed to convert string %q to integer", osdsPerDeviceCountString)
	}
	if osdsPerDeviceCount > 1 {
		logger.Debugf("won't use raw mode since osd per device is %d", osdsPerDeviceCount)
		useRawMode = false
	}

	// ceph-volume raw mode mode does not support metadata device if not running on PVC because the user has specified a whole device
	if a.metadataDevice != "" {
		logger.Debugf("won't use raw mode since there is a metadata device %q", a.metadataDevice)
		useRawMode = false
	}

	return useRawMode, nil
}

func (a *OsdAgent) initializeDevices(context *clusterd.Context, devices *DeviceOsdMapping, allowRawMode bool) error {
	// it's a little strange to split this into parts, looping here and in the init functions, but
	// the LVM mode init requires the ability to loop over all the devices looking for metadata.
	rawDevices := &DeviceOsdMapping{
		Entries: map[string]*DeviceOsdIDEntry{},
	}
	lvmDevices := &DeviceOsdMapping{
		Entries: map[string]*DeviceOsdIDEntry{},
	}

	for name, device := range devices.Entries {
		// Even if we can use raw mode, do NOT use raw mode on disks. Ceph bluestore disks can
		// sometimes appear as though they have "phantom" Atari (AHDI) partitions created on them
		// when they don't in reality. This is due to a series of bugs in the Linux kernel when it
		// is built with Atari support enabled. This behavior does not appear for raw mode OSDs on
		// partitions, and we need the raw mode to create partition-based OSDs. We cannot merely
		// skip creating OSDs on "phantom" partitions due to a bug in `ceph-volume raw inventory`
		// which reports only the phantom partitions (and malformed OSD info) when they exist and
		// ignores the original (correct) OSDs created on the raw disk.
		// See: https://github.com/rook/rook/issues/7940
		if device.DeviceInfo.Type != sys.DiskType && allowRawMode {
			rawDevices.Entries[name] = device
			continue
		}
		lvmDevices.Entries[name] = device
	}

	err := a.initializeDevicesRawMode(context, rawDevices)
	if err != nil {
		return err
	}

	err = a.initializeDevicesLVMMode(context, lvmDevices)
	if err != nil {
		return err
	}

	return nil
}

func (a *OsdAgent) initializeDevicesRawMode(context *clusterd.Context, devices *DeviceOsdMapping) error {
	baseCommand := "stdbuf"
	cephVolumeMode := "raw"
	baseArgs := []string{"-oL", cephVolumeCmd, cephVolumeMode, "prepare", "--bluestore"}

	for name, device := range devices.Entries {
		deviceArg := path.Join("/dev", name)
		if device.Data == -1 {
			logger.Infof("configuring new raw device %q", deviceArg)

			immediateExecuteArgs := append(baseArgs, []string{
				"--data",
				deviceArg,
			}...)

			// assign the device class specific to the device
			immediateExecuteArgs = a.appendDeviceClassArg(device, immediateExecuteArgs)

			// execute ceph-volume with the device
			op, err := context.Executor.ExecuteCommandWithCombinedOutput(baseCommand, immediateExecuteArgs...)
			if err != nil {
				cvLogFilePath := path.Join(cephLogDir, "ceph-volume.log")

				// Print c-v log before exiting
				cvLog := readCVLogContent(cvLogFilePath)
				if cvLog != "" {
					logger.Errorf("%s", cvLog)
				}

				// Return failure
				return errors.Wrapf(err, "failed to run ceph-volume raw command. %s", op) // fail return here as validation provided by ceph-volume
			}
			logger.Infof("%v", op)
		} else {
			logger.Infof("skipping device %q with osd %d already configured", deviceArg, device.Data)
		}
	}

	return nil
}

func (a *OsdAgent) initializeDevicesLVMMode(context *clusterd.Context, devices *DeviceOsdMapping) error {
	storeFlag := "--bluestore"

	logPath := "/tmp/ceph-log"
	if err := os.MkdirAll(logPath, 0700); err != nil {
		return errors.Wrapf(err, "failed to create dir %q", logPath)
	}

	// Use stdbuf to capture the python output buffer such that we can write to the pod log as the logging happens
	// instead of using the default buffering that will log everything after ceph-volume exits
	baseCommand := "stdbuf"
	baseArgs := []string{"-oL", cephVolumeCmd, "--log-path", logPath, "lvm", "batch", "--prepare", storeFlag, "--yes"}
	if a.storeConfig.EncryptedDevice {
		baseArgs = append(baseArgs, encryptedFlag)
	}

	osdsPerDeviceCount := sanitizeOSDsPerDevice(a.storeConfig.OSDsPerDevice)
	batchArgs := baseArgs

	metadataDevices := make(map[string]map[string]string)
	for name, device := range devices.Entries {
		if device.Data == -1 {
			if device.Metadata != nil {
				logger.Infof("skipping metadata device %s config since it will be configured with a data device", name)
				continue
			}

			logger.Infof("configuring new LVM device %s", name)
			deviceArg := path.Join("/dev", name)
			// ceph-volume prefers to use /dev/mapper/<name> if the device has this kind of alias
			for _, devlink := range device.PersistentDevicePaths {
				if strings.HasPrefix(devlink, "/dev/mapper") {
					deviceArg = devlink
				}
			}

			deviceOSDCount := osdsPerDeviceCount
			if device.Config.OSDsPerDevice > 1 {
				deviceOSDCount = sanitizeOSDsPerDevice(device.Config.OSDsPerDevice)
			}

			if a.metadataDevice != "" || device.Config.MetadataDevice != "" {
				// When mixed hdd/ssd devices are given, ceph-volume configures db lv on the ssd.
				// the device will be configured as a batch at the end of the method
				md := a.metadataDevice
				if device.Config.MetadataDevice != "" {
					md = device.Config.MetadataDevice
				}
				logger.Infof("using %s as metadataDevice for device %s and let ceph-volume lvm batch decide how to create volumes", md, deviceArg)
				if _, ok := metadataDevices[md]; ok {
					// Fail when two devices using the same metadata device have different values for osdsPerDevice
					metadataDevices[md]["devices"] += " " + deviceArg
					if deviceOSDCount != metadataDevices[md]["osdsperdevice"] {
						return errors.Errorf("metadataDevice (%s) has more than 1 osdsPerDevice value set: %s != %s", md, deviceOSDCount, metadataDevices[md]["osdsperdevice"])
					}
				} else {
					metadataDevices[md] = make(map[string]string)
					metadataDevices[md]["osdsperdevice"] = deviceOSDCount
					if device.Config.DeviceClass != "" {
						metadataDevices[md]["deviceclass"] = device.Config.DeviceClass
					}
					metadataDevices[md]["devices"] = deviceArg
				}
				deviceDBSizeMB := getDatabaseSize(a.storeConfig.DatabaseSizeMB, device.Config.DatabaseSizeMB)
				if storeFlag == "--bluestore" && deviceDBSizeMB > 0 {
					if deviceDBSizeMB < cephVolumeMinDBSize {
						// ceph-volume will convert this value to ?G. It needs to be > 1G to invoke lvcreate.
						logger.Infof("skipping databaseSizeMB setting (%d). For it should be larger than %dMB.", deviceDBSizeMB, cephVolumeMinDBSize)
					} else {
						dbSizeString := strconv.FormatUint(display.MbTob(uint64(deviceDBSizeMB)), 10)
						if _, ok := metadataDevices[md]["databasesizemb"]; ok {
							if metadataDevices[md]["databasesizemb"] != dbSizeString {
								return errors.Errorf("metadataDevice (%s) has more than 1 databaseSizeMB value set: %s != %s", md, metadataDevices[md]["databasesizemb"], dbSizeString)
							}
						} else {
							metadataDevices[md]["databasesizemb"] = dbSizeString
						}
					}
				}
			} else {
				immediateExecuteArgs := append(baseArgs, []string{
					osdsPerDeviceFlag,
					deviceOSDCount,
					deviceArg,
				}...)

				// assign the device class specific to the device
				immediateExecuteArgs = a.appendDeviceClassArg(device, immediateExecuteArgs)

				// Reporting
				immediateReportArgs := append(immediateExecuteArgs, []string{
					"--report",
				}...)

				logger.Infof("Base command - %+v", baseCommand)
				logger.Infof("immediateExecuteArgs - %+v", immediateExecuteArgs)
				logger.Infof("immediateReportArgs - %+v", immediateReportArgs)
				if err := context.Executor.ExecuteCommand(baseCommand, immediateReportArgs...); err != nil {
					return errors.Wrap(err, "failed ceph-volume report") // fail return here as validation provided by ceph-volume
				}

				// execute ceph-volume immediately with the device-specific setting instead of batching up multiple devices together
				if err := context.Executor.ExecuteCommand(baseCommand, immediateExecuteArgs...); err != nil {
					cvLog := readCVLogContent("/tmp/ceph-log/ceph-volume.log")
					if cvLog != "" {
						logger.Errorf("%s", cvLog)
					}

					return errors.Wrap(err, "failed ceph-volume")
				}

			}
		} else {
			logger.Infof("skipping device %s with osd %d already configured", name, device.Data)
		}
	}

	for md, conf := range metadataDevices {

		mdArgs := batchArgs
		if _, ok := conf["osdsperdevice"]; ok {
			mdArgs = append(mdArgs, []string{
				osdsPerDeviceFlag,
				conf["osdsperdevice"],
			}...)
		}
		if _, ok := conf["deviceclass"]; ok {
			mdArgs = append(mdArgs, []string{
				crushDeviceClassFlag,
				conf["deviceclass"],
			}...)
		}
		if _, ok := conf["databasesizemb"]; ok {
			mdArgs = append(mdArgs, []string{
				databaseSizeFlag,
				conf["databasesizemb"],
			}...)
		}
		mdArgs = append(mdArgs, strings.Split(conf["devices"], " ")...)

		// Do not change device names if udev persistent names are passed
		mdPath := md
		if !strings.HasPrefix(mdPath, "/dev") {
			mdPath = path.Join("/dev", md)
		}

		mdArgs = append(mdArgs, []string{
			dbDeviceFlag,
			mdPath,
		}...)

		// Reporting
		reportArgs := append(mdArgs, []string{
			"--report",
		}...)

		if err := context.Executor.ExecuteCommand(baseCommand, reportArgs...); err != nil {
			return errors.Wrap(err, "failed ceph-volume report") // fail return here as validation provided by ceph-volume
		}

		reportArgs = append(reportArgs, []string{
			"--format",
			"json",
		}...)

		cvOut, err := context.Executor.ExecuteCommandWithOutput(baseCommand, reportArgs...)
		if err != nil {
			return errors.Wrapf(err, "failed ceph-volume json report: %s", cvOut) // fail return here as validation provided by ceph-volume
		}

		logger.Debugf("ceph-volume reports: %+v", cvOut)

		// ceph version v14.2.13 and v15.2.8 changed the changed output format of `lvm batch --prepare --report`
		// use previous logic if ceph version does not fall into this range
		if !isNewStyledLvmBatch(a.clusterInfo.CephVersion) {
			var cvReport cephVolReport
			if err = json.Unmarshal([]byte(cvOut), &cvReport); err != nil {
				return errors.Wrap(err, "failed to unmarshal ceph-volume report json")
			}

			if mdPath != cvReport.Vg.Devices {
				return errors.Errorf("ceph-volume did not use the expected metadataDevice [%s]", mdPath)
			}
		} else {
			var cvReports []cephVolReportV2
			if err = json.Unmarshal([]byte(cvOut), &cvReports); err != nil {
				return errors.Wrap(err, "failed to unmarshal ceph-volume report json")
			}

			if len(strings.Split(conf["devices"], " ")) != len(cvReports) {
				return errors.Errorf("failed to create enough required devices, required: %s, actual: %v", cvOut, cvReports)
			}

			for _, report := range cvReports {
				if report.BlockDB != mdPath && !strings.HasSuffix(mdPath, report.BlockDB) {
					return errors.Errorf("wrong db device for %s, required: %s, actual: %s", report.Data, mdPath, report.BlockDB)
				}
			}
		}

		// execute ceph-volume batching up multiple devices
		if err := context.Executor.ExecuteCommand(baseCommand, mdArgs...); err != nil {
			return errors.Wrap(err, "failed ceph-volume") // fail return here as validation provided by ceph-volume
		}
	}

	return nil
}

func (a *OsdAgent) appendDeviceClassArg(device *DeviceOsdIDEntry, args []string) []string {
	deviceClass := device.Config.DeviceClass
	if deviceClass == "" {
		// fall back to the device class for all devices on the node
		deviceClass = a.storeConfig.DeviceClass
	}
	if deviceClass != "" {
		args = append(args, []string{
			crushDeviceClassFlag,
			deviceClass,
		}...)
	}
	return args
}

func lvmPreReq(context *clusterd.Context, pvcBacked, lvBackedPV bool) error {
	// Check for the presence of LVM on the host when NOT running on PVC
	// since this scenario is still using LVM
	ne := NewNsenter(context, lvmCommandToCheck, []string{"--help"})
	err := ne.checkIfBinaryExistsOnHost()
	if err != nil {
		return errors.Wrapf(err, "binary %q does not exist on the host, make sure lvm2 package is installed", lvmCommandToCheck)
	}

	// Update LVM configuration file
	// Only do this after Ceph Nautilus 14.2.6 since it will use the ceph-volume raw mode by default and not LVM anymore
	if err := UpdateLVMConfig(context, pvcBacked, lvBackedPV); err != nil {
		return errors.Wrap(err, "failed to update lvm configuration file")
	}

	return nil
}

func getDatabaseSize(globalSize int, deviceSize int) int {
	if deviceSize > 0 {
		globalSize = deviceSize
	}
	return globalSize
}

func sanitizeOSDsPerDevice(count int) string {
	if count < 1 {
		count = 1
	}
	return strconv.Itoa(count)
}

// GetCephVolumeLVMOSDs list OSD prepared with lvm mode
func GetCephVolumeLVMOSDs(context *clusterd.Context, clusterInfo *client.ClusterInfo, cephfsid, lv string, skipLVRelease, lvBackedPV bool) ([]oposd.OSDInfo, error) {
	// lv can be a block device if raw mode is used
	cvMode := "lvm"

	var lvPath string
	args := []string{cvMode, "list", lv, "--format", "json"}
	result, err := callCephVolume(context, false, args...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve ceph-volume %s list results", cvMode)
	}

	var osds []oposd.OSDInfo
	var cephVolumeResult map[string][]osdInfo
	err = json.Unmarshal([]byte(result), &cephVolumeResult)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal ceph-volume %s list results. %s", cvMode, result)
	}

	for name, osdInfo := range cephVolumeResult {
		id, err := strconv.Atoi(name)
		if err != nil {
			logger.Errorf("bad osd returned from ceph-volume %q", name)
			continue
		}
		var osdFSID, osdDeviceClass string
		for _, osd := range osdInfo {
			if osd.Tags.ClusterFSID != cephfsid {
				logger.Infof("skipping osd%d: %q running on a different ceph cluster %q", id, osd.Tags.OSDFSID, osd.Tags.ClusterFSID)
				continue
			}
			osdFSID = osd.Tags.OSDFSID
			osdDeviceClass = osd.Tags.CrushDeviceClass

			// If no lv is specified let's take the one we discovered
			if lv == "" {
				lvPath = osd.Path
			}

		}

		if len(osdFSID) == 0 {
			logger.Infof("Skipping osd%d as no instances are running on ceph cluster %q", id, cephfsid)
			continue
		}
		logger.Infof("osdInfo has %d elements. %+v", len(osdInfo), osdInfo)

		// If lv was passed as an arg let's use it in osdInfo
		if lv != "" {
			lvPath = lv
		}

		osd := oposd.OSDInfo{
			ID:            id,
			Cluster:       "ceph",
			UUID:          osdFSID,
			BlockPath:     lvPath,
			SkipLVRelease: skipLVRelease,
			LVBackedPV:    lvBackedPV,
			CVMode:        cvMode,
			Store:         "bluestore",
			DeviceClass:   osdDeviceClass,
		}
		osds = append(osds, osd)
	}
	logger.Infof("%d ceph-volume lvm osd devices configured on this node", len(osds))

	return osds, nil
}

func readCVLogContent(cvLogFilePath string) string {
	// Open c-v log file
	cvLogFile, err := os.Open(filepath.Clean(cvLogFilePath))
	if err != nil {
		logger.Errorf("failed to open ceph-volume log file %q. %v", cvLogFilePath, err)
		return ""
	}
	// #nosec G307 Calling defer to close the file without checking the error return is not a risk for a simple file open and close
	defer cvLogFile.Close()

	// Read c-v log file
	b, err := ioutil.ReadAll(cvLogFile)
	if err != nil {
		logger.Errorf("failed to read ceph-volume log file %q. %v", cvLogFilePath, err)
		return ""
	}

	return string(b)
}

// GetCephVolumeRawOSDs list OSD prepared with raw mode
func GetCephVolumeRawOSDs(context *clusterd.Context, clusterInfo *client.ClusterInfo, cephfsid, block, metadataBlock, walBlock string, lvBackedPV, skipDeviceClass bool) ([]oposd.OSDInfo, error) {
	// lv can be a block device if raw mode is used
	cvMode := "raw"

	// Whether to fill the blockPath using the list result or the value that was passed in the function's call
	var setDevicePathFromList bool

	// blockPath represents the path of the OSD block
	// it can be the one passed from the function's call or discovered by the c-v list command
	var blockPath string

	args := []string{cvMode, "list", block, "--format", "json"}
	if block == "" {
		setDevicePathFromList = true
		args = []string{cvMode, "list", "--format", "json"}
	}

	result, err := callCephVolume(context, false, args...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve ceph-volume %s list results", cvMode)
	}

	var osds []oposd.OSDInfo
	var cephVolumeResult map[string]osdInfoBlock
	err = json.Unmarshal([]byte(result), &cephVolumeResult)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal ceph-volume %s list results", cvMode)
	}

	for _, osdInfo := range cephVolumeResult {
		var osdFSID string
		osdID := osdInfo.OsdID

		if osdInfo.CephFsid != cephfsid {
			message := fmt.Sprintf("osd.%d: %q belonging to a different ceph cluster %q", osdID, osdInfo.OsdUUID, osdInfo.CephFsid)
			// We must return an error since the caller only checks the length of osds
			if isOnPVC {
				return nil, errors.Errorf("%s", message)
			}
			logger.Infof("skipping %s", message)
			continue
		}
		osdFSID = osdInfo.OsdUUID

		if len(osdFSID) == 0 {
			message := fmt.Sprintf("no instance of osd.%d is running on ceph cluster %q (incomplete prepare? consider wiping the disks)", osdID, cephfsid)
			if isOnPVC {
				return nil, errors.Errorf("%s", message)
			}
			logger.Infof("skipping since %s", message)
			continue
		}

		// If no block is specified let's take the one we discovered
		if setDevicePathFromList {
			blockPath = osdInfo.Device
		} else {
			blockPath = block
		}

		osd := oposd.OSDInfo{
			ID:      osdID,
			Cluster: "ceph",
			UUID:    osdFSID,
			// let's not use osdInfo.Device, the device reported by bluestore tool since it might change during the next re-attach
			// during the prepare sequence, the device is attached, then detached then re-attached
			// During the last attach it could end up with a different /dev/ name
			// Thus in the activation sequence we might activate the wrong OSD and have OSDInfo messed up
			// Hence, let's use the PVC name instead which will always remain consistent
			BlockPath:     blockPath,
			MetadataPath:  metadataBlock,
			WalPath:       walBlock,
			SkipLVRelease: true,
			LVBackedPV:    lvBackedPV,
			CVMode:        cvMode,
			Store:         "bluestore",
		}

		if !skipDeviceClass {
			diskInfo, err := clusterd.PopulateDeviceInfo(blockPath, context.Executor)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to get device info for %q", blockPath)
			}
			osd.DeviceClass = sys.GetDiskDeviceClass(diskInfo)
			logger.Infof("setting device class %q for device %q", osd.DeviceClass, diskInfo.Name)
		}

		// If this is an encrypted OSD
		if os.Getenv(oposd.CephVolumeEncryptedKeyEnvVarName) != "" {
			// // Set subsystem and label for recovery and detection
			// We use /mnt/<pvc_name> since LUKS label/subsystem must be applied on the main block device, not the resulting encrypted dm
			mainBlock := fmt.Sprintf("/mnt/%s", os.Getenv(oposd.PVCNameEnvVarName))
			err = setLUKSLabelAndSubsystem(context, clusterInfo, mainBlock)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to set subsystem and label to encrypted device %q for osd %d", mainBlock, osdID)
			}

			// Close encrypted device
			err = closeEncryptedDevice(context, block)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to close encrypted device %q for osd %d", block, osdID)
			}

			// If there is a metadata block
			if metadataBlock != "" {
				// Close encrypted device
				err = closeEncryptedDevice(context, metadataBlock)
				if err != nil {
					return nil, errors.Wrapf(err, "failed to close encrypted db device %q for osd %d", metadataBlock, osdID)
				}
			}

			// If there is a wal block
			if walBlock != "" {
				// Close encrypted device
				err = closeEncryptedDevice(context, walBlock)
				if err != nil {
					return nil, errors.Wrapf(err, "failed to close encrypted wal device %q for osd %d", walBlock, osdID)
				}
			}
		}

		osds = append(osds, osd)
	}
	logger.Infof("%d ceph-volume raw osd devices configured on this node", len(osds))

	return osds, nil
}

func callCephVolume(context *clusterd.Context, requiresCombinedOutput bool, args ...string) (string, error) {
	// Use stdbuf to capture the python output buffer such that we can write to the pod log as the
	// logging happens instead of using the default buffering that will log everything after
	// ceph-volume exits
	baseCommand := "stdbuf"

	// Send the log to a temp location that isn't persisted to disk so that we can print out the
	// failure log later without also printing out past failures
	// TODO: does this mess up expectations from the ceph log collector daemon?
	logPath := "/tmp/ceph-log"
	if err := os.MkdirAll(logPath, 0700); err != nil {
		return "", errors.Wrapf(err, "failed to create dir %q", logPath)
	}
	baseArgs := []string{"-oL", cephVolumeCmd, "--log-path", logPath}

	// Do not use combined output for "list" calls, otherwise we will get stderr is the output and this will break the json unmarshall
	f := context.Executor.ExecuteCommandWithOutput
	if requiresCombinedOutput {
		// If the action is preparing we need the combined output
		f = context.Executor.ExecuteCommandWithCombinedOutput
	}
	co, err := f(baseCommand, append(baseArgs, args...)...)
	if err != nil {
		// Print c-v log before exiting with failure
		cvLog := readCVLogContent("/tmp/ceph-log/ceph-volume.log")
		logger.Errorf("%s", co)
		if cvLog != "" {
			logger.Errorf("%s", cvLog)
		}

		return "", errors.Wrapf(err, "failed ceph-volume call (see ceph-volume log above for more details)")
	}
	logger.Debugf("%v", co)

	return co, nil
}

func appendOSDInfo(currentOSDs, osdsToAppend []oposd.OSDInfo) []oposd.OSDInfo {
	for _, osdToAppend := range osdsToAppend {
		if !isInOSDInfoList(osdToAppend.UUID, currentOSDs) {
			currentOSDs = append(currentOSDs, osdToAppend)
		}
	}
	return currentOSDs
}

func isInOSDInfoList(uuid string, osds []oposd.OSDInfo) bool {
	for _, osd := range osds {
		if osd.UUID == uuid {
			return true
		}
	}

	return false
}
