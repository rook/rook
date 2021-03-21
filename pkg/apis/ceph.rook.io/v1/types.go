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

package v1

import (
	"time"

	rookv1 "github.com/rook/rook/pkg/apis/rook.io/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ***************************************************************************
// IMPORTANT FOR CODE GENERATION
// If the types in this file are updated, you will need to run
// `make codegen` to generate the new types under the client/clientset folder.
// ***************************************************************************

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CephCluster is a Ceph storage cluster
type CephCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              ClusterSpec `json:"spec"`
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	// +nullable
	Status ClusterStatus `json:"status,omitempty"`
}

// CephClusterHealthCheckSpec represent the healthcheck for Ceph daemons
type CephClusterHealthCheckSpec struct {
	// DaemonHealth is the health check for a given daemon
	// +optional
	// +nullable
	DaemonHealth DaemonHealthSpec `json:"daemonHealth,omitempty"`
	// LivenessProbe allows to change the livenessprobe configuration for a given daemon
	// +optional
	LivenessProbe map[rookv1.KeyType]*rookv1.ProbeSpec `json:"livenessProbe,omitempty"`
}

// DaemonHealthSpec is a daemon health check
type DaemonHealthSpec struct {
	// Status represents the health check settings for the Ceph health
	// +optional
	// +nullable
	Status HealthCheckSpec `json:"status,omitempty"`
	// Monitor represents the health check settings for the Ceph monitor
	// +optional
	// +nullable
	Monitor HealthCheckSpec `json:"mon,omitempty"`
	// ObjectStorageDaemon represents the health check settings for the Ceph OSDs
	// +optional
	// +nullable
	ObjectStorageDaemon HealthCheckSpec `json:"osd,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CephClusterList is a list of CephCluster
type CephClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []CephCluster `json:"items"`
}

// ClusterSpec represents the specification of Ceph Cluster
type ClusterSpec struct {
	// The version information that instructs Rook to orchestrate a particular version of Ceph.
	// +optional
	// +nullable
	CephVersion CephVersionSpec `json:"cephVersion,omitempty"`

	// A spec for available storage in the cluster and how it should be used
	// +optional
	// +nullable
	Storage rookv1.StorageScopeSpec `json:"storage,omitempty"`

	// The annotations-related configuration to add/set on each Pod related object.
	// +nullable
	// +optional
	Annotations rookv1.AnnotationsSpec `json:"annotations,omitempty"`

	// The labels-related configuration to add/set on each Pod related object.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Labels rookv1.LabelsSpec `json:"labels,omitempty"`

	// The placement-related configuration to pass to kubernetes (affinity, node selector, tolerations).
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Placement rookv1.PlacementSpec `json:"placement,omitempty"`

	// Network related configuration
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Network NetworkSpec `json:"network,omitempty"`

	// Resources set resource requests and limits
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Resources rookv1.ResourceSpec `json:"resources,omitempty"`

	// PriorityClassNames sets priority classes on components
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	PriorityClassNames rookv1.PriorityClassNamesSpec `json:"priorityClassNames,omitempty"`

	// The path on the host where config and data can be persisted
	// +optional
	DataDirHostPath string `json:"dataDirHostPath,omitempty"`

	// SkipUpgradeChecks defines if an upgrade should be forced even if one of the check fails
	// +optional
	SkipUpgradeChecks bool `json:"skipUpgradeChecks,omitempty"`

	// ContinueUpgradeAfterChecksEvenIfNotHealthy defines if an upgrade should continue even if PGs are not clean
	// +optional
	ContinueUpgradeAfterChecksEvenIfNotHealthy bool `json:"continueUpgradeAfterChecksEvenIfNotHealthy,omitempty"`

	// WaitTimeoutForHealthyOSDInMinutes defines the time the operator would wait before an OSD can be stopped for upgrade or restart.
	// If the timeout exceeds and OSD is not ok to stop, then the operator would skip upgrade for the current OSD and proceed with the next one
	// if `continueUpgradeAfterChecksEvenIfNotHealthy` is `false`. If `continueUpgradeAfterChecksEvenIfNotHealthy` is `true`, then operator would
	// continue with the upgrade of an OSD even if its not ok to stop after the timeout. This timeout won't be applied if `skipUpgradeChecks` is `true`.
	// The default wait timeout is 10 minutes.
	// +optional
	WaitTimeoutForHealthyOSDInMinutes time.Duration `json:"waitTimeoutForHealthyOSDInMinutes,omitempty"`

	// A spec for configuring disruption management.
	// +nullable
	// +optional
	DisruptionManagement DisruptionManagementSpec `json:"disruptionManagement,omitempty"`

	// A spec for mon related options
	// +optional
	// +nullable
	Mon MonSpec `json:"mon,omitempty"`

	// A spec for the crash controller
	// +optional
	// +nullable
	CrashCollector CrashCollectorSpec `json:"crashCollector,omitempty"`

	// Dashboard settings
	// +optional
	// +nullable
	Dashboard DashboardSpec `json:"dashboard,omitempty"`

	// Prometheus based Monitoring settings
	// +optional
	// +nullable
	Monitoring MonitoringSpec `json:"monitoring,omitempty"`

	// Whether the Ceph Cluster is running external to this Kubernetes cluster
	// mon, mgr, osd, mds, and discover daemons will not be created for external clusters.
	// +optional
	// +nullable
	External ExternalSpec `json:"external,omitempty"`

	// A spec for mgr related options
	// +optional
	// +nullable
	Mgr MgrSpec `json:"mgr,omitempty"`

	// Remove the OSD that is out and safe to remove only if this option is true
	// +optional
	RemoveOSDsIfOutAndSafeToRemove bool `json:"removeOSDsIfOutAndSafeToRemove,omitempty"`

	// Indicates user intent when deleting a cluster; blocks orchestration and should not be set if cluster
	// deletion is not imminent.
	// +optional
	// +nullable
	CleanupPolicy CleanupPolicySpec `json:"cleanupPolicy,omitempty"`

	// Internal daemon healthchecks and liveness probe
	// +optional
	// +nullable
	HealthCheck CephClusterHealthCheckSpec `json:"healthCheck,omitempty"`

	// Security represents security settings
	// +optional
	// +nullable
	Security SecuritySpec `json:"security,omitempty"`

	// Logging represents loggings settings
	// +optional
	// +nullable
	LogCollector LogCollectorSpec `json:"logCollector,omitempty"`
}

// LogCollectorSpec is the logging spec
type LogCollectorSpec struct {
	// Enabled represents whether the log collector is enabled
	// +optional
	Enabled bool `json:"enabled,omitempty"`
	// Periodicity is the periodicity of the log rotation
	// +optional
	Periodicity string `json:"periodicity,omitempty"`
}

// SecuritySpec is security spec to include various security items such as kms
type SecuritySpec struct {
	// KeyManagementService is the main Key Management option
	// +optional
	// +nullable
	KeyManagementService KeyManagementServiceSpec `json:"kms,omitempty"`
}

// KeyManagementServiceSpec represent various details of the KMS server
type KeyManagementServiceSpec struct {
	// ConnectionDetails contains the KMS connection details (address, port etc)
	// +optional
	// +nullable
	// +kubebuilder:pruning:PreserveUnknownFields
	ConnectionDetails map[string]string `json:"connectionDetails,omitempty"`
	// TokenSecretName is the kubernetes secret containing the KMS token
	// +optional
	TokenSecretName string `json:"tokenSecretName,omitempty"`
}

// CephVersionSpec represents the settings for the Ceph version that Rook is orchestrating.
type CephVersionSpec struct {
	// Image is the container image used to launch the ceph daemons, such as ceph/ceph:v15.2.9
	Image string `json:"image"`

	// Whether to allow unsupported versions (do not set to true in production)
	// +optional
	AllowUnsupported bool `json:"allowUnsupported,omitempty"`
}

// DashboardSpec represents the settings for the Ceph dashboard
type DashboardSpec struct {
	// Enabled determines whether to enable the dashboard
	// +optional
	Enabled bool `json:"enabled,omitempty"`
	// URLPrefix is a prefix for all URLs to use the dashboard with a reverse proxy
	// +optional
	URLPrefix string `json:"urlPrefix,omitempty"`
	// Port is the dashboard webserver port
	// +optional
	Port int `json:"port,omitempty"`
	// SSL determines whether SSL should be used
	// +optional
	SSL bool `json:"ssl,omitempty"`
}

// MonitoringSpec represents the settings for Prometheus based Ceph monitoring
type MonitoringSpec struct {
	// Enabled determines whether to create the prometheus rules for the ceph cluster. If true, the prometheus
	// types must exist or the creation will fail.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// RulesNamespace is the namespace where the prometheus rules and alerts should be created.
	// If empty, the same namespace as the cluster will be used.
	// +optional
	RulesNamespace string `json:"rulesNamespace,omitempty"`

	// ExternalMgrEndpoints points to an existing Ceph prometheus exporter endpoint
	// +optional
	// +nullable
	ExternalMgrEndpoints []v1.EndpointAddress `json:"externalMgrEndpoints,omitempty"`

	// ExternalMgrPrometheusPort Prometheus exporter port
	// +optional
	ExternalMgrPrometheusPort uint16 `json:"externalMgrPrometheusPort,omitempty"`
}

// ClusterStatus represents the status of a Ceph cluster
type ClusterStatus struct {
	State       ClusterState    `json:"state,omitempty"`
	Phase       ConditionType   `json:"phase,omitempty"`
	Message     string          `json:"message,omitempty"`
	Conditions  []Condition     `json:"conditions,omitempty"`
	CephStatus  *CephStatus     `json:"ceph,omitempty"`
	CephStorage *CephStorage    `json:"storage,omitempty"`
	CephVersion *ClusterVersion `json:"version,omitempty"`
}

// CephStatus is the details health of a Ceph Cluster
type CephStatus struct {
	Health         string                       `json:"health,omitempty"`
	Details        map[string]CephHealthMessage `json:"details,omitempty"`
	LastChecked    string                       `json:"lastChecked,omitempty"`
	LastChanged    string                       `json:"lastChanged,omitempty"`
	PreviousHealth string                       `json:"previousHealth,omitempty"`
	Capacity       Capacity                     `json:"capacity,omitempty"`
}

// Capacity is the capacity information of a Ceph Cluster
type Capacity struct {
	TotalBytes     uint64 `json:"bytesTotal,omitempty"`
	UsedBytes      uint64 `json:"bytesUsed,omitempty"`
	AvailableBytes uint64 `json:"bytesAvailable,omitempty"`
	LastUpdated    string `json:"lastUpdated,omitempty"`
}

// CephStorage represents flavors of Ceph Cluster Storage
type CephStorage struct {
	DeviceClasses []DeviceClasses `json:"deviceClasses,omitempty"`
}

// DeviceClasses represents device classes of a Ceph Cluster
type DeviceClasses struct {
	Name string `json:"name,omitempty"`
}

// ClusterVersion represents the version of a Ceph Cluster
type ClusterVersion struct {
	Image   string `json:"image,omitempty"`
	Version string `json:"version,omitempty"`
}

// CephHealthMessage represents the health message of a Ceph Cluster
type CephHealthMessage struct {
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

// Condition represents
type Condition struct {
	Type               ConditionType      `json:"type,omitempty"`
	Status             v1.ConditionStatus `json:"status,omitempty"`
	Reason             ClusterReasonType  `json:"reason,omitempty"`
	Message            string             `json:"message,omitempty"`
	LastHeartbeatTime  metav1.Time        `json:"lastHeartbeatTime,omitempty"`
	LastTransitionTime metav1.Time        `json:"lastTransitionTime,omitempty"`
}

// ClusterReasonType is cluster reason
type ClusterReasonType string

const (
	// ClusterCreatedReason is cluster created reason
	ClusterCreatedReason ClusterReasonType = "ClusterCreated"
	// ClusterConnectedReason is cluster connected reason
	ClusterConnectedReason ClusterReasonType = "ClusterConnected"
	// ClusterProgressingReason is cluster progressing reason
	ClusterProgressingReason ClusterReasonType = "ClusterProgressing"
	// ClusterDeletingReason is cluster deleting reason
	ClusterDeletingReason ClusterReasonType = "ClusterDeleting"
	// ClusterConnectingReason is cluster connecting reason
	ClusterConnectingReason ClusterReasonType = "ClusterConnecting"
)

// ConditionType represent a resource's status
type ConditionType string

const (
	// ConditionConnecting represents Connecting state of an object
	ConditionConnecting ConditionType = "Connecting"
	// ConditionConnected represents Connected state of an object
	ConditionConnected ConditionType = "Connected"
	// ConditionProgressing represents Progressing state of an object
	ConditionProgressing ConditionType = "Progressing"
	// ConditionReady represents Ready state of an object
	ConditionReady ConditionType = "Ready"
	// ConditionFailure represents Failure state of an object
	ConditionFailure ConditionType = "Failure"
	// ConditionDeleting represents Deleting state of an object
	ConditionDeleting ConditionType = "Deleting"
)

// ClusterState represents the state of a Ceph Cluster
type ClusterState string

const (
	// ClusterStateCreating represents the Creating state of a Ceph Cluster
	ClusterStateCreating ClusterState = "Creating"
	// ClusterStateCreated represents the Created state of a Ceph Cluster
	ClusterStateCreated ClusterState = "Created"
	// ClusterStateUpdating represents the Updating state of a Ceph Cluster
	ClusterStateUpdating ClusterState = "Updating"
	// ClusterStateConnecting represents the Connecting state of a Ceph Cluster
	ClusterStateConnecting ClusterState = "Connecting"
	// ClusterStateConnected represents the Connected state of a Ceph Cluster
	ClusterStateConnected ClusterState = "Connected"
	// ClusterStateError represents the Error state of a Ceph Cluster
	ClusterStateError ClusterState = "Error"
)

// MonSpec represents the specification of the monitor
type MonSpec struct {
	// Count is the number of Ceph monitors
	// +kubebuilder:validation:Minimum=1
	Count int `json:"count"`
	// AllowMultiplePerNode determines if we can run multiple monitors on the same node (not recommended)
	// +optional
	AllowMultiplePerNode bool `json:"allowMultiplePerNode,omitempty"`
	// StretchCluster is the stretch cluster specification
	// +optional
	StretchCluster *StretchClusterSpec `json:"stretchCluster,omitempty"`
	// VolumeClaimTemplate is the PVC definition
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	VolumeClaimTemplate *v1.PersistentVolumeClaim `json:"volumeClaimTemplate,omitempty"`
}

// StretchClusterSpec represents the specification of a stretched Ceph Cluster
type StretchClusterSpec struct {
	// FailureDomainLabel the failure domain name (e,g: zone)
	// +optional
	FailureDomainLabel string `json:"failureDomainLabel,omitempty"`
	// SubFailureDomain is the failure domain within a zone
	// +optional
	SubFailureDomain string `json:"subFailureDomain,omitempty"`
	// Zones is the list of zones
	// +optional
	// +nullable
	Zones []StretchClusterZoneSpec `json:"zones,omitempty"`
}

// StretchClusterZoneSpec represents the specification of a stretched zone in a Ceph Cluster
type StretchClusterZoneSpec struct {
	// Name is the name of the zone
	// +optional
	Name string `json:"name,omitempty"`
	// Arbiter determines if the zone contains the arbiter
	// +optional
	Arbiter bool `json:"arbiter,omitempty"`
	// VolumeClaimTemplate is the PVC template
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	VolumeClaimTemplate *v1.PersistentVolumeClaim `json:"volumeClaimTemplate,omitempty"`
}

// MgrSpec represents options to configure a ceph mgr
type MgrSpec struct {
	// Count is the number of manager to run
	// +optional
	Count int `json:"count,omitempty"`
	// AllowMultiplePerNode allows to run multiple managers on the same node (not recommended)
	// +optional
	AllowMultiplePerNode bool `json:"allowMultiplePerNode,omitempty"`
	// Modules is the list of ceph manager modules to enable/disable
	// +optional
	// +nullable
	Modules []Module `json:"modules,omitempty"`
}

// Module represents mgr modules that the user wants to enable or disable
type Module struct {
	// Name is the name of the ceph manager module
	// +optional
	Name string `json:"name,omitempty"`
	// Enabled determines whether a module should be enabled or not
	// +optional
	Enabled bool `json:"enabled,omitempty"`
}

// ExternalSpec represents the options supported by an external cluster
// +kubebuilder:pruning:PreserveUnknownFields
// +nullable
type ExternalSpec struct {
	// Enable determines whether external mode is enabled or not
	// +optional
	Enable bool `json:"enable,omitempty"`
}

// CrashCollectorSpec represents options to configure the crash controller
type CrashCollectorSpec struct {
	// Disable determines whether we should enable the crash collector
	// +optional
	Disable bool `json:"disable,omitempty"`

	// DaysToRetain represents the number of days to retain crash until they get pruned
	// +optional
	DaysToRetain uint `json:"daysToRetain,omitempty"`
}

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CephBlockPool represents a Ceph Storage Pool
type CephBlockPool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              PoolSpec `json:"spec"`
	// +kubebuilder:pruning:PreserveUnknownFields
	Status *CephBlockPoolStatus `json:"status,omitempty"`
}

// CephBlockPoolList is a list of Ceph Storage Pools
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type CephBlockPoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []CephBlockPool `json:"items"`
}

const (
	// DefaultFailureDomain for PoolSpec
	DefaultFailureDomain = "host"
	// DefaultCRUSHRoot is the default name of the CRUSH root bucket
	DefaultCRUSHRoot = "default"
)

// PoolSpec represents the spec of ceph pool
type PoolSpec struct {
	// The failure domain: osd/host/(region or zone if available) - technically also any type in the crush map
	// +optional
	FailureDomain string `json:"failureDomain,omitempty"`

	// The root of the crush hierarchy utilized by the pool
	// +optional
	CrushRoot string `json:"crushRoot,omitempty"`

	// The device class the OSD should set to (options are: hdd, ssd, or nvme)
	// +kubebuilder:validation:Enum=ssd;hdd;nvme;""
	// +optional
	DeviceClass string `json:"deviceClass,omitempty"`

	// The inline compression mode in Bluestore OSD to set to (options are: none, passive, aggressive, force)
	// +kubebuilder:validation:Enum=none;passive;aggressive;force
	// +kubebuilder:default=none
	// +optional
	CompressionMode string `json:"compressionMode,omitempty"`

	// The replication settings
	// +optional
	Replicated ReplicatedSpec `json:"replicated,omitempty"`

	// The erasure code settings
	// +optional
	ErasureCoded ErasureCodedSpec `json:"erasureCoded,omitempty"`

	// Parameters is a list of properties to enable on a given pool
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	// +nullable
	Parameters map[string]string `json:"parameters,omitempty"`

	// EnableRBDStats is used to enable gathering of statistics for all RBD images in the pool
	EnableRBDStats bool `json:"enableRBDStats,omitempty"`

	// The mirroring settings
	Mirroring MirroringSpec `json:"mirroring,omitempty"`

	// The mirroring statusCheck
	// +kubebuilder:pruning:PreserveUnknownFields
	StatusCheck MirrorHealthCheckSpec `json:"statusCheck,omitempty"`

	// The quota settings
	// +optional
	// +nullable
	Quotas QuotaSpec `json:"quotas,omitempty"`
}

// MirrorHealthCheckSpec represents the health specification of a Ceph Storage Pool mirror
type MirrorHealthCheckSpec struct {
	// +optional
	// +nullable
	Mirror HealthCheckSpec `json:"mirror,omitempty"`
}

// CephBlockPoolStatus represents the mirroring status of Ceph Storage Pool
type CephBlockPoolStatus struct {
	// +optional
	Phase ConditionType `json:"phase,omitempty"`
	// +optional
	MirroringStatus *MirroringStatusSpec `json:"mirroringStatus,omitempty"`
	// +optional
	MirroringInfo *MirroringInfoSpec `json:"mirroringInfo,omitempty"`
	// +optional
	SnapshotScheduleStatus *SnapshotScheduleStatusSpec `json:"snapshotScheduleStatus,omitempty"`
	// Use only info and put mirroringStatus in it?
	// +optional
	// +nullable
	Info map[string]string `json:"info,omitempty"`
}

// MirroringStatusSpec is the status of the pool mirroring
type MirroringStatusSpec struct {
	// PoolMirroringStatus is the mirroring status of a pool
	// +optional
	PoolMirroringStatus `json:",inline"`
	// LastChecked is the last time time the status was checked
	// +optional
	LastChecked string `json:"lastChecked,omitempty"`
	// LastChanged is the last time time the status last changed
	// +optional
	LastChanged string `json:"lastChanged,omitempty"`
	// Details contains potential status errors
	// +optional
	Details string `json:"details,omitempty"`
}

// PoolMirroringStatus is the pool mirror status
type PoolMirroringStatus struct {
	// Summary is the mirroring status summary
	// +optional
	Summary *PoolMirroringStatusSummarySpec `json:"summary,omitempty"`
}

// PoolMirroringStatusSummarySpec is the summary output of the command
type PoolMirroringStatusSummarySpec struct {
	// Health is the mirroring health
	// +optional
	Health string `json:"health,omitempty"`
	// DaemonHealth is the health of the mirroring daemon
	// +optional
	DaemonHealth string `json:"daemon_health,omitempty"`
	// ImageHealth is the health of the mirrored image
	// +optional
	ImageHealth string `json:"image_health,omitempty"`
	// States is the various state for all mirrored images
	// +optional
	// +nullable
	States StatesSpec `json:"states,omitempty"`
}

// StatesSpec are rbd images mirroring state
type StatesSpec struct {
	// StartingReplay is when the replay of the mirroring journal starts
	// +optional
	StartingReplay int `json:"starting_replay,omitempty"`
	// Replaying is when the replay of the mirroring journal is on-going
	// +optional
	Replaying int `json:"replaying,omitempty"`
	// Syncing is when the image is syncing
	// +optional
	Syncing int `json:"syncing,omitempty"`
	// StopReplaying is when the replay of the mirroring journal stops
	// +optional
	StopReplaying int `json:"stopping_replay,omitempty"`
	// Stopped is when the mirroring state is stopped
	// +optional
	Stopped int `json:"stopped,omitempty"`
	// Unknown is when the mirroring state is unknown
	// +optional
	Unknown int `json:"unknown,omitempty"`
	// Error is when the mirroring state is errored
	// +optional
	Error int `json:"error,omitempty"`
}

// MirroringInfoSpec is the status of the pool mirroring
type MirroringInfoSpec struct {
	// +optional
	*PoolMirroringInfo `json:",inline"`
	// +optional
	LastChecked string `json:"lastChecked,omitempty"`
	// +optional
	LastChanged string `json:"lastChanged,omitempty"`
	// +optional
	Details string `json:"details,omitempty"`
}

// PoolMirroringInfo is the mirroring info of a given pool
type PoolMirroringInfo struct {
	// Mode is the mirroring mode
	// +optional
	Mode string `json:"mode,omitempty"`
	// SiteName is the current site name
	// +optional
	SiteName string `json:"site_name,omitempty"`
	// Peers are the list of peer sites connected to that cluster
	// +optional
	Peers []PeersSpec `json:"peers,omitempty"`
}

// PeersSpec contains peer details
type PeersSpec struct {
	// UUID is the peer UUID
	// +optional
	UUID string `json:"uuid,omitempty"`
	// Direction is the peer mirroring direction
	// +optional
	Direction string `json:"direction,omitempty"`
	// SiteName is the current site name
	// +optional
	SiteName string `json:"site_name,omitempty"`
	// MirrorUUID is the mirror UUID
	// +optional
	MirrorUUID string `json:"mirror_uuid,omitempty"`
	// ClientName is the CephX user used to connect to the peer
	// +optional
	ClientName string `json:"client_name,omitempty"`
}

// SnapshotScheduleStatusSpec is the status of the snapshot schedule
type SnapshotScheduleStatusSpec struct {
	// SnapshotSchedules is the list of snapshots scheduled
	// +nullable
	// +optional
	SnapshotSchedules []SnapshotSchedulesSpec `json:"snapshotSchedules,omitempty"`
	// LastChecked is the last time time the status was checked
	// +optional
	LastChecked string `json:"lastChecked,omitempty"`
	// LastChanged is the last time time the status last changed
	// +optional
	LastChanged string `json:"lastChanged,omitempty"`
	// Details contains potential status errors
	// +optional
	Details string `json:"details,omitempty"`
}

// SnapshotSchedulesSpec is the list of snapshot scheduled for images in a pool
type SnapshotSchedulesSpec struct {
	// Pool is the pool name
	// +optional
	Pool string `json:"pool,omitempty"`
	// Namespace is the RADOS namespace the image is part of
	// +optional
	Namespace string `json:"namespace,omitempty"`
	// Image is the mirrored image
	// +optional
	Image string `json:"image,omitempty"`
	// Items is the list schedules times for a given snapshot
	// +optional
	Items []SnapshotSchedule `json:"items,omitempty"`
}

// SnapshotSchedule is a schedule
type SnapshotSchedule struct {
	// Interval is the interval in which snapshots will be taken
	// +optional
	Interval string `json:"interval,omitempty"`
	// StartTime is the snapshot starting time
	// +optional
	StartTime string `json:"start_time,omitempty"`
}

// Status represents the status of an object
type Status struct {
	// +optional
	Phase string `json:"phase,omitempty"`
}

// ReplicatedSpec represents the spec for replication in a pool
type ReplicatedSpec struct {
	// Size - Number of copies per object in a replicated storage pool, including the object itself (required for replicated pool type)
	// +kubebuilder:validation:Minimum=1
	Size uint `json:"size"`

	// TargetSizeRatio gives a hint (%) to Ceph in terms of expected consumption of the total cluster capacity
	// +optional
	TargetSizeRatio float64 `json:"targetSizeRatio,omitempty"`

	// RequireSafeReplicaSize if false allows you to set replica 1
	// +optional
	RequireSafeReplicaSize bool `json:"requireSafeReplicaSize,omitempty"`

	// ReplicasPerFailureDomain the number of replica in the specified failure domain
	// +kubebuilder:validation:Minimum=1
	// +optional
	ReplicasPerFailureDomain uint `json:"replicasPerFailureDomain,omitempty"`

	// SubFailureDomain the name of the sub-failure domain
	// +optional
	SubFailureDomain string `json:"subFailureDomain,omitempty"`
}

// MirroringSpec represents the setting for a mirrored pool
type MirroringSpec struct {
	// Enabled whether this pool is mirrored or not
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// Mode is the mirroring mode: either "pool" or "image"
	// +optional
	Mode string `json:"mode,omitempty"`

	// SnapshotSchedules is the scheduling of snapshot for mirrored images/pools
	// +optional
	SnapshotSchedules []SnapshotScheduleSpec `json:"snapshotSchedules,omitempty"`
}

// SnapshotScheduleSpec represents the snapshot scheduling settings of a mirrored pool
type SnapshotScheduleSpec struct {
	// Interval represent the periodicity of the snapshot.
	// +optional
	Interval string `json:"interval,omitempty"`

	// StartTime indicates when to start the snapshot
	// +optional
	StartTime string `json:"startTime,omitempty"`
}

// QuotaSpec represents the spec for quotas in a pool
type QuotaSpec struct {
	// MaxBytes represents the quota in bytes
	// Deprecated in favor of MaxSize
	// +optional
	MaxBytes *uint64 `json:"maxBytes,omitempty"`

	// MaxSize represents the quota in bytes as a string
	// +kubebuilder:validation:Pattern=`^[0-9]+[\.]?[0-9]*([KMGTPE]i|[kMGTPE])?$`
	// +optional
	MaxSize *string `json:"maxSize,omitempty"`

	// MaxObjects represents the quota in objects
	// +optional
	MaxObjects *uint64 `json:"maxObjects,omitempty"`
}

// ErasureCodedSpec represents the spec for erasure code in a pool
type ErasureCodedSpec struct {
	// Number of coding chunks per object in an erasure coded storage pool (required for erasure-coded pool type)
	CodingChunks uint `json:"codingChunks"`

	// Number of data chunks per object in an erasure coded storage pool (required for erasure-coded pool type)
	DataChunks uint `json:"dataChunks"`

	// The algorithm for erasure coding
	// +optional
	Algorithm string `json:"algorithm,omitempty"`
}

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CephFilesystem represents a Ceph Filesystem
type CephFilesystem struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              FilesystemSpec `json:"spec"`
	// +kubebuilder:pruning:PreserveUnknownFields
	Status *Status `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CephFilesystemList represents a list of Ceph Filesystems
type CephFilesystemList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []CephFilesystem `json:"items"`
}

