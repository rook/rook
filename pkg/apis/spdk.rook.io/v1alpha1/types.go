/*
Copyright 2020 The Rook Authors. All rights reserved.

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
	rookv1 "github.com/rook/rook/pkg/apis/rook.io/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type SpdkNode struct {
	// node to deploy spdk service, matches 'kubernetes.io/hostname' label
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// per node image
	Image string `json:"image,omitempty"`

	// per node hugepage
	HugePages *HugePages `json:"hugePages,omitempty"`

	// auto discover usable nvme devices
	NvmeAutoDiscover bool `json:"nvmeAutoDiscover,omitempty"`

	// pcie addresses of spdk managed nvme devices
	// have to alias string type due to kubebuilder limitation
	// https://github.com/kubernetes-sigs/controller-tools/issues/342
	NvmePcieAddrs []SpdkPcieAddr `json:"nvmePcieAddrs,omitempty"`

	// target configuration
	Target SpdkTarget `json:"target,omitempty"`
}

// pcie address format: domain:bus:slot.function
// "0000:01:00.0", or "01:00.0" with domain omitted
// +kubebuilder:validation:Pattern="^([0-9a-fA-F]{4}:)?[0-9a-fA-F]{2}:[0-9a-zA-F]{2}\\.[0-7]$"
type SpdkPcieAddr string

type HugePages struct {
	// +kubebuilder:validation:Required
	PageSize resource.Quantity `json:"pageSize"`
	// +kubebuilder:validation:Required
	MemSize resource.Quantity `json:"memSize"`
}

type SpdkTarget struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum="nvme-rdma";"nvme-tcp";"iscsi"
	Type string `json:"type"`

	// service address
	Addr string `json:"addr,omitempty"`

	// +kubebuilder:validation:Minimum=1024
	// +kubebuilder:validation:Maximum=65535
	Port int `json:"port,omitempty"`
}

type SpdkStatus string

const (
	SpdkStatusDeploySpdk SpdkStatus = "Deploy SPDK"
	SpdkStatusDeployCsi  SpdkStatus = "Deploy CSI"
	SpdkStatusRunning    SpdkStatus = "Running"
	SpdkStatusError      SpdkStatus = "Error"
)

// ClusterSpec defines the desired state of Cluster
type ClusterSpec struct {
	Annotations rookv1.Annotations `json:"annotations,omitempty"`

	// +kubebuilder:validation:Required
	Image string `json:"image"`

	// +kubebuilder:validation:Required
	HugePages *HugePages `json:"hugePages"`

	// +kubebuilder:validation:Required
	SpdkNodes []SpdkNode `json:"spdkNodes"`
}

// ClusterStatus defines the observed state of Cluster
type ClusterStatus struct {
	Status SpdkStatus `json:"status,omitempty"`
	Error  string     `json:"error,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="STATUS",type=string,JSONPath=`.status.status`

// Cluster is the Schema for the clusters API
type Cluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterSpec   `json:"spec,omitempty"`
	Status ClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterList contains a list of Cluster
type ClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Cluster `json:"items"`
}
