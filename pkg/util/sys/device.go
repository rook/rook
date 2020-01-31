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
package sys

import (
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/rook/rook/pkg/util/exec"
)

const (
	DiskType     = "disk"
	SSDType      = "ssd"
	PartType     = "part"
	CryptType    = "crypt"
	LVMType      = "lvm"
	LinearType   = "linear"
	sgdisk       = "sgdisk"
	mountCmd     = "mount"
	cephLVPrefix = "ceph--"
)

type Partition struct {
	Name       string
	Size       uint64
	Label      string
	Filesystem string
}

// LocalDevice contains information about an unformatted block device
type LocalDisk struct {
	// Name is the device name
	Name string `json:"name"`
	// Parent is the device parent's name
	Parent string `json:"parent"`
	// HasChildren is whether the device has a children device
	HasChildren bool `json:"hasChildren"`
	// DevLinks is the persistent device path on the host
	DevLinks string `json:"devLinks"`
	// Size is the device capacity in byte
	Size uint64 `json:"size"`
	// UUID is used by /dev/disk/by-uuid
	UUID string `json:"uuid"`
	// Serial is the disk serial used by /dev/disk/by-id
	Serial string `json:"serial"`
	// Type is disk type
	Type string `json:"type"`
	// Rotational is the boolean whether the device is rotational: true for hdd, false for ssd and nvme
	Rotational bool `json:"rotational"`
	// ReadOnly is the boolean whether the device is readonly
	Readonly bool `json:"readOnly"`
	// Partitions is a partition slice
	Partitions []Partition
	// Filesystem is the filesystem currently on the device
	Filesystem string `json:"filesystem"`
	// Vendor is the device vendor
	Vendor string `json:"vendor"`
	// Model is the device model
	Model string `json:"model"`
	// WWN is the world wide name of the device
	WWN string `json:"wwn"`
	// WWNVendorExtension is the WWN_VENDOR_EXTENSION from udev info
	WWNVendorExtension string `json:"wwnVendorExtension"`
	// Empty checks whether the device is completely empty
	Empty bool `json:"empty"`
	// Information provided by Ceph Volume Inventory
	CephVolumeData string `json:"cephVolumeData,omitempty"`
}

func ListDevices(executor exec.Executor) ([]string, error) {
	cmd := "lsblk all"
	devices, err := executor.ExecuteCommandWithOutput(false, cmd, "lsblk", "--all", "--noheadings", "--list", "--output", "KNAME")
	if err != nil {
		return nil, fmt.Errorf("failed to list all devices: %+v", err)
	}

	return strings.Split(devices, "\n"), nil
}

func GetDevicePartitions(device string, executor exec.Executor) (partitions []Partition, unusedSpace uint64, err error) {

	var devicePath string
	splitDevicePath := strings.Split(device, "/")
	if len(splitDevicePath) == 1 {
		devicePath = fmt.Sprintf("/dev/%s", device) //device path for OSD on devices.
	} else {
		devicePath = device //use the exact device path (like /mnt/<pvc-name>) in case of PVC block device
	}

	cmd := fmt.Sprintf("lsblk %s", devicePath)
	output, err := executor.ExecuteCommandWithOutput(false, cmd, "lsblk", devicePath,
		"--bytes", "--pairs", "--output", "NAME,SIZE,TYPE,PKNAME")
	logger.Infof("Output: %+v", output)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get device %s partitions. %+v", device, err)
	}
	partInfo := strings.Split(output, "\n")
	var deviceSize uint64
	var totalPartitionSize uint64
	for _, info := range partInfo {
		props := parseKeyValuePairString(info)
		name := props["NAME"]
		if name == device {
			// found the main device
			logger.Info("Device found - ", name)
			deviceSize, err = strconv.ParseUint(props["SIZE"], 10, 64)
			if err != nil {
				return nil, 0, fmt.Errorf("failed to get device %s size. %+v", device, err)
			}
		} else if props["PKNAME"] == device && props["TYPE"] == PartType {
			// found a partition
			p := Partition{Name: name}
			p.Size, err = strconv.ParseUint(props["SIZE"], 10, 64)
			if err != nil {
				return nil, 0, fmt.Errorf("failed to get partition %s size. %+v", name, err)
			}
			totalPartitionSize += p.Size

			info, err := GetUdevInfo(name, executor)
			if err != nil {
				return nil, 0, err
			}
			if v, ok := info["PARTNAME"]; ok {
				p.Label = v
			}
			if v, ok := info["ID_PART_ENTRY_NAME"]; ok {
				p.Label = v
			}
			if v, ok := info["ID_FS_TYPE"]; ok {
				p.Filesystem = v
			}

			partitions = append(partitions, p)
		} else if strings.HasPrefix(name, cephLVPrefix) && props["TYPE"] == LVMType {
			p := Partition{Name: name}
			partitions = append(partitions, p)
		}
	}

	if deviceSize > 0 {
		unusedSpace = deviceSize - totalPartitionSize
	}
	return partitions, unusedSpace, nil
}

