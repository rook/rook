/*
Copyright 2018 The Rook Authors. All rights reserved.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rook "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
)

// ***************************************************************************
// IMPORTANT FOR CODE GENERATION
// If the types in this file are updated, you will need to run
// `make codegen` to generate the new types under the client/clientset folder.
// ***************************************************************************

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type CephCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              ClusterSpec   `json:"spec"`
	Status            ClusterStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type CephClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []CephCluster `json:"items"`
}

type ClusterSpec struct {
	// The version information that instructs Rook to orchestrate a particular version of Ceph.
	CephVersion CephVersionSpec `json:"cephVersion,omitempty"`

	// A spec for available storage in the cluster and how it should be used
	Storage rook.StorageScopeSpec `json:"storage,omitempty"`

	// The annotations-related configuration to add/set on each Pod related object.
	Annotations rook.AnnotationsSpec `json:"annotations,omitempty"`

	// The placement-related configuration to pass to kubernetes (affinity, node selector, tolerations).
	Placement rook.PlacementSpec `json:"placement,omitempty"`

	// Network related configuration
	Network NetworkSpec `json:"network,omitempty"`

	// Resources set resource requests and limits
	Resources rook.ResourceSpec `json:"resources,omitempty"`

	// PriorityClassNames sets priority classes on components
	PriorityClassNames rook.PriorityClassNamesSpec `json:"priorityClassNames,omitempty"`

	// The path on the host where config and data can be persisted.
	DataDirHostPath string `json:"dataDirHostPath,omitempty"`

	// SkipUpgradeChecks defines if an upgrade should be forced even if one of the check fails
	SkipUpgradeChecks bool `json:"skipUpgradeChecks,omitempty"`

	// ContinueUpgradeAfterChecksEvenIfNotHealthy defines if an upgrade should continue even if PGs are not clean
	ContinueUpgradeAfterChecksEvenIfNotHealthy bool `json:"continueUpgradeAfterChecksEvenIfNotHealthy,omitempty"`

	// A spec for configuring disruption management.
	DisruptionManagement DisruptionManagementSpec `json:"disruptionManagement,omitempty"`

	// A spec for mon related options
	Mon MonSpec `json:"mon,omitempty"`

	// A spec for rbd mirroring
	RBDMirroring RBDMirroringSpec `json:"rbdMirroring"`

	// A spec for the crash controller
	CrashCollector CrashCollectorSpec `json:"crashCollector"`

	// Dashboard settings
	Dashboard DashboardSpec `json:"dashboard,omitempty"`

	// Prometheus based Monitoring settings
	Monitoring MonitoringSpec `json:"monitoring,omitempty"`

	// Whether the Ceph Cluster is running external to this Kubernetes cluster
	// mon, mgr, osd, mds, and discover daemons will not be created for external clusters.
	External ExternalSpec `json:"external"`

	// A spec for mgr related options
	Mgr MgrSpec `json:"mgr,omitempty"`

	// Remove the OSD that is out and safe to remove only if this option is true
	RemoveOSDsIfOutAndSafeToRemove bool `json:"removeOSDsIfOutAndSafeToRemove"`
}

// VersionSpec represents the settings for the Ceph version that Rook is orchestrating.
type CephVersionSpec struct {
	// Image is the container image used to launch the ceph daemons, such as ceph/ceph:v13.2.6 or ceph/ceph:v14.2.2
	Image string `json:"image,omitempty"`

	// Whether to allow unsupported versions (do not set to true in production)
	AllowUnsupported bool `json:"allowUnsupported,omitempty"`
}

// DashboardSpec represents the settings for the Ceph dashboard
type DashboardSpec struct {
	// Whether to enable the dashboard
	Enabled bool `json:"enabled,omitempty"`
	// A prefix for all URLs to use the dashboard with a reverse proxy
	UrlPrefix string `json:"urlPrefix,omitempty"`
	// The dashboard webserver port
	Port int `json:"port,omitempty"`
	// Whether SSL should be used
	SSL bool `json:"ssl,omitempty"`
}

// MonitoringSpec represents the settings for Prometheus based Ceph monitoring
type MonitoringSpec struct {
	// Whether to create the prometheus rules for the ceph cluster. If true, the prometheus
	// types must exist or the creation will fail.
	Enabled bool `json:"enabled,omitempty"`

	// The namespace where the prometheus rules and alerts should be created.
	// If empty, the same namespace as the cluster will be used.
	RulesNamespace string `json:"rulesNamespace,omitempty"`
}

type ClusterStatus struct {
	State      ClusterState `json:"state,omitempty"`
	Message    string       `json:"message,omitempty"`
	CephStatus *CephStatus  `json:"ceph,omitempty"`
}

type CephStatus struct {
	Health         string                       `json:"health,omitempty"`
	Details        map[string]CephHealthMessage `json:"details,omitempty"`
	LastChecked    string                       `json:"lastChecked,omitempty"`
	LastChanged    string                       `json:"lastChanged,omitempty"`
	PreviousHealth string                       `json:"previousHealth,omitempty"`
}

type CephHealthMessage struct {
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type ClusterState string

const (
	ClusterStateCreating   ClusterState = "Creating"
	ClusterStateCreated    ClusterState = "Created"
	ClusterStateUpdating   ClusterState = "Updating"
	ClusterStateConnecting ClusterState = "Connecting"
	ClusterStateConnected  ClusterState = "Connected"
	ClusterStateError      ClusterState = "Error"
	// DefaultFailureDomain for PoolSpec
	DefaultFailureDomain = "host"
)

type MonSpec struct {
	Count                int                       `json:"count,omitempty"`
	AllowMultiplePerNode bool                      `json:"allowMultiplePerNode,omitempty"`
	VolumeClaimTemplate  *v1.PersistentVolumeClaim `json:"volumeClaimTemplate,omitempty"`
}

// MgrSpec represents options to configure a ceph mgr
type MgrSpec struct {
	Modules []Module `json:"modules,omitempty"`
}

// Module represents mgr modules that the user wants to enable or disable
type Module struct {
	Name    string `json:"name,omitempty"`
	Enabled bool   `json:"enabled"`
}

// ExternalSpec represents the options supported by an external cluster
type ExternalSpec struct {
	Enable bool `json:"enable"`
}

type RBDMirroringSpec struct {
	Workers int `json:"workers"`
}

// CrashCollectorSpec represents options to configure the crash controller
type CrashCollectorSpec struct {
	Disable bool `json:"disable"`
}

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type CephBlockPool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              PoolSpec `json:"spec"`
	Status            *Status  `json:"status"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type CephBlockPoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []CephBlockPool `json:"items"`
}

// PoolSpec represents the spec of ceph pool
type PoolSpec struct {
	// The failure domain: osd/host/(region or zone if available) - technically also any type in the crush map
	FailureDomain string `json:"failureDomain"`

	// The root of the crush hierarchy utilized by the pool
	CrushRoot string `json:"crushRoot"`

	// The device class the OSD should set to (options are: hdd, ssd, or nvme)
	DeviceClass string `json:"deviceClass"`

	// The replication settings
	Replicated ReplicatedSpec `json:"replicated"`

	// The erasure code settings
	ErasureCoded ErasureCodedSpec `json:"erasureCoded"`
}

type Status struct {
	Phase string `json:"phase,omitempty"`
}

// ReplicationSpec represents the spec for replication in a pool
type ReplicatedSpec struct {
	// Number of copies per object in a replicated storage pool, including the object itself (required for replicated pool type)
	Size uint `json:"size"`
}

// ErasureCodeSpec represents the spec for erasure code in a pool
type ErasureCodedSpec struct {
	// Number of coding chunks per object in an erasure coded storage pool (required for erasure-coded pool type)
	CodingChunks uint `json:"codingChunks"`

	// Number of data chunks per object in an erasure coded storage pool (required for erasure-coded pool type)
	DataChunks uint `json:"dataChunks"`

	// The algorithm for erasure coding
	Algorithm string `json:"algorithm"`
}

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type CephFilesystem struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              FilesystemSpec `json:"spec"`
	Status            *Status        `json:"status"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type CephFilesystemList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []CephFilesystem `json:"items"`
}

// FilesystemSpec represents the spec of a file system
type FilesystemSpec struct {
	// The metadata pool settings
	MetadataPool PoolSpec `json:"metadataPool,omitempty"`

	// The data pool settings
	DataPools []PoolSpec `json:"dataPools,omitempty"`

	// Preserve pools on filesystem deletion
	PreservePoolsOnDelete bool `json:"preservePoolsOnDelete"`

	// The mds pod info
	MetadataServer MetadataServerSpec `json:"metadataServer"`
}

type MetadataServerSpec struct {
	// The number of metadata servers that are active. The remaining servers in the cluster will be in standby mode.
	ActiveCount int32 `json:"activeCount"`

	// Whether each active MDS instance will have an active standby with a warm metadata cache for faster failover.
	// If false, standbys will still be available, but will not have a warm metadata cache.
	ActiveStandby bool `json:"activeStandby"`

	// The affinity to place the mds pods (default is to place on all available node) with a daemonset
	Placement rook.Placement `json:"placement"`

	// The annotations-related configuration to add/set on each Pod related object.
	Annotations rook.Annotations `json:"annotations,omitempty"`

	// The resource requirements for the rgw pods
	Resources v1.ResourceRequirements `json:"resources"`

	// PriorityClassName sets priority classes on components
	PriorityClassName string `json:"priorityClassName,omitempty"`
}

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type CephObjectStore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              ObjectStoreSpec `json:"spec"`
	Status            *Status         `json:"status"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type CephObjectStoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []CephObjectStore `json:"items"`
}

// ObjectStoreSpec represent the spec of a pool
type ObjectStoreSpec struct {
	// The metadata pool settings
	MetadataPool PoolSpec `json:"metadataPool"`

	// The data pool settings
	DataPool PoolSpec `json:"dataPool"`

	// Preserve pools on object store deletion
	PreservePoolsOnDelete bool `json:"preservePoolsOnDelete"`

	// The rgw pod info
	Gateway GatewaySpec `json:"gateway"`
}

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type CephObjectStoreUser struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              ObjectStoreUserSpec `json:"spec"`
	Status            *Status             `json:"status"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type CephObjectStoreUserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []CephObjectStoreUser `json:"items"`
}

// ObjectStoreUserSpec represent the spec of an Objectstoreuser
type ObjectStoreUserSpec struct {
	//The store the user will be created in
	Store string `json:"store,omitempty"`
	//The display name for the ceph users
	DisplayName string `json:"displayName,omitempty"`
}

type GatewaySpec struct {
	// The port the rgw service will be listening on (http)
	Port int32 `json:"port"`

	// The port the rgw service will be listening on (https)
	SecurePort int32 `json:"securePort"`

	// The number of pods in the rgw replicaset. If "allNodes" is specified, a daemonset is created.
	Instances int32 `json:"instances"`

	// Whether the rgw pods should be started as a daemonset on all nodes
	AllNodes bool `json:"allNodes"`

	// The name of the secret that stores the ssl certificate for secure rgw connections
	SSLCertificateRef string `json:"sslCertificateRef"`

	// The affinity to place the rgw pods (default is to place on any available node)
	Placement rook.Placement `json:"placement"`

	// The annotations-related configuration to add/set on each Pod related object.
	Annotations rook.Annotations `json:"annotations,omitempty"`

	// The resource requirements for the rgw pods
	Resources v1.ResourceRequirements `json:"resources"`

	// PriorityClassName sets priority classes on the rgw pods
	PriorityClassName string `json:"priorityClassName,omitempty"`
}

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type CephNFS struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              NFSGaneshaSpec `json:"spec"`
	Status            *Status        `json:"status"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type CephNFSList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []CephNFS `json:"items"`
}

// NFSGaneshaSpec represents the spec of an nfs ganesha server
type NFSGaneshaSpec struct {
	RADOS GaneshaRADOSSpec `json:"rados"`

	Server GaneshaServerSpec `json:"server"`
}

type GaneshaRADOSSpec struct {
	// Pool is the RADOS pool where NFS client recovery data is stored.
	Pool string `json:"pool"`

	// Namespace is the RADOS namespace where NFS client recovery data is stored.
	Namespace string `json:"namespace"`
}

type GaneshaServerSpec struct {
	// The number of active Ganesha servers
	Active int `json:"active"`

	// The affinity to place the ganesha pods
	Placement rook.Placement `json:"placement"`

	// The annotations-related configuration to add/set on each Pod related object.
	Annotations rook.Annotations `json:"annotations,omitempty"`

	// Resources set resource requests and limits
	Resources v1.ResourceRequirements `json:"resources,omitempty"`

	// PriorityClassName sets the priority class on the pods
	PriorityClassName string `json:"priorityClassName,omitempty"`
}

// NetworkSpec for Ceph includes backward compatibility code
type NetworkSpec struct {
	rook.NetworkSpec `json:",inline"`

	// HostNetwork to enable host network
	HostNetwork bool `json:"hostNetwork"`
}

// DisruptionManagementSpec configures management of daemon disruptions
type DisruptionManagementSpec struct {

	// This enables management of poddisruptionbudgets
	ManagePodBudgets bool `json:"managePodBudgets,omitempty"`

	// OSDMaintenanceTimeout sets how many additional minutes the DOWN/OUT interval is for drained failure domains
	// it only works if managePodBudgetss is true.
	// the default is 30 minutes
	OSDMaintenanceTimeout time.Duration `json:"osdMaintenanceTimeout,omitempty"`

	// This enables management of machinedisruptionbudgets
	ManageMachineDisruptionBudgets bool `json:"manageMachineDisruptionBudgets,omitempty"`

	// Namespace to look for MDBs by the machineDisruptionBudgetController
	MachineDisruptionBudgetNamespace string `json:"machineDisruptionBudgetNamespace,omitempty"`
}

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type CephClient struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              ClientSpec `json:"spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type CephClientList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []CephClient `json:"items"`
}

type ClientSpec struct {
	Name string            `json:"name"`
	Caps map[string]string `json:"caps"`
}
