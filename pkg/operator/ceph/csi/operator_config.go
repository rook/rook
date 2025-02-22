/*
Copyright 2024 The Rook Authors. All rights reserved.

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
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"

	csiopv1a1 "github.com/ceph/ceph-csi-operator/api/v1alpha1"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	k8scsiv1 "k8s.io/api/storage/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	opConfigCRName    = "ceph-csi-operator-config"
	imageSetConfigMap = "rook-csi-operator-image-set-configmap"
)

func (r *ReconcileCSI) createOrUpdateOperatorConfig(cluster cephv1.CephCluster) error {
	logger.Info("Creating ceph-CSI operator config CR")

	opConfig := &csiopv1a1.OperatorConfig{}
	opConfig.Name = opConfigCRName
	opConfig.Namespace = r.opConfig.OperatorNamespace

	imageSetCmName, err := r.createImageSetConfigmap()
	if err != nil {
		return errors.Wrapf(err, "failed to create ceph-CSI operator config ImageSetConfigmap for CR %s", opConfigCRName)
	}

	spec := r.generateCSIOpConfigSpec(cluster, opConfig, imageSetCmName)

	err = r.client.Get(r.opManagerContext, types.NamespacedName{Name: opConfigCRName, Namespace: r.opConfig.OperatorNamespace}, opConfig)
	if err != nil {
		if kerrors.IsNotFound(err) {
			opConfig.Spec = spec
			err = r.client.Create(r.opManagerContext, opConfig)
			if err != nil {
				return errors.Wrapf(err, "failed to create ceph-CSI operator operator config CR %q", opConfig.Name)
			}

			logger.Infof("Successfully created ceph-CSI operator config CR %q", opConfig.Name)
			return nil
		}
		return errors.Wrapf(err, "failed to get ceph-CSI operator operator config CR %q", opConfigCRName)
	}

	opConfig.Spec = spec
	err = r.client.Update(r.opManagerContext, opConfig)
	if err != nil {
		return errors.Wrapf(err, "failed to update ceph-CSI operator operator config CR %q", opConfig.Name)
	}
	logger.Infof("Successfully updated ceph-CSI operator config CR %q", opConfig.Name)

	return nil
}

func (r *ReconcileCSI) generateCSIOpConfigSpec(cluster cephv1.CephCluster, opConfig *csiopv1a1.OperatorConfig, imageSetCmName string) csiopv1a1.OperatorConfigSpec {
	cephfsClientType := csiopv1a1.KernelCephFsClient
	if CSIParam.ForceCephFSKernelClient == "false" {
		cephfsClientType = csiopv1a1.AutoDetectCephFsClient
	}

	opConfig.Spec = csiopv1a1.OperatorConfigSpec{
		DriverSpecDefaults: &csiopv1a1.DriverSpec{
			Log: &csiopv1a1.LogSpec{
				Verbosity: int(CSIParam.LogLevel),
			},
			ImageSet: &v1.LocalObjectReference{
				Name: imageSetCmName,
			},
			ClusterName:      &cluster.Name,
			EnableMetadata:   &CSIParam.CSIEnableMetadata,
			GenerateOMapInfo: &CSIParam.EnableOMAPGenerator,
			FsGroupPolicy:    k8scsiv1.FileFSGroupPolicy,
			NodePlugin: &csiopv1a1.NodePluginSpec{
				PodCommonSpec: csiopv1a1.PodCommonSpec{
					PrioritylClassName: &CSIParam.ProvisionerPriorityClassName,
					Affinity: &v1.Affinity{
						NodeAffinity: getNodeAffinity(pluginNodeAffinityEnv, &v1.NodeAffinity{}),
					},
					Tolerations: getToleration(pluginTolerationsEnv, []v1.Toleration{}),
				},
				Resources:              csiopv1a1.NodePluginResourcesSpec{},
				KubeletDirPath:         CSIParam.KubeletDirPath,
				EnableSeLinuxHostMount: &CSIParam.EnablePluginSelinuxHostMount,
			},
			ControllerPlugin: &csiopv1a1.ControllerPluginSpec{
				PodCommonSpec: csiopv1a1.PodCommonSpec{
					PrioritylClassName: &CSIParam.PluginPriorityClassName,
					Affinity: &v1.Affinity{
						NodeAffinity: getNodeAffinity(provisionerNodeAffinityEnv, &v1.NodeAffinity{}),
					},
					Tolerations: getToleration(provisionerTolerationsEnv, []v1.Toleration{}),
				},
				Replicas:  &CSIParam.ProvisionerReplicas,
				Resources: csiopv1a1.ControllerPluginResourcesSpec{},
			},
			DeployCsiAddons:  &CSIParam.EnableCSIAddonsSideCar,
			CephFsClientType: cephfsClientType,
		},
	}
	if CSIParam.EnableCSIEncryption {
		opConfig.Spec.DriverSpecDefaults.Encryption = &csiopv1a1.EncryptionSpec{
			ConfigMapRef: v1.LocalObjectReference{
				Name: "rook-ceph-csi-kms-config",
			},
		}
	}

	return opConfig.Spec
}

func (r *ReconcileCSI) createImageSetConfigmap() (string, error) {

	data := map[string]string{
		"provisioner": CSIParam.ProvisionerImage,
		"attacher":    CSIParam.AttacherImage,
		"resizer":     CSIParam.ResizerImage,
		"snapshotter": CSIParam.SnapshotterImage,
		"registrar":   CSIParam.RegistrarImage,
		"plugin":      CSIParam.CSIPluginImage,
		"addons":      CSIParam.CSIAddonsImage,
	}

	cm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rook-csi-operator-image-set-configmap",
			Namespace: r.opConfig.OperatorNamespace,
		},
		Data: data,
	}

	err := r.client.Get(r.opManagerContext, types.NamespacedName{Name: cm.Name, Namespace: r.opConfig.OperatorNamespace}, cm)
	if err != nil {
		if kerrors.IsNotFound(err) {
			err = r.client.Create(r.opManagerContext, cm)
			if err != nil {
				return "", errors.Wrapf(err, "failed to create imageSet cm  %q for ceph-CSI operator-config CR %q", cm.Name, opConfigCRName)
			}

			logger.Infof("Successfully create imageSet cm %s for ceph-CSI operator-config CR %q", cm.Name, opConfigCRName)
			return cm.Name, nil
		}
		return "", errors.Wrapf(err, "failed to get imageSet cm %q for ceph-CSI operator-config CR %q", cm.Name, opConfigCRName)
	}

	cm.Data = data
	err = r.client.Update(r.opManagerContext, cm)
	if err != nil {
		return "", errors.Wrapf(err, "failed to updated imageSet cm  %q ceph-CSI operator-config CR %q", cm.Name, opConfigCRName)
	}
	logger.Infof("Successfully updated imageSet cm %s for ceph-CSI operator-config CR %q", cm.Name, opConfigCRName)

	return cm.Name, nil
}
