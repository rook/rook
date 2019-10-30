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
	cephConfigDir         = "/var/lib/ceph"
	lvmConfPath           = "/etc/lvm/lvm.conf"
	restoreconPath        = "/usr/sbin/restorecon"
	restoreconPathNewPath = restoreconPath + ".old"
	redHatReleaseFile     = "/etc/redhat-release"
)

const (
	osdsPerDeviceFlag    = "--osds-per-device"
	crushDeviceClassFlag = "--crush-device-class"
	encryptedFlag        = "--dmcrypt"
	databaseSizeFlag     = "--block-db-size"
	dbDeviceFlag         = "--db-devices"
	cephVolumeCmd        = "ceph-volume"
	cephVolumeMinDBSize  = 1024 // 1GB
	restoreconNewContent = `#!/usr/bin/env bash
echo "restorecon command was replaced with a no-op."
echo "original restorecon command is now at %s"
`
)

func (a *OsdAgent) configureCVDevices(context *clusterd.Context, devices *DeviceOsdMapping) ([]oposd.OSDInfo, error) {
	var osds []oposd.OSDInfo
	var lv string

	var err error
	if len(devices.Entries) == 0 {
		logger.Infof("no new devices to configure. returning devices already configured with ceph-volume.")
		osds, err = getCephVolumeOSDs(context, a.cluster.Name, a.cluster.FSID, lv, false)
		if err != nil {
			logger.Infof("failed to get devices already provisioned by ceph-volume. %+v", err)
		}
		return osds, nil
	}

	err = createOSDBootstrapKeyring(context, a.cluster.Name, cephConfigDir)
	if err != nil {
		return nil, fmt.Errorf("failed to generate osd keyring. %+v", err)
	}
	// Update LVM configuration file
	if err := updateLVMConfig(context, a.pvcBacked); err != nil {
		return nil, fmt.Errorf("failed to update lvm configuration file, %+v", err) // fail return here as validation provided by ceph-volume
	}
	// Hide restorecon command, only when hostnetworking is enabled
	if err := replaceRestoreconCommand(); err != nil {
		return nil, fmt.Errorf("failed to hide 'restorecon' command. %+v", err)
	}
	if a.pvcBacked {
		if lv, err = a.initializeBlockPVC(context, devices); err != nil {
			return nil, fmt.Errorf("failed to initialize devices. %+v", err)
		}
	} else {
		if err = a.initializeDevices(context, devices); err != nil {
			return nil, fmt.Errorf("failed to initialize devices. %+v", err)
		}
	}

	osds, err = getCephVolumeOSDs(context, a.cluster.Name, a.cluster.FSID, lv, false)
	return osds, err
}

