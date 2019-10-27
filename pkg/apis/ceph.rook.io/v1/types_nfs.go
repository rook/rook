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
	"time"

	v1 "k8s.io/api/core/v1"
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

type CephNFS struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              NFSGaneshaSpec `json:"spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type CephNFSList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []CephNFS `json:"items"`
}

// NFSGaneshaSpec represents the spec of an nfs ganesha server
type NFSGaneshaSpec struct {
	RADOS GaneshaRADOSSpec `json:"rados"`

	Server GaneshaServerSpec `json:"server"`
}

type GaneshaRADOSSpec struct {
	// Pool is the RADOS pool where NFS client recovery data is stored.
	Pool string `json:"pool"`

	// Namespace is the RADOS namespace where NFS client recovery data is stored.
	Namespace string `json:"namespace"`
}

type GaneshaServerSpec struct {
	// The number of active Ganesha servers
	Active int `json:"active"`

	// The affinity to place the ganesha pods
	Placement rook.Placement `json:"placement"`

	// The annotations-related configuration to add/set on each Pod related object.
	Annotations rook.Annotations `json:"annotations,omitempty"`

	// Resources set resource requests and limits
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
}