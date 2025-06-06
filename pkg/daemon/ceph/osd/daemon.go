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
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	oposd "github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	"github.com/rook/rook/pkg/util/sys"
)

const (
	pvcDataTypeDevice     = "data"
	pvcMetadataTypeDevice = "metadata"
	pvcWalTypeDevice      = "wal"
	lvmCommandToCheck     = "lvm"
	bluestoreSignature    = "bluestore block device"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "cephosd")

// StartOSD starts an OSD on a device that was provisioned by ceph-volume
func StartOSD(context *clusterd.Context, osdType, osdID, osdUUID, lvPath string, pvcBackedOSD, lvBackedPV bool, cephArgs []string) error {
	// ensure the config mount point exists
	configDir := fmt.Sprintf("/var/lib/ceph/osd/ceph-%s", osdID)
	err := os.Mkdir(configDir, 0o750)
	if err != nil {
		logger.Errorf("failed to create config dir %q. %v", configDir, err)
	}

	// Update LVM config at runtime
	if err := UpdateLVMConfig(context, pvcBackedOSD, lvBackedPV); err != nil {
		return errors.Wrap(err, "failed to update lvm configuration file") // fail return here as validation provided by ceph-volume
	}

	var volumeGroupName string
	if pvcBackedOSD && !lvBackedPV {
		volumeGroupName := getVolumeGroupName(lvPath)
		if volumeGroupName == "" {
			return errors.Wrapf(err, "error fetching volume group name for OSD %q", osdID)
		}

		go handleTerminate(context, lvPath, volumeGroupName)

		// It's fine to continue if deactivate fails since we will return error if activate fails
		if op, err := context.Executor.ExecuteCommandWithCombinedOutput("vgchange", "-an", "-vv", volumeGroupName); err != nil {
			logger.Errorf("failed to deactivate volume group for lv %q. output: %s. %v", lvPath, op, err)
		}

		if op, err := context.Executor.ExecuteCommandWithCombinedOutput("vgchange", "-ay", "-vv", volumeGroupName); err != nil {
			return errors.Wrapf(err, "failed to activate volume group for lv %q. output: %s", lvPath, op)
		}
	}

	// activate the osd with ceph-volume
	storeFlag := "--" + osdType
	if err := context.Executor.ExecuteCommand("stdbuf", "-oL", "ceph-volume", "lvm", "activate", "--no-systemd", storeFlag, osdID, osdUUID); err != nil {
		return errors.Wrap(err, "failed to activate osd")
	}

	// run the ceph-osd daemon
	if err := context.Executor.ExecuteCommand("ceph-osd", cephArgs...); err != nil {
		// Instead of returning, we want to allow the lvm release to happen below, so we just log the err
		logger.Errorf("failed to start osd or shutting down. %v", err)
	}

	if pvcBackedOSD && !lvBackedPV {
		if err := releaseLVMDevice(context, volumeGroupName); err != nil {
			// Let's just report the error and not fail as a best-effort since some drivers will force detach anyway
			// Failing to release the device does not means the detach will fail so let's proceed
			logger.Errorf("failed to release device from lvm. %v", err)
			return nil
		}
	}

	return nil
}

func handleTerminate(context *clusterd.Context, lvPath, volumeGroupName string) {
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGTERM)

	<-sigc
	logger.Infof("shutdown signal received, exiting...")
	err := killCephOSDProcess(context, lvPath)
	if err != nil {
		logger.Errorf("failed to kill ceph-osd process. %v", err)
	}
}

func killCephOSDProcess(context *clusterd.Context, lvPath string) error {
	pid, err := context.Executor.ExecuteCommandWithOutput("fuser", "-a", lvPath)
	if err != nil {
		return errors.Wrapf(err, "failed to retrieve process ID for %q", lvPath)
	}

	logger.Infof("process ID for ceph-osd: %s", pid)

	// shut down the osd-ceph process so that lvm release does not show device in use error.
	if pid != "" {
		if err := context.Executor.ExecuteCommand("kill", pid); err != nil {
			return errors.Wrap(err, "failed to kill ceph-osd process")
		}
	}

	return nil
}

