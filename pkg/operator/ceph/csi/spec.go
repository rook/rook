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
	"fmt"
	"strconv"
	"strings"
	"time"

	controllerutil "github.com/rook/rook/pkg/operator/ceph/controller"

	"github.com/pkg/errors"

	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/k8sutil/cmdreporter"

	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8scsi "k8s.io/api/storage/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes"
)

type Param struct {
	CSIPluginImage               string
	RegistrarImage               string
	ProvisionerImage             string
	AttacherImage                string
	SnapshotterImage             string
	ResizerImage                 string
	DriverNamePrefix             string
	EnableSnapshotter            string
	EnableCSIGRPCMetrics         string
	KubeletDirPath               string
	ForceCephFSKernelClient      string
	CephFSPluginUpdateStrategy   string
	RBDPluginUpdateStrategy      string
	PluginPriorityClassName      string
	ProvisionerPriorityClassName string
	LogLevel                     uint8
	CephFSGRPCMetricsPort        uint16
	CephFSLivenessMetricsPort    uint16
	RBDGRPCMetricsPort           uint16
	RBDLivenessMetricsPort       uint16
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

	// template paths
	RBDPluginTemplatePath         string
	RBDProvisionerSTSTemplatePath string
	RBDProvisionerDepTemplatePath string

	CephFSPluginTemplatePath         string
	CephFSProvisionerSTSTemplatePath string
	CephFSProvisionerDepTemplatePath string

	// configuration map for csi
	ConfigName = "rook-ceph-csi-config"
	ConfigKey  = "csi-cluster-config-json"
)

// Specify default images as var instead of const so that they can be overridden with the Go
// linker's -X flag. This allows users to easily build images with a different opinionated set of
// images without having to specify them manually in charts/manifests which can make upgrades more
// manually challenging.
var (
	// image names
	DefaultCSIPluginImage   = "quay.io/cephcsi/cephcsi:v2.1.2"
	DefaultRegistrarImage   = "quay.io/k8scsi/csi-node-driver-registrar:v1.2.0"
	DefaultProvisionerImage = "quay.io/k8scsi/csi-provisioner:v1.4.0"
	DefaultAttacherImage    = "quay.io/k8scsi/csi-attacher:v2.1.0"
	DefaultSnapshotterImage = "quay.io/k8scsi/csi-snapshotter:v1.2.2"
	defaultResizerImage     = "quay.io/k8scsi/csi-resizer:v0.4.0"
)

const (
	KubeMinMajor                   = "1"
	KubeMinMinor                   = "13"
	provDeploymentSuppVersion      = "14"
	kubeMinVerForFilesystemRestore = "15"
	kubeMinVerForBlockRestore      = "16"

	// toleration and node affinity
	provisionerTolerationsEnv  = "CSI_PROVISIONER_TOLERATIONS"
	provisionerNodeAffinityEnv = "CSI_PROVISIONER_NODE_AFFINITY"
	pluginTolerationsEnv       = "CSI_PLUGIN_TOLERATIONS"
	pluginNodeAffinityEnv      = "CSI_PLUGIN_NODE_AFFINITY"

	// compute resource for CSI pods
	rbdProvisionerResource = "CSI_RBD_PROVISIONER_RESOURCE"
	rbdPluginResource      = "CSI_RBD_PLUGIN_RESOURCE"

	cephFSProvisionerResource = "CSI_CEPHFS_PROVISIONER_RESOURCE"
	cephFSPluginResource      = "CSI_CEPHFS_PLUGIN_RESOURCE"

	// kubelet directory path
	DefaultKubeletDirPath = "/var/lib/kubelet"

	// template
	DefaultRBDPluginTemplatePath         = "/etc/ceph-csi/rbd/csi-rbdplugin.yaml"
	DefaultRBDProvisionerSTSTemplatePath = "/etc/ceph-csi/rbd/csi-rbdplugin-provisioner-sts.yaml"
	DefaultRBDProvisionerDepTemplatePath = "/etc/ceph-csi/rbd/csi-rbdplugin-provisioner-dep.yaml"
	DefaultRBDPluginServiceTemplatePath  = "/etc/ceph-csi/rbd/csi-rbdplugin-svc.yaml"

	DefaultCephFSPluginTemplatePath         = "/etc/ceph-csi/cephfs/csi-cephfsplugin.yaml"
	DefaultCephFSProvisionerSTSTemplatePath = "/etc/ceph-csi/cephfs/csi-cephfsplugin-provisioner-sts.yaml"
	DefaultCephFSProvisionerDepTemplatePath = "/etc/ceph-csi/cephfs/csi-cephfsplugin-provisioner-dep.yaml"
	DefaultCephFSPluginServiceTemplatePath  = "/etc/ceph-csi/cephfs/csi-cephfsplugin-svc.yaml"

	// grpc metrics and liveness port for cephfs  and rbd
	DefaultCephFSGRPCMerticsPort     uint16 = 9091
	DefaultCephFSLivenessMerticsPort uint16 = 9081
	DefaultRBDGRPCMerticsPort        uint16 = 9090
	DefaultRBDLivenessMerticsPort    uint16 = 9080

	detectCSIVersionName   = "rook-ceph-csi-detect-version"
	operatorDeploymentName = "rook-ceph-operator"
	// default log level for csi containers
	defaultLogLevel uint8 = 0

	// update strategy
	rollingUpdate = "RollingUpdate"
	onDelete      = "OnDelete"
)

