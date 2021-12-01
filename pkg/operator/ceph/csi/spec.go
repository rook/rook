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
	"time"

	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/k8sutil/cmdreporter"

	"github.com/pkg/errors"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8scsi "k8s.io/api/storage/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"
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
	EnablePluginSelinuxHostMount   bool
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
	ProvisionerReplicas            int32
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

	csiDriverobj csiDriver
)

// Specify default images as var instead of const so that they can be overridden with the Go
// linker's -X flag. This allows users to easily build images with a different opinionated set of
// images without having to specify them manually in charts/manifests which can make upgrades more
// manually challenging.
var (
	// image names
	DefaultCSIPluginImage         = "quay.io/cephcsi/cephcsi:v3.4.0"
	DefaultRegistrarImage         = "k8s.gcr.io/sig-storage/csi-node-driver-registrar:v2.3.0"
	DefaultProvisionerImage       = "k8s.gcr.io/sig-storage/csi-provisioner:v3.0.0"
	DefaultAttacherImage          = "k8s.gcr.io/sig-storage/csi-attacher:v3.3.0"
	DefaultSnapshotterImage       = "k8s.gcr.io/sig-storage/csi-snapshotter:v4.2.0"
	DefaultResizerImage           = "k8s.gcr.io/sig-storage/csi-resizer:v1.3.0"
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
	KubeMinMajor                = "1"
	kubeMinVerForSnapshot       = "17"
	kubeMinVerForV1csiDriver    = "18"
	kubeMaxVerForBeta1csiDriver = "21"

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

	// default provisioner replicas
	defaultProvisionerReplicas int32 = 2

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

func (r *ReconcileCSI) startDrivers(ver *version.Info, ownerInfo *k8sutil.OwnerInfo, v *CephCSIVersion) error {
	var (
		err                                                   error
		rbdPlugin, cephfsPlugin                               *apps.DaemonSet
		rbdProvisionerDeployment, cephfsProvisionerDeployment *apps.Deployment
		rbdService, cephfsService                             *corev1.Service
	)

	tp := templateParam{
		Param:     CSIParam,
		Namespace: r.opConfig.OperatorNamespace,
	}
	// if the user didn't specify a custom DriverNamePrefix use
	// the namespace (and a dot).
	if tp.DriverNamePrefix == "" {
		tp.DriverNamePrefix = fmt.Sprintf("%s.", r.opConfig.OperatorNamespace)
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
			err = beta1CsiDriver{}.deleteCSIDriverInfo(r.opManagerContext, r.context.Clientset, RBDDriverName)
			if err != nil {
				logger.Errorf("failed to delete %q Driver Info. %v", RBDDriverName, err)
			}
		}
		if EnableCephFS && ver.Minor <= kubeMaxVerForBeta1csiDriver {
			err = beta1CsiDriver{}.deleteCSIDriverInfo(r.opManagerContext, r.context.Clientset, CephFSDriverName)
			if err != nil {
				logger.Errorf("failed to delete %q Driver Info. %v", CephFSDriverName, err)
			}
		}
	}

	tp.EnableCSIGRPCMetrics = fmt.Sprintf("%t", EnableCSIGRPCMetrics)

	// If not set or set to anything but "false", the kernel client will be enabled
	if strings.EqualFold(k8sutil.GetValue(r.opConfig.Parameters, "CSI_FORCE_CEPHFS_KERNEL_CLIENT", "true"), "false") {
		tp.ForceCephFSKernelClient = "false"
	} else {
		tp.ForceCephFSKernelClient = "true"
	}

	// parse GRPC and Liveness ports
	tp.CephFSGRPCMetricsPort, err = getPortFromConfig(r.opConfig.Parameters, "CSI_CEPHFS_GRPC_METRICS_PORT", DefaultCephFSGRPCMerticsPort)
	if err != nil {
		return errors.Wrap(err, "error getting CSI CephFS GRPC metrics port.")
	}
	tp.CephFSLivenessMetricsPort, err = getPortFromConfig(r.opConfig.Parameters, "CSI_CEPHFS_LIVENESS_METRICS_PORT", DefaultCephFSLivenessMerticsPort)
	if err != nil {
		return errors.Wrap(err, "error getting CSI CephFS liveness metrics port.")
	}

	tp.RBDGRPCMetricsPort, err = getPortFromConfig(r.opConfig.Parameters, "CSI_RBD_GRPC_METRICS_PORT", DefaultRBDGRPCMerticsPort)
	if err != nil {
		return errors.Wrap(err, "error getting CSI RBD GRPC metrics port.")
	}
	tp.RBDLivenessMetricsPort, err = getPortFromConfig(r.opConfig.Parameters, "CSI_RBD_LIVENESS_METRICS_PORT", DefaultRBDLivenessMerticsPort)
	if err != nil {
		return errors.Wrap(err, "error getting CSI RBD liveness metrics port.")
	}

	// default value `system-node-critical` is the highest available priority
	tp.PluginPriorityClassName = k8sutil.GetValue(r.opConfig.Parameters, "CSI_PLUGIN_PRIORITY_CLASSNAME", "")

	// default value `system-cluster-critical` is applied for some
	// critical pods in cluster but less priority than plugin pods
	tp.ProvisionerPriorityClassName = k8sutil.GetValue(r.opConfig.Parameters, "CSI_PROVISIONER_PRIORITY_CLASSNAME", "")

	if strings.EqualFold(k8sutil.GetValue(r.opConfig.Parameters, "CSI_ENABLE_OMAP_GENERATOR", "false"), "true") {
		tp.EnableOMAPGenerator = true
	}

	// if k8s >= v1.17 enable RBD and CephFS snapshotter by default
	if ver.Major == KubeMinMajor && ver.Minor >= kubeMinVerForSnapshot {
		tp.EnableRBDSnapshotter = true
		tp.EnableCephFSSnapshotter = true
	}

	if strings.EqualFold(k8sutil.GetValue(r.opConfig.Parameters, "CSI_ENABLE_RBD_SNAPSHOTTER", "true"), "false") {
		tp.EnableRBDSnapshotter = false
	}

	if strings.EqualFold(k8sutil.GetValue(r.opConfig.Parameters, "CSI_ENABLE_CEPHFS_SNAPSHOTTER", "true"), "false") {
		tp.EnableCephFSSnapshotter = false
	}

	tp.EnableVolumeReplicationSideCar = false
	if strings.EqualFold(k8sutil.GetValue(r.opConfig.Parameters, "CSI_ENABLE_VOLUME_REPLICATION", "false"), "true") {
		tp.EnableVolumeReplicationSideCar = true
	}

	if strings.EqualFold(k8sutil.GetValue(r.opConfig.Parameters, "CSI_CEPHFS_PLUGIN_UPDATE_STRATEGY", rollingUpdate), onDelete) {
		tp.CephFSPluginUpdateStrategy = onDelete
	} else {
		tp.CephFSPluginUpdateStrategy = rollingUpdate
	}

	if strings.EqualFold(k8sutil.GetValue(r.opConfig.Parameters, "CSI_RBD_PLUGIN_UPDATE_STRATEGY", rollingUpdate), onDelete) {
		tp.RBDPluginUpdateStrategy = onDelete
	} else {
		tp.RBDPluginUpdateStrategy = rollingUpdate
	}

	if strings.EqualFold(k8sutil.GetValue(r.opConfig.Parameters, "CSI_PLUGIN_ENABLE_SELINUX_HOST_MOUNT", "false"), "true") {
		tp.EnablePluginSelinuxHostMount = true
	}

	logger.Infof("Kubernetes version is %s.%s", ver.Major, ver.Minor)

	tp.ResizerImage = k8sutil.GetValue(r.opConfig.Parameters, "ROOK_CSI_RESIZER_IMAGE", DefaultResizerImage)

	logLevel := k8sutil.GetValue(r.opConfig.Parameters, "CSI_LOG_LEVEL", "")
	tp.LogLevel = defaultLogLevel
	if logLevel != "" {
		l, err := strconv.ParseUint(logLevel, 10, 8)
		if err != nil {
			logger.Errorf("failed to parse CSI_LOG_LEVEL. Defaulting to %d. %v", defaultLogLevel, err)
		} else {
			tp.LogLevel = uint8(l)
		}
	}

	tp.ProvisionerReplicas = defaultProvisionerReplicas
	nodes, err := r.context.Clientset.CoreV1().Nodes().List(r.opManagerContext, metav1.ListOptions{})
	if err == nil {
		if len(nodes.Items) == 1 {
			tp.ProvisionerReplicas = 1
		} else {
			replicas := k8sutil.GetValue(r.opConfig.Parameters, "CSI_PROVISIONER_REPLICAS", "2")
			r, err := strconv.ParseInt(replicas, 10, 32)
			if err != nil {
				logger.Errorf("failed to parse CSI_PROVISIONER_REPLICAS. Defaulting to %d. %v", defaultProvisionerReplicas, err)
			} else {
				tp.ProvisionerReplicas = int32(r)
			}
		}
	} else {
		logger.Errorf("failed to get nodes. Defaulting the number of replicas of provisioner pods to %d. %v", tp.ProvisionerReplicas, err)
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
		rbdService.Namespace = r.opConfig.OperatorNamespace
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
		cephfsService.Namespace = r.opConfig.OperatorNamespace
	}

	// get common provisioner tolerations and node affinity
	provisionerTolerations := getToleration(r.opConfig.Parameters, provisionerTolerationsEnv, []corev1.Toleration{})
	provisionerNodeAffinity := getNodeAffinity(r.opConfig.Parameters, provisionerNodeAffinityEnv, &corev1.NodeAffinity{})
	// get common plugin tolerations and node affinity
	pluginTolerations := getToleration(r.opConfig.Parameters, pluginTolerationsEnv, []corev1.Toleration{})
	pluginNodeAffinity := getNodeAffinity(r.opConfig.Parameters, pluginNodeAffinityEnv, &corev1.NodeAffinity{})

	if rbdPlugin != nil {
		// get RBD plugin tolerations and node affinity, defaults to common tolerations and node affinity if not specified
		rbdPluginTolerations := getToleration(r.opConfig.Parameters, rbdPluginTolerationsEnv, pluginTolerations)
		rbdPluginNodeAffinity := getNodeAffinity(r.opConfig.Parameters, rbdPluginNodeAffinityEnv, pluginNodeAffinity)
		// apply RBD plugin tolerations and node affinity
		applyToPodSpec(&rbdPlugin.Spec.Template.Spec, rbdPluginNodeAffinity, rbdPluginTolerations)
		// apply resource request and limit to rbdplugin containers
		applyResourcesToContainers(r.opConfig.Parameters, rbdPluginResource, &rbdPlugin.Spec.Template.Spec)
		err = ownerInfo.SetControllerReference(rbdPlugin)
		if err != nil {
			return errors.Wrapf(err, "failed to set owner reference to rbd plugin daemonset %q", rbdPlugin.Name)
		}
		multusApplied, err := r.applyCephClusterNetworkConfig(r.opManagerContext, &rbdPlugin.Spec.Template.ObjectMeta)
		if err != nil {
			return errors.Wrapf(err, "failed to apply network config to rbd plugin daemonset %q", rbdPlugin.Name)
		}
		if multusApplied {
			rbdPlugin.Spec.Template.Spec.HostNetwork = false
		}
		err = k8sutil.CreateDaemonSet(r.opManagerContext, csiRBDPlugin, r.opConfig.OperatorNamespace, r.context.Clientset, rbdPlugin)
		if err != nil {
			return errors.Wrapf(err, "failed to start rbdplugin daemonset %q", rbdPlugin.Name)
		}
		k8sutil.AddRookVersionLabelToDaemonSet(rbdPlugin)
	}

	if rbdProvisionerDeployment != nil {
		// get RBD provisioner tolerations and node affinity, defaults to common tolerations and node affinity if not specified
		rbdProvisionerTolerations := getToleration(r.opConfig.Parameters, rbdProvisionerTolerationsEnv, provisionerTolerations)
		rbdProvisionerNodeAffinity := getNodeAffinity(r.opConfig.Parameters, rbdProvisionerNodeAffinityEnv, provisionerNodeAffinity)
		// apply RBD provisioner tolerations and node affinity
		applyToPodSpec(&rbdProvisionerDeployment.Spec.Template.Spec, rbdProvisionerNodeAffinity, rbdProvisionerTolerations)
		// apply resource request and limit to rbd provisioner containers
		applyResourcesToContainers(r.opConfig.Parameters, rbdProvisionerResource, &rbdProvisionerDeployment.Spec.Template.Spec)
		err = ownerInfo.SetControllerReference(rbdProvisionerDeployment)
		if err != nil {
			return errors.Wrapf(err, "failed to set owner reference to rbd provisioner deployment %q", rbdProvisionerDeployment.Name)
		}
		antiAffinity := GetPodAntiAffinity("app", csiRBDProvisioner)
		rbdProvisionerDeployment.Spec.Template.Spec.Affinity.PodAntiAffinity = &antiAffinity
		rbdProvisionerDeployment.Spec.Strategy = apps.DeploymentStrategy{
			Type: apps.RecreateDeploymentStrategyType,
		}

		_, err = r.applyCephClusterNetworkConfig(r.opManagerContext, &rbdProvisionerDeployment.Spec.Template.ObjectMeta)
		if err != nil {
			return errors.Wrapf(err, "failed to apply network config to rbd plugin provisioner deployment %q", rbdProvisionerDeployment.Name)
		}
		_, err = k8sutil.CreateOrUpdateDeployment(r.opManagerContext, r.context.Clientset, rbdProvisionerDeployment)
		if err != nil {
			return errors.Wrapf(err, "failed to start rbd provisioner deployment %q", rbdProvisionerDeployment.Name)
		}
		k8sutil.AddRookVersionLabelToDeployment(rbdProvisionerDeployment)
		logger.Info("successfully started CSI Ceph RBD driver")
	}

	if rbdService != nil {
		rbdService.Namespace = r.opConfig.OperatorNamespace
		err = ownerInfo.SetControllerReference(rbdService)
		if err != nil {
			return errors.Wrapf(err, "failed to set owner reference to rbd service %q", rbdService)
		}
		_, err = k8sutil.CreateOrUpdateService(r.context.Clientset, r.opConfig.OperatorNamespace, rbdService)
		if err != nil {
			return errors.Wrapf(err, "failed to create rbd service %q", rbdService.Name)
		}
	}

	if cephfsPlugin != nil {
		// get CephFS plugin tolerations and node affinity, defaults to common tolerations and node affinity if not specified
		cephFSPluginTolerations := getToleration(r.opConfig.Parameters, cephFSPluginTolerationsEnv, pluginTolerations)
		cephFSPluginNodeAffinity := getNodeAffinity(r.opConfig.Parameters, cephFSPluginNodeAffinityEnv, pluginNodeAffinity)
		// apply CephFS plugin tolerations and node affinity
		applyToPodSpec(&cephfsPlugin.Spec.Template.Spec, cephFSPluginNodeAffinity, cephFSPluginTolerations)
		// apply resource request and limit to cephfs plugin containers
		applyResourcesToContainers(r.opConfig.Parameters, cephFSPluginResource, &cephfsPlugin.Spec.Template.Spec)
		err = ownerInfo.SetControllerReference(cephfsPlugin)
		if err != nil {
			return errors.Wrapf(err, "failed to set owner reference to cephfs plugin daemonset %q", cephfsPlugin.Name)
		}
		multusApplied, err := r.applyCephClusterNetworkConfig(r.opManagerContext, &cephfsPlugin.Spec.Template.ObjectMeta)
		if err != nil {
			return errors.Wrapf(err, "failed to apply network config to cephfs plugin daemonset %q", cephfsPlugin.Name)
		}
		if multusApplied {
			cephfsPlugin.Spec.Template.Spec.HostNetwork = false
		}
		err = k8sutil.CreateDaemonSet(r.opManagerContext, csiCephFSPlugin, r.opConfig.OperatorNamespace, r.context.Clientset, cephfsPlugin)
		if err != nil {
			return errors.Wrapf(err, "failed to start cephfs plugin daemonset %q", cephfsPlugin.Name)
		}
		k8sutil.AddRookVersionLabelToDaemonSet(cephfsPlugin)
	}

	if cephfsProvisionerDeployment != nil {
		// get CephFS provisioner tolerations and node affinity, defaults to common tolerations and node affinity if not specified
		cephFSProvisionerTolerations := getToleration(r.opConfig.Parameters, cephFSProvisionerTolerationsEnv, provisionerTolerations)
		cephFSProvisionerNodeAffinity := getNodeAffinity(r.opConfig.Parameters, cephFSProvisionerNodeAffinityEnv, provisionerNodeAffinity)
		// apply CephFS provisioner tolerations and node affinity
		applyToPodSpec(&cephfsProvisionerDeployment.Spec.Template.Spec, cephFSProvisionerNodeAffinity, cephFSProvisionerTolerations)
		// get resource details for cephfs provisioner
		// apply resource request and limit to cephfs provisioner containers
		applyResourcesToContainers(r.opConfig.Parameters, cephFSProvisionerResource, &cephfsProvisionerDeployment.Spec.Template.Spec)
		err = ownerInfo.SetControllerReference(cephfsProvisionerDeployment)
		if err != nil {
			return errors.Wrapf(err, "failed to set owner reference to cephfs provisioner deployment %q", cephfsProvisionerDeployment.Name)
		}
		antiAffinity := GetPodAntiAffinity("app", csiCephFSProvisioner)
		cephfsProvisionerDeployment.Spec.Template.Spec.Affinity.PodAntiAffinity = &antiAffinity
		cephfsProvisionerDeployment.Spec.Strategy = apps.DeploymentStrategy{
			Type: apps.RecreateDeploymentStrategyType,
		}

		_, err = r.applyCephClusterNetworkConfig(r.opManagerContext, &cephfsProvisionerDeployment.Spec.Template.ObjectMeta)
		if err != nil {
			return errors.Wrapf(err, "failed to apply network config to cephfs plugin provisioner deployment %q", cephfsProvisionerDeployment.Name)
		}
		_, err = k8sutil.CreateOrUpdateDeployment(r.opManagerContext, r.context.Clientset, cephfsProvisionerDeployment)
		if err != nil {
			return errors.Wrapf(err, "failed to start cephfs provisioner deployment %q", cephfsProvisionerDeployment.Name)
		}
		k8sutil.AddRookVersionLabelToDeployment(cephfsProvisionerDeployment)
		logger.Info("successfully started CSI CephFS driver")
	}
	if cephfsService != nil {
		err = ownerInfo.SetControllerReference(cephfsService)
		if err != nil {
			return errors.Wrapf(err, "failed to set owner reference to cephfs service %q", cephfsService)
		}
		_, err = k8sutil.CreateOrUpdateService(r.context.Clientset, r.opConfig.OperatorNamespace, cephfsService)
		if err != nil {
			return errors.Wrapf(err, "failed to create cephfs service %q", cephfsService.Name)
		}
	}

	if EnableRBD {
		err = csiDriverobj.createCSIDriverInfo(r.opManagerContext, r.context.Clientset, RBDDriverName, k8sutil.GetValue(r.opConfig.Parameters, "CSI_RBD_FSGROUPPOLICY", string(k8scsi.ReadWriteOnceWithFSTypeFSGroupPolicy)))
		if err != nil {
			return errors.Wrapf(err, "failed to create CSI driver object for %q", RBDDriverName)
		}
	}
	if EnableCephFS {
		err = csiDriverobj.createCSIDriverInfo(r.opManagerContext, r.context.Clientset, CephFSDriverName, k8sutil.GetValue(r.opConfig.Parameters, "CSI_CEPHFS_FSGROUPPOLICY", string(k8scsi.ReadWriteOnceWithFSTypeFSGroupPolicy)))
		if err != nil {
			return errors.Wrapf(err, "failed to create CSI driver object for %q", CephFSDriverName)
		}
	}

	return nil
}

