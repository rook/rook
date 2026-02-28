// Copyright 2024 The prometheus-operator Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package v1

// PodDNSConfig defines the DNS parameters of a pod in addition to
// those generated from DNSPolicy.
type PodDNSConfig struct {
	// nameservers defines the list of DNS name server IP addresses.
	// This will be appended to the base nameservers generated from DNSPolicy.
	// +optional
	// +listType:=set
	// +kubebuilder:validation:items:MinLength:=1
	Nameservers []string `json:"nameservers,omitempty"`

	// searches defines the list of DNS search domains for host-name lookup.
	// This will be appended to the base search paths generated from DNSPolicy.
	// +optional
	// +listType:=set
	// +kubebuilder:validation:items:MinLength:=1
	Searches []string `json:"searches,omitempty"`

	// options defines the list of DNS resolver options.
	// This will be merged with the base options generated from DNSPolicy.
	// Resolution options given in Options
	// will override those that appear in the base DNSPolicy.
	// +optional
	// +listType=map
	// +listMapKey=name
	Options []PodDNSConfigOption `json:"options,omitempty"`
}

// PodDNSConfigOption defines DNS resolver options of a pod.
type PodDNSConfigOption struct {
	// name is required and must be unique.
	// +kubebuilder:validation:MinLength=1
	// +required
	Name string `json:"name"`

	// value is optional.
	// +optional
	Value *string `json:"value,omitempty"`
}

// DNSPolicy specifies the DNS policy for the pod.
// +kubebuilder:validation:Enum=ClusterFirstWithHostNet;ClusterFirst;Default;None
type DNSPolicy string

const (
	// DNSClusterFirstWithHostNet defines that the pod should use cluster DNS
	// first, if it is available, then fall back on the default
	// (as determined by kubelet) DNS settings.
	DNSClusterFirstWithHostNet DNSPolicy = "ClusterFirstWithHostNet"

	// DNSClusterFirst defines that the pod should use cluster DNS
	// first unless hostNetwork is true, if it is available, then
	// fall back on the default (as determined by kubelet) DNS settings.
	DNSClusterFirst DNSPolicy = "ClusterFirst"

	// DNSDefault defines that the pod should use the default (as
	// determined by kubelet) DNS settings.
	DNSDefault DNSPolicy = "Default"

	// DNSNone defines that the pod should use empty DNS settings. DNS
	// parameters such as nameservers and search paths should be defined via
	// DNSConfig.
	DNSNone DNSPolicy = "None"
)

const (
// DefaultTerminationGracePeriodSeconds indicates the default duration in
// seconds a pod needs to terminate gracefully.
)
