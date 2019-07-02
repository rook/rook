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

// check whether a device is completely empty
func GetDeviceEmpty(device *sys.LocalDisk) bool {
	return device.Parent == "" && (device.Type == sys.DiskType || device.Type == sys.SSDType || device.Type == sys.CryptType || device.Type == sys.LVMType) && len(device.Partitions) == 0 && device.Filesystem == ""
}

func ignoreDevice(d string) bool {
	return isRBD.MatchString(d)
}

// Discover all the details of devices available on the local node
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
		disk := PopulateDeviceInfo(d, executor)
		if disk == nil {
			continue
		}
		disk = PopulateDeviceUdevInfo(d, executor, disk)
		disks = append(disks, disk)
	}

	return disks, nil
}

func PopulateDeviceInfo(d string, executor exec.Executor) *sys.LocalDisk {
	diskProps, err := sys.GetDeviceProperties(d, executor)
	if err != nil {
		logger.Warningf("skipping device %s: %+v", d, err)
		return nil
	}

	diskType, ok := diskProps["TYPE"]
	if !ok || (diskType != sys.SSDType && diskType != sys.CryptType && diskType != sys.DiskType && diskType != sys.PartType && diskType != sys.LinearType) {
		if !ok {
			logger.Warningf("skipping device %s: diskType is empty", d)
		} else {
			logger.Warningf("skipping device %s: unsupported diskType %+s", d, diskType)
		}
		// unsupported disk type, just continue
		return nil
	}

	// get the UUID for disks
	var diskUUID string
	if diskType != sys.PartType {
		diskUUID, err = sys.GetDiskUUID(d, executor)
		if err != nil {
			logger.Warningf("device %s has an unknown uuid. %+v", d, err)
			return nil
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

	return disk
}

func PopulateDeviceUdevInfo(d string, executor exec.Executor, disk *sys.LocalDisk) *sys.LocalDisk {
	udevInfo, err := sys.GetUdevInfo(d, executor)
	if err != nil {
		logger.Warningf("failed to get udev info for device %s: %+v", d, err)
		return disk
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

	return disk
}
