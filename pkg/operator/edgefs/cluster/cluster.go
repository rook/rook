/*
Copyright 2018 The Rook Authors. All rights reserved.

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

	edgefsv1alpha1 "github.com/rook/rook/pkg/apis/edgefs.rook.io/v1alpha1"
	rookv1alpha2 "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/edgefs/cluster/mgr"
	"github.com/rook/rook/pkg/operator/edgefs/cluster/target"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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
	Spec      edgefsv1alpha1.ClusterSpec
	ownerRef  metav1.OwnerReference
	targets   *target.Cluster
	mgrs      *mgr.Cluster
	stopCh    chan struct{}
}

func newCluster(c *edgefsv1alpha1.Cluster, context *clusterd.Context) *cluster {

	return &cluster{
		context:   context,
		Namespace: c.Namespace,
		Spec:      c.Spec,
		stopCh:    make(chan struct{}),
		ownerRef:  ClusterOwnerRef(c.Namespace, string(c.UID)),
	}
}

func (c *cluster) createInstance(rookImage string) error {

	logger.Infof("Cluster spec\n %+v", c.Spec)

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
	k8sutil.SetOwnerRef(c.context.Clientset, c.Namespace, &cm.ObjectMeta, &c.ownerRef)
	_, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Create(cm)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create override configmap %s. %+v", c.Namespace, err)
	}

	//
	// Create and start EdgeFS Targets StatefulSet
	//
	c.targets = target.New(c.context, c.Namespace, "latest", c.Spec.ServiceAccount, c.Spec.Storage, c.Spec.DataDirHostPath, c.Spec.DataVolumeSize,
		edgefsv1alpha1.GetTargetPlacement(c.Spec.Placement), c.Spec.Network, c.Spec.Resources, c.ownerRef)

	err = c.targets.Start(rookImage, c.Spec.DevicesResurrectMode)
	if err != nil {
		return fmt.Errorf("failed to start the targets. %+v", err)
	}

	//
	// Create and start EdgeFS manager Deployment (gRPC proxy, Prometheus metrics)
	//
	c.mgrs = mgr.New(c.context, c.Namespace, "latest", c.Spec.ServiceAccount, c.Spec.DataDirHostPath, c.Spec.DataVolumeSize,
		edgefsv1alpha1.GetMgrPlacement(c.Spec.Placement), c.Spec.Network,
		v1.ResourceRequirements{}, c.ownerRef)
	err = c.mgrs.Start(rookImage)
	if err != nil {
		return fmt.Errorf("failed to start the edgefs mgr. %+v", err)
	}

	logger.Infof("Done creating rook instance in namespace %s", c.Namespace)
	return nil
}

func (c *cluster) validateClusterSpec() error {
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

	logger.Info("Validate cluster spec")
	return nil
}

func clusterChanged(oldCluster, newCluster edgefsv1alpha1.ClusterSpec) bool {
	changeFound := false
	oldStorage := oldCluster.Storage
	newStorage := newCluster.Storage

	// sort the nodes by name then compare to see if there are changes
	sort.Sort(rookv1alpha2.NodesByName(oldStorage.Nodes))
	sort.Sort(rookv1alpha2.NodesByName(newStorage.Nodes))
	if !reflect.DeepEqual(oldStorage.Nodes, newStorage.Nodes) {
		logger.Infof("The list of nodes has changed")
		changeFound = true
	}

	return changeFound
}
