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
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	oposd "github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
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

	blockDBFlag     = "--block.db"
	blockDBSizeFlag = "--block.db-size"
	dataFlag        = "--data"
)

// These are not constants because they are used by the tests
var (
	cephConfigDir = "/var/lib/ceph"
	cephLogDir    = "/var/log/ceph"
	lvmConfPath   = "/etc/lvm/lvm.conf"
	cvLogDir      = ""

	isEncrypted = os.Getenv(oposd.EncryptedDeviceEnvVarName) == "true"
	isOnPVC     = os.Getenv(oposd.PVCBackedOSDVarName) == "true"
)

type osdInfoBlock struct {
	CephFsid  string `json:"ceph_fsid"`
	Device    string `json:"device"`
	DeviceDb  string `json:"device_db"`
	DeviceWal string `json:"device_wal"`
	OsdID     int    `json:"osd_id"`
	OsdUUID   string `json:"osd_uuid"`
	Type      string `json:"type"`
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

type cephVolReportV2 struct {
	BlockDB      string `json:"block_db"`
	Encryption   string `json:"encryption"`
	Data         string `json:"data"`
	DatabaseSize string `json:"data_size"`
	BlockDbSize  string `json:"block_db_size"`
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
				return nil, errors.Wrap(err, "failed to get device already provisioned by ceph-volume lvm")
			}
			osds = append(osds, lvmOsds...)
			if len(osds) > 0 {
				// "ceph-volume raw list" lists the existing OSD even if it is configured with lvm mode, so escape here to avoid dupe.
				return osds, nil
			}

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
				return nil, errors.Wrap(err, "failed to get device already provisioned by ceph-volume raw")
			}
			osds = append(osds, rawOsds...)
		} else {
			// Non-PVC case
			// List existing OSD(s) configured with ceph-volume lvm mode
			lvmOsds, err = GetCephVolumeLVMOSDs(context, a.clusterInfo, a.clusterInfo.FSID, lvPath, false, false)
			if err != nil {
				return nil, errors.Wrap(err, "failed to get devices already provisioned by ceph-volume")
			}
			osds = append(osds, lvmOsds...)

			// List existing OSD(s) configured with ceph-volume raw mode
			rawOsds, err = GetCephVolumeRawOSDs(context, a.clusterInfo, a.clusterInfo.FSID, block, "", "", false, false)
			if err != nil {
				return nil, errors.Wrap(err, "failed to get device already provisioned by ceph-volume raw")
			}
			osds = appendOSDInfo(osds, rawOsds)
		}

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
		if block, metadataBlock, walBlock, err = a.initializeBlockPVC(context, devices, lvBackedPV); err != nil {
			return nil, errors.Wrap(err, "failed to initialize devices on PVC")
		}
	} else {
		err = a.initializeDevices(context, devices)
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
	storeFlag := a.storeConfig.GetStoreFlag()

	// Create a specific log directory so that each prepare command will have its own log
	// Only do this if nothing is present so that we don't override existing logs
	cvLogDir = path.Join(cephLogDir, a.nodeName)
	err := os.MkdirAll(cvLogDir, 0750)
	if err != nil {
		logger.Errorf("failed to create ceph-volume log directory %q, continue with default %q. %v", cvLogDir, cephLogDir, err)
		baseArgs = []string{"-oL", cephVolumeCmd, "raw", "prepare", storeFlag}
	} else {
		baseArgs = []string{"-oL", cephVolumeCmd, "--log-path", cvLogDir, "raw", "prepare", storeFlag}
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

			deviceArg := device.Config.Name

			immediateExecuteArgs := append(baseArgs, []string{
				"--data",
				deviceArg,
			}...)

			if a.replaceOSD != nil {
				replaceOSDID := a.GetReplaceOSDId(device.DeviceInfo.RealPath)
				if replaceOSDID != -1 {
					immediateExecuteArgs = append(immediateExecuteArgs, []string{
						"--osd-id",
						fmt.Sprintf("%d", replaceOSDID),
					}...)
				}
			}

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
			if !isEncrypted {
				blockPath = deviceArg
			} else {
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
			}
		} else {
			logger.Infof("skipping device %q with osd %d already configured", device.Config.Name, device.Data)
		}
	}

	return blockPath, metadataBlockPath, walBlockPath, nil
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

	input, err := os.ReadFile(lvmConfPath)
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
		// the content changed, so depending which version is installed one of the two replace will work
		if lvBackedPV {
			// ceph-volume calls lvs to locate given "vg/lv", so allow "/dev" here. However, ignore loopback devices
			output = bytes.Replace(output, []byte(`# filter = [ "a|.*/|" ]`), []byte(`filter = [ "a|^/mnt/.*|", "r|^/dev/loop.*|", "a|^/dev/.*|", "r|.*|" ]`), 1)
			output = bytes.Replace(output, []byte(`# filter = [ "a|.*|" ]`), []byte(`filter = [ "a|^/mnt/.*|", "r|^/dev/loop.*|", "a|^/dev/.*|", "r|.*|" ]`), 1)
		} else {
			output = bytes.Replace(output, []byte(`# filter = [ "a|.*/|" ]`), []byte(`filter = [ "a|^/mnt/.*|", "r|.*|" ]`), 1)
			output = bytes.Replace(output, []byte(`# filter = [ "a|.*|" ]`), []byte(`filter = [ "a|^/mnt/.*|", "r|.*|" ]`), 1)
		}
	}

	if err = os.WriteFile(lvmConfPath, output, 0600); err != nil {
		return errors.Wrapf(err, "failed to update lvm config file %q", lvmConfPath)
	}

	logger.Infof("Successfully updated lvm config file %q", lvmConfPath)
	return nil
}

