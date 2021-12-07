/*
Copyright 2020 The Rook Authors. All rights reserved.

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
	"strconv"
	"strings"

	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/controller"
)

const (
	// CephDeviceSetLabelKey is the Rook device set label key
	CephDeviceSetLabelKey = "ceph.rook.io/DeviceSet"
	// CephSetIndexLabelKey is the Rook label key index
	CephSetIndexLabelKey = "ceph.rook.io/setIndex"
	// CephDeviceSetPVCIDLabelKey is the Rook PVC ID label key
	CephDeviceSetPVCIDLabelKey = "ceph.rook.io/DeviceSetPVCId"
	// OSDOverPVCLabelKey is the Rook PVC label key
	OSDOverPVCLabelKey = "ceph.rook.io/pvc"
	// TopologyLocationLabel is the crush location label added to OSD deployments
	TopologyLocationLabel = "topology-location-%s"
)

func makeStorageClassDeviceSetPVCLabel(storageClassDeviceSetName, pvcStorageClassDeviceSetPVCId string, setIndex int) map[string]string {
	return map[string]string{
		CephDeviceSetLabelKey:      storageClassDeviceSetName,
		CephSetIndexLabelKey:       fmt.Sprintf("%d", setIndex),
		CephDeviceSetPVCIDLabelKey: pvcStorageClassDeviceSetPVCId,
	}
}

func (c *Cluster) getOSDLabels(osd OSDInfo, failureDomainValue string, portable bool) map[string]string {
	stringID := fmt.Sprintf("%d", osd.ID)
	labels := controller.CephDaemonAppLabels(AppName, c.clusterInfo.Namespace, config.OsdType, stringID, c.clusterInfo.NamespacedName().Name, "cephclusters.ceph.rook.io", true)
	labels[OsdIdLabelKey] = stringID
	labels[FailureDomainKey] = failureDomainValue
	labels[portableKey] = strconv.FormatBool(portable)
	for k, v := range getOSDTopologyLocationLabels(osd.Location) {
		labels[k] = v
	}
	return labels
}

func getOSDTopologyLocationLabels(topologyLocation string) map[string]string {
	labels := map[string]string{}
	locations := strings.Split(topologyLocation, " ")
	for _, location := range locations {
		loc := strings.Split(location, "=")
		if len(loc) == 2 {
			labels[fmt.Sprintf(TopologyLocationLabel, loc[0])] = loc[1]
		}
	}
	return labels
}
