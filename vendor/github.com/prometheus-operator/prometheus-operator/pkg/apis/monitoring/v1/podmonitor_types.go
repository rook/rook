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
	PodMonitorsKind   = "PodMonitor"
	PodMonitorName    = "podmonitors"
	PodMonitorKindKey = "podmonitor"
)

// +genclient
// +k8s:openapi-gen=true
// +kubebuilder:resource:categories="prometheus-operator",shortName="pmon"
// +kubebuilder:subresource:status

// The `PodMonitor` custom resource definition (CRD) defines how `Prometheus` and `PrometheusAgent` can scrape metrics from a group of pods.
// Among other things, it allows to specify:
// * The pods to scrape via label selectors.
// * The container ports to scrape.
// * Authentication credentials to use.
// * Target and metric relabeling.
//
// `Prometheus` and `PrometheusAgent` objects select `PodMonitor` objects using label and namespace selectors.
type PodMonitor struct {
	// TypeMeta defines the versioned schema of this representation of an object.
	// +optional
	metav1.TypeMeta `json:",inline"`
	// metadata defines ObjectMeta as the metadata that all persisted resources.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// spec defines the specification of desired Pod selection for target discovery by Prometheus.
	// +required
	Spec PodMonitorSpec `json:"spec"`
	// status defines the status subresource. It is under active development and is updated only when the
	// "StatusForConfigurationResources" feature gate is enabled.
	//
	// Most recent observed status of the PodMonitor. Read-only.
	// More info:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	// +optional
	Status ConfigResourceStatus `json:"status,omitempty,omitzero"`
}

// DeepCopyObject implements the runtime.Object interface.
func (l *PodMonitor) DeepCopyObject() runtime.Object {
	return l.DeepCopy()
}

func (l *PodMonitor) Bindings() []WorkloadBinding {
	return l.Status.Bindings
}

// PodMonitorSpec contains specification parameters for a PodMonitor.
// +k8s:openapi-gen=true
type PodMonitorSpec struct {
	// jobLabel defines the label to use to retrieve the job name from.
	// `jobLabel` selects the label from the associated Kubernetes `Pod`
	// object which will be used as the `job` label for all metrics.
	//
	// For example if `jobLabel` is set to `foo` and the Kubernetes `Pod`
	// object is labeled with `foo: bar`, then Prometheus adds the `job="bar"`
	// label to all ingested metrics.
	//
	// If the value of this field is empty, the `job` label of the metrics
	// defaults to the namespace and name of the PodMonitor object (e.g. `<namespace>/<name>`).
	// +optional
	JobLabel string `json:"jobLabel,omitempty"`

	// podTargetLabels defines the labels which are transferred from the
	// associated Kubernetes `Pod` object onto the ingested metrics.
	//
	// +optional
	PodTargetLabels []string `json:"podTargetLabels,omitempty"`

	// podMetricsEndpoints defines how to scrape metrics from the selected pods.
	//
	// +optional
	PodMetricsEndpoints []PodMetricsEndpoint `json:"podMetricsEndpoints"`

	// selector defines the label selector to select the Kubernetes `Pod` objects to scrape metrics from.
	// +required
	Selector metav1.LabelSelector `json:"selector"`

	// selectorMechanism defines the mechanism used to select the endpoints to scrape.
	// By default, the selection process relies on relabel configurations to filter the discovered targets.
	// Alternatively, you can opt in for role selectors, which may offer better efficiency in large clusters.
	// Which strategy is best for your use case needs to be carefully evaluated.
	//
	// It requires Prometheus >= v2.17.0.
	//
	// +optional
	SelectorMechanism *SelectorMechanism `json:"selectorMechanism,omitempty"`

	// namespaceSelector defines in which namespace(s) Prometheus should discover the pods.
	// By default, the pods are discovered in the same namespace as the `PodMonitor` object but it is possible to select pods across different/all namespaces.
	// +optional
	NamespaceSelector NamespaceSelector `json:"namespaceSelector,omitempty"`

	// sampleLimit defines a per-scrape limit on the number of scraped samples
	// that will be accepted.
	//
	// +optional
	SampleLimit *uint64 `json:"sampleLimit,omitempty"`

	// targetLimit defines a limit on the number of scraped targets that will
	// be accepted.
	//
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
	//
	// It requires Prometheus >= v2.27.0.
	//
	// +optional
	LabelLimit *uint64 `json:"labelLimit,omitempty"`
	// labelNameLengthLimit defines the per-scrape limit on length of labels name that will be accepted for a sample.
	//
	// It requires Prometheus >= v2.27.0.
	//
	// +optional
	LabelNameLengthLimit *uint64 `json:"labelNameLengthLimit,omitempty"`
	// labelValueLengthLimit defines the per-scrape limit on length of labels value that will be accepted for a sample.
	//
	// It requires Prometheus >= v2.27.0.
	//
	// +optional
	LabelValueLengthLimit *uint64 `json:"labelValueLengthLimit,omitempty"`

	NativeHistogramConfig `json:",inline"`

	// keepDroppedTargets defines the per-scrape limit on the number of targets dropped by relabeling
	// that will be kept in memory. 0 means no limit.
	//
	// It requires Prometheus >= v2.47.0.
	//
	// +optional
	KeepDroppedTargets *uint64 `json:"keepDroppedTargets,omitempty"`

	// attachMetadata defines additional metadata which is added to the
	// discovered targets.
	//
	// It requires Prometheus >= v2.35.0.
	//
	// +optional
	AttachMetadata *AttachMetadata `json:"attachMetadata,omitempty"`

	// scrapeClass defines the scrape class to apply.
	// +optional
	// +kubebuilder:validation:MinLength=1
	ScrapeClassName *string `json:"scrapeClass,omitempty"`

	// bodySizeLimit when defined specifies a job level limit on the size
	// of uncompressed response body that will be accepted by Prometheus.
	//
	// It requires Prometheus >= v2.28.0.
	//
	// +optional
	BodySizeLimit *ByteSize `json:"bodySizeLimit,omitempty"`
}

