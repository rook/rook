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
	"io/ioutil"
	"log"
	"os"
	"path"
	"regexp"
	"time"

	"strings"

	"github.com/rook/rook/pkg/ceph/mon"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/clusterd/inventory"
	"github.com/rook/rook/pkg/util/sys"
)

const (
	dirOSDConfigFilename = "osd-dirs"
)

func Run(context *clusterd.Context, agent *OsdAgent) error {
	// write the latest config to the config dir
	if err := mon.GenerateAdminConnectionConfig(context, agent.cluster); err != nil {
		return fmt.Errorf("failed to write connection config. %+v", err)
	}

	logger.Infof("discovering hardware")
	hardware, err := inventory.DiscoverHardware(context.Executor)
	if err != nil {
		return fmt.Errorf("failed initial hardware discovery. %+v", err)
	}
	context.Inventory = &inventory.Config{Local: hardware}

	logger.Infof("creating and starting the osds")

	// initialize the desired osds
	devices, err := getAvailableDevices(context, hardware.Disks, agent.devices, agent.usingDeviceFilter)
	if err != nil {
		return fmt.Errorf("failed to get available devices. %+v", err)
	}

	logger.Infof("configuring osd devices: %+v", devices)
	err = agent.configureDevices(context, devices)
	if err != nil {
		return fmt.Errorf("failed to configure devices. %+v", err)
	}

	// initialize the data directories, with the default dir if no devices were specified
	devicesSpecified := len(devices.Entries) > 0
	dirs, err := getDataDirs(context, agent.directories, devicesSpecified)
	if err != nil {
		return fmt.Errorf("failed to get data dirs. %+v", err)
	}
	logger.Infof("configuring osd dirs: %+v", dirs)
	err = agent.configureDirs(context, dirs)
	if err != nil {
		return fmt.Errorf("failed to configure dirs %v. %+v", dirs, err)
	}
	err = saveDirConfig(context, dirs)
	if err != nil {
		return fmt.Errorf("failed to save osd dir config. %+v", err)
	}

	// FIX
	log.Printf("sleeping a while to let the osds run...")
	<-time.After(1000000 * time.Second)
	return nil
}

func getAvailableDevices(context *clusterd.Context, devices []*inventory.LocalDisk, desiredDevices string,
	usingDeviceFilter bool) (*DeviceOsdMapping, error) {

	var deviceList []string
	if !usingDeviceFilter {
		deviceList = strings.Split(desiredDevices, ",")
	}

	available := &DeviceOsdMapping{Entries: map[string]*DeviceOsdIDEntry{}}
	for _, device := range devices {
		if device.Type == sys.PartType {
			continue
		}
		ownPartitions, fs, err := checkIfDeviceAvailable(context.Executor, device.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to get device %s info. %+v", device.Name, err)
		}
		if fs == "" && ownPartitions {
			if desiredDevices == "all" {
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
					available.Entries[device.Name] = &DeviceOsdIDEntry{Data: unassignedOSDID}
				} else {
					logger.Infof("skipping device %s that does not match the device filter/list `%s`. %+v", device.Name, desiredDevices, err)
				}

			} else {
				logger.Infof("skipping device %s until the admin specifies it can be used by an osd", device.Name)
			}
		}
	}

	return available, nil
}

func getDataDirs(context *clusterd.Context, desiredDirs string, devicesSpecified bool) (map[string]int, error) {
	var dirList []string
	if desiredDirs != "" {
		dirList = strings.Split(desiredDirs, ",")
	}

	filePath := path.Join(context.ConfigDir, dirOSDConfigFilename)

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		// the config file doesn't exist yet
		if len(dirList) == 0 {
			if devicesSpecified {
				// no dirs desired, user is using devices instead
				return map[string]int{}, nil
			} else {
				// no devices or dirs specified, return the default data dir
				return map[string]int{context.ConfigDir: unassignedOSDID}, nil
			}
		}

		dirMap := make(map[string]int, len(dirList))
		addDirsToDirMap(dirList, &dirMap)
		return dirMap, nil
	}

	// read the saved dir map from disk
	var dirMap map[string]int
	b, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(b, &dirMap)
	if err != nil {
		return nil, err
	}

	// if the user has specified any directories to use, merge them into the saved config
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

func saveDirConfig(context *clusterd.Context, config map[string]int) error {
	if len(config) == 0 {
		return nil
	}

	b, err := json.Marshal(config)
	if err != nil {
		return err
	}
	filePath := path.Join(context.ConfigDir, dirOSDConfigFilename)
	err = ioutil.WriteFile(filePath, b, 0644)
	if err != nil {
		return err
	}

	logger.Debugf("saved osd dir config to %s", filePath)
	return nil
}
