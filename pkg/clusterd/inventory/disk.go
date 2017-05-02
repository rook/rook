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
package inventory

import (
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strconv"

	etcd "github.com/coreos/etcd/client"
	ctx "golang.org/x/net/context"

	"github.com/rook/rook/pkg/util/exec"
	"github.com/rook/rook/pkg/util/sys"
)

var (
	isRBD = regexp.MustCompile("^rbd[0-9]+p?[0-9]{0,}$")
)

func GetAvailableDevices(devices []*LocalDisk) []string {

	var available []string
	for _, device := range devices {
		logger.Debugf("Evaluating device %+v", device)
		if getDeviceEmpty(device) {
			logger.Debugf("Available device: %s", device.Name)
			available = append(available, device.Name)
		}
	}

	return available
}

func storeDevices(etcdClient etcd.KeysAPI, nodeID string, devices []*LocalDisk) error {
	// store the basic device info in etcd
	disks := toClusterDisks(devices)
	output, err := json.Marshal(disks)
	if err != nil {
		return fmt.Errorf("failed to marshal disks. %+v", err)
	}

	key := path.Join(NodesConfigKey, nodeID, disksKey)
	_, err = etcdClient.Set(ctx.Background(), key, string(output), nil)
	if err != nil {
		return fmt.Errorf("failed to store disks in etcd. %+v", err)
	}

	return nil
}

func loadDisksConfig(nodeConfig *NodeConfig, rawDisks string) error {
	var disks []*Disk
	if err := json.Unmarshal([]byte(rawDisks), &disks); err != nil {
		return fmt.Errorf("failed to deserialize disks. %+v", err)
	}
	nodeConfig.Disks = disks
	return nil
}

// Extract the basic disk info that will be used in cluster-wide orchestration decisions.
func toClusterDisks(devices []*LocalDisk) []*Disk {
	var disks []*Disk
	for _, device := range devices {
		if device.Type == sys.DiskType || device.Type == sys.SSDType {
			disk := &Disk{
				Type:       device.Type,
				Size:       device.Size,
				Rotational: device.Rotational,
				Empty:      device.Empty,
			}
			disks = append(disks, disk)
		}
	}

	return disks
}

// check whether a device is completely empty
func getDeviceEmpty(device *LocalDisk) bool {
	return device.Parent == "" && device.Type == sys.DiskType && device.FileSystem == ""
}

func ignoreDevice(d string) bool {
	return isRBD.MatchString(d)
}

// Discover all the details of devices available on the local node
func discoverDevices(executor exec.Executor) ([]*LocalDisk, error) {

	var disks []*LocalDisk
	devices, err := sys.ListDevices(executor)
	if err != nil {
		return nil, err
	}

	for _, d := range devices {

		if ignoreDevice(d) {
			// skip device
			continue
		}

		diskProps, err := sys.GetDeviceProperties(d, executor)
		if err != nil {
			logger.Warningf("skipping device %s: %+v", d, err)
			continue
		}

		diskType, ok := diskProps["TYPE"]
		if !ok || (diskType != sys.SSDType && diskType != sys.DiskType && diskType != sys.PartType) {
			// unsupported disk type, just continue
			continue
		}

		// get the UUID for disks
		var diskUUID string
		if diskType != sys.PartType {
			diskUUID, err = sys.GetDiskUUID(d, executor)
			if err != nil {
				logger.Warningf("skipping device %s with an unknown uuid. %+v", d, err)
				continue
			}
		}

		fs, err := sys.GetDeviceFilesystems(d, executor)
		if err != nil {
			return nil, err
		}

		disk := &LocalDisk{Name: d, UUID: diskUUID, FileSystem: fs}

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

		disk.Empty = getDeviceEmpty(disk)

		disks = append(disks, disk)
	}

	return disks, nil
}
