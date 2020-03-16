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

package clusterd

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"

	"github.com/coreos/pkg/capnslog"

	"github.com/rook/rook/pkg/util/exec"
	"github.com/rook/rook/pkg/util/sys"
)

var (
	logger = capnslog.NewPackageLogger("github.com/rook/rook", "inventory")
	isRBD  = regexp.MustCompile("^rbd[0-9]+p?[0-9]{0,}$")
)

// GetDeviceEmpty check whether a device is completely empty
func GetDeviceEmpty(device *sys.LocalDisk) bool {
	return device.Parent == "" && (device.Type == sys.DiskType || device.Type == sys.SSDType || device.Type == sys.CryptType || device.Type == sys.LVMType) && len(device.Partitions) == 0 && device.Filesystem == ""
}

func ignoreDevice(d string) bool {
	return isRBD.MatchString(d)
}

// DiscoverDevices all the details of devices available on the local node
func DiscoverDevices(executor exec.Executor) ([]*sys.LocalDisk, error) {
	var disks []*sys.LocalDisk
	devices, err := sys.ListDevices(executor)
	if err != nil {
		return nil, err
	}

	for _, d := range devices {

		if ignoreDevice(d) {
			// skip device
			continue
		}
		disk, err := PopulateDeviceInfo(d, executor)
		if err != nil {
			// skip device
			logger.Warningf("skipping device %s: %+v", d, err)
			continue
		}
		disk, err = PopulateDeviceUdevInfo(d, executor, disk)
		if err != nil {
			// go on without udev info
			logger.Warningf("failed to get udev info for device %s: %+v", d, err)
		}

		// Test if device has child, if so we skip it and only consider the partitions
		// which will come in later iterations of the loop
		// We only test if the type is 'disk', this is a property reported by lsblk
		// and means it's a parent block device
		if disk.Type == sys.DiskType {
			deviceChild, err := sys.ListDevicesChild(executor, d)
			if err != nil {
				logger.Warningf("failed to detect child devices for device %q, assuming they are none. %+v", d, err)
			}
			// lsblk will output at least 2 lines if they are partitions, one for the parent
			// and N for the child
			if len(deviceChild) > 1 {
				logger.Infof("skipping device %q because it has child, considering the child instead.", d)
				continue
			}
		}

		disks = append(disks, disk)
	}

	return disks, nil
}

// PopulateDeviceInfo returns the information of the specified block device
func PopulateDeviceInfo(d string, executor exec.Executor) (*sys.LocalDisk, error) {
	diskProps, err := sys.GetDeviceProperties(d, executor)
	if err != nil {
		return nil, err
	}

	diskType, ok := diskProps["TYPE"]
	if !ok || (diskType != sys.SSDType && diskType != sys.CryptType && diskType != sys.DiskType && diskType != sys.PartType && diskType != sys.LinearType && diskType != sys.LVMType) {
		if !ok {
			return nil, errors.New("diskType is empty")
		} else {
			return nil, fmt.Errorf("unsupported diskType %+s", diskType)
		}
	}

	// get the UUID for disks
	var diskUUID string
	if diskType != sys.PartType {
		diskUUID, err = sys.GetDiskUUID(d, executor)
		if err != nil {
			return nil, err
		}
	}

	disk := &sys.LocalDisk{Name: d, UUID: diskUUID}

	if val, ok := diskProps["TYPE"]; ok {
		disk.Type = val
	}
	if val, ok := diskProps["SIZE"]; ok {
		if size, err := strconv.ParseUint(val, 10, 64); err == nil {
			disk.Size = size
		}
	}
	if val, ok := diskProps["ROTA"]; ok {
		if rotates, err := strconv.ParseBool(val); err == nil {
			disk.Rotational = rotates
		}
	}
	if val, ok := diskProps["RO"]; ok {
		if ro, err := strconv.ParseBool(val); err == nil {
			disk.Readonly = ro
		}
	}
	if val, ok := diskProps["PKNAME"]; ok {
		disk.Parent = val
	}

	return disk, nil
}

// PopulateDeviceUdevInfo fills the udev info into the block device information
func PopulateDeviceUdevInfo(d string, executor exec.Executor, disk *sys.LocalDisk) (*sys.LocalDisk, error) {
	udevInfo, err := sys.GetUdevInfo(d, executor)
	if err != nil {
		return disk, err
	}
	// parse udev info output
	if val, ok := udevInfo["DEVLINKS"]; ok {
		disk.DevLinks = val
	}
	if val, ok := udevInfo["ID_FS_TYPE"]; ok {
		disk.Filesystem = val
	}
	if val, ok := udevInfo["ID_SERIAL"]; ok {
		disk.Serial = val
	}

	if val, ok := udevInfo["ID_VENDOR"]; ok {
		disk.Vendor = val
	}

	if val, ok := udevInfo["ID_MODEL"]; ok {
		disk.Model = val
	}

	if val, ok := udevInfo["ID_WWN_WITH_EXTENSION"]; ok {
		disk.WWNVendorExtension = val
	}

	if val, ok := udevInfo["ID_WWN"]; ok {
		disk.WWN = val
	}

	return disk, nil
}
