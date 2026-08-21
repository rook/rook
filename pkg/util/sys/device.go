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
	"encoding/json"
	"fmt"
	"os"
	osexec "os/exec"
	"strconv"
	"strings"

	"github.com/pkg/errors"

	"github.com/google/uuid"
	"github.com/rook/rook/pkg/util/exec"
)

const (
	// DiskType is a disk type
	DiskType = "disk"
	// SSDType is an ssd type
	SSDType = "ssd"
	// PartType is a partition type
	PartType = "part"
	// CryptType is an encrypted type
	CryptType = "crypt"
	// LVMType is an LVM type
	LVMType = "lvm"
	// MultiPath is for multipath devices
	MultiPath = "mpath"
	// LinearType is a linear type
	LinearType = "linear"
	// LoopType is a loop device type
	LoopType  = "loop"
	sgdiskCmd = "sgdisk"
	// CephLVPrefix is the prefix of an LV owned by ceph-volume
	CephLVPrefix = "ceph--"
)

// CephVolumeInventory represents the output of the ceph-volume inventory command
type CephVolumeInventory struct {
	Path            string          `json:"path"`
	Available       bool            `json:"available"`
	RejectedReasons json.RawMessage `json:"rejected_reasons"`
	SysAPI          json.RawMessage `json:"sys_api"`
	LVS             json.RawMessage `json:"lvs"`
}

// CephVolumeLVMList represents the output of the ceph-volume lvm list command
type CephVolumeLVMList map[string][]map[string]interface{}

// Partition represents partition metadata
type Partition struct {
	Name       string
	Size       uint64
	Label      string
	Filesystem string
}

// LocalDisk contains information about an unformatted block device
type LocalDisk struct {
	// Name is the device name
	Name string `json:"name"`
	// Parent is the device parent's name
	Parent string `json:"parent"`
	// HasChildren is whether the device has a child device
	HasChildren bool `json:"hasChildren"`
	// DevLinks is the persistent device path on the host
	DevLinks string `json:"devLinks"`
	// Size is the device capacity in bytes
	Size uint64 `json:"size"`
	// UUID is used by /dev/disk/by-uuid
	UUID string `json:"uuid"`
	// Serial is the disk serial used by /dev/disk/by-id
	Serial string `json:"serial"`
	// Type is the disk type
	Type string `json:"type"`
	// Rotational is the boolean whether the device is rotational: true for hdd, false for ssd and nvme
	Rotational bool `json:"rotational"`
	// Readonly is the boolean whether the device is readonly
	Readonly bool `json:"readOnly"`
	// Partitions is a partition slice
	Partitions []Partition
	// Filesystem is the filesystem currently on the device
	Filesystem string `json:"filesystem"`
	// Mountpoint is the mountpoint of the filesystem on the device
	Mountpoint string `json:"mountpoint"`
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
	// RealPath is the device pathname behind the PVC, behind /mnt/<pvc>/name
	RealPath string `json:"real-path,omitempty"`
	// KernelName is the kernel name of the device
	KernelName string `json:"kernel-name,omitempty"`
	// Whether this device should be encrypted
	Encrypted bool `json:"encrypted,omitempty"`
}

// ListDevices lists all devices available on a machine
func ListDevices(executor exec.Executor) ([]string, error) {
	devices, err := executor.ExecuteCommandWithOutput("lsblk", "--all", "--noheadings", "--list", "--output", "KNAME")
	if err != nil {
		return nil, fmt.Errorf("failed to list all devices: %+v", err)
	}

	return strings.Split(devices, "\n"), nil
}

// resolveDevicePath returns an absolute path for a device. Callers pass either
// a bare kernel name ("sdb") or an absolute path, which may be a device node
// ("/dev/sdb") or a mount point for a PVC-backed block device
// ("/mnt/<pvc-name>"). Only a bare kernel name needs the /dev prefix; adding it
// unconditionally would produce "/dev//dev/sdb".
func resolveDevicePath(device string) string {
	if len(strings.Split(device, "/")) == 1 {
		return fmt.Sprintf("/dev/%s", device)
	}

	return device
}

