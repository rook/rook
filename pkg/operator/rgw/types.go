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

// Package rgw to manage a rook object store.
package rgw

import (
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/pool"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// schemeGroupVersion is group version used to register these objects
var schemeGroupVersion = schema.GroupVersion{Group: k8sutil.CustomResourceGroup, Version: k8sutil.V1Alpha1}

// ObjectStore is the definition of the pool custom resource
type ObjectStore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              ObjectStoreSpec `json:"spec"`
}

// ObjectStoreList is the definition of a list of pools
type ObjectStoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []ObjectStore `json:"items"`
}

// ObjectStoreSpec represent the spec of a pool
type ObjectStoreSpec struct {
	// The metadata pool settings
	MetadataPool pool.PoolSpec `json:"metadataPool"`

	// The data pool settings
	DataPool pool.PoolSpec `json:"dataPool"`

	// The rgw pod info
	Gateway GatewaySpec `json:"gateway"`
}

type GatewaySpec struct {
	// The port the rgw service will be listening on (http)
	Port int32 `json:"port"`

	// The port the rgw service will be listening on (https)
	SecurePort int32 `json:"securePort"`

	// The number of pods in the rgw replicaset. If "allNodes" is specified, a daemonset is created.
	Instances int32 `json:"instances"`

	// Whether the rgw pods should be started as a daemonset on all nodes
	AllNodes bool `json:"allNodes"`

	// The name of the secret that stores the ssl certificate for secure rgw connections
	SSLCertificateRef string `json:"sslCertificateRef"`

	// The affinity to place the rgw pods (default is to place on any available node)
	Placement k8sutil.Placement `json:"placement"`
}
