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
	"path"
	"regexp"

	"strings"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/mon"
	oposd "github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/sys"
	"k8s.io/apimachinery/pkg/api/errors"
)

var (
	logger = capnslog.NewPackageLogger("github.com/rook/rook", "cephosd")
)

func RunFilestoreOnDevice(context *clusterd.Context, mountSourcePath, mountPath string, cephArgs []string) error {

	// start the OSD daemon in the foreground with the given config
	logger.Infof("starting filestore osd on a device")

	if err := sys.MountDevice(mountSourcePath, mountPath, context.Executor); err != nil {
		return fmt.Errorf("failed to mount device. %+v", err)
	}
	// unmount the device before exit
	defer sys.UnmountDevice(mountPath, context.Executor)

	// run the ceph-osd daemon
	if err := context.Executor.ExecuteCommand(false, "", "ceph-osd", cephArgs...); err != nil {
		return fmt.Errorf("failed to start osd. %+v", err)
	}

	return nil
}

func Provision(context *clusterd.Context, agent *OsdAgent) error {

	// set the initial orchestration status
	status := oposd.OrchestrationStatus{Status: oposd.OrchestrationStatusComputingDiff}
	if err := oposd.UpdateNodeStatus(agent.kv, agent.nodeName, status); err != nil {
		return err
	}

	// set the crush location in the osd config file
	cephConfig := mon.CreateDefaultCephConfig(context, agent.cluster, path.Join(context.ConfigDir, agent.cluster.Name))
	cephConfig.GlobalConfig.CrushLocation = agent.location

	// write the latest config to the config dir
	if err := mon.GenerateAdminConnectionConfigWithSettings(context, agent.cluster, cephConfig); err != nil {
		return fmt.Errorf("failed to write connection config. %+v", err)
	}

	logger.Infof("discovering hardware")
	rawDevices, err := clusterd.DiscoverDevices(context.Executor)
	if err != nil {
		return fmt.Errorf("failed initial hardware discovery. %+v", err)
	}
	context.Devices = rawDevices

	logger.Infof("creating and starting the osds")

	// determine the set of devices that can/should be used for OSDs.
	devices, err := getAvailableDevices(context, agent.devices, agent.metadataDevice, agent.usingDeviceFilter)
	if err != nil {
		return fmt.Errorf("failed to get available devices. %+v", err)
	}

	// determine the set of removed OSDs and the node's crush name (if needed)
	removedDevicesScheme, _, err := getRemovedDevices(agent)
	if err != nil {
		return fmt.Errorf("failed to get removed devices: %+v", err)
	}

	// determine the set of directories that can/should be used for OSDs, with the default dir if no devices were specified.  save off the node's crush name if needed.
	devicesSpecified := len(agent.devices) > 0
	dirs, removedDirs, err := getDataDirs(context, agent.kv, agent.directories, devicesSpecified, agent.nodeName)
	if err != nil {
		return fmt.Errorf("failed to get data dirs. %+v", err)
	}

	// orchestration is about to start, update the status
	status = oposd.OrchestrationStatus{Status: oposd.OrchestrationStatusOrchestrating}
	if err := oposd.UpdateNodeStatus(agent.kv, agent.nodeName, status); err != nil {
		return err
	}

	// start the desired OSDs on devices
	logger.Infof("configuring osd devices: %+v", devices)
	deviceOSDs, err := agent.configureDevices(context, devices)
	if err != nil {
		return fmt.Errorf("failed to configure devices. %+v", err)
	}

	// start up the OSDs for directories
	logger.Infof("configuring osd dirs: %+v", dirs)
	dirOSDs, err := agent.configureDirs(context, dirs)
	if err != nil {
		return fmt.Errorf("failed to configure dirs %v. %+v", dirs, err)
	}

	// now we can start removing OSDs from devices and directories
	logger.Infof("removing osd devices: %+v", removedDevicesScheme)
	if err := agent.removeDevices(context, removedDevicesScheme); err != nil {
		return fmt.Errorf("failed to remove devices. %+v", err)
	}

	logger.Infof("removing osd dirs: %+v", removedDirs)
	if err := agent.removeDirs(context, removedDirs); err != nil {
		return fmt.Errorf("failed to remove dirs. %+v", err)
	}

	logger.Info("saving osd dir map")
	if err := config.SaveOSDDirMap(agent.kv, agent.nodeName, dirs); err != nil {
		return fmt.Errorf("failed to save osd dir map. %+v", err)
	}

	logger.Infof("device osds:%v\ndir osds: %v", deviceOSDs, dirOSDs)
	osds := append(deviceOSDs, dirOSDs...)

	// orchestration is completed, update the status
	status = oposd.OrchestrationStatus{OSDs: osds, Status: oposd.OrchestrationStatusCompleted}
	if err := oposd.UpdateNodeStatus(agent.kv, agent.nodeName, status); err != nil {
		return err
	}

	return nil
}