// FilesystemSpec represents the spec of a file system
type FilesystemSpec struct {
	// The metadata pool settings
	MetadataPool PoolSpec `json:"metadataPool"`

	// The data pool settings
	DataPools []PoolSpec `json:"dataPools"`

	// Preserve pools on filesystem deletion
	// +optional
	PreservePoolsOnDelete bool `json:"preservePoolsOnDelete,omitempty"`

	// Preserve the fs in the cluster on CephFilesystem CR deletion. Setting this to true automatically implies PreservePoolsOnDelete is true.
	// +optional
	PreserveFilesystemOnDelete bool `json:"preserveFilesystemOnDelete,omitempty"`

	// The mds pod info
	MetadataServer MetadataServerSpec `json:"metadataServer"`

	// The mirroring settings
	// +nullable
	// +optional
	Mirroring FSMirroringSpec `json:"mirroring,omitempty"`
}

// MetadataServerSpec represents the specification of a Ceph Metadata Server
type MetadataServerSpec struct {
	// The number of metadata servers that are active. The remaining servers in the cluster will be in standby mode.
	// +kubebuilder:validation:Minimum=1
	ActiveCount int32 `json:"activeCount"`

	// Whether each active MDS instance will have an active standby with a warm metadata cache for faster failover.
	// If false, standbys will still be available, but will not have a warm metadata cache.
	// +optional
	ActiveStandby bool `json:"activeStandby,omitempty"`

	// The affinity to place the mds pods (default is to place on all available node) with a daemonset
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Placement rookv1.Placement `json:"placement,omitempty"`

	// The annotations-related configuration to add/set on each Pod related object.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Annotations rookv1.Annotations `json:"annotations,omitempty"`

	// The labels-related configuration to add/set on each Pod related object.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Labels rookv1.Labels `json:"labels,omitempty"`

	// The resource requirements for the rgw pods
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Resources v1.ResourceRequirements `json:"resources,omitempty"`

	// PriorityClassName sets priority classes on components
	// +optional
	PriorityClassName string `json:"priorityClassName,omitempty"`
}