func configRawDevice(name string, context *clusterd.Context) (*sys.LocalDisk, error) {
	rawDevice, err := clusterd.PopulateDeviceInfo(name, context.Executor)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get device info for %q", name)
	}

	// set the device type: data, block_db(metadata) or wal.
	if strings.HasPrefix(name, "/mnt") {
		rawDevice, err = clusterd.PopulateDeviceUdevInfo(rawDevice.KernelName, context.Executor, rawDevice)
		if err != nil {
			logger.Warningf("failed to get udev info for device %q. %v", name, err)
		}
		rawDevice.Type = pvcDataTypeDevice
	} else if strings.HasPrefix(name, "/srv") {
		rawDevice.Type = pvcMetadataTypeDevice
	} else if strings.HasPrefix(name, "/wal") {
		rawDevice.Type = pvcWalTypeDevice
	}

	return rawDevice, nil
}

// Provision provisions an OSD
func Provision(context *clusterd.Context, agent *OsdAgent, crushLocation, topologyAffinity, deviceFilter, metaDevice string) error {
	if agent.pvcBacked && os.Getenv(oposd.EncryptedDeviceEnvVarName) == "true" {
		logger.Debug("encryption configuration detecting, populating kek to an env variable")
		// Init KMS store, retrieve the KEK and store it as an env var for ceph-volume
		err := setKEKinEnv(context, agent.clusterInfo)
		if err != nil {
			return errors.Wrap(err, "failed to set kek as an environment variable")
		}
	}

	// Print dmsetup version
	err := dmsetupVersion(context)
	if err != nil {
		return errors.Wrap(err, "failed to print device mapper version")
	}

	// set the initial orchestration status
	status := oposd.OrchestrationStatus{Status: oposd.OrchestrationStatusOrchestrating}
	oposd.UpdateNodeOrPVCStatus(agent.clusterInfo.Context, agent.kv, agent.nodeName, status)

	logger.Infof("discovering hardware")

	var rawDevices []*sys.LocalDisk
	if agent.pvcBacked {
		for i := range agent.devices {
			rawDevice, err := configRawDevice(agent.devices[i].Name, context)
			if err != nil {
				return err
			}

			rawDevices = append(rawDevices, rawDevice)
		}
	} else {
		// We still need to use 'lsblk' as the underlying way to discover devices
		// Ideally, we would use the "ceph-volume inventory" command instead
		// However, it suffers from some limitation such as exposing available partitions and LVs
		// See: https://tracker.ceph.com/issues/43579
		rawDevices, err = clusterd.DiscoverDevicesWithFilter(context.Executor, deviceFilter, metaDevice)
		if err != nil {
			return errors.Wrap(err, "failed initial hardware discovery")
		}
	}

	context.Devices = rawDevices

	// Wipe the desired OSD disks in case they belong to a different ceph cluster.
	if agent.wipeDevicesFromOtherClusters {
		logger.Info("checking for OSD disks from a different cluster")
		err := agent.WipeDevicesFromOtherClusters(context)
		if err != nil {
			return errors.Wrapf(err, "failed to wipe devices from other clusters")
		}
	}

	logger.Info("creating and starting the osds")

	// determine the set of devices that can/should be used for OSDs.
	devices, err := getAvailableDevices(context, agent)
	if err != nil {
		return errors.Wrap(err, "failed to get available devices")
	}

	// orchestration is about to start, update the status
	status = oposd.OrchestrationStatus{Status: oposd.OrchestrationStatusOrchestrating, PvcBackedOSD: agent.pvcBacked}
	oposd.UpdateNodeOrPVCStatus(agent.clusterInfo.Context, agent.kv, agent.nodeName, status)

	// start the desired OSDs on devices
	logger.Infof("configuring osd devices: %+v", devices)

	deviceOSDs, err := agent.configureCVDevices(context, devices)
	if err != nil {
		return errors.Wrap(err, "failed to configure devices")
	}

	// Let's fail if no OSDs were configured
	// This likely means the filter for available devices passed (in PVC case)
	// but the resulting device was already configured for another cluster (disk not wiped and leftover)
	// So we need to make sure the list is filled up, otherwise fail
	if len(deviceOSDs) == 0 {
		logger.Warningf("skipping OSD configuration as no devices matched the storage settings for this node %q", agent.nodeName)
		status = oposd.OrchestrationStatus{OSDs: deviceOSDs, Status: oposd.OrchestrationStatusCompleted, PvcBackedOSD: agent.pvcBacked}
		oposd.UpdateNodeOrPVCStatus(agent.clusterInfo.Context, agent.kv, agent.nodeName, status)
		return nil
	}

	// Populate CRUSH location for each OSD on the host
	for i := range deviceOSDs {
		deviceOSDs[i].Location = crushLocation
		deviceOSDs[i].TopologyAffinity = topologyAffinity
	}

	logger.Infof("devices = %+v", deviceOSDs)

	// Since we are done configuring the PVC we need to release it from LVM
	// If we don't do this, the device will remain hold by LVM and we won't be able to detach it
	// When running on PVC, the device is:
	//  * attached on the prepare pod
	//  * osd is mkfs
	//  * detached from the prepare pod
	//  * attached to the activate pod
	//  * then the OSD runs
	if agent.pvcBacked && !deviceOSDs[0].SkipLVRelease && !deviceOSDs[0].LVBackedPV {
		// Try to discover the VG of that LV
		volumeGroupName := getVolumeGroupName(deviceOSDs[0].BlockPath)

		// If empty the osd is using the ceph-volume raw mode
		// so it's consuming a raw block device and LVM is not used
		// so there is nothing to de-activate
		if volumeGroupName != "" {
			if err := releaseLVMDevice(context, volumeGroupName); err != nil {
				return errors.Wrap(err, "failed to release device from lvm")
			}
		} else {
			// TODO
			// don't assume this and run a bluestore check on the device to be sure?
			logger.Infof("ceph-volume raw mode used by block %q, no VG to de-activate", deviceOSDs[0].BlockPath)
		}
	}

	// orchestration is completed, update the status
	status = oposd.OrchestrationStatus{OSDs: deviceOSDs, Status: oposd.OrchestrationStatusCompleted, PvcBackedOSD: agent.pvcBacked}
	oposd.UpdateNodeOrPVCStatus(agent.clusterInfo.Context, agent.kv, agent.nodeName, status)

	return nil
}

