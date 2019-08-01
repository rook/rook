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
package cluster

import (
	"fmt"
	"os"

	edgefsv1beta1 "github.com/rook/rook/pkg/apis/edgefs.rook.io/v1beta1"
	rookv1alpha2 "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/operator/discover"
	"github.com/rook/rook/pkg/operator/edgefs/cluster/target"
	"github.com/rook/rook/pkg/operator/edgefs/cluster/target/config"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/sys"
)

type ClusterReconfigureSpec struct {
	DeploymentConfig     edgefsv1beta1.ClusterDeploymentConfig
	ClusterNodesToDelete []string
	ClusterNodesToAdd    []string
}

func (c *cluster) createClusterReconfigurationSpec(existingConfig edgefsv1beta1.ClusterDeploymentConfig, validNodes []rookv1alpha2.Node, dro edgefsv1beta1.DevicesResurrectOptions) (ClusterReconfigureSpec, error) {

	deploymentType, err := c.getClusterDeploymentType()
	if err != nil {
		return ClusterReconfigureSpec{}, err
	}
	logger.Debugf("ClusterSpec: %s", ToJSON(c.Spec))

	transportKey := getClusterTransportKey(deploymentType)
	reconfigSpec := ClusterReconfigureSpec{
		DeploymentConfig: edgefsv1beta1.ClusterDeploymentConfig{
			DeploymentType: deploymentType,
			TransportKey:   transportKey,
			DevConfig:      make(map[string]edgefsv1beta1.DevicesConfig, 0),
			NeedPrivileges: c.needPrivelege(deploymentType),
		},
	}

	// Iterate over cluster nodes
	for _, node := range validNodes {
		// Copy devices confiruration for already existing devicesConfig
		// We can't modify devices config for already existing node in config map
		var devicesConfig edgefsv1beta1.DevicesConfig
		if nodeDevConfig, ok := existingConfig.DevConfig[node.Name]; ok {
			devicesConfig = nodeDevConfig
		} else {
			// create ned devices config for new node
			devicesConfig, err = c.createDevicesConfig(deploymentType, node, dro)
			if err != nil {
				logger.Warningf("Can't create DevicesConfig for %s node, Error: %+v", node.Name, err)
				continue
			}

		}

		reconfigSpec.DeploymentConfig.DevConfig[node.Name] = devicesConfig
	}

	// Calculate nodes to delete from cluster
	reconfigSpec.ClusterNodesToDelete = existingConfig.NodesDifference(reconfigSpec.DeploymentConfig)

	// Calculate nodes to add to cluster
	reconfigSpec.ClusterNodesToAdd = reconfigSpec.DeploymentConfig.NodesDifference(existingConfig)

	_, err = existingConfig.CompatibleWith(reconfigSpec.DeploymentConfig)
	if err != nil {
		return reconfigSpec, err
	}

	err = c.alignSlaveContainers(&reconfigSpec.DeploymentConfig)
	if err != nil {
		return reconfigSpec, err
	}

	err = c.validateDeploymentConfig(reconfigSpec.DeploymentConfig, dro)
	if err != nil {
		return reconfigSpec, err
	}

	return reconfigSpec, nil
}

func (c *cluster) getClusterDeploymentType() (string, error) {

	if len(c.Spec.Storage.Directories) > 0 && (len(c.Spec.DataDirHostPath) > 0 || c.Spec.DataVolumeSize.Value() != 0) {
		return edgefsv1beta1.DeploymentRtlfs, nil
	} else if c.HasDevicesSpecification() && (len(c.Spec.DataDirHostPath) > 0 || c.Spec.DataVolumeSize.Value() != 0) {
		return edgefsv1beta1.DeploymentRtrd, nil
	} else if len(c.Spec.DataDirHostPath) == 0 || c.Spec.DataVolumeSize.Value() == 0 {
		return edgefsv1beta1.DeploymentAutoRtlfs, nil
	}
	return "", fmt.Errorf("Can't determine deployment type for [%s] cluster", c.Namespace)
}

func (c *cluster) needPrivelege(deploymentType string) bool {

	needPriveleges := false
	// Set privileges==true in case of HostNetwork
	if len(c.Spec.Network.ServerIfName) > 0 || len(c.Spec.Network.BrokerIfName) > 0 {
		needPriveleges = true
	}

	if deploymentType == edgefsv1beta1.DeploymentRtrd {
		needPriveleges = true
	}
	return needPriveleges
}

