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
	Spec              ClusterSpec `json:"spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []Cluster `json:"items"`
}

type ClusterSpec struct {
	// The annotations-related configuration to add/set on each Pod related object.
	Annotations         rook.Annotations      `json:"annotations,omitempty"`
	Storage             rook.StorageScopeSpec `json:"scope,omitempty"`
	Network             NetworkSpec           `json:"network,omitempty"`
	Secure              bool                  `json:"secure,omitempty"`
	CachePercent        int                   `json:"cachePercent,omitempty"`
	MaxSQLMemoryPercent int                   `json:"maxSQLMemoryPercent,omitempty"`
}

// NetworkSpec describes network related settings of the cluster
type NetworkSpec struct {
	// Set of named ports that can be configured for this resource
	Ports []PortSpec `json:"ports,omitempty"`
}

// PortSpec is named port
type PortSpec struct {
	// Name of port
	Name string `json:"name,omitempty"`
	// Port number
	Port int32 `json:"port,omitempty"`
}
