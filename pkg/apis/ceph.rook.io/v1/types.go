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

	rook "github.com/rook/rook/pkg/apis/rook.io"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
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
// +kubebuilder:printcolumn:name="DataDirHostPath",type=string,JSONPath=`.spec.dataDirHostPath`,description="Directory used on the K8s nodes"
// +kubebuilder:printcolumn:name="MonCount",type=string,JSONPath=`.spec.mon.count`,description="Number of MONs"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`,description="Phase"
// +kubebuilder:printcolumn:name="Message",type=string,JSONPath=`.status.message`,description="Message"
// +kubebuilder:printcolumn:name="Health",type=string,JSONPath=`.status.ceph.health`,description="Ceph Health"
// +kubebuilder:printcolumn:name="External",type=boolean,JSONPath=`.spec.external.enable`
// +kubebuilder:subresource:status
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
	LivenessProbe map[rook.KeyType]*ProbeSpec `json:"livenessProbe,omitempty"`
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
	Storage StorageScopeSpec `json:"storage,omitempty"`

	// The annotations-related configuration to add/set on each Pod related object.
	// +nullable
	// +optional
	Annotations AnnotationsSpec `json:"annotations,omitempty"`

	// The labels-related configuration to add/set on each Pod related object.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Labels LabelsSpec `json:"labels,omitempty"`

	// The placement-related configuration to pass to kubernetes (affinity, node selector, tolerations).
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Placement PlacementSpec `json:"placement,omitempty"`

	// Network related configuration
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Network NetworkSpec `json:"network,omitempty"`

	// Resources set resource requests and limits
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Resources ResourceSpec `json:"resources,omitempty"`

	// PriorityClassNames sets priority classes on components
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	PriorityClassNames PriorityClassNamesSpec `json:"priorityClassNames,omitempty"`

	// The path on the host where config and data can be persisted
	// +kubebuilder:validation:Pattern=`^/(\S+)`
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
	// Image is the container image used to launch the ceph daemons, such as quay.io/ceph/ceph:<tag>
	// The full list of images can be found at https://quay.io/repository/ceph/ceph?tab=tags
	// +optional
	Image string `json:"image,omitempty"`

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
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=65535
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
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=65535
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

// CephDaemonsVersions show the current ceph version for different ceph daemons
type CephDaemonsVersions struct {
	// Mon shows Mon Ceph version
	// +optional
	Mon map[string]int `json:"mon,omitempty"`
	// Mgr shows Mgr Ceph version
	// +optional
	Mgr map[string]int `json:"mgr,omitempty"`
	// Osd shows Osd Ceph version
	// +optional
	Osd map[string]int `json:"osd,omitempty"`
	// Rgw shows Rgw Ceph version
	// +optional
	Rgw map[string]int `json:"rgw,omitempty"`
	// Mds shows Mds Ceph version
	// +optional
	Mds map[string]int `json:"mds,omitempty"`
	// RbdMirror shows RbdMirror Ceph version
	// +optional
	RbdMirror map[string]int `json:"rbd-mirror,omitempty"`
	// CephFSMirror shows CephFSMirror Ceph version
	// +optional
	CephFSMirror map[string]int `json:"cephfs-mirror,omitempty"`
	// Overall shows overall Ceph version
	// +optional
	Overall map[string]int `json:"overall,omitempty"`
}

