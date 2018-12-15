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

package target

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/coreos/pkg/capnslog"
	"github.com/golang/glog"
	edgefsv1alpha1 "github.com/rook/rook/pkg/apis/edgefs.rook.io/v1alpha1"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/discover"
	"github.com/rook/rook/pkg/operator/edgefs/cluster/target/config"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/sys"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clientset "k8s.io/client-go/kubernetes"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "edgefs-op-target")

const (
	appName                   = "rook-edgefs-target"
	targetAppNameFmt          = "rook-edgefs-target-id-%d"
	appNameFmt                = "rook-edgefs-target-%s"
	targetLabelKey            = "edgefs-target-id"
	defaultServiceAccountName = "rook-edgefs-cluster"
	unknownID                 = -1
	labelingRetries           = 5

	//deployment types
	deploymentRtlfs     = "rtlfs"
	deploymentRtrd      = "rtrd"
	deploymentAutoRtlfs = "autoRtlfs"
	nodeTypeLabelFmt    = "%s-nodetype"
)

// Cluster keeps track of the Targets
type Cluster struct {
	context          *clusterd.Context
	Namespace        string
	placement        rookalpha.Placement
	Version          string
	Storage          rookalpha.StorageScopeSpec
	dataDirHostPath  string
	dataVolumeSize   resource.Quantity
	HostNetworkSpec  edgefsv1alpha1.NetworkSpec
	Privileged       bool
	resources        v1.ResourceRequirements
	ownerRef         metav1.OwnerReference
	serviceAccount   string
	deploymentConfig ClusterDeploymentConfig
}

type ClusterDeploymentConfig struct {
	deploymentType string        //rtlfs, rtrd, autortlfs
	transportKey   string        //rtlfs or rtrd
	directories    []RtlfsDevice //cluster wide directories
	devConfig      map[string]DevicesConfig
	needPriviliges bool
}

type DevicesConfig struct {
	rtrd          RTDevices
	rtlfs         RtlfsDevices
	isGatewayNode bool
}

// New creates an instance of the Target manager
func New(
	context *clusterd.Context,
	namespace,
	version,
	serviceAccount string,
	storageSpec rookalpha.StorageScopeSpec,
	dataDirHostPath string,
	dataVolumeSize resource.Quantity,
	placement rookalpha.Placement,
	hostNetworkSpec edgefsv1alpha1.NetworkSpec,
	resources v1.ResourceRequirements,
	ownerRef metav1.OwnerReference,
) *Cluster {

	if serviceAccount == "" {
		// if the service account was not set, make a best effort with the example service account name since the default is unlikely to be sufficient.
		serviceAccount = defaultServiceAccountName
		logger.Infof("setting the target pods to use the service account name: %s", serviceAccount)
	}
	return &Cluster{
		context:          context,
		Namespace:        namespace,
		serviceAccount:   serviceAccount,
		placement:        placement,
		Version:          version,
		Storage:          storageSpec,
		dataDirHostPath:  dataDirHostPath,
		dataVolumeSize:   dataVolumeSize,
		HostNetworkSpec:  hostNetworkSpec,
		Privileged:       (isHostNetworkDefined(hostNetworkSpec) || os.Getenv("ROOK_HOSTPATH_REQUIRES_PRIVILEGED") == "true"),
		resources:        resources,
		ownerRef:         ownerRef,
		deploymentConfig: ClusterDeploymentConfig{devConfig: make(map[string]DevicesConfig, 0)},
	}
}

type DevicesResurrectOptions struct {
	needToResurrect bool
	needToZap       bool
	needToWait      bool
}

func ParseDevicesResurrectMode(resurrectMode string) DevicesResurrectOptions {
	drm := DevicesResurrectOptions{}
	if len(resurrectMode) == 0 {
		return drm
	}
	resurrectModeToLower := strings.ToLower(strings.TrimSpace(resurrectMode))
	switch resurrectModeToLower {
	case "restore":
		drm.needToResurrect = true
		break
	case "restorezap":
		drm.needToResurrect = true
		drm.needToZap = true
		break
	case "restorezapwait":
		drm.needToResurrect = true
		drm.needToZap = true
		drm.needToWait = true
		break
	}

	return drm
}

