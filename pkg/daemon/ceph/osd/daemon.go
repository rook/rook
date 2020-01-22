/*
Copyright 2016 The Rook Authors. All rights reserved.

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
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	oposd "github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/sys"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
)

var (
	logger = capnslog.NewPackageLogger("github.com/rook/rook", "cephosd")
)

// StartOSD starts an OSD on a device that was provisioned by ceph-volume
func StartOSD(context *clusterd.Context, osdType, osdID, osdUUID, lvPath string, pvcBackedOSD, lvBackedPV bool, cephArgs []string) error {

	// ensure the config mount point exists
	configDir := fmt.Sprintf("/var/lib/ceph/osd/ceph-%s", osdID)
	err := os.Mkdir(configDir, 0755)
	if err != nil {
		logger.Errorf("failed to create config dir %q. %v", configDir, err)
	}

	// Update LVM config at runtime
	if err := updateLVMConfig(context, pvcBackedOSD, lvBackedPV); err != nil {
		return errors.Wrapf(err, "failed to update lvm configuration file") // fail return here as validation provided by ceph-volume
	}

	var volumeGroupName string
	if pvcBackedOSD && !lvBackedPV {
		volumeGroupName, err = getVolumeGroupName(lvPath)
		if err != nil {
			return errors.Wrapf(err, "error fetching volume group name for OSD %q", osdID)
		}
		go handleTerminate(context, lvPath, volumeGroupName)

		if err := context.Executor.ExecuteCommand(false, "", "vgchange", "-an", volumeGroupName); err != nil {
			return errors.Wrapf(err, "failed to deactivate volume group for lv %q", lvPath)
		}

		if err := context.Executor.ExecuteCommand(false, "", "vgchange", "-ay", volumeGroupName); err != nil {
			return errors.Wrapf(err, "failed to activate volume group for lv %q", lvPath)
		}
	}

	// activate the osd with ceph-volume
	storeFlag := "--" + osdType
	if err := context.Executor.ExecuteCommand(false, "", "stdbuf", "-oL", "ceph-volume", "lvm", "activate", "--no-systemd", storeFlag, osdID, osdUUID); err != nil {
		return errors.Wrapf(err, "failed to activate osd")
	}

	// run the ceph-osd daemon
	if err := context.Executor.ExecuteCommand(false, "", "ceph-osd", cephArgs...); err != nil {
		// Instead of returning, we want to allow the lvm release to happen below, so we just log the err
		logger.Errorf("failed to start osd or shutting down. %v", err)
	}

	if pvcBackedOSD && !lvBackedPV {
		if err := releaseLVMDevice(context, volumeGroupName); err != nil {
			return errors.Wrapf(err, "failed to release device from lvm")
		}
	}

	return nil
}

func handleTerminate(context *clusterd.Context, lvPath, volumeGroupName string) error {
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGTERM)
	for {
		select {
		case <-sigc:
			logger.Infof("shutdown signal received, exiting...")
			err := killCephOSDProcess(context, lvPath)
			if err != nil {
				return errors.Wrapf(err, "failed to kill ceph-osd process")
			}
			return nil
		}
	}
}

func killCephOSDProcess(context *clusterd.Context, lvPath string) error {

	pid, err := context.Executor.ExecuteCommandWithOutput(false, "", "fuser", "-a", lvPath)
	if err != nil {
		return errors.Wrapf(err, "failed to retrieve process ID for %q", lvPath)
	}

	logger.Infof("process ID for ceph-osd: %s", pid)

	// shut down the osd-ceph process so that lvm release does not show device in use error.
	if pid != "" {
		// The OSD needs to exit as quickly as possible in order for the IO requests
		// to be redirected to other OSDs in the cluster. The OSD is designed to tolerate failures
		// of any kind, including power loss or kill -9. The upstream Ceph tests have for many years
		// been testing with kill -9 so this is expected to be safe. There is a fix upstream Ceph that will
		// improve the shutdown time of the OSD. For cleanliness we should consider removing the -9
		// once it is backported to Nautilus: https://github.com/ceph/ceph/pull/31677.
		if err := context.Executor.ExecuteCommand(false, "", "kill", "-9", pid); err != nil {
			return errors.Wrapf(err, "failed to kill ceph-osd process")
		}
	}

	return nil
}

// RunFilestoreOnDevice runs a Ceph filestore OSD on a device. For filestore devices, Rook must
// first mount the filesystem on disk (source path) to a path (mount path) from which the OSD will
// be run.
func RunFilestoreOnDevice(context *clusterd.Context, mountSourcePath, mountPath string, cephArgs []string) error {
	// start the OSD daemon in the foreground with the given config
	logger.Infof("starting filestore osd on a device")

	if err := sys.MountDevice(mountSourcePath, mountPath, context.Executor); err != nil {
		return errors.Wrapf(err, "failed to mount device")
	}
	// unmount the device before exit
	defer sys.UnmountDevice(mountPath, context.Executor)

	// run the ceph-osd daemon
	if err := context.Executor.ExecuteCommand(false, "", "ceph-osd", cephArgs...); err != nil {
		return errors.Wrapf(err, "failed to start osd")
	}

	return nil
}

// Provision provisions an OSD
func Provision(context *clusterd.Context, agent *OsdAgent, crushLocation string) error {
	// set the initial orchestration status
	status := oposd.OrchestrationStatus{Status: oposd.OrchestrationStatusComputingDiff}
	if err := oposd.UpdateNodeStatus(agent.kv, agent.nodeName, status); err != nil {
		return err
	}

	// create the ceph.conf with the default settings
	cephConfig, err := cephconfig.CreateDefaultCephConfig(context, agent.cluster)
	if err != nil {
		return errors.Wrapf(err, "failed to create default ceph config")
	}

	// write the latest config to the config dir
	confFilePath, err := cephconfig.GenerateAdminConnectionConfigWithSettings(context, agent.cluster, cephConfig)
	if err != nil {
		return errors.Wrapf(err, "failed to write connection config")
	}
	src, err := ioutil.ReadFile(confFilePath)
	if err != nil {
		return errors.Wrapf(err, "failed to copy connection config to /etc/ceph. failed to read the connection config")
	}
	err = ioutil.WriteFile(cephconfig.DefaultConfigFilePath(), src, 0444)
	if err != nil {
		return errors.Wrapf(err, "failed to copy connection config to /etc/ceph. failed to write %q", cephconfig.DefaultConfigFilePath())
	}
	dst, err := ioutil.ReadFile(cephconfig.DefaultConfigFilePath())
	if err == nil {
		logger.Debugf("config file @ %s: %s", cephconfig.DefaultConfigFilePath(), dst)
	} else {
		logger.Warningf("wrote and copied config file but failed to read it back from %s for logging. %v", cephconfig.DefaultConfigFilePath(), err)
	}

	logger.Infof("discovering hardware")
	var rawDevices []*sys.LocalDisk
	if agent.pvcBacked {
		if len(agent.devices) > 1 {
			return errors.New("more than one desired device found in case of PVC backed OSDs. we expect exactly one device")
		}
		rawDevice, err := clusterd.PopulateDeviceInfo(agent.devices[0].Name, context.Executor)
		if err != nil {
			return errors.Wrapf(err, "failed to get device info for %q", agent.devices[0].Name)
		}
		rawDevices = append(rawDevices, rawDevice)
	} else {
		rawDevices, err = clusterd.DiscoverDevices(context.Executor)
		if err != nil {
			return errors.Wrapf(err, "failed initial hardware discovery")
		}
	}

	context.Devices = rawDevices

	logger.Infof("creating and starting the osds")

	// determine the set of devices that can/should be used for OSDs.
	devices, err := getAvailableDevices(context, agent.devices, agent.metadataDevice, agent.pvcBacked)
	if err != nil {
		return errors.Wrapf(err, "failed to get available devices")
	}

	// orchestration is about to start, update the status
	status = oposd.OrchestrationStatus{Status: oposd.OrchestrationStatusOrchestrating, PvcBackedOSD: agent.pvcBacked}
	if err := oposd.UpdateNodeStatus(agent.kv, agent.nodeName, status); err != nil {
		return err
	}

	// start the desired OSDs on devices
	logger.Infof("configuring osd devices: %+v", devices)
	deviceOSDs, err := agent.configureDevices(context, devices)
	if err != nil {
		return errors.Wrapf(err, "failed to configure devices")
	}

	// Populate CRUSH location for each OSD on the host
	for i := range deviceOSDs {
		deviceOSDs[i].Location = crushLocation
	}

	// determine the set of directories that can/should be used for OSDs, with the default dir if no devices were specified. save off the node's crush name if needed.
	logger.Infof("devices = %+v", deviceOSDs)
	devicesConfigured := len(deviceOSDs) > 0
	dirs, err := getDataDirs(context, agent.kv, agent.directories, devicesConfigured, agent.nodeName)
	if err != nil {
		return errors.Wrapf(err, "failed to get data dirs")
	}

	// start up the OSDs for directories
	logger.Infof("configuring osd dirs: %+v", dirs)
	dirOSDs, err := agent.configureDirs(context, dirs)
	if err != nil {
		return errors.Wrapf(err, "failed to configure dirs %+v", dirs)
	}

	logger.Info("saving osd dir map")
	if err := config.SaveOSDDirMap(agent.kv, agent.nodeName, dirs); err != nil {
		return errors.Wrapf(err, "failed to save osd dir map")
	}

	logger.Infof("device osds:%+v\ndir osds: %+v", deviceOSDs, dirOSDs)

	if agent.pvcBacked && !deviceOSDs[0].SkipLVRelease && !deviceOSDs[0].LVBackedPV {
		volumeGroupName, err := getVolumeGroupName(deviceOSDs[0].LVPath)
		if err != nil {
			return errors.Wrapf(err, "error fetching volume group name")
		}
		if err := releaseLVMDevice(context, volumeGroupName); err != nil {
			return errors.Wrapf(err, "failed to release device from lvm")
		}
	}

	osds := append(deviceOSDs, dirOSDs...)

	// orchestration is completed, update the status
	status = oposd.OrchestrationStatus{OSDs: osds, Status: oposd.OrchestrationStatusCompleted, PvcBackedOSD: agent.pvcBacked}
	if err := oposd.UpdateNodeStatus(agent.kv, agent.nodeName, status); err != nil {
		return err
	}

	return nil
}

func getAvailableDevices(context *clusterd.Context, desiredDevices []DesiredDevice, metadataDevice string, pvcBacked bool) (*DeviceOsdMapping, error) {

	available := &DeviceOsdMapping{Entries: map[string]*DeviceOsdIDEntry{}}
	for _, device := range context.Devices {
		var partCount int
		var ownPartitions bool
		var err error
		var fs string
		if device.Type != sys.PartType {
			partCount, ownPartitions, fs, err = sys.CheckIfDeviceAvailable(context.Executor, device.Name, pvcBacked)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to get device %q info", device.Name)
			}

			if fs != "" || !ownPartitions {
				// not OK to use the device because it has a filesystem or rook doesn't own all its partitions
				logger.Infof("skipping device %q that is in use (not by rook). fs: %s, ownPartitions: %t", device.Name, fs, ownPartitions)
				continue
			}
		}

		if device.Filesystem != "" {
			logger.Infof("skipping device %q because it contains a filesystem %q", device.Name, device.Filesystem)
			continue
		}

		var deviceInfo *DeviceOsdIDEntry
		if metadataDevice != "" && metadataDevice == device.Name {
			// current device is desired as the metadata device
			deviceInfo = &DeviceOsdIDEntry{Data: unassignedOSDID, Metadata: []int{}}
		} else if len(desiredDevices) == 1 && desiredDevices[0].Name == "all" {
			// user has specified all devices, use the current one for data
			deviceInfo = &DeviceOsdIDEntry{Data: unassignedOSDID}
		} else if len(desiredDevices) > 0 {
			var matched bool
			var matchedDevice DesiredDevice
			for _, desiredDevice := range desiredDevices {
				if desiredDevice.IsFilter {
					// the desired devices is a regular expression
					matched, err = regexp.Match(desiredDevice.Name, []byte(device.Name))
					if err != nil {
						logger.Errorf("regex failed on device %q and filter %q. %v", device.Name, desiredDevice.Name, err)
						continue
					}

					if matched {
						logger.Infof("device %q matches device filter %q", device.Name, desiredDevice.Name)
					}
				} else if desiredDevice.IsDevicePathFilter {
					pathnames := append(strings.Fields(device.DevLinks), filepath.Join("/dev", device.Name))
					for _, pathname := range pathnames {
						matched, err = regexp.Match(desiredDevice.Name, []byte(pathname))
						if err != nil {
							logger.Errorf("regex failed on device %q and filter %q. %v", device.Name, desiredDevice.Name, err)
							continue
						}

						if matched {
							logger.Infof("device %q (aliases: %q) matches device path filter %q", device.Name, device.DevLinks, desiredDevice.Name)
							break
						}
					}
				} else if device.Name == desiredDevice.Name {
					logger.Infof("%q found in the desired devices", device.Name)
					matched = true
				}
				matchedDevice = desiredDevice
				if matched {
					break
				}
			}

			if err == nil && matched {
				// the current device matches the user specifies filter/list, use it for data
				logger.Infof("device %q is selected by the device filter/name %q", device.Name, matchedDevice.Name)
				deviceInfo = &DeviceOsdIDEntry{Data: unassignedOSDID, Config: matchedDevice, PersistentDevicePaths: strings.Fields(device.DevLinks)}
			} else {
				logger.Infof("skipping device %q that does not match the device filter/list (%v). %v", device.Name, desiredDevices, err)
			}
		} else {
			logger.Infof("skipping device %q until the admin specifies it can be used by an osd", device.Name)
		}

		if deviceInfo != nil {
			if partCount > 0 {
				deviceInfo.LegacyPartitionsFound = ownPartitions
			}
			available.Entries[device.Name] = deviceInfo
		}
	}

	return available, nil
}

func getDataDirs(context *clusterd.Context, kv *k8sutil.ConfigMapKVStore, desiredDirs string,
	devicesSpecified bool, nodeName string) (dirs map[string]int, err error) {

	var dirList []string
	if desiredDirs != "" {
		dirList = strings.Split(desiredDirs, ",")
	}

	// when user has not specified any dirs or any devices, legacy behavior was to give them the
	// default dir. no longer automatically create this fallback osd. the legacy conditional is
	// still important for determining when the fallback osd may be deleted.
	noDirsOrDevicesSpecified := len(dirList) == 0 && !devicesSpecified

	dirMap, err := config.LoadOSDDirMap(kv, nodeName)
	if err == nil {
		// we have an existing saved dir map, merge the user specified directories into it
		addDirsToDirMap(dirList, &dirMap)

		// determine which dirs are still active, which should be removed, then return them
		activeDirs := getActiveDirs(dirList, dirMap, context.ConfigDir, noDirsOrDevicesSpecified)
		return activeDirs, nil
	}

	if !kerrors.IsNotFound(err) {
		// real error when trying to load the osd dir map, return the err
		return nil, errors.Wrapf(err, "failed to load OSD dir map")
	}

	// the osd dirs map doesn't exist yet

	if len(dirList) == 0 {
		// no dirs should be used because the user has requested no dirs but they have requested devices
		return map[string]int{}, nil
	}

	// add the specified dirs to the map and return it
	dirMap = make(map[string]int, len(dirList))
	addDirsToDirMap(dirList, &dirMap)
	return dirMap, nil
}

func addDirsToDirMap(dirList []string, dirMap *map[string]int) {
	for _, d := range dirList {
		if _, ok := (*dirMap)[d]; !ok {
			// the users dir isn't already in the map, add it with an unassigned ID
			(*dirMap)[d] = unassignedOSDID
		}
	}
}

func getActiveDirs(currentDirList []string, savedDirMap map[string]int, configDir string, noDirsOrDevicesSpecified bool) (activeDirs map[string]int) {
	activeDirs = map[string]int{}

	for savedDir, id := range savedDirMap {
		foundSavedDir := false

		// If a legacy 'fallback' osd and no dirs/devices are yet specified, keep it to preserve
		// legacy behavior for migrated clusters.
		if savedDir == configDir && noDirsOrDevicesSpecified {
			foundSavedDir = true
		}

		for _, dir := range currentDirList {
			if dir == savedDir {
				foundSavedDir = true
				break
			}
		}

		if foundSavedDir {
			// the saved dir is still active
			activeDirs[savedDir] = id
		}
	}

	return activeDirs
}

//releaseLVMDevice deactivates the LV to release the device.
func releaseLVMDevice(context *clusterd.Context, volumeGroupName string) error {
	if err := context.Executor.ExecuteCommand(false, "", "lvchange", "-an", volumeGroupName); err != nil {
		return errors.Wrapf(err, "failed to deactivate LVM %s", volumeGroupName)
	}
	logger.Info("Successfully released device from lvm")
	return nil
}

//getVolumeGroupName returns the Volume group name from the given Logical Volume Path
func getVolumeGroupName(lvPath string) (string, error) {
	if lvPath == "" {
		return "", errors.Errorf("empty LV Path : %s", lvPath)
	}

	vgSlice := strings.Split(lvPath, "/")
	//Assert that lvpath is in correct format `/dev/<vg name>/<lv name>` before extracting the vg name
	if len(vgSlice) != 4 || vgSlice[2] == "" {
		return "", errors.Errorf("invalid LV Path : %s", lvPath)
	}

	return vgSlice[2], nil
}