// FSMirroringSpec represents the setting for a mirrored filesystem
type FSMirroringSpec struct {
	// Enabled whether this filesystem is mirrored or not
	// +optional
	Enabled bool `json:"enabled,omitempty"`
}

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CephObjectStore represents a Ceph Object Store Gateway
type CephObjectStore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              ObjectStoreSpec `json:"spec"`
	// +kubebuilder:pruning:PreserveUnknownFields
	Status *ObjectStoreStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CephObjectStoreList represents a Ceph Object Store Gateways
type CephObjectStoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []CephObjectStore `json:"items"`
}

// ObjectStoreSpec represent the spec of a pool
type ObjectStoreSpec struct {
	// The metadata pool settings
	// +optional
	MetadataPool PoolSpec `json:"metadataPool,omitempty"`

	// The data pool settings
	// +optional
	DataPool PoolSpec `json:"dataPool,omitempty"`

	// Preserve pools on object store deletion
	// +optional
	PreservePoolsOnDelete bool `json:"preservePoolsOnDelete,omitempty"`

	// The rgw pod info
	// +optional
	// +nullable
	Gateway GatewaySpec `json:"gateway"`

	// The multisite info
	// +optional
	// +nullable
	Zone ZoneSpec `json:"zone,omitempty"`

	// The rgw Bucket healthchecks and liveness probe
	// +optional
	// +nullable
	HealthCheck BucketHealthCheckSpec `json:"healthCheck,omitempty"`
}