func GetDeviceProperties(device string, executor exec.Executor) (map[string]string, error) {
	// As we are mounting the block mode PVs on /mnt we use the entire path,
	// e.g., if the device path is /mnt/example-pvc then its taken completely
	// else if its just vdb then the following is used
	devicePath := strings.Split(device, "/")
	if len(devicePath) == 1 {
		device = fmt.Sprintf("/dev/%s", device)
	}
	return GetDevicePropertiesFromPath(device, executor)
}

func GetDevicePropertiesFromPath(devicePath string, executor exec.Executor) (map[string]string, error) {
	cmd := fmt.Sprintf("lsblk %s", devicePath)
	output, err := executor.ExecuteCommandWithOutput(false, cmd, "lsblk", devicePath,
		"--bytes", "--nodeps", "--pairs", "--output", "SIZE,ROTA,RO,TYPE,PKNAME")
	if err != nil {
		// try to get more information about the command error
		cmdErr, ok := err.(*exec.CommandError)
		if ok && cmdErr.ExitStatus() == 32 {
			// certain device types (such as loop) return exit status 32 when probed further,
			// ignore and continue without logging
			return map[string]string{}, nil
		}

		return nil, err
	}

	return parseKeyValuePairString(output), nil
}

func IsLV(devicePath string, executor exec.Executor) (bool, error) {
	devProps, err := GetDevicePropertiesFromPath(devicePath, executor)
	if err != nil {
		return false, fmt.Errorf("failed to get device properties for %q: %+v", devicePath, err)
	}
	diskType, ok := devProps["TYPE"]
	if !ok {
		return false, fmt.Errorf("TYPE property is not found for %q", devicePath)
	}
	return diskType == LVMType, nil
}

func GetUdevInfo(device string, executor exec.Executor) (map[string]string, error) {
	cmd := fmt.Sprintf("udevadm info %s", device)
	output, err := executor.ExecuteCommandWithOutput(false, cmd, "udevadm", "info", "--query=property", fmt.Sprintf("/dev/%s", device))
	if err != nil {
		return nil, err
	}

	return parseUdevInfo(output), nil
}

// get the file systems available
func GetDeviceFilesystems(device string, executor exec.Executor) (string, error) {
	devicePath := strings.Split(device, "/")
	if len(devicePath) == 1 {
		device = fmt.Sprintf("/dev/%s", device)
	}
	cmd := fmt.Sprintf("get filesystem type for %s", device)
	output, err := executor.ExecuteCommandWithOutput(false, cmd, "udevadm", "info", "--query=property", device)
	if err != nil {
		return "", fmt.Errorf("command %s failed: %+v", cmd, err)
	}

	return parseFS(output), nil
}

func RemovePartitions(device string, executor exec.Executor) error {
	cmd := fmt.Sprintf("zap %s", device)
	err := executor.ExecuteCommand(false, cmd, sgdisk, "--zap-all", "/dev/"+device)
	if err != nil {
		return fmt.Errorf("failed to zap partitions on /dev/%s: %+v", device, err)
	}

	cmd = fmt.Sprintf("clear %s", device)
	err = executor.ExecuteCommand(false, cmd, sgdisk, "--clear", "--mbrtogpt", "/dev/"+device)
	if err != nil {
		return fmt.Errorf("failed to clear partitions on /dev/%s: %+v", device, err)
	}

	return nil
}

func CreatePartitions(device string, args []string, executor exec.Executor) error {
	cmd := fmt.Sprintf("partition %s", device)
	return executor.ExecuteCommand(false, cmd, sgdisk, args...)
}

