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
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Message",type=string,JSONPath=`.status.message`,description="Message"
// +kubebuilder:printcolumn:name="Health",type=string,JSONPath=`.status.ceph.health`,description="Ceph Health"
// +kubebuilder:printcolumn:name="External",type=boolean,JSONPath=`.spec.external.enable`
// +kubebuilder:printcolumn:name="FSID",type=string,JSONPath=`.status.ceph.fsid`,description="Ceph FSID"
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
	// LivenessProbe allows changing the livenessProbe configuration for a given daemon
	// +optional
	LivenessProbe map[KeyType]*ProbeSpec `json:"livenessProbe,omitempty"`
	// StartupProbe allows changing the startupProbe configuration for a given daemon
	// +optional
	StartupProbe map[KeyType]*ProbeSpec `json:"startupProbe,omitempty"`
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
	// Periodicity is the periodicity of the log rotation.
	// +kubebuilder:validation:Pattern=`^$|^(hourly|daily|weekly|monthly|1h|24h|1d)$`
	// +optional
	Periodicity string `json:"periodicity,omitempty"`
	// MaxLogSize is the maximum size of the log per ceph daemons. Must be at least 1M.
	// +optional
	MaxLogSize *resource.Quantity `json:"maxLogSize,omitempty"`
}

// SecuritySpec is security spec to include various security items such as kms
type SecuritySpec struct {
	// KeyManagementService is the main Key Management option
	// +optional
	// +nullable
	KeyManagementService KeyManagementServiceSpec `json:"kms,omitempty"`
	// KeyRotation defines options for Key Rotation.
	// +optional
	// +nullable
	KeyRotation KeyRotationSpec `json:"keyRotation,omitempty"`
}

