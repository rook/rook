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

package csi

import (
	"context"
	_ "embed"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	rookclient "github.com/rook/rook/pkg/client/clientset/versioned"
	controllerutil "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/k8sutil/cmdreporter"

	"github.com/pkg/errors"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8scsi "k8s.io/api/storage/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes"
)

type Param struct {
	CSIPluginImage                 string
	RegistrarImage                 string
	ProvisionerImage               string
	AttacherImage                  string
	SnapshotterImage               string
	ResizerImage                   string
	DriverNamePrefix               string
	EnableCSIGRPCMetrics           string
	KubeletDirPath                 string
	ForceCephFSKernelClient        string
	CephFSPluginUpdateStrategy     string
	RBDPluginUpdateStrategy        string
	PluginPriorityClassName        string
	ProvisionerPriorityClassName   string
	VolumeReplicationImage         string
	EnableCSIHostNetwork           bool
	EnableOMAPGenerator            bool
	EnableRBDSnapshotter           bool
	EnableCephFSSnapshotter        bool
	EnableVolumeReplicationSideCar bool
	LogLevel                       uint8
	CephFSGRPCMetricsPort          uint16
	CephFSLivenessMetricsPort      uint16
	RBDGRPCMetricsPort             uint16
	RBDLivenessMetricsPort         uint16
	ProvisionerReplicas            uint8
	CSICephFSPodLabels             map[string]string
	CSIRBDPodLabels                map[string]string
}

type templateParam struct {
	Param
	// non-global template only parameters
	Namespace string
}

var (
	CSIParam Param

	EnableRBD            = false
	EnableCephFS         = false
	EnableCSIGRPCMetrics = false
	AllowUnsupported     = false

	//driver names
	CephFSDriverName string
	RBDDriverName    string

	// configuration map for csi
	ConfigName = "rook-ceph-csi-config"
	ConfigKey  = "csi-cluster-config-json"

	csiLock      sync.Mutex
	csiDriverobj csiDriver
)

// Specify default images as var instead of const so that they can be overridden with the Go
// linker's -X flag. This allows users to easily build images with a different opinionated set of
// images without having to specify them manually in charts/manifests which can make upgrades more
// manually challenging.
var (
	// image names
	DefaultCSIPluginImage         = "quay.io/cephcsi/cephcsi:v3.4.0"
	DefaultRegistrarImage         = "k8s.gcr.io/sig-storage/csi-node-driver-registrar:v2.2.0"
	DefaultProvisionerImage       = "k8s.gcr.io/sig-storage/csi-provisioner:v2.2.2"
	DefaultAttacherImage          = "k8s.gcr.io/sig-storage/csi-attacher:v3.2.1"
	DefaultSnapshotterImage       = "k8s.gcr.io/sig-storage/csi-snapshotter:v4.1.1"
	DefaultResizerImage           = "k8s.gcr.io/sig-storage/csi-resizer:v1.2.0"
	DefaultVolumeReplicationImage = "quay.io/csiaddons/volumereplication-operator:v0.1.0"

	// Local package template path for RBD
	//go:embed template/rbd/csi-rbdplugin.yaml
	RBDPluginTemplatePath string
	//go:embed template/rbd/csi-rbdplugin-provisioner-dep.yaml
	RBDProvisionerDepTemplatePath string
	//go:embed template/rbd/csi-rbdplugin-svc.yaml
	RBDPluginServiceTemplatePath string

	// Local package template path for CephFS
	//go:embed template/cephfs/csi-cephfsplugin.yaml
	CephFSPluginTemplatePath string
	//go:embed template/cephfs/csi-cephfsplugin-provisioner-dep.yaml
	CephFSProvisionerDepTemplatePath string
	//go:embed template/cephfs/csi-cephfsplugin-svc.yaml
	CephFSPluginServiceTemplatePath string
)

