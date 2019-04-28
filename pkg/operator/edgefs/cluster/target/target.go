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

package target

import (
	"os"

	"github.com/coreos/pkg/capnslog"
	edgefsv1beta1 "github.com/rook/rook/pkg/apis/edgefs.rook.io/v1beta1"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	annotations      rookalpha.Annotations
	placement        rookalpha.Placement
	Version          string
	Storage          rookalpha.StorageScopeSpec
	dataDirHostPath  string
	dataVolumeSize   resource.Quantity
	HostNetworkSpec  edgefsv1beta1.NetworkSpec
	Privileged       bool
	resources        v1.ResourceRequirements
	resourceProfile  string
	chunkCacheSize   resource.Quantity
	ownerRef         metav1.OwnerReference
	serviceAccount   string
	deploymentConfig edgefsv1beta1.ClusterDeploymentConfig
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
	annotations rookalpha.Annotations,
	placement rookalpha.Placement,
	hostNetworkSpec edgefsv1beta1.NetworkSpec,
	resources v1.ResourceRequirements,
	resourceProfile string,
	chunkCacheSize resource.Quantity,
	ownerRef metav1.OwnerReference,
	deploymentConfig edgefsv1beta1.ClusterDeploymentConfig,
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
		annotations:      annotations,
		placement:        placement,
		Version:          version,
		Storage:          storageSpec,
		dataDirHostPath:  dataDirHostPath,
		dataVolumeSize:   dataVolumeSize,
		HostNetworkSpec:  hostNetworkSpec,
		Privileged:       (isHostNetworkDefined(hostNetworkSpec) || os.Getenv("ROOK_HOSTPATH_REQUIRES_PRIVILEGED") == "true"),
		resources:        resources,
		resourceProfile:  resourceProfile,
		chunkCacheSize:   chunkCacheSize,
		ownerRef:         ownerRef,
		deploymentConfig: deploymentConfig,
	}
}

// Start the target management
func (c *Cluster) Start(rookImage string, nodes []rookalpha.Node, dro edgefsv1beta1.DevicesResurrectOptions) (err error) {
	logger.Infof("start running targets in namespace %s", c.Namespace)

	logger.Infof("Target Image is %s", rookImage)

	headlessService, _ := c.makeHeadlessService()
	if _, err := c.context.Clientset.CoreV1().Services(c.Namespace).Create(headlessService); err != nil {
		if !errors.IsAlreadyExists(err) {
			return err
		}
		logger.Infof("headless service %s already exists in namespace %s", headlessService.Name, headlessService.Namespace)
	} else {
		logger.Infof("headless service %s started in namespace %s", headlessService.Name, headlessService.Namespace)
	}

	statefulSet, _ := c.makeStatefulSet(int32(len(nodes)), rookImage, dro)
	if _, err := c.context.Clientset.AppsV1().StatefulSets(c.Namespace).Create(statefulSet); err != nil {
		if !errors.IsAlreadyExists(err) {
			return err
		}
		logger.Infof("stateful set %s already exists in namespace %s", statefulSet.Name, statefulSet.Namespace)
	} else {
		logger.Infof("stateful set %s created in namespace %s", statefulSet.Name, statefulSet.Namespace)
	}
	return nil
}
