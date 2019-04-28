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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	APIVersion = CustomResourceGroup + "/" + Version
)

// ***************************************************************************
// IMPORTANT FOR CODE GENERATION
// If the types in this file are updated, you will need to run
// `make codegen` to generate the new types under the client/clientset folder.
// ***************************************************************************

// Kubernetes API Conventions:
// https://github.com/kubernetes/community/blob/af5c40530f50c3b36c13438187b311102093ede5/contributors/devel/api-conventions.md
// Applicable Here:
//  * Optional fields use a pointer to correctly handle empty values.

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Cluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              ClusterSpec   `json:"spec"`
	Status            ClusterStatus `json:"status"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []Cluster `json:"items"`
}

// ClusterSpec is the desired state for a Cassandra Cluster.
type ClusterSpec struct {
	// The annotations-related configuration to add/set on each Pod related object.
	Annotations rook.Annotations
	// Version of Cassandra to use.
	Version string `json:"version"`
	// Repository to pull the image from.
	Repository *string `json:"repository,omitempty"`
	// Mode selects an operating mode.
	Mode ClusterMode `json:"mode,omitempty"`
	// Datacenter that will make up this cluster.
	Datacenter DatacenterSpec `json:"datacenter"`
	// User-provided image for the sidecar that replaces default.
	SidecarImage *ImageSpec `json:"sidecarImage,omitempty"`
}

type ClusterMode string

const (
	ClusterModeCassandra ClusterMode = "cassandra"
	ClusterModeScylla    ClusterMode = "scylla"
)

// DatacenterSpec is the desired state for a Cassandra Datacenter.
type DatacenterSpec struct {
	// Name of the Cassandra Datacenter. Used in the cassandra-rackdc.properties file.
	Name string `json:"name"`
	// Racks of the specific Datacenter.
	Racks []RackSpec `json:"racks"`
}

// RackSpec is the desired state for a Cassandra Rack.
type RackSpec struct {
	// Name of the Cassandra Rack. Used in the cassandra-rackdc.properties file.
	Name string `json:"name"`
	// Members is the number of Cassandra instances in this rack.
	Members int32 `json:"members"`
	// User-provided ConfigMap applied to the specific statefulset.
	ConfigMapName *string `json:"configMapName,omitempty"`
	// Storage describes the underlying storage that Cassandra will consume.
	Storage rook.StorageScopeSpec `json:"storage"`
	// The annotations-related configuration to add/set on each Pod related object.
	Annotations rook.Annotations
	// Placement describes restrictions for the nodes Cassandra is scheduled on.
	Placement *rook.Placement `json:"placement,omitempty"`
	// Resources the Cassandra Pods will use.
	Resources corev1.ResourceRequirements `json:"resources"`
}

// ImageSpec is the desired state for a container image.
type ImageSpec struct {
	// Version of the image.
	Version string `json:"version"`
	// Repository to pull the image from.
	Repository string `json:"repository"`
}

// ClusterStatus is the status of a Cassandra Cluster
type ClusterStatus struct {
	Racks map[string]*RackStatus `json:"racks,omitempty"`
}

// RackStatus is the status of a Cassandra Rack
type RackStatus struct {
	// Members is the current number of members requested in the specific Rack
	Members int32 `json:"members"`
	// ReadyMembers is the number of ready members in the specific Rack
	ReadyMembers int32 `json:"readyMembers"`
	// Conditions are the latest available observations of a rack's state.
	Conditions []RackCondition `json:"conditions,omitempty"`
}

// RackCondition is an observation about the state of a rack.
type RackCondition struct {
	Type   RackConditionType `json:"type"`
	Status ConditionStatus   `json:"status"`
}

type RackConditionType string

const (
	RackConditionTypeMemberLeaving RackConditionType = "MemberLeaving"
)

type ConditionStatus string

// These are valid condition statuses. "ConditionTrue" means a resource is in the condition;
// "ConditionFalse" means a resource is not in the condition; "ConditionUnknown" means kubernetes
// can't decide if a resource is in the condition or not.
const (
	ConditionTrue    ConditionStatus = "True"
	ConditionFalse   ConditionStatus = "False"
	ConditionUnknown ConditionStatus = "Unknown"
)