// Start the target management
func (c *Cluster) Start(rookImage, devicesResurrectMode string) (err error) {
	logger.Infof("start running targets in namespace %s", c.Namespace)

	logger.Infof("Target Image is %s", rookImage)
	if c.Storage.UseAllNodes == false && len(c.Storage.Nodes) == 0 {
		logger.Warningf("useAllNodes is set to false and no nodes are specified, no Edgefs pods are going to be created")
	}

	//logger.Infof("Cluster storage config %+v", c.Storage.Config)
	if c.Storage.UseAllNodes {
		// resolve all storage nodes
		c.Storage.Nodes = nil
		rookSystemNS := os.Getenv(k8sutil.PodNamespaceEnvVar)
		allNodeDevices, err := discover.ListDevices(c.context, rookSystemNS, "" /* all nodes */)
		//logger.Infof("allNodeDevices: %+v", allNodeDevices)
		if err != nil {
			logger.Warningf("failed to get storage nodes from namespace %s: %v", rookSystemNS, err)
			return err
		}
		for nodeName := range allNodeDevices {
			storageNode := rookalpha.Node{
				Name: nodeName,
			}
			c.Storage.Nodes = append(c.Storage.Nodes, storageNode)
		}
	}
	validNodes := k8sutil.GetValidNodes(c.Storage.Nodes, c.context.Clientset, c.placement)
	//logger.Infof("validNOdes: %+v", validNodes)
	logger.Infof("%d of the %d storage nodes are valid", len(validNodes), len(c.Storage.Nodes))
	c.Storage.Nodes = validNodes

	rmo := ParseDevicesResurrectMode(devicesResurrectMode)
	logger.Infof("DevicesResurrect mode: %s options %+v", devicesResurrectMode, rmo)

	err = c.createDeploymentConfig(rmo.needToResurrect)
	if err != nil {
		logger.Errorf("Failed to create deploymentConfig %+v", err)
		return err
	}

	logger.Infof("Deployment Config : %+v", c.deploymentConfig)

	if err := c.createSetupConfigs(rmo.needToResurrect); err != nil {
		logger.Errorf("Failed to create/update Edgefs cluster configuration: %+v", err)
		return err
	}

	headlessService, _ := c.makeHeadlessService()
	if _, err := c.context.Clientset.CoreV1().Services(c.Namespace).Create(headlessService); err != nil {
		if !errors.IsAlreadyExists(err) {
			return err
		}
		logger.Infof("headless service %s already exists in namespace %s", headlessService.Name, headlessService.Namespace)
	} else {
		logger.Infof("headless service %s started in namespace %s", headlessService.Name, headlessService.Namespace)
	}

	statefulSet, _ := c.makeStatefulSet(int32(len(validNodes)), rookImage, rmo)
	if _, err := c.context.Clientset.AppsV1beta1().StatefulSets(c.Namespace).Create(statefulSet); err != nil {
		if !errors.IsAlreadyExists(err) {
			return err
		}
		logger.Infof("stateful set %s already exists in namespace %s", statefulSet.Name, statefulSet.Namespace)
	} else {
		logger.Infof("stateful set %s created in namespace %s", statefulSet.Name, statefulSet.Namespace)
	}
	return nil
}

