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
	"github.com/rook/rook/pkg/operator/ceph/csi"
	"github.com/rook/rook/pkg/operator/ceph/file"
	"github.com/rook/rook/pkg/operator/ceph/object"
	objectuser "github.com/rook/rook/pkg/operator/ceph/object/user"
	"github.com/rook/rook/pkg/operator/ceph/pool"
	"github.com/rook/rook/pkg/operator/ceph/provisioner"
	"github.com/rook/rook/pkg/operator/discover"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/sig-storage-lib-external-provisioner/controller"
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

var (
	// Whether to enable the flex driver. If true, the rook-ceph-agent daemonset will be started.
	EnableFlexDriver = true
	// Whether to enable the daemon for device discovery. If true, the rook-ceph-discover daemonset will be started.
	EnableDiscoveryDaemon = true
)

// Operator type for managing storage
type Operator struct {
	context         *clusterd.Context
	resources       []opkit.CustomResource
	rookImage       string
	securityAccount string
	// The custom resource that is global to the kubernetes cluster.
	// The cluster is global because you create multiple clusters in k8s
	clusterController     *cluster.ClusterController
	delayedDaemonsStarted bool
}

// New creates an operator instance
func New(context *clusterd.Context, volumeAttachmentWrapper attachment.Attachment, rookImage, securityAccount string) *Operator {
	schemes := []opkit.CustomResource{cluster.ClusterResource, pool.PoolResource, object.ObjectStoreResource, objectuser.ObjectStoreUserResource,
		file.FilesystemResource, attachment.VolumeResource}
	o := &Operator{
		context:         context,
		resources:       schemes,
		rookImage:       rookImage,
		securityAccount: securityAccount,
	}
	o.clusterController = cluster.NewClusterController(context, rookImage, volumeAttachmentWrapper, o.startSystemDaemons)
	return o
}

// Run the operator instance
func (o *Operator) Run() error {

	namespace := os.Getenv(k8sutil.PodNamespaceEnvVar)
	if namespace == "" {
		return fmt.Errorf("Rook operator namespace is not provided. Expose it via downward API in the rook operator manifest file using environment variable %s", k8sutil.PodNamespaceEnvVar)
	}

	if EnableDiscoveryDaemon {
		rookDiscover := discover.New(o.context.Clientset)
		if err := rookDiscover.Start(namespace, o.rookImage, o.securityAccount); err != nil {
			return fmt.Errorf("error starting device discovery daemonset. %+v", err)
		}
	}

	serverVersion, err := o.context.Clientset.Discovery().ServerVersion()
	if err != nil {
		return fmt.Errorf("error getting server version. %+v", err)
	}

	signalChan := make(chan os.Signal, 1)
	stopChan := make(chan struct{})
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

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

	var namespaceToWatch string
	if os.Getenv("ROOK_CURRENT_NAMESPACE_ONLY") == "true" {
		logger.Infof("Watching the current namespace for a cluster CRD")
		namespaceToWatch = namespace
	} else {
		logger.Infof("Watching all namespaces for cluster CRDs")
		namespaceToWatch = v1.NamespaceAll
	}

	// watch for changes to the rook clusters
	o.clusterController.StartWatch(namespaceToWatch, stopChan)

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

func (o *Operator) startSystemDaemons() error {
	if o.delayedDaemonsStarted {
		return nil
	}

	namespace := os.Getenv(k8sutil.PodNamespaceEnvVar)
	if namespace == "" {
		return fmt.Errorf("Rook operator namespace is not provided. Expose it via downward API in the rook operator manifest file using environment variable %s", k8sutil.PodNamespaceEnvVar)
	}

	if EnableFlexDriver {
		rookAgent := agent.New(o.context.Clientset)
		if err := rookAgent.Start(namespace, o.rookImage, o.securityAccount); err != nil {
			return fmt.Errorf("error starting agent daemonset: %v", err)
		}
	}

	serverVersion, err := o.context.Clientset.Discovery().ServerVersion()
	if err != nil {
		return fmt.Errorf("error getting server version: %v", err)
	}

	if !csi.CSIEnabled() {
		logger.Infof("CSI driver is not enabled")
		return nil
	}

	if serverVersion.Major < csi.KubeMinMajor || serverVersion.Major == csi.KubeMinMajor && serverVersion.Minor < csi.KubeMinMinor {
		logger.Infof("CSI driver is only supported in K8s 1.13 or newer. version=%s", serverVersion.String())
		return nil
	}

	if err = csi.ValidateCSIParam(); err != nil {
		return fmt.Errorf("invalid csi params: %v", err)
	}
	csi.SetCSINamespace(namespace)
	if err = csi.StartCSIDrivers(namespace, o.context.Clientset); err != nil {
		return fmt.Errorf("failed to start Ceph csi drivers: %v", err)
	}
	logger.Infof("successfully started Ceph CSI driver(s)")

	o.delayedDaemonsStarted = true
	return nil
}