func (a *OsdAgent) allowRawMode(context *clusterd.Context) (bool, error) {
	// by default assume raw mode
	allowRawMode := true

	// ceph-volume raw mode does not support encryption yet
	if a.storeConfig.EncryptedDevice {
		logger.Debug("won't use raw mode since encryption is enabled")
		allowRawMode = false
	}

	// ceph-volume raw mode does not support more than one OSD per disk
	osdsPerDeviceCountString := sanitizeOSDsPerDevice(a.storeConfig.OSDsPerDevice)
	osdsPerDeviceCount, err := strconv.Atoi(osdsPerDeviceCountString)
	if err != nil {
		return false, errors.Wrapf(err, "failed to convert string %q to integer", osdsPerDeviceCountString)
	}
	if osdsPerDeviceCount > 1 {
		logger.Debugf("won't use raw mode since osd per device is %d", osdsPerDeviceCount)
		allowRawMode = false
	}

	// ceph-volume raw mode does not support metadata device if not running on PVC because the user has specified a whole device
	if a.metadataDevice != "" {
		logger.Debugf("won't use raw mode since there is a metadata device %q", a.metadataDevice)
		allowRawMode = false
	}

	return allowRawMode, nil
}

// test if safe to use raw mode for a particular device
func isSafeToUseRawMode(device *DeviceOsdIDEntry) bool {
	// ceph-volume raw mode does not support more than one OSD per disk
	if device.Config.OSDsPerDevice > 1 {
		logger.Debugf("won't use raw mode for disk %q since osd per device is %d", device.Config.Name, device.Config.OSDsPerDevice)
		return false
	}

	// ceph-volume raw mode does not support metadata device if not running on PVC because the user has specified a whole device
	if device.Config.MetadataDevice != "" {
		logger.Debugf("won't use raw mode for disk %q since this disk has a metadata device", device.Config.Name)
		return false
	}

	return true
}

func lvmModeAllowed(device *DeviceOsdIDEntry, storeConfig *config.StoreConfig) bool {
	if device.DeviceInfo.Type == sys.LVMType {
		logger.Infof("skipping device %q for lvm mode since LVM logical volumes don't support `metadataDevice` or `osdsPerDevice` > 1", device.Config.Name)
		return false
	}
	if device.DeviceInfo.Type == sys.PartType && storeConfig.EncryptedDevice {
		logger.Infof("skipping partition %q for lvm mode since encryption is not supported on partitions with a `metadataDevice` or `osdsPerDevice > 1`", device.Config.Name)
		return false
	}

	return true
}

