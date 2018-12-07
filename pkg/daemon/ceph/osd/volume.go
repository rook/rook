/*
Copyright 2018 The Rook Authors. All rights reserved.

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
	"encoding/json"
	"fmt"
	"path"
	"strconv"
	"syscall"

	"github.com/rook/rook/pkg/clusterd"
	oposd "github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	"github.com/rook/rook/pkg/util/exec"
)

var cephConfigDir = "/var/lib/ceph"

const (
	osdsPerDeviceFlag = "--osds-per-device"
	encryptedFlag     = "--dmcrypt"
	cephVolumeCmd     = "ceph-volume"
)

func (a *OsdAgent) configureCVDevices(context *clusterd.Context, devices *DeviceOsdMapping) ([]oposd.OSDInfo, error) {
	var osds []oposd.OSDInfo

	var err error
	if len(devices.Entries) == 0 {
		logger.Infof("no new devices to configure. returning devices already configured with ceph-volume.")
		osds, err = getCephVolumeOSDs(context, a.cluster.Name)
		if err != nil {
			logger.Infof("failed to get devices already provisioned by ceph-volume. %+v", err)
		}
		return osds, nil
	}

	err = createOSDBootstrapKeyring(context, a.cluster.Name, cephConfigDir)
	if err != nil {
		return nil, fmt.Errorf("failed to generate osd keyring. %+v", err)
	}

	if err = a.initializeDevices(context, devices); err != nil {
		return nil, fmt.Errorf("failed to initialize devices. %+v", err)
	}

	osds, err = getCephVolumeOSDs(context, a.cluster.Name)
	return osds, err
}

func (a *OsdAgent) initializeDevices(context *clusterd.Context, devices *DeviceOsdMapping) error {
	storeFlag := "--bluestore"
	if a.storeConfig.StoreType == config.Filestore {
		storeFlag = "--filestore"
	}

	baseArgs := []string{"lvm", "batch", "--prepare", storeFlag, "--yes"}
	if a.storeConfig.EncryptedDevice {
		baseArgs = append(baseArgs, encryptedFlag)
	}

	batchArgs := append(baseArgs, []string{
		osdsPerDeviceFlag,
		strconv.Itoa(a.storeConfig.OSDsPerDevice),
	}...)

	// ceph-volume is soon implementing a parameter to specify the "fast devices", which correspond to the "metadataDevice" from the
	// crd spec. After that is implemented, we can implement this. In the meantime, we fall back to use rook's partitioning.
	metadataDeviceSpecified := false

	configured := 0
	for name, device := range devices.Entries {
		if device.LegacyPartitionsFound {
			logger.Infof("skipping device %s configured with legacy rook osd", name)
			continue
		}

		if device.Data == -1 {
			logger.Infof("configuring new device %s", name)
			deviceArg := path.Join("/dev", name)
			if metadataDeviceSpecified {
				// the device will be configured as a batch at the end of the method
				batchArgs = append(batchArgs, deviceArg)
				configured++
			} else {
				// execute ceph-volume immediately with the device-specific setting instead of batching up multiple devices together
				immediateExecuteArgs := append(baseArgs, []string{
					deviceArg,
					osdsPerDeviceFlag,
					strconv.Itoa(device.Config.OSDsPerDevice),
				}...)

				if err := context.Executor.ExecuteCommand(false, "", cephVolumeCmd, immediateExecuteArgs...); err != nil {
					return fmt.Errorf("failed ceph-volume. %+v", err)
				}

			}
		} else {
			logger.Infof("skipping device %s with osd %d already configured", name, device.Data)
		}
	}

	if configured > 0 {
		if err := context.Executor.ExecuteCommand(false, "", cephVolumeCmd, batchArgs...); err != nil {
			return fmt.Errorf("failed ceph-volume. %+v", err)
		}
	}

	return nil
}
func getCephVolumeSupported(context *clusterd.Context) (bool, error) {
	_, err := context.Executor.ExecuteCommandWithOutput(false, "", cephVolumeCmd, "lvm", "batch", "--prepare")
	if err != nil {
		if cmdErr, ok := err.(*exec.CommandError); ok {
			exitStatus := cmdErr.ExitStatus()
			if exitStatus == int(syscall.ENOENT) || exitStatus == int(syscall.EPERM) {
				logger.Infof("supported version of ceph-volume not available")
				return false, nil
			}
			return false, fmt.Errorf("unknown return code from ceph-volume when checking for compatibility: %d", exitStatus)
		}
		return false, fmt.Errorf("unknown ceph-volume failure. %+v", err)
	}

	return true, nil
}

func getCephVolumeOSDs(context *clusterd.Context, clusterName string) ([]oposd.OSDInfo, error) {
	result, err := context.Executor.ExecuteCommandWithOutput(false, "", cephVolumeCmd, "lvm", "list", "--format", "json")
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve ceph-volume results. %+v", err)
	}
	logger.Debug(result)

	var cephVolumeResult map[string][]osdInfo
	err = json.Unmarshal([]byte(result), &cephVolumeResult)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve ceph-volume results. %+v", err)
	}

	var osds []oposd.OSDInfo
	for name, osdInfo := range cephVolumeResult {
		id, err := strconv.Atoi(name)
		if err != nil {
			logger.Errorf("bad osd returned from ceph-volume: %s", name)
			continue
		}
		var osdFSID string
		isFilestore := false
		for _, osd := range osdInfo {
			osdFSID = osd.Tags.OSDFSID
			if osd.Type == "journal" {
				isFilestore = true
			}
		}
		logger.Infof("osdInfo has %d elements. %+v", len(osdInfo), osdInfo)

		configDir := "/var/lib/rook/osd" + name
		osd := oposd.OSDInfo{
			ID:                  id,
			DataPath:            configDir,
			Config:              fmt.Sprintf("%s/%s.config", configDir, clusterName),
			KeyringPath:         path.Join(configDir, "keyring"),
			Cluster:             "ceph",
			UUID:                osdFSID,
			CephVolumeInitiated: true,
			IsFileStore:         isFilestore,
		}
		osds = append(osds, osd)
	}

	logger.Infof("%d ceph-volume osd devices configured on this node", len(osds))
	return osds, nil
}

type osdInfo struct {
	Name string  `json:"name"`
	Path string  `json:"path"`
	Tags osdTags `json:"tags"`
	// "data" or "journal" for filestore and "block" for bluestore
	Type string `json:"type"`
}

type osdTags struct {
	OSDFSID   string `json:"ceph.osd_fsid"`
	Encrypted string `json:"ceph.encrypted"`
}