func matchDevLinks(devLinks, deviceName string) bool {
	for _, link := range strings.Split(devLinks, " ") {
		if link == deviceName {
			logger.Infof("%q found in the desired devices (matched by link: %q)", deviceName, link)
			return true
		}
	}
	return false
}

func getOsdUUIDImpl(device *sys.LocalDisk) (string, error) {
	// Old lsblk can't detect an existing OSD.
	// See: https://github.com/rook/rook/issues/10665
	logger.Infof("old lsblk can't detect bluestore signature, so try to detect here")

	f, err := os.Open(device.RealPath)
	if err != nil {
		err := fmt.Errorf("failed to open %q. %v", device.Name, err)
		return "", err
	}
	defer f.Close()

	readBuffer := make([]byte, 128)
	_, err = f.Read(readBuffer)
	if err != nil {
		err := fmt.Errorf("failed to read signature from %q. %v", device.Name, err)
		return "", err
	}

	reader := bufio.NewReader(bytes.NewReader(readBuffer))

	signature, err := reader.ReadString('\n')
	if err != nil {
		if err == io.EOF {
			return "", nil
		}

		err := fmt.Errorf("failed to read signature from %q. %v", device.Name, err)
		return "", err
	}

	if signature[:len(signature)-1] != bluestoreSignature {
		return "", nil
	}

	uuid, err := reader.ReadString('\n')
	if err != nil {
		err := fmt.Errorf("failed to read OSD uuid from %q. %v", device.Name, err)
		return "", err
	}

	return uuid[:len(uuid)-1], nil
}

var getOsdUUID func(device *sys.LocalDisk) (string, error) = getOsdUUIDImpl

