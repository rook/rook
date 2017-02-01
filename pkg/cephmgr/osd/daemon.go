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
	"log"
	"time"

	"github.com/rook/rook/pkg/cephmgr/mon"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/clusterd/inventory"
	"github.com/rook/rook/pkg/util/sys"
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
	devices, err := getAvailableDevices(dcontext, hardware.Disks)
	if err != nil {
		return fmt.Errorf("failed to get available devices. %+v", err)
	}

	logger.Infof("configuring osd devices: %+v", devices)
	err = agent.configureDevices(context, devices)
	if err != nil {
		return fmt.Errorf("failed to configure devices. %+v", err)
	}

	if len(devices.Entries) == 0 {
		dirs := map[string]int{context.ConfigDir: -1}
		logger.Infof("configuring osd dir: %+v", dirs)
		err = agent.configureDirs(context, dirs)
		if err != nil {
			return fmt.Errorf("failed to configure dir %s. %+v", context.ConfigDir, err)
		}
	}

	// FIX
	log.Printf("sleeping a while to let the osds run...")
	<-time.After(1000000 * time.Second)
	return nil
}

func getAvailableDevices(context *clusterd.DaemonContext, devices []*inventory.LocalDisk) (*DeviceOsdMapping, error) {
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
			available.Entries[device.Name] = &DeviceOsdIDEntry{Data: unassignedOSDID}
		}
	}

	return available, nil
}
