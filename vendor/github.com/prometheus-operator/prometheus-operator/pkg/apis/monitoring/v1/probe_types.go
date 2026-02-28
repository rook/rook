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
	"errors"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	ProbesKind   = "Probe"
	ProbeName    = "probes"
	ProbeKindKey = "probe"
)

// +genclient
// +k8s:openapi-gen=true
// +kubebuilder:resource:categories="prometheus-operator",shortName="prb"
// +kubebuilder:subresource:status

// The `Probe` custom resource definition (CRD) defines how to scrape metrics from prober exporters such as the [blackbox exporter](https://github.com/prometheus/blackbox_exporter).
//
// The `Probe` resource needs 2 pieces of information:
// * The list of probed addresses which can be defined statically or by discovering Kubernetes Ingress objects.
// * The prober which exposes the availability of probed endpoints (over various protocols such HTTP, TCP, ICMP, ...) as Prometheus metrics.
//
// `Prometheus` and `PrometheusAgent` objects select `Probe` objects using label and namespace selectors.
type Probe struct {
	// TypeMeta defines the versioned schema of this representation of an object.
	// +optional
	metav1.TypeMeta `json:",inline"`
	// metadata defines ObjectMeta as the metadata that all persisted resources.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// spec defines the specification of desired Ingress selection for target discovery by Prometheus.
	// +required
	Spec ProbeSpec `json:"spec"`
	// status defines the status subresource. It is under active development and is updated only when the
	// "StatusForConfigurationResources" feature gate is enabled.
	//
	// Most recent observed status of the Probe. Read-only.
	// More info:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	// +optional
	Status ConfigResourceStatus `json:"status,omitempty,omitzero"`
}

// DeepCopyObject implements the runtime.Object interface.
func (l *Probe) DeepCopyObject() runtime.Object {
	return l.DeepCopy()
}

func (l *Probe) Bindings() []WorkloadBinding {
	return l.Status.Bindings
}

