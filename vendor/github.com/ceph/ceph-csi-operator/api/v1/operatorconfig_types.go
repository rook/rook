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

// OperatorLogSpec provide log related settings for the operator
type OperatorLogSpec struct {
	// Operator's log level
	//+kubebuilder:validation:Optional
	//+kubebuilder:validation:Minimum=0
	//+kubebuilder:validation:Maximum=3
	Verbosity int `json:"verbosity,omitempty"`
}

// OperatorConfigSpec defines the desired state of OperatorConfig
type OperatorConfigSpec struct {
	//+kubebuilder:validation:Optional
	Log *OperatorLogSpec `json:"log,omitempty"`

	// Allow overwrite of hardcoded defaults for any driver managed by this operator
	//+kubebuilder:validation:Optional
	DriverSpecDefaults *DriverSpec `json:"driverSpecDefaults,omitempty"`
}

// OperatorConfigStatus defines the observed state of OperatorConfig
type OperatorConfigStatus struct {
}

//+kubebuilder:object:root=true
//+kubebuilder:storageversion
//+kubebuilder:subresource:status

// OperatorConfig is the Schema for the operatorconfigs API
type OperatorConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OperatorConfigSpec   `json:"spec,omitempty"`
	Status OperatorConfigStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// OperatorConfigList contains a list of OperatorConfig
type OperatorConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OperatorConfig `json:"items"`
}
