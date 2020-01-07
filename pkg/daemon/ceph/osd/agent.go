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

	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/proc"
)

const (
	unassignedOSDID = -1
)

// OsdAgent represents the OSD struct of an agent
type OsdAgent struct {
	cluster        *cephconfig.ClusterInfo
	nodeName       string
	forceFormat    bool
	osdProc        map[int]*proc.MonitoredProc
	devices        []DesiredDevice
	metadataDevice string
	procMan        *proc.ProcManager
	storeConfig    config.StoreConfig
	kv             *k8sutil.ConfigMapKVStore
	pvcBacked      bool
	configCounter  int32
	osdsCompleted  chan struct{}
}

type device struct {
	name     string
	osdCount int
}

// NewAgent is the instantiation of the OSD agent
func NewAgent(context *clusterd.Context, devices []DesiredDevice, metadataDevice string, forceFormat bool,
	storeConfig config.StoreConfig, cluster *cephconfig.ClusterInfo, nodeName string, kv *k8sutil.ConfigMapKVStore, pvcBacked bool) *OsdAgent {

	return &OsdAgent{
		devices:        devices,
		metadataDevice: metadataDevice,
		forceFormat:    forceFormat,
		storeConfig:    storeConfig,
		cluster:        cluster,
		nodeName:       nodeName,
		kv:             kv,
		pvcBacked:      pvcBacked,
		procMan:        proc.New(context.Executor),
		osdProc:        make(map[int]*proc.MonitoredProc),
	}
}

func getDeviceLVPath(context *clusterd.Context, deviceName string) string {
	cmd := fmt.Sprintf("get logical volume path for device")
	output, err := context.Executor.ExecuteCommandWithOutput(false, cmd, "pvdisplay", "-C", "-o", "lvpath", "--noheadings", deviceName)
	if err != nil {
		logger.Warningf("failed to retrieve logical volume path for %q. %v", deviceName, err)
		return ""
	}
	logger.Debugf("logical volume path for device %q is %q", deviceName, output)
	return output
}
