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

	edgefsv1beta1 "github.com/rook/rook/pkg/apis/edgefs.rook.io/v1beta1"
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
	context   *clusterd.Context
	Namespace string
	Spec      edgefsv1beta1.ClusterSpec
	ownerRef  metav1.OwnerReference
	targets   *target.Cluster
	mgrs      *mgr.Cluster
	stopCh    chan struct{}
}

func newCluster(c *edgefsv1beta1.Cluster, context *clusterd.Context) *cluster {

	return &cluster{
		context:   context,
		Namespace: c.Namespace,
		Spec:      c.Spec,
		stopCh:    make(chan struct{}),
		ownerRef:  ClusterOwnerRef(c.Namespace, string(c.UID)),
	}
}

func (c *cluster) createInstance(rookImage string) error {

	logger.Debugf("Cluster spec: %+v", c.Spec)
	//
	// Validate Cluster CRD
	//
	if err := c.validateClusterSpec(); err != nil {
		logger.Errorf("invalid cluster spec: %+v", err)
		return err
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
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create override configmap %s. %+v", c.Namespace, err)
	}

	clusterNodes, err := c.getClusterNodes()
	if err != nil {
		return fmt.Errorf("failed to get nodes for cluster %s. %s", c.Namespace, err)
	}

	dro := ParseDevicesResurrectMode(c.Spec.DevicesResurrectMode)
	logger.Infof("DevicesResurrect mode: %s options %+v", c.Spec.DevicesResurrectMode, dro)

	deploymentConfig, err := c.createDeploymentConfig(clusterNodes, dro.NeedToResurrect)
	if err != nil {
		logger.Errorf("Failed to create deploymentConfig %+v", err)
		return err
	}
	logger.Debugf("DeploymentConfig: %+v ", deploymentConfig)

	if err := c.createClusterConfigMap(clusterNodes, deploymentConfig, dro.NeedToResurrect); err != nil {
		logger.Errorf("Failed to create/update Edgefs cluster configuration: %+v", err)
		return err
	}
	//
	// Create and start EdgeFS prepare job (set some networking parameters that we cannot set via InitContainers)
	// Skip preparation job in case of resurrect option is on
	//

	if c.Spec.SkipHostPrepare == false && dro.NeedToResurrect == false {
		err = c.prepareHostNodes(rookImage, deploymentConfig)
		if err != nil {
			logger.Errorf("Failed to create preparation jobs. %+v", err)
		}
	} else {
		logger.Infof("EdgeFS node preparation will be skipped due skipHostPrepare=true or resurrect cluster option")
	}

	//
	// Create and start EdgeFS Targets StatefulSet
	//

	c.targets = target.New(c.context, c.Namespace, "latest", c.Spec.ServiceAccount, c.Spec.Storage, c.Spec.DataDirHostPath, c.Spec.DataVolumeSize,
		edgefsv1beta1.GetTargetAnnotations(c.Spec.Annotations), edgefsv1beta1.GetTargetPlacement(c.Spec.Placement), c.Spec.Network,
		c.Spec.Resources, c.Spec.ResourceProfile, c.Spec.ChunkCacheSize, c.ownerRef, deploymentConfig)

	err = c.targets.Start(rookImage, clusterNodes, dro)
	if err != nil {
		return fmt.Errorf("failed to start the targets. %+v", err)
	}

	//
	// Create and start EdgeFS manager Deployment (gRPC proxy, Prometheus metrics)
	//
	c.mgrs = mgr.New(c.context, c.Namespace, "latest", c.Spec.ServiceAccount, c.Spec.DataDirHostPath, c.Spec.DataVolumeSize,
		edgefsv1beta1.GetMgrAnnotations(c.Spec.Annotations), edgefsv1beta1.GetMgrPlacement(c.Spec.Placement), c.Spec.Network, c.Spec.Dashboard,
		v1.ResourceRequirements{}, c.Spec.ResourceProfile, c.ownerRef)
	err = c.mgrs.Start(rookImage)
	if err != nil {
		return fmt.Errorf("failed to start the edgefs mgr. %+v", err)
	}

	logger.Infof("Done creating rook instance in namespace %s", c.Namespace)
	return nil
}

func (c *cluster) prepareHostNodes(rookImage string, deploymentConfig edgefsv1beta1.ClusterDeploymentConfig) error {

	prep := prepare.New(c.context, c.Namespace, "latest", c.Spec.ServiceAccount,
		edgefsv1beta1.GetPrepareAnnotations(c.Spec.Annotations), edgefsv1beta1.GetPreparePlacement(c.Spec.Placement), v1.ResourceRequirements{}, c.ownerRef)

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
		return fmt.Errorf("DataDirHostPath or DataVolumeSize EdgeFS cluster's options not specified.")
	}

	if len(c.Spec.DataDirHostPath) > 0 && c.Spec.DataVolumeSize.Value() != 0 {
		return fmt.Errorf("Both deployment options DataDirHostPath and DataVolumeSize are specified. Should be only one deployment option in cluster specification.")
	}

	if len(c.Spec.Storage.Directories) > 0 &&
		((c.Spec.Storage.UseAllDevices != nil && *c.Spec.Storage.UseAllDevices) ||
			len(c.Spec.Storage.Devices) > 0 || len(c.Spec.Storage.DeviceFilter) > 0) {
		return fmt.Errorf("Directories option specified as well as Devices. Remove Directories or Devices option from cluster specification")
	}

	if c.Spec.TrlogProcessingInterval > 0 && (60%c.Spec.TrlogProcessingInterval) != 0 {
		return fmt.Errorf("Incorrect trlogProcessingInterval specified")
	}

	logger.Info("Validate cluster spec")
	return nil
}

func clusterChanged(oldCluster, newCluster edgefsv1beta1.ClusterSpec) bool {
	changeFound := false
	oldStorage := oldCluster.Storage
	newStorage := newCluster.Storage

	// Sort the nodes by name then compare to see if there are changes
	sort.Sort(rookv1alpha2.NodesByName(oldStorage.Nodes))
	sort.Sort(rookv1alpha2.NodesByName(newStorage.Nodes))
	if !reflect.DeepEqual(oldStorage.Nodes, newStorage.Nodes) {
		logger.Infof("The list of nodes has changed")
		changeFound = true
	}

	return changeFound
}
