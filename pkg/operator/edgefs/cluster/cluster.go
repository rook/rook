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
	"reflect"
	"sort"

	"github.com/google/go-cmp/cmp"
	edgefsv1 "github.com/rook/rook/pkg/apis/edgefs.rook.io/v1"
	rookv1alpha2 "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/edgefs/cluster/mgr"
	"github.com/rook/rook/pkg/operator/edgefs/cluster/prepare"
	"github.com/rook/rook/pkg/operator/edgefs/cluster/target"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	httpPortDefault           = int32(8080)
	httpPortName              = "http"
	grpcPortDefault           = int32(6789)
	grpcPortName              = "grpc"
	udpTotemPortDefault       = int32(5405)
	udpTotemPortName          = "totem"
	volumeNameDataDir         = "datadir"
	defaultServiceAccountName = "rook-edgefs-cluster"

	/* Volumes definitions */
	configName = "edgefs-config"
)

type cluster struct {
	context          *clusterd.Context
	Namespace        string
	Spec             edgefsv1.ClusterSpec
	ownerRef         metav1.OwnerReference
	targets          *target.Cluster
	mgrs             *mgr.Cluster
	stopCh           chan struct{}
	childControllers []childController
}

func newCluster(c *edgefsv1.Cluster, context *clusterd.Context) *cluster {

	return &cluster{
		context:   context,
		Namespace: c.Namespace,
		Spec:      c.Spec,
		stopCh:    make(chan struct{}),
		ownerRef:  ClusterOwnerRef(c.Name, string(c.UID)),
	}
}

// ChildController is implemented by CRs that are owned by the EdgefsCluster
type childController interface {
	// ParentClusterChanged is called when the EdgefsCluster CR is updated, for example for a newer edgefs version
	ParentClusterChanged(cluster edgefsv1.ClusterSpec)
}

