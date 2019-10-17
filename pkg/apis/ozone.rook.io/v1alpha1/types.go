/*
Copyright 2019 The Rook Authors. All rights reserved.

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
type OzoneObjectStore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              OzoneObjectStoreSpec `json:"spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type OzoneObjectStoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []OzoneObjectStore `json:"items"`
}

// ObjectStoreSpec represent the spec of a Ozone object store.
type OzoneObjectStoreSpec struct {
	// The version information that instructs Rook to orchestrate a particular version of Ozone.
	OzoneVersion OzoneVersionSpec `json:"ozoneVersion,omitempty"`

	// A spec for available storage in the cluster and how it should be used
	Storage rook.StorageScopeSpec `json:"scope,omitempty"`

	// The annotations to add/set on each Pod related object.
	Annotations rook.Annotations `json:"annotations,omitempty"`
}

// VersionSpec represents the settings for the Ozone version that Rook is orchestrating.
type OzoneVersionSpec struct {
	// Image is the container image used to launch the Ozone daemons, such as apache/ozone:0.5.0
	Image string `json:"image,omitempty"`
}