// ProbeSpec contains specification parameters for a Probe.
// +k8s:openapi-gen=true
type ProbeSpec struct {
	// jobName assigned to scraped metrics by default.
	// +optional
	JobName string `json:"jobName,omitempty"`
	// prober defines the specification for the prober to use for probing targets.
	// The prober.URL parameter is required. Targets cannot be probed if left empty.
	// +optional
	ProberSpec ProberSpec `json:"prober,omitempty"`
	// module to use for probing specifying how to probe the target.
	// Example module configuring in the blackbox exporter:
	// https://github.com/prometheus/blackbox_exporter/blob/master/example.yml
	// +optional
	Module string `json:"module,omitempty"`
	// targets defines a set of static or dynamically discovered targets to probe.
	// +optional
	Targets ProbeTargets `json:"targets,omitempty"`
	// interval at which targets are probed using the configured prober.
	// If not specified Prometheus' global scrape interval is used.
	// +optional
	Interval Duration `json:"interval,omitempty"`
	// scrapeTimeout defines the timeout for scraping metrics from the Prometheus exporter.
	// If not specified, the Prometheus global scrape timeout is used.
	// The value cannot be greater than the scrape interval otherwise the operator will reject the resource.
	// +optional
	ScrapeTimeout Duration `json:"scrapeTimeout,omitempty"`
	// tlsConfig defines the TLS configuration to use when scraping the endpoint.
	// +optional
	TLSConfig *SafeTLSConfig `json:"tlsConfig,omitempty"`
	// bearerTokenSecret defines the secret to mount to read bearer token for scraping targets. The secret
	// needs to be in the same namespace as the probe and accessible by
	// the Prometheus Operator.
	// +optional
	BearerTokenSecret v1.SecretKeySelector `json:"bearerTokenSecret,omitempty"`
	// basicAuth allow an endpoint to authenticate over basic authentication.
	// More info: https://prometheus.io/docs/operating/configuration/#endpoint
	// +optional
	BasicAuth *BasicAuth `json:"basicAuth,omitempty"`
	// oauth2 for the URL. Only valid in Prometheus versions 2.27.0 and newer.
	// +optional
	OAuth2 *OAuth2 `json:"oauth2,omitempty"`
	// metricRelabelings defines the RelabelConfig to apply to samples before ingestion.
	// +optional
	MetricRelabelConfigs []RelabelConfig `json:"metricRelabelings,omitempty"`
	// authorization section for this endpoint
	// +optional
	Authorization *SafeAuthorization `json:"authorization,omitempty"`
	// sampleLimit defines per-scrape limit on number of scraped samples that will be accepted.
	// +optional
	SampleLimit *uint64 `json:"sampleLimit,omitempty"`
	// targetLimit defines a limit on the number of scraped targets that will be accepted.
	// +optional
	TargetLimit *uint64 `json:"targetLimit,omitempty"`
	// scrapeProtocols defines the protocols to negotiate during a scrape. It tells clients the
	// protocols supported by Prometheus in order of preference (from most to least preferred).
	//
	// If unset, Prometheus uses its default value.
	//
	// It requires Prometheus >= v2.49.0.
	//
	// +listType=set
	// +optional
	ScrapeProtocols []ScrapeProtocol `json:"scrapeProtocols,omitempty"`
	// fallbackScrapeProtocol defines the protocol to use if a scrape returns blank, unparseable, or otherwise invalid Content-Type.
	//
	// It requires Prometheus >= v3.0.0.
	// +optional
	FallbackScrapeProtocol *ScrapeProtocol `json:"fallbackScrapeProtocol,omitempty"`
	// labelLimit defines the per-scrape limit on number of labels that will be accepted for a sample.
	// Only valid in Prometheus versions 2.27.0 and newer.
	// +optional
	LabelLimit *uint64 `json:"labelLimit,omitempty"`
	// labelNameLengthLimit defines the per-scrape limit on length of labels name that will be accepted for a sample.
	// Only valid in Prometheus versions 2.27.0 and newer.
	// +optional
	LabelNameLengthLimit *uint64 `json:"labelNameLengthLimit,omitempty"`
	// labelValueLengthLimit defines the per-scrape limit on length of labels value that will be accepted for a sample.
	// Only valid in Prometheus versions 2.27.0 and newer.
	// +optional
	LabelValueLengthLimit *uint64 `json:"labelValueLengthLimit,omitempty"`

	// +optional
	NativeHistogramConfig `json:",inline"`
	// keepDroppedTargets defines the per-scrape limit on the number of targets dropped by relabeling
	// that will be kept in memory. 0 means no limit.
	//
	// It requires Prometheus >= v2.47.0.
	//
	// +optional
	KeepDroppedTargets *uint64 `json:"keepDroppedTargets,omitempty"`

	// scrapeClass defines the scrape class to apply.
	// +optional
	// +kubebuilder:validation:MinLength=1
	ScrapeClassName *string `json:"scrapeClass,omitempty"`

	// params defines the list of HTTP query parameters for the scrape.
	// Please note that the `.spec.module` field takes precedence over the `module` parameter from this list when both are defined.
	// The module name must be added using Module under ProbeSpec.
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +listType=map
	// +listMapKey=name
	Params []ProbeParam `json:"params,omitempty"`
}

// ProbeParam defines specification of extra parameters for a Probe.
// +k8s:openapi-gen=true
type ProbeParam struct {
	// name defines the parameter name
	// +kubebuilder:validation:MinLength=1
	// +required
	Name string `json:"name,omitempty"`
	// values defines the parameter values
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:items:MinLength=1
	// +optional
	Values []string `json:"values,omitempty"`
}

