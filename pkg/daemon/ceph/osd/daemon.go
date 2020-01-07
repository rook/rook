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
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	oposd "github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/util/sys"
)

const (
	pvcDataTypeDevice     = "data"
	pvcMetadataTypeDevice = "metadata"
)

var (
	logger = capnslog.NewPackageLogger("github.com/rook/rook", "cephosd")
)

// StartOSD starts an OSD on a device that was provisioned by ceph-volume
func StartOSD(context *clusterd.Context, osdType, osdID, osdUUID, lvPath string, pvcBackedOSD, lvBackedPV bool, cephArgs []string) error {

	// ensure the config mount point exists
	configDir := fmt.Sprintf("/var/lib/ceph/osd/ceph-%s", osdID)
	err := os.Mkdir(configDir, 0755)
	if err != nil {
		logger.Errorf("failed to create config dir %q. %v", configDir, err)
	}

	// Update LVM config at runtime
	if err := updateLVMConfig(context, pvcBackedOSD, lvBackedPV); err != nil {
		return errors.Wrapf(err, "failed to update lvm configuration file") // fail return here as validation provided by ceph-volume
	}

	var volumeGroupName string
	if pvcBackedOSD && !lvBackedPV {
		volumeGroupName := getVolumeGroupName(lvPath)
		if volumeGroupName == "" {
			return errors.Wrapf(err, "error fetching volume group name for OSD %q", osdID)
		}

		go handleTerminate(context, lvPath, volumeGroupName)

		if err := context.Executor.ExecuteCommand(false, "", "vgchange", "-an", volumeGroupName); err != nil {
			return errors.Wrapf(err, "failed to deactivate volume group for lv %q", lvPath)
		}

		if err := context.Executor.ExecuteCommand(false, "", "vgchange", "-ay", volumeGroupName); err != nil {
			return errors.Wrapf(err, "failed to activate volume group for lv %q", lvPath)
		}
	}

	// activate the osd with ceph-volume
	storeFlag := "--" + osdType
	if err := context.Executor.ExecuteCommand(false, "", "stdbuf", "-oL", "ceph-volume", "lvm", "activate", "--no-systemd", storeFlag, osdID, osdUUID); err != nil {
		return errors.Wrapf(err, "failed to activate osd")
	}

	// run the ceph-osd daemon
	if err := context.Executor.ExecuteCommand(false, "", "ceph-osd", cephArgs...); err != nil {
		// Instead of returning, we want to allow the lvm release to happen below, so we just log the err
		logger.Errorf("failed to start osd or shutting down. %v", err)
	}

	if pvcBackedOSD && !lvBackedPV {
		if err := releaseLVMDevice(context, volumeGroupName); err != nil {
			return errors.Wrapf(err, "failed to release device from lvm")
		}
	}

	return nil
}

func handleTerminate(context *clusterd.Context, lvPath, volumeGroupName string) error {
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGTERM)
	for {
		select {
		case <-sigc:
			logger.Infof("shutdown signal received, exiting...")
			err := killCephOSDProcess(context, lvPath)
			if err != nil {
				return errors.Wrapf(err, "failed to kill ceph-osd process")
			}
			return nil
		}
	}
}

func killCephOSDProcess(context *clusterd.Context, lvPath string) error {

	pid, err := context.Executor.ExecuteCommandWithOutput(false, "", "fuser", "-a", lvPath)
	if err != nil {
		return errors.Wrapf(err, "failed to retrieve process ID for %q", lvPath)
	}

	logger.Infof("process ID for ceph-osd: %s", pid)

	// shut down the osd-ceph process so that lvm release does not show device in use error.
	if pid != "" {
		// The OSD needs to exit as quickly as possible in order for the IO requests
		// to be redirected to other OSDs in the cluster. The OSD is designed to tolerate failures
		// of any kind, including power loss or kill -9. The upstream Ceph tests have for many years
		// been testing with kill -9 so this is expected to be safe. There is a fix upstream Ceph that will
		// improve the shutdown time of the OSD. For cleanliness we should consider removing the -9
		// once it is backported to Nautilus: https://github.com/ceph/ceph/pull/31677.
		if err := context.Executor.ExecuteCommand(false, "", "kill", "-9", pid); err != nil {
			return errors.Wrapf(err, "failed to kill ceph-osd process")
		}
	}

	return nil
}

