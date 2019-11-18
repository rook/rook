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
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	edgefsv1 "github.com/rook/rook/pkg/apis/edgefs.rook.io/v1"
	rookv1alpha2 "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/operator/discover"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
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
func ParseDevicesResurrectMode(resurrectMode string) edgefsv1.DevicesResurrectOptions {
	drm := edgefsv1.DevicesResurrectOptions{}
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

func ToJSON(obj interface{}) string {
	bytes, err := json.Marshal(obj)
	if err != nil {
		logger.Errorf("JSON conversion failed: %+v", err)
		return ""
	}

	return string(bytes)
}

func (c *cluster) getClusterNodes() ([]rookv1alpha2.Node, error) {
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
			storageNode := rookv1alpha2.Node{
				Name: nodeName,
			}
			c.Spec.Storage.Nodes = append(c.Spec.Storage.Nodes, storageNode)
		}
		logger.Warningf("UseAllNodes prevents future cluster changes! Specify nodes to deploy via `nodes:` collection.")
	}
	validNodes := k8sutil.GetValidNodes(c.Spec.Storage, c.context.Clientset, edgefsv1.GetTargetPlacement(c.Spec.Placement))
	c.Spec.Storage.Nodes = validNodes
	return validNodes, nil
}

// retrieveDeploymentConfig restore ClusterDeploymentConfig from cluster's Kubernetes ConfigMap
func (c *cluster) retrieveDeploymentConfig() (edgefsv1.ClusterDeploymentConfig, error) {

	deploymentConfig := edgefsv1.ClusterDeploymentConfig{
		DevConfig: make(map[string]edgefsv1.DevicesConfig, 0),
	}

	cm, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Get(configName, metav1.GetOptions{})
	if err != nil {
		if apierrs.IsNotFound(err) {
			// When cluster config map doesn't exist, return config with empty DevicesConfig and current DeploymentType
			deploymentType, err := c.getClusterDeploymentType()
			if err != nil {
				return deploymentConfig, err
			}

			deploymentConfig.DeploymentType = deploymentType
			deploymentConfig.TransportKey = getClusterTransportKey(deploymentType)
			return deploymentConfig, nil
		}
		return deploymentConfig, err
	}

	setup := map[string]edgefsv1.SetupNode{}
	if nesetup, ok := cm.Data["nesetup"]; ok {
		err = json.Unmarshal([]byte(nesetup), &setup)
		if err != nil {
			logger.Errorf("invalid JSON in cluster configmap. %+v", err)
			return deploymentConfig, fmt.Errorf("invalid JSON in cluster configmap. %+v", err)
		}

		deploymentTypeAchived := false
		for nodeKey, nodeConfig := range setup {
			devicesConfig := edgefsv1.DevicesConfig{}

			devicesConfig.Rtlfs = nodeConfig.Rtlfs
			devicesConfig.Rtrd = nodeConfig.Rtrd
			devicesConfig.RtrdSlaves = nodeConfig.RtrdSlaves

			devicesConfig.IsGatewayNode = false
			if nodeConfig.NodeType == "gateway" {
				devicesConfig.IsGatewayNode = true
			}
			devicesConfig.Zone = nodeConfig.Ccowd.Zone
			deploymentConfig.DevConfig[nodeKey] = devicesConfig

			// we can't detect deployment type on gw node, move to next one
			if !devicesConfig.IsGatewayNode && !deploymentTypeAchived {
				if len(nodeConfig.Rtkvs.Devices) > 0 {
					deploymentConfig.DeploymentType = edgefsv1.DeploymentRtkvs
					deploymentConfig.TransportKey = edgefsv1.DeploymentRtkvs
				} else if len(nodeConfig.Rtrd.Devices) > 0 {
					deploymentConfig.DeploymentType = edgefsv1.DeploymentRtrd
					deploymentConfig.TransportKey = edgefsv1.DeploymentRtrd
					deploymentConfig.NeedPrivileges = true
				} else if len(nodeConfig.Rtlfs.Devices) > 0 {
					deploymentConfig.DeploymentType = edgefsv1.DeploymentRtlfs
					deploymentConfig.TransportKey = edgefsv1.DeploymentRtlfs
				} else if len(nodeConfig.RtlfsAutodetect) > 0 {
					deploymentConfig.DeploymentType = edgefsv1.DeploymentAutoRtlfs
					deploymentConfig.TransportKey = edgefsv1.DeploymentRtlfs
				}

				// hostNetwork option specified
				if nodeConfig.Ccowd.Network.ServerInterfaces != "eth0" {
					deploymentConfig.NeedPrivileges = true
				}
				deploymentTypeAchived = true
			}
		}

		if deploymentConfig.DeploymentType == "" || deploymentConfig.TransportKey == "" {
			return deploymentConfig, fmt.Errorf("Can't retrieve DeploymentConfig from config map. Unknown DeploymentType or TransportKey values")
		}
	}

	// Set privileges==true in case of HostNetwork
	if c.Spec.Network.IsHost() {
		deploymentConfig.NeedPrivileges = true
	}

	return deploymentConfig, nil
}

func (c *cluster) PrintRTDevices(containerIndex int, rtDevices edgefsv1.RTDevices) {

	if len(rtDevices.Devices) == 0 {
		logger.Infof("\t\tContainer[%d] Stub container. No devices assigned", containerIndex)
		return
	}

	for _, device := range rtDevices.Devices {
		logger.Infof("\t\tContainer[%d] Device: %s, Name: %s, Journal: %s", containerIndex, device.Device, device.Name, device.Journal)
	}
}