// CephStatus is the details health of a Ceph Cluster
type CephStatus struct {
	Health         string                       `json:"health,omitempty"`
	Details        map[string]CephHealthMessage `json:"details,omitempty"`
	LastChecked    string                       `json:"lastChecked,omitempty"`
	LastChanged    string                       `json:"lastChanged,omitempty"`
	PreviousHealth string                       `json:"previousHealth,omitempty"`
	Capacity       Capacity                     `json:"capacity,omitempty"`
	// +optional
	Versions *CephDaemonsVersions `json:"versions,omitempty"`
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

// Condition represents a status condition on any Rook-Ceph Custom Resource.
type Condition struct {
	Type               ConditionType      `json:"type,omitempty"`
	Status             v1.ConditionStatus `json:"status,omitempty"`
	Reason             ConditionReason    `json:"reason,omitempty"`
	Message            string             `json:"message,omitempty"`
	LastHeartbeatTime  metav1.Time        `json:"lastHeartbeatTime,omitempty"`
	LastTransitionTime metav1.Time        `json:"lastTransitionTime,omitempty"`
}

// ConditionReason is a reason for a condition
type ConditionReason string

const (
	// ClusterCreatedReason is cluster created reason
	ClusterCreatedReason ConditionReason = "ClusterCreated"
	// ClusterConnectedReason is cluster connected reason
	ClusterConnectedReason ConditionReason = "ClusterConnected"
	// ClusterProgressingReason is cluster progressing reason
	ClusterProgressingReason ConditionReason = "ClusterProgressing"
	// ClusterDeletingReason is cluster deleting reason
	ClusterDeletingReason ConditionReason = "ClusterDeleting"
	// ClusterConnectingReason is cluster connecting reason
	ClusterConnectingReason ConditionReason = "ClusterConnecting"

	// ReconcileSucceeded represents when a resource reconciliation was successful.
	ReconcileSucceeded ConditionReason = "ReconcileSucceeded"
	// ReconcileFailed represents when a resource reconciliation failed.
	ReconcileFailed ConditionReason = "ReconcileFailed"

	// DeletingReason represents when Rook has detected a resource object should be deleted.
	DeletingReason ConditionReason = "Deleting"
	// ObjectHasDependentsReason represents when a resource object has dependents that are blocking
	// deletion.
	ObjectHasDependentsReason ConditionReason = "ObjectHasDependents"
	// ObjectHasNoDependentsReason represents when a resource object has no dependents that are
	// blocking deletion.
	ObjectHasNoDependentsReason ConditionReason = "ObjectHasNoDependents"
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

	// ConditionDeletionIsBlocked represents when deletion of the object is blocked.
	ConditionDeletionIsBlocked ConditionType = "DeletionIsBlocked"
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
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=9
	// +optional
	Count int `json:"count,omitempty"`
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
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=2
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
// +kubebuilder:subresource:status
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
	// +nullable
	CrushRoot string `json:"crushRoot,omitempty"`

	// The device class the OSD should set to for use in the pool
	// +optional
	// +nullable
	DeviceClass string `json:"deviceClass,omitempty"`

	// The inline compression mode in Bluestore OSD to set to (options are: none, passive, aggressive, force)
	// +kubebuilder:validation:Enum=none;passive;aggressive;force;""
	// +kubebuilder:default=none
	// +optional
	// +nullable
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
	// +kubebuilder:validation:Minimum=0
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

	// HybridStorage represents hybrid storage tier settings
	// +optional
	// +nullable
	HybridStorage *HybridStorageSpec `json:"hybridStorage,omitempty"`
}

// HybridStorageSpec represents the settings for hybrid storage pool
type HybridStorageSpec struct {
	// PrimaryDeviceClass represents high performance tier (for example SSD or NVME) for Primary OSD
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Required
	// +required
	PrimaryDeviceClass string `json:"primaryDeviceClass"`
	// SecondaryDeviceClass represents low performance tier (for example HDDs) for remaining OSDs
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Required
	// +required
	SecondaryDeviceClass string `json:"secondaryDeviceClass"`
}

// MirroringSpec represents the setting for a mirrored pool
type MirroringSpec struct {
	// Enabled whether this pool is mirrored or not
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// Mode is the mirroring mode: either pool or image
	// +optional
	Mode string `json:"mode,omitempty"`

	// SnapshotSchedules is the scheduling of snapshot for mirrored images/pools
	// +optional
	SnapshotSchedules []SnapshotScheduleSpec `json:"snapshotSchedules,omitempty"`

	// Peers represents the peers spec
	// +nullable
	// +optional
	Peers *MirroringPeerSpec `json:"peers,omitempty"`
}

// SnapshotScheduleSpec represents the snapshot scheduling settings of a mirrored pool
type SnapshotScheduleSpec struct {
	// Path is the path to snapshot, only valid for CephFS
	// +optional
	Path string `json:"path,omitempty"`

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
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=9
	CodingChunks uint `json:"codingChunks"`

	// Number of data chunks per object in an erasure coded storage pool (required for erasure-coded pool type)
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=9
	DataChunks uint `json:"dataChunks"`

	// The algorithm for erasure coding
	// +optional
	Algorithm string `json:"algorithm,omitempty"`
}

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CephFilesystem represents a Ceph Filesystem
// +kubebuilder:printcolumn:name="ActiveMDS",type=string,JSONPath=`.spec.metadataServer.activeCount`,description="Number of desired active MDS daemons"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:subresource:status
type CephFilesystem struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              FilesystemSpec `json:"spec"`
	// +kubebuilder:pruning:PreserveUnknownFields
	Status *CephFilesystemStatus `json:"status,omitempty"`
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
	// +nullable
	MetadataPool PoolSpec `json:"metadataPool"`

	// The data pool settings
	// +nullable
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
	Mirroring *FSMirroringSpec `json:"mirroring,omitempty"`

	// The mirroring statusCheck
	// +kubebuilder:pruning:PreserveUnknownFields
	StatusCheck MirrorHealthCheckSpec `json:"statusCheck,omitempty"`
}