// GetDevicePartitions gets partitions on a given device
func GetDevicePartitions(device string, executor exec.Executor) (partitions []Partition, unusedSpace uint64, err error) {
	devicePath := resolveDevicePath(device)

	output, err := executor.ExecuteCommandWithOutput("lsblk", devicePath,
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
		} else if strings.HasPrefix(name, CephLVPrefix) && props["TYPE"] == LVMType {
			p := Partition{Name: name}
			partitions = append(partitions, p)
		}
	}

	if deviceSize > 0 {
		unusedSpace = deviceSize - totalPartitionSize
	}
	return partitions, unusedSpace, nil
}

// GetDeviceProperties gets device properties
func GetDeviceProperties(device string, executor exec.Executor) (map[string]string, error) {
	return GetDevicePropertiesFromPath(resolveDevicePath(device), executor)
}

// GetDevicePropertiesFromPath gets the device properties from a path
func GetDevicePropertiesFromPath(devicePath string, executor exec.Executor) (map[string]string, error) {
	output, err := executor.ExecuteCommandWithOutput("lsblk", devicePath,
		"--bytes", "--nodeps", "--pairs", "--paths", "--output", "SIZE,ROTA,RO,TYPE,PKNAME,NAME,KNAME,MOUNTPOINT,FSTYPE")
	if err != nil {
		logger.Errorf("failed to execute lsblk. output: %s", output)
		return nil, err
	}
	logger.Debugf("lsblk output: %q", output)

	return parseKeyValuePairString(output), nil
}

// IsLV returns whether a device is owned by LVM, i.e. is a logical volume
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

// GetUdevInfo gets udev information
func GetUdevInfo(device string, executor exec.Executor) (map[string]string, error) {
	output, err := executor.ExecuteCommandWithOutput("udevadm", "info", "--query=property", resolveDevicePath(device))
	if err != nil {
		return nil, err
	}
	logger.Debugf("udevadm info output: %q", output)

	return parseUdevInfo(output), nil
}

// GetDeviceFilesystems gets the file systems available
func GetDeviceFilesystems(device string, executor exec.Executor) (string, error) {
	device = resolveDevicePath(device)
	output, err := executor.ExecuteCommandWithOutput("udevadm", "info", "--query=property", device)
	if err != nil {
		return "", err
	}

	return parseFS(output), nil
}

// GetDiskUUID looks up the UUID for a disk.
func GetDiskUUID(device string, executor exec.Executor) (string, error) {
	if _, err := osexec.LookPath(sgdiskCmd); err != nil {
		return "", errors.Wrap(err, "sgdisk not found")
	}

	device = resolveDevicePath(device)

	output, err := executor.ExecuteCommandWithOutput(sgdiskCmd, "--print", device)
	if err != nil {
		return "", errors.Wrapf(err, "sgdisk failed. output=%s", output)
	}

	return parseUUID(device, output)
}

func GetDiskDeviceType(disk *LocalDisk) string {
	if disk.Rotational {
		return "hdd"
	}
	if strings.Contains(disk.RealPath, "nvme") {
		return "nvme"
	}
	return "ssd"
}

func GetDiskDeviceClass(crushDeviceClassVarName, deviceType string) string {
	crushDeviceClass := os.Getenv(crushDeviceClassVarName)
	if crushDeviceClass != "" {
		return crushDeviceClass
	} else {
		return deviceType
	}
}

// CheckIfDeviceAvailable checks if a device is available for consumption. The caller
// needs to decide based on the return values whether it is available.
func CheckIfDeviceAvailable(executor exec.Executor, devicePath string, pvcBacked bool) (bool, string, error) {
	checker := isDeviceAvailable

	isLV, err := IsLV(devicePath, executor)
	if err != nil {
		return false, "", fmt.Errorf("failed to determine if the device was LV. %v", err)
	}
	if isLV {
		checker = isLVAvailable
	}

	isAvailable, rejectedReason, err := checker(executor, devicePath)
	if err != nil {
		return false, "", fmt.Errorf("failed to determine if the device was available. %v", err)
	}

	return isAvailable, rejectedReason, nil
}

// GetLVName returns the LV name of the device in the form of "VG/LV".
func GetLVName(executor exec.Executor, devicePath string) (string, error) {
	devInfo, err := executor.ExecuteCommandWithOutput("dmsetup", "info", "-c", "--noheadings", "-o", "name", devicePath)
	if err != nil {
		return "", fmt.Errorf("failed to execute dmsetup info for %q. %v", devicePath, err)
	}
	out, err := executor.ExecuteCommandWithOutput("dmsetup", "splitname", "--noheadings", devInfo)
	if err != nil {
		return "", fmt.Errorf("failed to execute dmsetup splitname for %q. %v", devInfo, err)
	}
	split := strings.Split(out, ":")
	if len(split) < 2 {
		return "", fmt.Errorf("dmsetup splitname returned unexpected result for %q. output: %q", devInfo, out)
	}
	return fmt.Sprintf("%s/%s", split[0], split[1]), nil
}

