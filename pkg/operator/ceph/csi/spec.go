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
	"k8s.io/client-go/kubernetes"
)

type Param struct {
	Namespace string

	RBDPluginImage    string
	CephFSPluginImage string
	RegistrarImage    string
	ProvisionerImage  string
	AttacherImage     string
	SnapshotterImage  string
}

var (
	CSIParam Param

	EnableRBD    = true
	EnableCephFS = true

	// template paths
	RBDPluginTemplatePath      string
	RBDProvisionerTemplatePath string

	CephFSPluginTemplatePath      string
	CephFSProvisionerTemplatePath string
)

const (
	KubeMinMajor = "1"
	KubeMinMinor = "13"

	// image names
	DefaultRBDPluginImage    = "quay.io/cephcsi/rbdplugin:v1.0.0"
	DefaultCephFSPluginImage = "quay.io/cephcsi/cephfsplugin:v1.0.0"
	DefaultRegistrarImage    = "quay.io/k8scsi/csi-node-driver-registrar:v1.0.2"
	DefaultProvisionerImage  = "quay.io/k8scsi/csi-provisioner:v1.0.1"
	DefaultAttacherImage     = "quay.io/k8scsi/csi-attacher:v1.0.1"
	DefaultSnapshotterImage  = "quay.io/k8scsi/csi-snapshotter:v1.0.1"

	// template
	DefaultRBDPluginTemplatePath         = "/etc/ceph-csi/rbd/csi-rbdplugin.yaml"
	DefaultRBDProvisionerTemplatePath    = "/etc/ceph-csi/rbd/csi-rbdplugin-provisioner.yaml"
	DefaultCephFSPluginTemplatePath      = "/etc/ceph-csi/cephfs/csi-cephfsplugin.yaml"
	DefaultCephFSProvisionerTemplatePath = "/etc/ceph-csi/cephfs/csi-cephfsplugin-provisioner.yaml"

	ExitOnError = false // don't exit if CSI fails to deploy. Switch to true when flexdriver is disabled
)

func CSIEnabled() bool {
	return EnableRBD || EnableCephFS
}

func SetCSINamespace(namespace string) {
	CSIParam.Namespace = namespace
}

func ValidateCSIParam() error {
	if EnableRBD {
		if len(CSIParam.RBDPluginImage) == 0 {
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
		if len(RBDPluginTemplatePath) == 0 {
			return errors.New("missing rbd plugin template path")
		}
		if len(RBDProvisionerTemplatePath) == 0 {
			return errors.New("missing rbd provisioner template path")
		}
	}

	if EnableCephFS {
		if len(CSIParam.CephFSPluginImage) == 0 {
			return errors.New("missing csi cephfs plugin image")
		}
		if len(CSIParam.RegistrarImage) == 0 {
			return errors.New("missing csi registrar image")
		}
		if len(CSIParam.ProvisionerImage) == 0 {
			return errors.New("missing csi provisioner image")
		}
		if len(CephFSPluginTemplatePath) == 0 {
			return fmt.Errorf("missing cephfs plugin template path")
		}
		if len(CephFSProvisionerTemplatePath) == 0 {
			return errors.New("missing ceph provisioner template path")
		}
	}
	return nil
}

func StartCSIDrivers(namespace string, clientset kubernetes.Interface) error {
	var (
		err                               error
		rbdPlugin, cephfsPlugin           *apps.DaemonSet
		rbdProvisioner, cephfsProvisioner *apps.StatefulSet
	)

	if EnableRBD {
		rbdPlugin, err = templateToDaemonSet("rbdplugin", RBDPluginTemplatePath)
		if err != nil {
			return fmt.Errorf("failed to load rbd plugin template: %v", err)
		}
		rbdProvisioner, err = templateToStatefulSet("rbd-provisioner", RBDProvisionerTemplatePath)
		if err != nil {
			return fmt.Errorf("failed to load rbd provisioner template: %v", err)
		}
	}
	if EnableCephFS {
		cephfsPlugin, err = templateToDaemonSet("cephfsplugin", CephFSPluginTemplatePath)
		if err != nil {
			return fmt.Errorf("failed to load CephFS plugin template: %v", err)
		}
		cephfsProvisioner, err = templateToStatefulSet("cephfs-provisioner", CephFSProvisionerTemplatePath)
		if err != nil {
			return fmt.Errorf("failed to load CephFS provisioner template: %v", err)
		}
	}

	if rbdPlugin != nil {
		err = k8sutil.CreateDaemonSet("csi rbd plugin", namespace, clientset, rbdPlugin)
		if err != nil {
			return fmt.Errorf("failed to start rbdplugin daemonset: %v\n%v", err, rbdPlugin)
		}
	}
	if rbdProvisioner != nil {
		_, err = k8sutil.CreateStatefulSet("csi rbd provisioner", namespace, "csi-rbdplugin-provisioner", clientset, rbdProvisioner)
		if err != nil {
			return fmt.Errorf("failed to start rbd provisioner statefulset: %v\n%v", err, rbdProvisioner)
		}

	}

	if cephfsPlugin != nil {
		err = k8sutil.CreateDaemonSet("csi cephfs plugin", namespace, clientset, cephfsPlugin)
		if err != nil {
			return fmt.Errorf("failed to start cephfs plugin daemonset: %v\n%v", err, cephfsPlugin)
		}
	}
	if cephfsProvisioner != nil {
		_, err = k8sutil.CreateStatefulSet("csi cephfs provisioner", namespace, "csi-cephfsplugin-provisioner", clientset, cephfsProvisioner)
		if err != nil {
			return fmt.Errorf("failed to start cephfs provisioner statefulset: %v\n%v", err, cephfsProvisioner)
		}

	}
	return nil
}