const (
	KubeMinMajor                   = "1"
	ProvDeploymentSuppVersion      = "14"
	kubeMinVerForFilesystemRestore = "15"
	kubeMinVerForBlockRestore      = "16"
	kubeMinVerForSnapshot          = "17"
	kubeMinVerForV1csiDriver       = "18"
	kubeMaxVerForBeta1csiDriver    = "21"

	// common tolerations and node affinity
	provisionerTolerationsEnv  = "CSI_PROVISIONER_TOLERATIONS"
	provisionerNodeAffinityEnv = "CSI_PROVISIONER_NODE_AFFINITY"
	pluginTolerationsEnv       = "CSI_PLUGIN_TOLERATIONS"
	pluginNodeAffinityEnv      = "CSI_PLUGIN_NODE_AFFINITY"

	// CephFS tolerations and node affinity
	cephFSProvisionerTolerationsEnv  = "CSI_CEPHFS_PROVISIONER_TOLERATIONS"
	cephFSProvisionerNodeAffinityEnv = "CSI_CEPHFS_PROVISIONER_NODE_AFFINITY"
	cephFSPluginTolerationsEnv       = "CSI_CEPHFS_PLUGIN_TOLERATIONS"
	cephFSPluginNodeAffinityEnv      = "CSI_CEPHFS_PLUGIN_NODE_AFFINITY"

	// RBD tolerations and node affinity
	rbdProvisionerTolerationsEnv  = "CSI_RBD_PROVISIONER_TOLERATIONS"
	rbdProvisionerNodeAffinityEnv = "CSI_RBD_PROVISIONER_NODE_AFFINITY"
	rbdPluginTolerationsEnv       = "CSI_RBD_PLUGIN_TOLERATIONS"
	rbdPluginNodeAffinityEnv      = "CSI_RBD_PLUGIN_NODE_AFFINITY"

	// compute resource for CSI pods
	rbdProvisionerResource = "CSI_RBD_PROVISIONER_RESOURCE"
	rbdPluginResource      = "CSI_RBD_PLUGIN_RESOURCE"

	cephFSProvisionerResource = "CSI_CEPHFS_PROVISIONER_RESOURCE"
	cephFSPluginResource      = "CSI_CEPHFS_PLUGIN_RESOURCE"

	// kubelet directory path
	DefaultKubeletDirPath = "/var/lib/kubelet"

	// grpc metrics and liveness port for cephfs  and rbd
	DefaultCephFSGRPCMerticsPort     uint16 = 9091
	DefaultCephFSLivenessMerticsPort uint16 = 9081
	DefaultRBDGRPCMerticsPort        uint16 = 9090
	DefaultRBDLivenessMerticsPort    uint16 = 9080

	detectCSIVersionName = "rook-ceph-csi-detect-version"
	// default log level for csi containers
	defaultLogLevel uint8 = 0

	// update strategy
	rollingUpdate = "RollingUpdate"
	onDelete      = "OnDelete"

	// driver daemonset names
	csiRBDPlugin    = "csi-rbdplugin"
	csiCephFSPlugin = "csi-cephfsplugin"

	// driver deployment names
	csiRBDProvisioner    = "csi-rbdplugin-provisioner"
	csiCephFSProvisioner = "csi-cephfsplugin-provisioner"
)

func CSIEnabled() bool {
	return EnableRBD || EnableCephFS
}

func validateCSIParam() error {
	if len(CSIParam.CSIPluginImage) == 0 {
		return errors.New("missing csi rbd plugin image")
	}
	if len(CSIParam.RegistrarImage) == 0 {
		return errors.New("missing csi registrar image")
	}
	if len(CSIParam.ProvisionerImage) == 0 {
		return errors.New("missing csi provisioner image")
	}
	if len(CSIParam.AttacherImage) == 0 {
		return errors.New("missing csi attacher image")
	}

	return nil
}

