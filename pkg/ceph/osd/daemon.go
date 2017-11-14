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
	"encoding/json"
	"fmt"
	"log"
	"path"
	"regexp"
	"time"

	"strings"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/ceph/mon"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util/kvstore"
	"github.com/rook/rook/pkg/util/sys"
)

const (
	osdDirsKeyName = "osd-dirs"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "cephosd")

func Run(context *clusterd.Context, agent *OsdAgent) error {
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

	// initialize the desired osds
	devices, err := getAvailableDevices(context, agent.devices, agent.metadataDevice, agent.usingDeviceFilter)
	if err != nil {
		return fmt.Errorf("failed to get available devices. %+v", err)
	}

	logger.Infof("configuring osd devices: %+v", devices)
	err = agent.configureDevices(context, devices)
	if err != nil {
		return fmt.Errorf("failed to configure devices. %+v", err)
	}

	// initialize the data directories, with the default dir if no devices were specified
	devicesSpecified := len(agent.devices) > 0
	dirs, err := getDataDirs(context, agent.kv, agent.directories, devicesSpecified, agent.nodeName)
	if err != nil {
		return fmt.Errorf("failed to get data dirs. %+v", err)
	}
	logger.Infof("configuring osd dirs: %+v", dirs)
	err = agent.configureDirs(context, dirs)
	if err != nil {
		return fmt.Errorf("failed to configure dirs %v. %+v", dirs, err)
	}
	err = saveOSDDirMap(agent.kv, agent.nodeName, dirs)
	if err != nil {
		return fmt.Errorf("failed to save osd dir map. %+v", err)
	}

	// OSD processes monitoring
	mon := NewMonitor(context, agent)
	go mon.Run()

	// FIX
	log.Printf("sleeping a while to let the osds run...")
	<-time.After(1000000 * time.Second)

	return nil
}

func getAvailableDevices(context *clusterd.Context, desiredDevices string, metadataDevice string, usingDeviceFilter bool) (*DeviceOsdMapping, error) {

	var deviceList []string
	if !usingDeviceFilter {
		deviceList = strings.Split(desiredDevices, ",")
	}

	available := &DeviceOsdMapping{Entries: map[string]*DeviceOsdIDEntry{}}
	for _, device := range context.Devices {
		if device.Type == sys.PartType {
			continue
		}
		ownPartitions, fs, err := checkIfDeviceAvailable(context.Executor, device.Name)
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

func getDataDirs(context *clusterd.Context, kv kvstore.KeyValueStore, desiredDirs string,
	devicesSpecified bool, nodeName string) (map[string]int, error) {

	var dirList []string
	if desiredDirs != "" {
		dirList = strings.Split(desiredDirs, ",")
	}

	dirMap, err := loadOSDDirMap(kv, nodeName)
	if err == nil {
		// we have an existing saved dir map.  if the user has specified any directories to use, merge them into the saved dir map
		addDirsToDirMap(dirList, &dirMap)
		return dirMap, nil
	}

	if !kvstore.IsNotExist(err) {
		// real error when trying to load the osd dir map, return the err
		return nil, fmt.Errorf("failed to load OSD dir map: %+v", err)
	}

	// the osd dirs map doesn't exist yet
	if len(dirList) == 0 {
		// no dirs have been specified
		if devicesSpecified {
			// user is using devices instead of dirs
			return map[string]int{}, nil
		}

		// no devices or dirs specified, return the default data dir
		return map[string]int{context.ConfigDir: unassignedOSDID}, nil
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

func loadOSDDirMap(kv kvstore.KeyValueStore, nodeName string) (map[string]int, error) {
	dirMapRaw, err := kv.GetValue(getConfigStoreName(nodeName), osdDirsKeyName)
	if err != nil {
		return nil, err
	}

	var dirMap map[string]int
	err = json.Unmarshal([]byte(dirMapRaw), &dirMap)
	if err != nil {
		return nil, err
	}

	return dirMap, nil
}

func saveOSDDirMap(kv kvstore.KeyValueStore, nodeName string, dirMap map[string]int) error {
	if len(dirMap) == 0 {
		return nil
	}

	b, err := json.Marshal(dirMap)
	if err != nil {
		return err
	}

	err = kv.SetValue(getConfigStoreName(nodeName), osdDirsKeyName, string(b))
	if err != nil {
		return err
	}

	return nil
}