// MetadataServerSpec represents the specification of a Ceph Metadata Server
type MetadataServerSpec struct {
	// The number of metadata servers that are active. The remaining servers in the cluster will be in standby mode.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10
	ActiveCount int32 `json:"activeCount"`

	// Whether each active MDS instance will have an active standby with a warm metadata cache for faster failover.
	// If false, standbys will still be available, but will not have a warm metadata cache.
	// +optional
	ActiveStandby bool `json:"activeStandby,omitempty"`

	// The affinity to place the mds pods (default is to place on all available node) with a daemonset
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Placement Placement `json:"placement,omitempty"`

	// The annotations-related configuration to add/set on each Pod related object.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Annotations rook.Annotations `json:"annotations,omitempty"`

	// The labels-related configuration to add/set on each Pod related object.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Labels rook.Labels `json:"labels,omitempty"`

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

	// Peers represents the peers spec
	// +nullable
	// +optional
	Peers *MirroringPeerSpec `json:"peers,omitempty"`

	// SnapshotSchedules is the scheduling of snapshot for mirrored filesystems
	// +optional
	SnapshotSchedules []SnapshotScheduleSpec `json:"snapshotSchedules,omitempty"`

	// Retention is the retention policy for a snapshot schedule
	// One path has exactly one retention policy.
	// A policy can however contain multiple count-time period pairs in order to specify complex retention policies
	// +optional
	SnapshotRetention []SnapshotScheduleRetentionSpec `json:"snapshotRetention,omitempty"`
}

// SnapshotScheduleRetentionSpec is a retention policy
type SnapshotScheduleRetentionSpec struct {
	// Path is the path to snapshot
	// +optional
	Path string `json:"path,omitempty"`

	// Duration represents the retention duration for a snapshot
	// +optional
	Duration string `json:"duration,omitempty"`
}

// CephFilesystemStatus represents the status of a Ceph Filesystem
type CephFilesystemStatus struct {
	// +optional
	Phase ConditionType `json:"phase,omitempty"`
	// +optional
	SnapshotScheduleStatus *FilesystemSnapshotScheduleStatusSpec `json:"snapshotScheduleStatus,omitempty"`
	// Use only info and put mirroringStatus in it?
	// +optional
	// +nullable
	Info map[string]string `json:"info,omitempty"`
	// MirroringStatus is the filesystem mirroring status
	// +optional
	MirroringStatus *FilesystemMirroringInfoSpec `json:"mirroringStatus,omitempty"`
}