func CSIEnabled() bool {
	return EnableRBD || EnableCephFS
}

func ValidateCSIParam() error {

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

	if EnableRBD {
		if len(RBDPluginTemplatePath) == 0 {
			return errors.New("missing rbd plugin template path")
		}
		if len(RBDProvisionerSTSTemplatePath) == 0 && len(RBDProvisionerDepTemplatePath) == 0 {
			return errors.New("missing rbd provisioner template path")
		}
	}

	if EnableCephFS {
		if len(CephFSPluginTemplatePath) == 0 {
			return errors.New("missing cephfs plugin template path")
		}
		if len(CephFSProvisionerSTSTemplatePath) == 0 && len(CephFSProvisionerDepTemplatePath) == 0 {
			return errors.New("missing ceph provisioner template path")
		}
	}
	return nil
}

func startDrivers(namespace string, clientset kubernetes.Interface, ver *version.Info, ownerRef *metav1.OwnerReference) error {
	var (
		err                                                   error
		rbdPlugin, cephfsPlugin                               *apps.DaemonSet
		rbdProvisionerSTS, cephfsProvisionerSTS               *apps.StatefulSet
		rbdProvisionerDeployment, cephfsProvisionerDeployment *apps.Deployment
		deployProvSTS                                         bool
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

	enableSnap, err := k8sutil.GetOperatorSetting(clientset, controllerutil.OperatorSettingConfigMapName, "CSI_ENABLE_SNAPSHOTTER", "true")
	if err != nil {
		return errors.Wrap(err, "failed to load CSI_ENABLE_SNAPSHOTTER setting")
	}
	if !strings.EqualFold(enableSnap, "false") {
		tp.EnableSnapshotter = "true"
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

	if ver.Major > KubeMinMajor || (ver.Major == KubeMinMajor && ver.Minor < provDeploymentSuppVersion) {
		deployProvSTS = true
	}

	tp.ResizerImage, err = k8sutil.GetOperatorSetting(clientset, controllerutil.OperatorSettingConfigMapName, "ROOK_CSI_RESIZER_IMAGE", defaultResizerImage)
	if err != nil {
		return errors.Wrap(err, "failed to load ROOK_CSI_RESIZER_IMAGE setting")
	}
	if tp.ResizerImage == "" {
		tp.ResizerImage = defaultResizerImage
	}

	if ver.Major < KubeMinMajor || ver.Major == KubeMinMajor && ver.Minor < kubeMinVerForFilesystemRestore {
		logger.Warning("CSI Filesystem volume expansion requires Kubernetes version >=1.15.0")
	}
	if ver.Major < KubeMinMajor || ver.Major == KubeMinMajor && ver.Minor < kubeMinVerForBlockRestore {
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

	if EnableRBD {
		rbdPlugin, err = templateToDaemonSet("rbdplugin", RBDPluginTemplatePath, tp)
		if err != nil {
			return errors.Wrapf(err, "failed to load rbdplugin template")
		}
		if deployProvSTS {
			rbdProvisionerSTS, err = templateToStatefulSet("rbd-provisioner", RBDProvisionerSTSTemplatePath, tp)
			if err != nil {
				return errors.Wrapf(err, "failed to load rbd provisioner statefulset template")
			}
		} else {
			rbdProvisionerDeployment, err = templateToDeployment("rbd-provisioner", RBDProvisionerDepTemplatePath, tp)
			if err != nil {
				return errors.Wrapf(err, "failed to load rbd provisioner deployment template")
			}
		}
		rbdService, err = templateToService("rbd-service", DefaultRBDPluginServiceTemplatePath, tp)
		if err != nil {
			return errors.Wrapf(err, "failed to load rbd plugin service template")
		}
	}
	if EnableCephFS {
		cephfsPlugin, err = templateToDaemonSet("cephfsplugin", CephFSPluginTemplatePath, tp)
		if err != nil {
			return errors.Wrapf(err, "failed to load CephFS plugin template")
		}
		if deployProvSTS {
			cephfsProvisionerSTS, err = templateToStatefulSet("cephfs-provisioner", CephFSProvisionerSTSTemplatePath, tp)
			if err != nil {
				return errors.Wrapf(err, "failed to load CephFS provisioner statefulset template")
			}
		} else {
			cephfsProvisionerDeployment, err = templateToDeployment("cephfs-provisioner", CephFSProvisionerDepTemplatePath, tp)
			if err != nil {
				return errors.Wrapf(err, "failed to load rbd provisioner deployment template")
			}
		}
		cephfsService, err = templateToService("cephfs-service", DefaultCephFSPluginServiceTemplatePath, tp)
		if err != nil {
			return errors.Wrapf(err, "failed to load cephfs plugin service template")
		}
	}
	// get provisioner toleration and node affinity
	provisionerTolerations := getToleration(clientset, true)
	provisionerNodeAffinity := getNodeAffinity(clientset, true)
	// get plugin toleration and node affinity
	pluginTolerations := getToleration(clientset, false)
	pluginNodeAffinity := getNodeAffinity(clientset, false)
	if rbdPlugin != nil {
		applyToPodSpec(&rbdPlugin.Spec.Template.Spec, pluginNodeAffinity, pluginTolerations)
		// apply resource request and limit to rbdplugin containers
		applyResourcesToContainers(clientset, rbdPluginResource, &rbdPlugin.Spec.Template.Spec)
		k8sutil.SetOwnerRef(&rbdPlugin.ObjectMeta, ownerRef)
		err = k8sutil.CreateDaemonSet("csi-rbdplugin", namespace, clientset, rbdPlugin)
		if err != nil {
			return errors.Wrapf(err, "failed to start rbdplugin daemonset: %+v", rbdPlugin)
		}
		k8sutil.AddRookVersionLabelToDaemonSet(rbdPlugin)
	}

	if rbdProvisionerSTS != nil {
		applyToPodSpec(&rbdProvisionerSTS.Spec.Template.Spec, provisionerNodeAffinity, provisionerTolerations)
		// apply resource request and limit to rbd provisioner containers
		applyResourcesToContainers(clientset, rbdProvisionerResource, &rbdProvisionerSTS.Spec.Template.Spec)
		k8sutil.SetOwnerRef(&rbdProvisionerSTS.ObjectMeta, ownerRef)
		err = k8sutil.CreateStatefulSet(clientset, "csi-rbdplugin-provisioner", namespace, rbdProvisionerSTS)
		if err != nil {
			return errors.Wrapf(err, "failed to start rbd provisioner statefulset: %+v", rbdProvisionerSTS)
		}
		k8sutil.AddRookVersionLabelToStatefulSet(rbdProvisionerSTS)
	} else if rbdProvisionerDeployment != nil {
		applyToPodSpec(&rbdProvisionerDeployment.Spec.Template.Spec, provisionerNodeAffinity, provisionerTolerations)
		// apply resource request and limit to rbd provisioner containers
		applyResourcesToContainers(clientset, rbdProvisionerResource, &rbdProvisionerDeployment.Spec.Template.Spec)
		k8sutil.SetOwnerRef(&rbdProvisionerDeployment.ObjectMeta, ownerRef)
		antiAffinity := getPodAntiAffinity("app", "csi-rbdplugin-provisioner")
		rbdProvisionerDeployment.Spec.Template.Spec.Affinity.PodAntiAffinity = &antiAffinity
		rbdProvisionerDeployment.Spec.Strategy = apps.DeploymentStrategy{
			Type: apps.RecreateDeploymentStrategyType,
		}

		err = k8sutil.CreateDeployment(clientset, "csi-rbdplugin-provisioner", namespace, rbdProvisionerDeployment)
		if err != nil {
			return errors.Wrapf(err, "failed to start rbd provisioner deployment: %+v", rbdProvisionerDeployment)
		}
		k8sutil.AddRookVersionLabelToDeployment(rbdProvisionerDeployment)
	}

	if rbdService != nil {
		k8sutil.SetOwnerRef(&rbdService.ObjectMeta, ownerRef)
		_, err = k8sutil.CreateOrUpdateService(clientset, namespace, rbdService)
		if err != nil {
			return errors.Wrapf(err, "failed to create rbd service: %+v", rbdService)
		}
	}

	if cephfsPlugin != nil {
		applyToPodSpec(&cephfsPlugin.Spec.Template.Spec, pluginNodeAffinity, pluginTolerations)
		// apply resource request and limit to cephfs plugin containers
		applyResourcesToContainers(clientset, cephFSPluginResource, &cephfsPlugin.Spec.Template.Spec)
		k8sutil.SetOwnerRef(&cephfsPlugin.ObjectMeta, ownerRef)
		err = k8sutil.CreateDaemonSet("csi-cephfsplugin", namespace, clientset, cephfsPlugin)
		if err != nil {
			return errors.Wrapf(err, "failed to start cephfs plugin daemonset: %+v", cephfsPlugin)
		}
		k8sutil.AddRookVersionLabelToDaemonSet(cephfsPlugin)
	}

	if cephfsProvisionerSTS != nil {
		applyToPodSpec(&cephfsProvisionerSTS.Spec.Template.Spec, provisionerNodeAffinity, provisionerTolerations)
		// apply resource request and limit to cephfs provisioner containers
		applyResourcesToContainers(clientset, cephFSProvisionerResource, &cephfsProvisionerSTS.Spec.Template.Spec)
		k8sutil.SetOwnerRef(&cephfsProvisionerSTS.ObjectMeta, ownerRef)
		err = k8sutil.CreateStatefulSet(clientset, "csi-cephfsplugin-provisioner", namespace, cephfsProvisionerSTS)
		if err != nil {
			return errors.Wrapf(err, "failed to start cephfs provisioner statefulset: %+v", cephfsProvisionerSTS)
		}
		k8sutil.AddRookVersionLabelToStatefulSet(cephfsProvisionerSTS)

	} else if cephfsProvisionerDeployment != nil {
		applyToPodSpec(&cephfsProvisionerDeployment.Spec.Template.Spec, provisionerNodeAffinity, provisionerTolerations)
		// get resource details for cephfs provisioner
		// apply resource request and limit to cephfs provisioner containers
		applyResourcesToContainers(clientset, cephFSProvisionerResource, &cephfsProvisionerDeployment.Spec.Template.Spec)
		k8sutil.SetOwnerRef(&cephfsProvisionerDeployment.ObjectMeta, ownerRef)

		antiAffinity := getPodAntiAffinity("app", "csi-cephfsplugin-provisioner")
		cephfsProvisionerDeployment.Spec.Template.Spec.Affinity.PodAntiAffinity = &antiAffinity
		cephfsProvisionerDeployment.Spec.Strategy = apps.DeploymentStrategy{
			Type: apps.RecreateDeploymentStrategyType,
		}
		err = k8sutil.CreateDeployment(clientset, "csi-cephfsplugin-provisioner", namespace, cephfsProvisionerDeployment)
		if err != nil {
			return errors.Wrapf(err, "failed to start cephfs provisioner deployment: %+v", cephfsProvisionerDeployment)
		}
		k8sutil.AddRookVersionLabelToDeployment(cephfsProvisionerDeployment)
	}
	if cephfsService != nil {
		k8sutil.SetOwnerRef(&cephfsService.ObjectMeta, ownerRef)
		_, err = k8sutil.CreateOrUpdateService(clientset, namespace, cephfsService)
		if err != nil {
			return errors.Wrapf(err, "failed to create rbd service: %+v", cephfsService)
		}
	}

	if ver.Major > KubeMinMajor || (ver.Major == KubeMinMajor && ver.Minor >= provDeploymentSuppVersion) {
		err = createCSIDriverInfo(clientset, RBDDriverName)
		if err != nil {
			return errors.Wrapf(err, "failed to create CSI driver object for %q", RBDDriverName)
		}
		err = createCSIDriverInfo(clientset, CephFSDriverName)
		if err != nil {
			return errors.Wrapf(err, "failed to create CSI driver object for %q", CephFSDriverName)
		}
	}
	return nil
}

func StopCSIDrivers(namespace string, clientset kubernetes.Interface) error {
	logger.Warning("Skipped removing the CSI driver")

	// FIX: Restore the ownerRefs for all CSI resources in order to delegate entirely to the ConfigMap deletion.
	return DeleteCsiConfigMap(namespace, clientset)
}

// createCSIDriverInfo Registers CSI driver by creating a CSIDriver object
func createCSIDriverInfo(clientset kubernetes.Interface, name string) error {
	attach := true
	mountInfo := false
	// Create CSIDriver object
	csiDriver := &k8scsi.CSIDriver{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: k8scsi.CSIDriverSpec{
			AttachRequired: &attach,
			PodInfoOnMount: &mountInfo,
		},
	}
	csidrivers := clientset.StorageV1beta1().CSIDrivers()
	_, err := csidrivers.Create(csiDriver)
	if err == nil {
		logger.Infof("CSIDriver object created for driver %q", name)
		return nil
	}
	if apierrors.IsAlreadyExists(err) {
		logger.Infof("CSIDriver CRD already had been registered for %q", name)
		return nil
	}

	return err
}

// ValidateCSIVersion checks if the configured ceph-csi image is supported
func validateCSIVersion(clientset kubernetes.Interface, namespace, rookImage, serviceAccountName string, ownerRef *metav1.OwnerReference) error {
	timeout := 15 * time.Minute

	logger.Infof("detecting the ceph csi image version for image %q", CSIParam.CSIPluginImage)

	versionReporter, err := cmdreporter.New(
		clientset,
		ownerRef,
		detectCSIVersionName, detectCSIVersionName, namespace,
		[]string{"cephcsi"}, []string{"--version"},
		rookImage, CSIParam.CSIPluginImage)

	if err != nil {
		return errors.Wrap(err, "failed to set up ceph CSI version job")
	}

	job := versionReporter.Job()
	job.Spec.Template.Spec.ServiceAccountName = serviceAccountName

	// Apply csi provisioner toleration for csi version check job
	job.Spec.Template.Spec.Tolerations = getToleration(clientset, true)
	stdout, _, retcode, err := versionReporter.Run(timeout)
	if err != nil {
		return errors.Wrap(err, "failed to complete ceph CSI version job")
	}

	if retcode != 0 {
		return errors.Errorf("ceph CSI version job returned %d", retcode)
	}

	version, err := extractCephCSIVersion(stdout)
	if err != nil {
		if AllowUnsupported {
			logger.Infof("failed to extract csi version, but continuing since unsupported versions are allowed. %v", err)
			return nil
		}
		return errors.Wrap(err, "failed to extract ceph CSI version")
	}
	logger.Infof("Detected ceph CSI image version: %q", version)

	if !version.Supported() {
		return errors.Errorf("ceph CSI image needs to be at least version %q", minimum.String())
	}
	return nil
}
