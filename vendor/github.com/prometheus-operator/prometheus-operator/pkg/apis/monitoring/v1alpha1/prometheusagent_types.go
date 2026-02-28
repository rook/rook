// Copyright 2023 The prometheus-operator Authors
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

package v1alpha1

import (
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	PrometheusAgentsKind   = "PrometheusAgent"
	PrometheusAgentName    = "prometheusagents"
	PrometheusAgentKindKey = "prometheusagent"
)

func (l *PrometheusAgent) GetCommonPrometheusFields() monitoringv1.CommonPrometheusFields {
	return l.Spec.CommonPrometheusFields
}

func (l *PrometheusAgent) SetCommonPrometheusFields(f monitoringv1.CommonPrometheusFields) {
	l.Spec.CommonPrometheusFields = f
}

func (l *PrometheusAgent) GetStatus() monitoringv1.PrometheusStatus {
	return l.Status
}

// +genclient
// +k8s:openapi-gen=true
// +kubebuilder:resource:categories="prometheus-operator",shortName="promagent"
// +kubebuilder:printcolumn:name="Version",type="string",JSONPath=".spec.version",description="The version of Prometheus agent"
// +kubebuilder:printcolumn:name="Desired",type="integer",JSONPath=".spec.replicas",description="The number of desired replicas"
// +kubebuilder:printcolumn:name="Ready",type="integer",JSONPath=".status.availableReplicas",description="The number of ready replicas"
// +kubebuilder:printcolumn:name="Reconciled",type="string",JSONPath=".status.conditions[?(@.type == 'Reconciled')].status"
// +kubebuilder:printcolumn:name="Available",type="string",JSONPath=".status.conditions[?(@.type == 'Available')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="Paused",type="boolean",JSONPath=".status.paused",description="Whether the resource reconciliation is paused or not",priority=1
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specpath=.spec.shards,statuspath=.status.shards,selectorpath=.status.selector
// +genclient:method=GetScale,verb=get,subresource=scale,result=k8s.io/api/autoscaling/v1.Scale
// +genclient:method=UpdateScale,verb=update,subresource=scale,input=k8s.io/api/autoscaling/v1.Scale,result=k8s.io/api/autoscaling/v1.Scale

// The `PrometheusAgent` custom resource definition (CRD) defines a desired [Prometheus Agent](https://prometheus.io/blog/2021/11/16/agent/) setup to run in a Kubernetes cluster.
//
// The CRD is very similar to the `Prometheus` CRD except for features which aren't available in agent mode like rule evaluation, persistent storage and Thanos sidecar.
type PrometheusAgent struct {
	// TypeMeta defines the versioned schema of this representation of an object.
	metav1.TypeMeta `json:",inline"`
	// metadata defines ObjectMeta as the metadata that all persisted resources.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// spec defines the specification of the desired behavior of the Prometheus agent. More info:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	// +required
	Spec PrometheusAgentSpec `json:"spec"`
	// status defines the most recent observed status of the Prometheus cluster. Read-only.
	// More info:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	// +optional
	Status monitoringv1.PrometheusStatus `json:"status,omitempty"`
}

// DeepCopyObject implements the runtime.Object interface.
func (l *PrometheusAgent) DeepCopyObject() runtime.Object {
	return l.DeepCopy()
}

// PrometheusAgentList is a list of Prometheus agents.
// +k8s:openapi-gen=true
type PrometheusAgentList struct {
	// TypeMeta defines the versioned schema of this representation of an object.
	metav1.TypeMeta `json:",inline"`
	// metadata defines ListMeta as metadata for collection responses.
	metav1.ListMeta `json:"metadata,omitempty"`
	// List of Prometheus agents
	Items []PrometheusAgent `json:"items"`
}

// DeepCopyObject implements the runtime.Object interface.
func (l *PrometheusAgentList) DeepCopyObject() runtime.Object {
	return l.DeepCopy()
}

// PrometheusAgentSpec is a specification of the desired behavior of the Prometheus agent. More info:
// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
// +k8s:openapi-gen=true
// +kubebuilder:validation:XValidation:rule="!(has(self.mode) && self.mode == 'DaemonSet' && has(self.replicas))",message="replicas cannot be set when mode is DaemonSet"
// +kubebuilder:validation:XValidation:rule="!(has(self.mode) && self.mode == 'DaemonSet' && has(self.storage))",message="storage cannot be set when mode is DaemonSet"
// +kubebuilder:validation:XValidation:rule="!(has(self.mode) && self.mode == 'DaemonSet' && has(self.shards) && self.shards > 1)",message="shards cannot be greater than 1 when mode is DaemonSet"
// +kubebuilder:validation:XValidation:rule="!(has(self.mode) && self.mode == 'DaemonSet' && has(self.persistentVolumeClaimRetentionPolicy))",message="persistentVolumeClaimRetentionPolicy cannot be set when mode is DaemonSet"
// +kubebuilder:validation:XValidation:rule="!(has(self.mode) && self.mode == 'DaemonSet' && has(self.scrapeConfigSelector))",message="scrapeConfigSelector cannot be set when mode is DaemonSet"
// +kubebuilder:validation:XValidation:rule="!(has(self.mode) && self.mode == 'DaemonSet' && has(self.probeSelector))",message="probeSelector cannot be set when mode is DaemonSet"
type PrometheusAgentSpec struct {
	// mode defines how the Prometheus operator deploys the PrometheusAgent pod(s).
	//
	// (Alpha) Using this field requires the `PrometheusAgentDaemonSet` feature gate to be enabled.
	//
	// +optional
	Mode *PrometheusAgentMode `json:"mode,omitempty"`

	monitoringv1.CommonPrometheusFields `json:",inline"`
}

// +kubebuilder:validation:Enum=StatefulSet;DaemonSet
type PrometheusAgentMode string

const (
	// Deploys PrometheusAgent as DaemonSet.
	DaemonSetPrometheusAgentMode PrometheusAgentMode = "DaemonSet"

	// Deploys PrometheusAgent as StatefulSet.
	StatefulSetPrometheusAgentMode PrometheusAgentMode = "StatefulSet"
)
