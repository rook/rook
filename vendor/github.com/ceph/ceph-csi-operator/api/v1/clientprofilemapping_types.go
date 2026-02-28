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

// +kubebuilder:validation:MinItems:=2
// +kubebuilder:validation:MaxItems:=2
type BlockPoolIdPair []string

// MappingsSpec define a mapping between a local and remote profiles
type MappingsSpec struct {
	//+kubebuilder:validation:Required
	LocalClientProfile string `json:"localClientProfile,omitempty"`

	//+kubebuilder:validation:Required
	RemoteClientProfile string `json:"remoteClientProfile,omitempty"`

	//+kubebuilder:validation:Optional
	BlockPoolIdMapping []BlockPoolIdPair `json:"blockPoolIdMapping,omitempty"`
}

// ClientProfileMappingSpec defines the desired state of ClientProfileMapping
type ClientProfileMappingSpec struct {
	//+kubebuilder:validation:Required
	Mappings []MappingsSpec `json:"mappings,omitempty"`
}

// ClientProfileMappingStatus defines the observed state of ClientProfileMapping
type ClientProfileMappingStatus struct {
}

//+kubebuilder:object:root=true
//+kubebuilder:storageversion
//+kubebuilder:subresource:status

// ClientProfileMapping is the Schema for the clientprofilemappings API
type ClientProfileMapping struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClientProfileMappingSpec   `json:"spec,omitempty"`
	Status ClientProfileMappingStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ClientProfileMappingList contains a list of ClientProfileMapping
type ClientProfileMappingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClientProfileMapping `json:"items"`
}
