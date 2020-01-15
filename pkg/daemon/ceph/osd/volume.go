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
	"syscall"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	oposd "github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/util/display"
	"github.com/rook/rook/pkg/util/exec"
	"github.com/rook/rook/pkg/util/sys"
)

// These are not constants because they are used by the tests
var (
	cephConfigDir = "/var/lib/ceph"
	cephLogDir    = "/var/log/ceph"
	lvmConfPath   = "/etc/lvm/lvm.conf"
	cvLogDir      = ""
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

func (a *OsdAgent) configureCVDevices(context *clusterd.Context, devices *DeviceOsdMapping) ([]oposd.OSDInfo, error) {
	var osds []oposd.OSDInfo
	var lv string
	var lvBackedPV bool

	var err error
	if len(devices.Entries) == 0 {
		logger.Infof("no new devices to configure. returning devices already configured with ceph-volume.")
		osds, err = getCephVolumeOSDs(context, a.cluster.Name, a.cluster.FSID, lv, false, lvBackedPV)
		if err != nil {
			logger.Infof("failed to get devices already provisioned by ceph-volume. %v", err)
		}
		return osds, nil
	}

	err = createOSDBootstrapKeyring(context, a.cluster.Name, cephConfigDir)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to generate osd keyring")
	}
	// Update LVM configuration file
	if a.pvcBacked {
		for _, device := range devices.Entries {
			lvBackedPV, err = sys.IsLV(device.Config.Name, context.Executor)
			if err != nil {
				return nil, errors.Wrapf(err, "ailed to check device type")

			}
			break
		}
	}
	if err := updateLVMConfig(context, a.pvcBacked, lvBackedPV); err != nil {
		return nil, errors.Wrapf(err, "failed to update lvm configuration file") // fail return here as validation provided by ceph-volume
	}

	if a.pvcBacked {
		if lv, err = a.initializeBlockPVC(context, devices, lvBackedPV); err != nil {
			return nil, errors.Wrapf(err, "failed to initialize devices")
		}
	} else {
		if err = a.initializeDevices(context, devices); err != nil {
			return nil, errors.Wrapf(err, "failed to initialize devices")
		}
	}

	osds, err = getCephVolumeOSDs(context, a.cluster.Name, a.cluster.FSID, lv, lvBackedPV, lvBackedPV) // skip release if PV is LV
	return osds, err
}

func (a *OsdAgent) initializeBlockPVC(context *clusterd.Context, devices *DeviceOsdMapping, lvBackedPV bool) (string, error) {
	baseCommand := "stdbuf"
	var baseArgs []string

	// Create a specific log directory so that each prepare command will have its own log
	// Only do this if nothing is present so that we don't override existing logs
	cvLogDir = path.Join(cephLogDir, a.nodeName)
	err := os.MkdirAll(cvLogDir, 0755)
	if err != nil {
		logger.Errorf("failed to create ceph-volume log directory %q, continue with default %q. %v", cvLogDir, cephLogDir, err)
		baseArgs = []string{"-oL", cephVolumeCmd}
	} else {
		baseArgs = []string{"-oL", cephVolumeCmd, "--log-path", cvLogDir}
	}

	baseArgs = append(baseArgs, []string{
		"lvm",
		"prepare",
	}...)

	var lvpath string
	for name, device := range devices.Entries {
		if device.LegacyPartitionsFound {
			logger.Infof("skipping device %s configured with legacy rook osd", name)
			continue
		}
		if device.Data == -1 {
			logger.Infof("configuring new device %s", name)
			var err error
			var deviceArg string
			if lvBackedPV {
				// pass 'vg/lv' to ceph-volume
				deviceArg, err = getLVNameFromDevicePath(context, device.Config.Name)
				if err != nil {
					return "", errors.Wrapf(err, "failed to get lv name from device path %q", device.Config.Name)
				}
			} else {
				deviceArg = device.Config.Name
			}

			immediateExecuteArgs := append(baseArgs, []string{
				"--data",
				deviceArg,
			}...)

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
				return "", errors.Wrapf(err, "failed ceph-volume") // fail return here as validation provided by ceph-volume
			}
			logger.Infof("%v", op)
			if lvBackedPV {
				lvpath = deviceArg
			} else {
				lvpath = getLVPath(op)
				if lvpath == "" {
					return "", errors.New("failed to get lvpath from ceph-volume lvm prepare output")
				}
			}

		} else {
			logger.Infof("skipping device %s with osd %d already configured", name, device.Data)
		}
	}

	return lvpath, nil
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
	if a.storeConfig.StoreType == config.Filestore {
		storeFlag = "--filestore"
	}

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
		if device.LegacyPartitionsFound {
			logger.Infof("skipping device %s configured with legacy rook osd", name)
			continue
		}

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

		if a.cluster.CephVersion.IsAtLeast(cephver.CephVersion{Major: 14, Minor: 2, Extra: 1}) {
			mdArgs = append(mdArgs, []string{
				dbDeviceFlag,
				path.Join("/dev", md),
			}...)
		} else {
			mdArgs = append(mdArgs, path.Join("/dev", md))
		}

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
			return errors.Wrapf(err, "failed ceph-volume json report") // fail return here as validation provided by ceph-volume
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