func getClusterTransportKey(deploymentType string) string {
	transportKey := edgefsv1beta1.DeploymentRtrd
	if deploymentType == edgefsv1beta1.DeploymentRtlfs || deploymentType == edgefsv1beta1.DeploymentAutoRtlfs {
		transportKey = edgefsv1beta1.DeploymentRtlfs
	}
	return transportKey
}

// createDevicesConfig creates DevicesConfig for specific node
func (c *cluster) createDevicesConfig(deploymentType string, node rookv1alpha2.Node, dro edgefsv1beta1.DevicesResurrectOptions) (edgefsv1beta1.DevicesConfig, error) {
	devicesConfig := edgefsv1beta1.DevicesConfig{}
	devicesConfig.Rtrd.Devices = make([]edgefsv1beta1.RTDevice, 0)
	devicesConfig.RtrdSlaves = make([]edgefsv1beta1.RTDevices, 0)
	devicesConfig.Rtlfs.Devices = make([]edgefsv1beta1.RtlfsDevice, 0)

	n := c.resolveNode(node.Name)
	if n == nil {
		return devicesConfig, fmt.Errorf("Can't resolve node '%s'", node.Name)
	}

	storeConfig := config.ToStoreConfig(n.Config)
	// Apply Node's zone value
	devicesConfig.Zone = storeConfig.Zone

	// If node labeled as gateway then return empty devices and skip RTDevices detection
	if c.isGatewayLabeledNode(c.context.Clientset, node.Name) {
		devicesConfig.IsGatewayNode = true
		logger.Infof("Skipping node [%s] devices as labeled as gateway node", node.Name)
		return devicesConfig, nil
	}

	// Skip device detection in case of 'restore' option.
	// If dro.NeedToResurrect is true then there is no cluster's config map available
	if dro.NeedToResurrect {
		logger.Infof("Skipping node [%s] devices due 'restore' option", node.Name)
		devicesConfig.Rtlfs.Devices = target.GetRtlfsDevices(c.Spec.Storage.Directories, &storeConfig)
		if dro.SlaveContainers > 0 {
			devicesConfig.RtrdSlaves = make([]edgefsv1beta1.RTDevices, dro.SlaveContainers)
		}

		return devicesConfig, nil
	}

	if deploymentType == edgefsv1beta1.DeploymentRtrd {
		rookSystemNS := os.Getenv(k8sutil.PodNamespaceEnvVar)
		nodeDevices, _ := discover.ListDevices(c.context, rookSystemNS, n.Name)

		logger.Infof("[%s] available devices: ", n.Name)
		for _, dev := range nodeDevices[n.Name] {
			logger.Infof("\tName: %s, Size: %s, Type: %s, Rotational: %t, Empty: %t", dev.Name, edgefsv1beta1.ByteCountBinary(dev.Size), dev.Type, dev.Rotational, dev.Empty)
		}

		availDevs, deviceErr := discover.GetAvailableDevices(c.context, n.Name, c.Namespace,
			n.Devices, n.Selection.DeviceFilter, n.Selection.GetUseAllDevices())

		if deviceErr != nil {
			// Devices were specified but we couldn't find any.
			// User needs to fix CRD.
			return devicesConfig, fmt.Errorf("failed to create DevicesConfig for node %s cluster %s: %v", n.Name, c.Namespace, deviceErr)
		}

		// Selects Disks from availDevs and translate to RTDevices
		availDisks := []sys.LocalDisk{}
		logger.Infof("[%s] selected devices: ", n.Name)
		for _, dev := range availDevs {
			for _, disk := range nodeDevices[n.Name] {
				if disk.Name == dev.Name {
					diskType := "SSD"
					if disk.Rotational {
						diskType = "HDD"
					}
					logger.Infof("\tName: %s, Type: %s, Size: %s", disk.Name, diskType, edgefsv1beta1.ByteCountBinary(disk.Size))
					availDisks = append(availDisks, disk)
				}
			}
		}

		rtDevices, err := target.GetContainersRTDevices(n.Name, c.Spec.MaxContainerCapacity.Value(), availDisks, &storeConfig)
		if err != nil {
			logger.Warningf("Can't get rtDevices for node %s due %v", n.Name, err)
			rtDevices = make([]edgefsv1beta1.RTDevices, 1)
		}
		if len(rtDevices) > 0 {
			devicesConfig.Rtrd.Devices = rtDevices[0].Devices
			// append to RtrdSlaves in case of additional containers
			if len(rtDevices) > 1 {
				devicesConfig.RtrdSlaves = make([]edgefsv1beta1.RTDevices, len(rtDevices)-1)
				devicesConfig.RtrdSlaves = rtDevices[1:]
			}
		}
	} else {
		devicesConfig.Rtlfs.Devices = target.GetRtlfsDevices(c.Spec.Storage.Directories, &storeConfig)
	}

	return devicesConfig, nil
}

