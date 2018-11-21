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
	"strings"
	"syscall"

	"github.com/rook/rook/pkg/clusterd"
	oposd "github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	"github.com/rook/rook/pkg/util/exec"
)

var cephConfigDir = "/var/lib/ceph"

func (a *OsdAgent) configureDevices(context *clusterd.Context, devices *DeviceOsdMapping) ([]oposd.OSDInfo, bool, error) {
	var osds []oposd.OSDInfo

	useCephVolume, err := getCephVolumeSupported(context)
	if err != nil {
		return nil, false, fmt.Errorf("failed to detect if ceph-volume is available. %+v", err)
	}
	if !useCephVolume {
		return osds, false, nil
	}

	if devices == nil || len(devices.Entries) == 0 {
		logger.Infof("no new devices to configure. returning devices already configured with ceph-volume.")
		osds, err = getCephVolumeOSDs(context, a.cluster.Name)
		if err != nil {
			logger.Infof("failed to get devices already provisioned by ceph-volume. %+v", err)
		}
		return osds, true, nil
	}

	err = createOSDBootstrapKeyring(context, a.cluster.Name, cephConfigDir)
	if err != nil {
		return nil, true, fmt.Errorf("failed to generate osd keyring. %+v", err)
	}

	storeFlag := "--bluestore"
	if a.storeConfig.StoreType == config.Filestore {
		storeFlag = "--filestore"
	}

	volumeArgs := []string{"lvm", "batch", "--prepare", storeFlag, "--yes"}
	configured := 0
	for name, device := range devices.Entries {
		if device.LegacyPartitionsFound {
			logger.Infof("skipping device %s configured with legacy rook osd", name)
			continue
		}

		if device.Data == -1 {
			logger.Infof("configuring new device %s", name)
			volumeArgs = append(volumeArgs, path.Join("/dev", name))
			configured++
		} else {
			logger.Infof("skipping device %s with osd %d already configured", name, device.Data)
		}
	}

	if configured > 0 {
		err = context.Executor.ExecuteCommand(false, "", "ceph-volume", volumeArgs...)
		if err != nil {
			return osds, true, fmt.Errorf("failed ceph-volume. %+v", err)
		}
	}

	osds, err = getCephVolumeOSDs(context, a.cluster.Name)
	return osds, true, err
}

func getCephVolumeSupported(context *clusterd.Context) (bool, error) {
	_, err := context.Executor.ExecuteCommandWithOutput(false, "", "ceph-volume", "lvm", "batch", "--prepare")
	if err != nil {
		if cmdErr, ok := err.(*exec.CommandError); ok {
			exitStatus := cmdErr.ExitStatus()
			if exitStatus == int(syscall.ENOENT) {
				logger.Infof("supported version of ceph-volume not available")
				return false, nil
			}
			logger.Warningf("unknown return code from ceph-volume when checking for compatibility: %d", exitStatus)
		}
		logger.Warningf("unknown ceph-volume failure. %+v", err)
		return false, nil
	}

	return true, nil
}

func getCephVolumeOSDs(context *clusterd.Context, clusterName string) ([]oposd.OSDInfo, error) {
	result, err := context.Executor.ExecuteCommandWithOutput(false, "", "ceph-volume", "lvm", "list", "--format", "json")
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve ceph-volume results. %+v", err)
	}

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
			if strings.HasPrefix(osd.Path, "/dev/ceph-filestore") {
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

	logger.Infof("%d osd devices configured on this node", len(osds))
	return osds, nil
}

type osdInfo struct {
	Name string  `json:"name"`
	Path string  `json:"path"`
	Tags osdTags `json:"tags"`
}

type osdTags struct {
	OSDFSID   string `json:"ceph.osd_fsid"`
	Encrypted string `json:"ceph.encrypted"`
}
