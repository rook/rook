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
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/agent/flexvolume"
	"github.com/rook/rook/pkg/daemon/ceph/agent/flexvolume/attachment"
	"github.com/rook/rook/pkg/operator/ceph/agent"
	"github.com/rook/rook/pkg/operator/ceph/cluster"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/csi"
	"github.com/rook/rook/pkg/operator/ceph/provisioner"
	"github.com/rook/rook/pkg/operator/discover"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
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
	// EnableFlexDriver Whether to enable the flex driver. If true, the rook-ceph-agent daemonset will be started.
	EnableFlexDriver = true

	// EnableDiscoveryDaemon Whether to enable the daemon for device discovery. If true, the rook-ceph-discover daemonset will be started.
	EnableDiscoveryDaemon = true

	// ImmediateRetryResult Return this for a immediate retry of the reconciliation loop with the same request object.
	ImmediateRetryResult = reconcile.Result{Requeue: true}

	// WaitForRequeueIfCephClusterNotReadyAfter requeue after 10sec if the operator is not ready
	WaitForRequeueIfCephClusterNotReadyAfter = 10 * time.Second

	// WaitForRequeueIfCephClusterNotReady waits for the CephCluster to be ready
	WaitForRequeueIfCephClusterNotReady = reconcile.Result{Requeue: true, RequeueAfter: WaitForRequeueIfCephClusterNotReadyAfter}
)

// Operator type for managing storage
type Operator struct {
	context           *clusterd.Context
	resources         []k8sutil.CustomResource
	operatorNamespace string
	rookImage         string
	securityAccount   string
	// The custom resource that is global to the kubernetes cluster.
	// The cluster is global because you create multiple clusters in k8s
	clusterController     *cluster.ClusterController
	delayedDaemonsStarted bool
}

// New creates an operator instance
func New(context *clusterd.Context, volumeAttachmentWrapper attachment.Attachment, rookImage, securityAccount string) *Operator {
	schemes := []k8sutil.CustomResource{opcontroller.ClusterResource, attachment.VolumeResource}

	operatorNamespace := os.Getenv(k8sutil.PodNamespaceEnvVar)
	o := &Operator{
		context:           context,
		resources:         schemes,
		operatorNamespace: operatorNamespace,
		rookImage:         rookImage,
		securityAccount:   securityAccount,
	}
	operatorConfigCallbacks := []func() error{
		o.updateDrivers,
	}
	addCallbacks := []func() error{
		o.startDrivers,
	}
	o.clusterController = cluster.NewClusterController(context, rookImage, volumeAttachmentWrapper, operatorConfigCallbacks, addCallbacks)
	return o
}

// Run the operator instance
func (o *Operator) Run() error {

	if o.operatorNamespace == "" {
		return errors.Errorf("rook operator namespace is not provided. expose it via downward API in the rook operator manifest file using environment variable %q", k8sutil.PodNamespaceEnvVar)
	}

	if EnableDiscoveryDaemon {
		rookDiscover := discover.New(o.context.Clientset)
		if err := rookDiscover.Start(o.operatorNamespace, o.rookImage, o.securityAccount, true); err != nil {
			return errors.Wrap(err, "failed to start device discovery daemonset")
		}
	}

	serverVersion, err := o.context.Clientset.Discovery().ServerVersion()
	if err != nil {
		return errors.Wrap(err, "failed to get server version")
	}

	// Initialize signal handler
	signalChan := make(chan os.Signal, 1)
	stopChan := make(chan struct{})
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	// For Flex Driver, run volume provisioner for each of the supported configurations
	if EnableFlexDriver {
		for name, vendor := range provisionerConfigs {
			volumeProvisioner := provisioner.New(o.context, vendor)
			pc := controller.NewProvisionController(
				o.context.Clientset,
				name,
				volumeProvisioner,
				serverVersion.GitVersion,
			)
			go pc.Run(stopChan)
			logger.Infof("rook-provisioner %q started using %q flex vendor dir", name, vendor)
		}
	}

	var namespaceToWatch string
	if os.Getenv("ROOK_CURRENT_NAMESPACE_ONLY") == "true" {
		logger.Infof("watching the current namespace for a ceph cluster CR")
		namespaceToWatch = o.operatorNamespace
	} else {
		logger.Infof("watching all namespaces for ceph cluster CRs")
		namespaceToWatch = v1.NamespaceAll
	}

	// Start the controller-runtime Manager.
	go o.startManager(namespaceToWatch, stopChan)

	// Start the operator setting watcher
	go o.clusterController.StartOperatorSettingsWatch(namespaceToWatch, stopChan)

	// Signal handler to stop the operator
	for {
		select {
		case <-signalChan:
			logger.Info("shutdown signal received, exiting...")
			close(stopChan)
			o.clusterController.StopWatch()
			return nil
		}
	}
}