func FormatDevice(devicePath string, executor exec.Executor) error {
	cmd := fmt.Sprintf("mkfs.ext4 %s", devicePath)
	if err := executor.ExecuteCommand(false, cmd, "mkfs.ext4", devicePath); err != nil {
		return fmt.Errorf("command %s failed: %+v", cmd, err)
	}

	return nil
}

// look up the UUID for a disk.
func GetDiskUUID(device string, executor exec.Executor) (string, error) {
	if _, err := os.Stat("/usr/sbin/sgdisk"); err != nil {
		logger.Warningf("sgdisk not found. skipping disk UUID.")
		return "sgdiskNotFound", nil
	}

	devicePath := strings.Split(device, "/")
	if len(devicePath) == 1 {
		device = fmt.Sprintf("/dev/%s", device)
	}

	cmd := fmt.Sprintf("get disk %s uuid", device)

	output, err := executor.ExecuteCommandWithOutput(false, cmd,
		sgdisk, "--print", device)
	if err != nil {
		return "", err
	}

	return parseUUID(device, output)
}

func GetPartitionLabel(deviceName string, executor exec.Executor) (string, error) {
	// look up the partition's label with blkid because lsblk relies on udev which is
	// not available in containers
	devicePath := fmt.Sprintf("/dev/%s", deviceName)
	cmd := fmt.Sprintf("udevadm %s", devicePath)
	output, err := executor.ExecuteCommandWithOutput(false, cmd,
		"udevadm", "info", "--query=property", devicePath)
	if err != nil {
		return "", fmt.Errorf("failed to get partition label for device %s: %+v", deviceName, err)
	}

	return parsePartLabel(output), nil
}

func MountDevice(devicePath, mountPath string, executor exec.Executor) error {
	return MountDeviceWithOptions(devicePath, mountPath, "", "", executor)
}

// comma-separated list of mount options passed directly to mount command
func MountDeviceWithOptions(devicePath, mountPath, fstype, options string, executor exec.Executor) error {
	args := []string{}

	if fstype != "" {
		args = append(args, "-t", fstype)
	}

	if options != "" {
		args = append(args, "-o", options)
	}

	// device path and mount path are always the last 2 args
	args = append(args, devicePath, mountPath)

	os.MkdirAll(mountPath, 0755)
	cmd := fmt.Sprintf("mount %s", devicePath)
	if err := executor.ExecuteCommand(false, cmd, mountCmd, args...); err != nil {
		return fmt.Errorf("command %s failed: %+v", cmd, err)
	}

	return nil
}

func UnmountDevice(devicePath string, executor exec.Executor) error {
	cmd := fmt.Sprintf("umount %s", devicePath)
	if err := executor.ExecuteCommand(false, cmd, "umount", devicePath); err != nil {
		cmdErr, ok := err.(*exec.CommandError)
		if ok && cmdErr.ExitStatus() == 32 {
			logger.Infof("ignoring exit status 32 from unmount of device %s, err:%+v", devicePath, cmdErr)
		} else {
			return fmt.Errorf("command %s failed: %+v", cmd, err)
		}
	}

	return nil
}

// CheckIfDeviceAvailable checks if a device is available for consumption. The caller
// needs to decide based on the return values whether it is available. The return values are
// the number of partitions, whether Rook has created partitions on the device in the past
// possibly from the same or a previous cluster, the filesystem found, or an err if failed
// to retrieve the properties.
func CheckIfDeviceAvailable(executor exec.Executor, name string, pvcBacked bool) (int, bool, string, error) {
	ownPartitions := true
	partitions, _, err := GetDevicePartitions(name, executor)
	if err != nil {
		return 0, false, "", fmt.Errorf("failed to get %s partitions. %+v", name, err)
	}
	partCount := len(partitions)
	if !RookOwnsPartitions(partitions) {
		ownPartitions = false
	}

	var devFS string
	if !pvcBacked {
		// check if there is a file system on the device
		devFS, err = GetDeviceFilesystems(name, executor)
		if err != nil {
			return 0, false, "", fmt.Errorf("failed to get device %s filesystem: %+v", name, err)
		}
	} else {
		devFS, err = GetPVCDeviceFileSystems(executor, name)
		if err != nil {
			return 0, false, "", fmt.Errorf("failed to get pvc device %q filesystem. %+v", name, err)
		}
	}
	return partCount, ownPartitions, devFS, nil
}