// ObjectStoreSecuritySpec is spec to define security features like encryption
type ObjectStoreSecuritySpec struct {
	// +optional
	// +nullable
	SecuritySpec `json:""`

	// The settings for supporting AWS-SSE:S3 with RGW
	// +optional
	// +nullable
	ServerSideEncryptionS3 KeyManagementServiceSpec `json:"s3,omitempty"`
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

// KeyRotationSpec represents the settings for Key Rotation.
type KeyRotationSpec struct {
	// Enabled represents whether the key rotation is enabled.
	// +optional
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`
	// Schedule represents the cron schedule for key rotation.
	// +optional
	Schedule string `json:"schedule,omitempty"`
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

	// ImagePullPolicy describes a policy for if/when to pull a container image
	// One of Always, Never, IfNotPresent.
	// +kubebuilder:validation:Enum=IfNotPresent;Always;Never;""
	// +optional
	ImagePullPolicy v1.PullPolicy `json:"imagePullPolicy,omitempty"`
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
	// Endpoint for the Prometheus host
	// +optional
	PrometheusEndpoint string `json:"prometheusEndpoint,omitempty"`
	// Whether to verify the ssl endpoint for prometheus. Set to false for a self-signed cert.
	// +optional
	PrometheusEndpointSSLVerify bool `json:"prometheusEndpointSSLVerify,omitempty"`
}

// MonitoringSpec represents the settings for Prometheus based Ceph monitoring
type MonitoringSpec struct {
	// Enabled determines whether to create the prometheus rules for the ceph cluster. If true, the prometheus
	// types must exist or the creation will fail. Default is false.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// Whether to disable the metrics reported by Ceph. If false, the prometheus mgr module and Ceph exporter are enabled.
	// If true, the prometheus mgr module and Ceph exporter are both disabled. Default is false.
	// +optional
	MetricsDisabled bool `json:"metricsDisabled,omitempty"`

	// ExternalMgrEndpoints points to an existing Ceph prometheus exporter endpoint
	// +optional
	// +nullable
	ExternalMgrEndpoints []v1.EndpointAddress `json:"externalMgrEndpoints,omitempty"`

	// ExternalMgrPrometheusPort Prometheus exporter port
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=65535
	// +optional
	ExternalMgrPrometheusPort uint16 `json:"externalMgrPrometheusPort,omitempty"`

	// Port is the prometheus server port
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=65535
	// +optional
	Port int `json:"port,omitempty"`

	// Interval determines prometheus scrape interval
	// +optional
	Interval *metav1.Duration `json:"interval,omitempty"`
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
	// ObservedGeneration is the latest generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
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
	FSID     string               `json:"fsid,omitempty"`
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
	OSD           OSDStatus       `json:"osd,omitempty"`
}

// DeviceClasses represents device classes of a Ceph Cluster
type DeviceClasses struct {
	Name string `json:"name,omitempty"`
}

// OSDStatus represents OSD status of the ceph Cluster
type OSDStatus struct {
	// StoreType is a mapping between the OSD backend stores and number of OSDs using these stores
	StoreType map[string]int `json:"storeType,omitempty"`
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
	// ReconcileStarted represents when a resource reconciliation started.
	ReconcileStarted ConditionReason = "ReconcileStarted"

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
	// +optional
	FailureDomainLabel string `json:"failureDomainLabel,omitempty"`
	// Zones are specified when we want to provide zonal awareness to mons
	// +optional
	Zones []MonZoneSpec `json:"zones,omitempty"`
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
	Zones []MonZoneSpec `json:"zones,omitempty"`
}

// MonZoneSpec represents the specification of a zone in a Ceph Cluster
type MonZoneSpec struct {
	// Name is the name of the zone
	// +optional
	Name string `json:"name,omitempty"`
	// Arbiter determines if the zone contains the arbiter used for stretch cluster mode
	// +optional
	Arbiter bool `json:"arbiter,omitempty"`
	// VolumeClaimTemplate is the PVC template
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	VolumeClaimTemplate *v1.PersistentVolumeClaim `json:"volumeClaimTemplate,omitempty"`
}

// MgrSpec represents options to configure a ceph mgr
type MgrSpec struct {
	// Count is the number of manager daemons to run
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=5
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
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:subresource:status
type CephBlockPool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              NamedBlockPoolSpec `json:"spec"`
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

	// DEPRECATED: use Parameters instead, e.g., Parameters["compression_mode"] = "force"
	// The inline compression mode in Bluestore OSD to set to (options are: none, passive, aggressive, force)
	// +kubebuilder:validation:Enum=none;passive;aggressive;force;""
	// Do NOT set a default value for kubebuilder as this will override the Parameters
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

// NamedBlockPoolSpec allows a block pool to be created with a non-default name.
// This is more specific than the NamedPoolSpec so we get schema validation on the
// allowed pool names that can be specified.
type NamedBlockPoolSpec struct {
	// The desired name of the pool if different from the CephBlockPool CR name.
	// +kubebuilder:validation:Enum=device_health_metrics;.nfs;.mgr
	// +optional
	Name string `json:"name,omitempty"`
	// The core pool configuration
	PoolSpec `json:",inline"`
}

// NamedPoolSpec represents the named ceph pool spec
type NamedPoolSpec struct {
	// Name of the pool
	Name string `json:"name,omitempty"`
	// PoolSpec represents the spec of ceph pool
	PoolSpec `json:",inline"`
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
	// ObservedGeneration is the latest generation observed by the controller.
	// +optional
	ObservedGeneration int64       `json:"observedGeneration,omitempty"`
	Conditions         []Condition `json:"conditions,omitempty"`
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
	// ObservedGeneration is the latest generation observed by the controller.
	// +optional
	ObservedGeneration int64       `json:"observedGeneration,omitempty"`
	Conditions         []Condition `json:"conditions,omitempty"`
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
	// Number of coding chunks per object in an erasure coded storage pool (required for erasure-coded pool type).
	// This is the number of OSDs that can be lost simultaneously before data cannot be recovered.
	// +kubebuilder:validation:Minimum=0
	CodingChunks uint `json:"codingChunks"`

	// Number of data chunks per object in an erasure coded storage pool (required for erasure-coded pool type).
	// The number of chunks required to recover an object when any single OSD is lost is the same
	// as dataChunks so be aware that the larger the number of data chunks, the higher the cost of recovery.
	// +kubebuilder:validation:Minimum=0
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

	// The data pool settings, with optional predefined pool name.
	// +nullable
	DataPools []NamedPoolSpec `json:"dataPools"`

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
	Annotations Annotations `json:"annotations,omitempty"`

	// The labels-related configuration to add/set on each Pod related object.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Labels Labels `json:"labels,omitempty"`

	// The resource requirements for the rgw pods
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Resources v1.ResourceRequirements `json:"resources,omitempty"`

	// PriorityClassName sets priority classes on components
	// +optional
	PriorityClassName string `json:"priorityClassName,omitempty"`

	// +optional
	LivenessProbe *ProbeSpec `json:"livenessProbe,omitempty"`

	// +optional
	StartupProbe *ProbeSpec `json:"startupProbe,omitempty"`
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
	Conditions      []Condition                  `json:"conditions,omitempty"`
	// ObservedGeneration is the latest generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
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
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
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

	// The RGW health probes
	// +optional
	// +nullable
	HealthCheck ObjectHealthCheckSpec `json:"healthCheck,omitempty"`

	// Security represents security settings
	// +optional
	// +nullable
	Security *ObjectStoreSecuritySpec `json:"security,omitempty"`

	// The list of allowed namespaces in addition to the object store namespace
	// where ceph object store users may be created. Specify "*" to allow all
	// namespaces, otherwise list individual namespaces that are to be allowed.
	// This is useful for applications that need object store credentials
	// to be created in their own namespace, where neither OBCs nor COSI
	// is being used to create buckets. The default is empty.
	// +optional
	AllowUsersInNamespaces []string `json:"allowUsersInNamespaces,omitempty"`
}

// ObjectHealthCheckSpec represents the health check of an object store
type ObjectHealthCheckSpec struct {
	// livenessProbe field is no longer used
	// +kubebuilder:pruning:PreserveUnknownFields

	// +optional
	ReadinessProbe *ProbeSpec `json:"readinessProbe,omitempty"`
	// +optional
	StartupProbe *ProbeSpec `json:"startupProbe,omitempty"`
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

	// DisableMultisiteSyncTraffic, when true, prevents this object store's gateways from
	// transmitting multisite replication data. Note that this value does not affect whether
	// gateways receive multisite replication traffic: see ObjectZone.spec.customEndpoints for that.
	// If false or unset, this object store's gateways will be able to transmit multisite
	// replication data.
	// +optional
	DisableMultisiteSyncTraffic bool `json:"disableMultisiteSyncTraffic,omitempty"`

	// The annotations-related configuration to add/set on each Pod related object.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Annotations Annotations `json:"annotations,omitempty"`

	// The labels-related configuration to add/set on each Pod related object.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Labels Labels `json:"labels,omitempty"`

	// The resource requirements for the rgw pods
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Resources v1.ResourceRequirements `json:"resources,omitempty"`

	// PriorityClassName sets priority classes on the rgw pods
	// +optional
	PriorityClassName string `json:"priorityClassName,omitempty"`

	// ExternalRgwEndpoints points to external RGW endpoint(s). Multiple endpoints can be given, but
	// for stability of ObjectBucketClaims, we highly recommend that users give only a single
	// external RGW endpoint that is a load balancer that sends requests to the multiple RGWs.
	// +nullable
	// +optional
	ExternalRgwEndpoints []EndpointAddress `json:"externalRgwEndpoints,omitempty"`

	// The configuration related to add/set on each rgw service.
	// +optional
	// +nullable
	Service *RGWServiceSpec `json:"service,omitempty"`

	// Whether host networking is enabled for the rgw daemon. If not set, the network settings from the cluster CR will be applied.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	HostNetwork *bool `json:"hostNetwork,omitempty"`

	// Whether rgw dashboard is enabled for the rgw daemon. If not set, the rgw dashboard will be enabled.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	DashboardEnabled *bool `json:"dashboardEnabled,omitempty"`
}

// EndpointAddress is a tuple that describes a single IP address or host name. This is a subset of
// Kubernetes's v1.EndpointAddress.
// +structType=atomic
type EndpointAddress struct {
	// The IP of this endpoint. As a legacy behavior, this supports being given a DNS-adressable hostname as well.
	// +optional
	IP string `json:"ip" protobuf:"bytes,1,opt,name=ip"`

	// The DNS-addressable Hostname of this endpoint. This field will be preferred over IP if both are given.
	// +optional
	Hostname string `json:"hostname,omitempty" protobuf:"bytes,3,opt,name=hostname"`
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
	Endpoints ObjectEndpoints `json:"endpoints"`
	// +optional
	// +nullable
	Info       map[string]string `json:"info,omitempty"`
	Conditions []Condition       `json:"conditions,omitempty"`
	// ObservedGeneration is the latest generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

type ObjectEndpoints struct {
	// +optional
	// +nullable
	Insecure []string `json:"insecure"`
	// +optional
	// +nullable
	Secure []string `json:"secure"`
}

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CephObjectStoreUser represents a Ceph Object Store Gateway User
// +kubebuilder:resource:shortName=rcou;objectuser
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
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
	// ObservedGeneration is the latest generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
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
	// The namespace where the parent CephCluster and CephObjectStore are found
	// +optional
	ClusterNamespace string `json:"clusterNamespace,omitempty"`
}

// Additional admin-level capabilities for the Ceph object store user
type ObjectUserCapSpec struct {
	// +optional
	// +kubebuilder:validation:Enum={"*","read","write","read, write"}
	// Admin capabilities to read/write Ceph object store users. Documented in https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities
	User string `json:"user,omitempty"`
	// +optional
	// +kubebuilder:validation:Enum={"*","read","write","read, write"}
	// Admin capabilities to read/write Ceph object store users. Documented in https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities
	Users string `json:"users,omitempty"`
	// +optional
	// +kubebuilder:validation:Enum={"*","read","write","read, write"}
	// Admin capabilities to read/write Ceph object store buckets. Documented in https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities
	Bucket string `json:"bucket,omitempty"`
	// +optional
	// +kubebuilder:validation:Enum={"*","read","write","read, write"}
	// Admin capabilities to read/write Ceph object store buckets. Documented in https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities
	Buckets string `json:"buckets,omitempty"`
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
	// +optional
	// +kubebuilder:validation:Enum={"*","read","write","read, write"}
	// Admin capabilities to read/write roles for user. Documented in https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities
	Roles string `json:"roles,omitempty"`
	// +optional
	// +kubebuilder:validation:Enum={"*","read","write","read, write"}
	// Admin capabilities to read/write information about the user. Documented in https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities
	Info string `json:"info,omitempty"`
	// +optional
	// +kubebuilder:validation:Enum={"*","read","write","read, write"}
	// Add capabilities for user to send request to RGW Cache API header. Documented in https://docs.ceph.com/en/quincy/radosgw/rgw-cache/#cache-api
	AMZCache string `json:"amz-cache,omitempty"`
	// +optional
	// +kubebuilder:validation:Enum={"*","read","write","read, write"}
	// Add capabilities for user to change bucket index logging. Documented in https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities
	BiLog string `json:"bilog,omitempty"`
	// +optional
	// +kubebuilder:validation:Enum={"*","read","write","read, write"}
	// Add capabilities for user to change metadata logging. Documented in https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities
	MdLog string `json:"mdlog,omitempty"`
	// +optional
	// +kubebuilder:validation:Enum={"*","read","write","read, write"}
	// Add capabilities for user to change data logging. Documented in https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities
	DataLog string `json:"datalog,omitempty"`
	// +optional
	// +kubebuilder:validation:Enum={"*","read","write","read, write"}
	// Add capabilities for user to change user policies. Documented in https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities
	UserPolicy string `json:"user-policy,omitempty"`
	// +optional
	// +kubebuilder:validation:Enum={"*","read","write","read, write"}
	// Add capabilities for user to change oidc provider. Documented in https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities
	OidcProvider string `json:"oidc-provider,omitempty"`
	// +optional
	// +kubebuilder:validation:Enum={"*","read","write","read, write"}
	// Add capabilities for user to set rate limiter for user and bucket. Documented in https://docs.ceph.com/en/latest/radosgw/admin/?#add-remove-admin-capabilities
	RateLimit string `json:"ratelimit,omitempty"`
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

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CephObjectRealm represents a Ceph Object Store Gateway Realm
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
	Pull PullSpec `json:"pull,omitempty"`
}

// PullSpec represents the pulling specification of a Ceph Object Storage Gateway Realm
type PullSpec struct {
	// +kubebuilder:validation:Pattern=`^https*://`
	Endpoint string `json:"endpoint,omitempty"`
}

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CephObjectZoneGroup represents a Ceph Object Store Gateway Zone Group
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
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

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CephObjectZone represents a Ceph Object Store Gateway Zone
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
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

	// If this zone cannot be accessed from other peer Ceph clusters via the ClusterIP Service
	// endpoint created by Rook, you must set this to the externally reachable endpoint(s). You may
	// include the port in the definition. For example: "https://my-object-store.my-domain.net:443".
	// In many cases, you should set this to the endpoint of the ingress resource that makes the
	// CephObjectStore associated with this CephObjectStoreZone reachable to peer clusters.
	// The list can have one or more endpoints pointing to different RGW servers in the zone.
	//
	// If a CephObjectStore endpoint is omitted from this list, that object store's gateways will
	// not receive multisite replication data
	// (see CephObjectStore.spec.gateway.disableMultisiteSyncTraffic).
	// +nullable
	// +optional
	CustomEndpoints []string `json:"customEndpoints,omitempty"`

	// Preserve pools on object zone deletion
	// +optional
	// +kubebuilder:default=true
	PreservePoolsOnDelete bool `json:"preservePoolsOnDelete"`
}

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CephBucketTopic represents a Ceph Object Topic for Bucket Notifications
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:subresource:status
type CephBucketTopic struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              BucketTopicSpec `json:"spec"`
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Status *BucketTopicStatus `json:"status,omitempty"`
}

