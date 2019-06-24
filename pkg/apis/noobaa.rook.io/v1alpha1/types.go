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

// ***************************************************************************
// IMPORTANT FOR CODE GENERATION
// If the types in this file are updated, you will need to run
// `make codegen` to generate the new types under the client/clientset folder.
// ***************************************************************************

import (
	"reflect"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// APIVersion is the TypeMeta.APIVersion for noobaa's CRDs
	APIVersion = CustomResourceGroup + "/" + Version
)

var (
	// NooBaaSystemKind is TypeMeta.Kind for NooBaaSystem
	NooBaaSystemKind = reflect.TypeOf((*NooBaaSystem)(nil)).Elem().Name()
	// NooBaaBackingStoreKind is TypeMeta.Kind for NooBaaBackingStore
	NooBaaBackingStoreKind = reflect.TypeOf((*NooBaaBackingStore)(nil)).Elem().Name()
	// NooBaaBucketClassKind is TypeMeta.Kind for NooBaaBucketClass
	NooBaaBucketClassKind = reflect.TypeOf((*NooBaaBucketClass)(nil)).Elem().Name()
)

//////////////////
// NooBaaSystem //
//////////////////

// NooBaaSystem is the custom resource describing a noobaa system
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type NooBaaSystem struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              SystemSpec   `json:"spec"`
	Status            SystemStatus `json:"status"`
}

// NooBaaSystemList is just a list of noobaa systems
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type NooBaaSystemList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []NooBaaSystem `json:"items"`
}

// SystemSpec is the spec of a noobaa system
type SystemSpec struct {
	// Image (optional) overrides the default image for server pods
	// +optional
	Image string `json:"image,omitempty"`
}

// SystemStatus is the status info of a noobaa system
type SystemStatus struct {
	Accounts SystemAccountsStatus `json:"accounts"`
	Services SystemServicesStatus `json:"services"`
	// Readme is a user readable string with explanations on the system
	Readme string `json:"readme"`
}

// SystemAccountsStatus is the status info of admin account
type SystemAccountsStatus struct {
	Admin SystemUserStatus `json:"admin"`
}

// SystemServicesStatus is the status info of the system's services
type SystemServicesStatus struct {
	ServiceMgmt SystemServiceStatus `json:"serviceMgmt"`
	ServiceS3   SystemServiceStatus `json:"serviceS3"`
}

// SystemUserStatus is the status info of a user secret
type SystemUserStatus struct {
	SecretRef corev1.SecretReference `json:"secretRef"`
}

// SystemServiceStatus is the status info and network addresses of a service
type SystemServiceStatus struct {
	// NodePorts are the most basic network available
	// it uses the networks available on the hosts of kubernetes nodes.
	// This generally works from within a pod, and from the internal
	// network of the nodes, but may fail from public network.
	// https://kubernetes.io/docs/concepts/services-networking/service/#nodeport
	// +optional
	NodePorts []string `json:"nodePorts,omitempty"`
	// PodPorts are the second most basic network address
	// every pod has an IP in the cluster and the pods network is a mesh
	// so the operator running inside a pod in the cluster can use this address.
	// Note: pod IPs are not guaranteed to persist over restarts, so should be rediscovered.
	// Note2: when running the operator outside of the cluster, pod IP is not accessible.
	// +optional
	PodPorts []string `json:"podPorts,omitempty"`
	// InternalIP and InternalDNS are internal addresses of the service inside the cluster
	// https://kubernetes.io/docs/concepts/services-networking/service/#publishing-services-service-types
	// +optional
	InternalIP []string `json:"internalIP,omitempty"`
	// +optional
	InternalDNS []string `json:"internalDNS,omitempty"`
	// External public addresses for the service
	// LoadBalancerPorts such as AWS ELB provide public address and load balancing for the service
	// IngressPorts are manually created public addresses for the service
	// https://kubernetes.io/docs/concepts/services-networking/service/#external-ips
	// https://kubernetes.io/docs/concepts/services-networking/service/#loadbalancer
	// https://kubernetes.io/docs/concepts/services-networking/ingress/
	// +optional
	ExternalIP []string `json:"externalIP,omitempty"`
	// +optional
	ExternalDNS []string `json:"externalDNS,omitempty"`
}

////////////////////////
// NooBaaBackingStore //
////////////////////////

// NooBaaBackingStore is the custom resource describing a noobaa backing store
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type NooBaaBackingStore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              BackingStoreSpec   `json:"spec"`
	Status            BackingStoreStatus `json:"status"`
}

// NooBaaBackingStoreList is just a list of backing stores
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type NooBaaBackingStoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []NooBaaBackingStore `json:"items"`
}

