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
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	oposd "github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	"github.com/rook/rook/pkg/operator/k8sutil"
)

const (
	unassignedOSDID = -1
)

// OsdAgent represents the OSD struct of an agent
type OsdAgent struct {
	clusterInfo    *cephclient.ClusterInfo
	nodeName       string
	forceFormat    bool
	devices        []DesiredDevice
	metadataDevice string
	storeConfig    config.StoreConfig
	kv             *k8sutil.ConfigMapKVStore
	pvcBacked      bool
<<<<<<< HEAD
	replaceOSD     *oposd.OSDReplaceInfo
=======
	replaceOSD     *oposd.OSDInfo
>>>>>>> 79e767e0e (docs: remove deprecated toplogyKey beta labels)
}

// NewAgent is the instantiation of the OSD agent
func NewAgent(context *clusterd.Context, devices []DesiredDevice, metadataDevice string, forceFormat bool,
	storeConfig config.StoreConfig, clusterInfo *cephclient.ClusterInfo, nodeName string, kv *k8sutil.ConfigMapKVStore,
<<<<<<< HEAD
	replaceOSD *oposd.OSDReplaceInfo, pvcBacked bool) *OsdAgent {
=======
	replaceOSD *oposd.OSDInfo, pvcBacked bool) *OsdAgent {
>>>>>>> 79e767e0e (docs: remove deprecated toplogyKey beta labels)

	return &OsdAgent{
		devices:        devices,
		metadataDevice: metadataDevice,
		forceFormat:    forceFormat,
		storeConfig:    storeConfig,
		clusterInfo:    clusterInfo,
		nodeName:       nodeName,
		kv:             kv,
		pvcBacked:      pvcBacked,
		replaceOSD:     replaceOSD,
	}
}

func getDeviceLVPath(context *clusterd.Context, deviceName string) string {
	output, err := context.Executor.ExecuteCommandWithOutput("pvdisplay", "-C", "-o", "lvpath", "--noheadings", deviceName)
	if err != nil {
		logger.Warningf("failed to retrieve logical volume path for %q. %v", deviceName, err)
		return ""
	}
	logger.Debugf("logical volume path for device %q is %q", deviceName, output)
	return output
}

// GetReplaceOSDId returns the OSD ID based on the device name
func (a *OsdAgent) GetReplaceOSDId(device string) int {
<<<<<<< HEAD
	if device == a.replaceOSD.Path {
=======
	if device == a.replaceOSD.BlockPath {
>>>>>>> 79e767e0e (docs: remove deprecated toplogyKey beta labels)
		return a.replaceOSD.ID
	}

	return -1
}
