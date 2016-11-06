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
	"errors"
	"fmt"
	"log"
	"path"
	"strconv"
	"strings"

	etcd "github.com/coreos/etcd/client"
	"github.com/rook/rook/pkg/util"
	"github.com/rook/rook/pkg/util/exec"
	"github.com/rook/rook/pkg/util/sys"
	ctx "golang.org/x/net/context"
)

const (
	diskUUIDKey        = "uuid"
	diskSizeKey        = "size"
	diskRotationalKey  = "rotational"
	diskReadonlyKey    = "readonly"
	diskFileSystemKey  = "filesystem"
	diskTypeKey        = "type"
	diskParentKey      = "parent"
	diskHasChildrenKey = "children"
	diskMountPointKey  = "mountpoint"
)

func DiskTypeToStr(diskType DiskType) string {
	switch diskType {
	case Disk:
		return "disk"
	case Part:
		return "part"
	default:
		return "unknown"
	}
}

func StrToDiskType(diskType string) (DiskType, error) {
	diskType = strings.ToLower(diskType)
	switch diskType {
	case "disk":
		return Disk, nil
	case "part":
		return Part, nil
	default:
		return -1, errors.New(fmt.Sprintf("unknown disk type: %s", diskType))
	}
}

func GetDeviceSize(name, nodeID string, etcdClient etcd.KeysAPI) (uint64, error) {
	key := path.Join(GetNodeConfigKey(nodeID), disksKey, name, diskSizeKey)
	resp, err := etcdClient.Get(ctx.Background(), key, nil)
	if err != nil {
		return 0, err
	}

	size, err := strconv.ParseUint(resp.Node.Value, 10, 64)
	if err != nil {
		return 0, err
	}

	return size, nil
}

func GetDeviceFromUUID(uuid, nodeID string, etcdClient etcd.KeysAPI) (string, error) {
	key := path.Join(GetNodeConfigKey(nodeID), disksKey)
	devices, err := util.GetDirChildKeys(etcdClient, key)
	if err != nil {
		return "", err
	}

	for d := range devices.Iter() {
		resp, err := etcdClient.Get(ctx.Background(), path.Join(key, d, diskUUIDKey), nil)
		if err != nil || resp == nil || resp.Node == nil {
			continue
		}

		if resp.Node.Value == uuid {
			return d, nil
		}
	}

	return "", fmt.Errorf("device for uuid %s not found", uuid)
}

func SetDeviceUUID(nodeID, device, uuid string, etcdClient etcd.KeysAPI) error {
	key := path.Join(GetNodeConfigKey(nodeID), disksKey, device, diskUUIDKey)
	_, err := etcdClient.Set(ctx.Background(), key, uuid, nil)
	return err
}

func GetDeviceUUID(device, nodeID string, etcdClient etcd.KeysAPI) (string, error) {
	key := path.Join(GetNodeConfigKey(nodeID), disksKey, device, diskUUIDKey)
	resp, err := etcdClient.Get(ctx.Background(), key, nil)
	if err != nil {
		return "", err
	}

	return resp.Node.Value, nil
}

func getDiskInfo(diskInfo *etcd.Node) (*DiskConfig, error) {
	disk := &DiskConfig{}
	disk.Name = util.GetLeafKeyPath(diskInfo.Key)

	// iterate over all properties of the disk
	for _, diskProperty := range diskInfo.Nodes {
		diskPropertyName := util.GetLeafKeyPath(diskProperty.Key)
		switch diskPropertyName {
		case diskUUIDKey:
			disk.UUID = diskProperty.Value
		case diskSizeKey:
			size, err := strconv.ParseUint(diskProperty.Value, 10, 64)
			if err != nil {
				return nil, err
			} else {
				disk.Size = size
			}
		case diskRotationalKey:
			rotational, err := strconv.ParseInt(diskProperty.Value, 10, 64)
			if err != nil {
				return nil, err
			} else {
				disk.Rotational = itob(rotational)
			}
		case diskReadonlyKey:
			readonly, err := strconv.ParseInt(diskProperty.Value, 10, 64)
			if err != nil {
				return nil, err
			} else {
				disk.Readonly = itob(readonly)
			}
		case diskFileSystemKey:
			disk.FileSystem = diskProperty.Value
		case diskMountPointKey:
			disk.MountPoint = diskProperty.Value
		case diskTypeKey:
			diskType, err := StrToDiskType(diskProperty.Value)
			if err != nil {
				return nil, err
			} else {
				disk.Type = diskType
			}
		case diskParentKey:
			disk.Parent = diskProperty.Value
		case diskHasChildrenKey:
			hasChildren, err := strconv.ParseInt(diskProperty.Value, 10, 64)
			if err != nil {
				return nil, err
			} else {
				disk.HasChildren = itob(hasChildren)
			}
		default:
			log.Printf("unknown disk property key %s, skipping...", diskPropertyName)
		}
	}

	return disk, nil
}