func (a *OsdAgent) initializeDevices(context *clusterd.Context, devices *DeviceOsdMapping) error {
	// Should we allow ceph-volume raw mode?
	allowRawMode, err := a.allowRawMode(context)
	if err != nil {
		return errors.Wrap(err, "failed to determine which ceph-volume mode to use")
	}

	// If not raw mode we must execute a few LVM prerequisites
	if !allowRawMode {
		err = lvmPreReq(context)
		if err != nil {
			return errors.Wrap(err, "failed to run lvm prerequisites")
		}
	}

	// it's a little strange to split this into parts, looping here and in the init functions, but
	// the LVM mode init requires the ability to loop over all the devices looking for metadata.
	rawDevices := &DeviceOsdMapping{
		Entries: map[string]*DeviceOsdIDEntry{},
	}
	lvmDevices := &DeviceOsdMapping{
		Entries: map[string]*DeviceOsdIDEntry{},
	}

	for name, device := range devices.Entries {
		// Even if we can use raw mode, do NOT use raw mode on disks in Ceph < 16.2.6. Ceph bluestore disks can
		// sometimes appear as though they have "phantom" Atari (AHDI) partitions created on them
		// when they don't in reality. This is due to a series of bugs in the Linux kernel when it
		// is built with Atari support enabled. This behavior does not appear for raw mode OSDs on
		// partitions, and we need the raw mode to create partition-based OSDs. We cannot merely
		// skip creating OSDs on "phantom" partitions due to a bug in `ceph-volume raw inventory`
		// which reports only the phantom partitions (and malformed OSD info) when they exist and
		// ignores the original (correct) OSDs created on the raw disk.
		// See: https://github.com/rook/rook/issues/7940
		if allowRawMode && isSafeToUseRawMode(device) {
			rawDevices.Entries[name] = device
			continue
		}
		if lvmModeAllowed(device, &a.storeConfig) {
			lvmDevices.Entries[name] = device
		}
	}

	err = a.initializeDevicesRawMode(context, rawDevices)
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
	storeFlag := a.storeConfig.GetStoreFlag()

	baseArgs := []string{"-oL", cephVolumeCmd, cephVolumeMode, "prepare", storeFlag}

	for name, device := range devices.Entries {
		deviceArg := path.Join("/dev", name)
		if device.Data == -1 {
			logger.Infof("configuring new raw device %q", deviceArg)

			immediateExecuteArgs := append(baseArgs, []string{
				"--data",
				deviceArg,
			}...)

			if a.replaceOSD != nil {
				restoreOSDID := a.GetReplaceOSDId(deviceArg)
				if restoreOSDID != -1 {
					immediateExecuteArgs = append(immediateExecuteArgs, []string{
						"--osd-id",
						fmt.Sprintf("%d", restoreOSDID),
					}...)
				}
			}

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
	storeFlag := a.storeConfig.GetStoreFlag()
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

				var metadataDevice *sys.LocalDisk
				for _, localDevice := range context.Devices {
					if localDevice.Name == md || filepath.Join("/dev", localDevice.Name) == md {
						logger.Infof("%s found in the desired metadata devices", md)
						metadataDevice = localDevice
						break
					}
					if strings.HasPrefix(md, "/dev/") && matchDevLinks(localDevice.DevLinks, md) {
						metadataDevice = localDevice
						break
					}
				}
				if metadataDevice == nil {
					return errors.Errorf("metadata device %s is not found", md)
				}
				// lvm device format is /dev/<vg>/<lv>
				if metadataDevice.Type != sys.LVMType {
					md = metadataDevice.Name
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
				if metadataDevice.Type == sys.PartType {
					if a.metadataDevice != "" && device.Config.MetadataDevice == "" {
						return errors.Errorf("Partition device %s can not be specified as metadataDevice in the global OSD configuration or in the node level OSD configuration", md)
					}
					metadataDevices[md]["part"] = "true" // ceph-volume lvm batch only supports disk and lvm
				}
				deviceDBSizeMB := getDatabaseSize(a.storeConfig.DatabaseSizeMB, device.Config.DatabaseSizeMB)
				if a.storeConfig.IsValidStoreType() && deviceDBSizeMB > 0 {
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

		// Do not change device names if udev persistent names are passed
		mdPath := md
		if !strings.HasPrefix(mdPath, "/dev") {
			mdPath = path.Join("/dev", md)
		}

		var hasPart bool
		mdArgs := batchArgs
		osdsPerDevice := 1
		if part, ok := conf["part"]; ok && part == "true" {
			hasPart = true
		}
		if hasPart {
			// ceph-volume lvm prepare --data {vg/lv} --block.wal {partition} --block.db {/path/to/device}
			baseArgs := []string{"-oL", cephVolumeCmd, "--log-path", logPath, "lvm", "prepare", storeFlag}
			if a.storeConfig.EncryptedDevice {
				baseArgs = append(baseArgs, encryptedFlag)
			}
			mdArgs = baseArgs
			devices := strings.Split(conf["devices"], " ")
			if len(devices) > 1 {
				logger.Warningf("partition metadataDevice %s can only be used by one data device", md)
			}
			if _, ok := conf["osdsperdevice"]; ok {
				logger.Warningf("`ceph-volume osd prepare` doesn't support multiple OSDs per device")
			}
			mdArgs = append(mdArgs, []string{
				dataFlag,
				devices[0],
				blockDBFlag,
				mdPath,
			}...)
			if _, ok := conf["databasesizemb"]; ok {
				mdArgs = append(mdArgs, []string{
					blockDBSizeFlag,
					conf["databasesizemb"],
				}...)
			}
		} else {
			if _, ok := conf["osdsperdevice"]; ok {
				mdArgs = append(mdArgs, []string{
					osdsPerDeviceFlag,
					conf["osdsperdevice"],
				}...)
				v, _ := strconv.Atoi(conf["osdsperdevice"])
				if v > 1 {
					osdsPerDevice = v
				}
			}
			if _, ok := conf["databasesizemb"]; ok {
				mdArgs = append(mdArgs, []string{
					databaseSizeFlag,
					conf["databasesizemb"],
				}...)
			}
			mdArgs = append(mdArgs, strings.Split(conf["devices"], " ")...)
			mdArgs = append(mdArgs, []string{
				dbDeviceFlag,
				mdPath,
			}...)
		}

		if _, ok := conf["deviceclass"]; ok {
			mdArgs = append(mdArgs, []string{
				crushDeviceClassFlag,
				conf["deviceclass"],
			}...)
		}

		if !hasPart {
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

			var cvReports []cephVolReportV2
			if err = json.Unmarshal([]byte(cvOut), &cvReports); err != nil {
				return errors.Wrap(err, "failed to unmarshal ceph-volume report json")
			}

			if len(strings.Split(conf["devices"], " "))*osdsPerDevice != len(cvReports) {
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

func lvmPreReq(context *clusterd.Context) error {
	// Check for the presence of LVM on the host when NOT running on PVC
	// since this scenario is still using LVM
	ne := NewNsenter(context, lvmCommandToCheck, []string{"--help"})
	err := ne.checkIfBinaryExistsOnHost()
	if err != nil {
		return errors.Wrapf(err, "binary %q does not exist on the host, make sure lvm2 package is installed", lvmCommandToCheck)
	}

	// Update LVM configuration file
	if err := UpdateLVMConfig(context, false, false); err != nil {
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
	result, err := callCephVolume(context, args...)
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

		// TODO: Don't read osd store type from env variable
		osdStore := os.Getenv(oposd.OSDStoreTypeVarName)
		if osdStore == "" {
			osdStore = string(cephv1.StoreTypeBlueStore)
		}

		osd := oposd.OSDInfo{
			ID:            id,
			Cluster:       "ceph",
			UUID:          osdFSID,
			BlockPath:     lvPath,
			SkipLVRelease: skipLVRelease,
			LVBackedPV:    lvBackedPV,
			CVMode:        cvMode,
			Store:         osdStore,
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
	//nolint:gosec // Calling defer to close the file without checking the error return is not a risk for a simple file open and close
	defer cvLogFile.Close()

	// Read c-v log file
	b, err := io.ReadAll(cvLogFile)
	if err != nil {
		logger.Errorf("failed to read ceph-volume log file %q. %v", cvLogFilePath, err)
		return ""
	}

	return string(b)
}

// GetCephVolumeRawOSDs list OSD prepared with raw mode.
// Sometimes this function called against a device, sometimes it's not. For instance, in the cleanup
// scenario, we don't pass any block because we are looking for all the OSDs present on the machine.
// On the other hand, the PVC scenario always uses the PVC block as a block to check whether the
// disk is an OSD or not.
// The same goes for "metadataBlock" and "walBlock" they are only used in the prepare job.
func GetCephVolumeRawOSDs(context *clusterd.Context, clusterInfo *client.ClusterInfo, cephfsid, block, metadataBlock, walBlock string, lvBackedPV, skipDeviceClass bool) ([]oposd.OSDInfo, error) {
	// lv can be a block device if raw mode is used
	cvMode := "raw"

	// Whether to fill the blockPath using the list result or the value that was passed in the function's call
	var setDevicePathFromList bool

	// blockPath represents the path of the OSD block
	// it can be the one passed from the function's call or discovered by the c-v list command
	var blockPath string
	var blockMetadataPath string
	var blockWalPath string

	// If block is passed, check if it's an encrypted device, this is needed to get the correct
	// device path and populate the OSDInfo for that OSD
	// When the device is passed, this means we entered the case where no devices were found
	// available, this indicates OSD have been prepared already.
	// However, there is a scenario where we run the prepare job again and this is when the OSD
	// deployment is removed. The operator will reconcile upon deletion of the OSD deployment thus
	// re-running the prepare job to re-hydrate the OSDInfo.
	//
	// isCephEncryptedBlock() returns false if the disk is not a LUKS device with:
	// Device /dev/sdc is not a valid LUKS device.
	if block != "" {
		if isCephEncryptedBlock(context, cephfsid, block) {
			childDevice, err := sys.ListDevicesChild(context.Executor, block)
			if err != nil {
				return nil, err
			}

			var encryptedBlock string
			// Find the encrypted block as part of the output
			// Most of the time we get 2 devices, the parent and the child but we don't want to guess
			// which one is the child by looking at the index, instead the iterate over the list
			// Our encrypted device **always** have "-dmcrypt" in the name.
			for _, device := range childDevice {
				if strings.Contains(device, "-dmcrypt") {
					encryptedBlock = device
					break
				}
			}
			if encryptedBlock == "" {
				// The encrypted block is not opened.
				// The encrypted device is closed in some cases when
				// the OSD deployment has been removed manually accompanied
				// by any of following cases:
				// - node reboot
				// - csi managed PVC being unmounted etc
				// Let's re-open the block to re-hydrate the OSDInfo.
				logger.Debugf("encrypted block device %q is not open, opening it now", block)
				passphrase := os.Getenv(oposd.CephVolumeEncryptedKeyEnvVarName)
				if passphrase == "" {
					return nil, errors.Errorf("encryption passphrase is empty in env var %q", oposd.CephVolumeEncryptedKeyEnvVarName)
				}
				pvcName := os.Getenv(oposd.PVCNameEnvVarName)
				if pvcName == "" {
					return nil, errors.Errorf("pvc name is empty in env var %q", oposd.PVCNameEnvVarName)
				}

				target := oposd.EncryptionDMName(pvcName, oposd.DmcryptBlockType)
				// remove stale dm device left by previous OSD.
				err = removeEncryptedDevice(context, target)
				if err != nil {
					logger.Warningf("failed to remove stale dm device %q: %q", target, err)
				}
				err = openEncryptedDevice(context, block, target, passphrase)
				if err != nil {
					return nil, errors.Wrapf(err, "failed to open encrypted block device %q on %q", block, target)
				}

				// ceph-volume prefers to use /dev/mapper/<name>
				encryptedBlock = oposd.EncryptionDMPath(pvcName, oposd.DmcryptBlockType)
			}
			// If we have one child device, it should be the encrypted block but still verifying it
			isDeviceEncrypted, err := sys.IsDeviceEncrypted(context.Executor, encryptedBlock)
			if err != nil {
				return nil, err
			}

			// All good, now we set the right disk for ceph-volume to list against
			// The ceph-volume will look like:
			// [root@bde85e6b23ec /]# ceph-volume raw list /dev/mapper/ocs-deviceset-thin-1-data-0hmfgp-block-dmcrypt
			// {
			//     "4": {
			//         "ceph_fsid": "fea59c09-2d35-4096-bc46-edb0fd39ab86",
			//         "device": "/dev/mapper/ocs-deviceset-thin-1-data-0hmfgp-block-dmcrypt",
			//         "osd_id": 4,
			//         "osd_uuid": "fcff2062-e653-4d42-84e5-e8de639bed4b",
			//         "type": "bluestore"
			//     }
			// }
			if isDeviceEncrypted {
				block = encryptedBlock
			}
		}
	}

	args := []string{cvMode, "list", block, "--format", "json"}
	if block == "" {
		setDevicePathFromList = true
		args = []string{cvMode, "list", "--format", "json"}
	}

	result, err := callCephVolume(context, args...)
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
			blockMetadataPath = osdInfo.DeviceDb
			blockWalPath = osdInfo.DeviceWal
		} else {
			blockPath = block
			blockMetadataPath = metadataBlock
			blockWalPath = walBlock
		}

		osdStore := osdInfo.Type

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
			MetadataPath:  blockMetadataPath,
			WalPath:       blockWalPath,
			SkipLVRelease: true,
			LVBackedPV:    lvBackedPV,
			CVMode:        cvMode,
			Store:         osdStore,
			Encrypted:     strings.Contains(blockPath, "-dmcrypt"),
		}

		if !skipDeviceClass {
			diskInfo, err := clusterd.PopulateDeviceInfo(blockPath, context.Executor)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to get device info for %q", blockPath)
			}
			deviceType := sys.GetDiskDeviceType(diskInfo)
			osd.DeviceType = deviceType
			logger.Infof("setting device type %q for device %q", osd.DeviceType, diskInfo.Name)

			crushDeviceClass := sys.GetDiskDeviceClass(oposd.CrushDeviceClassVarName, deviceType)
			osd.DeviceClass = crushDeviceClass
			logger.Infof("setting device class %q for device %q", osd.DeviceClass, diskInfo.Name)
		}

		// If this is an encrypted OSD
		// Always rely on the env variable and NOT what we find as a block name, this function is
		// called by:
		//   * the prepare job, which pass the env variable for encryption or not
		//   * the cleanup job which lists **all** devices and cleans them up
		// They do different things, in the case of the prepare job we want to close the encrypted
		// device because the device is going to be detached from the pod and re-attached to the OSD
		// pod
		// For the cleanup pod we don't want to close the encrypted block since it will sanitize it
		// first and then close it
		if osd.Encrypted && os.Getenv(oposd.CephVolumeEncryptedKeyEnvVarName) != "" {
			// If label and subsystem are not set on the encrypted block let's set it
			// They will be set if the OSD deployment has been removed manually and the prepare job
			// runs again.
			// We use /mnt/<pvc_name> since LUKS label/subsystem must be applied on the main block device, not the resulting encrypted dm
			mainBlock := fmt.Sprintf("/mnt/%s", os.Getenv(oposd.PVCNameEnvVarName))
			if !isCephEncryptedBlock(context, cephfsid, mainBlock) {
				// Set subsystem and label for recovery and detection
				err = setLUKSLabelAndSubsystem(context, clusterInfo, mainBlock)
				if err != nil {
					return nil, errors.Wrapf(err, "failed to set subsystem and label to encrypted device %q for osd %d", mainBlock, osdID)
				}
			}

			// Close encrypted device
			if block != "" {
				err = CloseEncryptedDevice(context, block)
				if err != nil {
					return nil, errors.Wrapf(err, "failed to close encrypted device %q for osd %d", block, osdID)
				}
			}

			// If there is a metadata block
			if metadataBlock != "" {
				// Close encrypted device
				err = CloseEncryptedDevice(context, metadataBlock)
				if err != nil {
					return nil, errors.Wrapf(err, "failed to close encrypted db device %q for osd %d", metadataBlock, osdID)
				}
			}

			// If there is a wal block
			if walBlock != "" {
				// Close encrypted device
				err = CloseEncryptedDevice(context, walBlock)
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

func callCephVolume(context *clusterd.Context, args ...string) (string, error) {
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
