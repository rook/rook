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
	"github.com/rook/rook/pkg/apis/rook.io"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	APIVersion = CustomResourceGroup + "/" + Version

	// These are valid condition statuses. "ConditionTrue" means a resource is in the condition;
	// "ConditionFalse" means a resource is not in the condition; "ConditionUnknown" means kubernetes
	// can't decide if a resource is in the condition or not.
	ConditionTrue    ConditionStatus = "True"
	ConditionFalse   ConditionStatus = "False"
	ConditionUnknown ConditionStatus = "Unknown"
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
	Spec              ClusterSpec `json:"spec"`
	// +optional
	// +nullable
	Status ClusterStatus `json:"status,omitempty"`
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
	// +optional
	// +nullable
	Annotations rook.Annotations `json:"annotations,omitempty"`
	// Version of Cassandra to use.
	Version string `json:"version"`
	// Repository to pull the image from.
	// +optional
	// +nullable
	Repository *string `json:"repository,omitempty"`
	// Mode selects an operating mode.
	// +optional
	Mode ClusterMode `json:"mode,omitempty"`
	// Datacenter that will make up this cluster.
	// +optional
	// +nullable
	Datacenter DatacenterSpec `json:"datacenter,omitempty"`
	// User-provided image for the sidecar that replaces default.
	// +optional
	// +nullable
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
	// +optional
	// +nullable
	ConfigMapName *string `json:"configMapName,omitempty"`
	// User-provided ConfigMap for jmx prometheus exporter
	// +optional
	// +nullable
	JMXExporterConfigMapName *string `json:"jmxExporterConfigMapName,omitempty"`
	// Storage describes the underlying storage that Cassandra will consume.
	Storage StorageScopeSpec `json:"storage,omitempty"`
	// The annotations-related configuration to add/set on each Pod related object.
	// +optional
	// +nullable
	Annotations map[string]string `json:"annotations,omitempty"`
	// Placement describes restrictions for the nodes Cassandra is scheduled on.
	// +optional
	// +nullable
	Placement *Placement `json:"placement,omitempty"`
	// Resources the Cassandra Pods will use.
	// +optional
	// +nullable
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

// ImageSpec is the desired state for a container image.
type ImageSpec struct {
	// Version of the image.
	Version string `json:"version"`
	// Repository to pull the image from.
	// +optional
	Repository string `json:"repository,omitempty"`
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

type StorageScopeSpec struct {
	// +nullable
	// +optional
	Nodes []Node `json:"nodes,omitempty"`

	// PersistentVolumeClaims to use as storage
	// +optional
	VolumeClaimTemplates []v1.PersistentVolumeClaim `json:"volumeClaimTemplates,omitempty"`
}

// Node is a storage nodes
// +nullable
type Node struct {
	// +optional
	Name string `json:"name,omitempty"`
}

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
