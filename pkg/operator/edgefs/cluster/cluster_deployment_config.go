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
	"strings"

	edgefsv1 "github.com/rook/rook/pkg/apis/edgefs.rook.io/v1"
	rookv1alpha2 "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/operator/discover"
	"github.com/rook/rook/pkg/operator/edgefs/cluster/target"
	"github.com/rook/rook/pkg/operator/edgefs/cluster/target/config"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/sys"
)

type ClusterReconfigureSpec struct {
	DeploymentConfig     edgefsv1.ClusterDeploymentConfig
	ClusterNodesToDelete []string
	ClusterNodesToAdd    []string
}

func (c *cluster) createClusterReconfigurationSpec(existingConfig edgefsv1.ClusterDeploymentConfig, validNodes []rookv1alpha2.Node, dro edgefsv1.DevicesResurrectOptions) (ClusterReconfigureSpec, error) {

	deploymentType, err := c.getClusterDeploymentType()
	if err != nil {
		return ClusterReconfigureSpec{}, err
	}

	logger.Debugf("ClusterSpec: %s", ToJSON(c.Spec))

	transportKey := getClusterTransportKey(deploymentType)
	reconfigSpec := ClusterReconfigureSpec{
		DeploymentConfig: edgefsv1.ClusterDeploymentConfig{
			DeploymentType: deploymentType,
			TransportKey:   transportKey,
			DevConfig:      make(map[string]edgefsv1.DevicesConfig, 0),
			NeedPrivileges: c.needPrivelege(deploymentType),
		},
	}

	// In case of Storage.UseAllNodes we should make additional availability test due validNodes will not contains NotReady, Drained nodes
	if c.Spec.Storage.UseAllNodes {
		for specNodeName := range existingConfig.DevConfig {
			isSpecNodeValid := false
			for _, validNode := range validNodes {
				if specNodeName == validNode.Name {
					isSpecNodeValid = true
					break
				}
			}

			if !isSpecNodeValid {
				return ClusterReconfigureSpec{}, fmt.Errorf("Node '%s' is NOT valid. Check node status.", specNodeName)
			}
		}
	}

	// Iterate over available cluster nodes
	for _, node := range validNodes {
		// Copy devices confiruration for already existing devicesConfig
		// We can't modify devices config for already existing node in config map
		var devicesConfig edgefsv1.DevicesConfig
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
	logger.Debugf("NodesToDelete: %#v", reconfigSpec.ClusterNodesToDelete)
	// Calculate nodes to add to cluster
	reconfigSpec.ClusterNodesToAdd = reconfigSpec.DeploymentConfig.NodesDifference(existingConfig)
	logger.Debugf("NodesToAdd: %#v", reconfigSpec.ClusterNodesToAdd)

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

	storeConfig := config.ToStoreConfig(c.Spec.Storage.Config)
	if len(storeConfig.UseRtkvsBackend) > 0 {
		logger.Warningf("Using rtkvs")
		return edgefsv1.DeploymentRtkvs, nil
	} else if len(c.Spec.Storage.Directories) > 0 && (len(c.Spec.DataDirHostPath) > 0 || c.Spec.DataVolumeSize.Value() != 0) {
		return edgefsv1.DeploymentRtlfs, nil
	} else if c.HasDevicesSpecification() && (len(c.Spec.DataDirHostPath) > 0 || c.Spec.DataVolumeSize.Value() != 0) {
		return edgefsv1.DeploymentRtrd, nil
	} else if len(c.Spec.DataDirHostPath) == 0 || c.Spec.DataVolumeSize.Value() == 0 {
		return edgefsv1.DeploymentAutoRtlfs, nil
	}
	return "", fmt.Errorf("Can't determine deployment type for [%s] cluster", c.Namespace)
}

func (c *cluster) getClusterSysRepCount() int {
	if c.Spec.SystemReplicationCount > 0 {
		return c.Spec.SystemReplicationCount
	}
	return 3
}

func (c *cluster) getClusterFailureDomain() (string, error) {
	failureDomain := "host"
	if len(c.Spec.FailureDomain) > 0 {
		switch strings.ToLower(c.Spec.FailureDomain) {
		case "device":
			failureDomain = "device"
		case "host":
			failureDomain = "host"
		case "zone":
			failureDomain = "zone"
		default:
			return "", fmt.Errorf("Unknow failure domain %s, skipped", c.Spec.FailureDomain)
		}
	}
	return failureDomain, nil
}

func (c *cluster) needPrivelege(deploymentType string) bool {

	needPriveleges := false
	// Set privileges==true in case of HostNetwork
	if c.Spec.Network.IsHost() {
		needPriveleges = true
	}

	if deploymentType == edgefsv1.DeploymentRtrd || deploymentType == edgefsv1.DeploymentRtkvs {
		needPriveleges = true
	}
	return needPriveleges
}

func getClusterTransportKey(deploymentType string) string {
	transportKey := edgefsv1.DeploymentRtrd
	if deploymentType == edgefsv1.DeploymentRtkvs {
		transportKey = edgefsv1.DeploymentRtkvs
	} else if deploymentType == edgefsv1.DeploymentRtlfs || deploymentType == edgefsv1.DeploymentAutoRtlfs {
		transportKey = edgefsv1.DeploymentRtlfs
	}
	return transportKey
}

// createDevicesConfig creates DevicesConfig for specific node
func (c *cluster) createDevicesConfig(deploymentType string, node rookv1alpha2.Node, dro edgefsv1.DevicesResurrectOptions) (edgefsv1.DevicesConfig, error) {
	devicesConfig := edgefsv1.DevicesConfig{}
	devicesConfig.Rtrd.Devices = make([]edgefsv1.RTDevice, 0)
	devicesConfig.RtrdSlaves = make([]edgefsv1.RTDevices, 0)
	devicesConfig.Rtlfs.Devices = make([]edgefsv1.RtlfsDevice, 0)
	devicesConfig.Rtkvs.Devices = make([]edgefsv1.RtkvsDevice, 0)

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
			devicesConfig.RtrdSlaves = make([]edgefsv1.RTDevices, dro.SlaveContainers)
		}

		return devicesConfig, nil
	}

	if deploymentType == edgefsv1.DeploymentRtkvs {
		rookSystemNS := os.Getenv(k8sutil.PodNamespaceEnvVar)
		nodeDevices, _ := discover.ListDevices(c.context, rookSystemNS, n.Name)
		logger.Infof("[%s] available devices: ", n.Name)
		for _, dev := range nodeDevices[n.Name] {
			logger.Infof("\tName: %s, Size: %s, Type: %s, Rotational: %t, Empty: %t", dev.Name, edgefsv1.ByteCountBinary(dev.Size), dev.Type, dev.Rotational, dev.Empty)
		}
		disks := []string{}
		if storeConfig.UseRtkvsBackend == "kvssd" {
			// For KVSSD backend make sure the user selected devices are attached
			// TODO: make sure disks are truly KVSSD devices
			availDevs, deviceErr := discover.GetAvailableDevices(c.context, n.Name, c.Namespace,
				n.Devices, n.Selection.DeviceFilter, n.Selection.GetUseAllDevices())

			if deviceErr != nil {
				// Devices were specified but we couldn't find any.
				// User needs to fix CRD.
				return devicesConfig, fmt.Errorf("failed to create DevicesConfig for node %s cluster %s: %v", n.Name, c.Namespace, deviceErr)
			}

			if len(availDevs) == 0 {
				// For the rtkvs backed user has to explicitly specify NVME devices
				return devicesConfig, fmt.Errorf("failed to create DevicesConfig for node %s cluster %s: no NVME (KVSSD) devices in CRD", n.Name, c.Namespace)
			}
			// Make sure selected disks exist
			for _, dev := range availDevs {
				for _, disk := range nodeDevices[n.Name] {
					if disk.Name == dev.Name {
						fullPath := dev.FullPath
						if len(fullPath) == 0 {
							fullPath = "/dev/disk/by-id/" + target.GetIdDevLinkName(disk.DevLinks)
						}
						disks = append(disks, fullPath)
						logger.Infof("\t%s: using %s (%s) as a KVSSD drive", n.Name, fullPath, dev.Name)
					}
				}
			}
			if len(disks) == 0 {
				return devicesConfig, fmt.Errorf("failed to create DevicesConfig for node %s cluster %s: "+
					"none of specified KVSSD devices were detected", n.Name, c.Namespace)
			}
		} else {
			return devicesConfig, fmt.Errorf("failed to create DevicesConfig for node %s cluster %s:"+
				" rtkvs backend %s isn't supported", n.Name, c.Namespace, storeConfig.UseRtkvsBackend)
		}

		// storage.directories need to contain a mountpoint(s) for rtkvs journal/metadata
		// The user can specify single or several mountpoints. In the former case all VDEVs will share the same mountpoint,
		// in the latter each VDEV will get its own.
		// NOTE: directories are scooped at a cluster level and cannot be overridden on a per-node basis.

		if len(c.Spec.Storage.Directories) == 0 {
			return devicesConfig, fmt.Errorf("failed to create DevicesConfig for node %s cluster %s: "+
				"couldn't find KVSSD journals mountpoints in the CRD. Use storage.directories for this purpose",
				n.Name, c.Namespace)
		}
		devicesConfig.Rtkvs = target.GetRtkvsDevices(disks, c.Spec.Storage.Directories, &storeConfig)

	} else if deploymentType == edgefsv1.DeploymentRtrd {
		rookSystemNS := os.Getenv(k8sutil.PodNamespaceEnvVar)
		nodeDevices, _ := discover.ListDevices(c.context, rookSystemNS, n.Name)

		logger.Infof("[%s] available devices: ", n.Name)
		for _, dev := range nodeDevices[n.Name] {
			logger.Infof("\tName: %s, Size: %s, Type: %s, Rotational: %t, Empty: %t", dev.Name, edgefsv1.ByteCountBinary(dev.Size), dev.Type, dev.Rotational, dev.Empty)
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
					logger.Infof("\tName: %s, Type: %s, Size: %s", disk.Name, diskType, edgefsv1.ByteCountBinary(disk.Size))
					availDisks = append(availDisks, disk)
				}
			}
		}

		rtDevices, err := target.GetContainersRTDevices(n.Name, c.Spec.MaxContainerCapacity.Value(), availDisks, &storeConfig)
		if err != nil {
			logger.Warningf("Can't get rtDevices for node %s due %v", n.Name, err)
			rtDevices = make([]edgefsv1.RTDevices, 1)
		}
		if len(rtDevices) > 0 {
			devicesConfig.Rtrd.Devices = rtDevices[0].Devices
			// append to RtrdSlaves in case of additional containers
			if len(rtDevices) > 1 {
				devicesConfig.RtrdSlaves = make([]edgefsv1.RTDevices, len(rtDevices)-1)
				devicesConfig.RtrdSlaves = rtDevices[1:]
			}
		}
	} else {
		devicesConfig.Rtlfs.Devices = target.GetRtlfsDevices(c.Spec.Storage.Directories, &storeConfig)
	}

	return devicesConfig, nil
}