func (o *Operator) startDrivers() error {
	if o.delayedDaemonsStarted {
		return nil
	}

	o.delayedDaemonsStarted = true
	if err := o.updateDrivers(); err != nil {
		o.delayedDaemonsStarted = false // unset because failed to updateDrivers
		return err
	}

	return nil
}

func (o *Operator) updateDrivers() error {
	var err error

	// Skipping CSI driver update since the first cluster hasn't been started yet
	if !o.delayedDaemonsStarted {
		return nil
	}

	if o.operatorNamespace == "" {
		return errors.Errorf("rook operator namespace is not provided. expose it via downward API in the rook operator manifest file using environment variable %s", k8sutil.PodNamespaceEnvVar)
	}

	if EnableFlexDriver {
		rookAgent := agent.New(o.context.Clientset)
		if err := rookAgent.Start(o.operatorNamespace, o.rookImage, o.securityAccount); err != nil {
			return errors.Wrapf(err, "error starting agent daemonset")
		}
	}

	serverVersion, err := o.context.Clientset.Discovery().ServerVersion()
	if err != nil {
		return errors.Wrapf(err, "error getting server version")
	}

	if err = o.setCSIParams(); err != nil {
		return errors.Wrap(err, "failed to configure CSI parameters")
	}

	if !csi.CSIEnabled() {
		logger.Infof("CSI driver is not enabled")
		return nil
	}

	if serverVersion.Major < csi.KubeMinMajor || serverVersion.Major == csi.KubeMinMajor && serverVersion.Minor < csi.KubeMinMinor {
		logger.Infof("CSI driver is only supported in K8s 1.13 or newer. version=%s", serverVersion.String())
		// disable csi control variables to disable other csi functions
		csi.EnableRBD = false
		csi.EnableCephFS = false
		return nil
	}

	if err = csi.ValidateCSIParam(); err != nil {
		return errors.Wrapf(err, "invalid csi params")
	}

	if err = csi.ValidateCSIVersion(o.context.Clientset, o.operatorNamespace, o.rookImage, o.securityAccount); err != nil {
		return errors.Wrap(err, "invalid csi version")
	}

	if err = csi.StartCSIDrivers(o.operatorNamespace, o.context.Clientset, serverVersion); err != nil {
		return errors.Wrapf(err, "failed to start Ceph csi drivers")
	}
	logger.Infof("successfully started Ceph CSI driver(s)")
	return nil
}