// BucketHealthCheckSpec represents the health check of an object store
type BucketHealthCheckSpec struct {
	// +optional
	Bucket HealthCheckSpec `json:"bucket,omitempty"`
	// +optional
	LivenessProbe *rookv1.ProbeSpec `json:"livenessProbe,omitempty"`
}

// HealthCheckSpec represents the health check of an object store bucket
type HealthCheckSpec struct {
	// +optional
	Disabled bool `json:"disabled,omitempty"`
	// +optional
	Interval string `json:"interval,omitempty"`
	// +optional
	Timeout string `json:"timeout,omitempty"`
}

// GatewaySpec represents the specification of Ceph Object Store Gateway
type GatewaySpec struct {
	// The port the rgw service will be listening on (http)
	// +optional
	Port int32 `json:"port,omitempty"`

	// The port the rgw service will be listening on (https)
	// +optional
	SecurePort int32 `json:"securePort,omitempty"`

	// The number of pods in the rgw replicaset. If "allNodes" is specified, a daemonset is created.
	// +kubebuilder:validation:Minimum=1
	Instances int32 `json:"instances"`

	// The name of the secret that stores the ssl certificate for secure rgw connections
	// +optional
	SSLCertificateRef string `json:"sslCertificateRef,omitempty"`

	// The affinity to place the rgw pods (default is to place on any available node)
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Placement rookv1.Placement `json:"placement,omitempty"`

	// The annotations-related configuration to add/set on each Pod related object.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Annotations rookv1.Annotations `json:"annotations,omitempty"`

	// The labels-related configuration to add/set on each Pod related object.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Labels rookv1.Labels `json:"labels,omitempty"`

	// The resource requirements for the rgw pods
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Resources v1.ResourceRequirements `json:"resources,omitempty"`

	// PriorityClassName sets priority classes on the rgw pods
	// +optional
	PriorityClassName string `json:"priorityClassName,omitempty"`

	// ExternalRgwEndpoints points to external rgw endpoint(s)
	// +nullable
	// +optional
	ExternalRgwEndpoints []v1.EndpointAddress `json:"externalRgwEndpoints,omitempty"`
}

