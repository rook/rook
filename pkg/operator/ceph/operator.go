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
	"strings"
	"syscall"

	"github.com/coreos/pkg/capnslog"
	opkit "github.com/rook/operator-kit"
	rookv1alpha1 "github.com/rook/rook/pkg/apis/rook.io/v1alpha1"
	rookv1alpha2 "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/agent/flexvolume"
	"github.com/rook/rook/pkg/daemon/ceph/agent/flexvolume/attachment"
	"github.com/rook/rook/pkg/operator/ceph/agent"
	"github.com/rook/rook/pkg/operator/ceph/cluster"
	"github.com/rook/rook/pkg/operator/ceph/file"
	"github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/rook/rook/pkg/operator/ceph/pool"
	"github.com/rook/rook/pkg/operator/ceph/provisioner"
	"github.com/rook/rook/pkg/operator/ceph/provisioner/controller"
	"github.com/rook/rook/pkg/operator/discover"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	schemes := []opkit.CustomResource{cluster.ClusterResource, pool.PoolResource, object.ObjectStoreResource,
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

	// Look for any legacy volume attachments and migrate them now before starting the rook agents
	legacyVolumes, err := o.context.RookClientset.RookV1alpha1().VolumeAttachments(namespace).List(metav1.ListOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to list legacy volume attachments: %+v", err)
	}

	migrationErrors := []string{}
	for _, lva := range legacyVolumes.Items {
		va := rookv1alpha2.ConvertLegacyVolume(lva)
		if err := o.migrateLegacyVolume(lva, va); err != nil {
			migrationErrors = append(migrationErrors, err.Error())
		}
	}

	if len(migrationErrors) > 0 {
		return fmt.Errorf("failed to migrate %d legacy volume attachments in namespace %s: %+v",
			len(migrationErrors), namespace, strings.Join(migrationErrors, "\n"))
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

func (o *Operator) migrateLegacyVolume(legacyVolume rookv1alpha1.VolumeAttachment,
	volumeAttachment *rookv1alpha2.Volume) error {

	logger.Infof("migrating legacy volumeattachment %s in namespace %s", volumeAttachment.Name, volumeAttachment.Namespace)

	_, err := o.context.RookClientset.RookV1alpha2().Volumes(volumeAttachment.Namespace).Get(volumeAttachment.Name, metav1.GetOptions{})
	if err == nil {
		// volumeattachment of current type with same name/namespace already exists, don't overwrite it
		logger.Warningf("volumeattachment object %s in namespace %s already exists, will not overwrite with migrated legacy volumeattachment.",
			volumeAttachment.Name, volumeAttachment.Namespace)
	} else {
		if !errors.IsNotFound(err) {
			return err
		}

		// volumeattachment of current type does not already exist, create it now to complete the migration
		_, err = o.context.RookClientset.RookV1alpha2().Volumes(volumeAttachment.Namespace).Create(volumeAttachment)
		if err != nil {
			return err
		}

		logger.Infof("completed migration of legacy volumeattachment %s in namespace %s", volumeAttachment.Name, volumeAttachment.Namespace)
	}

	// delete the legacy volumeattachment instance, it should not be used anymore now that a migrated instance of the current type exists
	logger.Infof("deleting legacy volumeattachment %s in namespace %s", legacyVolume.Name, legacyVolume.Namespace)
	deletePropagation := metav1.DeletePropagationOrphan
	err = o.context.RookClientset.RookV1alpha1().VolumeAttachments(legacyVolume.Namespace).Delete(
		legacyVolume.Name, &metav1.DeleteOptions{PropagationPolicy: &deletePropagation})
	return err
}