func (a *OsdAgent) initializeBlockPVC(context *clusterd.Context, devices *DeviceOsdMapping) (string, error) {
	baseCommand := "stdbuf"
	baseArgs := []string{"-oL", cephVolumeCmd, "lvm", "prepare"}
	var lvpath string
	for name, device := range devices.Entries {
		if device.LegacyPartitionsFound {
			logger.Infof("skipping device %s configured with legacy rook osd", name)
			continue
		}
		if device.Data == -1 {
			logger.Infof("configuring new device %s", name)
			deviceArg := device.Config.Name
			immediateExecuteArgs := append(baseArgs, []string{
				"--data",
				deviceArg,
			}...)
			// execute ceph-volume with the device

			if op, err := context.Executor.ExecuteCommandWithCombinedOutput(false, "", baseCommand, immediateExecuteArgs...); err != nil {
				return "", fmt.Errorf("failed ceph-volume. %+v", err) // fail return here as validation provided by ceph-volume
			} else {
				logger.Infof("%v", op)
				lvpath = getLVPath(op)
				if lvpath == "" {
					return "", fmt.Errorf("failed to get lvpath from ceph-volume lvm prepare output")
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

func updateLVMConfig(context *clusterd.Context, onPVC bool) error {

	input, err := ioutil.ReadFile(lvmConfPath)
	if err != nil {
		return fmt.Errorf("failed to read lvm config file. %+v", err)
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
		output = bytes.Replace(output, []byte(`# filter = [ "a|.*/|" ]`), []byte(`filter = [ "a|^/mnt/.*|", "r|.*|" ]`), 1)
	}

	if err = ioutil.WriteFile(lvmConfPath, output, 0644); err != nil {
		return fmt.Errorf("failed to update lvm config file. %+v", err)
	}

	logger.Info("Successfully updated lvm config file")
	return nil
}

func replaceRestoreconCommand() error {
	// Check if Host Networking is enabled
	hostnetworking := os.Getenv("ROOK_HOST_NETWORKING")
	if hostnetworking == "false" {
		logger.Debugf("ROOK_HOST_NETWORKING is %q, not replacing 'restorecon' command", hostnetworking)
		return nil
	}

	// Check whether we are running on RHEL
	// The existence of /etc/redhat-release should be enough
	_, err := os.Stat(redHatReleaseFile)
	if os.IsNotExist(err) {
		logger.Debugf("%q does not exist, not replacing 'restorecon' command, only doing this on red hat systems", redHatReleaseFile)
		return nil
	}

	logger.Debugf("renaming %q to %q", restoreconPath, restoreconPathNewPath)
	err = os.Rename(restoreconPath, restoreconPathNewPath)
	if err != nil {
		return fmt.Errorf("failed to rename %q to %q. %+v", restoreconPath, restoreconPathNewPath, err)
	}

	logger.Debugf("writing new content to %q", restoreconPath)
	err = ioutil.WriteFile(restoreconPath, []byte(fmt.Sprintf(restoreconNewContent, restoreconPathNewPath)), 0755)
	if err != nil {
		return fmt.Errorf("failed to write new content of restorecon to %q. %+v", restoreconPath, err)
	}

	logger.Infof("Successfully replaced restorecon command to %q", restoreconPathNewPath)
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
						return fmt.Errorf("metadataDevice (%s) has more than 1 osdsPerDevice value set: %s != %s", md, deviceOSDCount, metadataDevices[md]["osdsperdevice"])
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
								return fmt.Errorf("metadataDevice (%s) has more than 1 databaseSizeMB value set: %s != %s", md, metadataDevices[md]["databasesizemb"], dbSizeString)
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
					return fmt.Errorf("failed ceph-volume report. %+v", err) // fail return here as validation provided by ceph-volume
				}

				// execute ceph-volume immediately with the device-specific setting instead of batching up multiple devices together
				if err := context.Executor.ExecuteCommand(false, "", baseCommand, immediateExecuteArgs...); err != nil {
					return fmt.Errorf("failed ceph-volume. %+v", err)
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
			return fmt.Errorf("failed ceph-volume report. %+v", err) // fail return here as validation provided by ceph-volume
		}

		reportArgs = append(reportArgs, []string{
			"--format",
			"json",
		}...)

		cvOut, err := context.Executor.ExecuteCommandWithCombinedOutput(false, "", baseCommand, reportArgs...)
		if err != nil {
			return fmt.Errorf("failed ceph-volume json report. %+v", err) // fail return here as validation provided by ceph-volume
		}

		logger.Debugf("ceph-volume report: %+v", cvOut)

		var cvReport cephVolReport
		if err = json.Unmarshal([]byte(cvOut), &cvReport); err != nil {
			return fmt.Errorf("failed to unmarshal ceph-volume report json. %+v", err)
		}

		if path.Join("/dev", md) != cvReport.Vg.Devices {
			return fmt.Errorf("ceph-volume did not use the expected metadataDevice [%s]", md)
		}

		// execute ceph-volume batching up multiple devices
		if err := context.Executor.ExecuteCommand(false, "", baseCommand, mdArgs...); err != nil {
			return fmt.Errorf("failed ceph-volume. %+v", err) // fail return here as validation provided by ceph-volume
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
			return false, fmt.Errorf("unknown return code from ceph-volume when checking for compatibility: %d", exitStatus)
		}
		return false, fmt.Errorf("unknown ceph-volume failure. %+v", err)
	}

	return true, nil
}

func getCephVolumeOSDs(context *clusterd.Context, clusterName string, cephfsid string, lv string, skipLVRelease bool) ([]oposd.OSDInfo, error) {

	result, err := context.Executor.ExecuteCommandWithCombinedOutput(false, "", cephVolumeCmd, "lvm", "list", lv, "--format", "json")
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve ceph-volume results. %+v", err)
	}
	logger.Debug(result)

	var cephVolumeResult map[string][]osdInfo
	err = json.Unmarshal([]byte(result), &cephVolumeResult)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve ceph-volume results. %+v", err)
	}

	var osds []oposd.OSDInfo
	for name, osdInfo := range cephVolumeResult {
		id, err := strconv.Atoi(name)
		if err != nil {
			logger.Errorf("bad osd returned from ceph-volume: %s", name)
			continue
		}
		var osdFSID string
		isFilestore := false
		for _, osd := range osdInfo {
			if osd.Tags.ClusterFSID != cephfsid {
				logger.Infof("skipping osd%d: %s running on a different ceph cluster: %s", id, osd.Tags.OSDFSID, osd.Tags.ClusterFSID)
				continue
			}
			osdFSID = osd.Tags.OSDFSID
			if osd.Type == "journal" {
				isFilestore = true
			}
		}
		if len(osdFSID) == 0 {
			logger.Infof("Skipping osd%d as no instances are running on ceph cluster: %s", id, cephfsid)
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
		}
		osds = append(osds, osd)
	}
	logger.Infof("%d ceph-volume osd devices configured on this node", len(osds))

	return osds, nil
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
