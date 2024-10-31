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
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"

	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (r *ReconcileCSI) validateAndConfigureDrivers(ownerInfo *k8sutil.OwnerInfo) error {
	var (
		err error
	)

	if err = r.setParams(); err != nil {
		return errors.Wrapf(err, "failed to configure CSI parameters")
	}

	if err = validateCSIParam(); err != nil {
		return errors.Wrapf(err, "failed to validate CSI parameters")
	}

	if CSIEnabled() {
		if err = r.startDrivers(ownerInfo); err != nil {
			return errors.Wrap(err, "failed to start ceph csi drivers")
		}
	}

	// Check whether RBD or CephFS needs to be disabled
	return r.stopDrivers()
}

func (r *ReconcileCSI) setParams() error {
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

	// parse Liveness port
	CSIParam.CephFSLivenessMetricsPort, err = getPortFromConfig(r.opConfig.Parameters, "CSI_CEPHFS_LIVENESS_METRICS_PORT", DefaultCephFSLivenessMerticsPort)
	if err != nil {
		return errors.Wrap(err, "error getting CSI CephFS liveness metrics port.")
	}
	CSIParam.CSIAddonsPort, err = getPortFromConfig(r.opConfig.Parameters, "CSIADDONS_PORT", DefaultCSIAddonsPort)
	if err != nil {
		return errors.Wrap(err, "failed to get CSI Addons port")
	}
	CSIParam.RBDLivenessMetricsPort, err = getPortFromConfig(r.opConfig.Parameters, "CSI_RBD_LIVENESS_METRICS_PORT", DefaultRBDLivenessMerticsPort)
	if err != nil {
		return errors.Wrap(err, "error getting CSI RBD liveness metrics port.")
	}

	CSIParam.EnableLiveness, err = strconv.ParseBool(k8sutil.GetValue(r.opConfig.Parameters, "CSI_ENABLE_LIVENESS", "false"))
	if err != nil {
		return errors.Wrap(err, "failed to parse value for 'CSI_ENABLE_LIVENESS'")
	}

	CSIParam.Privileged = controller.HostPathRequiresPrivileged()

	// default value `system-node-critical` is the highest available priority
	CSIParam.PluginPriorityClassName = k8sutil.GetValue(r.opConfig.Parameters, "CSI_PLUGIN_PRIORITY_CLASSNAME", "")

	// default value `system-cluster-critical` is applied for some
	// critical pods in cluster but less priority than plugin pods
	CSIParam.ProvisionerPriorityClassName = k8sutil.GetValue(r.opConfig.Parameters, "CSI_PROVISIONER_PRIORITY_CLASSNAME", "")

	CSIParam.EnableOMAPGenerator = false
	if strings.EqualFold(k8sutil.GetValue(r.opConfig.Parameters, "CSI_ENABLE_OMAP_GENERATOR", "false"), "true") {
		CSIParam.EnableOMAPGenerator = true
	}

	CSIParam.EnableCSIDriverSeLinuxMount = true

	CSIParam.EnableRBDSnapshotter = true
	if strings.EqualFold(k8sutil.GetValue(r.opConfig.Parameters, "CSI_ENABLE_RBD_SNAPSHOTTER", "true"), "false") {
		CSIParam.EnableRBDSnapshotter = false
	}

	CSIParam.EnableCephFSSnapshotter = true
	if strings.EqualFold(k8sutil.GetValue(r.opConfig.Parameters, "CSI_ENABLE_CEPHFS_SNAPSHOTTER", "true"), "false") {
		CSIParam.EnableCephFSSnapshotter = false
	}

	CSIParam.EnableNFSSnapshotter = true
	if strings.EqualFold(k8sutil.GetValue(r.opConfig.Parameters, "CSI_ENABLE_NFS_SNAPSHOTTER", "true"), "false") {
		CSIParam.EnableNFSSnapshotter = false
	}

	CSIParam.EnableCSIAddonsSideCar = false
	_, err = r.context.ApiExtensionsClient.ApiextensionsV1().CustomResourceDefinitions().Get(r.opManagerContext, "csiaddonsnodes.csiaddons.openshift.io", metav1.GetOptions{})
	if err == nil {
		CSIParam.EnableCSIAddonsSideCar = true
	}
	if strings.EqualFold(k8sutil.GetValue(r.opConfig.Parameters, "CSI_ENABLE_CSIADDONS", ""), "false") {
		CSIParam.EnableCSIAddonsSideCar = false
	}
	if strings.EqualFold(k8sutil.GetValue(r.opConfig.Parameters, "CSI_ENABLE_CSIADDONS", ""), "true") {
		CSIParam.EnableCSIAddonsSideCar = true
	}

	CSIParam.EnableCSITopology = false
	if strings.EqualFold(k8sutil.GetValue(r.opConfig.Parameters, "CSI_ENABLE_TOPOLOGY", "false"), "true") {
		CSIParam.EnableCSITopology = true
	}

	CSIParam.EnableCSIEncryption = false
	if strings.EqualFold(k8sutil.GetValue(r.opConfig.Parameters, "CSI_ENABLE_ENCRYPTION", "false"), "true") {
		CSIParam.EnableCSIEncryption = true
	}

	CSIParam.CSIEnableMetadata = false
	if strings.EqualFold(k8sutil.GetValue(r.opConfig.Parameters, "CSI_ENABLE_METADATA", "false"), "true") {
		CSIParam.CSIEnableMetadata = true
	}

	if strings.EqualFold(k8sutil.GetValue(r.opConfig.Parameters, "CSI_CEPHFS_PLUGIN_UPDATE_STRATEGY", rollingUpdate), onDelete) {
		CSIParam.CephFSPluginUpdateStrategy = onDelete
	} else {
		CSIParam.CephFSPluginUpdateStrategy = rollingUpdate
		CSIParam.CephFSPluginUpdateStrategyMaxUnavailable = k8sutil.GetValue(r.opConfig.Parameters, "CSI_CEPHFS_PLUGIN_UPDATE_STRATEGY_MAX_UNAVAILABLE", "1")
	}

	if strings.EqualFold(k8sutil.GetValue(r.opConfig.Parameters, "CSI_NFS_PLUGIN_UPDATE_STRATEGY", rollingUpdate), onDelete) {
		CSIParam.NFSPluginUpdateStrategy = onDelete
	} else {
		CSIParam.NFSPluginUpdateStrategy = rollingUpdate
	}

	// Default values are based on Kubernetes official documentation.
	// https://kubernetes.io/docs/tasks/manage-daemon/update-daemon-set/#daemonset-update-strategy
	if strings.EqualFold(k8sutil.GetValue(r.opConfig.Parameters, "CSI_RBD_PLUGIN_UPDATE_STRATEGY", rollingUpdate), onDelete) {
		CSIParam.RBDPluginUpdateStrategy = onDelete
	} else {
		CSIParam.RBDPluginUpdateStrategy = rollingUpdate
		CSIParam.RBDPluginUpdateStrategyMaxUnavailable = k8sutil.GetValue(r.opConfig.Parameters, "CSI_RBD_PLUGIN_UPDATE_STRATEGY_MAX_UNAVAILABLE", "1")
	}

	CSIParam.EnablePluginSelinuxHostMount = false
	if strings.EqualFold(k8sutil.GetValue(r.opConfig.Parameters, "CSI_PLUGIN_ENABLE_SELINUX_HOST_MOUNT", "false"), "true") {
		CSIParam.EnablePluginSelinuxHostMount = true
	}

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

	sidecarLogLevel := k8sutil.GetValue(r.opConfig.Parameters, "CSI_SIDECAR_LOG_LEVEL", "")
	CSIParam.SidecarLogLevel = defaultSidecarLogLevel
	if sidecarLogLevel != "" {
		l, err := strconv.ParseUint(sidecarLogLevel, 10, 8)
		if err != nil {
			logger.Errorf("failed to parse CSI_SIDECAR_LOG_LEVEL. Defaulting to %d. %v", defaultSidecarLogLevel, err)
		} else {
			CSIParam.SidecarLogLevel = uint8(l)
		}
	}

	leaderElectionLeaseDuration := k8sutil.GetValue(r.opConfig.Parameters, "CSI_LEADER_ELECTION_LEASE_DURATION", "")
	CSIParam.LeaderElectionLeaseDuration = defaultLeaderElectionLeaseDuration
	if leaderElectionLeaseDuration != "" {
		d, err := time.ParseDuration(leaderElectionLeaseDuration)
		if err != nil {
			logger.Errorf("failed to parse CSI_LEADER_ELECTION_LEASE_DURATION. Defaulting to %s. %v", defaultLeaderElectionLeaseDuration, err)
		} else {
			CSIParam.LeaderElectionLeaseDuration = d
		}
	}

	leaderElectionRenewDeadline := k8sutil.GetValue(r.opConfig.Parameters, "CSI_LEADER_ELECTION_RENEW_DEADLINE", "")
	CSIParam.LeaderElectionRenewDeadline = defaultLeaderElectionRenewDeadline
	if leaderElectionRenewDeadline != "" {
		d, err := time.ParseDuration(leaderElectionRenewDeadline)
		if err != nil {
			logger.Errorf("failed to parse CSI_LEADER_ELECTION_RENEW_DEADLINE. Defaulting to %s. %v", defaultLeaderElectionRenewDeadline, err)
		} else {
			CSIParam.LeaderElectionRenewDeadline = d
		}
	}

	leaderElectionRetryPeriod := k8sutil.GetValue(r.opConfig.Parameters, "CSI_LEADER_ELECTION_RETRY_PERIOD", "")
	CSIParam.LeaderElectionRetryPeriod = defaultLeaderElectionRetryPeriod
	if leaderElectionRetryPeriod != "" {
		d, err := time.ParseDuration(leaderElectionRetryPeriod)
		if err != nil {
			logger.Errorf("failed to parse CSI_LEADER_ELECTION_RETRY_PERIOD. Defaulting to %s. %v", defaultLeaderElectionRetryPeriod, err)
		} else {
			CSIParam.LeaderElectionRetryPeriod = d
		}
	}

	CSIParam.ProvisionerReplicas = defaultProvisionerReplicas
	nodes, err := r.context.Clientset.CoreV1().Nodes().List(r.opManagerContext, metav1.ListOptions{})
	if err == nil {
		if len(nodes.Items) == 1 {
			CSIParam.ProvisionerReplicas = 1
		} else {
			replicaStr := k8sutil.GetValue(r.opConfig.Parameters, "CSI_PROVISIONER_REPLICAS", "2")
			replicas, err := strconv.ParseInt(replicaStr, 10, 32)
			if err != nil {
				logger.Errorf("failed to parse CSI_PROVISIONER_REPLICAS. Defaulting to %d. %v", defaultProvisionerReplicas, err)
			} else {
				CSIParam.ProvisionerReplicas = int32(replicas)
			}
		}
	} else {
		logger.Errorf("failed to get nodes. Defaulting the number of replicas of provisioner pods to %d. %v", CSIParam.ProvisionerReplicas, err)
	}

	CSIParam.CSIPluginImage = getImage(r.opConfig.Parameters, "ROOK_CSI_CEPH_IMAGE", DefaultCSIPluginImage)
	CSIParam.RegistrarImage = getImage(r.opConfig.Parameters, "ROOK_CSI_REGISTRAR_IMAGE", DefaultRegistrarImage)
	CSIParam.ProvisionerImage = getImage(r.opConfig.Parameters, "ROOK_CSI_PROVISIONER_IMAGE", DefaultProvisionerImage)
	CSIParam.AttacherImage = getImage(r.opConfig.Parameters, "ROOK_CSI_ATTACHER_IMAGE", DefaultAttacherImage)
	CSIParam.SnapshotterImage = getImage(r.opConfig.Parameters, "ROOK_CSI_SNAPSHOTTER_IMAGE", DefaultSnapshotterImage)
	CSIParam.ResizerImage = getImage(r.opConfig.Parameters, "ROOK_CSI_RESIZER_IMAGE", DefaultResizerImage)
	CSIParam.KubeletDirPath = k8sutil.GetValue(r.opConfig.Parameters, "ROOK_CSI_KUBELET_DIR_PATH", DefaultKubeletDirPath)
	CSIParam.CSIAddonsImage = getImage(r.opConfig.Parameters, "ROOK_CSIADDONS_IMAGE", DefaultCSIAddonsImage)
	CSIParam.CSIDomainLabels = k8sutil.GetValue(r.opConfig.Parameters, "CSI_TOPOLOGY_DOMAIN_LABELS", "")
	csiCephFSPodLabels := k8sutil.GetValue(r.opConfig.Parameters, "ROOK_CSI_CEPHFS_POD_LABELS", "")
	CSIParam.CSICephFSPodLabels = k8sutil.ParseStringToLabels(csiCephFSPodLabels)
	csiNFSPodLabels := k8sutil.GetValue(r.opConfig.Parameters, "ROOK_CSI_NFS_POD_LABELS", "")
	CSIParam.CSINFSPodLabels = k8sutil.ParseStringToLabels(csiNFSPodLabels)
	csiRBDPodLabels := k8sutil.GetValue(r.opConfig.Parameters, "ROOK_CSI_RBD_POD_LABELS", "")
	CSIParam.CSIRBDPodLabels = k8sutil.ParseStringToLabels(csiRBDPodLabels)
	CSIParam.CSIClusterName = k8sutil.GetValue(r.opConfig.Parameters, "CSI_CLUSTER_NAME", "")
	CSIParam.ImagePullPolicy = k8sutil.GetValue(r.opConfig.Parameters, "ROOK_CSI_IMAGE_PULL_POLICY", DefaultCSIImagePullPolicy)
	CSIParam.CephFSKernelMountOptions = k8sutil.GetValue(r.opConfig.Parameters, "CSI_CEPHFS_KERNEL_MOUNT_OPTIONS", "")

	CSIParam.CephFSAttachRequired = true
	if strings.EqualFold(k8sutil.GetValue(r.opConfig.Parameters, "CSI_CEPHFS_ATTACH_REQUIRED", "true"), "false") {
		CSIParam.CephFSAttachRequired = false
	}
	CSIParam.RBDAttachRequired = true
	if strings.EqualFold(k8sutil.GetValue(r.opConfig.Parameters, "CSI_RBD_ATTACH_REQUIRED", "true"), "false") {
		CSIParam.RBDAttachRequired = false
	}
	CSIParam.NFSAttachRequired = true
	if strings.EqualFold(k8sutil.GetValue(r.opConfig.Parameters, "CSI_NFS_ATTACH_REQUIRED", "true"), "false") {
		CSIParam.NFSAttachRequired = false
	}

	CSIParam.DriverNamePrefix = k8sutil.GetValue(r.opConfig.Parameters, "CSI_DRIVER_NAME_PREFIX", r.opConfig.OperatorNamespace)

	_, err = r.context.ApiExtensionsClient.ApiextensionsV1().CustomResourceDefinitions().Get(context.TODO(), "volumegroupsnapshotclasses.groupsnapshot.storage.k8s.io", metav1.GetOptions{})
	if err != nil && !kerrors.IsNotFound(err) {
		return errors.Wrapf(err, "failed to get volumegroupsnapshotclasses.groupsnapshot.storage.k8s.io CRD")
	}
	CSIParam.VolumeGroupSnapshotSupported = (err == nil)

	CSIParam.EnableVolumeGroupSnapshot = true
	if strings.EqualFold(k8sutil.GetValue(r.opConfig.Parameters, "CSI_ENABLE_VOLUME_GROUP_SNAPSHOT", "true"), "false") {
		CSIParam.EnableVolumeGroupSnapshot = false
	}

	kubeApiBurst := k8sutil.GetValue(r.opConfig.Parameters, "CSI_KUBE_API_BURST", "")
	CSIParam.KubeApiBurst = 0
	if kubeApiBurst != "" {
		k, err := strconv.ParseUint(kubeApiBurst, 10, 16)
		if err != nil {
			logger.Errorf("failed to parse CSI_KUBE_API_BURST. %v", err)
		} else {
			CSIParam.KubeApiBurst = uint16(k)
		}
	}

	kubeApiQPS := k8sutil.GetValue(r.opConfig.Parameters, "CSI_KUBE_API_QPS", "")
	CSIParam.KubeApiQPS = 0
	if kubeApiQPS != "" {
		k, err := strconv.ParseFloat(kubeApiQPS, 32)
		if err != nil {
			logger.Errorf("failed to parse CSI_KUBE_API_QPS. %v", err)
		} else {
			CSIParam.KubeApiQPS = float32(k)
		}
	}

	return nil
}
