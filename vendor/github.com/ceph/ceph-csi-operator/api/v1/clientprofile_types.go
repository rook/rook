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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CephCsiSecretsSpec defines the secrets used by the client profile
// to access the Ceph cluster and perform operations
// on volumes.
type CephCsiSecretsSpec struct {
	//+kubebuilder:validation:Optional
	ControllerPublishSecret corev1.SecretReference `json:"controllerPublishSecret,omitempty"`
}

// CephFsConfigSpec defines the desired CephFs configuration
type CephFsConfigSpec struct {
	//+kubebuilder:validation:Optional
	SubVolumeGroup string `json:"subVolumeGroup,omitempty"`

	//+kubebuilder:validation:Optional
	KernelMountOptions map[string]string `json:"kernelMountOptions,omitempty"`

	//+kubebuilder:validation:Optional
	FuseMountOptions map[string]string `json:"fuseMountOptions,omitempty"`

	//+kubebuilder:validation:XValidation:rule="self == oldSelf",message="field is immutable"
	//+kubebuilder:validation:Optional
	RadosNamespace *string `json:"radosNamespace,omitempty"`

	//+kubebuilder:validation:Optional
	CephCsiSecrets *CephCsiSecretsSpec `json:"cephCsiSecrets,omitempty"`
}

// RbdConfigSpec defines the desired RBD configuration
type RbdConfigSpec struct {
	//+kubebuilder:validation:XValidation:rule="self == oldSelf",message="field is immutable"
	//+kubebuilder:validation:Optional
	RadosNamespace string `json:"radosNamespace,omitempty"`

	//+kubebuilder:validation:Optional
	CephCsiSecrets *CephCsiSecretsSpec `json:"cephCsiSecrets,omitempty"`
}

// NfsConfigSpec cdefines the desired NFS configuration
type NfsConfigSpec struct {
}

// ClientProfileSpec defines the desired state of Ceph CSI
// configuration for volumes and snapshots configured to use
// this profile
type ClientProfileSpec struct {
	//+kubebuilder:validation:Required
	//+kubebuilder:validation:XValidation:rule=self.name != "",message="'.name' cannot be empty"
	CephConnectionRef corev1.LocalObjectReference `json:"cephConnectionRef"`

	//+kubebuilder:validation:Optional
	CephFs *CephFsConfigSpec `json:"cephFs,omitempty"`

	//+kubebuilder:validation:Optional
	Rbd *RbdConfigSpec `json:"rbd,omitempty"`

	//+kubebuilder:validation:Optional
	Nfs *NfsConfigSpec `json:"nfs,omitempty"`
}

// ClientProfileStatus defines the observed state of Ceph CSI
// configuration for volumes and snapshots configured to use
// this profile
type ClientProfileStatus struct {
}

//+kubebuilder:object:root=true
//+kubebuilder:storageversion
//+kubebuilder:subresource:status

// ClientProfile is the Schema for the clientprofiles API
type ClientProfile struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClientProfileSpec   `json:"spec,omitempty"`
	Status ClientProfileStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ClientProfileList contains a list of ClientProfile
type ClientProfileList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClientProfile `json:"items"`
}
