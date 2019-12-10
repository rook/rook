/*
Copyright 2019 The Rook Authors. All rights reserved.

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
	rook "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
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

type Cluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              ClusterSpec   `json:"spec"`
	Status            ClusterStatus `json:"status,omitempty"`
}

type ClusterStatus struct {
	State   ClusterState `json:"state,omitempty"`
	Message string       `json:"message,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []Cluster `json:"items"`
}

type ClusterSpec struct {
	Storage rook.StorageScopeSpec `json:"storage,omitempty"`
	// The annotations-related configuration to add/set on each Pod related object.
	Annotations rook.AnnotationsSpec `json:"annotations,omitempty"`
	// The placement-related configuration to pass to kubernetes (affinity, node selector, tolerations).
	Placement rook.PlacementSpec `json:"placement,omitempty"`
	Network   rook.NetworkSpec   `json:"network,omitempty"`
	Dashboard DashboardSpec      `json:"dashboard,omitempty"`
	// Resources set resource requests and limits
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
	// The path on the host where config and data can be persisted.
	DataDirHostPath         string            `json:"dataDirHostPath,omitempty"`
	ServiceAccount          string            `json:"serviceAccount,omitempty"`
	DataVolumeSize          resource.Quantity `json:"dataVolumeSize,omitempty"`
	DevicesResurrectMode    string            `json:"devicesResurrectMode,omitempty"`
	EdgefsImageName         string            `json:"edgefsImageName,omitempty"`
	SkipHostPrepare         bool              `json:"skipHostPrepare,omitempty"`
	ResourceProfile         string            `json:"resourceProfile,omitempty"`
	ChunkCacheSize          resource.Quantity `json:"chunkCacheSize,omitempty"`
	TrlogProcessingInterval int               `json:"trlogProcessingInterval,omitempty"`
	TrlogKeepDays           int               `json:"trlogKeepDays,omitempty"`
	SystemReplicationCount  int               `json:"sysRepCount,omitempty"`
	FailureDomain           string            `json:"failureDomain,omitempty"`
	CommitNWait             int               `json:"commitNWait,omitempty"`
	NoIP4Frag               bool              `json:"noIP4Frag,omitempty"`
	MaxContainerCapacity    resource.Quantity `json:"maxContainerCapacity,omitempty"`
	UseHostLocalTime        bool              `json:"useHostLocalTime,omitempty"`
	SysChunkSize            int               `json:"sysMaxChunkSize,omitempty"`
}

type DashboardSpec struct {
	LocalAddr string `json:"localAddr"`
}

type ClusterState string

const (
	ClusterStateCreating ClusterState = "Creating"
	ClusterStateCreated  ClusterState = "Created"
	ClusterStateUpdating ClusterState = "Updating"
	ClusterStateDeleting ClusterState = "Deleting"
	ClusterStateError    ClusterState = "Error"
)

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type NFS struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              NFSSpec `json:"spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type NFSList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []NFS `json:"items"`
}

// NFSSpec represent the spec of a pool
type NFSSpec struct {
	// The annotations-related configuration to add/set on each Pod related object.
	Annotations rook.Annotations `json:"annotations,omitempty"`
	// The affinity to place the NFS pods (default is to place on any available nodes in EdgeFS running namespace)
	Placement rook.Placement `json:"placement"`
	// Resources set resource requests and limits
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
	// The number of pods in the NFS replicaset
	Instances         int32             `json:"instances"`
	RelaxedDirUpdates bool              `json:"relaxedDirUpdates,omitempty"`
	ResourceProfile   string            `json:"resourceProfile,omitempty"`
	ChunkCacheSize    resource.Quantity `json:"chunkCacheSize,omitempty"`
}

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type S3 struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              S3Spec `json:"spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type S3List struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []S3 `json:"items"`
}

// S3Spec represent the spec of a s3 service
type S3Spec struct {
	// The annotations-related configuration to add/set on each Pod related object.
	Annotations rook.Annotations `json:"annotations,omitempty"`
	// The affinity to place the S3 pods (default is to place on any available nodes in EdgeFS running namespace)
	Placement rook.Placement `json:"placement"`
	// Resources set resource requests and limits
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
	// The number of pods in the S3 replicaset
	Instances int32 `json:"instances"`
	//S3 Http port (default value 9982)
	Port uint `json:"port,omitempty"`
	//S3 Https port (default value 9443)
	SecurePort uint `json:"securePort,omitempty"`
	// Service type to expose (default value ClusterIP)
	ServiceType string `json:"serviceType,omitempty"`
	// S3 Http external port
	ExternalPort uint `json:"externalPort,omitempty"`
	// S3 Https external port
	SecureExternalPort uint `json:"secureExternalPort,omitempty"`
	// The name of the secret that stores the ssl certificate for secure s3 connections
	SSLCertificateRef string `json:"sslCertificateRef,omitempty"`
	// S3 type: s3 (bucket as url, default), s3s (bucket as DNS subdomain), s3g (new, experimental)
	S3Type          string            `json:"s3type,omitempty"`
	ResourceProfile string            `json:"resourceProfile,omitempty"`
	ChunkCacheSize  resource.Quantity `json:"chunkCacheSize,omitempty"`
}

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type SWIFT struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              SWIFTSpec `json:"spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type SWIFTList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []SWIFT `json:"items"`
}

// SWIFTSpec represent the spec of a swift service
type SWIFTSpec struct {
	// The affinity to place the SWIFT pods (default is to place on any available nodes in EdgeFS running namespace)
	Placement rook.Placement `json:"placement"`
	// Resources set resource requests and limits
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
	// The number of pods in the S3 replicaset
	Instances int32 `json:"instances"`
	//S3 Http port (default value 9982)
	Port uint `json:"port,omitempty"`
	//S3 Https port (default value 9443)
	SecurePort uint `json:"securePort,omitempty"`
	// Service type to expose (default value ClusterIP)
	ServiceType string `json:"serviceType,omitempty"`
	// S3 Http external port
	ExternalPort uint `json:"externalPort,omitempty"`
	// S3 Https external port
	SecureExternalPort uint `json:"secureExternalPort,omitempty"`
	// The name of the secret that stores the ssl certificate for secure s3 connections
	SSLCertificateRef string            `json:"sslCertificateRef,omitempty"`
	ResourceProfile   string            `json:"resourceProfile,omitempty"`
	ChunkCacheSize    resource.Quantity `json:"chunkCacheSize,omitempty"`
}

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type S3X struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              S3XSpec `json:"spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type S3XList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []S3X `json:"items"`
}

// S3XSpec represent the spec of a s3 service
type S3XSpec struct {
	// The annotations-related configuration to add/set on each Pod related object.
	Annotations rook.Annotations `json:"annotations,omitempty"`
	// The affinity to place the S3X pods (default is to place on any available nodes in EdgeFS running namespace)
	Placement rook.Placement `json:"placement"`
	// Resources set resource requests and limits
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
	// The number of pods in the S3X replicaset
	Instances int32 `json:"instances"`
	//S3X Http port (default value 3000)
	Port uint `json:"port,omitempty"`
	//S3X Https port (default value 3001)
	SecurePort uint `json:"securePort,omitempty"`
	// Service type to expose (default value ClusterIP)
	ServiceType string `json:"serviceType,omitempty"`
	// S3 Http external port
	ExternalPort uint `json:"externalPort,omitempty"`
	// S3 Https external port
	SecureExternalPort uint `json:"secureExternalPort,omitempty"`
	// The name of the secret that stores the ssl certificate for secure s3x connections
	SSLCertificateRef string            `json:"sslCertificateRef,omitempty"`
	ResourceProfile   string            `json:"resourceProfile,omitempty"`
	ChunkCacheSize    resource.Quantity `json:"chunkCacheSize,omitempty"`
}

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ISCSI struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              ISCSISpec `json:"spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ISCSIList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []ISCSI `json:"items"`
}

// ISCSISpec represent the spec of a iscsi service
type ISCSISpec struct {
	// The annotations-related configuration to add/set on each Pod related object.
	Annotations rook.Annotations `json:"annotations,omitempty"`
	// The affinity to place the ISCSI pods (default is to place on any available nodes in EdgeFS running namespace)
	Placement rook.Placement `json:"placement"`
	// Resources set resource requests and limits
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
	// The number of pods in the ISCSI replicaset
	Instances int32 `json:"instances"`
	//IISCSI Http port (default value 3000)
	TargetName string `json:"targetName,omitempty"`
	//specific ISCSI target parameters
	TargetParams    TargetParametersSpec `json:"targetParams"`
	ResourceProfile string               `json:"resourceProfile,omitempty"`
	ChunkCacheSize  resource.Quantity    `json:"chunkCacheSize,omitempty"`
}

type TargetParametersSpec struct {
	MaxRecvDataSegmentLength uint `json:"maxRecvDataSegmentLength"`
	DefaultTime2Retain       uint `json:"defaultTime2Retain"`
	DefaultTime2Wait         uint `json:"defaultTime2Wait"`
	FirstBurstLength         uint `json:"firstBurstLength"`
	MaxBurstLength           uint `json:"maxBurstLength"`
	MaxQueueCmd              uint `json:"maxQueueCmd"`
}

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ISGW struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              ISGWSpec `json:"spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ISGWList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []ISGW `json:"items"`
}

type ISGWConfig struct {
	Server  string   `json:"server,omitempty"`
	Clients []string `json:"clients,omitempty"`
}

// ISGWSpec represent the spec of a isgw service
type ISGWSpec struct {
	// The annotations-related configuration to add/set on each Pod related object.
	Annotations rook.Annotations `json:"annotations,omitempty"`
	// The affinity to place the ISGW pods (default is to place on any available nodes in EdgeFS running namespace)
	Placement rook.Placement `json:"placement"`
	// Resources set resource requests and limits
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
	// The number of pods in the ISGW replicaset
	Instances int32 `json:"instances"`
	// ISGW I/O flow direction
	Direction string `json:"direction,omitempty"`
	// ISGW remote URL
	RemoteURL string `json:"remoteURL,omitempty"`
	// ISGW ServiceType (default is ClusterIP)
	ServiceType string `json:"serviceType,omitempty"`
	// ISGW Relay config
	Config ISGWConfig `json:"config,omitempty"`
	// ISGW external port
	ExternalPort uint `json:"externalPort,omitempty"`
	// ISGW Replication Type
	ReplicationType string `json:"replicationType,omitempty"`
	// ISGW Metadata Only flag, all or versions
	MetadataOnly string `json:"metadataOnly,omitempty"`
	// ISGW Dynamic Fetch Addr (default is '-', means disabled)
	DynamicFetchAddr string `json:"dynamicFetchAddr,omitempty"`
	// ISGW Endpoint local address (default value 0.0.0.0:14000)
	LocalAddr string `json:"localAddr,omitempty"`
	// ISGW Encrypted Tunnel flag
	UseEncryptedTunnel bool              `json:"useEncryptedTunnel,omitempty"`
	ResourceProfile    string            `json:"resourceProfile,omitempty"`
	ChunkCacheSize     resource.Quantity `json:"chunkCacheSize,omitempty"`
}