// BucketTopicStatus represents the Status of a CephBucketTopic
type BucketTopicStatus struct {
	// +optional
	Phase string `json:"phase,omitempty"`
	// The ARN of the topic generated by the RGW
	// +optional
	// +nullable
	ARN *string `json:"ARN,omitempty"`
	// ObservedGeneration is the latest generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// CephBucketTopicList represents a list Ceph Object Store Bucket Notification Topics
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type CephBucketTopicList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []CephBucketTopic `json:"items"`
}

// BucketTopicSpec represent the spec of a Bucket Topic
type BucketTopicSpec struct {
	// The name of the object store on which to define the topic
	// +kubebuilder:validation:MinLength=1
	ObjectStoreName string `json:"objectStoreName"`
	// The namespace of the object store on which to define the topic
	// +kubebuilder:validation:MinLength=1
	ObjectStoreNamespace string `json:"objectStoreNamespace"`
	// Data which is sent in each event
	// +optional
	OpaqueData string `json:"opaqueData,omitempty"`
	// Indication whether notifications to this endpoint are persistent or not
	// +optional
	Persistent bool `json:"persistent,omitempty"`
	// Contains the endpoint spec of the topic
	Endpoint TopicEndpointSpec `json:"endpoint"`
}

// TopicEndpointSpec contains exactly one of the endpoint specs of a Bucket Topic
type TopicEndpointSpec struct {
	// Spec of HTTP endpoint
	// +optional
	HTTP *HTTPEndpointSpec `json:"http,omitempty"`
	// Spec of AMQP endpoint
	// +optional
	AMQP *AMQPEndpointSpec `json:"amqp,omitempty"`
	// Spec of Kafka endpoint
	// +optional
	Kafka *KafkaEndpointSpec `json:"kafka,omitempty"`
}

