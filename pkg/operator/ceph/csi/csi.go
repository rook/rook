/*
Copyright 2020 The Rook Authors. All rights reserved.

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
	"strconv"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	controllerutil "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes"
)

func ValidateAndConfigureDrivers(context *clusterd.Context, namespace, rookImage, securityAccount string, serverVersion *version.Info, ownerInfo *k8sutil.OwnerInfo) {
	var v *CephCSIVersion
	var err error
	if !AllowUnsupported && CSIEnabled() {
		if v, err = validateCSIVersion(context.Clientset, namespace, rookImage, securityAccount, ownerInfo); err != nil {
			logger.Errorf("invalid csi version. %+v", err)
			return
		}
	} else {
		logger.Info("Skipping csi version check, since unsupported versions are allowed or csi is disabled")
	}

	if CSIEnabled() {
		if err := startDrivers(context.Clientset, context.RookClientset, namespace, serverVersion, ownerInfo, v); err != nil {
			logger.Errorf("failed to start Ceph csi drivers. %v", err)
			return
		}
	}

	stopDrivers(context.Clientset, namespace, serverVersion)
}

func SetParams(clientset kubernetes.Interface) error {
	var err error

	csiEnableRBD, err := k8sutil.GetOperatorSetting(clientset, controllerutil.OperatorSettingConfigMapName, "ROOK_CSI_ENABLE_RBD", "true")
	if err != nil {
		return errors.Wrap(err, "unable to determine if CSI driver for RBD is enabled")
	}
	if EnableRBD, err = strconv.ParseBool(csiEnableRBD); err != nil {
		return errors.Wrap(err, "unable to parse value for 'ROOK_CSI_ENABLE_RBD'")
	}

	csiEnableCephFS, err := k8sutil.GetOperatorSetting(clientset, controllerutil.OperatorSettingConfigMapName, "ROOK_CSI_ENABLE_CEPHFS", "true")
	if err != nil {
		return errors.Wrap(err, "unable to determine if CSI driver for CephFS is enabled")
	}
	if EnableCephFS, err = strconv.ParseBool(csiEnableCephFS); err != nil {
		return errors.Wrap(err, "unable to parse value for 'ROOK_CSI_ENABLE_CEPHFS'")
	}

	csiAllowUnsupported, err := k8sutil.GetOperatorSetting(clientset, controllerutil.OperatorSettingConfigMapName, "ROOK_CSI_ALLOW_UNSUPPORTED_VERSION", "false")
	if err != nil {
		return errors.Wrap(err, "unable to determine if unsupported version is allowed")
	}
	if AllowUnsupported, err = strconv.ParseBool(csiAllowUnsupported); err != nil {
		return errors.Wrap(err, "unable to parse value for 'ROOK_CSI_ALLOW_UNSUPPORTED_VERSION'")
	}

	csiEnableCSIGRPCMetrics, err := k8sutil.GetOperatorSetting(clientset, controllerutil.OperatorSettingConfigMapName, "ROOK_CSI_ENABLE_GRPC_METRICS", "false")
	if err != nil {
		return errors.Wrap(err, "unable to determine if CSI GRPC metrics is enabled")
	}
	if EnableCSIGRPCMetrics, err = strconv.ParseBool(csiEnableCSIGRPCMetrics); err != nil {
		return errors.Wrap(err, "unable to parse value for 'ROOK_CSI_ENABLE_GRPC_METRICS'")
	}

	CSIParam.CSIPluginImage, err = k8sutil.GetOperatorSetting(clientset, controllerutil.OperatorSettingConfigMapName, "ROOK_CSI_CEPH_IMAGE", DefaultCSIPluginImage)
	if err != nil {
		return errors.Wrap(err, "unable to configure CSI plugin image")
	}
	CSIParam.RegistrarImage, err = k8sutil.GetOperatorSetting(clientset, controllerutil.OperatorSettingConfigMapName, "ROOK_CSI_REGISTRAR_IMAGE", DefaultRegistrarImage)
	if err != nil {
		return errors.Wrap(err, "unable to configure CSI registrar image")
	}
	CSIParam.ProvisionerImage, err = k8sutil.GetOperatorSetting(clientset, controllerutil.OperatorSettingConfigMapName, "ROOK_CSI_PROVISIONER_IMAGE", DefaultProvisionerImage)
	if err != nil {
		return errors.Wrap(err, "unable to configure CSI provisioner image")
	}
	CSIParam.AttacherImage, err = k8sutil.GetOperatorSetting(clientset, controllerutil.OperatorSettingConfigMapName, "ROOK_CSI_ATTACHER_IMAGE", DefaultAttacherImage)
	if err != nil {
		return errors.Wrap(err, "unable to configure CSI attacher image")
	}
	CSIParam.SnapshotterImage, err = k8sutil.GetOperatorSetting(clientset, controllerutil.OperatorSettingConfigMapName, "ROOK_CSI_SNAPSHOTTER_IMAGE", DefaultSnapshotterImage)
	if err != nil {
		return errors.Wrap(err, "unable to configure CSI snapshotter image")
	}
	CSIParam.KubeletDirPath, err = k8sutil.GetOperatorSetting(clientset, controllerutil.OperatorSettingConfigMapName, "ROOK_CSI_KUBELET_DIR_PATH", DefaultKubeletDirPath)
	if err != nil {
		return errors.Wrap(err, "unable to configure CSI kubelet directory path")
	}

	csiCephFSPodLabels, err := k8sutil.GetOperatorSetting(clientset, controllerutil.OperatorSettingConfigMapName, "ROOK_CSI_CEPHFS_POD_LABELS", "")
	if err != nil {
		return errors.Wrap(err, "unable to configure CSI CephFS pod labels")
	}
	CSIParam.CSICephFSPodLabels = k8sutil.ParseStringToLabels(csiCephFSPodLabels)

	csiRBDPodLabels, err := k8sutil.GetOperatorSetting(clientset, controllerutil.OperatorSettingConfigMapName, "ROOK_CSI_RBD_POD_LABELS", "")
	if err != nil {
		return errors.Wrap(err, "unable to configure CSI RBD pod labels")
	}
	CSIParam.CSIRBDPodLabels = k8sutil.ParseStringToLabels(csiRBDPodLabels)

	return nil
}
