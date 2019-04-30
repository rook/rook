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
	"strconv"
	"strings"
	"time"

	edgefsv1beta1 "github.com/rook/rook/pkg/apis/edgefs.rook.io/v1beta1"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/operator/discover"
	"github.com/rook/rook/pkg/operator/edgefs/cluster/target"
	"github.com/rook/rook/pkg/operator/edgefs/cluster/target/config"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/sys"
	"k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clientset "k8s.io/client-go/kubernetes"
)

const (
	labelingRetries  = 5
	nodeTypeLabelFmt = "%s-nodetype"
)

// ParseDevicesResurrectMode parse resurrect options string. String format "restore|restorezap|restorezapwait:<SlaveContainersCount>":
func ParseDevicesResurrectMode(resurrectMode string) edgefsv1beta1.DevicesResurrectOptions {
	drm := edgefsv1beta1.DevicesResurrectOptions{}
	if len(resurrectMode) == 0 {
		return drm
	}

	resurrectModeParts := strings.Split(resurrectMode, ":")
	if len(resurrectModeParts) > 1 {
		cntCount, err := strconv.Atoi(strings.TrimSpace(resurrectModeParts[1]))
		if err == nil {
			drm.SlaveContainers = cntCount
		}
	}

	resurrectModeToLower := strings.ToLower(strings.TrimSpace(resurrectModeParts[0]))

	switch resurrectModeToLower {
	case "restore":
		drm.NeedToResurrect = true
		break
	case "restorezap":
		drm.NeedToResurrect = true
		drm.NeedToZap = true
		break
	case "restorezapwait":
		drm.NeedToResurrect = true
		drm.NeedToZap = true
		drm.NeedToWait = true
		break
	}

	return drm
}

func (c *cluster) getClusterNodes() ([]rookalpha.Node, error) {
	if c.Spec.Storage.UseAllNodes {
		c.Spec.Storage.Nodes = nil
		// Resolve all storage nodes
		rookSystemNS := os.Getenv(k8sutil.PodNamespaceEnvVar)
		allNodeDevices, err := discover.ListDevices(c.context, rookSystemNS, "" /* all nodes */)
		if err != nil {
			logger.Warningf("failed to get storage nodes from namespace %s: %v", rookSystemNS, err)
			return nil, err
		}
		for nodeName := range allNodeDevices {
			storageNode := rookalpha.Node{
				Name: nodeName,
			}
			c.Spec.Storage.Nodes = append(c.Spec.Storage.Nodes, storageNode)
		}
	}
	validNodes := k8sutil.GetValidNodes(c.Spec.Storage.Nodes, c.context.Clientset, edgefsv1beta1.GetTargetPlacement(c.Spec.Placement))
	c.Spec.Storage.Nodes = validNodes
	return validNodes, nil
}