// createInstance returns done [true - no need to polling status, false continue polling]
func (c *cluster) createInstance(rookImage string, isClusterUpdate bool) (bool, error) {

	logger.Debugf("Cluster [%s] spec: %+v", c.Namespace, c.Spec)

	// Validate Cluster CRD
	//
	if err := c.validateClusterSpec(); err != nil {
		logger.Errorf("Invalid cluster [%s] spec. Error: %+v", c.Namespace, err)
		return false, err
	}
	// Create a configmap for overriding edgefs config settings
	// These settings should only be modified by a user after they are initialized
	placeholderConfig := map[string]string{
		k8sutil.ConfigOverrideVal: "",
	}
	cm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: k8sutil.ConfigOverrideName,
		},
		Data: placeholderConfig,
	}
	k8sutil.SetOwnerRef(&cm.ObjectMeta, &c.ownerRef)
	_, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Create(cm)
	if err != nil {
		if errors.IsAlreadyExists(err) {
			// Cluster already exists, do not do anything
			if !isClusterUpdate {
				logger.Infof("Cluster [%s] already exists. Skipping creation...", c.Namespace)
				return true, nil
			}
			// in case of update just skip checking
		} else {
			return true, fmt.Errorf("Failed to create override configmap %s. %+v", c.Namespace, err)
		}
	}

	// copy original Cluster nodes spec. c.getClusterNodes mutates available nodes specification
	originalSpecNodes := c.Spec.Storage.Nodes
	clusterNodes, err := c.getClusterNodes()
	if err != nil {
		return true, fmt.Errorf("Failed to get nodes for cluster [%s]. Error: %s", c.Namespace, err)
	}

	// check Spec.Storage.Nodes for availability, if not return error and stop deployment/update
	for _, specNode := range originalSpecNodes {
		isSpecNodeValid := false
		specNodeName := specNode.Name
		for _, validNode := range clusterNodes {
			if specNodeName == validNode.Name {
				isSpecNodeValid = true
				break
			}
		}

		if !isSpecNodeValid {
			return true, fmt.Errorf("Node '%s' is NOT valid. Check node status.", specNodeName)
		}
	}

	dro := ParseDevicesResurrectMode(c.Spec.DevicesResurrectMode)
	logger.Infof("DevicesResurrect options: %+v", dro)

	// Retrive existing cluster config from Kubernetes ConfigMap
	existingConfig, err := c.retrieveDeploymentConfig()
	if err != nil {
		return true, fmt.Errorf("Failed to retrive DeploymentConfig for cluster [%s]. Error: %s", c.Namespace, err)
	}

	clusterReconfiguration, err := c.createClusterReconfigurationSpec(existingConfig, clusterNodes, dro)
	if err != nil {
		if isClusterUpdate {
			return true, fmt.Errorf("Failed to update [%s] EdgeFS cluster configuration. Error: %s", c.Namespace, err)
		} else {
			return true, fmt.Errorf("Failed to create [%s] EdgeFS cluster configuration. Error: %s", c.Namespace, err)
		}
	}

	logger.Debugf("Recovered ClusterConfig: %s", ToJSON(existingConfig))
	c.PrintDeploymentConfig(&clusterReconfiguration.DeploymentConfig)

	// Unlabel nodes
	for _, nodeName := range clusterReconfiguration.ClusterNodesToDelete {
		//c.UnlabelNode(node)
		logger.Infof("Unlabeling host `%s` as [%s] cluster's target node", nodeName, c.Namespace)
		c.UnlabelTargetNode(nodeName)
	}

	for _, nodeName := range clusterReconfiguration.ClusterNodesToAdd {
		//c.LabelNode(node)
		logger.Infof("Labeling host `%s` as [%s] cluster's target node", nodeName, c.Namespace)
		c.LabelTargetNode(nodeName)
	}

	if err := c.createClusterConfigMap(clusterReconfiguration.DeploymentConfig, dro.NeedToResurrect); err != nil {
		logger.Errorf("Failed to create/update Edgefs [%s] cluster configuration: %+v", c.Namespace, err)
		return true, err
	}

	//
	// Create and start EdgeFS prepare job (set some networking parameters that we cannot set via InitContainers)
	// Skip preparation job in case of resurrect option is on
	//

	if c.Spec.SkipHostPrepare == false && dro.NeedToResurrect == false {
		err = c.prepareHostNodes(rookImage, clusterReconfiguration.DeploymentConfig)
		if err != nil {
			logger.Errorf("Failed to create [%s] cluster preparation jobs. %+v", c.Namespace, err)
		}
	} else {
		logger.Infof("EdgeFS node preparation will be skipped due skipHostPrepare=true or resurrect cluster option")
	}

	if err := c.createClusterConfigMap(clusterReconfiguration.DeploymentConfig, dro.NeedToResurrect); err != nil {
		logger.Errorf("Failed to create/update [%s] Edgefs cluster configuration: %+v", c.Namespace, err)
		return true, err
	}

	//
	// Create and start EdgeFS Targets StatefulSet
	//

	// Do not update targets when clusterUpdate and restore option set.
	// Because we can't recover information from 'restored' cluster's config map and deploymentConfig is incorrect
	// Rest of deployments should be updated as is
	if !(isClusterUpdate && dro.NeedToResurrect) {
		c.targets = target.New(c.context, c.Namespace, "latest", c.Spec.ServiceAccount, c.Spec.Storage, c.Spec.DataDirHostPath, c.Spec.DataVolumeSize,
			edgefsv1.GetTargetAnnotations(c.Spec.Annotations), edgefsv1.GetTargetPlacement(c.Spec.Placement), c.Spec.Network,
			c.Spec.Resources, c.Spec.ResourceProfile, c.Spec.ChunkCacheSize, c.ownerRef, clusterReconfiguration.DeploymentConfig, c.Spec.UseHostLocalTime)

		err = c.targets.Start(rookImage, clusterNodes, dro)
		if err != nil {
			return false, fmt.Errorf("Failed to start the targets of [%s] cluster. %+v", c.Namespace, err)
		}

	}
	//
	// Create and start EdgeFS manager Deployment (gRPC proxy, Prometheus metrics)
	//
	c.mgrs = mgr.New(c.context, c.Namespace, "latest", c.Spec.ServiceAccount, c.Spec.DataDirHostPath, c.Spec.DataVolumeSize,
		edgefsv1.GetMgrAnnotations(c.Spec.Annotations), edgefsv1.GetMgrPlacement(c.Spec.Placement), c.Spec.Network, c.Spec.Dashboard,
		v1.ResourceRequirements{}, c.Spec.ResourceProfile, c.ownerRef, c.Spec.UseHostLocalTime)

	err = c.mgrs.Start(rookImage)
	if err != nil {
		return false, fmt.Errorf("failed to start the [%s] edgefs mgr. %+v", c.Namespace, err)
	}

	logger.Infof("Done creating [%s] Edgefs cluster instance", c.Namespace)

	// Notify the child controllers that the cluster spec might have changed
	for _, child := range c.childControllers {
		child.ParentClusterChanged(c.Spec)
	}

	return true, nil
}