// FilesystemMirroringInfo is the status of the pool mirroring
type FilesystemMirroringInfoSpec struct {
	// PoolMirroringStatus is the mirroring status of a filesystem
	// +nullable
	// +optional
	FilesystemMirroringAllInfo []FilesystemMirroringInfo `json:"daemonsStatus,omitempty"`
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

// FilesystemSnapshotScheduleStatusSpec is the status of the snapshot schedule
type FilesystemSnapshotScheduleStatusSpec struct {
	// SnapshotSchedules is the list of snapshots scheduled
	// +nullable
	// +optional
	SnapshotSchedules []FilesystemSnapshotSchedulesSpec `json:"snapshotSchedules,omitempty"`
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

// FilesystemSnapshotSchedulesSpec is the list of snapshot scheduled for images in a pool
type FilesystemSnapshotSchedulesSpec struct {
	// Fs is the name of the Ceph Filesystem
	// +optional
	Fs string `json:"fs,omitempty"`
	// Subvol is the name of the sub volume
	// +optional
	Subvol string `json:"subvol,omitempty"`
	// Path is the path on the filesystem
	// +optional
	Path string `json:"path,omitempty"`
	// +optional
	RelPath string `json:"rel_path,omitempty"`
	// +optional
	Schedule string `json:"schedule,omitempty"`
	// +optional
	Retention FilesystemSnapshotScheduleStatusRetention `json:"retention,omitempty"`
}

// FilesystemSnapshotScheduleStatusRetention is the retention specification for a filesystem snapshot schedule
type FilesystemSnapshotScheduleStatusRetention struct {
	// Start is when the snapshot schedule starts
	// +optional
	Start string `json:"start,omitempty"`
	// Created is when the snapshot schedule was created
	// +optional
	Created string `json:"created,omitempty"`
	// First is when the first snapshot schedule was taken
	// +optional
	First string `json:"first,omitempty"`
	// Last is when the last snapshot schedule was taken
	// +optional
	Last string `json:"last,omitempty"`
	// LastPruned is when the last snapshot schedule was pruned
	// +optional
	LastPruned string `json:"last_pruned,omitempty"`
	// CreatedCount is total amount of snapshots
	// +optional
	CreatedCount int `json:"created_count,omitempty"`
	// PrunedCount is total amount of pruned snapshots
	// +optional
	PrunedCount int `json:"pruned_count,omitempty"`
	// Active is whether the scheduled is active or not
	// +optional
	Active bool `json:"active,omitempty"`
}

// FilesystemMirrorInfoSpec is the filesystem mirror status of a given filesystem
type FilesystemMirroringInfo struct {
	// DaemonID is the cephfs-mirror name
	// +optional
	DaemonID int `json:"daemon_id,omitempty"`
	// Filesystems is the list of filesystems managed by a given cephfs-mirror daemon
	// +optional
	Filesystems []FilesystemsSpec `json:"filesystems,omitempty"`
}

// FilesystemsSpec is spec for the mirrored filesystem
type FilesystemsSpec struct {
	// FilesystemID is the filesystem identifier
	// +optional
	FilesystemID int `json:"filesystem_id,omitempty"`
	// Name is name of the filesystem
	// +optional
	Name string `json:"name,omitempty"`
	// DirectoryCount is the number of directories in the filesystem
	// +optional
	DirectoryCount int `json:"directory_count,omitempty"`
	// Peers represents the mirroring peers
	// +optional
	Peers []FilesystemMirrorInfoPeerSpec `json:"peers,omitempty"`
}

// FilesystemMirrorInfoPeerSpec is the specification of a filesystem peer mirror
type FilesystemMirrorInfoPeerSpec struct {
	// UUID is the peer unique identifier
	// +optional
	UUID string `json:"uuid,omitempty"`
	// Remote are the remote cluster information
	// +optional
	Remote *PeerRemoteSpec `json:"remote,omitempty"`
	// Stats are the stat a peer mirror
	// +optional
	Stats *PeerStatSpec `json:"stats,omitempty"`
}

type PeerRemoteSpec struct {
	// ClientName is cephx name
	// +optional
	ClientName string `json:"client_name,omitempty"`
	// ClusterName is the name of the cluster
	// +optional
	ClusterName string `json:"cluster_name,omitempty"`
	// FsName is the filesystem name
	// +optional
	FsName string `json:"fs_name,omitempty"`
}

// PeerStatSpec are the mirror stat with a given peer
type PeerStatSpec struct {
	// FailureCount is the number of mirroring failure
	// +optional
	FailureCount int `json:"failure_count,omitempty"`
	// RecoveryCount is the number of recovery attempted after failures
	// +optional
	RecoveryCount int `json:"recovery_count,omitempty"`
}

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CephObjectStore represents a Ceph Object Store Gateway
// +kubebuilder:subresource:status
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
	// +nullable
	MetadataPool PoolSpec `json:"metadataPool,omitempty"`

	// The data pool settings
	// +optional
	// +nullable
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

	// Security represents security settings
	// +optional
	// +nullable
	Security *SecuritySpec `json:"security,omitempty"`
}

// BucketHealthCheckSpec represents the health check of an object store
type BucketHealthCheckSpec struct {
	// +optional
	Bucket HealthCheckSpec `json:"bucket,omitempty"`
	// +optional
	LivenessProbe *ProbeSpec `json:"livenessProbe,omitempty"`
}

// HealthCheckSpec represents the health check of an object store bucket
type HealthCheckSpec struct {
	// +optional
	Disabled bool `json:"disabled,omitempty"`
	// Interval is the internal in second or minute for the health check to run like 60s for 60 seconds
	// +optional
	Interval *metav1.Duration `json:"interval,omitempty"`
	// +optional
	Timeout string `json:"timeout,omitempty"`
}

// GatewaySpec represents the specification of Ceph Object Store Gateway
type GatewaySpec struct {
	// The port the rgw service will be listening on (http)
	// +optional
	Port int32 `json:"port,omitempty"`

	// The port the rgw service will be listening on (https)
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=65535
	// +nullable
	// +optional
	SecurePort int32 `json:"securePort,omitempty"`

	// The number of pods in the rgw replicaset.
	// +nullable
	// +optional
	Instances int32 `json:"instances,omitempty"`

	// The name of the secret that stores the ssl certificate for secure rgw connections
	// +nullable
	// +optional
	SSLCertificateRef string `json:"sslCertificateRef,omitempty"`

	// The name of the secret that stores custom ca-bundle with root and intermediate certificates.
	// +nullable
	// +optional
	CaBundleRef string `json:"caBundleRef,omitempty"`

	// The affinity to place the rgw pods (default is to place on any available node)
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Placement Placement `json:"placement,omitempty"`

	// The annotations-related configuration to add/set on each Pod related object.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Annotations rook.Annotations `json:"annotations,omitempty"`

	// The labels-related configuration to add/set on each Pod related object.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Labels rook.Labels `json:"labels,omitempty"`

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

	// The configuration related to add/set on each rgw service.
	// +optional
	// +nullable
	Service *RGWServiceSpec `json:"service,omitempty"`
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
	Info       map[string]string `json:"info,omitempty"`
	Conditions []Condition       `json:"conditions,omitempty"`
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

// CephObjectStoreUser represents a Ceph Object Store Gateway User
// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:shortName=rcou;objectuser
// +kubebuilder:subresource:status
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
	// The store the user will be created in
	// +optional
	Store string `json:"store,omitempty"`
	// The display name for the ceph users
	// +optional
	DisplayName string `json:"displayName,omitempty"`
	// +optional
	// +nullable
	Capabilities *ObjectUserCapSpec `json:"capabilities,omitempty"`
	// +optional
	// +nullable
	Quotas *ObjectUserQuotaSpec `json:"quotas,omitempty"`
}

// Additional admin-level capabilities for the Ceph object store user
type ObjectUserCapSpec struct {
	// +optional
	// +kubebuilder:validation:Enum={"*","read","write","read, write"}
	// Admin capabilities to read/write Ceph object store users. Documented in https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities
	User string `json:"user,omitempty"`
	// +optional
	// +kubebuilder:validation:Enum={"*","read","write","read, write"}
	// Admin capabilities to read/write Ceph object store buckets. Documented in https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities
	Bucket string `json:"bucket,omitempty"`
	// +optional
	// +kubebuilder:validation:Enum={"*","read","write","read, write"}
	// Admin capabilities to read/write Ceph object store metadata. Documented in https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities
	MetaData string `json:"metadata,omitempty"`
	// +optional
	// +kubebuilder:validation:Enum={"*","read","write","read, write"}
	// Admin capabilities to read/write Ceph object store usage. Documented in https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities
	Usage string `json:"usage,omitempty"`
	// +optional
	// +kubebuilder:validation:Enum={"*","read","write","read, write"}
	// Admin capabilities to read/write Ceph object store zones. Documented in https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities
	Zone string `json:"zone,omitempty"`
}

// ObjectUserQuotaSpec can be used to set quotas for the object store user to limit their usage. See the [Ceph docs](https://docs.ceph.com/en/latest/radosgw/admin/?#quota-management) for more
type ObjectUserQuotaSpec struct {
	// Maximum bucket limit for the ceph user
	// +optional
	// +nullable
	MaxBuckets *int `json:"maxBuckets,omitempty"`
	// Maximum size limit of all objects across all the user's buckets
	// See https://pkg.go.dev/k8s.io/apimachinery/pkg/api/resource#Quantity for more info.
	// +optional
	// +nullable
	MaxSize *resource.Quantity `json:"maxSize,omitempty"`
	// Maximum number of objects across all the user's buckets
	// +optional
	// +nullable
	MaxObjects *int64 `json:"maxObjects,omitempty"`
}

// CephObjectRealm represents a Ceph Object Store Gateway Realm
// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:subresource:status
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
// +kubebuilder:subresource:status
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
// +kubebuilder:subresource:status
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
	// +nullable
	MetadataPool PoolSpec `json:"metadataPool"`

	// The data pool settings
	// +nullable
	DataPool PoolSpec `json:"dataPool"`
}