// Provision provisions an OSD
func Provision(context *clusterd.Context, agent *OsdAgent, crushLocation string) error {
	// set the initial orchestration status
	status := oposd.OrchestrationStatus{Status: oposd.OrchestrationStatusComputingDiff}
	if err := oposd.UpdateNodeStatus(agent.kv, agent.nodeName, status); err != nil {
		return err
	}

	// create the ceph.conf with the default settings
	cephConfig, err := cephconfig.CreateDefaultCephConfig(context, agent.cluster)
	if err != nil {
		return errors.Wrapf(err, "failed to create default ceph config")
	}

	// write the latest config to the config dir
	confFilePath, err := cephconfig.GenerateAdminConnectionConfigWithSettings(context, agent.cluster, cephConfig)
	if err != nil {
		return errors.Wrapf(err, "failed to write connection config")
	}
	src, err := ioutil.ReadFile(filepath.Clean(confFilePath))
	if err != nil {
		return errors.Wrapf(err, "failed to copy connection config to /etc/ceph. failed to read the connection config")
	}
	err = ioutil.WriteFile(cephconfig.DefaultConfigFilePath(), src, 0444)
	if err != nil {
		return errors.Wrapf(err, "failed to copy connection config to /etc/ceph. failed to write %q", cephconfig.DefaultConfigFilePath())
	}
	dst, err := ioutil.ReadFile(cephconfig.DefaultConfigFilePath())
	if err == nil {
		logger.Debugf("config file @ %s: %s", cephconfig.DefaultConfigFilePath(), dst)
	} else {
		logger.Warningf("wrote and copied config file but failed to read it back from %s for logging. %v", cephconfig.DefaultConfigFilePath(), err)
	}

	logger.Infof("discovering hardware")

	var rawDevices []*sys.LocalDisk
	if agent.pvcBacked {
		rawDevice, err := clusterd.PopulateDeviceInfo(agent.devices[0].Name, context.Executor)
		if err != nil {
			return errors.Wrapf(err, "failed to get device info for %q", agent.devices[0].Name)
		}
		rawDevice.Type = pvcDataTypeDevice
		rawDevices = append(rawDevices, rawDevice)

		// We have a metadata device
		if len(agent.devices) > 1 {
			rawMetadataDevice, err := clusterd.PopulateDeviceInfo(agent.devices[1].Name, context.Executor)
			if err != nil {
				return errors.Wrapf(err, "failed to get device info for %q", agent.devices[1].Name)
			}

			// set it's a metadata device
			rawMetadataDevice.Type = pvcMetadataTypeDevice
			rawDevices = append(rawDevices, rawMetadataDevice)
		}
	} else {
		// We still need to use 'lsblk' as the underlaying way to discover devices
		// Ideally, we would use the "ceph-volume inventory" command instead
		// However, it suffers from some limitation such as exposing available partitions and LVs
		// See: https://tracker.ceph.com/issues/43579
		rawDevices, err = clusterd.DiscoverDevices(context.Executor)
		if err != nil {
			return errors.Wrapf(err, "failed initial hardware discovery")
		}
	}

	context.Devices = rawDevices

	logger.Info("creating and starting the osds")

	// determine the set of devices that can/should be used for OSDs.
	devices, err := getAvailableDevices(context, agent.devices, agent.metadataDevice, agent.pvcBacked, agent.cluster.CephVersion)
	if err != nil {
		return errors.Wrap(err, "failed to get available devices")
	}

	// orchestration is about to start, update the status
	status = oposd.OrchestrationStatus{Status: oposd.OrchestrationStatusOrchestrating, PvcBackedOSD: agent.pvcBacked}
	if err := oposd.UpdateNodeStatus(agent.kv, agent.nodeName, status); err != nil {
		return err
	}

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
		return nil
	}

	logger.Infof("devices = %+v", deviceOSDs)

	// Populate CRUSH location for each OSD on the host
	for i := range deviceOSDs {
		deviceOSDs[i].Location = crushLocation
	}

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
		// so it's consumming a raw block device and LVM is not used
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

	osds := deviceOSDs

	// orchestration is completed, update the status
	status = oposd.OrchestrationStatus{OSDs: osds, Status: oposd.OrchestrationStatusCompleted, PvcBackedOSD: agent.pvcBacked}
	if err := oposd.UpdateNodeStatus(agent.kv, agent.nodeName, status); err != nil {
		return err
	}

	return nil
}

