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

// Package operator to manage Kubernetes storage.
package operator

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/coreos/pkg/capnslog"
	opkit "github.com/rook/operator-kit"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/agent/flexvolume"
	"github.com/rook/rook/pkg/daemon/ceph/agent/flexvolume/attachment"
	"github.com/rook/rook/pkg/operator/ceph/agent"
	"github.com/rook/rook/pkg/operator/ceph/cluster"
	"github.com/rook/rook/pkg/operator/ceph/file"
	"github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/pkg/operator/ceph/object/user"
	"github.com/rook/rook/pkg/operator/ceph/pool"
	"github.com/rook/rook/pkg/operator/ceph/provisioner"
	"github.com/rook/rook/pkg/operator/ceph/provisioner/controller"
	"github.com/rook/rook/pkg/operator/discover"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/api/core/v1"
)

// volume provisioner constant
const (
	provisionerName       = "ceph.rook.io/block"
	provisionerNameLegacy = "rook.io/block"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "operator")

// The supported configurations for the volume provisioner
var provisionerConfigs = map[string]string{
	provisionerName:       flexvolume.FlexvolumeVendor,
	provisionerNameLegacy: flexvolume.FlexvolumeVendorLegacy,
}

// Operator type for managing storage
type Operator struct {
	context         *clusterd.Context
	resources       []opkit.CustomResource
	rookImage       string
	securityAccount string
	// The custom resource that is global to the kubernetes cluster.
	// The cluster is global because you create multiple clusters in k8s
	clusterController *cluster.ClusterController
}

// New creates an operator instance
func New(context *clusterd.Context, volumeAttachmentWrapper attachment.Attachment, rookImage, securityAccount string) *Operator {
	clusterController := cluster.NewClusterController(context, rookImage, volumeAttachmentWrapper)

	schemes := []opkit.CustomResource{cluster.ClusterResource, pool.PoolResource, object.ObjectStoreResource, objectuser.ObjectStoreUserResource,
		file.FilesystemResource, attachment.VolumeResource}
	return &Operator{
		context:           context,
		clusterController: clusterController,
		resources:         schemes,
		rookImage:         rookImage,
		securityAccount:   securityAccount,
	}
}

// Run the operator instance
func (o *Operator) Run() error {

	namespace := os.Getenv(k8sutil.PodNamespaceEnvVar)
	if namespace == "" {
		return fmt.Errorf("Rook operator namespace is not provided. Expose it via downward API in the rook operator manifest file using environment variable %s", k8sutil.PodNamespaceEnvVar)
	}

	rookAgent := agent.New(o.context.Clientset)

	if err := rookAgent.Start(namespace, o.rookImage, o.securityAccount); err != nil {
		return fmt.Errorf("Error starting agent daemonset: %v", err)
	}

	rookDiscover := discover.New(o.context.Clientset)
	if err := rookDiscover.Start(namespace, o.rookImage, o.securityAccount); err != nil {
		return fmt.Errorf("Error starting device discovery daemonset: %v", err)
	}

	signalChan := make(chan os.Signal, 1)
	stopChan := make(chan struct{})
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	serverVersion, err := o.context.Clientset.Discovery().ServerVersion()
	if err != nil {
		return fmt.Errorf("Error getting server version: %v", err)
	}

	// Run volume provisioner for each of the supported configurations
	for name, vendor := range provisionerConfigs {
		volumeProvisioner := provisioner.New(o.context, vendor)
		pc := controller.NewProvisionController(
			o.context.Clientset,
			name,
			volumeProvisioner,
			serverVersion.GitVersion,
		)
		go pc.Run(stopChan)
		logger.Infof("rook-provisioner %s started using %s flex vendor dir", name, vendor)
	}

	// watch for changes to the rook clusters
	o.clusterController.StartWatch(v1.NamespaceAll, stopChan)

	for {
		select {
		case <-signalChan:
			logger.Infof("shutdown signal received, exiting...")
			close(stopChan)
			o.clusterController.StopWatch()
			return nil
		}
	}
}