// ValidateZones validates all nodes in cluster that each one has valid zone number or all of them has zone == 0
func validateZones(deploymentConfig edgefsv1.ClusterDeploymentConfig, sysRepCount int, failureDomain string) error {
	validZonesFound := 0
	for _, nodeDevConfig := range deploymentConfig.DevConfig {
		if nodeDevConfig.Zone > 0 {
			validZonesFound = validZonesFound + 1
		}
	}

	if validZonesFound > 0 && len(deploymentConfig.DevConfig) != validZonesFound {
		return fmt.Errorf("Valid Zone number must be propagated to all nodes")
	}

	if failureDomain == "zone" {
		if validZonesFound < sysRepCount {
			return fmt.Errorf("Assigned cluster zones count =%d should be greater or equal then cluster.Spec.SysRepCount = %d", validZonesFound, sysRepCount)
		}
	}

	return nil
}

func (c *cluster) validateDeploymentConfig(deploymentConfig edgefsv1.ClusterDeploymentConfig, dro edgefsv1.DevicesResurrectOptions) error {

	if len(deploymentConfig.TransportKey) == 0 || len(deploymentConfig.DeploymentType) == 0 {
		return fmt.Errorf("ClusterDeploymentConfig has no valid TransportKey or DeploymentType")
	}

	sysRepCount := c.getClusterSysRepCount()
	failureDomain, err := c.getClusterFailureDomain()
	if err != nil {
		return err
	}

	err = validateZones(deploymentConfig, sysRepCount, failureDomain)
	if err != nil {
		return err
	}

	if deploymentConfig.TransportKey == edgefsv1.DeploymentRtkvs {
		if len(c.Spec.Storage.Directories) == 0 {
			return fmt.Errorf("RTKVS configuration error: storage.directory has to " +
				"contain at least one host's directory. It will be used to store certain metadata types.")
		}
		if c.Spec.Storage.UseAllNodes || (c.Spec.Storage.UseAllDevices != nil && *c.Spec.Storage.UseAllDevices) {
			return fmt.Errorf("RTKVS configuration error: storage.useAllNodes and storage.useAllDevices options aren't allowed." +
				" A per-node NVME KVSSD disks list is expected")
		}

		switch failureDomain {
		case "device":
			rtkvsDevices := deploymentConfig.GetRtkvsDevicesCount()
			if rtkvsDevices < sysRepCount {
				return fmt.Errorf("Rtkvs devices should be greater or equal then SysRepCount=%d on all nodes summary", sysRepCount)
			}
		case "host":
			targets := deploymentConfig.GetTargetsCount()
			if targets < sysRepCount {
				return fmt.Errorf("Rtkvs targets=(%d) should be greater or equal then SysRepCount=%d on all nodes summary", targets, sysRepCount)
			}
		}
	} else if deploymentConfig.TransportKey == edgefsv1.DeploymentRtlfs {
		// Check directories devices count on all nodes for autoRtlfs mode
		switch failureDomain {
		case "device":
			deploymentNodesCount := len(deploymentConfig.DevConfig)
			if len(c.Spec.Storage.Directories) > 0 {
				if len(c.Spec.Storage.Directories)*deploymentNodesCount < sysRepCount {
					return fmt.Errorf("Rtlfs devices should be greater or equal then SysRepCount=%d on all nodes summary", sysRepCount)
				}
			} else { // Autoftlfs case: 4 folders per node
				if (4 * deploymentNodesCount) < sysRepCount {
					return fmt.Errorf("AutoRtlfs devices should be greater or equal then SysRepCount=%d on all nodes summary", sysRepCount)
				}
			}

		case "host":
			targets := deploymentConfig.GetTargetsCount()
			if targets < sysRepCount {
				return fmt.Errorf("Rtlfs containers=(%d) should be greater or equal then SysRepCount=%d on all nodes summary", targets, sysRepCount)
			}
		}
	} else if deploymentConfig.TransportKey == edgefsv1.DeploymentRtrd {
		for nodeName, devCfg := range deploymentConfig.DevConfig {
			if devCfg.IsGatewayNode {
				continue
			}
			if len(devCfg.Rtrd.Devices) == 0 && !dro.NeedToResurrect {
				return fmt.Errorf("Node %s has no devices to deploy", nodeName)
			}
		}

		switch failureDomain {
		case "device":
			rtrdDevicesCount := deploymentConfig.GetRtrdDevicesCount()
			if rtrdDevicesCount < sysRepCount {
				return fmt.Errorf("Rtrd devices=(%d) should be greater or equal then SysRepCount=%d on all nodes summary", rtrdDevicesCount, sysRepCount)
			}
		case "host":
			containers := deploymentConfig.GetRtrdContainersCount()
			if containers < sysRepCount {
				return fmt.Errorf("Rtrd containers=(%d) should be greater or equal then SysRepCount=%d on all nodes summary", containers, sysRepCount)
			}
		}
	}
	return nil
}

func (c *cluster) alignSlaveContainers(deploymentConfig *edgefsv1.ClusterDeploymentConfig) error {

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
			nodeDevConfig.RtrdSlaves = append(nodeDevConfig.RtrdSlaves, edgefsv1.RTDevices{})
		}

		// Update nodeDevConfig record in deploymentConfig
		if stubContainersCount > 0 {
			deploymentConfig.DevConfig[nodeName] = nodeDevConfig
		}
	}

	return nil
}