func btoi(b bool) int {
	if b {
		return 1
	} else {
		return 0
	}
}

func itob(i int64) bool {
	if i == 0 {
		return false
	} else {
		return true
	}
}

func discoverDisks(nodeConfigKey string, etcdClient etcd.KeysAPI, executor exec.Executor) error {

	devices, err := sys.ListDevices(executor)
	if err != nil {
		return err
	}

	for _, d := range devices {

		diskProps, err := sys.GetDeviceProperties(d, executor)
		if err != nil {
			log.Printf("skipping device %s: %+v", d, err)
			continue
		}

		diskType, ok := diskProps["TYPE"]
		if !ok || (diskType != "ssd" && diskType != "disk" && diskType != "part") {
			// unsupported disk type, just continue
			continue
		}

		// get the UUID for disks
		var diskUUID string
		if diskType != "part" {
			diskUUID, err = sys.GetDiskUUID(d, executor)
			if err != nil {
				log.Printf("skipping device %s with an unknown uuid. %+v", d, err)
				continue
			}
		}

		fs, err := sys.GetDeviceFilesystems(d, executor)
		if err != nil {
			return err
		}

		dkey := path.Join(nodeConfigKey, disksKey, d)

		if _, err := etcdClient.Set(ctx.Background(), path.Join(dkey, diskUUIDKey), diskUUID, nil); err != nil {
			return err
		}
		if err := setSimpleDiskProperty("SIZE", diskSizeKey, dkey, diskProps, etcdClient); err != nil {
			return err
		}
		if err := setSimpleDiskProperty("ROTA", diskRotationalKey, dkey, diskProps, etcdClient); err != nil {
			return err
		}
		if err := setSimpleDiskProperty("RO", diskReadonlyKey, dkey, diskProps, etcdClient); err != nil {
			return err
		}
		if err := setSimpleDiskProperty("PKNAME", diskParentKey, dkey, diskProps, etcdClient); err != nil {
			return err
		}
		if _, err := etcdClient.Set(ctx.Background(), path.Join(dkey, diskFileSystemKey), fs, nil); err != nil {
			return err
		}
	}

	return nil
}

func setSimpleDiskProperty(propName, keyName, diskKey string, diskPropMap map[string]string, etcdClient etcd.KeysAPI) error {
	val, ok := diskPropMap[propName]
	if !ok {
		return fmt.Errorf("disk property %s not found in map: %+v", propName, diskPropMap)
	}
	if _, err := etcdClient.Set(ctx.Background(), path.Join(diskKey, keyName), val, nil); err != nil {
		return err
	}

	return nil
}

// test usage only
func TestSetDiskInfo(etcdClient *util.MockEtcdClient, hardwareKey string, name string, uuid string, size uint64, rotational bool, readonly bool,
	filesystem string, mountPoint string, diskType DiskType, parent string, hasChildren bool) DiskConfig {

	diskKey := path.Join(hardwareKey, disksKey, name)
	etcdClient.Set(ctx.Background(), path.Join(diskKey, diskUUIDKey), uuid, nil)
	etcdClient.Set(ctx.Background(), path.Join(diskKey, diskSizeKey), strconv.FormatUint(size, 10), nil)
	etcdClient.Set(ctx.Background(), path.Join(diskKey, diskRotationalKey), strconv.Itoa(btoi(rotational)), nil)
	etcdClient.Set(ctx.Background(), path.Join(diskKey, diskReadonlyKey), strconv.Itoa(btoi(readonly)), nil)
	etcdClient.Set(ctx.Background(), path.Join(diskKey, diskFileSystemKey), filesystem, nil)
	etcdClient.Set(ctx.Background(), path.Join(diskKey, diskMountPointKey), mountPoint, nil)
	etcdClient.Set(ctx.Background(), path.Join(diskKey, diskTypeKey), DiskTypeToStr(diskType), nil)
	etcdClient.Set(ctx.Background(), path.Join(diskKey, diskParentKey), parent, nil)
	etcdClient.Set(ctx.Background(), path.Join(diskKey, diskHasChildrenKey), strconv.Itoa(btoi(hasChildren)), nil)

	return DiskConfig{
		Name:        name,
		UUID:        uuid,
		Size:        size,
		Rotational:  rotational,
		Readonly:    readonly,
		FileSystem:  filesystem,
		MountPoint:  mountPoint,
		Type:        diskType,
		Parent:      parent,
		HasChildren: hasChildren,
	}
}
