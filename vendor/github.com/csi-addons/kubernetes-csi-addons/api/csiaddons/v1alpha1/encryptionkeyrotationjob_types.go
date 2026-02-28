/*
Copyright 2024 The Kubernetes-CSI-Addons Authors.

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

// EncryptionKeyRotationJobSpec defines the desired state of EncryptionKeyRotationJob
type EncryptionKeyRotationJobSpec struct {
	// Target represents tvolume target on which operation will be
	// performed.
	// +kubebuilder:validation:Required
	Target TargetSpec `json:"target"`

	// BackOffLimit specifies the number of retries allowed before marking reclaim
	// space operation as failed. If not specified, defaults to 6. Maximum allowed
	// value is 60 and minimum allowed value is 0.
	// +optional
	// +kubebuilder:validation:Maximum=60
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default:=6
	BackoffLimit int32 `json:"backOffLimit"`

	// RetryDeadlineSeconds specifies the duration in seconds relative to the
	// start time that the operation may be retried; value MUST be positive integer.
	// If not specified, defaults to 600 seconds. Maximum allowed
	// value is 1800.
	// +optional
	// +kubebuilder:validation:Maximum=1800
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default:=600
	RetryDeadlineSeconds int64 `json:"retryDeadlineSeconds"`

	// Timeout specifies the timeout in seconds for the grpc request sent to the
	// CSI driver.
	// Minimum allowed value is 60.
	// +optional
	// +kubebuilder:validation:Minimum=60
	Timeout *int64 `json:"timeout,omitempty"`
}

// EncryptionKeyRotationJobStatus defines the observed state of EncryptionKeyRotationJob
type EncryptionKeyRotationJobStatus struct {
	// Result indicates the result of EncryptionKeyRotationJob.
	Result OperationResult `json:"result,omitempty"`

	// Message contains any message from the EncryptionKeyRotationJob.
	Message string `json:"message,omitempty"`

	// Conditions are the list of conditions and their status.
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Retries indicates the number of times the operation is retried.
	Retries        int32        `json:"retries,omitempty"`
	StartTime      *metav1.Time `json:"startTime,omitempty"`
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:JSONPath=".metadata.namespace",name=Namespace,type=string
// +kubebuilder:printcolumn:JSONPath=".metadata.creationTimestamp",name=Age,type=date
// +kubebuilder:printcolumn:JSONPath=".status.retries",name=Retries,type=integer
// +kubebuilder:printcolumn:JSONPath=".status.result",name=Result,type=string

// EncryptionKeyRotationJob is the Schema for the encryptionkeyrotationjobs API
type EncryptionKeyRotationJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +kubebuilder:validation:Required
	Spec EncryptionKeyRotationJobSpec `json:"spec,omitempty"`

	Status EncryptionKeyRotationJobStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EncryptionKeyRotationJobList contains a list of EncryptionKeyRotationJob
type EncryptionKeyRotationJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EncryptionKeyRotationJob `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EncryptionKeyRotationJob{}, &EncryptionKeyRotationJobList{})
}
