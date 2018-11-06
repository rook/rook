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
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rook "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
)

const (
	// ClusterDomainDefault is the default local cluster domain
	ClusterDomainDefault = "cluster.local"
)

// ***************************************************************************
// IMPORTANT FOR CODE GENERATION
// If the types in this file are updated, you will need to run
// `make codegen` to generate the new types under the client/clientset folder.
// ***************************************************************************

// +genclient
// +genclient:noStatus
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ObjectStore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              ObjectStoreSpec `json:"spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type ObjectStoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []ObjectStore `json:"items"`
}

// ObjectStoreSpec represent the spec of a Minio object store.
type ObjectStoreSpec struct {
	// A spec for available storage in the cluster and how it should be used
	Storage rook.StorageScopeSpec `json:"scope,omitempty"`

	// The placement-related configuration to pass to kubernetes (affinity, node selector,
	// tolerations).
	Placement rook.PlacementSpec `json:"placement,omitempty"`

	// Minio cluster credential configuration.
	Credentials v1.SecretReference `json:"credentials"`

	// The amount of storage that will be available in the object store.
	StorageSize string `json:"storageAmount"`

	// ClusterDomain is the local cluster domain for this cluster. This should be set if an
	// alternative cluster domain is in use.  If not set, then the default of cluster.local will
	// be assumed.
	//
	// This field is needed to workaround https://github.com/minio/minio/issues/6775, and is
	// expected to be removed in the future.
	ClusterDomain string `json:"clusterDomain,omitempty"`
}