func (c *Cluster) HasDevicesSpecification() bool {

	if len(c.Storage.DeviceFilter) > 0 || len(c.Storage.Devices) > 0 {
		return true
	}

	for _, node := range c.Storage.Nodes {
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

func (c *Cluster) createDeploymentConfig(resurrect bool) error {

	//Fill deploymentConfig devices struct
	for _, node := range c.Storage.Nodes {
		n := c.resolveNode(node.Name)

		if n == nil {
			return fmt.Errorf("node %s did not resolve to start target", node.Name)
		}

		devicesConfig := DevicesConfig{}
		devicesConfig.rtrd.Devices = make([]RTDevice, 0)
		devicesConfig.rtlfs.Devices = make([]RtlfsDevice, 0)

		// if node labeled as gateway then return empty devises and skip RTDevices detection
		if c.isGatewayLabeledNode(c.context.Clientset, node.Name) {
			devicesConfig.isGatewayNode = true
			c.deploymentConfig.devConfig[node.Name] = devicesConfig
			continue
		}

		rookSystemNS := os.Getenv(k8sutil.PodNamespaceEnvVar)
		nodeDevices, _ := discover.ListDevices(c.context, rookSystemNS, n.Name)

		availDevs, deviceErr := discover.GetAvailableDevices(c.context, n.Name, c.Namespace,
			n.Devices, n.Selection.DeviceFilter, n.Selection.GetUseAllDevices())

		if deviceErr != nil {
			// Devices were specified but we couldn't find any.
			// User needs to fix CRD.
			return fmt.Errorf("failed to get devices for node %s cluster %s: %v",
				n.Name, c.Namespace, deviceErr)
		}

		// selects Disks from availDevs and translate to RTDevices
		availDisks := []sys.LocalDisk{}
		for _, dev := range availDevs {
			for _, disk := range nodeDevices[n.Name] {
				if disk.Name == dev.Name {
					availDisks = append(availDisks, disk)
				}
			}
		}

		storeConfig := config.ToStoreConfig(n.Config)
		logger.Infof("Storage config for node: %s is %+v", n.Name, storeConfig)
		rtDevices, err := GetRTDevices(availDisks, &storeConfig)
		if err != nil {
			logger.Warningf("Can't get rtDevices for node %s due %v", n.Name, err)
			rtDevices = make([]RTDevice, 0)
		}

		devicesConfig.rtrd.Devices = rtDevices
		devicesConfig.rtlfs.Devices = getRtlfsDevices(c.Storage.Directories)
		c.deploymentConfig.devConfig[node.Name] = devicesConfig
	}

	// Add Directories to deploymentConfig
	c.deploymentConfig.directories = getRtlfsDevices(c.Storage.Directories)

	if len(c.Storage.Directories) > 0 && (len(c.dataDirHostPath) > 0 || c.dataVolumeSize.Value() != 0) {
		c.deploymentConfig.deploymentType = deploymentRtlfs
		c.deploymentConfig.transportKey = "rtlfs"

		// check directories devices count on all nodes
		if len(c.Storage.Directories)*len(c.Storage.Nodes) < 3 {
			return fmt.Errorf("Rtlfs devices should be more then 3 on all nodes summary")
		}

	} else if c.HasDevicesSpecification() && (len(c.dataDirHostPath) > 0 || c.dataVolumeSize.Value() != 0) {

		// Check all deployment nodes has available disk devices
		devicesCount := 0
		for nodeName, devCfg := range c.deploymentConfig.devConfig {

			if devCfg.isGatewayNode {
				continue
			}

			if len(devCfg.rtrd.Devices) == 0 && !resurrect {
				return fmt.Errorf("Node %s has no available devices", nodeName)
			}
			devicesCount += len(devCfg.rtrd.Devices)
		}

		// check new deployment devices count
		if !resurrect && devicesCount < 3 {
			return fmt.Errorf("Disk devices should be more then 3 on all nodes summary")
		}

		c.deploymentConfig.deploymentType = deploymentRtrd
		c.deploymentConfig.transportKey = "rtrd"
		c.deploymentConfig.needPriviliges = true
	} else if len(c.dataDirHostPath) == 0 || c.dataVolumeSize.Value() == 0 {
		c.deploymentConfig.deploymentType = deploymentAutoRtlfs
		c.deploymentConfig.transportKey = "rtlfs"
	} else {
		return fmt.Errorf("Unknown deployment type! Cluster spec:\n %+v", c)
	}

	//set priviliges==true in case of HostNetwork
	if len(c.HostNetworkSpec.ServerIfName) > 0 || len(c.HostNetworkSpec.BrokerIfName) > 0 {
		c.deploymentConfig.needPriviliges = true
	}

	return nil
}

// creates a qualified name of the headless service for a given replica id and namespace,
// e.g., edgefs-0.edgefs.rook-edgefs
func createQualifiedHeadlessServiceName(replicaNum int, namespace string) string {
	return fmt.Sprintf("%s-%d.%s.%s", appName, replicaNum, appName, namespace)
}

func (c *Cluster) resolveNode(nodeName string) *rookalpha.Node {
	// fully resolve the storage config and resources for this node
	rookNode := c.Storage.ResolveNode(nodeName)
	if rookNode == nil {
		return nil
	}

	// Apply directories from ClusterStorageSpec only
	rookNode.Directories = c.Storage.Directories

	rookNode.Resources = k8sutil.MergeResourceRequirements(rookNode.Resources, c.resources)

	// ensure no invalid dirs are specified
	var validDirs []rookalpha.Directory
	for _, dir := range rookNode.Directories {
		if dir.Path == k8sutil.DataDir || dir.Path == c.dataDirHostPath {
			logger.Warningf("skipping directory %s that would conflict with the dataDirHostPath", dir.Path)
			continue
		}
		validDirs = append(validDirs, dir)
	}
	rookNode.Directories = validDirs

	return rookNode
}

func (c *Cluster) AddLabelsToNode(cs clientset.Interface, nodeName string, labels map[string]string) error {
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
func (c *Cluster) RemoveLabelOffNode(cs clientset.Interface, nodeName string, labelKeys []string) error {
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
				glog.V(2).Infof("Conflict when trying to remove a labels %v from %v", labelKeys, nodeName)
			}
		} else {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	return err
}

func (c *Cluster) isGatewayLabeledNode(cs clientset.Interface, nodeName string) bool {
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

func (c *Cluster) getNodeLabels(cs clientset.Interface, nodeName string) (map[string]string, error) {
	node, err := cs.CoreV1().Nodes().Get(nodeName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	if node.Labels == nil {
		return node.Labels, nil
	}
	return node.Labels, nil
}