// HTTPEndpointSpec represent the spec of an HTTP endpoint of a Bucket Topic
type HTTPEndpointSpec struct {
	// The URI of the HTTP endpoint to push notification to
	// +kubebuilder:validation:MinLength=1
	URI string `json:"uri"`
	// Indicate whether the server certificate is validated by the client or not
	// +optional
	DisableVerifySSL bool `json:"disableVerifySSL,omitempty"`
	// Send the notifications with the CloudEvents header: https://github.com/cloudevents/spec/blob/main/cloudevents/adapters/aws-s3.md
	// Supported for Ceph Quincy (v17) or newer.
	// +optional
	SendCloudEvents bool `json:"sendCloudEvents,omitempty"`
}

// AMQPEndpointSpec represent the spec of an AMQP endpoint of a Bucket Topic
type AMQPEndpointSpec struct {
	// The URI of the AMQP endpoint to push notification to
	// +kubebuilder:validation:MinLength=1
	URI string `json:"uri"`
	// Name of the exchange that is used to route messages based on topics
	// +kubebuilder:validation:MinLength=1
	Exchange string `json:"exchange"`
	// Indicate whether the server certificate is validated by the client or not
	// +optional
	DisableVerifySSL bool `json:"disableVerifySSL,omitempty"`
	// The ack level required for this topic (none/broker/routeable)
	// +kubebuilder:validation:Enum=none;broker;routeable
	// +kubebuilder:default=broker
	// +optional
	AckLevel string `json:"ackLevel,omitempty"`
}

// KafkaEndpointSpec represent the spec of a Kafka endpoint of a Bucket Topic
type KafkaEndpointSpec struct {
	// The URI of the Kafka endpoint to push notification to
	// +kubebuilder:validation:MinLength=1
	URI string `json:"uri"`
	// Indicate whether to use SSL when communicating with the broker
	// +optional
	UseSSL bool `json:"useSSL,omitempty"`
	// Indicate whether the server certificate is validated by the client or not
	// +optional
	DisableVerifySSL bool `json:"disableVerifySSL,omitempty"`
	// The ack level required for this topic (none/broker)
	// +kubebuilder:validation:Enum=none;broker
	// +kubebuilder:default=broker
	// +optional
	AckLevel string `json:"ackLevel,omitempty"`
}

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CephBucketNotification represents a Bucket Notifications
// +kubebuilder:subresource:status
type CephBucketNotification struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              BucketNotificationSpec `json:"spec"`
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Status *Status `json:"status,omitempty"`
}

// CephBucketNotificationList represents a list Ceph Object Store Bucket Notification Topics
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type CephBucketNotificationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []CephBucketNotification `json:"items"`
}

// BucketNotificationSpec represent the event type of the bucket notification
// +kubebuilder:validation:Enum="s3:ObjectCreated:*";"s3:ObjectCreated:Put";"s3:ObjectCreated:Post";"s3:ObjectCreated:Copy";"s3:ObjectCreated:CompleteMultipartUpload";"s3:ObjectRemoved:*";"s3:ObjectRemoved:Delete";"s3:ObjectRemoved:DeleteMarkerCreated"
type BucketNotificationEvent string

// BucketNotificationSpec represent the spec of a Bucket Notification
type BucketNotificationSpec struct {
	// The name of the topic associated with this notification
	// +kubebuilder:validation:MinLength=1
	Topic string `json:"topic"`
	// List of events that should trigger the notification
	// +optional
	Events []BucketNotificationEvent `json:"events,omitempty"`
	// Spec of notification filter
	// +optional
	Filter *NotificationFilterSpec `json:"filter,omitempty"`
}

// NotificationFilterRule represent a single rule in the Notification Filter spec
type NotificationFilterRule struct {
	// Name of the metadata or tag
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// Value to filter on
	Value string `json:"value"`
}

// NotificationKeyFilterRule represent a single key rule in the Notification Filter spec
type NotificationKeyFilterRule struct {
	// Name of the filter - prefix/suffix/regex
	// +kubebuilder:validation:Enum=prefix;suffix;regex
	Name string `json:"name"`
	// Value to filter on
	Value string `json:"value"`
}

// NotificationFilterSpec represent the spec of a Bucket Notification filter
type NotificationFilterSpec struct {
	// Filters based on the object's key
	// +optional
	KeyFilters []NotificationKeyFilterRule `json:"keyFilters,omitempty"`
	// Filters based on the object's metadata
	// +optional
	MetadataFilters []NotificationFilterRule `json:"metadataFilters,omitempty"`
	// Filters based on the object's tags
	// +optional
	TagFilters []NotificationFilterRule `json:"tagFilters,omitempty"`
}

