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
	"errors"
	"fmt"

	"github.com/rook/rook/pkg/operator/k8sutil"

	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes"
)

type Param struct {
	CSIPluginImage       string
	RegistrarImage       string
	ProvisionerImage     string
	AttacherImage        string
	SnapshotterImage     string
	DriverNamePrefix     string
	EnableCSIGRPCMetrics string
	KubeletDirPath       string
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

const (
	KubeMinMajor              = "1"
	KubeMinMinor              = "13"
	provDeploymentSuppVersion = "14"

	// image names
	DefaultCSIPluginImage   = "quay.io/cephcsi/cephcsi:v1.2.1"
	DefaultRegistrarImage   = "quay.io/k8scsi/csi-node-driver-registrar:v1.1.0"
	DefaultProvisionerImage = "quay.io/k8scsi/csi-provisioner:v1.3.0"
	DefaultAttacherImage    = "quay.io/k8scsi/csi-attacher:v1.2.0"
	DefaultSnapshotterImage = "quay.io/k8scsi/csi-snapshotter:v1.2.0"

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
			return fmt.Errorf("missing cephfs plugin template path")
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
	if err := CreateCsiConfigMap(namespace, clientset); err != nil {
		return fmt.Errorf("failed creating csi config map. %+v", err)
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

	if ver.Minor < provDeploymentSuppVersion {
		deployProvSTS = true
	}

	if EnableRBD {
		rbdPlugin, err = templateToDaemonSet("rbdplugin", RBDPluginTemplatePath, tp)
		if err != nil {
			return fmt.Errorf("failed to load rbdplugin template: %+v", err)
		}
		if deployProvSTS {
			rbdProvisionerSTS, err = templateToStatefulSet("rbd-provisioner", RBDProvisionerSTSTemplatePath, tp)
			if err != nil {
				return fmt.Errorf("failed to load rbd provisioner statefulset template: %+v", err)
			}
		} else {
			rbdProvisionerDeployment, err = templateToDeployment("rbd-provisioner", RBDProvisionerDepTemplatePath, tp)
			if err != nil {
				return fmt.Errorf("failed to load rbd provisioner deployment template: %+v", err)
			}
		}
		rbdService, err = templateToService("rbd-service", DefaultRBDPluginServiceTemplatePath, tp)
		if err != nil {
			return fmt.Errorf("failed to load rbd plugin service template: %+v", err)
		}
	}
	if EnableCephFS {
		cephfsPlugin, err = templateToDaemonSet("cephfsplugin", CephFSPluginTemplatePath, tp)
		if err != nil {
			return fmt.Errorf("failed to load CephFS plugin template: %+v", err)
		}
		if deployProvSTS {
			cephfsProvisionerSTS, err = templateToStatefulSet("cephfs-provisioner", CephFSProvisionerSTSTemplatePath, tp)
			if err != nil {
				return fmt.Errorf("failed to load CephFS provisioner statefulset template: %+v", err)
			}
		} else {
			cephfsProvisionerDeployment, err = templateToDeployment("cephfs-provisioner", CephFSProvisionerDepTemplatePath, tp)
			if err != nil {
				return fmt.Errorf("failed to load rbd provisioner deployment template: %+v", err)
			}
		}
		cephfsService, err = templateToService("cephfs-service", DefaultCephFSPluginServiceTemplatePath, tp)
		if err != nil {
			return fmt.Errorf("failed to load cephfs plugin service template: %+v", err)
		}
	}

	if rbdPlugin != nil {
		err = k8sutil.CreateDaemonSet("csi-rbdplugin", namespace, clientset, rbdPlugin)
		if err != nil {
			return fmt.Errorf("failed to start rbdplugin daemonset: %+v\n%+v", err, rbdPlugin)
		}
	}
	if rbdProvisionerSTS != nil {
		err = k8sutil.CreateStatefulSet("csi-rbdplugin-provisioner", namespace, clientset, rbdProvisionerSTS)
		if err != nil {
			return fmt.Errorf("failed to start rbd provisioner statefulset: %+v\n%+v", err, rbdProvisionerSTS)
		}
	} else if rbdProvisionerDeployment != nil {
		err = k8sutil.CreateDeployment("csi-rbdplugin-provisioner", namespace, clientset, rbdProvisionerDeployment)
		if err != nil {
			return fmt.Errorf("failed to start rbd provisioner deployment: %+v\n%+v", err, rbdProvisionerDeployment)
		}
	}

	if rbdService != nil {
		_, err = k8sutil.CreateOrUpdateService(clientset, namespace, rbdService)
		if err != nil {
			return fmt.Errorf("failed to create rbd service: %+v\n%+v", err, rbdService)
		}
	}

	if cephfsPlugin != nil {
		err = k8sutil.CreateDaemonSet("csi-cephfsplugin", namespace, clientset, cephfsPlugin)
		if err != nil {
			return fmt.Errorf("failed to start cephfs plugin daemonset: %+v\n%+v", err, cephfsPlugin)
		}
	}

	if cephfsProvisionerSTS != nil {
		err = k8sutil.CreateStatefulSet("csi-cephfsplugin-provisioner", namespace, clientset, cephfsProvisionerSTS)
		if err != nil {
			return fmt.Errorf("failed to start cephfs provisioner statefulset: %+v\n%+v", err, cephfsProvisionerSTS)
		}

	} else if cephfsProvisionerDeployment != nil {
		err = k8sutil.CreateDeployment("csi-cephfsplugin-provisioner", namespace, clientset, cephfsProvisionerDeployment)
		if err != nil {
			return fmt.Errorf("failed to start cephfs provisioner deployment: %+v\n%+v", err, cephfsProvisionerDeployment)
		}
	}
	if cephfsService != nil {
		_, err = k8sutil.CreateOrUpdateService(clientset, namespace, cephfsService)
		if err != nil {
			return fmt.Errorf("failed to create rbd service: %+v\n%+v", err, cephfsService)
		}
	}
	return nil
}
