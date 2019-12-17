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
	"os"
	"strings"

	"github.com/pkg/errors"

	"github.com/rook/rook/pkg/operator/k8sutil"

	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8scsi "k8s.io/api/storage/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes"
)

type Param struct {
	CSIPluginImage             string
	RegistrarImage             string
	ProvisionerImage           string
	AttacherImage              string
	SnapshotterImage           string
	DriverNamePrefix           string
	EnableSnapshotter          string
	EnableCSIGRPCMetrics       string
	KubeletDirPath             string
	ForceCephFSKernelClient    string
	CephFSPluginUpdateStrategy string
	RBDPluginUpdateStrategy    string
	CephFSGRPCMetricsPort      uint16
	CephFSLivenessMetricsPort  uint16
	RBDGRPCMetricsPort         uint16
	RBDLivenessMetricsPort     uint16
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
	DefaultCSIPluginImage   = "quay.io/cephcsi/cephcsi:v1.2.2"
	DefaultRegistrarImage   = "quay.io/k8scsi/csi-node-driver-registrar:v1.1.0"
	DefaultProvisionerImage = "quay.io/k8scsi/csi-provisioner:v1.4.0"
	DefaultAttacherImage    = "quay.io/k8scsi/csi-attacher:v1.2.0"
	DefaultSnapshotterImage = "quay.io/k8scsi/csi-snapshotter:v1.2.2"
)

