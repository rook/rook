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
package v1alpha2

import (
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ************************************************************************************
// IMPORTANT FOR CODE GENERATION
// If any of the types with codegen tags in this file are updated, you will need to run
// `make codegen` to generate the new types under the client/clientset folder.
// ************************************************************************************

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type StorageScopeSpec struct {
	metav1.TypeMeta `json:",inline"`
	Nodes           []Node            `json:"nodes,omitempty"`
	UseAllNodes     bool              `json:"useAllNodes,omitempty"`
	NodeCount       int               `json:"nodeCount,omitempty"`
	Location        string            `json:"location,omitempty"`
	Config          map[string]string `json:"config"`
	Selection
}

type Node struct {
	Name      string                  `json:"name,omitempty"`
	Location  string                  `json:"location,omitempty"`
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
	Config    map[string]string       `json:"config"`
	Selection
}

type Device struct {
	Name     string            `json:"name,omitempty"`
	FullPath string            // TODO: FullPath to be supported for devices
	Config   map[string]string `json:"config"`
}

type Directory struct {
	Path   string            `json:"path,omitempty"`
	Config map[string]string `json:"config"`
}

type Selection struct {
	// Whether to consume all the storage devices found on a machine
	UseAllDevices *bool `json:"useAllDevices,omitempty"`
	// A regular expression to allow more fine-grained selection of devices on nodes across the cluster
	DeviceFilter string `json:"deviceFilter,omitempty"`
	// List of devices to use as storage devices
	Devices []Device `json:"devices,omitempty"`
	// List of host directories to use as storage
	Directories []Directory `json:"directories,omitempty"`
	// PersistentVolumeClaims to use as storage
	VolumeClaimTemplates []v1.PersistentVolumeClaim `json:"volumeClaimTemplates,omitempty"`
}

type PlacementSpec map[string]Placement

type Placement struct {
	NodeAffinity    *v1.NodeAffinity    `json:"nodeAffinity,omitempty"`
	PodAffinity     *v1.PodAffinity     `json:"podAffinity,omitempty"`
	PodAntiAffinity *v1.PodAntiAffinity `json:"podAntiAffinity,omitempty"`
	Tolerations     []v1.Toleration     `json:"tolerations,omitempty"`
}

type ResourceSpec map[string]v1.ResourceRequirements

type NetworkSpec struct {
	metav1.TypeMeta `json:",inline"`

	// HostNetwork to enable host network
	HostNetwork bool `json:"hostNetwork"`

	// Set of named ports that can be configured for this resource
	Ports []PortSpec `json:"ports,omitempty"`
}

type PortSpec struct {
	Name string `json:"name,omitempty"`
	Port int32  `json:"port,omitempty"`
}

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Volume struct {
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

type VolumeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []Volume `json:"items"`
}
