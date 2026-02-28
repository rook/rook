/*
Copyright 2021 The Kubernetes-CSI-Addons Authors.

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
)

type FenceState string

const (
	// Fenced means the CIDRs should be in fenced state
	Fenced FenceState = "Fenced"

	// Unfenced means the CIDRs should be in unfenced state
	Unfenced FenceState = "Unfenced"
)

type FencingOperationResult string

const (
	// FencingOperationResultSucceeded represents the Succeeded operation state.
	FencingOperationResultSucceeded FencingOperationResult = "Succeeded"

	// FencingOperationResultFailed represents the Failed operation state.
	FencingOperationResultFailed FencingOperationResult = "Failed"
)

const (
	// FenceOperationSuccessfulMessage represents successful message on fence operation
	FenceOperationSuccessfulMessage = "fencing operation successful"

	// UnFenceOperationSuccessfulMessage represents successful message on unfence operation
	UnFenceOperationSuccessfulMessage = "unfencing operation successful"
)

// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="secret is immutable"
// SecretSpec defines the secrets to be used for the network fencing operation.
type SecretSpec struct {
	// Name specifies the name of the secret.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name,omitempty"`

	// Namespace specifies the namespace in which the secret
	// is located.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="namespace is immutable"
	Namespace string `json:"namespace,omitempty"`
}

// NetworkFenceSpec defines the desired state of NetworkFence
// +kubebuilder:validation:XValidation:rule="has(self.driver) || has(self.networkFenceClassName)",message="one of driver or networkFenceClassName must be present"
// +kubebuilder:validation:XValidation:rule="has(self.networkFenceClassName) || has(self.secret)",message="secret must be present when networkFenceClassName is not specified"
type NetworkFenceSpec struct {
	// NetworkFenceClassName contains the name of the NetworkFenceClass
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="networkFenceClassName is immutable"
	NetworkFenceClassName string `json:"networkFenceClassName"`

	// Driver contains the name of CSI driver, required if NetworkFenceClassName is absent
	// +kubebuilder:deprecatedversion:warning="specifying driver in networkfence is deprecated, please use networkFenceClassName instead"
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="driver is immutable"
	Driver string `json:"driver"`

	// FenceState contains the desired state for the CIDRs
	// mentioned in the Spec. i.e. Fenced or Unfenced
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=Fenced;Unfenced
	// +kubebuilder:default:=Fenced
	FenceState FenceState `json:"fenceState"`

	// Cidrs contains a list of CIDR blocks, which are required to be fenced.
	// +kubebuilder:validation:Required
	Cidrs []string `json:"cidrs"`

	// Secret is a kubernetes secret, which is required to perform the fence/unfence operation.
	// +kubebuilder:deprecatedversion:warning="specifying secrets in networkfence is deprecated, please use networkFenceClassName instead"
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="secrets are immutable"
	Secret SecretSpec `json:"secret,omitempty"`

	// Parameters is used to pass additional parameters to the CSI driver.
	// +kubebuilder:deprecatedversion:warning="specifying parameters in networkfence is deprecated, please use networkFenceClassName instead"
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="parameters are immutable"
	Parameters map[string]string `json:"parameters,omitempty"`
}

// NetworkFenceStatus defines the observed state of NetworkFence
type NetworkFenceStatus struct {
	// Result indicates the result of Network Fence/Unfence operation.
	Result FencingOperationResult `json:"result,omitempty"`

	// Message contains any message from the NetworkFence operation.
	Message string `json:"message,omitempty"`

	// Conditions are the list of conditions and their status.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Driver",type="string",JSONPath=".spec.driver"
//+kubebuilder:printcolumn:name="Cidrs",type="string",JSONPath=".spec.cidrs"
//+kubebuilder:printcolumn:name="FenceState",type="string",JSONPath=".spec.fenceState"
//+kubebuilder:printcolumn:JSONPath=".metadata.creationTimestamp",name=Age,type=date
//+kubebuilder:printcolumn:JSONPath=".status.result",name=Result,type=string
//+kubebuilder:resource:path=networkfences,scope=Cluster,singular=networkfence

// NetworkFence is the Schema for the networkfences API
type NetworkFence struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +kubebuilder:validation:Required
	Spec NetworkFenceSpec `json:"spec"`

	Status NetworkFenceStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NetworkFenceList contains a list of NetworkFence
type NetworkFenceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NetworkFence `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NetworkFence{}, &NetworkFenceList{})
}
