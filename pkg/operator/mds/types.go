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

// Package mds to manage a rook file system.
package mds

import (
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/pool"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// schemeGroupVersion is group version used to register these objects
var schemeGroupVersion = schema.GroupVersion{Group: k8sutil.CustomResourceGroup, Version: k8sutil.V1Alpha1}

// Filesystem is the definition of the file system custom resource
type Filesystem struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              FilesystemSpec `json:"spec"`
}

// FilesystemList is the definition of a list of file systems
type FilesystemList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []Filesystem `json:"items"`
}

// FilesystemSpec represent the spec of a file system
type FilesystemSpec struct {
	// The metadata pool settings
	MetadataPool pool.PoolSpec `json:"metadataPool"`

	// The data pool settings
	DataPools []pool.PoolSpec `json:"dataPools"`

	// The mds pod info
	MetadataServer MetadataServerSpec `json:"metadataServer"`
}

type MetadataServerSpec struct {
	// The number of metadata servers that are active. The remaining servers in the cluster will be in standby mode.
	ActiveCount int32 `json:"activeCount"`

	// Whether each active MDS instance will have an active standby with a warm metadata cache for faster failover.
	// If false, standbys will still be available, but will not have a warm metadata cache.
	ActiveStandby bool `json:"activeStandby"`

	// The affinity to place the mds pods (default is to place on all available node) with a daemonset
	Placement k8sutil.Placement `json:"placement"`
}