// RGWServiceSpec represent the spec for RGW service
type RGWServiceSpec struct {
	// The annotations-related configuration to add/set on each rgw service.
	// nullable
	// optional
	Annotations Annotations `json:"annotations,omitempty"`
}

// +genclient
// +genclient:noStatus
// +kubebuilder:resource:shortName=nfs,path=cephnfses

// CephNFS represents a Ceph NFS
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
	// +nullable
	// +optional
	RADOS GaneshaRADOSSpec `json:"rados,omitempty"`

	// Server is the Ganesha Server specification
	Server GaneshaServerSpec `json:"server"`

	// Security allows specifying security configurations for the NFS cluster
	// +nullable
	// +optional
	Security *NFSSecuritySpec `json:"security"`
}

// GaneshaRADOSSpec represents the specification of a Ganesha RADOS object
type GaneshaRADOSSpec struct {
	// The Ceph pool used store the shared configuration for NFS-Ganesha daemons.
	// This setting is deprecated, as it is internally required to be ".nfs".
	// +optional
	Pool string `json:"pool,omitempty"`

	// The namespace inside the Ceph pool (set by 'pool') where shared NFS-Ganesha config is stored.
	// This setting is deprecated as it is internally set to the name of the CephNFS.
	// +optional
	Namespace string `json:"namespace,omitempty"`
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
	Annotations Annotations `json:"annotations,omitempty"`

	// The labels-related configuration to add/set on each Pod related object.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Labels Labels `json:"labels,omitempty"`

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

	// Whether host networking is enabled for the Ganesha server. If not set, the network settings from the cluster CR will be applied.
	// +nullable
	// +optional
	HostNetwork *bool `json:"hostNetwork,omitempty"`

	// A liveness-probe to verify that Ganesha server has valid run-time state.
	// If LivenessProbe.Disabled is false and LivenessProbe.Probe is nil uses default probe.
	// +optional
	LivenessProbe *ProbeSpec `json:"livenessProbe,omitempty"`
}

// NFSSecuritySpec represents security configurations for an NFS server pod
type NFSSecuritySpec struct {
	// SSSD enables integration with System Security Services Daemon (SSSD). SSSD can be used to
	// provide user ID mapping from a number of sources. See https://sssd.io for more information
	// about the SSSD project.
	// +optional
	// +nullable
	SSSD *SSSDSpec `json:"sssd,omitempty"`

	// Kerberos configures NFS-Ganesha to secure NFS client connections with Kerberos.
	// +optional
	// +nullable
	Kerberos *KerberosSpec `json:"kerberos,omitempty"`
}

// KerberosSpec represents configuration for Kerberos.
type KerberosSpec struct {
	// PrincipalName corresponds directly to NFS-Ganesha's NFS_KRB5:PrincipalName config. In
	// practice, this is the service prefix of the principal name. The default is "nfs".
	// This value is combined with (a) the namespace and name of the CephNFS (with a hyphen between)
	// and (b) the Realm configured in the user-provided krb5.conf to determine the full principal
	// name: <principalName>/<namespace>-<name>@<realm>. e.g., nfs/rook-ceph-my-nfs@example.net.
	// See https://github.com/nfs-ganesha/nfs-ganesha/wiki/RPCSEC_GSS for more detail.
	// +optional
	// +kubebuilder:default="nfs"
	PrincipalName string `json:"principalName"`

	// DomainName should be set to the Kerberos Realm.
	// +optional
	DomainName string `json:"domainName"`

	// ConfigFiles defines where the Kerberos configuration should be sourced from. Config files
	// will be placed into the `/etc/krb5.conf.rook/` directory.
	//
	// If this is left empty, Rook will not add any files. This allows you to manage the files
	// yourself however you wish. For example, you may build them into your custom Ceph container
	// image or use the Vault agent injector to securely add the files via annotations on the
	// CephNFS spec (passed to the NFS server pods).
	//
	// Rook configures Kerberos to log to stderr. We suggest removing logging sections from config
	// files to avoid consuming unnecessary disk space from logging to files.
	// +optional
	ConfigFiles KerberosConfigFiles `json:"configFiles"`

	// KeytabFile defines where the Kerberos keytab should be sourced from. The keytab file will be
	// placed into `/etc/krb5.keytab`. If this is left empty, Rook will not add the file.
	// This allows you to manage the `krb5.keytab` file yourself however you wish. For example, you
	// may build it into your custom Ceph container image or use the Vault agent injector to
	// securely add the file via annotations on the CephNFS spec (passed to the NFS server pods).
	// +optional
	KeytabFile KerberosKeytabFile `json:"keytabFile"`
}

// KerberosConfigFiles represents the source(s) from which Kerberos configuration should come.
type KerberosConfigFiles struct {
	// VolumeSource accepts a pared down version of the standard Kubernetes VolumeSource for
	// Kerberos configuration files like what is normally used to configure Volumes for a Pod. For
	// example, a ConfigMap, Secret, or HostPath. The volume may contain multiple files, all of
	// which will be loaded.
	VolumeSource *ConfigFileVolumeSource `json:"volumeSource,omitempty"`
}

// KerberosKeytabFile represents the source(s) from which the Kerberos keytab file should come.
type KerberosKeytabFile struct {
	// VolumeSource accepts a pared down version of the standard Kubernetes VolumeSource for the
	// Kerberos keytab file like what is normally used to configure Volumes for a Pod. For example,
	// a Secret or HostPath.
	// There are two requirements for the source's content:
	//   1. The config file must be mountable via `subPath: krb5.keytab`. For example, in a
	//      Secret, the data item must be named `krb5.keytab`, or `items` must be defined to
	//      select the key and give it path `krb5.keytab`. A HostPath directory must have the
	//      `krb5.keytab` file.
	//   2. The volume or config file must have mode 0600.
	VolumeSource *ConfigFileVolumeSource `json:"volumeSource,omitempty"`
}

