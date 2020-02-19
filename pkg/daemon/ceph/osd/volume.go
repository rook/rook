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
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
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
	// "data" or "journal" for filestore and "block" for bluestore
	Type string `json:"type"`
}

type osdTags struct {
	OSDFSID     string `json:"ceph.osd_fsid"`
	Encrypted   string `json:"ceph.encrypted"`
	ClusterFSID string `json:"ceph.cluster_fsid"`
}

type cephVolReport struct {
	Changed bool      `json:"changed"`
	Vg      cephVolVg `json:"vg"`
}

type cephVolVg struct {
	Devices string `json:"devices"`
}

func (a *OsdAgent) configureCVDevices(context *clusterd.Context, devices *DeviceOsdMapping) ([]oposd.OSDInfo, error) {
	var osds []oposd.OSDInfo
	var lvmOsds []oposd.OSDInfo
	var rawOsds []oposd.OSDInfo
	var lvBackedPV bool
	var block, lvPath, metadataBlock string
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

			// List THE existing OSD configured with ceph-volume lvm mode
			lvmOsds, err = getCephVolumeLVMOSDs(context, a.cluster.Name, a.cluster.FSID, lvPath, skipLVRelease, lvBackedPV)
			if err != nil {
				logger.Infof("failed to get device already provisioned by ceph-volume lvm. %v", err)
			}
			osds = append(osds, lvmOsds...)

			// List THE existing OSD configured with ceph-volume raw mode
			if a.cluster.CephVersion.IsAtLeast(cephVolumeRawModeMinCephVersion) && !lvBackedPV {
				// For block mode
				block = fmt.Sprintf("/mnt/%s", a.nodeName)

				// This is hard to determine a potential metadata device here
				// Also, I don't think (leseb) this code we have run in this condition
				// I tried several things:
				//    * evict a node, osd moves, the prepare job was never relaunched ever because we check for the osd deployment and skip the prepare
				//    * restarted the operator, again the prepare job was not re-run
				//
				// I'm leaving this code with an empty metadata device for now
				metadataBlock = ""

				rawOsds, err = getCephVolumeRawOSDs(context, a.cluster.Name, a.cluster.FSID, block, metadataBlock, lvBackedPV)
				if err != nil {
					logger.Infof("failed to get device already provisioned by ceph-volume raw. %v", err)
				}
				osds = append(osds, rawOsds...)
			}

			return osds, nil
		}

		// List existing OSD(s) configured with ceph-volume lvm mode
		lvmOsds, err = getCephVolumeLVMOSDs(context, a.cluster.Name, a.cluster.FSID, lvPath, false, lvBackedPV)
		if err != nil {
			logger.Infof("failed to get devices already provisioned by ceph-volume. %v", err)
		}
		osds = append(osds, lvmOsds...)

		return osds, nil
	}

	// Create OSD bootstrap keyring
	err = createOSDBootstrapKeyring(context, a.cluster.Name, cephConfigDir)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to generate osd keyring")
	}

	// Check if the PVC is an LVM block device (certain StorageClass do this)
	if a.pvcBacked {
		for _, device := range devices.Entries {
			dev := device.Config.Name
			// When not using PV backend an LV
			// Otherwise lsblk will fail since the block will be '/dev/mnt/set1-0-data-l6p5q'
			// And thus won't be a block device
			//
			// The same goes the metadata block device which is stored in /srv
			if !strings.HasPrefix(dev, "/mnt") && !strings.HasPrefix(dev, "/srv") {
				dev = path.Join("/dev", dev)
			}
			lvBackedPV, err = sys.IsLV(dev, context.Executor)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to check device type")
			}
			break
		}
	}

	// Update LVM configuration file
	// Only do this after Ceph Nautilus 14.2.6 since it will use the ceph-volume raw mode by default and not LVM anymore
	//
	// Or keep doing this if the PV is backend by an LV already
	if !a.cluster.CephVersion.IsAtLeast(cephVolumeRawModeMinCephVersion) || lvBackedPV {
		if err := updateLVMConfig(context, a.pvcBacked, lvBackedPV); err != nil {
			return nil, errors.Wrap(err, "failed to update lvm configuration file")
		}
	}

	// If running on OSD on PVC
	if a.pvcBacked {
		if block, metadataBlock, err = a.initializeBlockPVC(context, devices, lvBackedPV); err != nil {
			return nil, errors.Wrapf(err, "failed to initialize devices")
		}
	} else {
		if err = a.initializeDevices(context, devices); err != nil {
			return nil, errors.Wrapf(err, "failed to initialize devices")
		}
	}

	// List OSD configured with ceph-volume lvm mode
	lvmOsds, err = getCephVolumeLVMOSDs(context, a.cluster.Name, a.cluster.FSID, block, false, lvBackedPV)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get devices already provisioned by ceph-volume lvm")
	}
	osds = append(osds, lvmOsds...)

	// List THE configured OSD with ceph-volume raw mode
	if a.cluster.CephVersion.IsAtLeast(cephVolumeRawModeMinCephVersion) && !lvBackedPV {
		block = fmt.Sprintf("/mnt/%s", a.nodeName)
		rawOsds, err = getCephVolumeRawOSDs(context, a.cluster.Name, a.cluster.FSID, block, metadataBlock, lvBackedPV)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get devices already provisioned by ceph-volume raw")
		}
		osds = append(osds, rawOsds...)
	}

	return osds, err
}