// ZoneSpec represents a Ceph Object Store Gateway Zone specification
type ZoneSpec struct {
	// RGW Zone the Object Store is in
	Name string `json:"name"`
}

// ObjectStoreStatus represents the status of a Ceph Object Store resource
type ObjectStoreStatus struct {
	// +optional
	Phase ConditionType `json:"phase,omitempty"`
	// +optional
	Message string `json:"message,omitempty"`
	// +optional
	BucketStatus *BucketStatus `json:"bucketStatus,omitempty"`
	// +optional
	// +nullable
	Info map[string]string `json:"info,omitempty"`
}

// BucketStatus represents the status of a bucket
type BucketStatus struct {
	// +optional
	Health ConditionType `json:"health,omitempty"`
	// +optional
	Details string `json:"details,omitempty"`
	// +optional
	LastChecked string `json:"lastChecked,omitempty"`
	// +optional
	LastChanged string `json:"lastChanged,omitempty"`
}

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=rcou;objectuser
// CephObjectStoreUser represents a Ceph Object Store Gateway User
type CephObjectStoreUser struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              ObjectStoreUserSpec `json:"spec"`
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Status *ObjectStoreUserStatus `json:"status,omitempty"`
}

// ObjectStoreUserStatus represents the status Ceph Object Store Gateway User
type ObjectStoreUserStatus struct {
	// +optional
	Phase string `json:"phase,omitempty"`
	// +optional
	// +nullable
	Info map[string]string `json:"info,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CephObjectStoreUserList represents a list Ceph Object Store Gateway Users
type CephObjectStoreUserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []CephObjectStoreUser `json:"items"`
}

// ObjectStoreUserSpec represent the spec of an Objectstoreuser
type ObjectStoreUserSpec struct {
	//The store the user will be created in
	// +optional
	Store string `json:"store,omitempty"`
	//The display name for the ceph users
	// +optional
	DisplayName string `json:"displayName,omitempty"`
}

// CephObjectRealm represents a Ceph Object Store Gateway Realm
// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type CephObjectRealm struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	// +nullable
	// +optional
	Spec ObjectRealmSpec `json:"spec,omitempty"`
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Status *Status `json:"status,omitempty"`
}

// CephObjectRealmList represents a list Ceph Object Store Gateway Realms
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type CephObjectRealmList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []CephObjectRealm `json:"items"`
}

// ObjectRealmSpec represent the spec of an ObjectRealm
type ObjectRealmSpec struct {
	Pull PullSpec `json:"pull"`
}

// PullSpec represents the pulling specification of a Ceph Object Storage Gateway Realm
type PullSpec struct {
	Endpoint string `json:"endpoint"`
}

// CephObjectZoneGroup represents a Ceph Object Store Gateway Zone Group
// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type CephObjectZoneGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              ObjectZoneGroupSpec `json:"spec"`
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Status *Status `json:"status,omitempty"`
}

// CephObjectZoneGroupList represents a list Ceph Object Store Gateway Zone Groups
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type CephObjectZoneGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []CephObjectZoneGroup `json:"items"`
}

// ObjectZoneGroupSpec represent the spec of an ObjectZoneGroup
type ObjectZoneGroupSpec struct {
	//The display name for the ceph users
	Realm string `json:"realm"`
}

// CephObjectZone represents a Ceph Object Store Gateway Zone
// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type CephObjectZone struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              ObjectZoneSpec `json:"spec"`
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Status *Status `json:"status,omitempty"`
}

// CephObjectZoneList represents a list Ceph Object Store Gateway Zones
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type CephObjectZoneList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []CephObjectZone `json:"items"`
}

// ObjectZoneSpec represent the spec of an ObjectZone
type ObjectZoneSpec struct {
	//The display name for the ceph users
	ZoneGroup string `json:"zoneGroup"`

	// The metadata pool settings
	MetadataPool PoolSpec `json:"metadataPool"`

	// The data pool settings
	DataPool PoolSpec `json:"dataPool"`
}

// CephNFS represents a Ceph NFS
// +genclient
// +genclient:noStatus
// +kubebuilder:resource:shortName=nfs,path=cephnfses
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type CephNFS struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              NFSGaneshaSpec `json:"spec"`
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Status *Status `json:"status,omitempty"`
}

// CephNFSList represents a list Ceph NFSes
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type CephNFSList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []CephNFS `json:"items"`
}

// NFSGaneshaSpec represents the spec of an nfs ganesha server
type NFSGaneshaSpec struct {
	// RADOS is the Ganesha RADOS specification
	RADOS GaneshaRADOSSpec `json:"rados"`

	// Server is the Ganesha Server specification
	Server GaneshaServerSpec `json:"server"`
}

// GaneshaRADOSSpec represents the specification of a Ganesha RADOS object
type GaneshaRADOSSpec struct {
	// Pool is the RADOS pool where NFS client recovery data is stored.
	Pool string `json:"pool"`

	// Namespace is the RADOS namespace where NFS client recovery data is stored.
	Namespace string `json:"namespace"`
}

// GaneshaServerSpec represents the specification of a Ganesha Server
type GaneshaServerSpec struct {
	// The number of active Ganesha servers
	Active int `json:"active"`

	// The affinity to place the ganesha pods
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Placement rookv1.Placement `json:"placement,omitempty"`

	// The annotations-related configuration to add/set on each Pod related object.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Annotations rookv1.Annotations `json:"annotations,omitempty"`

	// The labels-related configuration to add/set on each Pod related object.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Labels rookv1.Labels `json:"labels,omitempty"`

	// Resources set resource requests and limits
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Resources v1.ResourceRequirements `json:"resources,omitempty"`

	// PriorityClassName sets the priority class on the pods
	// +optional
	PriorityClassName string `json:"priorityClassName,omitempty"`

	// LogLevel set logging level
	// +optional
	LogLevel string `json:"logLevel,omitempty"`
}

// NetworkSpec for Ceph includes backward compatibility code
type NetworkSpec struct {
	rookv1.NetworkSpec `json:",inline"`

	// HostNetwork to enable host network
	// +optional
	HostNetwork bool `json:"hostNetwork,omitempty"`

	// IPFamily is the single stack IPv6 or IPv4 protocol
	// +nullable
	// +optional
	IPFamily IPFamilyType `json:"ipFamily,omitempty"`
}

// DisruptionManagementSpec configures management of daemon disruptions
type DisruptionManagementSpec struct {
	// This enables management of poddisruptionbudgets
	// +optional
	ManagePodBudgets bool `json:"managePodBudgets,omitempty"`

	// OSDMaintenanceTimeout sets how many additional minutes the DOWN/OUT interval is for drained failure domains
	// it only works if managePodBudgets is true.
	// the default is 30 minutes
	// +optional
	OSDMaintenanceTimeout time.Duration `json:"osdMaintenanceTimeout,omitempty"`

	// PGHealthCheckTimeout is the time (in minutes) that the operator will wait for the placement groups to become
	// healthy (active+clean) after a drain was completed and OSDs came back up. Rook will continue with the next drain
	// if the timeout exceeds. It only works if managePodBudgets is true.
	// No values or 0 means that the operator will wait until the placement groups are healthy before unblocking the next drain.
	// +optional
	PGHealthCheckTimeout time.Duration `json:"pgHealthCheckTimeout,omitempty"`

	// This enables management of machinedisruptionbudgets
	// +optional
	ManageMachineDisruptionBudgets bool `json:"manageMachineDisruptionBudgets,omitempty"`

	// Namespace to look for MDBs by the machineDisruptionBudgetController
	// +optional
	MachineDisruptionBudgetNamespace string `json:"machineDisruptionBudgetNamespace,omitempty"`
}

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CephClient represents a Ceph Client
type CephClient struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	// Spec represents the specification of a Ceph Client
	Spec ClientSpec `json:"spec"`
	// Status represents the status of a Ceph Client
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Status *CephClientStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CephClientList represents a list of Ceph Clients
type CephClientList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []CephClient `json:"items"`
}

// ClientSpec represents the specification of a Ceph Client
type ClientSpec struct {
	// +optional
	Name string `json:"name,omitempty"`
	// +kubebuilder:pruning:PreserveUnknownFields
	Caps map[string]string `json:"caps"`
}

// CephClientStatus represents the Status of Ceph Client
type CephClientStatus struct {
	// +optional
	Phase ConditionType `json:"phase,omitempty"`
	// +optional
	// +nullable
	Info map[string]string `json:"info,omitempty"`
}

// CleanupPolicySpec represents a Ceph Cluster cleanup policy
type CleanupPolicySpec struct {
	// Confirmation represents the cleanup confirmation
	// +optional
	// +nullable
	Confirmation CleanupConfirmationProperty `json:"confirmation,omitempty"`
	// SanitizeDisks represents way we sanitize disks
	// +optional
	// +nullable
	SanitizeDisks SanitizeDisksSpec `json:"sanitizeDisks,omitempty"`
	// AllowUninstallWithVolumes defines whether we can proceed with the uninstall if they are RBD images still present
	// +optional
	AllowUninstallWithVolumes bool `json:"allowUninstallWithVolumes,omitempty"`
}

// CleanupConfirmationProperty represents the cleanup confirmation
type CleanupConfirmationProperty string

// SanitizeDataSourceProperty represents a sanitizing data source
type SanitizeDataSourceProperty string

// SanitizeMethodProperty represents a disk sanitizing method
type SanitizeMethodProperty string

// SanitizeDisksSpec represents a disk sanitizing specification
type SanitizeDisksSpec struct {
	// Method is the method we use to sanitize disks
	// +optional
	Method SanitizeMethodProperty `json:"method,omitempty"`
	// DataSource is the data source to use to sanitize the disk with
	// +optional
	// +kubebuilder:validation:Enum=zero;random
	DataSource SanitizeDataSourceProperty `json:"dataSource,omitempty"`
	// Iteration is the number of pass to apply the sanitizing
	// +optional
	Iteration int32 `json:"iteration,omitempty"`
}

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CephRBDMirror represents a Ceph RBD Mirror
type CephRBDMirror struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              RBDMirroringSpec `json:"spec"`
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Status *Status `json:"status,omitempty"`
}

// CephRBDMirrorList represents a list Ceph RBD Mirrors
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type CephRBDMirrorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []CephRBDMirror `json:"items"`
}

// RBDMirroringSpec represents the specification of an RBD mirror daemon
type RBDMirroringSpec struct {
	// Count represents the number of rbd mirror instance to run
	// +kubebuilder:validation:Minimum=1
	Count int `json:"count"`

	// RBDMirroringPeerSpec represents the peers spec
	// +nullable
	// +optional
	Peers RBDMirroringPeerSpec `json:"peers,omitempty"`

	// The affinity to place the rgw pods (default is to place on any available node)
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Placement rookv1.Placement `json:"placement,omitempty"`

	// The annotations-related configuration to add/set on each Pod related object.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Annotations rookv1.Annotations `json:"annotations,omitempty"`

	// The labels-related configuration to add/set on each Pod related object.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Labels rookv1.Labels `json:"labels,omitempty"`

	// The resource requirements for the rbd mirror pods
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Resources v1.ResourceRequirements `json:"resources,omitempty"`

	// PriorityClassName sets priority class on the rbd mirror pods
	// +optional
	PriorityClassName string `json:"priorityClassName,omitempty"`
}

// RBDMirroringPeerSpec represents the specification of an RBD mirror peer
type RBDMirroringPeerSpec struct {
	// SecretNames represents the Kubernetes Secret names to add rbd-mirror peers
	// +optional
	SecretNames []string `json:"secretNames,omitempty"`
}

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CephFilesystemMirror is the Ceph Filesystem Mirror object definition
type CephFilesystemMirror struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              FilesystemMirroringSpec `json:"spec"`
	// +optional
	Status *Status `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CephFilesystemMirrorList is a list of CephFilesystemMirror
type CephFilesystemMirrorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []CephFilesystemMirror `json:"items"`
}

// FilesystemMirroringSpec is the filesystem mirorring specification
type FilesystemMirroringSpec struct {
	// The affinity to place the rgw pods (default is to place on any available node)
	// +nullable
	// +optional
	Placement rookv1.Placement `json:"placement,omitempty"`

	// The annotations-related configuration to add/set on each Pod related object.
	// +nullable
	// +optional
	Annotations rookv1.Annotations `json:"annotations,omitempty"`

	// The labels-related configuration to add/set on each Pod related object.
	// +nullable
	// +optional
	Labels rookv1.Labels `json:"labels,omitempty"`

	// The resource requirements for the cephfs-mirror pods
	// +nullable
	// +optional
	Resources v1.ResourceRequirements `json:"resources,omitempty"`

	// PriorityClassName sets priority class on the cephfs-mirror pods
	// +optional
	PriorityClassName string `json:"priorityClassName,omitempty"`
}

// IPFamilyType represents the single stack Ipv4 or Ipv6 protocol.
type IPFamilyType string

const (
	// IPv6 internet protocol version
	IPv6 IPFamilyType = "IPv6"
	// IPv4 internet protocol version
	IPv4 IPFamilyType = "IPv4"
)