// RGWServiceSpec represent the spec for RGW service
type RGWServiceSpec struct {
	// The annotations-related configuration to add/set on each rgw service.
	// nullable
	// optional
	Annotations rook.Annotations `json:"annotations,omitempty"`
}

// CephNFS represents a Ceph NFS
// +genclient
// +genclient:noStatus
// +kubebuilder:resource:shortName=nfs,path=cephnfses
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:subresource:status
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
	Placement Placement `json:"placement,omitempty"`

	// The annotations-related configuration to add/set on each Pod related object.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Annotations rook.Annotations `json:"annotations,omitempty"`

	// The labels-related configuration to add/set on each Pod related object.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Labels rook.Labels `json:"labels,omitempty"`

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
	// Provider is what provides network connectivity to the cluster e.g. "host" or "multus"
	// +nullable
	// +optional
	Provider string `json:"provider,omitempty"`

	// Selectors string values describe what networks will be used to connect the cluster.
	// Meanwhile the keys describe each network respective responsibilities or any metadata
	// storage provider decide.
	// +nullable
	// +optional
	Selectors map[string]string `json:"selectors,omitempty"`

	// HostNetwork to enable host network
	// +optional
	HostNetwork bool `json:"hostNetwork,omitempty"`

	// IPFamily is the single stack IPv6 or IPv4 protocol
	// +kubebuilder:validation:Enum=IPv4;IPv6
	// +nullable
	// +optional
	IPFamily IPFamilyType `json:"ipFamily,omitempty"`

	// DualStack determines whether Ceph daemons should listen on both IPv4 and IPv6
	// +optional
	DualStack bool `json:"dualStack,omitempty"`
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
// +kubebuilder:subresource:status
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
// +kubebuilder:validation:Pattern=`^$|^yes-really-destroy-data$`
type CleanupConfirmationProperty string