func startDrivers(clientset kubernetes.Interface, rookclientset rookclient.Interface, namespace string, ver *version.Info, ownerInfo *k8sutil.OwnerInfo, v *CephCSIVersion) error {
	ctx := context.TODO()
	var (
		err                                                   error
		rbdPlugin, cephfsPlugin                               *apps.DaemonSet
		rbdProvisionerDeployment, cephfsProvisionerDeployment *apps.Deployment
		rbdService, cephfsService                             *corev1.Service
	)

	tp := templateParam{
		Param:     CSIParam,
		Namespace: namespace,
	}
	// if the user didn't specify a custom DriverNamePrefix use
	// the namespace (and a dot).
	if tp.DriverNamePrefix == "" {
		tp.DriverNamePrefix = fmt.Sprintf("%s.", namespace)
	}

	CephFSDriverName = tp.DriverNamePrefix + "cephfs.csi.ceph.com"
	RBDDriverName = tp.DriverNamePrefix + "rbd.csi.ceph.com"

	csiDriverobj = beta1CsiDriver{}
	if ver.Major > KubeMinMajor || ver.Major == KubeMinMajor && ver.Minor >= kubeMinVerForV1csiDriver {
		csiDriverobj = v1CsiDriver{}
		// In case of an k8s version upgrade, delete the beta CSIDriver object;
		// before the creation of updated v1 object to avoid conflicts.
		// Also, attempt betav1 driver object deletion only if version is less
		// than maximum supported version for betav1 object.(unavailable in v1.22+)
		// Ignore if not found.
		if EnableRBD && ver.Minor <= kubeMaxVerForBeta1csiDriver {
			err = beta1CsiDriver{}.deleteCSIDriverInfo(ctx, clientset, RBDDriverName)
			if err != nil {
				logger.Errorf("failed to delete %q Driver Info. %v", RBDDriverName, err)
			}
		}
		if EnableCephFS && ver.Minor <= kubeMaxVerForBeta1csiDriver {
			err = beta1CsiDriver{}.deleteCSIDriverInfo(ctx, clientset, CephFSDriverName)
			if err != nil {
				logger.Errorf("failed to delete %q Driver Info. %v", CephFSDriverName, err)
			}
		}
	}

	tp.EnableCSIGRPCMetrics = fmt.Sprintf("%t", EnableCSIGRPCMetrics)

	// If not set or set to anything but "false", the kernel client will be enabled
	kClient, err := k8sutil.GetOperatorSetting(clientset, controllerutil.OperatorSettingConfigMapName, "CSI_FORCE_CEPHFS_KERNEL_CLIENT", "true")
	if err != nil {
		return errors.Wrap(err, "failed to load CSI_FORCE_CEPHFS_KERNEL_CLIENT setting")
	}
	if strings.EqualFold(kClient, "false") {
		tp.ForceCephFSKernelClient = "false"
	} else {
		tp.ForceCephFSKernelClient = "true"
	}
	// parse GRPC and Liveness ports
	tp.CephFSGRPCMetricsPort, err = getPortFromConfig(clientset, "CSI_CEPHFS_GRPC_METRICS_PORT", DefaultCephFSGRPCMerticsPort)
	if err != nil {
		return errors.Wrap(err, "error getting CSI CephFS GRPC metrics port.")
	}
	tp.CephFSLivenessMetricsPort, err = getPortFromConfig(clientset, "CSI_CEPHFS_LIVENESS_METRICS_PORT", DefaultCephFSLivenessMerticsPort)
	if err != nil {
		return errors.Wrap(err, "error getting CSI CephFS liveness metrics port.")
	}

	tp.RBDGRPCMetricsPort, err = getPortFromConfig(clientset, "CSI_RBD_GRPC_METRICS_PORT", DefaultRBDGRPCMerticsPort)
	if err != nil {
		return errors.Wrap(err, "error getting CSI RBD GRPC metrics port.")
	}
	tp.RBDLivenessMetricsPort, err = getPortFromConfig(clientset, "CSI_RBD_LIVENESS_METRICS_PORT", DefaultRBDLivenessMerticsPort)
	if err != nil {
		return errors.Wrap(err, "error getting CSI RBD liveness metrics port.")
	}

	// default value `system-node-critical` is the highest available priority
	tp.PluginPriorityClassName, err = k8sutil.GetOperatorSetting(clientset, controllerutil.OperatorSettingConfigMapName, "CSI_PLUGIN_PRIORITY_CLASSNAME", "")
	if err != nil {
		return errors.Wrap(err, "failed to load CSI_PLUGIN_PRIORITY_CLASSNAME setting")
	}

	// default value `system-cluster-critical` is applied for some
	// critical pods in cluster but less priority than plugin pods
	tp.ProvisionerPriorityClassName, err = k8sutil.GetOperatorSetting(clientset, controllerutil.OperatorSettingConfigMapName, "CSI_PROVISIONER_PRIORITY_CLASSNAME", "")
	if err != nil {
		return errors.Wrap(err, "failed to load CSI_PROVISIONER_PRIORITY_CLASSNAME setting")
	}

	// OMAP generator will be enabled by default
	// If AllowUnsupported is set to false and if CSI version is less than
	// <3.2.0 disable OMAP generator sidecar
	if !v.SupportsOMAPController() {
		tp.EnableOMAPGenerator = false
	}

	enableOMAPGenerator, err := k8sutil.GetOperatorSetting(clientset, controllerutil.OperatorSettingConfigMapName, "CSI_ENABLE_OMAP_GENERATOR", "false")
	if err != nil {
		return errors.Wrap(err, "failed to load CSI_ENABLE_OMAP_GENERATOR setting")
	}
	if strings.EqualFold(enableOMAPGenerator, "true") {
		tp.EnableOMAPGenerator = true
	}

	// if k8s >= v1.17 enable RBD and CephFS snapshotter by default
	if ver.Major == KubeMinMajor && ver.Minor >= kubeMinVerForSnapshot {
		tp.EnableRBDSnapshotter = true
		tp.EnableCephFSSnapshotter = true
	}
	enableRBDSnapshotter, err := k8sutil.GetOperatorSetting(clientset, controllerutil.OperatorSettingConfigMapName, "CSI_ENABLE_RBD_SNAPSHOTTER", "true")
	if err != nil {
		return errors.Wrap(err, "failed to load CSI_ENABLE_RBD_SNAPSHOTTER setting")
	}
	if strings.EqualFold(enableRBDSnapshotter, "false") {
		tp.EnableRBDSnapshotter = false
	}
	enableCephFSSnapshotter, err := k8sutil.GetOperatorSetting(clientset, controllerutil.OperatorSettingConfigMapName, "CSI_ENABLE_CEPHFS_SNAPSHOTTER", "true")
	if err != nil {
		return errors.Wrap(err, "failed to load CSI_ENABLE_CEPHFS_SNAPSHOTTER setting")
	}
	if strings.EqualFold(enableCephFSSnapshotter, "false") {
		tp.EnableCephFSSnapshotter = false
	}

	tp.EnableVolumeReplicationSideCar = false
	enableVolumeReplicationSideCar, err := k8sutil.GetOperatorSetting(clientset, controllerutil.OperatorSettingConfigMapName, "CSI_ENABLE_VOLUME_REPLICATION", "false")
	if err != nil {
		return errors.Wrap(err, "failed to load CSI_ENABLE_VOLUME_REPLICATION setting")
	}
	if strings.EqualFold(enableVolumeReplicationSideCar, "true") {
		tp.EnableVolumeReplicationSideCar = true
	}

	updateStrategy, err := k8sutil.GetOperatorSetting(clientset, controllerutil.OperatorSettingConfigMapName, "CSI_CEPHFS_PLUGIN_UPDATE_STRATEGY", rollingUpdate)
	if err != nil {
		return errors.Wrap(err, "failed to load CSI_CEPHFS_PLUGIN_UPDATE_STRATEGY setting")
	}
	if strings.EqualFold(updateStrategy, onDelete) {
		tp.CephFSPluginUpdateStrategy = onDelete
	} else {
		tp.CephFSPluginUpdateStrategy = rollingUpdate
	}

	updateStrategy, err = k8sutil.GetOperatorSetting(clientset, controllerutil.OperatorSettingConfigMapName, "CSI_RBD_PLUGIN_UPDATE_STRATEGY", rollingUpdate)
	if err != nil {
		return errors.Wrap(err, "failed to load CSI_RBD_PLUGIN_UPDATE_STRATEGY setting")
	}
	if strings.EqualFold(updateStrategy, onDelete) {
		tp.RBDPluginUpdateStrategy = onDelete
	} else {
		tp.RBDPluginUpdateStrategy = rollingUpdate
	}

	logger.Infof("Kubernetes version is %s.%s", ver.Major, ver.Minor)

	tp.ResizerImage, err = k8sutil.GetOperatorSetting(clientset, controllerutil.OperatorSettingConfigMapName, "ROOK_CSI_RESIZER_IMAGE", DefaultResizerImage)
	if err != nil {
		return errors.Wrap(err, "failed to load ROOK_CSI_RESIZER_IMAGE setting")
	}
	if tp.ResizerImage == "" {
		tp.ResizerImage = DefaultResizerImage
	}

	if ver.Major == KubeMinMajor && ver.Minor < kubeMinVerForFilesystemRestore {
		logger.Warning("CSI Filesystem volume expansion requires Kubernetes version >=1.15.0")
	}
	if ver.Major == KubeMinMajor && ver.Minor < kubeMinVerForBlockRestore {
		logger.Warning("CSI Block volume expansion requires Kubernetes version >=1.16.0")
	}

	logLevel, err := k8sutil.GetOperatorSetting(clientset, controllerutil.OperatorSettingConfigMapName, "CSI_LOG_LEVEL", "")
	if err != nil {
		// logging a warning and intentionally continuing with the default log level
		logger.Warningf("failed to load CSI_LOG_LEVEL. Defaulting to %d. %v", defaultLogLevel, err)
	}
	tp.LogLevel = defaultLogLevel
	if logLevel != "" {
		l, err := strconv.ParseUint(logLevel, 10, 8)
		if err != nil {
			logger.Errorf("failed to parse CSI_LOG_LEVEL. Defaulting to %d. %v", defaultLogLevel, err)
		} else {
			tp.LogLevel = uint8(l)
		}
	}

	tp.ProvisionerReplicas = 2
	nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err == nil {
		if len(nodes.Items) == 1 {
			tp.ProvisionerReplicas = 1
		}
	} else {
		logger.Errorf("failed to get nodes. Defaulting the number of replicas of provisioner pods to 2. %v", err)
	}

	if EnableRBD {
		rbdPlugin, err = templateToDaemonSet("rbdplugin", RBDPluginTemplatePath, tp)
		if err != nil {
			return errors.Wrap(err, "failed to load rbdplugin template")
		}

		rbdProvisionerDeployment, err = templateToDeployment("rbd-provisioner", RBDProvisionerDepTemplatePath, tp)
		if err != nil {
			return errors.Wrap(err, "failed to load rbd provisioner deployment template")
		}

		rbdService, err = templateToService("rbd-service", RBDPluginServiceTemplatePath, tp)
		if err != nil {
			return errors.Wrap(err, "failed to load rbd plugin service template")
		}
		rbdService.Namespace = namespace
	}
	if EnableCephFS {
		cephfsPlugin, err = templateToDaemonSet("cephfsplugin", CephFSPluginTemplatePath, tp)
		if err != nil {
			return errors.Wrap(err, "failed to load CephFS plugin template")
		}

		cephfsProvisionerDeployment, err = templateToDeployment("cephfs-provisioner", CephFSProvisionerDepTemplatePath, tp)
		if err != nil {
			return errors.Wrap(err, "failed to load rbd provisioner deployment template")
		}

		cephfsService, err = templateToService("cephfs-service", CephFSPluginServiceTemplatePath, tp)
		if err != nil {
			return errors.Wrap(err, "failed to load cephfs plugin service template")
		}
		cephfsService.Namespace = namespace
	}

	// get common provisioner tolerations and node affinity
	provisionerTolerations := getToleration(clientset, provisionerTolerationsEnv, []corev1.Toleration{})
	provisionerNodeAffinity := getNodeAffinity(clientset, provisionerNodeAffinityEnv, &corev1.NodeAffinity{})
	// get common plugin tolerations and node affinity
	pluginTolerations := getToleration(clientset, pluginTolerationsEnv, []corev1.Toleration{})
	pluginNodeAffinity := getNodeAffinity(clientset, pluginNodeAffinityEnv, &corev1.NodeAffinity{})

	if rbdPlugin != nil {
		// get RBD plugin tolerations and node affinity, defaults to common tolerations and node affinity if not specified
		rbdPluginTolerations := getToleration(clientset, rbdPluginTolerationsEnv, pluginTolerations)
		rbdPluginNodeAffinity := getNodeAffinity(clientset, rbdPluginNodeAffinityEnv, pluginNodeAffinity)
		// apply RBD plugin tolerations and node affinity
		applyToPodSpec(&rbdPlugin.Spec.Template.Spec, rbdPluginNodeAffinity, rbdPluginTolerations)
		// apply resource request and limit to rbdplugin containers
		applyResourcesToContainers(clientset, rbdPluginResource, &rbdPlugin.Spec.Template.Spec)
		err = ownerInfo.SetControllerReference(rbdPlugin)
		if err != nil {
			return errors.Wrapf(err, "failed to set owner reference to rbd plugin daemonset %q", rbdPlugin.Name)
		}
		multusApplied, err := applyCephClusterNetworkConfig(ctx, &rbdPlugin.Spec.Template.ObjectMeta, rookclientset)
		if err != nil {
			return errors.Wrapf(err, "failed to apply network config to rbd plugin daemonset: %+v", rbdPlugin)
		}
		if multusApplied {
			rbdPlugin.Spec.Template.Spec.HostNetwork = false
		}
		err = k8sutil.CreateDaemonSet(csiRBDPlugin, namespace, clientset, rbdPlugin)
		if err != nil {
			return errors.Wrapf(err, "failed to start rbdplugin daemonset: %+v", rbdPlugin)
		}
		k8sutil.AddRookVersionLabelToDaemonSet(rbdPlugin)
	}

	if rbdProvisionerDeployment != nil {
		// get RBD provisioner tolerations and node affinity, defaults to common tolerations and node affinity if not specified
		rbdProvisionerTolerations := getToleration(clientset, rbdProvisionerTolerationsEnv, provisionerTolerations)
		rbdProvisionerNodeAffinity := getNodeAffinity(clientset, rbdProvisionerNodeAffinityEnv, provisionerNodeAffinity)
		// apply RBD provisioner tolerations and node affinity
		applyToPodSpec(&rbdProvisionerDeployment.Spec.Template.Spec, rbdProvisionerNodeAffinity, rbdProvisionerTolerations)
		// apply resource request and limit to rbd provisioner containers
		applyResourcesToContainers(clientset, rbdProvisionerResource, &rbdProvisionerDeployment.Spec.Template.Spec)
		err = ownerInfo.SetControllerReference(rbdProvisionerDeployment)
		if err != nil {
			return errors.Wrapf(err, "failed to set owner reference to rbd provisioner deployment %q", rbdProvisionerDeployment.Name)
		}
		antiAffinity := GetPodAntiAffinity("app", csiRBDProvisioner)
		rbdProvisionerDeployment.Spec.Template.Spec.Affinity.PodAntiAffinity = &antiAffinity
		rbdProvisionerDeployment.Spec.Strategy = apps.DeploymentStrategy{
			Type: apps.RecreateDeploymentStrategyType,
		}

		_, err = applyCephClusterNetworkConfig(ctx, &rbdProvisionerDeployment.Spec.Template.ObjectMeta, rookclientset)
		if err != nil {
			return errors.Wrapf(err, "failed to apply network config to rbd plugin provisioner deployment: %+v", rbdProvisionerDeployment)
		}
		_, err = k8sutil.CreateOrUpdateDeployment(clientset, rbdProvisionerDeployment)
		if err != nil {
			return errors.Wrapf(err, "failed to start rbd provisioner deployment: %+v", rbdProvisionerDeployment)
		}
		k8sutil.AddRookVersionLabelToDeployment(rbdProvisionerDeployment)
		logger.Info("successfully started CSI Ceph RBD driver")
	}

	if rbdService != nil {
		rbdService.Namespace = namespace
		err = ownerInfo.SetControllerReference(rbdService)
		if err != nil {
			return errors.Wrapf(err, "failed to set owner reference to rbd service %q", rbdService)
		}
		_, err = k8sutil.CreateOrUpdateService(clientset, namespace, rbdService)
		if err != nil {
			return errors.Wrapf(err, "failed to create rbd service: %+v", rbdService)
		}
	}

	if cephfsPlugin != nil {
		// get CephFS plugin tolerations and node affinity, defaults to common tolerations and node affinity if not specified
		cephFSPluginTolerations := getToleration(clientset, cephFSPluginTolerationsEnv, pluginTolerations)
		cephFSPluginNodeAffinity := getNodeAffinity(clientset, cephFSPluginNodeAffinityEnv, pluginNodeAffinity)
		// apply CephFS plugin tolerations and node affinity
		applyToPodSpec(&cephfsPlugin.Spec.Template.Spec, cephFSPluginNodeAffinity, cephFSPluginTolerations)
		// apply resource request and limit to cephfs plugin containers
		applyResourcesToContainers(clientset, cephFSPluginResource, &cephfsPlugin.Spec.Template.Spec)
		err = ownerInfo.SetControllerReference(cephfsPlugin)
		if err != nil {
			return errors.Wrapf(err, "failed to set owner reference to cephfs plugin daemonset %q", cephfsPlugin.Name)
		}
		multusApplied, err := applyCephClusterNetworkConfig(ctx, &cephfsPlugin.Spec.Template.ObjectMeta, rookclientset)
		if err != nil {
			return errors.Wrapf(err, "failed to apply network config to cephfs plugin daemonset: %+v", cephfsPlugin)
		}
		if multusApplied {
			cephfsPlugin.Spec.Template.Spec.HostNetwork = false
		}
		err = k8sutil.CreateDaemonSet(csiCephFSPlugin, namespace, clientset, cephfsPlugin)
		if err != nil {
			return errors.Wrapf(err, "failed to start cephfs plugin daemonset: %+v", cephfsPlugin)
		}
		k8sutil.AddRookVersionLabelToDaemonSet(cephfsPlugin)
	}

	if cephfsProvisionerDeployment != nil {
		// get CephFS provisioner tolerations and node affinity, defaults to common tolerations and node affinity if not specified
		cephFSProvisionerTolerations := getToleration(clientset, cephFSProvisionerTolerationsEnv, provisionerTolerations)
		cephFSProvisionerNodeAffinity := getNodeAffinity(clientset, cephFSProvisionerNodeAffinityEnv, provisionerNodeAffinity)
		// apply CephFS provisioner tolerations and node affinity
		applyToPodSpec(&cephfsProvisionerDeployment.Spec.Template.Spec, cephFSProvisionerNodeAffinity, cephFSProvisionerTolerations)
		// get resource details for cephfs provisioner
		// apply resource request and limit to cephfs provisioner containers
		applyResourcesToContainers(clientset, cephFSProvisionerResource, &cephfsProvisionerDeployment.Spec.Template.Spec)
		err = ownerInfo.SetControllerReference(cephfsProvisionerDeployment)
		if err != nil {
			return errors.Wrapf(err, "failed to set owner reference to cephfs provisioner deployment %q", cephfsProvisionerDeployment.Name)
		}
		antiAffinity := GetPodAntiAffinity("app", csiCephFSProvisioner)
		cephfsProvisionerDeployment.Spec.Template.Spec.Affinity.PodAntiAffinity = &antiAffinity
		cephfsProvisionerDeployment.Spec.Strategy = apps.DeploymentStrategy{
			Type: apps.RecreateDeploymentStrategyType,
		}

		_, err = applyCephClusterNetworkConfig(ctx, &cephfsProvisionerDeployment.Spec.Template.ObjectMeta, rookclientset)
		if err != nil {
			return errors.Wrapf(err, "failed to apply network config to cephfs plugin provisioner deployment: %+v", cephfsProvisionerDeployment)
		}
		_, err = k8sutil.CreateOrUpdateDeployment(clientset, cephfsProvisionerDeployment)
		if err != nil {
			return errors.Wrapf(err, "failed to start cephfs provisioner deployment: %+v", cephfsProvisionerDeployment)
		}
		k8sutil.AddRookVersionLabelToDeployment(cephfsProvisionerDeployment)
		logger.Info("successfully started CSI CephFS driver")
	}
	if cephfsService != nil {
		err = ownerInfo.SetControllerReference(cephfsService)
		if err != nil {
			return errors.Wrapf(err, "failed to set owner reference to cephfs service %q", cephfsService)
		}
		_, err = k8sutil.CreateOrUpdateService(clientset, namespace, cephfsService)
		if err != nil {
			return errors.Wrapf(err, "failed to create cephfs service: %+v", cephfsService)
		}
	}

	if EnableRBD {
		fsGroupPolicyForRBD, err := k8sutil.GetOperatorSetting(clientset, controllerutil.OperatorSettingConfigMapName, "CSI_RBD_FSGROUPPOLICY", string(k8scsi.ReadWriteOnceWithFSTypeFSGroupPolicy))
		if err != nil {
			// logging a warning and intentionally continuing with the default log level
			logger.Warningf("failed to parse CSI_RBD_FSGROUPPOLICY. Defaulting to %q. %v", k8scsi.ReadWriteOnceWithFSTypeFSGroupPolicy, err)
		}
		err = csiDriverobj.createCSIDriverInfo(ctx, clientset, RBDDriverName, fsGroupPolicyForRBD)
		if err != nil {
			return errors.Wrapf(err, "failed to create CSI driver object for %q", RBDDriverName)
		}
	}
	if EnableCephFS {
		fsGroupPolicyForCephFS, err := k8sutil.GetOperatorSetting(clientset, controllerutil.OperatorSettingConfigMapName, "CSI_CEPHFS_FSGROUPPOLICY", string(k8scsi.ReadWriteOnceWithFSTypeFSGroupPolicy))
		if err != nil {
			// logging a warning and intentionally continuing with the default
			// log level
			logger.Warningf("failed to parse CSI_CEPHFS_FSGROUPPOLICY. Defaulting to %q. %v", k8scsi.NoneFSGroupPolicy, err)
		}
		err = csiDriverobj.createCSIDriverInfo(ctx, clientset, CephFSDriverName, fsGroupPolicyForCephFS)
		if err != nil {
			return errors.Wrapf(err, "failed to create CSI driver object for %q", CephFSDriverName)
		}
	}

	return nil
}