func (c *cluster) createDeploymentConfig(nodes []rookalpha.Node, resurrect bool) (edgefsv1beta1.ClusterDeploymentConfig, error) {
	deploymentConfig := edgefsv1beta1.ClusterDeploymentConfig{DevConfig: make(map[string]edgefsv1beta1.DevicesConfig, 0)}
	// Fill deploymentConfig devices struct
	for _, node := range nodes {
		n := c.resolveNode(node.Name)
		storeConfig := config.ToStoreConfig(n.Config)

		if n == nil {
			return deploymentConfig, fmt.Errorf("node %s did not resolve to start target", node.Name)
		}

		devicesConfig := edgefsv1beta1.DevicesConfig{}
		devicesConfig.Rtrd.Devices = make([]edgefsv1beta1.RTDevice, 0)
		devicesConfig.Rtlfs.Devices = make([]edgefsv1beta1.RtlfsDevice, 0)

		// Apply Node's zone value
		devicesConfig.Zone = storeConfig.Zone

		// If node labeled as gateway then return empty devices and skip RTDevices detection
		if c.isGatewayLabeledNode(c.context.Clientset, node.Name) {
			devicesConfig.IsGatewayNode = true
			deploymentConfig.DevConfig[node.Name] = devicesConfig
			continue
		}

		rookSystemNS := os.Getenv(k8sutil.PodNamespaceEnvVar)
		nodeDevices, _ := discover.ListDevices(c.context, rookSystemNS, n.Name)

		availDevs, deviceErr := discover.GetAvailableDevices(c.context, n.Name, c.Namespace,
			n.Devices, n.Selection.DeviceFilter, n.Selection.GetUseAllDevices())

		if deviceErr != nil {
			// Devices were specified but we couldn't find any.
			// User needs to fix CRD.
			return deploymentConfig, fmt.Errorf("failed to get devices for node %s cluster %s: %v",
				n.Name, c.Namespace, deviceErr)
		}

		// Selects Disks from availDevs and translate to RTDevices
		availDisks := []sys.LocalDisk{}
		for _, dev := range availDevs {
			for _, disk := range nodeDevices[n.Name] {
				if disk.Name == dev.Name {
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
		devicesConfig.Rtlfs.Devices = target.GetRtlfsDevices(c.Spec.Storage.Directories, &storeConfig)
		deploymentConfig.DevConfig[node.Name] = devicesConfig
	}

	err := ValidateSlaveContainers(&deploymentConfig)
	if err != nil {
		return deploymentConfig, err
	}

	err = ValidateZones(&deploymentConfig)
	if err != nil {
		return deploymentConfig, err
	}
	// Add Directories to deploymentConfig
	clusterStorageConfig := config.ToStoreConfig(c.Spec.Storage.Config)
	deploymentConfig.Directories = target.GetRtlfsDevices(c.Spec.Storage.Directories, &clusterStorageConfig)

	if len(c.Spec.Storage.Directories) > 0 && (len(c.Spec.DataDirHostPath) > 0 || c.Spec.DataVolumeSize.Value() != 0) {
		deploymentConfig.DeploymentType = edgefsv1beta1.DeploymentRtlfs
		deploymentConfig.TransportKey = "rtlfs"

		// Check directories devices count on all nodes
		if len(c.Spec.Storage.Directories)*len(nodes) < 3 {
			return deploymentConfig, fmt.Errorf("Rtlfs devices should be more then 3 on all nodes summary")
		}

	} else if c.HasDevicesSpecification() && (len(c.Spec.DataDirHostPath) > 0 || c.Spec.DataVolumeSize.Value() != 0) {

		// Check all deployment nodes has available disk devices
		devicesCount := 0
		for nodeName, devCfg := range deploymentConfig.DevConfig {

			if devCfg.IsGatewayNode {
				continue
			}

			if len(devCfg.Rtrd.Devices) == 0 && !resurrect {
				return deploymentConfig, fmt.Errorf("Node %s has no available devices", nodeName)
			}
			devicesCount += len(devCfg.Rtrd.Devices)
		}

		// Check new deployment devices count
		if !resurrect && devicesCount < 3 {
			return deploymentConfig, fmt.Errorf("Disk devices should be more then 3 on all nodes summary")
		}

		deploymentConfig.DeploymentType = edgefsv1beta1.DeploymentRtrd
		deploymentConfig.TransportKey = "rtrd"
		deploymentConfig.NeedPrivileges = true
	} else if len(c.Spec.DataDirHostPath) == 0 || c.Spec.DataVolumeSize.Value() == 0 {
		deploymentConfig.DeploymentType = edgefsv1beta1.DeploymentAutoRtlfs
		deploymentConfig.TransportKey = "rtlfs"
	} else {
		return deploymentConfig, fmt.Errorf("Unknown deployment type! Cluster spec:\n %+v", c)
	}

	// Set privileges==true in case of HostNetwork
	if len(c.Spec.Network.ServerIfName) > 0 || len(c.Spec.Network.BrokerIfName) > 0 {
		deploymentConfig.NeedPrivileges = true
	}

	return deploymentConfig, nil
}

// ValidateSlaveContainers validates containers count for each deployment node, container's count MUST be equal for for each node
func ValidateSlaveContainers(deploymentConfig *edgefsv1beta1.ClusterDeploymentConfig) error {

	isFirstNode := true
	prevNodeContainersCount := 0
	nodeContainersCount := 0
	for nodeName, nodeDevConfig := range deploymentConfig.DevConfig {
		// Skip GW node
		if nodeDevConfig.IsGatewayNode {
			continue
		}

		nodeContainersCount = len(nodeDevConfig.RtrdSlaves)
		if isFirstNode {
			prevNodeContainersCount = nodeContainersCount
			isFirstNode = false
		}
		if nodeContainersCount != prevNodeContainersCount {
			return fmt.Errorf("Node [%s] has different containers count %d then others nodes %d", nodeName, nodeContainersCount, prevNodeContainersCount)
		}
	}
	return nil
}

// ValidateZones validates all nodes in cluster that each one has valid zone number or all of them has zone == 0
func ValidateZones(deploymentConfig *edgefsv1beta1.ClusterDeploymentConfig) error {
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

func (c *cluster) resolveNode(nodeName string) *rookalpha.Node {
	// Fully resolve the storage config and resources for this node
	rookNode := c.Spec.Storage.ResolveNode(nodeName)
	if rookNode == nil {
		return nil
	}

	// Apply directories from ClusterStorageSpec only
	rookNode.Directories = c.Spec.Storage.Directories

	rookNode.Resources = k8sutil.MergeResourceRequirements(rookNode.Resources, c.Spec.Resources)

	// Ensure no invalid dirs are specified
	var validDirs []rookalpha.Directory
	for _, dir := range rookNode.Directories {
		if dir.Path == k8sutil.DataDir || dir.Path == c.Spec.DataDirHostPath {
			logger.Warningf("skipping directory %s that would conflict with the dataDirHostPath", dir.Path)
			continue
		}
		validDirs = append(validDirs, dir)
	}
	rookNode.Directories = validDirs

	return rookNode
}

func (c *cluster) AddLabelsToNode(cs clientset.Interface, nodeName string, labels map[string]string) error {
	tokens := make([]string, 0, len(labels))
	for k, v := range labels {
		tokens = append(tokens, "\""+k+"\":\""+v+"\"")
	}
	labelString := "{" + strings.Join(tokens, ",") + "}"
	patch := fmt.Sprintf(`{"metadata":{"labels":%v}}`, labelString)
	var err error
	for attempt := 0; attempt < labelingRetries; attempt++ {
		_, err = cs.CoreV1().Nodes().Patch(nodeName, types.MergePatchType, []byte(patch))
		if err != nil {
			if !apierrs.IsConflict(err) {
				return err
			}
		} else {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	return err
}

// RemoveLabelOffNode is for cleaning up labels temporarily added to node,
// won't fail if target label doesn't exist or has been removed.
func (c *cluster) RemoveLabelOffNode(cs clientset.Interface, nodeName string, labelKeys []string) error {
	var node *v1.Node
	var err error
	for attempt := 0; attempt < labelingRetries; attempt++ {
		node, err = cs.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if node.Labels == nil {
			return nil
		}
		for _, labelKey := range labelKeys {
			if node.Labels == nil || len(node.Labels[labelKey]) == 0 {
				break
			}
			delete(node.Labels, labelKey)
		}
		_, err = cs.CoreV1().Nodes().Update(node)
		if err != nil {
			if !apierrs.IsConflict(err) {
				return err
			} else {
				logger.Warningf("Conflict when trying to remove a labels %v from %v", labelKeys, nodeName)
			}
		} else {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	return err
}

func (c *cluster) isGatewayLabeledNode(cs clientset.Interface, nodeName string) bool {
	labelMap, err := c.getNodeLabels(cs, nodeName)
	if err != nil || labelMap == nil {
		return false
	}

	if nodeType, ok := labelMap[fmt.Sprintf(nodeTypeLabelFmt, c.Namespace)]; ok {
		if nodeType == "gateway" {
			return true
		}
	}

	return false
}

func (c *cluster) getNodeLabels(cs clientset.Interface, nodeName string) (map[string]string, error) {
	node, err := cs.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	if node.Labels == nil {
		return node.Labels, nil
	}
	return node.Labels, nil
}

func (c *cluster) HasDevicesSpecification() bool {

	if len(c.Spec.Storage.DeviceFilter) > 0 || len(c.Spec.Storage.Devices) > 0 {
		return true
	}

	for _, node := range c.Spec.Storage.Nodes {
		useAllDevices := node.UseAllDevices
		if useAllDevices != nil && *useAllDevices {
			return true
		}

		if len(node.DeviceFilter) > 0 || len(node.Devices) > 0 {
			return true
		}
	}

	return false
}
