/*
Copyright 2017 The Rook Authors. All rights reserved.

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
package v1alpha1

import (
	"k8s.io/api/core/v1"
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

type Cluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              ClusterSpec   `json:"spec"`
	Status            ClusterStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []Cluster `json:"items"`
}

type ClusterSpec struct {
	// The type of backend for the cluster (only "ceph" is implemented)
	Backend string `json:"backend"`

	// The path on the host where config and data can be persisted.
	DataDirHostPath string `json:"dataDirHostPath"`

	// The placement-related configuration to pass to kubernetes (affinity, node selector, tolerations).
	Placement PlacementSpec `json:"placement,omitempty"`

	// A spec for available storage in the cluster and how it should be used
	Storage StorageSpec `json:"storage"`

	// HostNetwork to enable host network
	HostNetwork bool `json:"hostNetwork"`

	// MonCount sets the mon size
	MonCount int `json:"monCount"`

	// Resources set resource requests and limits
	Resources ResourceSpec `json:"resources,omitempty"`
}

type ClusterStatus struct {
	State   ClusterState `json:"state,omitempty"`
	Message string       `json:"message,omitempty"`
}

type ClusterState string

const (
	ClusterStateCreating ClusterState = "Creating"
	ClusterStateCreated  ClusterState = "Created"
	ClusterStateUpdating ClusterState = "Updating"
	ClusterStateError    ClusterState = "Error"
)

type ResourceSpec struct {
	API v1.ResourceRequirements `json:"api,omitempty"`
	Mgr v1.ResourceRequirements `json:"mgr,omitempty"`
	Mon v1.ResourceRequirements `json:"mon,omitempty"`
	OSD v1.ResourceRequirements `json:"osd,omitempty"`
}

type StorageSpec struct {
	Nodes       []Node `json:"nodes,omitempty"`
	UseAllNodes bool   `json:"useAllNodes,omitempty"`
	Selection
	Config
}

type Node struct {
	Name      string                  `json:"name,omitempty"`
	Devices   []Device                `json:"devices,omitempty"`
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
	Selection
	Config
}

type Device struct {
	Name string `json:"name,omitempty"`
}

type Directory struct {
	Path string `json:"path,omitempty"`
}

type Selection struct {
	// Whether to consume all the storage devices found on a machine
	UseAllDevices *bool `json:"useAllDevices,omitempty"`

	// A regular expression to allow more fine-grained selection of devices on nodes across the cluster
	DeviceFilter string `json:"deviceFilter,omitempty"`

	MetadataDevice string `json:"metadataDevice,omitempty"`

	Directories []Directory `json:"directories,omitempty"`
}

type Config struct {
	StoreConfig StoreConfig `json:"storeConfig,omitempty"`
	Location    string      `json:"location,omitempty"`
}

type StoreConfig struct {
	StoreType      string `json:"storeType,omitempty"`
	WalSizeMB      int    `json:"walSizeMB,omitempty"`
	DatabaseSizeMB int    `json:"databaseSizeMB,omitempty"`
	JournalSizeMB  int    `json:"journalSizeMB,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type PlacementSpec struct {
	metav1.TypeMeta `json:",inline"`
	All             Placement `json:"all,omitempty"`
	API             Placement `json:"api,omitempty"`
	Mgr             Placement `json:"mgr,omitempty"`
	Mon             Placement `json:"mon,omitempty"`
	OSD             Placement `json:"osd,omitempty"`
}

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Pool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              PoolSpec `json:"spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type PoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []Pool `json:"items"`
}

// PoolSpec represent the spec of a pool
type PoolSpec struct {
	// The failure domain: osd or host (technically also any type in the crush map)
	FailureDomain string `json:"failureDomain"`

	// The root of the crush hierarchy utilized by the pool
	CrushRoot string `json:"crushRoot"`

	// The replication settings
	Replicated ReplicatedSpec `json:"replicated"`

	// The erasure code settings
	ErasureCoded ErasureCodedSpec `json:"erasureCoded"`
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

type VolumeAttachment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Attachments       []Attachment `json:"attachments"`
}

type Attachment struct {
	Node         string `json:"node"`
	PodNamespace string `json:"podNamespace"`
	PodName      string `json:"podName"`
	ClusterName  string `json:"clusterName"`
	MountDir     string `json:"mountDir"`
	ReadOnly     bool   `json:"readOnly"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type VolumeAttachmentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []VolumeAttachment `json:"items"`
}

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Filesystem struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              FilesystemSpec `json:"spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type FilesystemList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []Filesystem `json:"items"`
}

// FilesystemSpec represents the spec of a file system
type FilesystemSpec struct {
	// The metadata pool settings
	MetadataPool PoolSpec `json:"metadataPool"`

	// The data pool settings
	DataPools []PoolSpec `json:"dataPools"`

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
	Placement Placement `json:"placement"`

	// The resource requirements for the rgw pods
	Resources v1.ResourceRequirements `json:"resources"`
}

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ObjectStore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              ObjectStoreSpec `json:"spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ObjectStoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []ObjectStore `json:"items"`
}

// ObjectStoreSpec represent the spec of a pool
type ObjectStoreSpec struct {
	// The metadata pool settings
	MetadataPool PoolSpec `json:"metadataPool"`

	// The data pool settings
	DataPool PoolSpec `json:"dataPool"`

	// The rgw pod info
	Gateway GatewaySpec `json:"gateway"`
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
	Placement Placement `json:"placement"`

	// The resource requirements for the rgw pods
	Resources v1.ResourceRequirements `json:"resources"`
}

// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Placement encapsulates the various kubernetes options that control where pods are scheduled and executed.
type Placement struct {
	metav1.TypeMeta `json:",inline"`
	NodeAffinity    *v1.NodeAffinity    `json:"nodeAffinity,omitempty"`
	PodAffinity     *v1.PodAffinity     `json:"podAffinity,omitempty"`
	PodAntiAffinity *v1.PodAntiAffinity `json:"podAntiAffinity,omitempty"`
	Tolerations     []v1.Toleration     `json:"tolerations,omitempty"`
}