func stopDrivers(clientset kubernetes.Interface, namespace string, ver *version.Info) {
	if !EnableRBD {
		logger.Info("CSI Ceph RBD driver disabled")
		succeeded := deleteCSIDriverResources(clientset, ver, namespace, csiRBDPlugin, csiRBDProvisioner, "csi-rbdplugin-metrics", RBDDriverName)
		if succeeded {
			logger.Info("successfully removed CSI Ceph RBD driver")
		} else {
			logger.Error("failed to remove CSI Ceph RBD driver")
		}
	}

	if !EnableCephFS {
		logger.Info("CSI CephFS driver disabled")
		succeeded := deleteCSIDriverResources(clientset, ver, namespace, csiCephFSPlugin, csiCephFSProvisioner, "csi-cephfsplugin-metrics", CephFSDriverName)
		if succeeded {
			logger.Info("successfully removed CSI CephFS driver")
		} else {
			logger.Error("failed to remove CSI CephFS driver")
		}
	}
}

func deleteCSIDriverResources(
	clientset kubernetes.Interface, ver *version.Info, namespace, daemonset, deployment, service, driverName string) bool {
	ctx := context.TODO()
	succeeded := true
	csiDriverobj = beta1CsiDriver{}
	if ver.Major > KubeMinMajor || ver.Major == KubeMinMajor && ver.Minor >= kubeMinVerForV1csiDriver {
		csiDriverobj = v1CsiDriver{}
	}
	err := k8sutil.DeleteDaemonset(clientset, namespace, daemonset)
	if err != nil {
		logger.Errorf("failed to delete the %q. %v", daemonset, err)
		succeeded = false
	}

	err = k8sutil.DeleteDeployment(clientset, namespace, deployment)
	if err != nil {
		logger.Errorf("failed to delete the %q. %v", deployment, err)
		succeeded = false
	}

	err = k8sutil.DeleteService(clientset, namespace, service)
	if err != nil {
		logger.Errorf("failed to delete the %q. %v", service, err)
		succeeded = false
	}

	err = csiDriverobj.deleteCSIDriverInfo(ctx, clientset, driverName)
	if err != nil {
		logger.Errorf("failed to delete %q Driver Info. %v", driverName, err)
		succeeded = false
	}
	return succeeded
}