func getAvailableDevices(context *clusterd.Context, agent *OsdAgent) (*DeviceOsdMapping, error) {
	desiredDevices := agent.devices
	logger.Debugf("desiredDevices are %+v", desiredDevices)

	logger.Debug("context.Devices are:")
	for _, disk := range context.Devices {
		logger.Debugf("%+v", disk)
	}

	available := &DeviceOsdMapping{Entries: map[string]*DeviceOsdIDEntry{}}
	for _, device := range context.Devices {
		// Add detection for mounted device and skip if mounted
		if device.Mountpoint != "" {
			logger.Infof("skipping device %q with mountpoint %q", device.Name, device.Mountpoint)
			continue
		}

		// Ignore device with filesystem signature since c-v inventory
		// cannot detect that correctly
		// see: https://tracker.ceph.com/issues/43585
		if device.Filesystem != "" {
			// Allow further inspection of that device before skipping it
			if device.Filesystem == "crypto_LUKS" && agent.pvcBacked {
				if isCephEncryptedBlock(context, agent.clusterInfo.FSID, device.Name) {
					logger.Infof("encrypted disk %q is an OSD part of this cluster, skipping it", device.Name)
				} else {
					logger.Infof("encrypted disk %q is unknown, skipping it", device.Name)
				}
				// We must skip so that the device is not marked as available, but will later be
				// picked up by the GetCephVolumeRawOSDs() call.
				// This handles the case where the OSD deployment has been removed and the prepare
				// job kicks in again to re-deploy the OSD.
				continue

			} else if device.Filesystem == "mpath_member" && agent.pvcBacked {
				logger.Infof("allowing multipath disk %q with filesystem %q", device.Name, device.Filesystem)
			} else {
				logger.Infof("skipping device %q because it contains a filesystem %q", device.Name, device.Filesystem)
				continue
			}
		} else {
			uuid, err := getOsdUUID(device)
			if err != nil {
				logger.Errorf("skipping device %q, failed to get OSD information. %v", device.Name, err)
				continue
			}

			if uuid != "" {
				logger.Infof("skipping device %q, detected an existing OSD. UUID=%s", device.Name, uuid)
				continue
			}
		}

		if device.Type == sys.PartType {
			device, err := clusterd.PopulateDeviceUdevInfo(device.Name, context.Executor, device)
			if err != nil {
				logger.Errorf("failed to get udev info of partition %q. %v", device.Name, err)
				continue
			}
		}

		// Check if the desired device is available
		//
		// We need to use the /dev path, provided by the NAME property from "lsblk --paths",
		// especially when running on PVC and/or on dm device
		// When running on PVC we use the real device name instead of the Kubernetes mountpoint
		// When running on dm device we use the dm device name like "/dev/mapper/foo" instead of "/dev/dm-1"
		// Otherwise ceph-volume inventory will fail on the udevadm check
		// udevadm does not support device path different than /dev or /sys
		//
		// So earlier lsblk extracted the '/dev' path, hence the device.Name property
		// device.Name can be 'xvdca', later this is formatted to '/dev/xvdca'
		var err error
		var isAvailable bool
		rejectedReason := ""
		if agent.pvcBacked {
			block := fmt.Sprintf("/mnt/%s", agent.nodeName)
			rawOsds, err := GetCephVolumeRawOSDs(context, agent.clusterInfo, agent.clusterInfo.FSID, block, agent.metadataDevice, "", false, true)
			if err != nil {
				isAvailable = false
				rejectedReason = fmt.Sprintf("failed to detect if there is already an osd. %v", err)
			} else if len(rawOsds) > 0 {
				isAvailable = false
				rejectedReason = "already in use by a raw OSD, no need to reconfigure"
			} else {
				isAvailable = true
			}
		} else {
			isAvailable, rejectedReason, err = sys.CheckIfDeviceAvailable(context.Executor, device.RealPath, agent.pvcBacked)
			if err != nil {
				isAvailable = false
				rejectedReason = fmt.Sprintf("failed to check if the device %q is available. %v", device.Name, err)
			}
		}

		if !isAvailable {
			logger.Infof("skipping device %q: %s.", device.Name, rejectedReason)
			continue
		}
		logger.Infof("device %q is available.", device.Name)

		if device.Type == sys.PartType && agent.storeConfig.EncryptedDevice {
			logger.Infof("partition %q is not picked because encrypted OSD on partition is not allowed", device.Name)
			continue
		}

		var deviceInfo *DeviceOsdIDEntry
		if agent.metadataDevice != "" && agent.metadataDevice == device.Name {
			// current device is desired as the metadata device
			deviceInfo = &DeviceOsdIDEntry{Data: unassignedOSDID, Metadata: []int{}, DeviceInfo: device}
		} else if len(desiredDevices) == 1 && desiredDevices[0].Name == "all" {
			// user has specified all devices, use the current one for data
			if device.Type == sys.LVMType {
				logger.Infof("logical volume %q is not picked by `useAllDevices: true`. please specify the exact device name (e.g. /dev/vg/lv) in `devices` field instead", device.Name)
				continue
			} else if device.Type == sys.LoopType {
				logger.Infof("loop device %q is not picked by `useAllDevices: true`. please specify the exact device name (e.g. /dev/loop0) in `devices` field  instead", device.Name)
				continue
			}
			deviceInfo = &DeviceOsdIDEntry{Data: unassignedOSDID, DeviceInfo: device}
		} else if len(desiredDevices) > 0 {
			var matched bool
			var matchedDevice DesiredDevice
			for _, desiredDevice := range desiredDevices {
				if desiredDevice.IsFilter {
					// the desired devices is a regular expression
					if device.Type == sys.LVMType {
						logger.Infof("logical volume %q is not picked by `deviceFilter`. please specify the exact device name (e.g. /dev/vg/lv) in `devices` field instead", device.Name)
						continue
					} else if device.Type == sys.LoopType {
						logger.Infof("loop device %q is not picked by `deviceFilter`. please specify the exact device name (e.g. /dev/loop0) in `devices` field instead", device.Name)
						continue
					}
					matched, err = regexp.Match(desiredDevice.Name, []byte(device.Name))
					if err != nil {
						logger.Errorf("regex failed on device %q and filter %q. %v", device.Name, desiredDevice.Name, err)
						continue
					}

					if matched {
						logger.Infof("device %q matches device filter %q", device.Name, desiredDevice.Name)
					}
				} else if desiredDevice.IsDevicePathFilter {
					if device.Type == sys.LVMType {
						logger.Infof("logical volume %q is not picked by `devicePathFilter`. please specify the exact device name (e.g. /dev/vg/lv) in `devices` field instead", device.Name)
						continue
					} else if device.Type == sys.LoopType {
						logger.Infof("loop device %q is not picked by `devicePathFilter`. please specify the exact device name (e.g. /dev/loop0) in `devices` field instead", device.Name)
						continue
					}
					pathnames := append(strings.Fields(device.DevLinks), filepath.Join("/dev", device.Name))
					for _, pathname := range pathnames {
						matched, err = regexp.Match(desiredDevice.Name, []byte(pathname))
						if err != nil {
							logger.Errorf("regex failed on device %q and filter %q. %v", device.Name, desiredDevice.Name, err)
							continue
						}

						if matched {
							logger.Infof("device %q (aliases: %q) matches device path filter %q", device.Name, device.DevLinks, desiredDevice.Name)
							break
						}
					}
				} else {
					// the desired device is a file
					if device.Name == desiredDevice.Name || filepath.Join("/dev", device.Name) == desiredDevice.Name {
						logger.Infof("%q found in the desired devices", device.Name)
						matched = true
					} else if strings.HasPrefix(desiredDevice.Name, "/dev/") {
						matched = matchDevLinks(device.DevLinks, desiredDevice.Name)
					}
					if matched && device.Type == sys.LVMType {
						if agent.storeConfig.EncryptedDevice {
							logger.Infof("logical volume %q is not picked because encrypted OSD on LV is not allowed", device.Name)
							matched = false
							continue
						}
						if desiredDevice.MetadataDevice != "" {
							logger.Infof("logical volume %q is not picked because OSD on LV with metadata device is not allowed", device.Name)
							matched = false
							continue
						}
					}
				}

				if matched {
					matchedDevice = desiredDevice
					matchedDevice.UpdateDeviceClass(agent, device)
					break
				}
			}

			if err == nil && matched {
				// the current device matches the user specifies filter/list, use it for data
				logger.Infof("device %q is selected by the device filter/name %q", device.Name, matchedDevice.Name)
				deviceInfo = &DeviceOsdIDEntry{Data: unassignedOSDID, Config: matchedDevice, PersistentDevicePaths: strings.Fields(device.DevLinks), DeviceInfo: device}

				// set that this is not an OSD but a metadata device
				if device.Type == pvcMetadataTypeDevice {
					logger.Infof("metadata device %q is selected by the device filter/name %q", device.Name, matchedDevice.Name)
					deviceInfo = &DeviceOsdIDEntry{Config: matchedDevice, PersistentDevicePaths: strings.Fields(device.DevLinks), Metadata: []int{1}, DeviceInfo: device}
				}

				// set that this is not an OSD but a wal device
				if device.Type == pvcWalTypeDevice {
					logger.Infof("wal device %q is selected by the device filter/name %q", device.Name, matchedDevice.Name)
					deviceInfo = &DeviceOsdIDEntry{Config: matchedDevice, PersistentDevicePaths: strings.Fields(device.DevLinks), Metadata: []int{2}, DeviceInfo: device}
				}
			} else {
				logger.Infof("skipping device %q that does not match the device filter/list (%v). %v", device.Name, desiredDevices, err)
			}
		} else {
			logger.Infof("skipping device %q until the admin specifies it can be used by an osd", device.Name)
		}

		if deviceInfo != nil {
			// When running on PVC, we typically have a single device only
			// So it's fine to name the first entry of the map "data" instead of the PVC name
			// It is particularly useful when a metadata PVC is used because we need to identify it in the map
			// So the entry must be named "metadata" so it can accessed later
			if agent.pvcBacked {
				switch device.Type {
				case pvcDataTypeDevice:
					available.Entries[pvcDataTypeDevice] = deviceInfo
				case pvcMetadataTypeDevice:
					available.Entries[pvcMetadataTypeDevice] = deviceInfo
				case pvcWalTypeDevice:
					available.Entries[pvcWalTypeDevice] = deviceInfo
				}
			} else {
				available.Entries[device.Name] = deviceInfo
			}
		}
	}

	return available, nil
}

