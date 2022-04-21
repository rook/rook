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
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/operator/k8sutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"
)

func (r *ReconcileCSI) validateAndConfigureDrivers(serverVersion *version.Info, ownerInfo *k8sutil.OwnerInfo) error {
	var (
		v   *CephCSIVersion
		err error
	)

	if err = r.setParams(serverVersion); err != nil {
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
		if err = r.startDrivers(serverVersion, ownerInfo, v); err != nil {
			return errors.Wrap(err, "failed to start ceph csi drivers")
		}
	}

	// Check whether RBD or CephFS needs to be disabled
	return r.stopDrivers(serverVersion)
}

func (r *ReconcileCSI) setParams(ver *version.Info) error {
	var err error

	if EnableRBD, err = strconv.ParseBool(k8sutil.GetValue(r.opConfig.Parameters, "ROOK_CSI_ENABLE_RBD", "true")); err != nil {
		return errors.Wrap(err, "unable to parse value for 'ROOK_CSI_ENABLE_RBD'")
	}

	if EnableCephFS, err = strconv.ParseBool(k8sutil.GetValue(r.opConfig.Parameters, "ROOK_CSI_ENABLE_CEPHFS", "true")); err != nil {
		return errors.Wrap(err, "unable to parse value for 'ROOK_CSI_ENABLE_CEPHFS'")
	}

	if EnableNFS, err = strconv.ParseBool(k8sutil.GetValue(r.opConfig.Parameters, "ROOK_CSI_ENABLE_NFS", "false")); err != nil {
		return errors.Wrap(err, "unable to parse value for 'ROOK_CSI_ENABLE_NFS'")
	}

	if AllowUnsupported, err = strconv.ParseBool(k8sutil.GetValue(r.opConfig.Parameters, "ROOK_CSI_ALLOW_UNSUPPORTED_VERSION", "false")); err != nil {
		return errors.Wrap(err, "unable to parse value for 'ROOK_CSI_ALLOW_UNSUPPORTED_VERSION'")
	}

	if EnableCSIGRPCMetrics, err = strconv.ParseBool(k8sutil.GetValue(r.opConfig.Parameters, "ROOK_CSI_ENABLE_GRPC_METRICS", "false")); err != nil {
		return errors.Wrap(err, "unable to parse value for 'ROOK_CSI_ENABLE_GRPC_METRICS'")
	}
	CSIParam.EnableCSIGRPCMetrics = fmt.Sprintf("%t", EnableCSIGRPCMetrics)

	if CSIParam.EnableCSIHostNetwork, err = strconv.ParseBool(k8sutil.GetValue(r.opConfig.Parameters, "CSI_ENABLE_HOST_NETWORK", "true")); err != nil {
		return errors.Wrap(err, "failed to parse value for 'CSI_ENABLE_HOST_NETWORK'")
	}

	// If not set or set to anything but "false", the kernel client will be enabled
	if strings.EqualFold(k8sutil.GetValue(r.opConfig.Parameters, "CSI_FORCE_CEPHFS_KERNEL_CLIENT", "true"), "false") {
		CSIParam.ForceCephFSKernelClient = "false"
	} else {
		CSIParam.ForceCephFSKernelClient = "true"
	}

	// parse RPC timeout
	timeout := k8sutil.GetValue(r.opConfig.Parameters, grpcTimeout, "150")
	timeoutSeconds, err := strconv.Atoi(timeout)
	if err != nil {
		logger.Errorf("failed to parse %q. Defaulting to %d. %v", grpcTimeout, defaultGRPCTimeout, err)
		timeoutSeconds = defaultGRPCTimeout
	}
	if timeoutSeconds < 120 {
		logger.Warningf("%s is %q but it should be >= 120, setting the default value %d", grpcTimeout, timeout, defaultGRPCTimeout)
		timeoutSeconds = defaultGRPCTimeout
	}
	CSIParam.GRPCTimeout = time.Duration(timeoutSeconds) * time.Second

	// parse GRPC and Liveness ports
	CSIParam.CephFSGRPCMetricsPort, err = getPortFromConfig(r.opConfig.Parameters, "CSI_CEPHFS_GRPC_METRICS_PORT", DefaultCephFSGRPCMerticsPort)
	if err != nil {
		return errors.Wrap(err, "error getting CSI CephFS GRPC metrics port.")
	}
	CSIParam.CephFSLivenessMetricsPort, err = getPortFromConfig(r.opConfig.Parameters, "CSI_CEPHFS_LIVENESS_METRICS_PORT", DefaultCephFSLivenessMerticsPort)
	if err != nil {
		return errors.Wrap(err, "error getting CSI CephFS liveness metrics port.")
	}

	CSIParam.RBDGRPCMetricsPort, err = getPortFromConfig(r.opConfig.Parameters, "CSI_RBD_GRPC_METRICS_PORT", DefaultRBDGRPCMerticsPort)
	if err != nil {
		return errors.Wrap(err, "error getting CSI RBD GRPC metrics port.")
	}
	CSIParam.CSIAddonsPort, err = getPortFromConfig(r.opConfig.Parameters, "CSIADDONS_PORT", DefaultCSIAddonsPort)
	if err != nil {
		return errors.Wrap(err, "failed to get CSI Addons port")
	}
	CSIParam.RBDLivenessMetricsPort, err = getPortFromConfig(r.opConfig.Parameters, "CSI_RBD_LIVENESS_METRICS_PORT", DefaultRBDLivenessMerticsPort)
	if err != nil {
		return errors.Wrap(err, "error getting CSI RBD liveness metrics port.")
	}

	// default value `system-node-critical` is the highest available priority
	CSIParam.PluginPriorityClassName = k8sutil.GetValue(r.opConfig.Parameters, "CSI_PLUGIN_PRIORITY_CLASSNAME", "")

	// default value `system-cluster-critical` is applied for some
	// critical pods in cluster but less priority than plugin pods
	CSIParam.ProvisionerPriorityClassName = k8sutil.GetValue(r.opConfig.Parameters, "CSI_PROVISIONER_PRIORITY_CLASSNAME", "")

	if strings.EqualFold(k8sutil.GetValue(r.opConfig.Parameters, "CSI_ENABLE_OMAP_GENERATOR", "false"), "true") {
		CSIParam.EnableOMAPGenerator = true
	}

	// SA token projection is stable only from kubernetes version 1.20.
	if ver.Major == KubeMinMajor && ver.Minor >= KubeMinVerForOIDCTokenProjection {
		CSIParam.EnableOIDCTokenProjection = true
	}

	// if k8s >= v1.17 enable RBD and CephFS snapshotter by default
	if ver.Major == KubeMinMajor && ver.Minor >= kubeMinVerForSnapshot {
		CSIParam.EnableRBDSnapshotter = true
		CSIParam.EnableCephFSSnapshotter = true
	}

	if strings.EqualFold(k8sutil.GetValue(r.opConfig.Parameters, "CSI_ENABLE_RBD_SNAPSHOTTER", "true"), "false") {
		CSIParam.EnableRBDSnapshotter = false
	}

	if strings.EqualFold(k8sutil.GetValue(r.opConfig.Parameters, "CSI_ENABLE_CEPHFS_SNAPSHOTTER", "true"), "false") {
		CSIParam.EnableCephFSSnapshotter = false
	}

	CSIParam.EnableVolumeReplicationSideCar = false
	if strings.EqualFold(k8sutil.GetValue(r.opConfig.Parameters, "CSI_ENABLE_VOLUME_REPLICATION", "false"), "true") {
		CSIParam.EnableVolumeReplicationSideCar = true
	}

	CSIParam.EnableCSIAddonsSideCar = false
	if strings.EqualFold(k8sutil.GetValue(r.opConfig.Parameters, "CSI_ENABLE_CSIADDONS", "false"), "true") {
		CSIParam.EnableCSIAddonsSideCar = true
	}

	if strings.EqualFold(k8sutil.GetValue(r.opConfig.Parameters, "CSI_ENABLE_ENCRYPTION", "false"), "true") {
		CSIParam.EnableCSIEncryption = true
	}

	if strings.EqualFold(k8sutil.GetValue(r.opConfig.Parameters, "CSI_CEPHFS_PLUGIN_UPDATE_STRATEGY", rollingUpdate), onDelete) {
		CSIParam.CephFSPluginUpdateStrategy = onDelete
	} else {
		CSIParam.CephFSPluginUpdateStrategy = rollingUpdate
	}

	if strings.EqualFold(k8sutil.GetValue(r.opConfig.Parameters, "CSI_NFS_PLUGIN_UPDATE_STRATEGY", rollingUpdate), onDelete) {
		CSIParam.NFSPluginUpdateStrategy = onDelete
	} else {
		CSIParam.NFSPluginUpdateStrategy = rollingUpdate
	}

	if strings.EqualFold(k8sutil.GetValue(r.opConfig.Parameters, "CSI_RBD_PLUGIN_UPDATE_STRATEGY", rollingUpdate), onDelete) {
		CSIParam.RBDPluginUpdateStrategy = onDelete
	} else {
		CSIParam.RBDPluginUpdateStrategy = rollingUpdate
	}

	if strings.EqualFold(k8sutil.GetValue(r.opConfig.Parameters, "CSI_PLUGIN_ENABLE_SELINUX_HOST_MOUNT", "false"), "true") {
		CSIParam.EnablePluginSelinuxHostMount = true
	}

	logger.Infof("Kubernetes version is %s.%s", ver.Major, ver.Minor)

	CSIParam.ResizerImage = k8sutil.GetValue(r.opConfig.Parameters, "ROOK_CSI_RESIZER_IMAGE", DefaultResizerImage)

	logLevel := k8sutil.GetValue(r.opConfig.Parameters, "CSI_LOG_LEVEL", "")
	CSIParam.LogLevel = defaultLogLevel
	if logLevel != "" {
		l, err := strconv.ParseUint(logLevel, 10, 8)
		if err != nil {
			logger.Errorf("failed to parse CSI_LOG_LEVEL. Defaulting to %d. %v", defaultLogLevel, err)
		} else {
			CSIParam.LogLevel = uint8(l)
		}
	}

	CSIParam.ProvisionerReplicas = defaultProvisionerReplicas
	nodes, err := r.context.Clientset.CoreV1().Nodes().List(r.opManagerContext, metav1.ListOptions{})
	if err == nil {
		if len(nodes.Items) == 1 {
			CSIParam.ProvisionerReplicas = 1
		} else {
			replicas := k8sutil.GetValue(r.opConfig.Parameters, "CSI_PROVISIONER_REPLICAS", "2")
			r, err := strconv.ParseInt(replicas, 10, 32)
			if err != nil {
				logger.Errorf("failed to parse CSI_PROVISIONER_REPLICAS. Defaulting to %d. %v", defaultProvisionerReplicas, err)
			} else {
				CSIParam.ProvisionerReplicas = int32(r)
			}
		}
	} else {
		logger.Errorf("failed to get nodes. Defaulting the number of replicas of provisioner pods to %d. %v", CSIParam.ProvisionerReplicas, err)
	}

	CSIParam.CSIPluginImage = k8sutil.GetValue(r.opConfig.Parameters, "ROOK_CSI_CEPH_IMAGE", DefaultCSIPluginImage)
	CSIParam.NFSPluginImage = k8sutil.GetValue(r.opConfig.Parameters, "ROOK_CSI_NFS_IMAGE", DefaultNFSPluginImage)
	CSIParam.RegistrarImage = k8sutil.GetValue(r.opConfig.Parameters, "ROOK_CSI_REGISTRAR_IMAGE", DefaultRegistrarImage)
	CSIParam.ProvisionerImage = k8sutil.GetValue(r.opConfig.Parameters, "ROOK_CSI_PROVISIONER_IMAGE", DefaultProvisionerImage)
	CSIParam.AttacherImage = k8sutil.GetValue(r.opConfig.Parameters, "ROOK_CSI_ATTACHER_IMAGE", DefaultAttacherImage)
	CSIParam.SnapshotterImage = k8sutil.GetValue(r.opConfig.Parameters, "ROOK_CSI_SNAPSHOTTER_IMAGE", DefaultSnapshotterImage)
	CSIParam.KubeletDirPath = k8sutil.GetValue(r.opConfig.Parameters, "ROOK_CSI_KUBELET_DIR_PATH", DefaultKubeletDirPath)
	CSIParam.VolumeReplicationImage = k8sutil.GetValue(r.opConfig.Parameters, "CSI_VOLUME_REPLICATION_IMAGE", DefaultVolumeReplicationImage)
	CSIParam.CSIAddonsImage = k8sutil.GetValue(r.opConfig.Parameters, "ROOK_CSIADDONS_IMAGE", DefaultCSIAddonsImage)
	csiCephFSPodLabels := k8sutil.GetValue(r.opConfig.Parameters, "ROOK_CSI_CEPHFS_POD_LABELS", "")
	CSIParam.CSICephFSPodLabels = k8sutil.ParseStringToLabels(csiCephFSPodLabels)
	csiNFSPodLabels := k8sutil.GetValue(r.opConfig.Parameters, "ROOK_CSI_NFS_POD_LABELS", "")
	CSIParam.CSINFSPodLabels = k8sutil.ParseStringToLabels(csiNFSPodLabels)
	csiRBDPodLabels := k8sutil.GetValue(r.opConfig.Parameters, "ROOK_CSI_RBD_POD_LABELS", "")
	CSIParam.CSIRBDPodLabels = k8sutil.ParseStringToLabels(csiRBDPodLabels)

	return nil
}
