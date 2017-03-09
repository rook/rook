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

	"github.com/rook/rook/pkg/cephmgr/mon"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/clusterd/inventory"
	"github.com/rook/rook/pkg/util/sys"
)

const (
	dirOSDConfigFilename = "osd-dirs"
)

func Run(dcontext *clusterd.DaemonContext, agent *OsdAgent) error {

	logger.Infof("discovering hardware")
	context := clusterd.ToContext(dcontext)
	hardware, err := inventory.DiscoverHardware(context.Executor)
	if err != nil {
		return fmt.Errorf("failed initial hardware discovery. %+v", err)
	}
	context.Inventory = &inventory.Config{Local: hardware}

	// Connect to the ceph cluster
	logger.Infof("connecting to the mons")
	adminConn, err := mon.ConnectToClusterAsAdmin(context, agent.factory, agent.cluster)
	if err != nil {
		return fmt.Errorf("failed to open mon connection to config the osds. %+v", err)
	}
	defer adminConn.Shutdown()
	logger.Infof("Connected to the mons!")

	logger.Infof("creating and starting the osds")

	// generate and write the OSD bootstrap keyring
	if err := createOSDBootstrapKeyring(adminConn, context.ConfigDir, agent.cluster.Name); err != nil {
		return fmt.Errorf("failed to create osd bootstrap keyring. %+v", err)
	}

	// initialize the desired osds
	devices, err := getAvailableDevices(dcontext, hardware.Disks, agent.devices)
	if err != nil {
		return fmt.Errorf("failed to get available devices. %+v", err)
	}

	logger.Infof("configuring osd devices: %+v", devices)
	err = agent.configureDevices(context, devices)
	if err != nil {
		return fmt.Errorf("failed to configure devices. %+v", err)
	}

	// initialize the data directories, with the default dir if no devices were specified
	useDataDir := len(devices.Entries) == 0
	dirs, err := getDataDirs(dcontext, useDataDir)
	if err != nil {
		return fmt.Errorf("failed to get data dirs. %+v", err)
	}
	logger.Infof("configuring osd dirs: %+v", dirs)
	err = agent.configureDirs(context, dirs)
	if err != nil {
		return fmt.Errorf("failed to configure dirs %v. %+v", dirs, err)
	}
	err = saveDirConfig(dcontext, dirs)
	if err != nil {
		return fmt.Errorf("failed to save osd dir config. %+v", err)
	}

	// FIX
	log.Printf("sleeping a while to let the osds run...")
	<-time.After(1000000 * time.Second)
	return nil
}

func getAvailableDevices(context *clusterd.DaemonContext, devices []*inventory.LocalDisk, desiredDevices string) (*DeviceOsdMapping, error) {
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
				// the desired devices is a regular expression
				matched, err := regexp.Match(desiredDevices, []byte(device.Name))
				if err == nil && matched {
					available.Entries[device.Name] = &DeviceOsdIDEntry{Data: unassignedOSDID}
				} else {
					logger.Infof("skipping device %s that does not match the regular expression %s. %+v", device.Name, desiredDevices, err)
				}

			} else {
				logger.Infof("skipping device %s until the admin specifies it can be used by an osd", device.Name)
			}
		}
	}

	return available, nil
}

func getDataDirs(context *clusterd.DaemonContext, useDataDir bool) (map[string]int, error) {
	filePath := path.Join(context.ConfigDir, dirOSDConfigFilename)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		// the config file doesn't exist yet, just return the empty map unless the data dir is desired
		if useDataDir {
			return map[string]int{context.ConfigDir: unassignedOSDID}, nil
		}

		return map[string]int{}, nil
	}

	b, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var dirs map[string]int
	err = json.Unmarshal(b, &dirs)
	if err != nil {
		return nil, err
	}
	return dirs, nil
}

func saveDirConfig(context *clusterd.DaemonContext, config map[string]int) error {
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
