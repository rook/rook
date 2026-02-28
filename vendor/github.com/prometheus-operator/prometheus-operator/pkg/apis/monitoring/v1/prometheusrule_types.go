// Copyright 2018 The prometheus-operator Authors
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

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	PrometheusRuleKind    = "PrometheusRule"
	PrometheusRuleName    = "prometheusrules"
	PrometheusRuleKindKey = "prometheusrule"
)

// +genclient
// +k8s:openapi-gen=true
// +kubebuilder:resource:categories="prometheus-operator",shortName="promrule"
// +kubebuilder:subresource:status

// The `PrometheusRule` custom resource definition (CRD) defines [alerting](https://prometheus.io/docs/prometheus/latest/configuration/alerting_rules/) and [recording](https://prometheus.io/docs/prometheus/latest/configuration/recording_rules/) rules to be evaluated by `Prometheus` or `ThanosRuler` objects.
//
// `Prometheus` and `ThanosRuler` objects select `PrometheusRule` objects using label and namespace selectors.
type PrometheusRule struct {
	// TypeMeta defines the versioned schema of this representation of an object.
	metav1.TypeMeta `json:",inline"`
	// metadata defines ObjectMeta as the metadata that all persisted resources.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// spec defines the specification of desired alerting rule definitions for Prometheus.
	// +required
	Spec PrometheusRuleSpec `json:"spec"`
	// status defines the status subresource. It is under active development and is updated only when the
	// "StatusForConfigurationResources" feature gate is enabled.
	//
	// Most recent observed status of the PrometheusRule. Read-only.
	// More info:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	// +optional
	Status ConfigResourceStatus `json:"status,omitempty,omitzero"`
}

// DeepCopyObject implements the runtime.Object interface.
func (f *PrometheusRule) DeepCopyObject() runtime.Object {
	return f.DeepCopy()
}

func (f *PrometheusRule) Bindings() []WorkloadBinding {
	return f.Status.Bindings
}

// PrometheusRuleSpec contains specification parameters for a Rule.
// +k8s:openapi-gen=true
type PrometheusRuleSpec struct {
	// groups defines the content of Prometheus rule file
	// +listType=map
	// +listMapKey=name
	// +optional
	Groups []RuleGroup `json:"groups,omitempty"`
}

// RuleGroup and Rule are copied instead of vendored because the
// upstream Prometheus struct definitions don't have json struct tags.

// RuleGroup is a list of sequentially evaluated recording and alerting rules.
// +k8s:openapi-gen=true
type RuleGroup struct {
	// name defines the name of the rule group.
	// +kubebuilder:validation:MinLength=1
	// +required
	Name string `json:"name"`
	// labels define the labels to add or overwrite before storing the result for its rules.
	// The labels defined at the rule level take precedence.
	//
	// It requires Prometheus >= 3.0.0.
	// The field is ignored for Thanos Ruler.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
	// interval defines how often rules in the group are evaluated.
	// +optional
	Interval *Duration `json:"interval,omitempty"`
	// query_offset defines the offset the rule evaluation timestamp of this particular group by the specified duration into the past.
	//
	// It requires Prometheus >= v2.53.0.
	// It is not supported for ThanosRuler.
	// +optional
	//nolint:kubeapilinter // The json tag doesn't meet the conventions to be compatible with Prometheus format.
	QueryOffset *Duration `json:"query_offset,omitempty"`
	// rules defines the list of alerting and recording rules.
	// +optional
	Rules []Rule `json:"rules,omitempty"`
	// partial_response_strategy is only used by ThanosRuler and will
	// be ignored by Prometheus instances.
	// More info: https://github.com/thanos-io/thanos/blob/main/docs/components/rule.md#partial-response
	// +kubebuilder:validation:Pattern="^(?i)(abort|warn)?$"
	// +optional
	//nolint:kubeapilinter // The json tag doesn't meet the conventions to be compatible with Prometheus format.
	PartialResponseStrategy string `json:"partial_response_strategy,omitempty"`
	// limit defines the number of alerts an alerting rule and series a recording
	// rule can produce.
	// Limit is supported starting with Prometheus >= 2.31 and Thanos Ruler >= 0.24.
	// +optional
	Limit *int `json:"limit,omitempty"`
}

// Rule describes an alerting or recording rule
// See Prometheus documentation: [alerting](https://www.prometheus.io/docs/prometheus/latest/configuration/alerting_rules/) or [recording](https://www.prometheus.io/docs/prometheus/latest/configuration/recording_rules/#recording-rules) rule
// +k8s:openapi-gen=true
// +kubebuilder:validation:OneOf=Record,Alert
type Rule struct {
	// record defines the name of the time series to output to. Must be a valid metric name.
	// Only one of `record` and `alert` must be set.
	// +optional
	Record string `json:"record,omitempty"`
	// alert defines the name of the alert. Must be a valid label value.
	// Only one of `record` and `alert` must be set.
	// +optional
	Alert string `json:"alert,omitempty"`
	// expr defines the PromQL expression to evaluate.
	// +required
	Expr intstr.IntOrString `json:"expr"`
	// for defines how alerts are considered firing once they have been returned for this long.
	// +optional
	For *Duration `json:"for,omitempty"`
	// keep_firing_for defines how long an alert will continue firing after the condition that triggered it has cleared.
	// +optional
	//nolint:kubeapilinter // The json tag doesn't meet the conventions to be compatible with Prometheus format.
	KeepFiringFor *NonEmptyDuration `json:"keep_firing_for,omitempty"`
	// labels defines labels to add or overwrite.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
	// annotations defines annotations to add to each alert.
	// Only valid for alerting rules.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// PrometheusRuleList is a list of PrometheusRules.
// +k8s:openapi-gen=true
type PrometheusRuleList struct {
	// TypeMeta defines the versioned schema of this representation of an object.
	metav1.TypeMeta `json:",inline"`
	// metadata defines ListMeta as metadata for collection responses.
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`
	// List of Rules
	// +required
	Items []PrometheusRule `json:"items"`
}

// DeepCopyObject implements the runtime.Object interface.
func (l *PrometheusRuleList) DeepCopyObject() runtime.Object {
	return l.DeepCopy()
}