func getCephVolumeSupported(context *clusterd.Context) (bool, error) {

	_, err := context.Executor.ExecuteCommandWithCombinedOutput(false, "", cephVolumeCmd, "lvm", "batch", "--prepare")

	if err != nil {
		if cmdErr, ok := err.(*exec.CommandError); ok {
			exitStatus := cmdErr.ExitStatus()
			if exitStatus == int(syscall.ENOENT) || exitStatus == int(syscall.EPERM) {
				logger.Infof("supported version of ceph-volume not available")
				return false, nil
			}
			return false, errors.Errorf("unknown return code from ceph-volume when checking for compatibility: %d", exitStatus)
		}
		return false, errors.Wrapf(err, "unknown ceph-volume failure")
	}

	return true, nil
}

func getCephVolumeOSDs(context *clusterd.Context, clusterName string, cephfsid string, lv string, skipLVRelease, lvBackedPV bool) ([]oposd.OSDInfo, error) {

	result, err := context.Executor.ExecuteCommandWithCombinedOutput(false, "", cephVolumeCmd, "lvm", "list", lv, "--format", "json")
	if err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve ceph-volume results")
	}
	logger.Debug(result)

	var cephVolumeResult map[string][]osdInfo
	err = json.Unmarshal([]byte(result), &cephVolumeResult)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to retrieve ceph-volume results")
	}

	var osds []oposd.OSDInfo
	for name, osdInfo := range cephVolumeResult {
		id, err := strconv.Atoi(name)
		if err != nil {
			logger.Errorf("bad osd returned from ceph-volume: %q", name)
			continue
		}
		var osdFSID string
		isFilestore := false
		for _, osd := range osdInfo {
			if osd.Tags.ClusterFSID != cephfsid {
				logger.Infof("skipping osd%d: %q running on a different ceph cluster %q", id, osd.Tags.OSDFSID, osd.Tags.ClusterFSID)
				continue
			}
			osdFSID = osd.Tags.OSDFSID
			if osd.Type == "journal" {
				isFilestore = true
			}
		}
		if len(osdFSID) == 0 {
			logger.Infof("Skipping osd%d as no instances are running on ceph cluster %q", id, cephfsid)
			continue
		}
		logger.Infof("osdInfo has %d elements. %+v", len(osdInfo), osdInfo)

		configDir := "/var/lib/rook/osd" + name
		osd := oposd.OSDInfo{
			ID:                  id,
			DataPath:            configDir,
			Config:              fmt.Sprintf("%s/%s.config", configDir, clusterName),
			KeyringPath:         path.Join(configDir, "keyring"),
			Cluster:             "ceph",
			UUID:                osdFSID,
			CephVolumeInitiated: true,
			IsFileStore:         isFilestore,
			LVPath:              lv,
			SkipLVRelease:       skipLVRelease,
			LVBackedPV:          lvBackedPV,
		}
		osds = append(osds, osd)
	}
	logger.Infof("%d ceph-volume osd devices configured on this node", len(osds))

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