// SSSDSpec represents configuration for System Security Services Daemon (SSSD).
type SSSDSpec struct {
	// Sidecar tells Rook to run SSSD in a sidecar alongside the NFS-Ganesha server in each NFS pod.
	// +optional
	Sidecar *SSSDSidecar `json:"sidecar,omitempty"`
}

// SSSDSidecar represents configuration when SSSD is run in a sidecar.
type SSSDSidecar struct {
	// Image defines the container image that should be used for the SSSD sidecar.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`

	// SSSDConfigFile defines where the SSSD configuration should be sourced from. The config file
	// will be placed into `/etc/sssd/sssd.conf`. If this is left empty, Rook will not add the file.
	// This allows you to manage the `sssd.conf` file yourself however you wish. For example, you
	// may build it into your custom Ceph container image or use the Vault agent injector to
	// securely add the file via annotations on the CephNFS spec (passed to the NFS server pods).
	// +optional
	SSSDConfigFile SSSDSidecarConfigFile `json:"sssdConfigFile"`

	// AdditionalFiles defines any number of additional files that should be mounted into the SSSD
	// sidecar. These files may be referenced by the sssd.conf config file.
	// +optional
	AdditionalFiles []SSSDSidecarAdditionalFile `json:"additionalFiles,omitempty"`

	// Resources allow specifying resource requests/limits on the SSSD sidecar container.
	// +optional
	Resources v1.ResourceRequirements `json:"resources,omitempty"`

	// DebugLevel sets the debug level for SSSD. If unset or set to 0, Rook does nothing. Otherwise,
	// this may be a value between 1 and 10. See SSSD docs for more info:
	// https://sssd.io/troubleshooting/basics.html#sssd-debug-logs
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=10
	DebugLevel int `json:"debugLevel,omitempty"`
}

// SSSDSidecarConfigFile represents the source(s) from which the SSSD configuration should come.
type SSSDSidecarConfigFile struct {
	// VolumeSource accepts a pared down version of the standard Kubernetes VolumeSource for the
	// SSSD configuration file like what is normally used to configure Volumes for a Pod. For
	// example, a ConfigMap, Secret, or HostPath. There are two requirements for the source's
	// content:
	//   1. The config file must be mountable via `subPath: sssd.conf`. For example, in a ConfigMap,
	//      the data item must be named `sssd.conf`, or `items` must be defined to select the key
	//      and give it path `sssd.conf`. A HostPath directory must have the `sssd.conf` file.
	//   2. The volume or config file must have mode 0600.
	VolumeSource *ConfigFileVolumeSource `json:"volumeSource,omitempty"`
}

// SSSDSidecarAdditionalFile represents the source from where additional files for the the SSSD
// configuration should come from and are made available.
type SSSDSidecarAdditionalFile struct {
	// SubPath defines the sub-path in `/etc/sssd/rook-additional/` where the additional file(s)
	// will be placed. Each subPath definition must be unique and must not contain ':'.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^[^:]+$`
	SubPath string `json:"subPath"`

	// VolumeSource accepts a pared down version of the standard Kubernetes VolumeSource for the
	// additional file(s) like what is normally used to configure Volumes for a Pod. Fore example, a
	// ConfigMap, Secret, or HostPath. Each VolumeSource adds one or more additional files to the
	// SSSD sidecar container in the `/etc/sssd/rook-additional/<subPath>` directory.
	// Be aware that some files may need to have a specific file mode like 0600 due to requirements
	// by SSSD for some files. For example, CA or TLS certificates.
	VolumeSource *ConfigFileVolumeSource `json:"volumeSource"`
}

// NetworkSpec for Ceph includes backward compatibility code
type NetworkSpec struct {
	// Provider is what provides network connectivity to the cluster e.g. "host" or "multus"
	// +nullable
	// +optional
	Provider NetworkProviderType `json:"provider,omitempty"`

	// Selectors define NetworkAttachmentDefinitions to be used for Ceph public and/or cluster
	// networks when the "multus" network provider is used. This config section is not used for
	// other network providers.
	//
	// Valid keys are "public" and "cluster". Refer to Ceph networking documentation for more:
	// https://docs.ceph.com/en/reef/rados/configuration/network-config-ref/
	//
	// Refer to Multus network annotation documentation for help selecting values:
	// https://github.com/k8snetworkplumbingwg/multus-cni/blob/master/docs/how-to-use.md#run-pod-with-network-annotation
	//
	// Rook will make a best-effort attempt to automatically detect CIDR address ranges for given
	// network attachment definitions. Rook's methods are robust but may be imprecise for
	// sufficiently complicated networks. Rook's auto-detection process obtains a new IP address
	// lease for each CephCluster reconcile. If Rook fails to detect, incorrectly detects, only
	// partially detects, or if underlying networks do not support reusing old IP addresses, it is
	// best to use the 'addressRanges' config section to specify CIDR ranges for the Ceph cluster.
	//
	// As a contrived example, one can use a theoretical Kubernetes-wide network for Ceph client
	// traffic and a theoretical Rook-only network for Ceph replication traffic as shown:
	//   selectors:
	//     public: "default/cluster-fast-net"
	//     cluster: "rook-ceph/ceph-backend-net"
	//
	// +nullable
	// +optional
	Selectors map[CephNetworkType]string `json:"selectors,omitempty"`

	// AddressRanges specify a list of CIDRs that Rook will apply to Ceph's 'public_network' and/or
	// 'cluster_network' configurations. This config section may be used for the "host" or "multus"
	// network providers.
	// +nullable
	// +optional
	AddressRanges *AddressRangesSpec `json:"addressRanges,omitempty"`

	// Settings for network connections such as compression and encryption across the
	// wire.
	// +nullable
	// +optional
	Connections *ConnectionsSpec `json:"connections,omitempty"`

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

	// Enable multiClusterService to export the Services between peer clusters
	// +optional
	MultiClusterService MultiClusterServiceSpec `json:"multiClusterService,omitempty"`
}

// NetworkProviderType defines valid network providers for Rook.
// +kubebuilder:validation:Enum="";host;multus
type NetworkProviderType string

const (
	NetworkProviderDefault = NetworkProviderType("")
	NetworkProviderHost    = NetworkProviderType("host")
	NetworkProviderMultus  = NetworkProviderType("multus")
)

