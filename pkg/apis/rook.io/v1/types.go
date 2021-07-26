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
	v1 "k8s.io/api/core/v1"
)

// ************************************************************************************
// IMPORTANT FOR CODE GENERATION
// If any of the types with codegen tags in this file are updated, you will need to run
// `make codegen` to generate the new types under the client/clientset folder.
// ************************************************************************************

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

// NetworkSpec represents cluster network settings
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
}

// KeyType type safety
type KeyType string

// AnnotationsSpec is the main spec annotation for all daemons
// +kubebuilder:pruning:PreserveUnknownFields
// +nullable
type AnnotationsSpec map[KeyType]Annotations

// Annotations are annotations
type Annotations map[string]string

// LabelsSpec is the main spec label for all daemons
type LabelsSpec map[KeyType]Labels

// Labels are label for a given daemons
type Labels map[string]string

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