func applyCephClusterNetworkConfig(ctx context.Context, objectMeta *metav1.ObjectMeta, rookclientset rookclient.Interface) (bool, error) {
	var isMultusApplied bool
	cephClusters, err := rookclientset.CephV1().CephClusters(objectMeta.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, errors.Wrap(err, "failed to find CephClusters")
	}
	for _, cephCluster := range cephClusters.Items {
		if cephCluster.Spec.Network.IsMultus() {
			err = k8sutil.ApplyMultus(cephCluster.Spec.Network, objectMeta)
			if err != nil {
				return false, errors.Wrapf(err, "failed to apply multus configuration to CephCluster %q", cephCluster.Name)
			}
			isMultusApplied = true
		}
	}

	return isMultusApplied, nil
}

// ValidateCSIVersion checks if the configured ceph-csi image is supported
func validateCSIVersion(clientset kubernetes.Interface, namespace, rookImage, serviceAccountName string, ownerInfo *k8sutil.OwnerInfo) (*CephCSIVersion, error) {
	timeout := 15 * time.Minute

	logger.Infof("detecting the ceph csi image version for image %q", CSIParam.CSIPluginImage)

	versionReporter, err := cmdreporter.New(
		clientset,
		ownerInfo,
		detectCSIVersionName, detectCSIVersionName, namespace,
		[]string{"cephcsi"}, []string{"--version"},
		rookImage, CSIParam.CSIPluginImage)

	if err != nil {
		return nil, errors.Wrap(err, "failed to set up ceph CSI version job")
	}

	job := versionReporter.Job()
	job.Spec.Template.Spec.ServiceAccountName = serviceAccountName

	// Apply csi provisioner toleration for csi version check job
	job.Spec.Template.Spec.Tolerations = getToleration(clientset, provisionerTolerationsEnv, []corev1.Toleration{})
	stdout, _, retcode, err := versionReporter.Run(timeout)
	if err != nil {
		return nil, errors.Wrap(err, "failed to complete ceph CSI version job")
	}

	if retcode != 0 {
		return nil, errors.Errorf("ceph CSI version job returned %d", retcode)
	}

	version, err := extractCephCSIVersion(stdout)
	if err != nil {
		return nil, errors.Wrap(err, "failed to extract ceph CSI version")
	}
	logger.Infof("Detected ceph CSI image version: %q", version)

	if !version.Supported() {
		return nil, errors.Errorf("ceph CSI image needs to be at least version %q", minimum.String())
	}
	return version, nil
}
