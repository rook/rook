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
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type Param struct {
	Namespace string

	CSIPluginImage   string
	RegistrarImage   string
	ProvisionerImage string
	AttacherImage    string
	SnapshotterImage string
}

var (
	CSIParam Param

	EnableRBD    = false
	EnableCephFS = false

	// template paths
	RBDPluginTemplatePath      string
	RBDProvisionerTemplatePath string

	CephFSPluginTemplatePath      string
	CephFSProvisionerTemplatePath string

	// configuration map for csi
	ConfigName = "rook-ceph-csi-config"
	ConfigKey  = "csi-cluster-config-json"
)

const (
	KubeMinMajor = "1"
	KubeMinMinor = "13"

	// image names
	DefaultCSIPluginImage   = "quay.io/cephcsi/cephcsi:canary"
	DefaultRegistrarImage   = "quay.io/k8scsi/csi-node-driver-registrar:v1.1.0"
	DefaultProvisionerImage = "quay.io/k8scsi/csi-provisioner:v1.2.0"
	DefaultAttacherImage    = "quay.io/k8scsi/csi-attacher:v1.1.1"
	DefaultSnapshotterImage = "quay.io/k8scsi/csi-snapshotter:v1.1.0"

	// template
	DefaultRBDPluginTemplatePath         = "/etc/ceph-csi/rbd/csi-rbdplugin.yaml"
	DefaultRBDProvisionerTemplatePath    = "/etc/ceph-csi/rbd/csi-rbdplugin-provisioner.yaml"
	DefaultCephFSPluginTemplatePath      = "/etc/ceph-csi/cephfs/csi-cephfsplugin.yaml"
	DefaultCephFSProvisionerTemplatePath = "/etc/ceph-csi/cephfs/csi-cephfsplugin-provisioner.yaml"
)

func CSIEnabled() bool {
	return EnableRBD || EnableCephFS
}

func SetCSINamespace(namespace string) {
	CSIParam.Namespace = namespace
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
		if len(RBDProvisionerTemplatePath) == 0 {
			return errors.New("missing rbd provisioner template path")
		}
	}

	if EnableCephFS {
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

	// create an empty config map. config map will be filled with data
	// later when clusters have mons
	CreateCsiConfigMap(namespace, clientset)

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

func CreateCsiConfigMap(namespace string, clientset kubernetes.Interface) error {
	configMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConfigName,
			Namespace: namespace,
		},
	}
	configMap.Data = map[string]string{
		ConfigKey: "[]",
	}

	if _, err := clientset.CoreV1().ConfigMaps(namespace).Create(configMap); err != nil {
		if !k8serrors.IsAlreadyExists(err) {
			return err
		}
	}
	return nil
}
