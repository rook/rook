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
	"github.com/rook/rook/pkg/util/display"
	"github.com/rook/rook/pkg/util/exec"
)

var cephConfigDir = "/var/lib/ceph"

const (
	osdsPerDeviceFlag   = "--osds-per-device"
	encryptedFlag       = "--dmcrypt"
	databaseSizeFlag    = "--block-db-size"
	cephVolumeCmd       = "ceph-volume"
	cephVolumeMinDBSize = 1024 // 1GB
)

func (a *OsdAgent) configureCVDevices(context *clusterd.Context, devices *DeviceOsdMapping) ([]oposd.OSDInfo, error) {
	var osds []oposd.OSDInfo

	var err error
	if len(devices.Entries) == 0 {
		logger.Infof("no new devices to configure. returning devices already configured with ceph-volume.")
		osds, err = getCephVolumeOSDs(context, a.cluster.Name, a.cluster.FSID)
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

	osds, err = getCephVolumeOSDs(context, a.cluster.Name, a.cluster.FSID)
	return osds, err
}

func (a *OsdAgent) initializeDevices(context *clusterd.Context, devices *DeviceOsdMapping) error {
	storeFlag := "--bluestore"
	if a.storeConfig.StoreType == config.Filestore {
		storeFlag = "--filestore"
	}

	// Use stdbuf to capture the python output buffer such that we can write to the pod log as the logging happens
	// instead of using the default buffering that will log everything after ceph-volume exits
	baseCommand := "stdbuf"
	baseArgs := []string{"-oL", cephVolumeCmd, "lvm", "batch", "--prepare", storeFlag, "--yes"}
	if a.storeConfig.EncryptedDevice {
		baseArgs = append(baseArgs, encryptedFlag)
	}

	batchArgs := append(baseArgs, []string{
		osdsPerDeviceFlag,
		sanitizeOSDsPerDevice(a.storeConfig.OSDsPerDevice),
	}...)

	if a.storeConfig.StoreType == config.Bluestore && a.storeConfig.DatabaseSizeMB > 0 {
		if a.storeConfig.DatabaseSizeMB < cephVolumeMinDBSize {
			// ceph-volume will convert this value to ?G. It needs to be > 1G to invoke lvcreate.
			logger.Infof("skipping databaseSizeMB setting. For it should be larger than %dMB.", cephVolumeMinDBSize)
		} else {
			batchArgs = append(batchArgs, []string{
				databaseSizeFlag,
				// ceph-volume takes in this value in bytes
				strconv.FormatUint(display.MbTob(uint64(a.storeConfig.DatabaseSizeMB)), 10),
			}...)
		}
	}

	// When mixed hdd/ssd devices are given, ceph-volume configures db lv on the ssd.
	metadataDeviceSpecified := false
	if a.metadataDevice != "" {
		logger.Infof("using %s as metadataDevice and let ceph-volume lvm batch decide how to create volumes", a.metadataDevice)
		metadataDeviceSpecified = true
		batchArgs = append(batchArgs, path.Join("/dev", a.metadataDevice))
	}

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
				immediateExecuteArgs := append(baseArgs, []string{
					deviceArg,
					osdsPerDeviceFlag,
					sanitizeOSDsPerDevice(device.Config.OSDsPerDevice),
				}...)

				// Reporting
				immediateReportArgs := append(immediateExecuteArgs, "--report")

				if err := context.Executor.ExecuteCommand(false, "", baseCommand, immediateReportArgs...); err != nil {
					return fmt.Errorf("failed ceph-volume report. %+v", err) // fail return here as validation provided by ceph-volume
				}

				// execute ceph-volume immediately with the device-specific setting instead of batching up multiple devices together
				if err := context.Executor.ExecuteCommand(false, "", baseCommand, immediateExecuteArgs...); err != nil {
					return fmt.Errorf("failed ceph-volume. %+v", err)
				}

			}
		} else {
			logger.Infof("skipping device %s with osd %d already configured", name, device.Data)
		}
	}

	if configured > 0 {
		// Reporting
		reportArgs := append(batchArgs, []string{
			"--report",
		}...)

		if err := context.Executor.ExecuteCommand(false, "", baseCommand, reportArgs...); err != nil {
			return fmt.Errorf("failed ceph-volume report. %+v", err) // fail return here as validation provided by ceph-volume
		}

		// execute ceph-volume batching up multiple devices
		if err := context.Executor.ExecuteCommand(false, "", baseCommand, batchArgs...); err != nil {
			return fmt.Errorf("failed ceph-volume. %+v", err) // fail return here as validation provided by ceph-volume
		}
	}

	return nil
}

func sanitizeOSDsPerDevice(count int) string {
	if count < 1 {
		count = 1
	}
	return strconv.Itoa(count)
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

func getCephVolumeOSDs(context *clusterd.Context, clusterName string, cephfsid string) ([]oposd.OSDInfo, error) {

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
			if osd.Tags.ClusterFSID != cephfsid {
				logger.Infof("skipping osd%d: %s running on a different ceph cluster: %s", id, osd.Tags.OSDFSID, osd.Tags.ClusterFSID)
				continue
			}
			osdFSID = osd.Tags.OSDFSID
			if osd.Type == "journal" {
				isFilestore = true
			}
		}
		if len(osdFSID) == 0 {
			logger.Infof("Skipping osd%d as no instances are running on ceph cluster: %s", id, cephfsid)
			continue
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
	OSDFSID     string `json:"ceph.osd_fsid"`
	Encrypted   string `json:"ceph.encrypted"`
	ClusterFSID string `json:"ceph.cluster_fsid"`
}
