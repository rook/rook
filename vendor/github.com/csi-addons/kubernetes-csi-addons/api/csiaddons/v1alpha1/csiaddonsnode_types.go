/*
Copyright 2021 The Kubernetes-CSI-Addons Authors.

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

// CSIAddonsNodeState defines the state of the operation.
type CSIAddonsNodeState string

const (
	// Connected represents the Connected state.
	CSIAddonsNodeStateConnected CSIAddonsNodeState = "Connected"

	// Failed represents the Connection Failed state.
	CSIAddonsNodeStateFailed CSIAddonsNodeState = "Failed"
)

type CSIAddonsNodeDriver struct {
	// Name is the name of the CSI driver that this object refers to.
	// This must be the same name returned by the CSI-Addons GetIdentity()
	// call for that driver. The name of the driver is in the format:
	// `example.csi.ceph.com`
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="name is immutable"
	Name string `json:"name"`

	// EndPoint is url that contains the ip-address to which the CSI-Addons
	// side-car listens to.
	EndPoint string `json:"endpoint"`

	// NodeID is the ID of the node to identify on which node the side-car
	// is running.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="nodeID is immutable"
	NodeID string `json:"nodeID"`
}

// CSIAddonsNodeSpec defines the desired state of CSIAddonsNode
type CSIAddonsNodeSpec struct {
	// Driver is the information of the CSI Driver existing on a node.
	// If the driver is uninstalled, this can become empty.
	Driver CSIAddonsNodeDriver `json:"driver"`
}

// CSIAddonsNodeStatus defines the observed state of CSIAddonsNode
type CSIAddonsNodeStatus struct {
	// State represents the state of the CSIAddonsNode object.
	// It informs whether or not the CSIAddonsNode is Connected
	// to the CSI Driver.
	State CSIAddonsNodeState `json:"state,omitempty"`

	// Message is a human-readable message indicating details about why the CSIAddonsNode
	// is in this state.
	// +optional
	Message string `json:"message,omitempty"`

	// Reason is a brief CamelCase string that describes any failure and is meant
	// for machine parsing and tidy display in the CLI.
	// +optional
	Reason string `json:"reason,omitempty"`

	// A list of capabilities advertised by the sidecar
	Capabilities []string `json:"capabilities,omitempty"`

	// NetworkFenceClientStatus contains the status of the clients required for fencing.
	NetworkFenceClientStatus []NetworkFenceClientStatus `json:"networkFenceClientStatus,omitempty"`
}

// NetworkFenceClientStatus contains the status of the clients required for fencing.
type NetworkFenceClientStatus struct {
	NetworkFenceClassName string         `json:"networkFenceClassName"`
	ClientDetails         []ClientDetail `json:"ClientDetails"`
}

// ClientDetail contains the details of the client required for fencing.
type ClientDetail struct {
	// Id is the unique identifier of the client where it belongs to.
	Id string `json:"id"`
	// Cidrs is the list of CIDR blocks that are fenced.
	Cidrs []string `json:"cidrs"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:JSONPath=".metadata.namespace",name=namespace,type=string
//+kubebuilder:printcolumn:JSONPath=".metadata.creationTimestamp",name=Age,type=date
//+kubebuilder:printcolumn:JSONPath=".spec.driver.name",name=DriverName,type=string
//+kubebuilder:printcolumn:JSONPath=".spec.driver.endpoint",name=Endpoint,type=string
//+kubebuilder:printcolumn:JSONPath=".spec.driver.nodeID",name=NodeID,type=string

// CSIAddonsNode is the Schema for the csiaddonsnode API
type CSIAddonsNode struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +kubebuilder:validation:Required
	Spec CSIAddonsNodeSpec `json:"spec"`

	Status CSIAddonsNodeStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CSIAddonsNodeList contains a list of CSIAddonsNode
type CSIAddonsNodeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CSIAddonsNode `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CSIAddonsNode{}, &CSIAddonsNodeList{})
}
