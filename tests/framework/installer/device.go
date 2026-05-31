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

package installer

import (
	"strings"

	"github.com/rook/rook/pkg/util/exec"
	"github.com/rook/rook/pkg/util/sys"
)

// IsAdditionalDeviceAvailableOnCluster checks whether a given device is available to become an OSD
func IsAdditionalDeviceAvailableOnCluster() bool {
	executor := &exec.CommandExecutor{}
	devices, err := sys.ListDevices(executor)
	if err != nil {
		return false
	}
	disks := 0
	logger.Infof("devices : %v", devices)
	for _, device := range devices {
		if strings.Contains(device, "loop") {
			continue
		}
		props, _ := sys.GetDeviceProperties(device, executor)
		if props["TYPE"] != "disk" {
			continue
		}
		devicePath, ok := props["NAME"]
		if !ok {
			logger.Warningf("failed to find device path for %q", device)
			continue
		}

		pvcBackedOSD := false
		isAvailable, rejectedReason, err := sys.CheckIfDeviceAvailable(executor, devicePath, pvcBackedOSD)
		if err != nil {
			logger.Warningf("failed to detect device %q availability. %v", device, err)
			continue
		}
		if !isAvailable {
			logger.Infof("skipping device %q because %s", device, rejectedReason)
			continue
		}
		if strings.HasPrefix(device, "rbd") {
			logger.Infof("skipping unexpected rbd device %q", device)
			continue
		}
		logger.Infof("available device %q", device)
		disks++
	}
	if disks > 0 {
		return true
	}
	logger.Info("No additional disks found on cluster")
	return false
}
