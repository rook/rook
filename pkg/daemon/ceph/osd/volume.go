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

	"github.com/rook/rook/pkg/clusterd"
	oposd "github.com/rook/rook/pkg/operator/ceph/cluster/osd"
)

func (a *OsdAgent) configureDevices(context *clusterd.Context, devices *DeviceOsdMapping) ([]oposd.OSDInfo, error) {

	err := createOSDBootstrapKeyring(context, a.cluster.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to generate osd keyring. %+v", err)
	}

	var osds []oposd.OSDInfo
	if devices == nil || len(devices.Entries) == 0 {
		return osds, nil
	}

	volumeArgs := []string{"lvm", "batch", "--prepare", "--bluestore", "--yes"}
	configured := 0
	for name, device := range devices.Entries {
		if device.Data == -1 {
			logger.Infof("configuring new device %s", name)
			volumeArgs = append(volumeArgs, path.Join("/dev", name))
			configured++
		} else {
			logger.Infof("skipping existing device %s: %d", name, device.Data)
		}
	}

	if configured == 0 {
		logger.Infof("no osd devices attempted configuration on this node")
		return osds, nil
	}

	err = context.Executor.ExecuteCommand(false, "", "ceph-volume", volumeArgs...)
	if err != nil {
		logger.Errorf("failed ceph-volume. %+v", err)
		return osds, nil
	}

	return getCephVolumeOSDs(context, a.cluster.Name)
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
		if len(osdInfo) != 1 {
			logger.Errorf("only expecting one element in the osdInfo array but there were %d", len(osdInfo))
		}

		configDir := "/var/lib/rook/osd" + name
		osd := oposd.OSDInfo{
			ID:          id,
			DataPath:    configDir,
			Config:      fmt.Sprintf("%s/%s.config", configDir, clusterName),
			KeyringPath: path.Join(configDir, "keyring"),
			Cluster:     "ceph",
			UUID:        osdInfo[0].Tags.OSDFSID,
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