// ProbeTargets defines how to discover the probed targets.
// One of the `staticConfig` or `ingress` must be defined.
// If both are defined, `staticConfig` takes precedence.
// +k8s:openapi-gen=true
type ProbeTargets struct {
	// staticConfig defines the static list of targets to probe and the
	// relabeling configuration.
	// If `ingress` is also defined, `staticConfig` takes precedence.
	// More info: https://prometheus.io/docs/prometheus/latest/configuration/configuration/#static_config.
	// +optional
	StaticConfig *ProbeTargetStaticConfig `json:"staticConfig,omitempty"`
	// ingress defines the Ingress objects to probe and the relabeling
	// configuration.
	// If `staticConfig` is also defined, `staticConfig` takes precedence.
	// +optional
	Ingress *ProbeTargetIngress `json:"ingress,omitempty"`
}

// Validate semantically validates the given ProbeTargets.
func (it *ProbeTargets) Validate() error {
	if it.StaticConfig == nil && it.Ingress == nil {
		return errors.New("at least one of .spec.targets.staticConfig and .spec.targets.ingress is required")
	}

	return nil
}

// ProbeTargetStaticConfig defines the set of static targets considered for probing.
// +k8s:openapi-gen=true
type ProbeTargetStaticConfig struct {
	// static defines the list of hosts to probe.
	// +optional
	Targets []string `json:"static,omitempty"`
	// labels defines all labels assigned to all metrics scraped from the targets.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
	// relabelingConfigs defines relabelings to be apply to the label set of the targets before it gets
	// scraped.
	// More info: https://prometheus.io/docs/prometheus/latest/configuration/configuration/#relabel_config
	// +optional
	RelabelConfigs []RelabelConfig `json:"relabelingConfigs,omitempty"`
}

// ProbeTargetIngress defines the set of Ingress objects considered for probing.
// The operator configures a target for each host/path combination of each ingress object.
// +k8s:openapi-gen=true
type ProbeTargetIngress struct {
	// selector to select the Ingress objects.
	// +optional
	Selector metav1.LabelSelector `json:"selector,omitempty"`
	// namespaceSelector defines from which namespaces to select Ingress objects.
	// +optional
	NamespaceSelector NamespaceSelector `json:"namespaceSelector,omitempty"`
	// relabelingConfigs to apply to the label set of the target before it gets
	// scraped.
	// The original ingress address is available via the
	// `__tmp_prometheus_ingress_address` label. It can be used to customize the
	// probed URL.
	// The original scrape job's name is available via the `__tmp_prometheus_job_name` label.
	// More info: https://prometheus.io/docs/prometheus/latest/configuration/configuration/#relabel_config
	// +optional
	RelabelConfigs []RelabelConfig `json:"relabelingConfigs,omitempty"`
}

// ProberSpec contains specification parameters for the Prober used for probing.
// +k8s:openapi-gen=true
type ProberSpec struct {
	// url defines the address of the prober.
	//
	// Unlike what the name indicates, the value should be in the form of
	// `address:port` without any scheme which should be specified in the
	// `scheme` field.
	//
	// +kubebuilder:validation:MinLength=1
	// +required
	URL string `json:"url"`

	// scheme defines the HTTP scheme to use when scraping the prober.
	//
	// +optional
	Scheme *Scheme `json:"scheme,omitempty"`

	// path to collect metrics from.
	// Defaults to `/probe`.
	//
	// +kubebuilder:default:="/probe"
	// +optional
	Path string `json:"path,omitempty"`

	// +optional
	ProxyConfig `json:",inline"`
}

// ProbeList is a list of Probes.
// +k8s:openapi-gen=true
type ProbeList struct {
	// TypeMeta defines the versioned schema of this representation of an object.
	// +optional
	metav1.TypeMeta `json:",inline"`
	// metadata defines ListMeta as metadata for collection responses.
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`
	// List of Probes
	// +required
	Items []Probe `json:"items"`
}

// DeepCopyObject implements the runtime.Object interface.
func (l *ProbeList) DeepCopyObject() runtime.Object {
	return l.DeepCopy()
}
