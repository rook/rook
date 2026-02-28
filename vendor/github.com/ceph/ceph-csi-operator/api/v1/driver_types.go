/*
Copyright 2025.

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

package v1

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PeriodicityType string

const (
	HourlyPeriod  PeriodicityType = "hourly"
	DailyPeriod   PeriodicityType = "daily"
	WeeklyPeriod  PeriodicityType = "weekly"
	MonthlyPeriod PeriodicityType = "monthly"
)

// +kubebuilder:validation:XValidation:message="Either maxLogSize or periodicity must be set",rule="(has(self.maxLogSize)) || (has(self.periodicity))"
type LogRotationSpec struct {
	// MaxFiles is the number of logrtoate files
	// Default to 7
	//+kubebuilder:validation:Optional
	MaxFiles int `json:"maxFiles,omitempty"`

	// MaxLogSize is the maximum size of the log file per csi pods
	//+kubebuilder:validation:Optional
	MaxLogSize resource.Quantity `json:"maxLogSize,omitempty"`

	// Periodicity is the periodicity of the log rotation.
	//+kubebuilder:validation:Optional
	//+kubebuilder:validation:Enum:=hourly;daily;weekly;monthly
	Periodicity PeriodicityType `json:"periodicity,omitempty"`

	// LogHostPath is the prefix directory path for the csi log files
	// Default to /var/lib/cephcsi
	//+kubebuilder:validation:Optional
	LogHostPath string `json:"logHostPath,omitempty"`
}

type LogSpec struct {
	// Log verbosity level for driver pods,
	// Supported values from 0 to 5. 0 for general useful logs (the default), 5 for trace level verbosity.
	// Default to 0
	//+kubebuilder:validation:Minimum=0
	//+kubebuilder:validation:Maximum=5
	//+kubebuilder:validation:Optional
	Verbosity int `json:"verbosity,omitempty"`

	// log rotation for csi pods
	//+kubebuilder:validation:Optional
	Rotation *LogRotationSpec `json:"rotation,omitempty"`
}

type SnapshotPolicyType string

const (
	// Disables the feature and remove the snapshotter sidercar
	NoneSnapshotPolicy SnapshotPolicyType = "none"

	// Enable the volumegroupsnapshot feature (will results in deployment of a snapshotter sidecar)
	VolumeGroupSnapshotPolicy SnapshotPolicyType = "volumeGroupSnapshot"

	// Enable the volumesnapshot feature (will results in deployment of a snapshotter sidecar)
	VolumeSnapshotSnapshotPolicy SnapshotPolicyType = "volumeSnapshot"
)

type EncryptionSpec struct {
	//+kubebuilder:validation:Required
	//+kubebuilder:validation:XValidation:rule=self.name != "",message="'.name' cannot be empty"
	ConfigMapRef corev1.LocalObjectReference `json:"configMapName,omitempty"`
}

type VolumeSpec struct {
	//+kubebuilder:validation:Optional
	Volume corev1.Volume `json:"volume,omitempty"`

	//+kubebuilder:validation:Optional
	Mount corev1.VolumeMount `json:"mount,omitempty"`
}

type PodCommonSpec struct {
	// Service account name to be used for driver's pods
	//+kubebuilder:validation:Optional
	ServiceAccountName *string `json:"serviceAccountName,omitempty"`

	// Pod's user defined priority class name
	//+kubebuilder:validation:Optional
	PrioritylClassName *string `json:"priorityClassName,omitempty"`

	// Pod's labels
	//+kubebuilder:validation:Optional
	Labels map[string]string `json:"labels,omitempty"`

	// Pod's annotations
	//+kubebuilder:validation:Optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// Pod's affinity settings
	//+kubebuilder:validation:Optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// Pod's tolerations list
	//+kubebuilder:validation:Optional
	//+kubebuilder:validation:minItems:=1
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// Volume and volume mount definitions to attach to the pod
	//+kubebuilder:validation:Optional
	//+kubebuilder:validation:minItems:=1
	Volumes []VolumeSpec `json:"volumes,omitempty"`

	// To indicate the image pull policy to be applied to all the containers in the csi driver pods.
	//+kubebuilder:validation:Optional
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy"`
}

type NodePluginResourcesSpec struct {
	//+kubebuilder:validation:Optional
	Registrar *corev1.ResourceRequirements `json:"registrar,omitempty"`

	//+kubebuilder:validation:Optional
	Liveness *corev1.ResourceRequirements `json:"liveness,omitempty"`

	//+kubebuilder:validation:Optional
	Addons *corev1.ResourceRequirements `json:"addons,omitempty"`

	//+kubebuilder:validation:Optional
	LogRotator *corev1.ResourceRequirements `json:"logRotator,omitempty"`

	//+kubebuilder:validation:Optional
	Plugin *corev1.ResourceRequirements `json:"plugin,omitempty"`
}

// TopologySpec defines the topology settings for the plugin pods
type TopologySpec struct {
	// Domain labels define which node labels to use as domains for CSI nodeplugins to advertise their domains
	//+kubebuilder:validation:Required
	DomainLabels []string `json:"domainLabels,omitempty"`
}
type NodePluginSpec struct {
	// Embedded common pods spec
	PodCommonSpec `json:",inline"`

	// Driver's plugin daemonset update strategy, supported values are OnDelete and RollingUpdate.
	// Default value is RollingUpdate with MaxAvailabile set to 1
	//+kubebuilder:validation:Optional
	UpdateStrategy *appsv1.DaemonSetUpdateStrategy `json:"updateStrategy,omitempty"`

	// Resource requirements for plugin's containers
	//+kubebuilder:validation:Optional
	Resources NodePluginResourcesSpec `json:"resources,omitempty"`

	// kubelet directory path, if kubelet configured to use other than /var/lib/kubelet path.
	//+kubebuilder:validation:Optional
	KubeletDirPath string `json:"kubeletDirPath,omitempty"`

	// Control the host mount of /etc/selinux for csi plugin pods. Defaults to false
	//+kubebuilder:validation:Optional
	EnableSeLinuxHostMount *bool `json:"enableSeLinuxHostMount,omitempty"`
	// Topology settings for the plugin pods
	//+kubebuilder:validation:Optional
	Topology *TopologySpec `json:"topology,omitempty"`
}

type ControllerPluginResourcesSpec struct {
	//+kubebuilder:validation:Optional
	Attacher *corev1.ResourceRequirements `json:"attacher,omitempty"`

	//+kubebuilder:validation:Optional
	Snapshotter *corev1.ResourceRequirements `json:"snapshotter,omitempty"`

	//+kubebuilder:validation:Optional
	Resizer *corev1.ResourceRequirements `json:"resizer,omitempty"`

	//+kubebuilder:validation:Optional
	Provisioner *corev1.ResourceRequirements `json:"provisioner,omitempty"`

	//+kubebuilder:validation:Optional
	OMapGenerator *corev1.ResourceRequirements `json:"omapGenerator,omitempty"`

	//+kubebuilder:validation:Optional
	Liveness *corev1.ResourceRequirements `json:"liveness,omitempty"`

	//+kubebuilder:validation:Optional
	Addons *corev1.ResourceRequirements `json:"addons,omitempty"`

	//+kubebuilder:validation:Optional
	LogRotator *corev1.ResourceRequirements `json:"logRotator,omitempty"`

	//+kubebuilder:validation:Optional
	Plugin *corev1.ResourceRequirements `json:"plugin,omitempty"`
}

type ControllerPluginSpec struct {
	// hostNetwork setting to be propagated to CSI controller plugin pods
	HostNetwork *bool `json:"hostNetwork,omitempty"`
	// Embedded common pods spec
	PodCommonSpec `json:",inline"`

	// DeploymentStrategy describes how to replace existing pods with new ones
	// Default value is Recreate
	//+kubebuilder:validation:Optional
	DeploymentStrategy *appsv1.DeploymentStrategy `json:"deploymentStrategy,omitempty"`

	// Set replicas for controller plugin's deployment. Defaults to 2
	//+kubebuilder:validation:Optional
	//+kubebuilder:validation:Minimum:=1
	Replicas *int32 `json:"replicas,omitempty"`

	// Resource requirements for controller plugin's containers
	//+kubebuilder:validation:Optional
	Resources ControllerPluginResourcesSpec `json:"resources,omitempty"`

	// To enable logrotation for csi pods,
	// Some platforms require controller plugin to run privileged,
	// For example, OpenShift with SELinux restrictions requires the pod to be privileged to write to hostPath.
	//+kubebuilder:validation:Optional
	Privileged *bool `json:"privileged,omitempty"`
}

type LivenessSpec struct {
	// Port to expose liveness metrics
	//+kubebuilder:validation:Required
	//+kubebuilder:validation:Minimum:=1024
	//+kubebuilder:validation:Maximum:=65535
	MetricsPort int `json:"metricsPort,omitempty"`
}

type LeaderElectionSpec struct {
	// Duration in seconds that non-leader candidates will wait to force acquire leadership.
	// Default to 137 seconds.
	//+kubebuilder:validation:Optional
	//+kubebuilder:validation:Minimum:=0
	LeaseDuration int `json:"leaseDuration,omitempty"`

	// Deadline in seconds that the acting leader will retry refreshing leadership before giving up.
	// Defaults to 107 seconds.
	//+kubebuilder:validation:Optional
	//+kubebuilder:validation:Minimum:=0
	RenewDeadline int `json:"renewDeadline,omitempty"`

	// Retry Period in seconds the LeaderElector clients should wait between tries of actions.
	// Defaults to 26 seconds.
	//+kubebuilder:validation:Optional
	//+kubebuilder:validation:Minimum:=0
	RetryPeriod int `json:"retryPeriod,omitempty"`
}

type CephFsClientType string

const (
	AutoDetectCephFsClient CephFsClientType = "autodetect"
	KernelCephFsClient     CephFsClientType = "kernel"

	// Ceph CSI does not allow us to force Fuse client at this point
	// FuseCephFsClient       CephFsClientType = "fuse"
)

// DriverSpec defines the desired state of Driver
type DriverSpec struct {
	// Logging configuration for driver's pods
	//+kubebuilder:validation:Optional
	Log *LogSpec `json:"log,omitempty"`

	// A reference to a ConfigMap resource holding image overwrite for deployed
	// containers
	//+kubebuilder:validation:Optional
	//+kubebuilder:validation:XValidation:rule=self.name != "",message="'.name' cannot be empty"
	ImageSet *corev1.LocalObjectReference `json:"imageSet,omitempty"`

	// Cluster name identifier to set as metadata on the CephFS subvolume and RBD images. This will be useful in cases
	// when two container orchestrator clusters (Kubernetes/OCP) are using a single ceph cluster.
	//+kubebuilder:validation:Optional
	ClusterName *string `json:"clusterName,omitempty"`

	// Set to true to enable adding volume metadata on the CephFS subvolumes and RBD images.
	// Not all users might be interested in getting volume/snapshot details as metadata on CephFS subvolume and RBD images.
	// Hence enable metadata is false by default.
	//+kubebuilder:validation:Optional
	EnableMetadata *bool `json:"enableMetadata,omitempty"`

	// Set to true to enable fencing for the driver.
	// Fencing is a feature that allows the driver to fence a node when it is tainted with node.kubernetes.io/out-of-service.
	//+kubebuilder:validation:Optional
	EnableFencing *bool `json:"enableFencing,omitempty"`

	// Set the gRPC timeout for gRPC call issued by the driver components
	//+kubebuilder:validation:Optional
	//+kubebuilder:validation:Minimum:=0
	GRpcTimeout int `json:"grpcTimeout,omitempty"`

	// Select a policy for snapshot behavior: none, autodetect, snapshot, sanpshotGroup
	//+kubebuilder:validation:Optional
	//+kubebuilder:validation:Enum:=none;volumeGroupSnapshot;volumeSnapshot
	SnapshotPolicy SnapshotPolicyType `json:"snapshotPolicy,omitempty"`

	// OMAP generator will generate the omap mapping between the PV name and the RBD image.
	// Need to be enabled when we are using rbd mirroring feature.
	// By default OMAP generator sidecar is not deployed with Csi controller plugin pod, to enable
	// it set it to true.
	//+kubebuilder:validation:Optional
	GenerateOMapInfo *bool `json:"generateOMapInfo,omitempty"`

	// Policy for modifying a volume's ownership or permissions when the PVC is being mounted.
	// supported values are documented at https://kubernetes-csi.github.io/docs/support-fsgroup.html
	//+kubebuilder:validation:Optional
	FsGroupPolicy storagev1.FSGroupPolicy `json:"fsGroupPolicy,omitempty"`

	// Driver's encryption settings
	//+kubebuilder:validation:Optional
	Encryption *EncryptionSpec `json:"encryption,omitempty"`

	// Driver's plugin configuration
	//+kubebuilder:validation:Optional
	NodePlugin *NodePluginSpec `json:"nodePlugin,omitempty"`

	// Driver's controller plugin configuration
	//+kubebuilder:validation:Optional
	ControllerPlugin *ControllerPluginSpec `json:"controllerPlugin,omitempty"`

	// Whether to skip any attach operation altogether for CephCsi PVCs.
	// See more details [here](https://kubernetes-csi.github.io/docs/skip-attach.html#skip-attach-with-csi-driver-object).
	// If set to false it skips the volume attachments and makes the creation of pods using the CephCsi PVC fast.
	// **WARNING** It's highly discouraged to use this for RWO volumes. for RBD PVC it can cause data corruption,
	// csi-addons operations like Reclaimspace and PVC Keyrotation will also not be supported if set to false
	// since we'll have no VolumeAttachments to determine which node the PVC is mounted on.
	// Refer to this [issue](https://github.com/kubernetes/kubernetes/issues/103305) for more details.
	//+kubebuilder:validation:Optional
	AttachRequired *bool `json:"attachRequired,omitempty"`

	// Liveness metrics configuration.
	// disabled by default.
	//+kubebuilder:validation:Optional
	Liveness *LivenessSpec `json:"liveness,omitempty"`

	// Leader election setting
	//+kubebuilder:validation:Optional
	LeaderElection *LeaderElectionSpec `json:"leaderElection,omitempty"`

	// TODO: do we want Csi addon specific field? or should we generalize to
	// a list of additional sidecars?
	//+kubebuilder:validation:Optional
	DeployCsiAddons *bool `json:"deployCsiAddons,omitempty"`

	// Select between between cephfs kernel driver and ceph-fuse
	// If you select a non-kernel client, your application may be disrupted during upgrade.
	// See the upgrade guide: https://rook.io/docs/rook/latest/ceph-upgrade.html
	// NOTE! cephfs quota is not supported in kernel version < 4.17
	//+kubebuilder:validation:Optional
	//+kubebuilder:validation:Enum:=autodetect;kernel
	CephFsClientType CephFsClientType `json:"cephFsClientType,omitempty"`

	// Set mount options to use https://docs.ceph.com/en/latest/man/8/mount.ceph/#options
	// Set to "ms_mode=secure" when connections.encrypted is enabled in Ceph
	//+kubebuilder:validation:Optional
	KernelMountOptions map[string]string `json:"kernelMountOptions,omitempty"`

	// Set mount options to use when using the Fuse client
	//+kubebuilder:validation:Optional
	FuseMountOptions map[string]string `json:"fuseMountOptions,omitempty"`
}

// DriverStatus defines the observed state of Driver
type DriverStatus struct {
}

//+kubebuilder:object:root=true
//+kubebuilder:storageversion
//+kubebuilder:subresource:status

// +kubebuilder:validation:XValidation:rule=self.metadata.name.matches('^(.+\\.)?(rbd|cephfs|nfs)?\\.csi\\.ceph\\.com$'),message=".metadata.name must match: '[<prefix>.](rbd|cephfs|nfs).csi.ceph.com'"
// Driver is the Schema for the drivers API
type Driver struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DriverSpec   `json:"spec,omitempty"`
	Status DriverStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DriverList contains a list of Driver
type DriverList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Driver `json:"items"`
}