func (r *ReconcileCSI) stopDrivers(ver *version.Info) {
	if !EnableRBD {
		logger.Info("CSI Ceph RBD driver disabled")
		succeeded := r.deleteCSIDriverResources(ver, csiRBDPlugin, csiRBDProvisioner, "csi-rbdplugin-metrics", RBDDriverName)
		if succeeded {
			logger.Info("successfully removed CSI Ceph RBD driver")
		} else {
			logger.Error("failed to remove CSI Ceph RBD driver")
		}
	}

	if !EnableCephFS {
		logger.Info("CSI CephFS driver disabled")
		succeeded := r.deleteCSIDriverResources(ver, csiCephFSPlugin, csiCephFSProvisioner, "csi-cephfsplugin-metrics", CephFSDriverName)
		if succeeded {
			logger.Info("successfully removed CSI CephFS driver")
		} else {
			logger.Error("failed to remove CSI CephFS driver")
		}
	}
}

func (r *ReconcileCSI) deleteCSIDriverResources(ver *version.Info, daemonset, deployment, service, driverName string) bool {
	succeeded := true
	csiDriverobj = beta1CsiDriver{}
	if ver.Major > KubeMinMajor || ver.Major == KubeMinMajor && ver.Minor >= kubeMinVerForV1csiDriver {
		csiDriverobj = v1CsiDriver{}
	}
	err := k8sutil.DeleteDaemonset(r.opManagerContext, r.context.Clientset, r.opConfig.OperatorNamespace, daemonset)
	if err != nil {
		logger.Errorf("failed to delete the %q. %v", daemonset, err)
		succeeded = false
	}

	err = k8sutil.DeleteDeployment(r.opManagerContext, r.context.Clientset, r.opConfig.OperatorNamespace, deployment)
	if err != nil {
		logger.Errorf("failed to delete the %q. %v", deployment, err)
		succeeded = false
	}

	err = k8sutil.DeleteService(r.context.Clientset, r.opConfig.OperatorNamespace, service)
	if err != nil {
		logger.Errorf("failed to delete the %q. %v", service, err)
		succeeded = false
	}

	err = csiDriverobj.deleteCSIDriverInfo(r.opManagerContext, r.context.Clientset, driverName)
	if err != nil {
		logger.Errorf("failed to delete %q Driver Info. %v", driverName, err)
		succeeded = false
	}
	return succeeded
}

