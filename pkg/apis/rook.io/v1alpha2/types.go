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

package v1alpha2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ************************************************************************************
// IMPORTANT FOR CODE GENERATION
// If any of the types with codegen tags in this file are updated, you will need to run
// `make codegen` to generate the new types under the client/clientset folder.
// ************************************************************************************

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type Volume struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Attachments       []Attachment `json:"attachments"`
}

type Attachment struct {
	Node         string `json:"node"`
	PodNamespace string `json:"podNamespace"`
	PodName      string `json:"podName"`
	ClusterName  string `json:"clusterName"`
	MountDir     string `json:"mountDir"`
	ReadOnly     bool   `json:"readOnly"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type VolumeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []Volume `json:"items"`
}