// finds the disk uuid in the output of sgdisk
func parseUUID(device, output string) (string, error) {
	// find the line with the uuid
	lines := strings.SplitSeq(output, "\n")
	for line := range lines {
		// If GPT is not found in a disk, sgdisk creates a new GPT in memory and reports its UUID.
		// This ID changes each call and is not appropriate to identify the device.
		if strings.Contains(line, "Creating new GPT entries in memory.") {
			break
		}
		if strings.Contains(line, "Disk identifier (GUID)") {
			words := strings.SplitSeq(line, " ")
			for word := range words {
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
			propMap[kvp[0]] = strings.ReplaceAll(kvp[1], `"`, "")
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

func isDeviceAvailable(executor exec.Executor, devicePath string) (bool, string, error) {
	CVInventory, err := inventoryDevice(executor, devicePath)
	if err != nil {
		return false, "", fmt.Errorf("failed to determine if the device %q is available. %v", devicePath, err)
	}

	if CVInventory.Available {
		return true, "", nil
	}

	return false, string(CVInventory.RejectedReasons), nil
}

func inventoryDevice(executor exec.Executor, devicePath string) (CephVolumeInventory, error) {
	var CVInventory CephVolumeInventory

	args := []string{"inventory", "--format", "json", devicePath}
	inventory, err := executor.ExecuteCommandWithOutput("ceph-volume", args...)
	if err != nil {
		return CVInventory, fmt.Errorf("failed to execute ceph-volume inventory on disk %q. %s. %v", devicePath, inventory, err)
	}

	bInventory := []byte(inventory)
	err = json.Unmarshal(bInventory, &CVInventory)
	if err != nil {
		return CVInventory, fmt.Errorf("failed to unmarshal json data coming from ceph-volume inventory %q. %q. %v", devicePath, inventory, err)
	}

	return CVInventory, nil
}

func isLVAvailable(executor exec.Executor, devicePath string) (bool, string, error) {
	lv, err := GetLVName(executor, devicePath)
	if err != nil {
		return false, "", fmt.Errorf("failed to get the LV name for the device %q. %v", devicePath, err)
	}

	cvLVMList, err := lvmList(executor, lv)
	if err != nil {
		return false, "", fmt.Errorf("failed to determine if the device %q is available. %v", devicePath, err)
	}

	if len(cvLVMList) == 0 {
		return true, "", nil
	}
	return false, "Used by Ceph", nil
}

func lvmList(executor exec.Executor, lv string) (CephVolumeLVMList, error) {
	args := []string{"lvm", "list", "--format", "json", lv}
	output, err := executor.ExecuteCommandWithOutput("ceph-volume", args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute ceph-volume lvm list on LV %q. %v", lv, err)
	}

	var cvLVMList CephVolumeLVMList
	err = json.Unmarshal([]byte(output), &cvLVMList)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling json data coming from ceph-volume lvm list %q. %v", lv, err)
	}

	return cvLVMList, nil
}

// ListDevicesChild lists all children available on a device
// For an encrypted device, it will return the encrypted device like so:
// lsblk --noheadings --output NAME --path --list /dev/sdd
// /dev/sdd
// /dev/mapper/ocs-deviceset-thin-1-data-0hmfgp-block-dmcrypt
func ListDevicesChild(executor exec.Executor, device string) ([]string, error) {
	childListRaw, err := executor.ExecuteCommandWithOutput("lsblk", "--noheadings", "--path", "--list", "--output", "NAME", device)
	if err != nil {
		return []string{}, fmt.Errorf("failed to list child devices of %q. %v", device, err)
	}

	return strings.Split(childListRaw, "\n"), nil
}

// IsDeviceEncrypted returns whether the disk has a "crypt" label on it
func IsDeviceEncrypted(executor exec.Executor, device string) (bool, error) {
	deviceType, err := executor.ExecuteCommandWithOutput("lsblk", "--noheadings", "--output", "TYPE", device)
	if err != nil {
		return false, fmt.Errorf("failed to get devices type of %q. %v", device, err)
	}

	return deviceType == "crypt", nil
}