func (r *ReconcileCSI) applyCephClusterNetworkConfig(ctx context.Context, objectMeta *metav1.ObjectMeta) (bool, error) {
	var isMultusApplied bool
	cephClusters, err := r.context.RookClientset.CephV1().CephClusters(objectMeta.Namespace).List(ctx, metav1.ListOptions{})
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
func (r *ReconcileCSI) validateCSIVersion(ownerInfo *k8sutil.OwnerInfo) (*CephCSIVersion, error) {
	timeout := 15 * time.Minute

	logger.Infof("detecting the ceph csi image version for image %q", CSIParam.CSIPluginImage)

	versionReporter, err := cmdreporter.New(
		r.context.Clientset,
		ownerInfo,
		detectCSIVersionName,
		detectCSIVersionName,
		r.opConfig.OperatorNamespace,
		[]string{"cephcsi"},
		[]string{"--version"},
		r.opConfig.Image,
		CSIParam.CSIPluginImage,
	)

	if err != nil {
		return nil, errors.Wrap(err, "failed to set up ceph CSI version job")
	}

	job := versionReporter.Job()
	job.Spec.Template.Spec.ServiceAccountName = r.opConfig.ServiceAccount

	// Apply csi provisioner toleration and affinity for csi version check job
	job.Spec.Template.Spec.Tolerations = getToleration(r.opConfig.Parameters, provisionerTolerationsEnv, []corev1.Toleration{})
	job.Spec.Template.Spec.Affinity = &corev1.Affinity{
		NodeAffinity: getNodeAffinity(r.opConfig.Parameters, provisionerNodeAffinityEnv, &corev1.NodeAffinity{}),
	}
	stdout, _, retcode, err := versionReporter.Run(r.opManagerContext, timeout)
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