// SanitizeDataSourceProperty represents a sanitizing data source
type SanitizeDataSourceProperty string

// SanitizeMethodProperty represents a disk sanitizing method
type SanitizeMethodProperty string

// SanitizeDisksSpec represents a disk sanitizing specification
type SanitizeDisksSpec struct {
	// Method is the method we use to sanitize disks
	// +optional
	// +kubebuilder:validation:Enum=complete;quick
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
// +kubebuilder:subresource:status
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

	// Peers represents the peers spec
	// +nullable
	// +optional
	Peers MirroringPeerSpec `json:"peers,omitempty"`

	// The affinity to place the rgw pods (default is to place on any available node)
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Placement Placement `json:"placement,omitempty"`

	// The annotations-related configuration to add/set on each Pod related object.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Annotations rook.Annotations `json:"annotations,omitempty"`

	// The labels-related configuration to add/set on each Pod related object.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Labels rook.Labels `json:"labels,omitempty"`

	// The resource requirements for the rbd mirror pods
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Resources v1.ResourceRequirements `json:"resources,omitempty"`

	// PriorityClassName sets priority class on the rbd mirror pods
	// +optional
	PriorityClassName string `json:"priorityClassName,omitempty"`
}

// MirroringPeerSpec represents the specification of a mirror peer
type MirroringPeerSpec struct {
	// SecretNames represents the Kubernetes Secret names to add rbd-mirror or cephfs-mirror peers
	// +optional
	SecretNames []string `json:"secretNames,omitempty"`
}

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CephFilesystemMirror is the Ceph Filesystem Mirror object definition
// +kubebuilder:subresource:status
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