func (a *OsdAgent) initializeBlockPVC(context *clusterd.Context, devices *DeviceOsdMapping, lvBackedPV bool) (string, string, error) {

	// we need to return the block if raw mode is used and the lv if lvm mode
	baseCommand := "stdbuf"
	var baseArgs []string

	cephVolumeMode := "lvm"
	if a.cluster.CephVersion.IsAtLeast(cephVolumeRawModeMinCephVersion) && !lvBackedPV {
		cephVolumeMode = "raw"
	}

	// Create a specific log directory so that each prepare command will have its own log
	// Only do this if nothing is present so that we don't override existing logs
	cvLogDir = path.Join(cephLogDir, a.nodeName)
	err := os.MkdirAll(cvLogDir, 0755)
	if err != nil {
		logger.Errorf("failed to create ceph-volume log directory %q, continue with default %q. %v", cvLogDir, cephLogDir, err)
		baseArgs = []string{"-oL", cephVolumeCmd, cephVolumeMode, "prepare", "--bluestore"}
	} else {
		// Always force Bluestore!
		baseArgs = []string{"-oL", cephVolumeCmd, "--log-path", cvLogDir, cephVolumeMode, "prepare", "--bluestore"}
	}

	var metadataArg []string
	var metadataDev bool
	var blockPath, metadataBlockPath string

	// Problem: map is an unordered collection
	// therefore the iteration order of a map is not guaranteed to be the same every time you iterate over it.
	// So we could first get the metadata device and then the main block in a scenario where a metadata PVC is present
	for name, device := range devices.Entries {
		// If this is the metadata device there is nothing to do
		// it'll be used in one of the iterations
		if name == "metadata" {
			logger.Debugf("device %q is a metadata device, skipping this iteration it will be used in the next one", device.Config.Name)
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

			crushDeviceClass := os.Getenv(oposd.CrushDeviceClassVarName)
			if crushDeviceClass != "" {
				metadataArg = append(metadataArg, []string{crushDeviceClassFlag, crushDeviceClass}...)
			}
			metadataBlockPath = devices.Entries["metadata"].Config.Name
		}

		if device.Data == -1 {
			logger.Infof("configuring new device %q", device.Config.Name)
			var err error
			var deviceArg string
			if lvBackedPV {
				// pass 'vg/lv' to ceph-volume
				deviceArg, err = getLVNameFromDevicePath(context, device.Config.Name)
				if err != nil {
					return "", "", errors.Wrapf(err, "failed to get lv name from device path %q", device.Config.Name)
				}
			} else {
				deviceArg = device.Config.Name
			}

			immediateExecuteArgs := append(baseArgs, []string{
				"--data",
				deviceArg,
			}...)

			// Add the cli argument for the metadata device
			if metadataDev {
				immediateExecuteArgs = append(immediateExecuteArgs, metadataArg...)
			}

			// execute ceph-volume with the device
			op, err := context.Executor.ExecuteCommandWithCombinedOutput(false, "", baseCommand, immediateExecuteArgs...)
			if err != nil {
				cvLogFilePath := path.Join(cvLogDir, "ceph-volume.log")

				// Print c-v log before exiting
				cvLog := readCVLogContent(cvLogFilePath)
				if cvLog != "" {
					logger.Errorf("%s", cvLog)
				}

				// Return failure
				return "", "", errors.Wrapf(err, "failed ceph-volume") // fail return here as validation provided by ceph-volume
			}
			logger.Infof("%v", op)
			// if raw mode is used or PV on LV, let's return the path of the device
			if lvBackedPV || cephVolumeMode == "raw" {
				blockPath = deviceArg
			} else {
				blockPath = getLVPath(op)
				if blockPath == "" {
					return "", "", errors.New("failed to get lv path from ceph-volume lvm prepare output")
				}
			}
		} else {
			logger.Infof("skipping device %q with osd %d already configured", device.Config.Name, device.Data)
		}
	}

	return blockPath, metadataBlockPath, nil
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

func getLVNameFromDevicePath(context *clusterd.Context, devicePath string) (string, error) {
	devInfo, err := context.Executor.ExecuteCommandWithOutput(true, "",
		"dmsetup", "info", "-c", "--noheadings", "-o", "name", devicePath)
	if err != nil {
		return "", errors.Wrapf(err, "failed dmsetup info. output: %q", devInfo)
	}
	out, err := context.Executor.ExecuteCommandWithOutput(true, "", "dmsetup", "splitname", devInfo, "--noheadings")
	if err != nil {
		return "", errors.Wrapf(err, "failed dmsetup splitname %q", devInfo)
	}
	split := strings.Split(out, ":")
	if len(split) < 2 {
		return "", errors.Wrapf(err, "dmsetup splitname returned unexpected result for %q. output: %q", devInfo, out)
	}
	return fmt.Sprintf("%s/%s", split[0], split[1]), nil
}

func updateLVMConfig(context *clusterd.Context, onPVC, lvBackedPV bool) error {

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

	if err = ioutil.WriteFile(lvmConfPath, output, 0644); err != nil {
		return errors.Wrapf(err, "failed to update lvm config file %q", lvmConfPath)
	}

	logger.Infof("Successfully updated lvm config file %q", lvmConfPath)
	return nil
}

func (a *OsdAgent) initializeDevices(context *clusterd.Context, devices *DeviceOsdMapping) error {
	storeFlag := "--bluestore"

	// Use stdbuf to capture the python output buffer such that we can write to the pod log as the logging happens
	// instead of using the default buffering that will log everything after ceph-volume exits
	baseCommand := "stdbuf"
	baseArgs := []string{"-oL", cephVolumeCmd, "lvm", "batch", "--prepare", storeFlag, "--yes"}
	if a.storeConfig.EncryptedDevice {
		baseArgs = append(baseArgs, encryptedFlag)
	}

	osdsPerDeviceCount := sanitizeOSDsPerDevice(a.storeConfig.OSDsPerDevice)
	batchArgs := baseArgs

	metadataDevices := make(map[string]map[string]string)
	for name, device := range devices.Entries {
		if device.Data == -1 {
			logger.Infof("configuring new device %s", name)
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

				if device.Config.DeviceClass != "" {
					immediateExecuteArgs = append(immediateExecuteArgs, []string{
						crushDeviceClassFlag,
						device.Config.DeviceClass,
					}...)
				}

				// Reporting
				immediateReportArgs := append(immediateExecuteArgs, []string{
					"--report",
				}...)

				logger.Infof("Base command - %+v", baseCommand)
				logger.Infof("immediateReportArgs - %+v", baseCommand)
				logger.Infof("immediateExecuteArgs - %+v", immediateExecuteArgs)
				if err := context.Executor.ExecuteCommand(false, "", baseCommand, immediateReportArgs...); err != nil {
					return errors.Wrapf(err, "failed ceph-volume report") // fail return here as validation provided by ceph-volume
				}

				// execute ceph-volume immediately with the device-specific setting instead of batching up multiple devices together
				if err := context.Executor.ExecuteCommand(false, "", baseCommand, immediateExecuteArgs...); err != nil {
					return errors.Wrapf(err, "failed ceph-volume")
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

		mdArgs = append(mdArgs, []string{
			dbDeviceFlag,
			path.Join("/dev", md),
		}...)

		// Reporting
		reportArgs := append(mdArgs, []string{
			"--report",
		}...)

		if err := context.Executor.ExecuteCommand(false, "", baseCommand, reportArgs...); err != nil {
			return errors.Wrapf(err, "failed ceph-volume report") // fail return here as validation provided by ceph-volume
		}

		reportArgs = append(reportArgs, []string{
			"--format",
			"json",
		}...)

		cvOut, err := context.Executor.ExecuteCommandWithCombinedOutput(false, "", baseCommand, reportArgs...)
		if err != nil {
			return errors.Wrapf(err, "failed ceph-volume json report: %s", cvOut) // fail return here as validation provided by ceph-volume
		}

		logger.Debugf("ceph-volume report: %+v", cvOut)

		var cvReport cephVolReport
		if err = json.Unmarshal([]byte(cvOut), &cvReport); err != nil {
			return errors.Wrapf(err, "failed to unmarshal ceph-volume report json")
		}

		if path.Join("/dev", md) != cvReport.Vg.Devices {
			return errors.Errorf("ceph-volume did not use the expected metadataDevice [%s]", md)
		}

		// execute ceph-volume batching up multiple devices
		if err := context.Executor.ExecuteCommand(false, "", baseCommand, mdArgs...); err != nil {
			return errors.Wrapf(err, "failed ceph-volume") // fail return here as validation provided by ceph-volume
		}
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

func getCephVolumeLVMOSDs(context *clusterd.Context, clusterName string, cephfsid, lv string, skipLVRelease, lvBackedPV bool) ([]oposd.OSDInfo, error) {
	// lv can be a block device if raw mode is used
	cvMode := "lvm"

	result, err := context.Executor.ExecuteCommandWithOutput(false, "", cephVolumeCmd, cvMode, "list", lv, "--format", "json")
	if err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve ceph-volume %s list results", cvMode)
	}
	logger.Debugf("%v", result)

	var osds []oposd.OSDInfo
	var cephVolumeResult map[string][]osdInfo
	err = json.Unmarshal([]byte(result), &cephVolumeResult)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal ceph-volume %s list results", cvMode)
	}

	for name, osdInfo := range cephVolumeResult {
		id, err := strconv.Atoi(name)
		if err != nil {
			logger.Errorf("bad osd returned from ceph-volume %q", name)
			continue
		}
		var osdFSID string
		store := "bluestore"
		for _, osd := range osdInfo {
			if osd.Tags.ClusterFSID != cephfsid {
				logger.Infof("skipping osd%d: %q running on a different ceph cluster %q", id, osd.Tags.OSDFSID, osd.Tags.ClusterFSID)
				continue
			}
			osdFSID = osd.Tags.OSDFSID
			if osd.Type == "journal" {
				store = "filestore"
			}
		}
		if len(osdFSID) == 0 {
			logger.Infof("Skipping osd%d as no instances are running on ceph cluster %q", id, cephfsid)
			continue
		}
		logger.Infof("osdInfo has %d elements. %+v", len(osdInfo), osdInfo)

		osd := oposd.OSDInfo{
			ID:            id,
			Cluster:       "ceph",
			UUID:          osdFSID,
			BlockPath:     lv,
			SkipLVRelease: skipLVRelease,
			LVBackedPV:    lvBackedPV,
			CVMode:        cvMode,
			Store:         store,
		}
		osds = append(osds, osd)
	}
	logger.Infof("%d ceph-volume lvm osd devices configured on this node", len(osds))

	return osds, nil
}

func readCVLogContent(cvLogFilePath string) string {
	// Open c-v log file
	cvLogFile, err := os.Open(cvLogFilePath)
	if err != nil {
		logger.Errorf("failed to open ceph-volume log file %q. %v", cvLogFilePath, err)
		return ""
	}
	defer cvLogFile.Close()

	// Read c-v log file
	b, err := ioutil.ReadAll(cvLogFile)
	if err != nil {
		logger.Errorf("failed to read ceph-volume log file %q. %v", cvLogFilePath, err)
		return ""
	}

	return string(b)
}

func getCephVolumeRawOSDs(context *clusterd.Context, clusterName string, cephfsid, block, metadataBlock string, lvBackedPV bool) ([]oposd.OSDInfo, error) {
	// lv can be a block device if raw mode is used
	cvMode := "raw"

	result, err := context.Executor.ExecuteCommandWithOutput(false, "", cephVolumeCmd, cvMode, "list", block, "--format", "json")
	if err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve ceph-volume %s list results", cvMode)
	}
	logger.Debugf("%v", result)

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
			logger.Infof("skipping osd.%d: %s running on a different ceph cluster %q", osdID, osdInfo.OsdUUID, osdInfo.CephFsid)
			continue
		}
		osdFSID = osdInfo.OsdUUID

		if len(osdFSID) == 0 {
			logger.Infof("Skipping osd.%d as no instances are running on ceph cluster %q", osdID, cephfsid)
			continue
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
			BlockPath:     block,
			MetadataPath:  metadataBlock,
			SkipLVRelease: true,
			LVBackedPV:    lvBackedPV,
			CVMode:        cvMode,
			Store:         "bluestore",
		}
		osds = append(osds, osd)

	}
	logger.Infof("%d ceph-volume raw osd devices configured on this node", len(osds))

	return osds, nil
}