// BackingStoreSpec is the spec section of a backing store
type BackingStoreSpec struct {
	Type      BackingStoreType           `json:"type"`
	S3        *BackingStoreS3Spec        `json:"s3,omitempty"`
	Gcs       *BackingStoreGcsSpec       `json:"gcs,omitempty"`
	AzureBlob *BackingStoreAzureBlobSpec `json:"azureBlob,omitempty"`
	OBC       *BackingStoreOBCSpec       `json:"obc,omitempty"`
	PVC       *BackingStorePVCSpec       `json:"pvc,omitempty"`
}

// BackingStoreStatus is the status info of a backing store
type BackingStoreStatus struct {
	Health string `json:"health,omitempty"`
	Readme string `json:"readme,omitempty"`
}

// BackingStoreType is a type enum of the available types for backing stores
type BackingStoreType string

const (
	// BackingStoreTypeS3 is the type enum for S3 type backing store
	BackingStoreTypeS3 BackingStoreType = "S3"
	// BackingStoreTypeGcs is the type enum for Google Cloud Storage type backing store
	BackingStoreTypeGcs BackingStoreType = "GCS"
	// BackingStoreTypeAzureBlob is the type enum for Azure Blob type backing store
	BackingStoreTypeAzureBlob BackingStoreType = "AzureBlob"
	// BackingStoreTypeOBC is the type enum for OBC type backing store
	BackingStoreTypeOBC BackingStoreType = "OBC"
	// BackingStoreTypePVC is the type enum for PVC type backing store
	BackingStoreTypePVC BackingStoreType = "PVC"
)

// BackingStoreS3Spec is the spec for S3 type backing store
type BackingStoreS3Spec struct {
	Secret           corev1.SecretReference `json:"secret,omitempty"`
	Region           string                 `json:"region,omitempty"`
	EndpointURL      string                 `json:"endpointUrl,omitempty"`
	BucketName       string                 `json:"bucketName,omitempty"`
	SSLEnabled       bool                   `json:"sslEnabled,omitempty"`
	ForcePathStyle   bool                   `json:"forcePathStyle,omitempty"`
	SignatureVersion string                 `json:"signatureVersion,omitempty"`
}

// BackingStoreGcsSpec is the spec for Google Cloud Storage type backing store
type BackingStoreGcsSpec struct {
	Secret     corev1.SecretReference `json:"secret,omitempty"`
	BucketName string                 `json:"bucketName,omitempty"`
}

// BackingStoreAzureBlobSpec is the spec for Azure Blob type backing store
type BackingStoreAzureBlobSpec struct {
	Secret     corev1.SecretReference `json:"secret,omitempty"`
	BucketName string                 `json:"bucketName,omitempty"`
}

// BackingStoreOBCSpec is the spec for OBC type backing store
type BackingStoreOBCSpec struct {
	StorageClassName   string            `json:"storageClassName,omitempty"`
	BucketName         string            `json:"bucketName,omitempty"`
	GenerateBucketName string            `json:"generateBucketName,omitempty"`
	AdditionalConfig   map[string]string `json:"additionalConfig,omitempty"`
}

// BackingStorePVCSpec is the spec for PVC type backing store
type BackingStorePVCSpec struct {
	StorageClassName string `json:"storageClassName,omitempty"`
	Count            int32  `json:"count,omitempty"`
	Size             int64  `json:"size,omitempty"`
}

/////////////////////////
// NooBaa Bucket Class //
/////////////////////////

// NooBaaBucketClass is the custom resource describing a noobaa bucket class
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type NooBaaBucketClass struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
	Spec              BucketClassSpec   `json:"spec"`
	Status            BucketClassStatus `json:"status"`
}

// NooBaaBucketClassList is a list of bucket classes
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type NooBaaBucketClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []NooBaaBucketClass `json:"items"`
}

// BucketClassSpec is the spec section of a bucket class
type BucketClassSpec struct {
	TieringPolicy TieringPolicy `json:"tieringPolicy"`
}

// BucketClassStatus is the status info of a bucket class
type BucketClassStatus struct {
	Health  string `json:"health,omitempty"`
	Buckets int32  `json:"buckets,omitempty"`
	Readme  string `json:"readme,omitempty"`
}

// TieringPolicy is defining a placement policy for a bucket / bucket class
type TieringPolicy struct {
	Tiers []TierPolicy `json:"tiers"`
}

// TierPolicy is defining a placement policy for a tier
type TierPolicy struct {
	MirrorPolicy MirrorPolicy `json:"mirrorPolicy"`
}

// MirrorPolicy is defining a placement policy of mirroring data across stores
type MirrorPolicy struct {
	Mirrors []SpreadPolicy `json:"mirrors"`
}

// SpreadPolicy is defining a placement policy of spreading data across stores
type SpreadPolicy struct {
	Spread []BackingStoreRef `json:"spread"`
}

// BackingStoreRef is a named reference to a backing store
type BackingStoreRef struct {
	Name string `json:"name"`
}
