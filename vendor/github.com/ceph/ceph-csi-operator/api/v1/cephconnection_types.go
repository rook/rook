/*
Copyright 2025.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ReadAffinitySpec capture Ceph CSI read affinity settings
type ReadAffinitySpec struct {
	//+kubebuilder:validation:Required
	//+kubebuilder:validation:MinItems:=1
	CrushLocationLabels []string `json:"crushLocationLabels,omitempty"`
}

// CephConnectionSpec defines the desired state of CephConnection
type CephConnectionSpec struct {
	//+kubebuilder:validation:Required
	//+kubebuilder:validation:MinItems:=1
	Monitors []string `json:"monitors"`

	//+kubebuilder:validation:Optional
	ReadAffinity *ReadAffinitySpec `json:"readAffinity,omitempty"`

	//+kubebuilder:validation:Optional
	//+kubebuilder:validation:Minimum:=1
	RbdMirrorDaemonCount int `json:"rbdMirrorDaemonCount,omitempty"`
}

// CephConnectionStatus defines the observed state of CephConnection
type CephConnectionStatus struct {
}

//+kubebuilder:object:root=true
//+kubebuilder:storageversion
//+kubebuilder:subresource:status

// CephConnection is the Schema for the cephconnections API
type CephConnection struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CephConnectionSpec   `json:"spec,omitempty"`
	Status CephConnectionStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CephConnectionList contains a list of CephConnections
type CephConnectionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CephConnection `json:"items"`
}