// CephNetworkType should be "public" or "cluster".
// Allow any string so that over-specified legacy clusters do not break on CRD update.
type CephNetworkType string

const (
	CephNetworkPublic  = CephNetworkType("public")
	CephNetworkCluster = CephNetworkType("cluster")
)

type AddressRangesSpec struct {
	// Public defines a list of CIDRs to use for Ceph public network communication.
	// +optional
	Public CIDRList `json:"public"`

	// Cluster defines a list of CIDRs to use for Ceph cluster network communication.
	// +optional
	Cluster CIDRList `json:"cluster"`
}

// An IPv4 or IPv6 network CIDR.
//
// This naive kubebuilder regex provides immediate feedback for some typos and for a common problem
// case where the range spec is forgotten (e.g., /24). Rook does in-depth validation in code.
// +kubebuilder:validation:Pattern=`^[0-9a-fA-F:.]{2,}\/[0-9]{1,3}$`
type CIDR string

// A list of CIDRs.
type CIDRList []CIDR

type MultiClusterServiceSpec struct {
	// Enable multiClusterService to export the mon and OSD services to peer cluster.
	// Ensure that peer clusters are connected using an MCS API compatible application,
	// like Globalnet Submariner.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// ClusterID uniquely identifies a cluster. It is used as a prefix to nslookup exported
	// services. For example: <clusterid>.<svc>.<ns>.svc.clusterset.local
	ClusterID string `json:"clusterID,omitempty"`
}
type ConnectionsSpec struct {
	// Encryption settings for the network connections.
	// +nullable
	// +optional
	Encryption *EncryptionSpec `json:"encryption,omitempty"`

	// Compression settings for the network connections.
	// +nullable
	// +optional
	Compression *CompressionSpec `json:"compression,omitempty"`

	// Whether to require msgr2 (port 3300) even if compression or encryption are not enabled.
	// If true, the msgr1 port (6789) will be disabled.
	// Requires a kernel that supports msgr2 (kernel 5.11 or CentOS 8.4 or newer).
	// +optional
	RequireMsgr2 bool `json:"requireMsgr2,omitempty"`
}

type EncryptionSpec struct {
	// Whether to encrypt the data in transit across the wire to prevent eavesdropping
	// the data on the network. The default is not set. Even if encryption is not enabled,
	// clients still establish a strong initial authentication for the connection
	// and data integrity is still validated with a crc check. When encryption is enabled,
	// all communication between clients and Ceph daemons, or between Ceph daemons will
	// be encrypted.
	// +optional
	Enabled bool `json:"enabled,omitempty"`
}

type CompressionSpec struct {
	// Whether to compress the data in transit across the wire.
	// The default is not set. Requires Ceph Quincy (v17) or newer.
	// +optional
	Enabled bool `json:"enabled,omitempty"`
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

	// Deprecated. This enables management of machinedisruptionbudgets.
	// +optional
	ManageMachineDisruptionBudgets bool `json:"manageMachineDisruptionBudgets,omitempty"`

	// Deprecated. Namespace to look for MDBs by the machineDisruptionBudgetController
	// +optional
	MachineDisruptionBudgetNamespace string `json:"machineDisruptionBudgetNamespace,omitempty"`
}

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CephClient represents a Ceph Client
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
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
	// ObservedGeneration is the latest generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
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
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
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
	Annotations Annotations `json:"annotations,omitempty"`

	// The labels-related configuration to add/set on each Pod related object.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +nullable
	// +optional
	Labels Labels `json:"labels,omitempty"`

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
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
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
	Annotations Annotations `json:"annotations,omitempty"`

	// The labels-related configuration to add/set on each Pod related object.
	// +nullable
	// +optional
	Labels Labels `json:"labels,omitempty"`

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
	// +optional
	Store OSDStore `json:"store,omitempty"`
	// +optional
	// FlappingRestartIntervalHours defines the time for which the OSD pods, that failed with zero exit code, will sleep before restarting.
	// This is needed for OSD flapping where OSD daemons are marked down more than 5 times in 600 seconds by Ceph.
	// Preventing the OSD pods to restart immediately in such scenarios will prevent Rook from marking OSD as `up` and thus
	// peering of the PGs mapped to the OSD.
	// User needs to manually restart the OSD pod if they manage to fix the underlying OSD flapping issue before the restart interval.
	// The sleep will be disabled if this interval is set to 0.
	FlappingRestartIntervalHours int `json:"flappingRestartIntervalHours"`
}

// OSDStore is the backend storage type used for creating the OSDs
type OSDStore struct {
	// Type of backend storage to be used while creating OSDs. If empty, then bluestore will be used
	// +optional
	// +kubebuilder:validation:Enum=bluestore;bluestore-rdr;
	Type string `json:"type,omitempty"`
	// UpdateStore updates the backend store for existing OSDs. It destroys each OSD one at a time, cleans up the backing disk
	// and prepares same OSD on that disk
	// +optional
	// +kubebuilder:validation:Pattern=`^$|^yes-really-update-store$`
	UpdateStore string `json:"updateStore,omitempty"`
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
type PlacementSpec map[KeyType]Placement

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
type PriorityClassNamesSpec map[KeyType]string

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

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CephFilesystemSubVolumeGroup represents a Ceph Filesystem SubVolumeGroup
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:subresource:status
type CephFilesystemSubVolumeGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	// Spec represents the specification of a Ceph Filesystem SubVolumeGroup
	Spec CephFilesystemSubVolumeGroupSpec `json:"spec"`
	// Status represents the status of a CephFilesystem SubvolumeGroup
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Status *CephFilesystemSubVolumeGroupStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CephFilesystemSubVolumeGroup represents a list of Ceph Clients
type CephFilesystemSubVolumeGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []CephFilesystemSubVolumeGroup `json:"items"`
}

