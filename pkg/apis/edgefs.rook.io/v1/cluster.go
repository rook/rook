/*
Copyright 2019 The Rook Authors. All rights reserved.

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
package v1

import (
	"fmt"
)

const (
	DeploymentRtlfs     = "rtlfs"
	DeploymentRtrd      = "rtrd"
	DeploymentAutoRtlfs = "autoRtlfs"
	DeploymentRtkvs     = "rtkvs"
)

type ClusterDeploymentConfig struct {
	DeploymentType string        //rtlfs, rtrd, autortlfs
	TransportKey   string        //rtlfs or rtrd
	Directories    []RtlfsDevice //cluster wide directories
	DevConfig      map[string]DevicesConfig
	NeedPrivileges bool
}

// GetRtlfsDevices returns array of Rtlfs devices in cluster,
// Rtlfs devices must be the same over the cluster configuration,
// so we can get first non gateway deviceConfig
func (deploymentConfig *ClusterDeploymentConfig) GetRtlfsDevices() []RtlfsDevice {
	rtlfsDevices := make([]RtlfsDevice, 0)
	for _, devConfig := range deploymentConfig.DevConfig {
		if !devConfig.IsGatewayNode {
			return devConfig.Rtlfs.Devices
		}
	}
	return rtlfsDevices
}

func (deploymentConfig *ClusterDeploymentConfig) GetRtkvsDevicesCount() int {
	rtkvsDevicesCount := 0
	for _, devConfig := range deploymentConfig.DevConfig {
		if devConfig.IsGatewayNode {
			continue
		}
		rtkvsDevicesCount += len(devConfig.Rtkvs.Devices)
	}
	return rtkvsDevicesCount
}

func (deploymentConfig *ClusterDeploymentConfig) GetRtrdDevicesCount() int {
	rtrdsDevicesCount := 0
	for _, devConfig := range deploymentConfig.DevConfig {
		if devConfig.IsGatewayNode {
			continue
		}
		rtrdsDevicesCount += len(devConfig.Rtrd.Devices)
		for _, slave := range devConfig.RtrdSlaves {
			rtrdsDevicesCount += len(slave.Devices)
		}
	}
	return rtrdsDevicesCount
}

func (deploymentConfig *ClusterDeploymentConfig) GetTargetsCount() int {
	targets := 0
	for _, devConfig := range deploymentConfig.DevConfig {
		if devConfig.IsGatewayNode {
			continue
		}
		targets++
	}
	return targets
}

func (deploymentConfig *ClusterDeploymentConfig) GetRtrdContainersCount() int {

	containers := 0
	for _, devConfig := range deploymentConfig.DevConfig {
		if devConfig.IsGatewayNode {
			continue
		}
		// 1 is main target container
		containers += 1 + len(devConfig.RtrdSlaves)
	}
	return containers
}

func (deploymentConfig *ClusterDeploymentConfig) CompatibleWith(newConfig ClusterDeploymentConfig) (bool, error) {
	if deploymentConfig.DeploymentType != newConfig.DeploymentType {
		return false, fmt.Errorf("DeploymentType `%s` != `%s` for updated cluster configuration", deploymentConfig.DeploymentType, newConfig.DeploymentType)
	}

	if deploymentConfig.TransportKey != newConfig.TransportKey {
		return false, fmt.Errorf("TransportKey `%s` != `%s` for updated cluster configuration", deploymentConfig.TransportKey, newConfig.TransportKey)
	}

	return true, nil
}

// NodesDifference produces A\B for set of node names
// In case of
//           A: existing cluster configuration
//           B: updated cluster configuration
// A\B -> nodes to delete from cluster
// B\A -> nodes to add to cluster
func (deploymentConfig *ClusterDeploymentConfig) NodesDifference(B ClusterDeploymentConfig) []string {
	difference := make([]string, 0)
	for nodeName := range deploymentConfig.DevConfig {
		if _, ok := B.DevConfig[nodeName]; !ok {
			difference = append(difference, nodeName)
		}
	}
	return difference
}

type DevicesConfig struct {
	Rtrd          RTDevices
	RtrdSlaves    []RTDevices
	Rtlfs         RtlfsDevices
	Rtkvs         RtkvsDevices
	Zone          int
	IsGatewayNode bool
}

// GetRtrdDeviceCount returns all rtrd's devices count on specific node
func (dc *DevicesConfig) GetRtrdDeviceCount() int {
	count := len(dc.Rtrd.Devices)
	if count > 0 {
		for _, rtrdSlave := range dc.RtrdSlaves {
			count += len(rtrdSlave.Devices)
		}
	}
	return count
}

type DevicesResurrectOptions struct {
	NeedToResurrect bool
	NeedToZap       bool
	NeedToWait      bool
	SlaveContainers int
}
