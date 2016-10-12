package inventory

import (
	"errors"
	"fmt"
	"log"
	"path"
	"strconv"
	"strings"

	etcd "github.com/coreos/etcd/client"
	"github.com/quantum/castle/pkg/util"
	"github.com/quantum/castle/pkg/util/exec"
	"github.com/quantum/castle/pkg/util/sys"
	ctx "golang.org/x/net/context"
)

const (
	DiskNameKey        = "name"
	DiskUUIDKey        = "uuid"
	DiskSizeKey        = "size"
	DiskRotationalKey  = "rotational"
	DiskReadonlyKey    = "readonly"
	DiskFileSystemKey  = "filesystem"
	DiskTypeKey        = "type"
	DiskParentKey      = "parent"
	DiskHasChildrenKey = "children"
	DiskMountPointKey  = "mountpoint"
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

func GetSerialFromDevice(device, nodeID string, etcdClient etcd.KeysAPI) (string, error) {
	disksKey := path.Join(GetNodeConfigKey(nodeID), DisksKey)
	serials, err := util.GetDirChildKeys(etcdClient, disksKey)
	if err != nil {
		return "", err
	}

	serialResult := ""
	for s := range serials.Iter() {
		resp, err := etcdClient.Get(ctx.Background(), path.Join(disksKey, s, DiskNameKey), nil)
		if err != nil || resp == nil || resp.Node == nil {
			continue
		}

		if resp.Node.Value == device {
			serialResult = s
			break
		}
	}

	if serialResult == "" {
		return "", fmt.Errorf("serial for device %s not found", device)
	}

	return serialResult, nil
}

func GetDiskInfo(diskInfo *etcd.Node) (*DiskConfig, error) {
	disk := &DiskConfig{}
	disk.Serial = util.GetLeafKeyPath(diskInfo.Key)

	// iterate over all properties of the disk
	for _, diskProperty := range diskInfo.Nodes {
		diskPropertyName := util.GetLeafKeyPath(diskProperty.Key)
		switch diskPropertyName {
		case DiskNameKey:
			disk.Name = diskProperty.Value
		case DiskUUIDKey:
			disk.UUID = diskProperty.Value
		case DiskSizeKey:
			size, err := strconv.ParseUint(diskProperty.Value, 10, 64)
			if err != nil {
				return nil, err
			} else {
				disk.Size = size
			}
		case DiskRotationalKey:
			rotational, err := strconv.ParseInt(diskProperty.Value, 10, 64)
			if err != nil {
				return nil, err
			} else {
				disk.Rotational = itob(rotational)
			}
		case DiskReadonlyKey:
			readonly, err := strconv.ParseInt(diskProperty.Value, 10, 64)
			if err != nil {
				return nil, err
			} else {
				disk.Readonly = itob(readonly)
			}
		case DiskFileSystemKey:
			disk.FileSystem = diskProperty.Value
		case DiskMountPointKey:
			disk.MountPoint = diskProperty.Value
		case DiskTypeKey:
			diskType, err := StrToDiskType(diskProperty.Value)
			if err != nil {
				return nil, err
			} else {
				disk.Type = diskType
			}
		case DiskParentKey:
			disk.Parent = diskProperty.Value
		case DiskHasChildrenKey:
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
	disksKey := path.Join(nodeConfigKey, DisksKey)

	cmd := "lsblk all"
	devices, err := executor.ExecuteCommandWithOutput(cmd, "lsblk", "--all", "-n", "-l", "--output", "KNAME")
	if err != nil {
		return fmt.Errorf("failed to list all devices: %+v", err)
	}

	for _, d := range strings.Split(devices, "\n") {
		cmd := fmt.Sprintf("lsblk /dev/%s", d)
		diskPropsRaw, err := executor.ExecuteCommandWithOutput(cmd, "lsblk", fmt.Sprintf("/dev/%s", d),
			"-b", "-d", "-P", "-o", "SERIAL,UUID,SIZE,ROTA,RO,TYPE,PKNAME")
		if err != nil {
			// try to get more information about the command error
			cmdErr, ok := err.(*exec.CommandError)
			if ok && cmdErr.ExitStatus() == 32 {
				// certain device types (such as loop) return exit status 32 when probed further,
				// ignore and continue without logging
				continue
			}

			log.Printf("failed to get properties of device %s: %+v", d, err)
			continue
		}

		diskPropMap := parseKeyValuePairString(diskPropsRaw)
		serial, ok := diskPropMap["SERIAL"]
		if !ok || serial == "" {
			// disk doesn't have a serial, just continue
			continue
		}

		diskType, ok := diskPropMap["TYPE"]
		if !ok || (diskType != "ssd" && diskType != "disk" && diskType != "part") {
			// unsupported disk type, just continue
			continue
		}

		fs, err := sys.GetDeviceFilesystem(d, executor)
		if err != nil {
			return err
		}

		mountPoint, err := sys.GetDeviceMountPoint(d, executor)
		if err != nil {
			return err
		}

		hasChildren, err := sys.DoesDeviceHaveChildren(d, executor)
		if err != nil {
			return err
		}

		dkey := path.Join(disksKey, serial)

		if _, err := etcdClient.Set(ctx.Background(), path.Join(dkey, DiskNameKey), d, nil); err != nil {
			return err
		}
		if err := setSimpleDiskProperty("UUID", DiskUUIDKey, dkey, diskPropMap, etcdClient); err != nil {
			return err
		}
		if err := setSimpleDiskProperty("SIZE", DiskSizeKey, dkey, diskPropMap, etcdClient); err != nil {
			return err
		}
		if err := setSimpleDiskProperty("ROTA", DiskRotationalKey, dkey, diskPropMap, etcdClient); err != nil {
			return err
		}
		if err := setSimpleDiskProperty("RO", DiskReadonlyKey, dkey, diskPropMap, etcdClient); err != nil {
			return err
		}
		if err := setSimpleDiskProperty("PKNAME", DiskParentKey, dkey, diskPropMap, etcdClient); err != nil {
			return err
		}
		if _, err := etcdClient.Set(ctx.Background(), path.Join(dkey, DiskHasChildrenKey), strconv.Itoa(btoi(hasChildren)), nil); err != nil {
			return err
		}
		if _, err := etcdClient.Set(ctx.Background(), path.Join(dkey, DiskFileSystemKey), fs, nil); err != nil {
			return err
		}
		if _, err := etcdClient.Set(ctx.Background(), path.Join(dkey, DiskMountPointKey), mountPoint, nil); err != nil {
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
func TestSetDiskInfo(etcdClient *util.MockEtcdClient, hardwareKey string, serial string, name string, uuid string, size uint64, rotational bool, readonly bool,
	filesystem string, mountPoint string, diskType DiskType, parent string, hasChildren bool) DiskConfig {

	disksKey := path.Join(hardwareKey, DisksKey)
	etcdClient.Set(ctx.Background(), disksKey, "", &etcd.SetOptions{Dir: true})

	diskKey := path.Join(disksKey, serial)
	etcdClient.Set(ctx.Background(), diskKey, "", &etcd.SetOptions{Dir: true})

	etcdClient.Set(ctx.Background(), path.Join(diskKey, DiskNameKey), name, nil)
	etcdClient.Set(ctx.Background(), path.Join(diskKey, DiskUUIDKey), uuid, nil)
	etcdClient.Set(ctx.Background(), path.Join(diskKey, DiskSizeKey), strconv.FormatUint(size, 10), nil)
	etcdClient.Set(ctx.Background(), path.Join(diskKey, DiskRotationalKey), strconv.Itoa(btoi(rotational)), nil)
	etcdClient.Set(ctx.Background(), path.Join(diskKey, DiskReadonlyKey), strconv.Itoa(btoi(readonly)), nil)
	etcdClient.Set(ctx.Background(), path.Join(diskKey, DiskFileSystemKey), filesystem, nil)
	etcdClient.Set(ctx.Background(), path.Join(diskKey, DiskMountPointKey), mountPoint, nil)
	etcdClient.Set(ctx.Background(), path.Join(diskKey, DiskTypeKey), DiskTypeToStr(diskType), nil)
	etcdClient.Set(ctx.Background(), path.Join(diskKey, DiskParentKey), parent, nil)
	etcdClient.Set(ctx.Background(), path.Join(diskKey, DiskHasChildrenKey), strconv.Itoa(btoi(hasChildren)), nil)

	return DiskConfig{
		Serial:      serial,
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