// CephFilesystemSubVolumeGroupSpec represents the specification of a Ceph Filesystem SubVolumeGroup
type CephFilesystemSubVolumeGroupSpec struct {
	// FilesystemName is the name of Ceph Filesystem SubVolumeGroup volume name. Typically it's the name of
	// the CephFilesystem CR. If not coming from the CephFilesystem CR, it can be retrieved from the
	// list of Ceph Filesystem volumes with `ceph fs volume ls`. To learn more about Ceph Filesystem
	// abstractions see https://docs.ceph.com/en/latest/cephfs/fs-volumes/#fs-volumes-and-subvolumes
	FilesystemName string `json:"filesystemName"`
}

// CephFilesystemSubVolumeGroupStatus represents the Status of Ceph Filesystem SubVolumeGroup
type CephFilesystemSubVolumeGroupStatus struct {
	// +optional
	Phase ConditionType `json:"phase,omitempty"`
	// +optional
	// +nullable
	Info map[string]string `json:"info,omitempty"`
	// ObservedGeneration is the latest generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CephBlockPoolRadosNamespace represents a Ceph BlockPool Rados Namespace
// +kubebuilder:subresource:status
type CephBlockPoolRadosNamespace struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	// Spec represents the specification of a Ceph BlockPool Rados Namespace
	Spec CephBlockPoolRadosNamespaceSpec `json:"spec"`
	// Status represents the status of a CephBlockPool Rados Namespace
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Status *CephBlockPoolRadosNamespaceStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CephBlockPoolRadosNamespaceList represents a list of Ceph BlockPool Rados Namespace
type CephBlockPoolRadosNamespaceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []CephBlockPoolRadosNamespace `json:"items"`
}

// CephBlockPoolRadosNamespaceSpec represents the specification of a CephBlockPool Rados Namespace
type CephBlockPoolRadosNamespaceSpec struct {
	// BlockPoolName is the name of Ceph BlockPool. Typically it's the name of
	// the CephBlockPool CR.
	BlockPoolName string `json:"blockPoolName"`
}

// CephBlockPoolRadosNamespaceStatus represents the Status of Ceph BlockPool
// Rados Namespace
type CephBlockPoolRadosNamespaceStatus struct {
	// +optional
	Phase ConditionType `json:"phase,omitempty"`
	// +optional
	// +nullable
	Info map[string]string `json:"info,omitempty"`
}

// Represents the source of a volume to mount.
// Only one of its members may be specified.
// This is a subset of the full Kubernetes API's VolumeSource that is reduced to what is most likely
// to be useful for mounting config files/dirs into Rook pods.
type ConfigFileVolumeSource struct {
	// hostPath represents a pre-existing file or directory on the host
	// machine that is directly exposed to the container. This is generally
	// used for system agents or other privileged things that are allowed
	// to see the host machine. Most containers will NOT need this.
	// More info: https://kubernetes.io/docs/concepts/storage/volumes#hostpath
	// ---
	// +optional
	HostPath *v1.HostPathVolumeSource `json:"hostPath,omitempty" protobuf:"bytes,1,opt,name=hostPath"`
	// emptyDir represents a temporary directory that shares a pod's lifetime.
	// More info: https://kubernetes.io/docs/concepts/storage/volumes#emptydir
	// +optional
	EmptyDir *v1.EmptyDirVolumeSource `json:"emptyDir,omitempty" protobuf:"bytes,2,opt,name=emptyDir"`
	// secret represents a secret that should populate this volume.
	// More info: https://kubernetes.io/docs/concepts/storage/volumes#secret
	// +optional
	Secret *v1.SecretVolumeSource `json:"secret,omitempty" protobuf:"bytes,6,opt,name=secret"`
	// persistentVolumeClaimVolumeSource represents a reference to a
	// PersistentVolumeClaim in the same namespace.
	// More info: https://kubernetes.io/docs/concepts/storage/persistent-volumes#persistentvolumeclaims
	// +optional
	PersistentVolumeClaim *v1.PersistentVolumeClaimVolumeSource `json:"persistentVolumeClaim,omitempty" protobuf:"bytes,10,opt,name=persistentVolumeClaim"`
	// configMap represents a configMap that should populate this volume
	// +optional
	ConfigMap *v1.ConfigMapVolumeSource `json:"configMap,omitempty" protobuf:"bytes,19,opt,name=configMap"`
	// projected items for all in one resources secrets, configmaps, and downward API
	Projected *v1.ProjectedVolumeSource `json:"projected,omitempty" protobuf:"bytes,26,opt,name=projected"`
}

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CephCOSIDriver represents the CRD for the Ceph COSI Driver Deployment
// +kubebuilder:resource:shortName=cephcosi
type CephCOSIDriver struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	// Spec represents the specification of a Ceph COSI Driver
	Spec CephCOSIDriverSpec `json:"spec"`
}

// CephCOSIDriverList represents a list of Ceph COSI Driver
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type CephCOSIDriverList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []CephCOSIDriver `json:"items"`
}

// CephCOSIDriverSpec represents the specification of a Ceph COSI Driver
type CephCOSIDriverSpec struct {
	// Image is the container image to run the Ceph COSI driver
	// +optional
	Image string `json:"image,omitempty"`
	// ObjectProvisionerImage is the container image to run the COSI driver sidecar
	// +optional
	ObjectProvisionerImage string `json:"objectProvisionerImage,omitempty"`
	// DeploymentStrategy is the strategy to use to deploy the COSI driver.
	// +optional
	// +kubebuilder:validation:Enum=Never;Auto;Always
	DeploymentStrategy COSIDeploymentStrategy `json:"deploymentStrategy,omitempty"`
	// Placement is the placement strategy to use for the COSI driver
	// +optional
	Placement Placement `json:"placement,omitempty"`
	// Resources is the resource requirements for the COSI driver
	// +optional
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
}

// COSIDeploymentStrategy represents the strategy to use to deploy the Ceph COSI driver
type COSIDeploymentStrategy string

const (
	// Never means the Ceph COSI driver will never deployed
	COSIDeploymentStrategyNever COSIDeploymentStrategy = "Never"
	// Auto means the Ceph COSI driver will be deployed automatically if object store is present
	COSIDeploymentStrategyAuto COSIDeploymentStrategy = "Auto"
	// Always means the Ceph COSI driver will be deployed even if the object store is not present
	COSIDeploymentStrategyAlways COSIDeploymentStrategy = "Always"
)