const (
	KubeMinMajor              = "1"
	KubeMinMinor              = "13"
	provDeploymentSuppVersion = "14"

	// toleration and node affinity
	provisionerTolerationsEnv  = "CSI_PROVISIONER_TOLERATIONS"
	provisionerNodeAffinityEnv = "CSI_PROVISIONER_NODE_AFFINITY"
	pluginTolerationsEnv       = "CSI_PLUGIN_TOLERATIONS"
	pluginNodeAffinityEnv      = "CSI_PLUGIN_NODE_AFFINITY"

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

func StartCSIDrivers(namespace string, clientset kubernetes.Interface, ver *version.Info) error {
	var (
		err                                                   error
		rbdPlugin, cephfsPlugin                               *apps.DaemonSet
		rbdProvisionerSTS, cephfsProvisionerSTS               *apps.StatefulSet
		rbdProvisionerDeployment, cephfsProvisionerDeployment *apps.Deployment
		deployProvSTS                                         bool
		rbdService, cephfsService                             *corev1.Service
	)

	// create an empty config map. config map will be filled with data
	// later when clusters have mons
	configMap, err := CreateCsiConfigMap(namespace, clientset)
	if err != nil {
		return errors.Wrapf(err, "failed creating csi config map")
	}
	ownerRef := metav1.OwnerReference{
		APIVersion: "v1",
		Kind:       "ConfigMap",
		Name:       configMap.Name,
		UID:        configMap.UID,
	}

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
	kClinet := os.Getenv("CSI_FORCE_CEPHFS_KERNEL_CLIENT")
	if strings.EqualFold(kClinet, "false") {
		tp.ForceCephFSKernelClient = "false"
	} else {
		tp.ForceCephFSKernelClient = "true"
	}
	// parse GRPC and Liveness ports
	tp.CephFSGRPCMetricsPort = getPortFromENV("CSI_CEPHFS_GRPC_METRICS_PORT", DefaultCephFSGRPCMerticsPort)
	tp.CephFSLivenessMetricsPort = getPortFromENV("CSI_CEPHFS_LIVENESS_METRICS_PORT", DefaultCephFSLivenessMerticsPort)

	tp.RBDGRPCMetricsPort = getPortFromENV("CSI_RBD_GRPC_METRICS_PORT", DefaultRBDGRPCMerticsPort)
	tp.RBDLivenessMetricsPort = getPortFromENV("CSI_RBD_LIVENESS_METRICS_PORT", DefaultRBDLivenessMerticsPort)

	enableSnap := os.Getenv("CSI_ENABLE_SNAPSHOTTER")
	if !strings.EqualFold(enableSnap, "false") {
		tp.EnableSnapshotter = "true"
	}

	updateStrategy := os.Getenv("CSI_CEPHFS_PLUGIN_UPDATE_STRATEGY")
	if strings.EqualFold(updateStrategy, "ondelete") {
		tp.CephFSPluginUpdateStrategy = "OnDelete"
	} else {
		tp.CephFSPluginUpdateStrategy = "RollingUpdate"
	}

	updateStrategy = os.Getenv("CSI_RBD_PLUGIN_UPDATE_STRATEGY")
	if strings.EqualFold(updateStrategy, "ondelete") {
		tp.RBDPluginUpdateStrategy = "OnDelete"
	} else {
		tp.RBDPluginUpdateStrategy = "RollingUpdate"
	}

	if ver.Major > KubeMinMajor || (ver.Major == KubeMinMajor && ver.Minor < provDeploymentSuppVersion) {
		deployProvSTS = true
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
	provisionerTolerations := getToleration(true)
	provisionerNodeAffinity := getNodeAffinity(true)
	// get plugin toleration and node affinity
	pluginTolerations := getToleration(false)
	pluginNodeAffinity := getNodeAffinity(false)
	if rbdPlugin != nil {
		applyToPodSpec(&rbdPlugin.Spec.Template.Spec, pluginNodeAffinity, pluginTolerations)
		k8sutil.SetOwnerRef(&rbdPlugin.ObjectMeta, &ownerRef)
		err = k8sutil.CreateDaemonSet("csi-rbdplugin", namespace, clientset, rbdPlugin)
		if err != nil {
			return errors.Wrapf(err, "failed to start rbdplugin daemonset: %+v", rbdPlugin)
		}
		k8sutil.AddRookVersionLabelToDaemonSet(rbdPlugin)
	}

	if rbdProvisionerSTS != nil {
		applyToPodSpec(&rbdProvisionerSTS.Spec.Template.Spec, provisionerNodeAffinity, provisionerTolerations)
		k8sutil.SetOwnerRef(&rbdProvisionerSTS.ObjectMeta, &ownerRef)
		err = k8sutil.CreateStatefulSet("csi-rbdplugin-provisioner", namespace, clientset, rbdProvisionerSTS)
		if err != nil {
			return errors.Wrapf(err, "failed to start rbd provisioner statefulset: %+v", rbdProvisionerSTS)
		}
		k8sutil.AddRookVersionLabelToStatefulSet(rbdProvisionerSTS)
	} else if rbdProvisionerDeployment != nil {
		applyToPodSpec(&rbdProvisionerDeployment.Spec.Template.Spec, provisionerNodeAffinity, provisionerTolerations)
		k8sutil.SetOwnerRef(&rbdProvisionerDeployment.ObjectMeta, &ownerRef)
		err = k8sutil.CreateDeployment("csi-rbdplugin-provisioner", namespace, clientset, rbdProvisionerDeployment)
		if err != nil {
			return errors.Wrapf(err, "failed to start rbd provisioner deployment: %+v", rbdProvisionerDeployment)
		}
		k8sutil.AddRookVersionLabelToDeployment(rbdProvisionerDeployment)
	}

	if rbdService != nil {
		k8sutil.SetOwnerRef(&rbdService.ObjectMeta, &ownerRef)
		_, err = k8sutil.CreateOrUpdateService(clientset, namespace, rbdService)
		if err != nil {
			return errors.Wrapf(err, "failed to create rbd service: %+v", rbdService)
		}
	}

	if cephfsPlugin != nil {
		applyToPodSpec(&cephfsPlugin.Spec.Template.Spec, pluginNodeAffinity, pluginTolerations)
		k8sutil.SetOwnerRef(&cephfsPlugin.ObjectMeta, &ownerRef)
		err = k8sutil.CreateDaemonSet("csi-cephfsplugin", namespace, clientset, cephfsPlugin)
		if err != nil {
			return errors.Wrapf(err, "failed to start cephfs plugin daemonset: %+v", cephfsPlugin)
		}
		k8sutil.AddRookVersionLabelToDaemonSet(cephfsPlugin)
	}

	if cephfsProvisionerSTS != nil {
		applyToPodSpec(&cephfsProvisionerSTS.Spec.Template.Spec, provisionerNodeAffinity, provisionerTolerations)
		k8sutil.SetOwnerRef(&cephfsProvisionerSTS.ObjectMeta, &ownerRef)
		err = k8sutil.CreateStatefulSet("csi-cephfsplugin-provisioner", namespace, clientset, cephfsProvisionerSTS)
		if err != nil {
			return errors.Wrapf(err, "failed to start cephfs provisioner statefulset: %+v", cephfsProvisionerSTS)
		}
		k8sutil.AddRookVersionLabelToStatefulSet(cephfsProvisionerSTS)

	} else if cephfsProvisionerDeployment != nil {
		applyToPodSpec(&cephfsProvisionerDeployment.Spec.Template.Spec, provisionerNodeAffinity, provisionerTolerations)
		k8sutil.SetOwnerRef(&cephfsProvisionerDeployment.ObjectMeta, &ownerRef)
		err = k8sutil.CreateDeployment("csi-cephfsplugin-provisioner", namespace, clientset, cephfsProvisionerDeployment)
		if err != nil {
			return errors.Wrapf(err, "failed to start cephfs provisioner deployment: %+v", cephfsProvisionerDeployment)
		}
		k8sutil.AddRookVersionLabelToDeployment(cephfsProvisionerDeployment)
	}
	if cephfsService != nil {
		k8sutil.SetOwnerRef(&cephfsService.ObjectMeta, &ownerRef)
		_, err = k8sutil.CreateOrUpdateService(clientset, namespace, cephfsService)
		if err != nil {
			return errors.Wrapf(err, "failed to create rbd service: %+v", cephfsService)
		}
	}

	if ver.Major > KubeMinMajor || (ver.Major == KubeMinMajor && ver.Minor >= provDeploymentSuppVersion) {
		err = createCSIDriverInfo(clientset, RBDDriverName, ownerRef)
		if err != nil {
			return errors.Wrapf(err, "failed to create CSI driver object for %q", RBDDriverName)
		}
		err = createCSIDriverInfo(clientset, CephFSDriverName, ownerRef)
		if err != nil {
			return errors.Wrapf(err, "failed to create CSI driver object for %q", CephFSDriverName)
		}
	}
	return nil
}

func StopCSIDrivers(namespace string, clientset kubernetes.Interface) error {
	// As we have placed ownerRefs to the ConfigMap for all CSI resources, we delegate entirely to its deletion method.
	return DeleteCsiConfigMap(namespace, clientset)
}

// createCSIDriverInfo Registers CSI driver by creating a CSIDriver object
func createCSIDriverInfo(clientset kubernetes.Interface, name string, ownerRef metav1.OwnerReference) error {
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
	k8sutil.SetOwnerRef(&csiDriver.ObjectMeta, &ownerRef)
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
