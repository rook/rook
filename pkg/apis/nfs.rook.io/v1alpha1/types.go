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
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	v1 "k8s.io/api/core/v1"
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

type NFSServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              NFSServerSpec `json:"spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type NFSServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []NFSServer `json:"items"`
}

// NFSServerSpec represents the spec of NFS daemon
type NFSServerSpec struct {
	// The annotations-related configuration to add/set on each Pod related object.
	Annotations rookalpha.Annotations `json:"annotations,omitempty"`

	// Replicas of the NFS daemon
	Replicas int `json:"replicas,omitempty"`

	// The parameters to configure the NFS export
	Exports []ExportsSpec `json:"exports,omitempty"`
}

// ExportsSpec represents the spec of NFS exports
type ExportsSpec struct {
	// Name of the export
	Name string `json:"name,omitempty"`

	// The NFS server configuration
	Server ServerSpec `json:"server,omitempty"`

	// PVC from which the NFS daemon gets storage for sharing
	PersistentVolumeClaim v1.PersistentVolumeClaimVolumeSource `json:"persistentVolumeClaim,omitempty"`
}

// ServerSpec represents the spec for configuring the NFS server
type ServerSpec struct {
	// Reading and Writing permissions on the export
	// Valid values are "ReadOnly", "ReadWrite" and "none"
	AccessMode string `json:"accessMode,omitempty"`

	// This prevents the root users connected remotely from having root privileges
	// Valid values are "none", "rootid", "root", and "all"
	Squash string `json:"squash,omitempty"`

	// The clients allowed to access the NFS export
	AllowedClients []AllowedClientsSpec `json:"allowedClients,omitempty"`
}

// AllowedClientsSpec represents the client specs for accessing the NFS export
type AllowedClientsSpec struct {

	// Name of the clients group
	Name string `json:"name,omitempty"`

	// The clients that can access the share
	// Values can be hostname, ip address, netgroup, CIDR network address, or all
	Clients []string `json:"clients,omitempty"`

	// Reading and Writing permissions for the client to access the NFS export
	// Valid values are "ReadOnly", "ReadWrite" and "none"
	// Gets overridden when ServerSpec.accessMode is specified
	AccessMode string `json:"accessMode,omitempty"`

	// Squash options for clients
	// Valid values are "none", "rootid", "root", and "all"
	// Gets overridden when ServerSpec.squash is specified
	Squash string `json:"squash,omitempty"`
}