func (o *Operator) setCSIParams() error {
	var err error

	csiEnableRBD, err := k8sutil.GetOperatorSetting(o.context.Clientset, opcontroller.OperatorSettingConfigMapName, "ROOK_CSI_ENABLE_RBD", "true")
	if err != nil {
		return errors.Wrap(err, "unable to determine if CSI driver for RBD is enabled")
	}
	if csi.EnableRBD, err = strconv.ParseBool(csiEnableRBD); err != nil {
		return errors.Wrap(err, "unable to parse value for 'ROOK_CSI_ENABLE_RBD'")
	}

	csiEnableCephFS, err := k8sutil.GetOperatorSetting(o.context.Clientset, opcontroller.OperatorSettingConfigMapName, "ROOK_CSI_ENABLE_CEPHFS", "true")
	if err != nil {
		return errors.Wrap(err, "unable to determine if CSI driver for CephFS is enabled")
	}
	if csi.EnableCephFS, err = strconv.ParseBool(csiEnableCephFS); err != nil {
		return errors.Wrap(err, "unable to parse value for 'ROOK_CSI_ENABLE_CEPHFS'")
	}

	csiAllowUnsupported, err := k8sutil.GetOperatorSetting(o.context.Clientset, opcontroller.OperatorSettingConfigMapName, "ROOK_CSI_ALLOW_UNSUPPORTED_VERSION", "false")
	if err != nil {
		return errors.Wrap(err, "unable to determine if unsupported version is allowed")
	}
	if csi.AllowUnsupported, err = strconv.ParseBool(csiAllowUnsupported); err != nil {
		return errors.Wrap(err, "unable to parse value for 'ROOK_CSI_ALLOW_UNSUPPORTED_VERSION'")
	}

	csiEnableCSIGRPCMetrics, err := k8sutil.GetOperatorSetting(o.context.Clientset, opcontroller.OperatorSettingConfigMapName, "ROOK_CSI_ENABLE_GRPC_METRICS", "true")
	if err != nil {
		return errors.Wrap(err, "unable to determine if CSI GRPC metrics is enabled")
	}
	if csi.EnableCSIGRPCMetrics, err = strconv.ParseBool(csiEnableCSIGRPCMetrics); err != nil {
		return errors.Wrap(err, "unable to parse value for 'ROOK_CSI_ENABLE_GRPC_METRICS'")
	}

	csi.CSIParam.CSIPluginImage, err = k8sutil.GetOperatorSetting(o.context.Clientset, opcontroller.OperatorSettingConfigMapName, "ROOK_CSI_CEPH_IMAGE", csi.DefaultCSIPluginImage)
	if err != nil {
		return errors.Wrap(err, "unable to configure CSI plugin image")
	}
	csi.CSIParam.RegistrarImage, err = k8sutil.GetOperatorSetting(o.context.Clientset, opcontroller.OperatorSettingConfigMapName, "ROOK_CSI_REGISTRAR_IMAGE", csi.DefaultRegistrarImage)
	if err != nil {
		return errors.Wrap(err, "unable to configure CSI registrar image")
	}
	csi.CSIParam.ProvisionerImage, err = k8sutil.GetOperatorSetting(o.context.Clientset, opcontroller.OperatorSettingConfigMapName, "ROOK_CSI_PROVISIONER_IMAGE", csi.DefaultProvisionerImage)
	if err != nil {
		return errors.Wrap(err, "unable to configure CSI provisioner image")
	}
	csi.CSIParam.AttacherImage, err = k8sutil.GetOperatorSetting(o.context.Clientset, opcontroller.OperatorSettingConfigMapName, "ROOK_CSI_ATTACHER_IMAGE", csi.DefaultAttacherImage)
	if err != nil {
		return errors.Wrap(err, "unable to configure CSI attacher image")
	}
	csi.CSIParam.SnapshotterImage, err = k8sutil.GetOperatorSetting(o.context.Clientset, opcontroller.OperatorSettingConfigMapName, "ROOK_CSI_SNAPSHOTTER_IMAGE", csi.DefaultSnapshotterImage)
	if err != nil {
		return errors.Wrap(err, "unable to configure CSI snapshotter image")
	}
	csi.CSIParam.KubeletDirPath, err = k8sutil.GetOperatorSetting(o.context.Clientset, opcontroller.OperatorSettingConfigMapName, "ROOK_CSI_KUBELET_DIR_PATH", csi.DefaultKubeletDirPath)
	if err != nil {
		return errors.Wrap(err, "unable to configure CSI kubelet directory path")
	}
	return nil
}