func getAvailableDevices(context *clusterd.Context, desiredDevices []DesiredDevice, metadataDevice string, pvcBacked bool, cephVersion cephver.CephVersion) (*DeviceOsdMapping, error) {

	logger.Debugf("desiredDevices are %+v", desiredDevices)
	logger.Debugf("context.Devices are %+v", context.Devices)

	available := &DeviceOsdMapping{Entries: map[string]*DeviceOsdIDEntry{}}
	for _, device := range context.Devices {
		// Ignore 'dm' device since they are not handled by c-v properly
		// see: https://tracker.ceph.com/issues/43209
		if strings.HasPrefix(device.Name, sys.DeviceMapperPrefix) && device.Type == sys.LVMType {
			logger.Infof("skipping 'dm' device %q", device.Name)
			continue
		}

		// Ignore device with filesystem signature since c-v inventory
		// cannot detect that correctly
		// see: https://tracker.ceph.com/issues/43585
		if device.Filesystem != "" {
			logger.Infof("skipping device %q because it contains a filesystem %q", device.Name, device.Filesystem)
			continue
		}

		// We need to swap the device name
		// We need to use the /dev path, provided by the NAME property from lsblk
		// instead of the /mnt/<block name>
		deviceToCheck := device.Name
		if pvcBacked {
			deviceToCheck = device.RealName
		}

		// If we detect a partition we have to make sure that ceph-volume will be able to consume it
		// ceph-volume version 14.2.8 has the right code to support partitions
		if device.Type == sys.PartType {
			if !cephVersion.IsAtLeast(cephVolumeRawModeMinCephVersion) {
				logger.Infof("skipping device %q because it is a partition and ceph version is too old, you need at least ceph %q", device.Name, cephVolumeRawModeMinCephVersion.String())
				continue
			}
		}

		// Check if the desired device is available
		// When running on PVC we use the real device name instead of the Kubernetes mountpoint
		// Otherwise ceph-volume inventory will fail on the udevadm check
		// udevadm does not support device path different than /dev or /sys
		//
		// So earlier lsblk extracted the '/dev' path, hence the device.Name property
		// device.Name can be 'xvdca', later this is formated to '/dev/xvdca'
		isAvailable, rejectedReason, err := sys.CheckIfDeviceAvailable(context.Executor, deviceToCheck, pvcBacked)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get device %q info", device.Name)
		}

		if !isAvailable {
			logger.Infof("skipping device %q: %s.", device.Name, rejectedReason)
			continue
		} else {
			logger.Infof("device %q is available.", device.Name)
		}

		var deviceInfo *DeviceOsdIDEntry
		if metadataDevice != "" && metadataDevice == device.Name {
			// current device is desired as the metadata device
			deviceInfo = &DeviceOsdIDEntry{Data: unassignedOSDID, Metadata: []int{}}
		} else if len(desiredDevices) == 1 && desiredDevices[0].Name == "all" {
			// user has specified all devices, use the current one for data
			deviceInfo = &DeviceOsdIDEntry{Data: unassignedOSDID}
		} else if len(desiredDevices) > 0 {
			var matched bool
			var matchedDevice DesiredDevice
			for _, desiredDevice := range desiredDevices {
				if desiredDevice.IsFilter {
					// the desired devices is a regular expression
					matched, err = regexp.Match(desiredDevice.Name, []byte(device.Name))
					if err != nil {
						logger.Errorf("regex failed on device %q and filter %q. %v", device.Name, desiredDevice.Name, err)
						continue
					}

					if matched {
						logger.Infof("device %q matches device filter %q", device.Name, desiredDevice.Name)
					}
				} else if desiredDevice.IsDevicePathFilter {
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
				} else if device.Name == desiredDevice.Name {
					logger.Infof("%q found in the desired devices", device.Name)
					matched = true
				} else if strings.HasPrefix(desiredDevice.Name, "/dev/") {
					devLinks := strings.Split(device.DevLinks, " ")
					for _, link := range devLinks {
						if link == desiredDevice.Name {
							logger.Infof("%q found in the desired devices (matched by link: %q)", device.Name, link)
							matched = true
							break
						}
					}
				}
				matchedDevice = desiredDevice
				if matched {
					break
				}
			}

			if err == nil && matched {
				// the current device matches the user specifies filter/list, use it for data
				logger.Infof("device %q is selected by the device filter/name %q", device.Name, matchedDevice.Name)
				deviceInfo = &DeviceOsdIDEntry{Data: unassignedOSDID, Config: matchedDevice, PersistentDevicePaths: strings.Fields(device.DevLinks)}

				// set that this is not an OSD but a metadata device
				if device.Type == pvcMetadataTypeDevice {
					logger.Infof("metadata device %q is selected by the device filter/name %q", device.Name, matchedDevice.Name)
					deviceInfo = &DeviceOsdIDEntry{Config: matchedDevice, PersistentDevicePaths: strings.Fields(device.DevLinks), Metadata: []int{1}}
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
			if pvcBacked {
				if device.Type == pvcDataTypeDevice {
					available.Entries[pvcDataTypeDevice] = deviceInfo
				} else if device.Type == pvcMetadataTypeDevice {
					available.Entries[pvcMetadataTypeDevice] = deviceInfo
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
	if err := context.Executor.ExecuteCommand(false, "", "lvchange", "-an", volumeGroupName); err != nil {
		return errors.Wrapf(err, "failed to deactivate LVM %s", volumeGroupName)
	}
	logger.Info("Successfully released device from lvm")
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