// PodMonitorList is a list of PodMonitors.
// +k8s:openapi-gen=true
type PodMonitorList struct {
	// TypeMeta defines the versioned schema of this representation of an object.
	// +optional
	metav1.TypeMeta `json:",inline"`
	// metadata defines ListMeta as metadata for collection responses.
	metav1.ListMeta `json:"metadata,omitempty"`
	// List of PodMonitors
	Items []PodMonitor `json:"items"`
}

// DeepCopyObject implements the runtime.Object interface.
func (l *PodMonitorList) DeepCopyObject() runtime.Object {
	return l.DeepCopy()
}

// PodMetricsEndpoint defines an endpoint serving Prometheus metrics to be scraped by
// Prometheus.
//
// +k8s:openapi-gen=true
type PodMetricsEndpoint struct {
	// port defines the `Pod` port name which exposes the endpoint.
	//
	// If the pod doesn't expose a port with the same name, it will result
	// in no targets being discovered.
	//
	// If a `Pod` has multiple `Port`s with the same name (which is not
	// recommended), one target instance per unique port number will be
	// generated.
	//
	// It takes precedence over the `portNumber` and `targetPort` fields.
	// +optional
	Port *string `json:"port,omitempty"`

	// portNumber defines the `Pod` port number which exposes the endpoint.
	//
	// The `Pod` must declare the specified `Port` in its spec or the
	// target will be dropped by Prometheus.
	//
	// This cannot be used to enable scraping of an undeclared port.
	// To scrape targets on a port which isn't exposed, you need to use
	// relabeling to override the `__address__` label (but beware of
	// duplicate targets if the `Pod` has other declared ports).
	//
	// In practice Prometheus will select targets for which the
	// matches the target's __meta_kubernetes_pod_container_port_number.
	//
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +optional
	PortNumber *int32 `json:"portNumber,omitempty"`

	// targetPort defines the name or number of the target port of the `Pod` object behind the Service, the
	// port must be specified with container port property.
	//
	// Deprecated: use 'port' or 'portNumber' instead.
	// +optional
	TargetPort *intstr.IntOrString `json:"targetPort,omitempty"`

	// path defines the HTTP path from which to scrape for metrics.
	//
	// If empty, Prometheus uses the default value (e.g. `/metrics`).
	// +optional
	Path string `json:"path,omitempty"`

	// scheme defines the HTTP scheme to use for scraping.
	//
	// +optional
	Scheme *Scheme `json:"scheme,omitempty"`

	// params define optional HTTP URL parameters.
	// +optional
	Params map[string][]string `json:"params,omitempty"`

	// interval at which Prometheus scrapes the metrics from the target.
	//
	// If empty, Prometheus uses the global scrape interval.
	// +optional
	Interval Duration `json:"interval,omitempty"`

	// scrapeTimeout defines the timeout after which Prometheus considers the scrape to be failed.
	//
	// If empty, Prometheus uses the global scrape timeout unless it is less
	// than the target's scrape interval value in which the latter is used.
	// The value cannot be greater than the scrape interval otherwise the operator will reject the resource.
	// +optional
	ScrapeTimeout Duration `json:"scrapeTimeout,omitempty"`

	// honorLabels when true preserves the metric's labels when they collide
	// with the target's labels.
	// +optional
	HonorLabels bool `json:"honorLabels,omitempty"`

	// honorTimestamps defines whether Prometheus preserves the timestamps
	// when exposed by the target.
	//
	// +optional
	HonorTimestamps *bool `json:"honorTimestamps,omitempty"`

	// trackTimestampsStaleness defines whether Prometheus tracks staleness of
	// the metrics that have an explicit timestamp present in scraped data.
	// Has no effect if `honorTimestamps` is false.
	//
	// It requires Prometheus >= v2.48.0.
	//
	// +optional
	TrackTimestampsStaleness *bool `json:"trackTimestampsStaleness,omitempty"`

	// metricRelabelings defines the relabeling rules to apply to the
	// samples before ingestion.
	//
	// +optional
	MetricRelabelConfigs []RelabelConfig `json:"metricRelabelings,omitempty"`

	// relabelings defines the relabeling rules to apply the target's
	// metadata labels.
	//
	// The Operator automatically adds relabelings for a few standard Kubernetes fields.
	//
	// The original scrape job's name is available via the `__tmp_prometheus_job_name` label.
	//
	// More info: https://prometheus.io/docs/prometheus/latest/configuration/configuration/#relabel_config
	//
	// +optional
	RelabelConfigs []RelabelConfig `json:"relabelings,omitempty"`

	// filterRunning when true, the pods which are not running (e.g. either in Failed or
	// Succeeded state) are dropped during the target discovery.
	//
	// If unset, the filtering is enabled.
	//
	// More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#pod-phase
	//
	// +optional
	FilterRunning *bool `json:"filterRunning,omitempty"`

	HTTPConfig `json:",inline"`
}
