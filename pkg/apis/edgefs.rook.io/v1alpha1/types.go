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
package v1alpha1

import (
	rook "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
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
	// The placement-related configuration to pass to kubernetes (affinity, node selector, tolerations).
	Placement rook.PlacementSpec `json:"placement,omitempty"`
	Network   rook.NetworkSpec   `json:"network,omitempty"`

	// Resources set resource requests and limits
	Resources rook.ResourceSpec `json:"resources,omitempty"`
	// The path on the host where config and data can be persisted.
	DataDirHostPath string            `json:"dataDirHostPath,omitempty"`
	ServiceAccount  string            `json:"serviceAccount,omitempty"`
	DataVolumeSize  resource.Quantity `json:"dataVolumeSize,omitempty"`
	ZapDevices      bool              `json:"zapDevices,omitempty"`
	Resurrect       bool              `json:"resurrect,omitempty"`
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
	// The affinity to place the NFS pods (default is to place on any available nodes in EdgeFS running namespace)
	Placement rook.Placement `json:"placement"`
	// Resources set resource requests and limits
	Resources rook.ResourceSpec `json:"resources,omitempty"`
	// The number of pods in the NFS replicaset
	Instances int32 `json:"instances"`
}