// FilesystemMirroringSpec is the filesystem mirroring specification
type FilesystemMirroringSpec struct {
	// The affinity to place the rgw pods (default is to place on any available node)
	// +nullable
	// +optional
	Placement Placement `json:"placement,omitempty"`

	// The annotations-related configuration to add/set on each Pod related object.
	// +nullable
	// +optional
	Annotations rook.Annotations `json:"annotations,omitempty"`

	// The labels-related configuration to add/set on each Pod related object.
	// +nullable
	// +optional
	Labels rook.Labels `json:"labels,omitempty"`

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

type StorageScopeSpec struct {
	// +nullable
	// +optional
	Nodes []Node `json:"nodes,omitempty"`
	// +optional
	UseAllNodes bool `json:"useAllNodes,omitempty"`
	// +optional
	OnlyApplyOSDPlacement bool `json:"onlyApplyOSDPlacement,omitempty"`
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Config    map[string]string `json:"config,omitempty"`
	Selection `json:",inline"`
	// +nullable
	// +optional
	StorageClassDeviceSets []StorageClassDeviceSet `json:"storageClassDeviceSets,omitempty"`
}

// Node is a storage nodes
// +nullable
type Node struct {
	// +optional
	Name string `json:"name,omitempty"`
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Config    map[string]string `json:"config,omitempty"`
	Selection `json:",inline"`
}

// Device represents a disk to use in the cluster
type Device struct {
	// +optional
	Name string `json:"name,omitempty"`
	// +optional
	FullPath string `json:"fullpath,omitempty"`
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Config map[string]string `json:"config,omitempty"`
}

type Selection struct {
	// Whether to consume all the storage devices found on a machine
	// +optional
	UseAllDevices *bool `json:"useAllDevices,omitempty"`
	// A regular expression to allow more fine-grained selection of devices on nodes across the cluster
	// +optional
	DeviceFilter string `json:"deviceFilter,omitempty"`
	// A regular expression to allow more fine-grained selection of devices with path names
	// +optional
	DevicePathFilter string `json:"devicePathFilter,omitempty"`
	// List of devices to use as storage devices
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Devices []Device `json:"devices,omitempty"`
	// PersistentVolumeClaims to use as storage
	// +optional
	VolumeClaimTemplates []v1.PersistentVolumeClaim `json:"volumeClaimTemplates,omitempty"`
}

// PlacementSpec is the placement for core ceph daemons part of the CephCluster CRD
type PlacementSpec map[rook.KeyType]Placement

// Placement is the placement for an object
type Placement struct {
	// NodeAffinity is a group of node affinity scheduling rules
	// +optional
	NodeAffinity *v1.NodeAffinity `json:"nodeAffinity,omitempty"`
	// PodAffinity is a group of inter pod affinity scheduling rules
	// +optional
	PodAffinity *v1.PodAffinity `json:"podAffinity,omitempty"`
	// PodAntiAffinity is a group of inter pod anti affinity scheduling rules
	// +optional
	PodAntiAffinity *v1.PodAntiAffinity `json:"podAntiAffinity,omitempty"`
	// The pod this Toleration is attached to tolerates any taint that matches
	// the triple <key,value,effect> using the matching operator <operator>
	// +optional
	Tolerations []v1.Toleration `json:"tolerations,omitempty"`
	// TopologySpreadConstraint specifies how to spread matching pods among the given topology
	// +optional
	TopologySpreadConstraints []v1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`
}

// ResourceSpec is a collection of ResourceRequirements that describes the compute resource requirements
type ResourceSpec map[string]v1.ResourceRequirements

// ProbeSpec is a wrapper around Probe so it can be enabled or disabled for a Ceph daemon
type ProbeSpec struct {
	// Disabled determines whether probe is disable or not
	// +optional
	Disabled bool `json:"disabled,omitempty"`
	// Probe describes a health check to be performed against a container to determine whether it is
	// alive or ready to receive traffic.
	// +optional
	Probe *v1.Probe `json:"probe,omitempty"`
}

// PriorityClassNamesSpec is a map of priority class names to be assigned to components
type PriorityClassNamesSpec map[rook.KeyType]string

// StorageClassDeviceSet is a storage class device set
// +nullable
type StorageClassDeviceSet struct {
	// Name is a unique identifier for the set
	Name string `json:"name"`
	// Count is the number of devices in this set
	// +kubebuilder:validation:Minimum=1
	Count int `json:"count"`
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Resources v1.ResourceRequirements `json:"resources,omitempty"` // Requests/limits for the devices
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Placement Placement `json:"placement,omitempty"` // Placement constraints for the device daemons
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	PreparePlacement *Placement `json:"preparePlacement,omitempty"` // Placement constraints for the device preparation
	// Provider-specific device configuration
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Config map[string]string `json:"config,omitempty"`
	// VolumeClaimTemplates is a list of PVC templates for the underlying storage devices
	VolumeClaimTemplates []v1.PersistentVolumeClaim `json:"volumeClaimTemplates"`
	// Portable represents OSD portability across the hosts
	// +optional
	Portable bool `json:"portable,omitempty"`
	// TuneSlowDeviceClass Tune the OSD when running on a slow Device Class
	// +optional
	TuneSlowDeviceClass bool `json:"tuneDeviceClass,omitempty"`
	// TuneFastDeviceClass Tune the OSD when running on a fast Device Class
	// +optional
	TuneFastDeviceClass bool `json:"tuneFastDeviceClass,omitempty"`
	// Scheduler name for OSD pod placement
	// +optional
	SchedulerName string `json:"schedulerName,omitempty"`
	// Whether to encrypt the deviceSet
	// +optional
	Encrypted bool `json:"encrypted,omitempty"`
}