//GetPVCDeviceFileSystems returns the file system on a PVC device.
func GetPVCDeviceFileSystems(executor exec.Executor, device string) (string, error) {
	cmd := fmt.Sprintf("get pvc filesystem type for %q", device)
	output, err := executor.ExecuteCommandWithOutput(false, cmd, "lsblk", device, "--bytes", "--nodeps", "--noheadings", "--output", "FSTYPE")
	if err != nil {
		return "", fmt.Errorf("command %q failed. %+v", cmd, err)
	}
	logger.Debugf("filesystem on pvc device %q is %q", device, output)

	return output, nil
}

// RookOwnsPartitions check if all partitions in list are owned by Rook
func RookOwnsPartitions(partitions []Partition) bool {
	// if there are partitions, they must all have the rook osd label
	ownPartitions := true
	for _, p := range partitions {
		if strings.HasPrefix(p.Label, "ROOK-OSD") {
			logger.Infof("rook partition: %s", p.Label)
		} else {
			logger.Infof("non-rook partition: %s", p.Label)
			ownPartitions = false
		}
	}

	// if there are no partitions, or the partitions are all from rook OSDs, then rook owns the device
	return ownPartitions
}

// finds the disk uuid in the output of sgdisk
func parseUUID(device, output string) (string, error) {

	// find the line with the uuid
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "Disk identifier (GUID)") {
			words := strings.Split(line, " ")
			for _, word := range words {
				// we expect most words in the line not to be a uuid, but will return the first one that is
				result, err := uuid.Parse(word)
				if err == nil {
					return result.String(), nil
				}
			}
		}
	}

	return "", fmt.Errorf("uuid not found for device %s. output=%s", device, output)
}

// converts a raw key value pair string into a map of key value pairs
// example raw string of `foo="0" bar="1" baz="biz"` is returned as:
// map[string]string{"foo":"0", "bar":"1", "baz":"biz"}
func parseKeyValuePairString(propsRaw string) map[string]string {
	// first split the single raw string on spaces and initialize a map of
	// a length equal to the number of pairs
	props := strings.Split(propsRaw, " ")
	propMap := make(map[string]string, len(props))

	for _, kvpRaw := range props {
		// split each individual key value pair on the equals sign
		kvp := strings.Split(kvpRaw, "=")
		if len(kvp) == 2 {
			// first element is the final key, second element is the final value
			// (don't forget to remove surrounding quotes from the value)
			propMap[kvp[0]] = strings.Replace(kvp[1], `"`, "", -1)
		}
	}

	return propMap
}

// find fs from udevadm info
func parseFS(output string) string {
	m := parseUdevInfo(output)
	if v, ok := m["ID_FS_TYPE"]; ok {
		return v
	}
	return ""
}

// find fs from udevadm info
func parseFSUUID(output string) string {
	m := parseUdevInfo(output)
	if v, ok := m["ID_FS_UUID"]; ok {
		return v
	}
	return ""
}

// find partition label from udevadm info
func parsePartLabel(output string) string {
	m := parseUdevInfo(output)
	if v, ok := m["ID_PART_ENTRY_NAME"]; ok {
		return v
	}
	if v, ok := m["PARTNAME"]; ok {
		return v
	}
	return ""
}

func parseUdevInfo(output string) map[string]string {
	lines := strings.Split(output, "\n")
	result := make(map[string]string, len(lines))
	for _, v := range lines {
		pairs := strings.Split(v, "=")
		if len(pairs) > 1 {
			result[pairs[0]] = pairs[1]
		}
	}
	return result
}

// ListDevicesChild list all child available on a device
func ListDevicesChild(executor exec.Executor, device string) ([]string, error) {
	cmd := "lsblk for child"

	childListRaw, err := executor.ExecuteCommandWithOutput(false, cmd, "lsblk", "--noheadings", "--pairs", path.Join("/dev", device))
	if err != nil {
		return []string{}, fmt.Errorf("failed to list child devices of %q. %+v", device, err)
	}

	return strings.Split(childListRaw, "\n"), nil
}