// releaseLVMDevice deactivates the LV to release the device.
func releaseLVMDevice(context *clusterd.Context, volumeGroupName string) error {
	if op, err := context.Executor.ExecuteCommandWithCombinedOutput("lvchange", "-an", "-vv", volumeGroupName); err != nil {
		return errors.Wrapf(err, "failed to deactivate LVM %s. output: %s", volumeGroupName, op)
	}
	logger.Info("successfully released device from lvm")
	return nil
}

// getVolumeGroupName returns the Volume group name from the given Logical Volume Path
func getVolumeGroupName(lvPath string) string {
	vgSlice := strings.Split(lvPath, "/")
	// Assert that lvpath is in correct format `/dev/<vg name>/<lv name>` before extracting the vg name
	if len(vgSlice) != 4 || vgSlice[2] == "" {
		logger.Warningf("invalid LV Path: %q", lvPath)
		return ""
	}

	return vgSlice[2]
}

// GetOSDInfoById returns the osdInfo using the ceph volume list
func GetOSDInfoById(context *clusterd.Context, clusterInfo *client.ClusterInfo, osdID int) (*oposd.OSDInfo, error) {
	// LVM mode OSDs
	osdLVMList, err := GetCephVolumeLVMOSDs(context, clusterInfo, clusterInfo.FSID, "", false, false)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list lvm osd(s)")
	}

	for _, osdInfo := range osdLVMList {
		if osdInfo.ID == osdID {
			return &osdInfo, nil
		}
	}

	// Raw mode OSDs
	osdRawList, err := GetCephVolumeRawOSDs(context, clusterInfo, clusterInfo.FSID, "", "", "", false, true)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list raw osd(s)")
	}

	for _, osdInfo := range osdRawList {
		if osdInfo.ID == osdID {
			return &osdInfo, nil
		}
	}

	return nil, fmt.Errorf("failed to get details for OSD %d using ceph-volume list", osdID)
}
