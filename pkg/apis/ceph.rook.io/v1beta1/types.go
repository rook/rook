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
package v1beta1

import (
	"k8s.io/api/core/v1"
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
	// A spec for available storage in the cluster and how it should be used
	Storage rook.StorageScopeSpec `json:"storage,omitempty"`

	// The placement-related configuration to pass to kubernetes (affinity, node selector, tolerations).
	Placement rook.PlacementSpec `json:"placement,omitempty"`

	// Network related configuration
	Network rook.NetworkSpec `json:"network,omitempty"`

	// Resources set resource requests and limits
	Resources rook.ResourceSpec `json:"resources,omitempty"`

	// The service account in which to start the cluster resources if the default is not sufficient (OSD pods)
	ServiceAccount string `json:"serviceAccount,omitempty"`

	// The path on the host where config and data can be persisted.
	DataDirHostPath string `json:"dataDirHostPath,omitempty"`

	// A spec for mon related options
	Mon MonSpec `json:"mon"`

	// Dashboard settings
	Dashboard DashboardSpec `json:"dashboard,omitempty"`
}

// DashboardSpec represents the settings for the Ceph dashboard
type DashboardSpec struct {
	// Whether to enable the dashboard
	Enabled bool `json:"enabled,omitempty"`
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

type MonSpec struct {
	Count                int  `json:"count"`
	AllowMultiplePerNode bool `json:"allowMultiplePerNode"`
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
	Placement rook.Placement `json:"placement"`

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
	Placement rook.Placement `json:"placement"`

	// The resource requirements for the rgw pods
	Resources v1.ResourceRequirements `json:"resources"`
}