// ValidateZones validates all nodes in cluster that each one has valid zone number or all of them has zone == 0
func validateZones(deploymentConfig edgefsv1beta1.ClusterDeploymentConfig) error {
	validZonesFound := 0
	for _, nodeDevConfig := range deploymentConfig.DevConfig {
		if nodeDevConfig.Zone > 0 {
			validZonesFound = validZonesFound + 1
		}
	}

	if validZonesFound > 0 && len(deploymentConfig.DevConfig) != validZonesFound {
		return fmt.Errorf("Valid Zone number must be propagated to all nodes")
	}

	return nil
}

func (c *cluster) validateDeploymentConfig(deploymentConfig edgefsv1beta1.ClusterDeploymentConfig, dro edgefsv1beta1.DevicesResurrectOptions) error {

	if len(deploymentConfig.TransportKey) == 0 || len(deploymentConfig.DeploymentType) == 0 {
		return fmt.Errorf("ClusterDeploymentConfig has no valid TransportKey or DeploymentType")
	}

	err := validateZones(deploymentConfig)
	if err != nil {
		return err
	}

	deploymentNodesCount := len(deploymentConfig.DevConfig)
	if deploymentConfig.TransportKey == edgefsv1beta1.DeploymentRtlfs {
		// Check directories devices count on all nodes
		if len(c.Spec.Storage.Directories)*deploymentNodesCount < 3 {
			return fmt.Errorf("Rtlfs devices should be more then 3 on all nodes summary")
		}
	} else if deploymentConfig.TransportKey == edgefsv1beta1.DeploymentRtrd {
		// Check all deployment nodes has available disk devices
		devicesCount := 0
		for nodeName, devCfg := range deploymentConfig.DevConfig {

			if devCfg.IsGatewayNode {
				continue
			}

			if len(devCfg.Rtrd.Devices) == 0 && !dro.NeedToResurrect {
				return fmt.Errorf("Node %s has no devices to deploy", nodeName)
			}
			devicesCount += devCfg.GetRtrdDeviceCount()
		}

		// Check new deployment devices count
		if !dro.NeedToResurrect && devicesCount < 3 {
			return fmt.Errorf("At least 3 empty disks required for Edgefs cluster. Those disks should be distributed over all nodes specified for [%s] EdgeFS cluster. Currently there are `%d` nodes cluster and `%d` disks only", c.Namespace, len(deploymentConfig.DevConfig), devicesCount)
		}
	}
	return nil
}

func (c *cluster) alignSlaveContainers(deploymentConfig *edgefsv1beta1.ClusterDeploymentConfig) error {

	nodeContainersCount := 0

	// Get Max containers count over cluster's nodes
	maxContainerCount := 0
	for _, nodeDevConfig := range deploymentConfig.DevConfig {
		// Skip GW node
		if nodeDevConfig.IsGatewayNode {
			continue
		}
		nodeContainersCount = len(nodeDevConfig.RtrdSlaves)
		if maxContainerCount < nodeContainersCount {
			maxContainerCount = nodeContainersCount
		}
	}

	for nodeName, nodeDevConfig := range deploymentConfig.DevConfig {

		// Skip GW node
		if nodeDevConfig.IsGatewayNode {
			continue
		}

		nodeContainersCount = len(nodeDevConfig.RtrdSlaves)
		stubContainersCount := maxContainerCount - nodeContainersCount

		for i := 0; i < stubContainersCount; i++ {
			nodeDevConfig.RtrdSlaves = append(nodeDevConfig.RtrdSlaves, edgefsv1beta1.RTDevices{})
		}

		// Update nodeDevConfig record in deploymentConfig
		if stubContainersCount > 0 {
			deploymentConfig.DevConfig[nodeName] = nodeDevConfig
		}
	}

	return nil
}
