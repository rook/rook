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
	"github.com/rook/rook/pkg/operator/k8sutil"
	"k8s.io/apimachinery/pkg/version"
)

func (r *ReconcileCSI) validateAndConfigureDrivers(serverVersion *version.Info, ownerInfo *k8sutil.OwnerInfo) error {
	var (
		v   *CephCSIVersion
		err error
	)

	if err = r.setParams(); err != nil {
		return errors.Wrapf(err, "failed to configure CSI parameters")
	}

	if err = validateCSIParam(); err != nil {
		return errors.Wrapf(err, "failed to validate CSI parameters")
	}

	if !AllowUnsupported && CSIEnabled() {
		if v, err = r.validateCSIVersion(ownerInfo); err != nil {
			return errors.Wrapf(err, "invalid csi version")
		}
	} else {
		logger.Info("skipping csi version check, since unsupported versions are allowed or csi is disabled")
	}

	if CSIEnabled() {
		maxRetries := 3
		for i := 0; i < maxRetries; i++ {
			if err = r.startDrivers(serverVersion, ownerInfo, v); err != nil {
				logger.Errorf("failed to start Ceph csi drivers, will retry starting csi drivers %d more times. %v", maxRetries-i-1, err)
			} else {
				break
			}
		}
		return errors.Wrap(err, "failed to start ceph csi drivers")
	}

	// Check whether RBD or CephFS needs to be disabled
	r.stopDrivers(serverVersion)

	return nil
}

func (r *ReconcileCSI) setParams() error {
	var err error

	if EnableRBD, err = strconv.ParseBool(k8sutil.GetValue(r.opConfig.Parameters, "ROOK_CSI_ENABLE_RBD", "true")); err != nil {
		return errors.Wrap(err, "unable to parse value for 'ROOK_CSI_ENABLE_RBD'")
	}

	if EnableCephFS, err = strconv.ParseBool(k8sutil.GetValue(r.opConfig.Parameters, "ROOK_CSI_ENABLE_CEPHFS", "true")); err != nil {
		return errors.Wrap(err, "unable to parse value for 'ROOK_CSI_ENABLE_CEPHFS'")
	}

	if AllowUnsupported, err = strconv.ParseBool(k8sutil.GetValue(r.opConfig.Parameters, "ROOK_CSI_ALLOW_UNSUPPORTED_VERSION", "false")); err != nil {
		return errors.Wrap(err, "unable to parse value for 'ROOK_CSI_ALLOW_UNSUPPORTED_VERSION'")
	}

	if EnableCSIGRPCMetrics, err = strconv.ParseBool(k8sutil.GetValue(r.opConfig.Parameters, "ROOK_CSI_ENABLE_GRPC_METRICS", "false")); err != nil {
		return errors.Wrap(err, "unable to parse value for 'ROOK_CSI_ENABLE_GRPC_METRICS'")
	}

	if CSIParam.EnableCSIHostNetwork, err = strconv.ParseBool(k8sutil.GetValue(r.opConfig.Parameters, "CSI_ENABLE_HOST_NETWORK", "true")); err != nil {
		return errors.Wrap(err, "failed to parse value for 'CSI_ENABLE_HOST_NETWORK'")
	}

	CSIParam.CSIPluginImage = k8sutil.GetValue(r.opConfig.Parameters, "ROOK_CSI_CEPH_IMAGE", DefaultCSIPluginImage)
	CSIParam.RegistrarImage = k8sutil.GetValue(r.opConfig.Parameters, "ROOK_CSI_REGISTRAR_IMAGE", DefaultRegistrarImage)
	CSIParam.ProvisionerImage = k8sutil.GetValue(r.opConfig.Parameters, "ROOK_CSI_PROVISIONER_IMAGE", DefaultProvisionerImage)
	CSIParam.AttacherImage = k8sutil.GetValue(r.opConfig.Parameters, "ROOK_CSI_ATTACHER_IMAGE", DefaultAttacherImage)
	CSIParam.SnapshotterImage = k8sutil.GetValue(r.opConfig.Parameters, "ROOK_CSI_SNAPSHOTTER_IMAGE", DefaultSnapshotterImage)
	CSIParam.KubeletDirPath = k8sutil.GetValue(r.opConfig.Parameters, "ROOK_CSI_KUBELET_DIR_PATH", DefaultKubeletDirPath)
	CSIParam.VolumeReplicationImage = k8sutil.GetValue(r.opConfig.Parameters, "CSI_VOLUME_REPLICATION_IMAGE", DefaultVolumeReplicationImage)
	csiCephFSPodLabels := k8sutil.GetValue(r.opConfig.Parameters, "ROOK_CSI_CEPHFS_POD_LABELS", "")
	CSIParam.CSICephFSPodLabels = k8sutil.ParseStringToLabels(csiCephFSPodLabels)
	csiRBDPodLabels := k8sutil.GetValue(r.opConfig.Parameters, "ROOK_CSI_RBD_POD_LABELS", "")
	CSIParam.CSIRBDPodLabels = k8sutil.ParseStringToLabels(csiRBDPodLabels)

	return nil
}