func getAvailableDevices(context *clusterd.Context, desiredDevices string, metadataDevice string, usingDeviceFilter bool) (*DeviceOsdMapping, error) {

	var deviceList []string
	if !usingDeviceFilter {
		deviceList = strings.Split(desiredDevices, ",")
	}

	available := &DeviceOsdMapping{Entries: map[string]*DeviceOsdIDEntry{}}

	if oposd.IsRemovingNode(desiredDevices) {
		// the node is being removed, just return an empty set
		return available, nil
	}

	for _, device := range context.Devices {
		if device.Type == sys.PartType {
			continue
		}
		ownPartitions, fs, err := sys.CheckIfDeviceAvailable(context.Executor, device.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to get device %s info. %+v", device.Name, err)
		}

		if fs != "" || !ownPartitions {
			// not OK to use the device because it has a filesystem or rook doesn't own all its partitions
			logger.Infof("skipping device %s that is in use (not by rook). fs: %s, ownPartitions: %t", device.Name, fs, ownPartitions)
			continue
		}

		if metadataDevice != "" && metadataDevice == device.Name {
			// current device is desired as the metadata device
			available.Entries[device.Name] = &DeviceOsdIDEntry{Data: unassignedOSDID, Metadata: []int{}}
		} else if desiredDevices == "all" {
			// user has specified all devices, use the current one for data
			available.Entries[device.Name] = &DeviceOsdIDEntry{Data: unassignedOSDID}
		} else if desiredDevices != "" {
			var matched bool
			var err error
			if usingDeviceFilter {
				// the desired devices is a regular expression
				matched, err = regexp.Match(desiredDevices, []byte(device.Name))
			} else {
				for i := range deviceList {
					if device.Name == deviceList[i] {
						matched = true
						break
					}
				}
			}

			if err == nil && matched {
				// the current device matches the user specifies filter/list, use it for data
				available.Entries[device.Name] = &DeviceOsdIDEntry{Data: unassignedOSDID}
			} else {
				logger.Infof("skipping device %s that does not match the device filter/list `%s`. %+v", device.Name, desiredDevices, err)
			}
		} else {
			logger.Infof("skipping device %s until the admin specifies it can be used by an osd", device.Name)
		}
	}

	return available, nil
}

func getDataDirs(context *clusterd.Context, kv *k8sutil.ConfigMapKVStore, desiredDirs string,
	devicesSpecified bool, nodeName string) (dirs, removedDirs map[string]int, err error) {

	var dirList []string
	if desiredDirs != "" {
		dirList = strings.Split(desiredDirs, ",")
	}

	if len(dirList) == 0 && !devicesSpecified {
		// user has not specified any dirs or any devices, give them the default dir at least
		dirList = append(dirList, context.ConfigDir)
	}

	removedDirs = make(map[string]int)

	dirMap, err := config.LoadOSDDirMap(kv, nodeName)
	if err == nil {
		// we have an existing saved dir map, merge the user specified directories into it
		addDirsToDirMap(dirList, &dirMap)

		// determine which dirs are still active, which should be removed, then return them
		activeDirs, removedDirs := getActiveAndRemovedDirs(dirList, dirMap)
		return activeDirs, removedDirs, nil
	}

	if !errors.IsNotFound(err) {
		// real error when trying to load the osd dir map, return the err
		return nil, nil, fmt.Errorf("failed to load OSD dir map: %+v", err)
	}

	// the osd dirs map doesn't exist yet

	if len(dirList) == 0 {
		// no dirs should be used because the user has requested no dirs but they have requested devices
		return map[string]int{}, removedDirs, nil
	}

	// add the specified dirs to the map and return it
	dirMap = make(map[string]int, len(dirList))
	addDirsToDirMap(dirList, &dirMap)
	return dirMap, removedDirs, nil
}

func addDirsToDirMap(dirList []string, dirMap *map[string]int) {
	for _, d := range dirList {
		if _, ok := (*dirMap)[d]; !ok {
			// the users dir isn't already in the map, add it with an unassigned ID
			(*dirMap)[d] = unassignedOSDID
		}
	}
}

func getRemovedDevices(agent *OsdAgent) (*config.PerfScheme, *DeviceOsdMapping, error) {
	removedDevicesScheme := config.NewPerfScheme()
	removedDevicesMapping := &DeviceOsdMapping{Entries: map[string]*DeviceOsdIDEntry{}}

	if !oposd.IsRemovingNode(agent.devices) {
		// TODO: support more removed device scenarios beyond just entire node removal
		return removedDevicesScheme, removedDevicesMapping, nil
	}

	scheme, err := config.LoadScheme(agent.kv, config.GetConfigStoreName(agent.nodeName))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load agent's partition scheme: %+v", err)
	}

	for _, entry := range scheme.Entries {
		// determine which partition the data lives on for this entry
		dataDetails, ok := entry.Partitions[entry.GetDataPartitionType()]
		if !ok || dataDetails == nil {
			return nil, nil, fmt.Errorf("failed to find data partition for entry %+v", entry)
		}

		// add the current scheme entry to the removed devices scheme and its device to the removed
		// devices mapping
		removedDevicesScheme.Entries = append(removedDevicesScheme.Entries, entry)
		removedDevicesMapping.Entries[dataDetails.Device] = &DeviceOsdIDEntry{Data: entry.ID}
	}

	return removedDevicesScheme, removedDevicesMapping, nil
}

func getActiveAndRemovedDirs(currentDirList []string, savedDirMap map[string]int) (activeDirs, removedDirs map[string]int) {
	activeDirs = map[string]int{}
	removedDirs = map[string]int{}

	for savedDir, id := range savedDirMap {
		foundSavedDir := false
		for _, dir := range currentDirList {
			if dir == savedDir {
				foundSavedDir = true
				break
			}
		}

		if foundSavedDir {
			// the saved dir is still active
			activeDirs[savedDir] = id
		} else {
			// the saved dir was not found in the current dir list, meaning the user wants this dir removed
			removedDirs[savedDir] = id
		}
	}

	return activeDirs, removedDirs
}
