/*
Copyright 2017 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

Some of the code below came from https://github.com/coreos/etcd-operator
which also has the apache 2.0 license.
*/

// Package pool to manage a rook pool.
package pool

import (
	"github.com/rook/rook/pkg/operator/k8sutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// schemeGroupVersion is group version used to register these objects
var schemeGroupVersion = schema.GroupVersion{Group: k8sutil.CustomResourceGroup, Version: k8sutil.V1Alpha1}

// Pool is the definition of the pool custom resource
type Pool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              PoolSpec `json:"spec"`
}

// PoolList is the definition of a list of pools
type PoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []Pool `json:"items"`
}

// PoolSpec represent the spec of a pool
type PoolSpec struct {
	// The failure domain: osd, host, rack
	FailureDomain string `json:"failureDomain"`

	// The replication settings
	Replicated ReplicatedSpec `json:"replicated"`

	// The erasure code setteings
	ErasureCoded ErasureCodedSpec `json:"erasureCoded"`
}

// ReplicationSpec represents the spec for replication in a pool
type ReplicatedSpec struct {
	// Number of copies per object in a replicated storage pool, including the object itself (required for replicated pool type)
	Size uint `json:"size"`
}

// ErasureCodeSpec represents the spec for erasure code in a pool
type ErasureCodedSpec struct {
	// Number of coding chunks per object in an erasure coded storage pool (required for erasure-coded pool type)
	CodingChunks uint `json:"codingChunks"`

	// Number of data chunks per object in an erasure coded storage pool (required for erasure-coded pool type)
	DataChunks uint `json:"dataChunks"`

	// The algorithm for erasure coding
	Algorithm string `json:"algorithm"`
}