func (c *cluster) prepareHostNodes(rookImage string, deploymentConfig edgefsv1.ClusterDeploymentConfig) error {

	prep := prepare.New(c.context, c.Namespace, "latest", c.Spec.ServiceAccount,
		edgefsv1.GetPrepareAnnotations(c.Spec.Annotations), edgefsv1.GetPreparePlacement(c.Spec.Placement), v1.ResourceRequirements{}, c.ownerRef)

	for nodeName, devicesConfig := range deploymentConfig.DevConfig {

		logger.Debugf("HostNodePreparation %s devConfig: %+v", nodeName, devicesConfig)
		err := prep.Start(rookImage, nodeName)
		if err != nil {
			return fmt.Errorf("failed to start the edgefs preparation on node %s . %+v", nodeName, err)
		}
	}
	return nil
}

func (c *cluster) validateClusterSpec() error {

	if c.Spec.ResourceProfile != "" && c.Spec.ResourceProfile != "embedded" && c.Spec.ResourceProfile != "performance" {
		return fmt.Errorf("Unrecognized resource profile '%s'", c.Spec.ResourceProfile)
	}

	rMemReq := c.Spec.Resources.Requests.Memory()
	rMemLim := c.Spec.Resources.Limits.Memory()

	// performance profile mins
	memReq := "2048Mi"
	memLim := "8192Mi"

	// Auto adjust to embedded if not specifically asked and less then mins
	if c.Spec.ResourceProfile == "" {
		if !rMemReq.IsZero() && rMemReq.Cmp(resource.MustParse(memReq)) < 0 {
			c.Spec.ResourceProfile = "embedded"
			logger.Infof("adjusting target resourceProfile to embedded due to specified memReq %v less then %s", rMemReq, memReq)
		}
		if !rMemLim.IsZero() && rMemLim.Cmp(resource.MustParse(memLim)) < 0 {
			c.Spec.ResourceProfile = "embedded"
			logger.Infof("adjusting target resourceProfile to embedded due to specified memLim %v less then %s", rMemLim, memLim)
		}
	}

	if c.Spec.ResourceProfile == "embedded" {
		memReq = "256Mi"
		memLim = "1024Mi"
	}

	if !rMemReq.IsZero() && rMemReq.Cmp(resource.MustParse(memReq)) < 0 {
		return fmt.Errorf("memory resource request %v is less then minimally allowed %s", rMemReq, memReq)
	}

	if !rMemLim.IsZero() && rMemLim.Cmp(resource.MustParse(memLim)) < 0 {
		return fmt.Errorf("memory resource limit %v is less then minimally allowed %s", rMemLim, memLim)
	}

	if len(c.Spec.DataDirHostPath) == 0 && c.Spec.DataVolumeSize.Value() == 0 {
		return fmt.Errorf("DataDirHostPath or DataVolumeSize EdgeFS cluster's options not specified")
	}

	if len(c.Spec.DataDirHostPath) > 0 && c.Spec.DataVolumeSize.Value() != 0 {
		return fmt.Errorf("Both deployment options DataDirHostPath and DataVolumeSize are specified. Should be only one deployment option in cluster specification")
	}

	if len(c.Spec.Storage.Directories) > 0 &&
		((c.Spec.Storage.UseAllDevices != nil && *c.Spec.Storage.UseAllDevices) ||
			len(c.Spec.Storage.Devices) > 0 || len(c.Spec.Storage.DeviceFilter) > 0) {
		return fmt.Errorf("Directories option specified as well as Devices. Remove Directories or Devices option from cluster specification")
	}

	if c.Spec.TrlogProcessingInterval > 0 && (60%c.Spec.TrlogProcessingInterval) != 0 {
		return fmt.Errorf("Incorrect trlogProcessingInterval specified")
	}

	return nil
}

func clusterChanged(oldCluster, newCluster edgefsv1.ClusterSpec) bool {
	// sort the nodes by name then compare to see if there are changes
	sort.Sort(rookv1alpha2.NodesByName(oldCluster.Storage.Nodes))
	sort.Sort(rookv1alpha2.NodesByName(newCluster.Storage.Nodes))

	var diff string
	// any change in the crd will trigger an orchestration
	if !reflect.DeepEqual(oldCluster, newCluster) {
		func() {
			defer func() {
				if err := recover(); err != nil {
					logger.Warningf("Encountered an issue getting cluster change differences: %v", err)
				}
			}()

			// resource.Quantity has non-exportable fields, so we use its comparator method
			resourceQtyComparer := cmp.Comparer(func(x, y resource.Quantity) bool { return x.Cmp(y) == 0 })
			diff = cmp.Diff(oldCluster, newCluster, resourceQtyComparer)
			logger.Infof("The Cluster CR has changed. diff=%s", diff)
		}()
	}

	if len(diff) > 0 {
		return true
	}

	return false
}
