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