func (c *cluster) PrintRtlfsDevices(containerIndex int, rtlfsDevices edgefsv1.RtlfsDevices) {

	for _, device := range rtlfsDevices.Devices {
		logger.Infof("\t\tContainer[%d] Path: %s, Name: %s, MaxSize: %s", containerIndex, device.Path, device.Name, edgefsv1.ByteCountBinary(device.Maxsize))
	}
}

func (c *cluster) PrintRtkvsDevices(containerIndex int, rtkvsDevices edgefsv1.RtkvsDevices) {
	for _, device := range rtkvsDevices.Devices {
		logger.Infof("\t\tContainer[%d] Path: %s, Name: %s, Backend: %s, JournalPath: %s, JournalMaxSize: %s,"+
			" plevelOverride: %d, sync: %d, wal_disabled: %d",
			containerIndex, device.Path, device.Name, rtkvsDevices.Backend, device.JornalPath,
			edgefsv1.ByteCountBinary(device.JournalMaxsize), device.PlevelOverride, device.Sync, device.WalDisabled)
	}
}

func (c *cluster) PrintDeploymentConfig(deploymentConfig *edgefsv1.ClusterDeploymentConfig) {
	logger.Infof("[%s] DeploymentConfig: ", c.Namespace)
	logger.Infof("DeploymentType: %s", deploymentConfig.DeploymentType)
	logger.Infof("TransportKey: %s", deploymentConfig.TransportKey)
	logger.Infof("Directories: %+v", deploymentConfig.Directories)
	logger.Infof("NeedPrivileges: %t", deploymentConfig.NeedPrivileges)
	for nodeName, nodeDevConfig := range deploymentConfig.DevConfig {
		logger.Infof("\tNode [%s] devices:", nodeName)

		if nodeDevConfig.IsGatewayNode {
			logger.Infof("\t\tContainer[0] Configured as Edgefs gateway. No devices selected")
			continue
		}

		switch deploymentConfig.DeploymentType {
		case edgefsv1.DeploymentRtrd:
			c.PrintRTDevices(0, nodeDevConfig.Rtrd)
			for index, slaveDevices := range nodeDevConfig.RtrdSlaves {
				c.PrintRTDevices(index+1, slaveDevices)
			}
		case edgefsv1.DeploymentRtlfs:
			c.PrintRtlfsDevices(0, nodeDevConfig.Rtlfs)
		case edgefsv1.DeploymentRtkvs:
			c.PrintRtkvsDevices(0, nodeDevConfig.Rtkvs)
		case edgefsv1.DeploymentAutoRtlfs:
			logger.Infof("\t\tContainer[0] Path: /mnt/disks/disk0")
			logger.Infof("\t\tContainer[0] Path: /mnt/disks/disk1")
			logger.Infof("\t\tContainer[0] Path: /mnt/disks/disk2")
			logger.Infof("\t\tContainer[0] Path: /mnt/disks/disk3")
		default:
			logger.Errorf("[%s] Unknown DeploymentType '%s'", c.Namespace, deploymentConfig.DeploymentType)
		}
	}
}

func (c *cluster) resolveNode(nodeName string) *rookv1alpha2.Node {
	// Fully resolve the storage config and resources for this node
	rookNode := c.Spec.Storage.ResolveNode(nodeName)
	if rookNode == nil {
		return nil
	}

	// Apply directories from ClusterStorageSpec only
	rookNode.Directories = c.Spec.Storage.Directories

	rookNode.Resources = k8sutil.MergeResourceRequirements(rookNode.Resources, c.Spec.Resources)

	// Ensure no invalid dirs are specified
	var validDirs []rookv1alpha2.Directory
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

func (c *cluster) LabelTargetNode(nodeName string) {
	c.AddLabelsToNode(nodeName, map[string]string{c.Namespace: "cluster"})
}

func (c *cluster) AddLabelsToNode(nodeName string, labels map[string]string) error {
	tokens := make([]string, 0, len(labels))
	for k, v := range labels {
		tokens = append(tokens, "\""+k+"\":\""+v+"\"")
	}
	labelString := "{" + strings.Join(tokens, ",") + "}"
	patch := fmt.Sprintf(`{"metadata":{"labels":%v}}`, labelString)
	var err error
	for attempt := 0; attempt < labelingRetries; attempt++ {
		_, err = c.context.Clientset.CoreV1().Nodes().Patch(nodeName, types.MergePatchType, []byte(patch))
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

func (c *cluster) UnlabelTargetNode(nodeName string) {
	c.RemoveLabelOffNode(nodeName, []string{c.Namespace})
}

// RemoveLabelOffNode is for cleaning up labels temporarily added to node,
// won't fail if target label doesn't exist or has been removed.
func (c *cluster) RemoveLabelOffNode(nodeName string, labelKeys []string) error {
	var node *v1.Node
	var err error
	for attempt := 0; attempt < labelingRetries; attempt++ {
		node, err = c.context.Clientset.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
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
		_, err = c.context.Clientset.CoreV1().Nodes().Update(node)
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

	if c.Spec.Storage.UseAllDevices != nil && *c.Spec.Storage.UseAllDevices {
		return true
	}

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
