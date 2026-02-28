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
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	PrometheusesKind  = "Prometheus"
	PrometheusName    = "prometheuses"
	PrometheusKindKey = "prometheus"
)

// ScrapeProtocol represents a protocol used by Prometheus for scraping metrics.
// Supported values are:
// * `OpenMetricsText0.0.1`
// * `OpenMetricsText1.0.0`
// * `PrometheusProto`
// * `PrometheusText0.0.4`
// * `PrometheusText1.0.0`
// +kubebuilder:validation:Enum=PrometheusProto;OpenMetricsText0.0.1;OpenMetricsText1.0.0;PrometheusText0.0.4;PrometheusText1.0.0
type ScrapeProtocol string

const (
	PrometheusProto      ScrapeProtocol = "PrometheusProto"
	PrometheusText0_0_4  ScrapeProtocol = "PrometheusText0.0.4"
	PrometheusText1_0_0  ScrapeProtocol = "PrometheusText1.0.0"
	OpenMetricsText0_0_1 ScrapeProtocol = "OpenMetricsText0.0.1"
	OpenMetricsText1_0_0 ScrapeProtocol = "OpenMetricsText1.0.0"
)

// RuntimeConfig configures the values for the process behavior.
type RuntimeConfig struct {
	// goGC defines the Go garbage collection target percentage. Lowering this number may increase the CPU usage.
	// See: https://tip.golang.org/doc/gc-guide#GOGC
	// +optional
	// +kubebuilder:validation:Minimum=-1
	GoGC *int32 `json:"goGC,omitempty"`
}

// PrometheusInterface is used by Prometheus and PrometheusAgent to share common methods, e.g. config generation.
// +k8s:deepcopy-gen=false
type PrometheusInterface interface {
	metav1.ObjectMetaAccessor
	schema.ObjectKind

	GetCommonPrometheusFields() CommonPrometheusFields
	SetCommonPrometheusFields(CommonPrometheusFields)

	GetStatus() PrometheusStatus
}

var _ = PrometheusInterface(&Prometheus{})

func (l *Prometheus) GetCommonPrometheusFields() CommonPrometheusFields {
	return l.Spec.CommonPrometheusFields
}

func (l *Prometheus) SetCommonPrometheusFields(f CommonPrometheusFields) {
	l.Spec.CommonPrometheusFields = f
}

func (l *Prometheus) GetStatus() PrometheusStatus {
	return l.Status
}

// +kubebuilder:validation:Enum=OnResource;OnShard
type AdditionalLabelSelectors string

const (
	// Automatically add a label selector that will select all pods matching the same Prometheus/PrometheusAgent resource (irrespective of their shards).
	ResourceNameLabelSelector AdditionalLabelSelectors = "OnResource"

	// Automatically add a label selector that will select all pods matching the same shard.
	ShardAndResourceNameLabelSelector AdditionalLabelSelectors = "OnShard"
)

type CoreV1TopologySpreadConstraint v1.TopologySpreadConstraint

type TopologySpreadConstraint struct {
	CoreV1TopologySpreadConstraint `json:",inline"`

	// additionalLabelSelectors Defines what Prometheus Operator managed labels should be added to labelSelector on the topologySpreadConstraint.
	// +optional
	AdditionalLabelSelectors *AdditionalLabelSelectors `json:"additionalLabelSelectors,omitempty"`
}

// +kubebuilder:validation:MinLength:=1
type EnableFeature string

// CommonPrometheusFields are the options available to both the Prometheus server and agent.
// +k8s:deepcopy-gen=true
type CommonPrometheusFields struct {
	// podMetadata defines labels and annotations which are propagated to the Prometheus pods.
	//
	// The following items are reserved and cannot be overridden:
	// * "prometheus" label, set to the name of the Prometheus object.
	// * "app.kubernetes.io/instance" label, set to the name of the Prometheus object.
	// * "app.kubernetes.io/managed-by" label, set to "prometheus-operator".
	// * "app.kubernetes.io/name" label, set to "prometheus".
	// * "app.kubernetes.io/version" label, set to the Prometheus version.
	// * "operator.prometheus.io/name" label, set to the name of the Prometheus object.
	// * "operator.prometheus.io/shard" label, set to the shard number of the Prometheus object.
	// * "kubectl.kubernetes.io/default-container" annotation, set to "prometheus".
	// +optional
	PodMetadata *EmbeddedObjectMetadata `json:"podMetadata,omitempty"`

	// serviceMonitorSelector defines the serviceMonitors to be selected for target discovery. An empty label
	// selector matches all objects. A null label selector matches no objects.
	//
	// If `spec.serviceMonitorSelector`, `spec.podMonitorSelector`, `spec.probeSelector`
	// and `spec.scrapeConfigSelector` are null, the Prometheus configuration is unmanaged.
	// The Prometheus operator will ensure that the Prometheus configuration's
	// Secret exists, but it is the responsibility of the user to provide the raw
	// gzipped Prometheus configuration under the `prometheus.yaml.gz` key.
	// This behavior is *deprecated* and will be removed in the next major version
	// of the custom resource definition. It is recommended to use
	// `spec.additionalScrapeConfigs` instead.
	// +optional
	ServiceMonitorSelector *metav1.LabelSelector `json:"serviceMonitorSelector,omitempty"`
	// serviceMonitorNamespaceSelector defines the namespaces to match for ServicedMonitors discovery. An empty label selector
	// matches all namespaces. A null label selector (default value) matches the current
	// namespace only.
	// +optional
	ServiceMonitorNamespaceSelector *metav1.LabelSelector `json:"serviceMonitorNamespaceSelector,omitempty"`

	// podMonitorSelector defines the podMonitors to be selected for target discovery. An empty label selector
	// matches all objects. A null label selector matches no objects.
	//
	// If `spec.serviceMonitorSelector`, `spec.podMonitorSelector`, `spec.probeSelector`
	// and `spec.scrapeConfigSelector` are null, the Prometheus configuration is unmanaged.
	// The Prometheus operator will ensure that the Prometheus configuration's
	// Secret exists, but it is the responsibility of the user to provide the raw
	// gzipped Prometheus configuration under the `prometheus.yaml.gz` key.
	// This behavior is *deprecated* and will be removed in the next major version
	// of the custom resource definition. It is recommended to use
	// `spec.additionalScrapeConfigs` instead.
	// +optional
	PodMonitorSelector *metav1.LabelSelector `json:"podMonitorSelector,omitempty"`
	// podMonitorNamespaceSelector defines the namespaces to match for PodMonitors discovery. An empty label selector
	// matches all namespaces. A null label selector (default value) matches the current
	// namespace only.
	// +optional
	PodMonitorNamespaceSelector *metav1.LabelSelector `json:"podMonitorNamespaceSelector,omitempty"`

	// probeSelector defines the probes to be selected for target discovery. An empty label selector
	// matches all objects. A null label selector matches no objects.
	//
	// If `spec.serviceMonitorSelector`, `spec.podMonitorSelector`, `spec.probeSelector`
	// and `spec.scrapeConfigSelector` are null, the Prometheus configuration is unmanaged.
	// The Prometheus operator will ensure that the Prometheus configuration's
	// Secret exists, but it is the responsibility of the user to provide the raw
	// gzipped Prometheus configuration under the `prometheus.yaml.gz` key.
	// This behavior is *deprecated* and will be removed in the next major version
	// of the custom resource definition. It is recommended to use
	// `spec.additionalScrapeConfigs` instead.
	// +optional
	ProbeSelector *metav1.LabelSelector `json:"probeSelector,omitempty"`
	// probeNamespaceSelector defines the namespaces to match for Probe discovery. An empty label
	// selector matches all namespaces. A null label selector matches the
	// current namespace only.
	// +optional
	ProbeNamespaceSelector *metav1.LabelSelector `json:"probeNamespaceSelector,omitempty"`

	// scrapeConfigSelector defines the scrapeConfigs to be selected for target discovery. An empty label
	// selector matches all objects. A null label selector matches no objects.
	//
	// If `spec.serviceMonitorSelector`, `spec.podMonitorSelector`, `spec.probeSelector`
	// and `spec.scrapeConfigSelector` are null, the Prometheus configuration is unmanaged.
	// The Prometheus operator will ensure that the Prometheus configuration's
	// Secret exists, but it is the responsibility of the user to provide the raw
	// gzipped Prometheus configuration under the `prometheus.yaml.gz` key.
	// This behavior is *deprecated* and will be removed in the next major version
	// of the custom resource definition. It is recommended to use
	// `spec.additionalScrapeConfigs` instead.
	//
	// Note that the ScrapeConfig custom resource definition is currently at Alpha level.
	//
	// +optional
	ScrapeConfigSelector *metav1.LabelSelector `json:"scrapeConfigSelector,omitempty"`
	// scrapeConfigNamespaceSelector defines the namespaces to match for ScrapeConfig discovery. An empty label selector
	// matches all namespaces. A null label selector matches the current
	// namespace only.
	//
	// Note that the ScrapeConfig custom resource definition is currently at Alpha level.
	//
	// +optional
	ScrapeConfigNamespaceSelector *metav1.LabelSelector `json:"scrapeConfigNamespaceSelector,omitempty"`

	// version of Prometheus being deployed. The operator uses this information
	// to generate the Prometheus StatefulSet + configuration files.
	//
	// If not specified, the operator assumes the latest upstream version of
	// Prometheus available at the time when the version of the operator was
	// released.
	// +optional
	Version string `json:"version,omitempty"`

	// paused defines when a Prometheus deployment is paused, no actions except for deletion
	// will be performed on the underlying objects.
	// +optional
	Paused bool `json:"paused,omitempty"`

	// image defines the container image name for Prometheus. If specified, it takes precedence
	// over the `spec.baseImage`, `spec.tag` and `spec.sha` fields.
	//
	// Specifying `spec.version` is still necessary to ensure the Prometheus
	// Operator knows which version of Prometheus is being configured.
	//
	// If neither `spec.image` nor `spec.baseImage` are defined, the operator
	// will use the latest upstream version of Prometheus available at the time
	// when the operator was released.
	//
	// +optional
	Image *string `json:"image,omitempty"`
	// imagePullPolicy defines the image pull policy for the 'prometheus', 'init-config-reloader' and 'config-reloader' containers.
	// See https://kubernetes.io/docs/concepts/containers/images/#image-pull-policy for more details.
	// +kubebuilder:validation:Enum="";Always;Never;IfNotPresent
	// +optional
	ImagePullPolicy v1.PullPolicy `json:"imagePullPolicy,omitempty"`
	// imagePullSecrets defines an optional list of references to Secrets in the same namespace
	// to use for pulling images from registries.
	// See http://kubernetes.io/docs/user-guide/images#specifying-imagepullsecrets-on-a-pod
	// +optional
	ImagePullSecrets []v1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// replicas defines the number of replicas of each shard to deploy for a Prometheus deployment.
	// `spec.replicas` multiplied by `spec.shards` is the total number of Pods
	// created.
	//
	// Default: 1
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// shards defines the number of shards to distribute the scraped targets onto.
	//
	// `spec.replicas` multiplied by `spec.shards` is the total number of Pods
	// being created.
	//
	// When not defined, the operator assumes only one shard.
	//
	// Note that scaling down shards will not reshard data onto the remaining
	// instances, it must be manually moved. Increasing shards will not reshard
	// data either but it will continue to be available from the same
	// instances. To query globally, use either
	// * Thanos sidecar + querier for query federation and Thanos Ruler for rules.
	// * Remote-write to send metrics to a central location.
	//
	// By default, the sharding of targets is performed on:
	// * The `__address__` target's metadata label for PodMonitor,
	// ServiceMonitor and ScrapeConfig resources.
	// * The `__param_target__` label for Probe resources.
	//
	// Users can define their own sharding implementation by setting the
	// `__tmp_hash` label during the target discovery with relabeling
	// configuration (either in the monitoring resources or via scrape class).
	//
	// You can also disable sharding on a specific target by setting the
	// `__tmp_disable_sharding` label with relabeling configuration. When
	// the label value isn't empty, all Prometheus shards will scrape the target.
	// +optional
	Shards *int32 `json:"shards,omitempty"`

	// replicaExternalLabelName defines the name of Prometheus external label used to denote the replica name.
	// The external label will _not_ be added when the field is set to the
	// empty string (`""`).
	//
	// Default: "prometheus_replica"
	// +optional
	ReplicaExternalLabelName *string `json:"replicaExternalLabelName,omitempty"`
	// prometheusExternalLabelName defines the name of Prometheus external label used to denote the Prometheus instance
	// name. The external label will _not_ be added when the field is set to
	// the empty string (`""`).
	//
	// Default: "prometheus"
	// +optional
	PrometheusExternalLabelName *string `json:"prometheusExternalLabelName,omitempty"`

	// logLevel for Prometheus and the config-reloader sidecar.
	// +kubebuilder:validation:Enum="";debug;info;warn;error
	// +optional
	LogLevel string `json:"logLevel,omitempty"`
	// logFormat for Log level for Prometheus and the config-reloader sidecar.
	// +kubebuilder:validation:Enum="";logfmt;json
	// +optional
	LogFormat string `json:"logFormat,omitempty"`

	// scrapeInterval defines interval between consecutive scrapes.
	//
	// Default: "30s"
	// +kubebuilder:default:="30s"
	// +optional
	ScrapeInterval Duration `json:"scrapeInterval,omitempty"`
	// scrapeTimeout defines the number of seconds to wait until a scrape request times out.
	// The value cannot be greater than the scrape interval otherwise the operator will reject the resource.
	// +optional
	ScrapeTimeout Duration `json:"scrapeTimeout,omitempty"`

	// scrapeProtocols defines the protocols to negotiate during a scrape. It tells clients the
	// protocols supported by Prometheus in order of preference (from most to least preferred).
	//
	// If unset, Prometheus uses its default value.
	//
	// It requires Prometheus >= v2.49.0.
	//
	// `PrometheusText1.0.0` requires Prometheus >= v3.0.0.
	//
	// +listType=set
	// +optional
	ScrapeProtocols []ScrapeProtocol `json:"scrapeProtocols,omitempty"`

	// externalLabels defines the labels to add to any time series or alerts when communicating with
	// external systems (federation, remote storage, Alertmanager).
	// Labels defined by `spec.replicaExternalLabelName` and
	// `spec.prometheusExternalLabelName` take precedence over this list.
	// +optional
	ExternalLabels map[string]string `json:"externalLabels,omitempty"`

	// enableRemoteWriteReceiver defines the Prometheus to be used as a receiver for the Prometheus remote
	// write protocol.
	//
	// WARNING: This is not considered an efficient way of ingesting samples.
	// Use it with caution for specific low-volume use cases.
	// It is not suitable for replacing the ingestion via scraping and turning
	// Prometheus into a push-based metrics collection system.
	// For more information see https://prometheus.io/docs/prometheus/latest/querying/api/#remote-write-receiver
	//
	// It requires Prometheus >= v2.33.0.
	// +optional
	EnableRemoteWriteReceiver bool `json:"enableRemoteWriteReceiver,omitempty"`

	// enableOTLPReceiver defines the Prometheus to be used as a receiver for the OTLP Metrics protocol.
	//
	// Note that the OTLP receiver endpoint is automatically enabled if `.spec.otlpConfig` is defined.
	//
	// It requires Prometheus >= v2.47.0.
	// +optional
	EnableOTLPReceiver *bool `json:"enableOTLPReceiver,omitempty"`

	// remoteWriteReceiverMessageVersions list of the protobuf message versions to accept when receiving the
	// remote writes.
	//
	// It requires Prometheus >= v2.54.0.
	//
	// +kubebuilder:validation:MinItems=1
	// +listType:=set
	// +optional
	RemoteWriteReceiverMessageVersions []RemoteWriteMessageVersion `json:"remoteWriteReceiverMessageVersions,omitempty"`

	// enableFeatures enables access to Prometheus feature flags. By default, no features are enabled.
	//
	// Enabling features which are disabled by default is entirely outside the
	// scope of what the maintainers will support and by doing so, you accept
	// that this behaviour may break at any time without notice.
	//
	// For more information see https://prometheus.io/docs/prometheus/latest/feature_flags/
	//
	// +listType:=set
	// +optional
	EnableFeatures []EnableFeature `json:"enableFeatures,omitempty"`

	// externalUrl defines the external URL under which the Prometheus service is externally
	// available. This is necessary to generate correct URLs (for instance if
	// Prometheus is accessible behind an Ingress resource).
	// +optional
	ExternalURL string `json:"externalUrl,omitempty"`
	// routePrefix defines the route prefix Prometheus registers HTTP handlers for.
	//
	// This is useful when using `spec.externalURL`, and a proxy is rewriting
	// HTTP routes of a request, and the actual ExternalURL is still true, but
	// the server serves requests under a different route prefix. For example
	// for use with `kubectl proxy`.
	// +optional
	RoutePrefix string `json:"routePrefix,omitempty"`

	// storage defines the storage used by Prometheus.
	// +optional
	Storage *StorageSpec `json:"storage,omitempty"`

	// volumes allows the configuration of additional volumes on the output
	// StatefulSet definition. Volumes specified will be appended to other
	// volumes that are generated as a result of StorageSpec objects.
	// +optional
	Volumes []v1.Volume `json:"volumes,omitempty"`
	// volumeMounts allows the configuration of additional VolumeMounts.
	//
	// VolumeMounts will be appended to other VolumeMounts in the 'prometheus'
	// container, that are generated as a result of StorageSpec objects.
	// +optional
	VolumeMounts []v1.VolumeMount `json:"volumeMounts,omitempty"`

	// persistentVolumeClaimRetentionPolicy defines the field controls if and how PVCs are deleted during the lifecycle of a StatefulSet.
	// The default behavior is all PVCs are retained.
	// This is an alpha field from kubernetes 1.23 until 1.26 and a beta field from 1.26.
	// It requires enabling the StatefulSetAutoDeletePVC feature gate.
	//
	// +optional
	PersistentVolumeClaimRetentionPolicy *appsv1.StatefulSetPersistentVolumeClaimRetentionPolicy `json:"persistentVolumeClaimRetentionPolicy,omitempty"`

	// web defines the configuration of the Prometheus web server.
	// +optional
	Web *PrometheusWebSpec `json:"web,omitempty"`

	// resources defines the resources requests and limits of the 'prometheus' container.
	// +optional
	Resources v1.ResourceRequirements `json:"resources,omitempty"`

	// nodeSelector defines on which Nodes the Pods are scheduled.
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// serviceAccountName is the name of the ServiceAccount to use to run the
	// Prometheus Pods.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// automountServiceAccountToken defines whether a service account token should be automatically mounted in the pod.
	// If the field isn't set, the operator mounts the service account token by default.
	//
	// **Warning:** be aware that by default, Prometheus requires the service account token for Kubernetes service discovery.
	// It is possible to use strategic merge patch to project the service account token into the 'prometheus' container.
	// +optional
	AutomountServiceAccountToken *bool `json:"automountServiceAccountToken,omitempty"`

	// secrets defines a list of Secrets in the same namespace as the Prometheus
	// object, which shall be mounted into the Prometheus Pods.
	// Each Secret is added to the StatefulSet definition as a volume named `secret-<secret-name>`.
	// The Secrets are mounted into /etc/prometheus/secrets/<secret-name> in the 'prometheus' container.
	// +listType:=set
	// +optional
	Secrets []string `json:"secrets,omitempty"`
	// configMaps defines a list of ConfigMaps in the same namespace as the Prometheus
	// object, which shall be mounted into the Prometheus Pods.
	// Each ConfigMap is added to the StatefulSet definition as a volume named `configmap-<configmap-name>`.
	// The ConfigMaps are mounted into /etc/prometheus/configmaps/<configmap-name> in the 'prometheus' container.
	// +optional
	ConfigMaps []string `json:"configMaps,omitempty"`

	// affinity defines the Pods' affinity scheduling rules if specified.
	// +optional
	Affinity *v1.Affinity `json:"affinity,omitempty"`
	// tolerations defines the Pods' tolerations if specified.
	// +optional
	Tolerations []v1.Toleration `json:"tolerations,omitempty"`

	// topologySpreadConstraints defines the pod's topology spread constraints if specified.
	// +optional
	TopologySpreadConstraints []TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`

	// remoteWrite defines the list of remote write configurations.
	// +optional
	RemoteWrite []RemoteWriteSpec `json:"remoteWrite,omitempty"`

	// otlp defines the settings related to the OTLP receiver feature.
	// It requires Prometheus >= v2.55.0.
	//
	// +optional
	OTLP *OTLPConfig `json:"otlp,omitempty"`

	// securityContext holds pod-level security attributes and common container settings.
	// This defaults to the default PodSecurityContext.
	// +optional
	SecurityContext *v1.PodSecurityContext `json:"securityContext,omitempty"`

	// dnsPolicy defines the DNS policy for the pods.
	//
	// +optional
	DNSPolicy *DNSPolicy `json:"dnsPolicy,omitempty"`
	// dnsConfig defines the DNS configuration for the pods.
	//
	// +optional
	DNSConfig *PodDNSConfig `json:"dnsConfig,omitempty"`
	// listenLocal when true, the Prometheus server listens on the loopback address
	// instead of the Pod IP's address.
	//
	// +optional
	ListenLocal bool `json:"listenLocal,omitempty"`

	// enableServiceLinks defines whether information about services should be injected into pod's environment variables
	// +optional
	EnableServiceLinks *bool `json:"enableServiceLinks,omitempty"`

	// containers allows injecting additional containers or modifying operator
	// generated containers. This can be used to allow adding an authentication
	// proxy to the Pods or to change the behavior of an operator generated
	// container. Containers described here modify an operator generated
	// container if they share the same name and modifications are done via a
	// strategic merge patch.
	//
	// The names of containers managed by the operator are:
	// * `prometheus`
	// * `config-reloader`
	// * `thanos-sidecar`
	//
	// Overriding containers is entirely outside the scope of what the
	// maintainers will support and by doing so, you accept that this behaviour
	// may break at any time without notice.
	// +optional
	Containers []v1.Container `json:"containers,omitempty"`
	// initContainers allows injecting initContainers to the Pod definition. Those
	// can be used to e.g.  fetch secrets for injection into the Prometheus
	// configuration from external sources. Any errors during the execution of
	// an initContainer will lead to a restart of the Pod. More info:
	// https://kubernetes.io/docs/concepts/workloads/pods/init-containers/
	// InitContainers described here modify an operator generated init
	// containers if they share the same name and modifications are done via a
	// strategic merge patch.
	//
	// The names of init container name managed by the operator are:
	// * `init-config-reloader`.
	//
	// Overriding init containers is entirely outside the scope of what the
	// maintainers will support and by doing so, you accept that this behaviour
	// may break at any time without notice.
	// +optional
	InitContainers []v1.Container `json:"initContainers,omitempty"`

	// additionalScrapeConfigs allows specifying a key of a Secret containing
	// additional Prometheus scrape configurations. Scrape configurations
	// specified are appended to the configurations generated by the Prometheus
	// Operator. Job configurations specified must have the form as specified
	// in the official Prometheus documentation:
	// https://prometheus.io/docs/prometheus/latest/configuration/configuration/#scrape_config.
	// As scrape configs are appended, the user is responsible to make sure it
	// is valid. Note that using this feature may expose the possibility to
	// break upgrades of Prometheus. It is advised to review Prometheus release
	// notes to ensure that no incompatible scrape configs are going to break
	// Prometheus after the upgrade.
	// +optional
	AdditionalScrapeConfigs *v1.SecretKeySelector `json:"additionalScrapeConfigs,omitempty"`

	// apiserverConfig allows specifying a host and auth methods to access the
	// Kuberntees API server.
	// If null, Prometheus is assumed to run inside of the cluster: it will
	// discover the API servers automatically and use the Pod's CA certificate
	// and bearer token file at /var/run/secrets/kubernetes.io/serviceaccount/.
	// +optional
	APIServerConfig *APIServerConfig `json:"apiserverConfig,omitempty"`

	// priorityClassName assigned to the Pods.
	// +optional
	PriorityClassName string `json:"priorityClassName,omitempty"`
	// portName used for the pods and governing service.
	// Default: "web"
	// +kubebuilder:default:="web"
	// +optional
	PortName string `json:"portName,omitempty"`

	// arbitraryFSAccessThroughSMs when true, ServiceMonitor, PodMonitor and Probe object are forbidden to
	// reference arbitrary files on the file system of the 'prometheus'
	// container.
	// When a ServiceMonitor's endpoint specifies a `bearerTokenFile` value
	// (e.g.  '/var/run/secrets/kubernetes.io/serviceaccount/token'), a
	// malicious target can get access to the Prometheus service account's
	// token in the Prometheus' scrape request. Setting
	// `spec.arbitraryFSAccessThroughSM` to 'true' would prevent the attack.
	// Users should instead provide the credentials using the
	// `spec.bearerTokenSecret` field.
	// +optional
	ArbitraryFSAccessThroughSMs ArbitraryFSAccessThroughSMsConfig `json:"arbitraryFSAccessThroughSMs,omitempty"`

	// overrideHonorLabels when true, Prometheus resolves label conflicts by renaming the labels in the scraped data
	//  to “exported_” for all targets created from ServiceMonitor, PodMonitor and
	// ScrapeConfig objects. Otherwise the HonorLabels field of the service or pod monitor applies.
	// In practice,`OverrideHonorLabels:true` enforces `honorLabels:false`
	// for all ServiceMonitor, PodMonitor and ScrapeConfig objects.
	// +optional
	OverrideHonorLabels bool `json:"overrideHonorLabels,omitempty"`
	// overrideHonorTimestamps when true, Prometheus ignores the timestamps for all the targets created
	// from service and pod monitors.
	// Otherwise the HonorTimestamps field of the service or pod monitor applies.
	// +optional
	OverrideHonorTimestamps bool `json:"overrideHonorTimestamps,omitempty"`

	// ignoreNamespaceSelectors when true, `spec.namespaceSelector` from all PodMonitor, ServiceMonitor
	// and Probe objects will be ignored. They will only discover targets
	// within the namespace of the PodMonitor, ServiceMonitor and Probe
	// object.
	// +optional
	IgnoreNamespaceSelectors bool `json:"ignoreNamespaceSelectors,omitempty"`

	// enforcedNamespaceLabel when not empty, a label will be added to:
	//
	// 1. All metrics scraped from `ServiceMonitor`, `PodMonitor`, `Probe` and `ScrapeConfig` objects.
	// 2. All metrics generated from recording rules defined in `PrometheusRule` objects.
	// 3. All alerts generated from alerting rules defined in `PrometheusRule` objects.
	// 4. All vector selectors of PromQL expressions defined in `PrometheusRule` objects.
	//
	// The label will not added for objects referenced in `spec.excludedFromEnforcement`.
	//
	// The label's name is this field's value.
	// The label's value is the namespace of the `ServiceMonitor`,
	// `PodMonitor`, `Probe`, `PrometheusRule` or `ScrapeConfig` object.
	// +optional
	EnforcedNamespaceLabel string `json:"enforcedNamespaceLabel,omitempty"`

	// enforcedSampleLimit when defined specifies a global limit on the number
	// of scraped samples that will be accepted. This overrides any
	// `spec.sampleLimit` set by ServiceMonitor, PodMonitor, Probe objects
	// unless `spec.sampleLimit` is greater than zero and less than
	// `spec.enforcedSampleLimit`.
	//
	// It is meant to be used by admins to keep the overall number of
	// samples/series under a desired limit.
	//
	// When both `enforcedSampleLimit` and `sampleLimit` are defined and greater than zero, the following rules apply:
	// * Scrape objects without a defined sampleLimit value will inherit the global sampleLimit value (Prometheus >= 2.45.0) or the enforcedSampleLimit value (Prometheus < v2.45.0).
	//   If Prometheus version is >= 2.45.0 and the `enforcedSampleLimit` is greater than the `sampleLimit`, the `sampleLimit` will be set to `enforcedSampleLimit`.
	// * Scrape objects with a sampleLimit value less than or equal to enforcedSampleLimit keep their specific value.
	// * Scrape objects with a sampleLimit value greater than enforcedSampleLimit are set to enforcedSampleLimit.
	//
	//
	// +optional
	EnforcedSampleLimit *uint64 `json:"enforcedSampleLimit,omitempty"`
	// enforcedTargetLimit when defined specifies a global limit on the number
	// of scraped targets. The value overrides any `spec.targetLimit` set by
	// ServiceMonitor, PodMonitor, Probe objects unless `spec.targetLimit` is
	// greater than zero and less than `spec.enforcedTargetLimit`.
	//
	// It is meant to be used by admins to to keep the overall number of
	// targets under a desired limit.
	//
	// When both `enforcedTargetLimit` and `targetLimit` are defined and greater than zero, the following rules apply:
	// * Scrape objects without a defined targetLimit value will inherit the global targetLimit value (Prometheus >= 2.45.0) or the enforcedTargetLimit value (Prometheus < v2.45.0).
	//   If Prometheus version is >= 2.45.0 and the `enforcedTargetLimit` is greater than the `targetLimit`, the `targetLimit` will be set to `enforcedTargetLimit`.
	// * Scrape objects with a targetLimit value less than or equal to enforcedTargetLimit keep their specific value.
	// * Scrape objects with a targetLimit value greater than enforcedTargetLimit are set to enforcedTargetLimit.
	//
	//
	// +optional
	EnforcedTargetLimit *uint64 `json:"enforcedTargetLimit,omitempty"`
	// enforcedLabelLimit when defined specifies a global limit on the number
	// of labels per sample. The value overrides any `spec.labelLimit` set by
	// ServiceMonitor, PodMonitor, Probe objects unless `spec.labelLimit` is
	// greater than zero and less than `spec.enforcedLabelLimit`.
	//
	// It requires Prometheus >= v2.27.0.
	//
	// When both `enforcedLabelLimit` and `labelLimit` are defined and greater than zero, the following rules apply:
	// * Scrape objects without a defined labelLimit value will inherit the global labelLimit value (Prometheus >= 2.45.0) or the enforcedLabelLimit value (Prometheus < v2.45.0).
	//   If Prometheus version is >= 2.45.0 and the `enforcedLabelLimit` is greater than the `labelLimit`, the `labelLimit` will be set to `enforcedLabelLimit`.
	// * Scrape objects with a labelLimit value less than or equal to enforcedLabelLimit keep their specific value.
	// * Scrape objects with a labelLimit value greater than enforcedLabelLimit are set to enforcedLabelLimit.
	//
	//
	// +optional
	EnforcedLabelLimit *uint64 `json:"enforcedLabelLimit,omitempty"`
	// enforcedLabelNameLengthLimit when defined specifies a global limit on the length
	// of labels name per sample. The value overrides any `spec.labelNameLengthLimit` set by
	// ServiceMonitor, PodMonitor, Probe objects unless `spec.labelNameLengthLimit` is
	// greater than zero and less than `spec.enforcedLabelNameLengthLimit`.
	//
	// It requires Prometheus >= v2.27.0.
	//
	// When both `enforcedLabelNameLengthLimit` and `labelNameLengthLimit` are defined and greater than zero, the following rules apply:
	// * Scrape objects without a defined labelNameLengthLimit value will inherit the global labelNameLengthLimit value (Prometheus >= 2.45.0) or the enforcedLabelNameLengthLimit value (Prometheus < v2.45.0).
	//   If Prometheus version is >= 2.45.0 and the `enforcedLabelNameLengthLimit` is greater than the `labelNameLengthLimit`, the `labelNameLengthLimit` will be set to `enforcedLabelNameLengthLimit`.
	// * Scrape objects with a labelNameLengthLimit value less than or equal to enforcedLabelNameLengthLimit keep their specific value.
	// * Scrape objects with a labelNameLengthLimit value greater than enforcedLabelNameLengthLimit are set to enforcedLabelNameLengthLimit.
	//
	//
	// +optional
	EnforcedLabelNameLengthLimit *uint64 `json:"enforcedLabelNameLengthLimit,omitempty"`
	// enforcedLabelValueLengthLimit when not null defines a global limit on the length
	// of labels value per sample. The value overrides any `spec.labelValueLengthLimit` set by
	// ServiceMonitor, PodMonitor, Probe objects unless `spec.labelValueLengthLimit` is
	// greater than zero and less than `spec.enforcedLabelValueLengthLimit`.
	//
	// It requires Prometheus >= v2.27.0.
	//
	// When both `enforcedLabelValueLengthLimit` and `labelValueLengthLimit` are defined and greater than zero, the following rules apply:
	// * Scrape objects without a defined labelValueLengthLimit value will inherit the global labelValueLengthLimit value (Prometheus >= 2.45.0) or the enforcedLabelValueLengthLimit value (Prometheus < v2.45.0).
	//   If Prometheus version is >= 2.45.0 and the `enforcedLabelValueLengthLimit` is greater than the `labelValueLengthLimit`, the `labelValueLengthLimit` will be set to `enforcedLabelValueLengthLimit`.
	// * Scrape objects with a labelValueLengthLimit value less than or equal to enforcedLabelValueLengthLimit keep their specific value.
	// * Scrape objects with a labelValueLengthLimit value greater than enforcedLabelValueLengthLimit are set to enforcedLabelValueLengthLimit.
	//
	//
	// +optional
	EnforcedLabelValueLengthLimit *uint64 `json:"enforcedLabelValueLengthLimit,omitempty"`
	// enforcedKeepDroppedTargets when defined specifies a global limit on the number of targets
	// dropped by relabeling that will be kept in memory. The value overrides
	// any `spec.keepDroppedTargets` set by
	// ServiceMonitor, PodMonitor, Probe objects unless `spec.keepDroppedTargets` is
	// greater than zero and less than `spec.enforcedKeepDroppedTargets`.
	//
	// It requires Prometheus >= v2.47.0.
	//
	// When both `enforcedKeepDroppedTargets` and `keepDroppedTargets` are defined and greater than zero, the following rules apply:
	// * Scrape objects without a defined keepDroppedTargets value will inherit the global keepDroppedTargets value (Prometheus >= 2.45.0) or the enforcedKeepDroppedTargets value (Prometheus < v2.45.0).
	//   If Prometheus version is >= 2.45.0 and the `enforcedKeepDroppedTargets` is greater than the `keepDroppedTargets`, the `keepDroppedTargets` will be set to `enforcedKeepDroppedTargets`.
	// * Scrape objects with a keepDroppedTargets value less than or equal to enforcedKeepDroppedTargets keep their specific value.
	// * Scrape objects with a keepDroppedTargets value greater than enforcedKeepDroppedTargets are set to enforcedKeepDroppedTargets.
	//
	//
	// +optional
	EnforcedKeepDroppedTargets *uint64 `json:"enforcedKeepDroppedTargets,omitempty"`
	// enforcedBodySizeLimit when defined specifies a global limit on the size
	// of uncompressed response body that will be accepted by Prometheus.
	// Targets responding with a body larger than this many bytes will cause
	// the scrape to fail.
	//
	// It requires Prometheus >= v2.28.0.
	//
	// When both `enforcedBodySizeLimit` and `bodySizeLimit` are defined and greater than zero, the following rules apply:
	// * Scrape objects without a defined bodySizeLimit value will inherit the global bodySizeLimit value (Prometheus >= 2.45.0) or the enforcedBodySizeLimit value (Prometheus < v2.45.0).
	//   If Prometheus version is >= 2.45.0 and the `enforcedBodySizeLimit` is greater than the `bodySizeLimit`, the `bodySizeLimit` will be set to `enforcedBodySizeLimit`.
	// * Scrape objects with a bodySizeLimit value less than or equal to enforcedBodySizeLimit keep their specific value.
	// * Scrape objects with a bodySizeLimit value greater than enforcedBodySizeLimit are set to enforcedBodySizeLimit.
	//
	// +optional
	EnforcedBodySizeLimit ByteSize `json:"enforcedBodySizeLimit,omitempty"`

	// nameValidationScheme defines the validation scheme for metric and label names.
	//
	// It requires Prometheus >= v2.55.0.
	//
	// +optional
	NameValidationScheme *NameValidationSchemeOptions `json:"nameValidationScheme,omitempty"`

	// nameEscapingScheme defines the character escaping scheme that will be requested when scraping
	// for metric and label names that do not conform to the legacy Prometheus
	// character set.
	//
	// It requires Prometheus >= v3.4.0.
	//
	// +optional
	NameEscapingScheme *NameEscapingSchemeOptions `json:"nameEscapingScheme,omitempty"`

	// convertClassicHistogramsToNHCB defines whether to convert all scraped classic histograms into a native
	// histogram with custom buckets.
	//
	// It requires Prometheus >= v3.4.0.
	//
	// +optional
	ConvertClassicHistogramsToNHCB *bool `json:"convertClassicHistogramsToNHCB,omitempty"`

	// scrapeClassicHistograms defines whether to scrape a classic histogram that is also exposed as a native histogram.
	//
	// Notice: `scrapeClassicHistograms` corresponds to the `always_scrape_classic_histograms` field in the Prometheus configuration.
	//
	// It requires Prometheus >= v3.5.0.
	//
	// +optional
	ScrapeClassicHistograms *bool `json:"scrapeClassicHistograms,omitempty"`

	// minReadySeconds defines the minimum number of seconds for which a newly created Pod should be ready
	// without any of its container crashing for it to be considered available.
	//
	// If unset, pods will be considered available as soon as they are ready.
	//
	// +kubebuilder:validation:Minimum:=0
	// +optional
	MinReadySeconds *int32 `json:"minReadySeconds,omitempty"`

	// hostAliases defines the optional list of hosts and IPs that will be injected into the Pod's
	// hosts file if specified.
	//
	// +listType=map
	// +listMapKey=ip
	// +optional
	HostAliases []HostAlias `json:"hostAliases,omitempty"`

	// additionalArgs allows setting additional arguments for the 'prometheus' container.
	//
	// It is intended for e.g. activating hidden flags which are not supported by
	// the dedicated configuration options yet. The arguments are passed as-is to the
	// Prometheus container which may cause issues if they are invalid or not supported
	// by the given Prometheus version.
	//
	// In case of an argument conflict (e.g. an argument which is already set by the
	// operator itself) or when providing an invalid argument, the reconciliation will
	// fail and an error will be logged.
	//
	// +optional
	AdditionalArgs []Argument `json:"additionalArgs,omitempty"`

	// walCompression defines the compression of the write-ahead log (WAL) using Snappy.
	//
	// WAL compression is enabled by default for Prometheus >= 2.20.0
	//
	// Requires Prometheus v2.11.0 and above.
	//
	// +optional
	WALCompression *bool `json:"walCompression,omitempty"`

	// excludedFromEnforcement defines the list of references to PodMonitor, ServiceMonitor, Probe and PrometheusRule objects
	// to be excluded from enforcing a namespace label of origin.
	//
	// It is only applicable if `spec.enforcedNamespaceLabel` set to true.
	//
	// +optional
	ExcludedFromEnforcement []ObjectReference `json:"excludedFromEnforcement,omitempty"`

	// hostNetwork defines the host's network namespace if true.
	//
	// Make sure to understand the security implications if you want to enable
	// it (https://kubernetes.io/docs/concepts/configuration/overview/ ).
	//
	// When hostNetwork is enabled, this will set the DNS policy to
	// `ClusterFirstWithHostNet` automatically (unless `.spec.DNSPolicy` is set
	// to a different value).
	//
	// +optional
	HostNetwork bool `json:"hostNetwork,omitempty"`

	// podTargetLabels are appended to the `spec.podTargetLabels` field of all
	// PodMonitor and ServiceMonitor objects.
	//
	// +optional
	PodTargetLabels []string `json:"podTargetLabels,omitempty"`

	// tracingConfig defines tracing in Prometheus.
	//
	// This is an *experimental feature*, it may change in any upcoming release
	// in a breaking way.
	//
	// +optional
	TracingConfig *PrometheusTracingConfig `json:"tracingConfig,omitempty"`
	// bodySizeLimit defines per-scrape on response body size.
	// Only valid in Prometheus versions 2.45.0 and newer.
	//
	// Note that the global limit only applies to scrape objects that don't specify an explicit limit value.
	// If you want to enforce a maximum limit for all scrape objects, refer to enforcedBodySizeLimit.
	//
	// +optional
	BodySizeLimit *ByteSize `json:"bodySizeLimit,omitempty"`
	// sampleLimit defines per-scrape limit on number of scraped samples that will be accepted.
	// Only valid in Prometheus versions 2.45.0 and newer.
	//
	// Note that the global limit only applies to scrape objects that don't specify an explicit limit value.
	// If you want to enforce a maximum limit for all scrape objects, refer to enforcedSampleLimit.
	//
	// +optional
	SampleLimit *uint64 `json:"sampleLimit,omitempty"`
	// targetLimit defines a limit on the number of scraped targets that will be accepted.
	// Only valid in Prometheus versions 2.45.0 and newer.
	//
	// Note that the global limit only applies to scrape objects that don't specify an explicit limit value.
	// If you want to enforce a maximum limit for all scrape objects, refer to enforcedTargetLimit.
	//
	// +optional
	TargetLimit *uint64 `json:"targetLimit,omitempty"`
	// labelLimit defines per-scrape limit on number of labels that will be accepted for a sample.
	// Only valid in Prometheus versions 2.45.0 and newer.
	//
	// Note that the global limit only applies to scrape objects that don't specify an explicit limit value.
	// If you want to enforce a maximum limit for all scrape objects, refer to enforcedLabelLimit.
	//
	// +optional
	LabelLimit *uint64 `json:"labelLimit,omitempty"`
	// labelNameLengthLimit defines the per-scrape limit on length of labels name that will be accepted for a sample.
	// Only valid in Prometheus versions 2.45.0 and newer.
	//
	// Note that the global limit only applies to scrape objects that don't specify an explicit limit value.
	// If you want to enforce a maximum limit for all scrape objects, refer to enforcedLabelNameLengthLimit.
	//
	// +optional
	LabelNameLengthLimit *uint64 `json:"labelNameLengthLimit,omitempty"`
	// labelValueLengthLimit defines the per-scrape limit on length of labels value that will be accepted for a sample.
	// Only valid in Prometheus versions 2.45.0 and newer.
	//
	// Note that the global limit only applies to scrape objects that don't specify an explicit limit value.
	// If you want to enforce a maximum limit for all scrape objects, refer to enforcedLabelValueLengthLimit.
	//
	// +optional
	LabelValueLengthLimit *uint64 `json:"labelValueLengthLimit,omitempty"`
	// keepDroppedTargets defines the per-scrape limit on the number of targets dropped by relabeling
	// that will be kept in memory. 0 means no limit.
	//
	// It requires Prometheus >= v2.47.0.
	//
	// Note that the global limit only applies to scrape objects that don't specify an explicit limit value.
	// If you want to enforce a maximum limit for all scrape objects, refer to enforcedKeepDroppedTargets.
	//
	// +optional
	KeepDroppedTargets *uint64 `json:"keepDroppedTargets,omitempty"`

	// reloadStrategy defines the strategy used to reload the Prometheus configuration.
	// If not specified, the configuration is reloaded using the /-/reload HTTP endpoint.
	// +optional
	ReloadStrategy *ReloadStrategyType `json:"reloadStrategy,omitempty"`

	// maximumStartupDurationSeconds defines the maximum time that the `prometheus` container's startup probe will wait before being considered failed. The startup probe will return success after the WAL replay is complete.
	// If set, the value should be greater than 60 (seconds). Otherwise it will be equal to 900 seconds (15 minutes).
	// +optional
	// +kubebuilder:validation:Minimum=60
	MaximumStartupDurationSeconds *int32 `json:"maximumStartupDurationSeconds,omitempty"`

	// scrapeClasses defines the list of scrape classes to expose to scraping objects such as
	// PodMonitors, ServiceMonitors, Probes and ScrapeConfigs.
	//
	// This is an *experimental feature*, it may change in any upcoming release
	// in a breaking way.
	//
	// +listType=map
	// +listMapKey=name
	// +optional
	ScrapeClasses []ScrapeClass `json:"scrapeClasses,omitempty"`

	// serviceDiscoveryRole defines the service discovery role used to discover targets from
	// `ServiceMonitor` objects and Alertmanager endpoints.
	//
	// If set, the value should be either "Endpoints" or "EndpointSlice".
	// If unset, the operator assumes the "Endpoints" role.
	//
	// +optional
	ServiceDiscoveryRole *ServiceDiscoveryRole `json:"serviceDiscoveryRole,omitempty"`

	// tsdb defines the runtime reloadable configuration of the timeseries database(TSDB).
	// It requires Prometheus >= v2.39.0 or PrometheusAgent >= v2.54.0.
	//
	// +optional
	TSDB *TSDBSpec `json:"tsdb,omitempty"`

	// scrapeFailureLogFile defines the file to which scrape failures are logged.
	// Reloading the configuration will reopen the file.
	//
	// If the filename has an empty path, e.g. 'file.log', The Prometheus Pods
	// will mount the file into an emptyDir volume at `/var/log/prometheus`.
	// If a full path is provided, e.g. '/var/log/prometheus/file.log', you
	// must mount a volume in the specified directory and it must be writable.
	// It requires Prometheus >= v2.55.0.
	//
	// +kubebuilder:validation:MinLength=1
	// +optional
	ScrapeFailureLogFile *string `json:"scrapeFailureLogFile,omitempty"`

	// serviceName defines the name of the service name used by the underlying StatefulSet(s) as the governing service.
	// If defined, the Service  must be created before the Prometheus/PrometheusAgent resource in the same namespace and it must define a selector that matches the pod labels.
	// If empty, the operator will create and manage a headless service named `prometheus-operated` for Prometheus resources,
	// or `prometheus-agent-operated` for PrometheusAgent resources.
	// When deploying multiple Prometheus/PrometheusAgent resources in the same namespace, it is recommended to specify a different value for each.
	// See https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/#stable-network-id for more details.
	// +optional
	// +kubebuilder:validation:MinLength=1
	ServiceName *string `json:"serviceName,omitempty"`

	// runtime defines the values for the Prometheus process behavior
	// +optional
	Runtime *RuntimeConfig `json:"runtime,omitempty"`

	// terminationGracePeriodSeconds defines the optional duration in seconds the pod needs to terminate gracefully.
	// Value must be non-negative integer. The value zero indicates stop immediately via
	// the kill signal (no opportunity to shut down) which may lead to data corruption.
	//
	// Defaults to 600 seconds.
	//
	// +kubebuilder:validation:Minimum:=0
	// +optional
	TerminationGracePeriodSeconds *int64 `json:"terminationGracePeriodSeconds,omitempty"`

	// hostUsers supports the user space in Kubernetes.
	//
	// More info: https://kubernetes.io/docs/tasks/configure-pod-container/user-namespaces/
	//
	//
	// The feature requires at least Kubernetes 1.28 with the `UserNamespacesSupport` feature gate enabled.
	// Starting Kubernetes 1.33, the feature is enabled by default.
	//
	// +optional
	HostUsers *bool `json:"hostUsers,omitempty"`
}

// Specifies the validation scheme for metric and label names.
//
// Supported values are:
//   - `UTF8NameValidationScheme` for UTF-8 support.
//   - `LegacyNameValidationScheme` for letters, numbers, colons, and underscores.
//
// Note that `LegacyNameValidationScheme` cannot be used along with the
// OpenTelemetry `NoUTF8EscapingWithSuffixes` translation strategy (if
// enabled).
//
// +kubebuilder:validation:Enum=UTF8;Legacy
type NameValidationSchemeOptions string

const (
	UTF8NameValidationScheme   NameValidationSchemeOptions = "UTF8"
	LegacyNameValidationScheme NameValidationSchemeOptions = "Legacy"
)

// Specifies the character escaping scheme that will be applied when scraping
// for metric and label names that do not conform to the legacy Prometheus
// character set.
//
// Supported values are:
//
//   - `AllowUTF8`, full UTF-8 support, no escaping needed.
//   - `Underscores`, legacy-invalid characters are escaped to underscores.
//   - `Dots`, dot characters are escaped to `_dot_`, underscores to `__`, and
//     all other legacy-invalid characters to underscores.
//   - `Values`, the string is prefixed by `U__` and all invalid characters are
//     escaped to their unicode value, surrounded by underscores.
//
// +kubebuilder:validation:Enum=AllowUTF8;Underscores;Dots;Values
type NameEscapingSchemeOptions string

const (
	AllowUTF8NameEscapingScheme   NameEscapingSchemeOptions = "AllowUTF8"
	UnderscoresNameEscapingScheme NameEscapingSchemeOptions = "Underscores"
	DotsNameEscapingScheme        NameEscapingSchemeOptions = "Dots"
	ValuesNameEscapingScheme      NameEscapingSchemeOptions = "Values"
)

// +kubebuilder:validation:Enum=HTTP;ProcessSignal
type ReloadStrategyType string

const (
	// HTTPReloadStrategyType reloads the configuration using the /-/reload HTTP endpoint.
	HTTPReloadStrategyType ReloadStrategyType = "HTTP"

	// ProcessSignalReloadStrategyType reloads the configuration by sending a SIGHUP signal to the process.
	ProcessSignalReloadStrategyType ReloadStrategyType = "ProcessSignal"
)

// +kubebuilder:validation:Enum=Endpoints;EndpointSlice
type ServiceDiscoveryRole string

const (
	EndpointsRole     ServiceDiscoveryRole = "Endpoints"
	EndpointSliceRole ServiceDiscoveryRole = "EndpointSlice"
)

func (cpf *CommonPrometheusFields) PrometheusURIScheme() string {
	if cpf.Web != nil && cpf.Web.TLSConfig != nil {
		return "https"
	}

	return "http"
}

func (cpf *CommonPrometheusFields) WebRoutePrefix() string {
	if cpf.RoutePrefix != "" {
		return cpf.RoutePrefix
	}

	return "/"
}

// +genclient
// +k8s:openapi-gen=true
// +kubebuilder:resource:categories="prometheus-operator",shortName="prom"
// +kubebuilder:printcolumn:name="Version",type="string",JSONPath=".spec.version",description="The version of Prometheus"
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

// The `Prometheus` custom resource definition (CRD) defines a desired [Prometheus](https://prometheus.io/docs/prometheus) setup to run in a Kubernetes cluster. It allows to specify many options such as the number of replicas, persistent storage, and Alertmanagers where firing alerts should be sent and many more.
//
// For each `Prometheus` resource, the Operator deploys one or several `StatefulSet` objects in the same namespace. The number of StatefulSets is equal to the number of shards which is 1 by default.
//
// The resource defines via label and namespace selectors which `ServiceMonitor`, `PodMonitor`, `Probe` and `PrometheusRule` objects should be associated to the deployed Prometheus instances.
//
// The Operator continuously reconciles the scrape and rules configuration and a sidecar container running in the Prometheus pods triggers a reload of the configuration when needed.
type Prometheus struct {
	// TypeMeta defines the versioned schema of this representation of an object.
	metav1.TypeMeta `json:",inline"`
	// metadata defines ObjectMeta as the metadata that all persisted resources.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// spec defines the specification of the desired behavior of the Prometheus cluster. More info:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	// +required
	Spec PrometheusSpec `json:"spec"`
	// status defines the most recent observed status of the Prometheus cluster. Read-only.
	// More info:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	// +optional
	Status PrometheusStatus `json:"status,omitempty"`
}

// DeepCopyObject implements the runtime.Object interface.
func (l *Prometheus) DeepCopyObject() runtime.Object {
	return l.DeepCopy()
}

// PrometheusList is a list of Prometheuses.
// +k8s:openapi-gen=true
type PrometheusList struct {
	// TypeMeta defines the versioned schema of this representation of an object.
	// +optional
	metav1.TypeMeta `json:",inline"`
	// metadata defines ListMeta as metadata for collection responses.
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`
	// List of Prometheuses
	Items []Prometheus `json:"items"`
}

// DeepCopyObject implements the runtime.Object interface.
func (l *PrometheusList) DeepCopyObject() runtime.Object {
	return l.DeepCopy()
}

// PrometheusSpec is a specification of the desired behavior of the Prometheus cluster. More info:
// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
// +k8s:openapi-gen=true
type PrometheusSpec struct {
	// +optional
	CommonPrometheusFields `json:",inline"`

	// baseImage is deprecated: use 'spec.image' instead.
	// +optional
	BaseImage string `json:"baseImage,omitempty"`
	// tag is deprecated: use 'spec.image' instead. The image's tag can be specified as part of the image name.
	// +optional
	Tag string `json:"tag,omitempty"`
	// sha is deprecated: use 'spec.image' instead. The image's digest can be specified as part of the image name.
	// +optional
	SHA string `json:"sha,omitempty"`

	// retention defines how long to retain the Prometheus data.
	//
	// Default: "24h" if `spec.retention` and `spec.retentionSize` are empty.
	// +optional
	Retention Duration `json:"retention,omitempty"`
	// retentionSize defines the maximum number of bytes used by the Prometheus data.
	// +optional
	RetentionSize ByteSize `json:"retentionSize,omitempty"`

	// shardRetentionPolicy defines the retention policy for the Prometheus shards.
	// (Alpha) Using this field requires the 'PrometheusShardRetentionPolicy' feature gate to be enabled.
	//
	// The final goals for this feature can be seen at https://github.com/prometheus-operator/prometheus-operator/blob/main/Documentation/proposals/202310-shard-autoscaling.md#graceful-scale-down-of-prometheus-servers,
	// however, the feature is not yet fully implemented in this PR. The limitation being:
	// * Retention duration is not settable, for now, shards are retained forever.
	//
	// +optional
	ShardRetentionPolicy *ShardRetentionPolicy `json:"shardRetentionPolicy,omitempty"`

	// disableCompaction when true, the Prometheus compaction is disabled.
	// When `spec.thanos.objectStorageConfig` or `spec.objectStorageConfigFile` are defined, the operator automatically
	// disables block compaction to avoid race conditions during block uploads (as the Thanos documentation recommends).
	// +optional
	DisableCompaction bool `json:"disableCompaction,omitempty"`

	// rules defines the configuration of the Prometheus rules' engine.
	// +optional
	Rules Rules `json:"rules,omitempty"`
	// prometheusRulesExcludedFromEnforce defines the list of PrometheusRule objects to which the namespace label
	// enforcement doesn't apply.
	// This is only relevant when `spec.enforcedNamespaceLabel` is set to true.
	// +optional
	// Deprecated: use `spec.excludedFromEnforcement` instead.
	PrometheusRulesExcludedFromEnforce []PrometheusRuleExcludeConfig `json:"prometheusRulesExcludedFromEnforce,omitempty"`
	// ruleSelector defines the prometheusRule objects to be selected for rule evaluation. An empty
	// label selector matches all objects. A null label selector matches no
	// objects.
	// +optional
	RuleSelector *metav1.LabelSelector `json:"ruleSelector,omitempty"`
	// ruleNamespaceSelector defines the namespaces to match for PrometheusRule discovery. An empty label selector
	// matches all namespaces. A null label selector matches the current
	// namespace only.
	// +optional
	RuleNamespaceSelector *metav1.LabelSelector `json:"ruleNamespaceSelector,omitempty"`

	// query defines the configuration of the Prometheus query service.
	// +optional
	Query *QuerySpec `json:"query,omitempty"`

	// alerting defines the settings related to Alertmanager.
	// +optional
	Alerting *AlertingSpec `json:"alerting,omitempty"`
	// additionalAlertRelabelConfigs defines a key of a Secret containing
	// additional Prometheus alert relabel configurations. The alert relabel
	// configurations are appended to the configuration generated by the
	// Prometheus Operator. They must be formatted according to the official
	// Prometheus documentation:
	//
	// https://prometheus.io/docs/prometheus/latest/configuration/configuration/#alert_relabel_configs
	//
	// The user is responsible for making sure that the configurations are valid
	//
	// Note that using this feature may expose the possibility to break
	// upgrades of Prometheus. It is advised to review Prometheus release notes
	// to ensure that no incompatible alert relabel configs are going to break
	// Prometheus after the upgrade.
	// +optional
	AdditionalAlertRelabelConfigs *v1.SecretKeySelector `json:"additionalAlertRelabelConfigs,omitempty"`
	// additionalAlertManagerConfigs defines a key of a Secret containing
	// additional Prometheus Alertmanager configurations. The Alertmanager
	// configurations are appended to the configuration generated by the
	// Prometheus Operator. They must be formatted according to the official
	// Prometheus documentation:
	//
	// https://prometheus.io/docs/prometheus/latest/configuration/configuration/#alertmanager_config
	//
	// The user is responsible for making sure that the configurations are valid
	//
	// Note that using this feature may expose the possibility to break
	// upgrades of Prometheus. It is advised to review Prometheus release notes
	// to ensure that no incompatible AlertManager configs are going to break
	// Prometheus after the upgrade.
	// +optional
	AdditionalAlertManagerConfigs *v1.SecretKeySelector `json:"additionalAlertManagerConfigs,omitempty"`

	// remoteRead defines the list of remote read configurations.
	// +optional
	RemoteRead []RemoteReadSpec `json:"remoteRead,omitempty"`

	// thanos defines the configuration of the optional Thanos sidecar.
	//
	// +optional
	Thanos *ThanosSpec `json:"thanos,omitempty"`

	// queryLogFile specifies where the file to which PromQL queries are logged.
	//
	// If the filename has an empty path, e.g. 'query.log', The Prometheus Pods
	// will mount the file into an emptyDir volume at `/var/log/prometheus`.
	// If a full path is provided, e.g. '/var/log/prometheus/query.log', you
	// must mount a volume in the specified directory and it must be writable.
	// This is because the prometheus container runs with a read-only root
	// filesystem for security reasons.
	// Alternatively, the location can be set to a standard I/O stream, e.g.
	// `/dev/stdout`, to log query information to the default Prometheus log
	// stream.
	// +optional
	QueryLogFile string `json:"queryLogFile,omitempty"`

	// allowOverlappingBlocks enables vertical compaction and vertical query
	// merge in Prometheus.
	//
	// Deprecated: this flag has no effect for Prometheus >= 2.39.0 where overlapping blocks are enabled by default.
	// +optional
	AllowOverlappingBlocks bool `json:"allowOverlappingBlocks,omitempty"`

	// exemplars related settings that are runtime reloadable.
	// It requires to enable the `exemplar-storage` feature flag to be effective.
	// +optional
	Exemplars *Exemplars `json:"exemplars,omitempty"`

	// evaluationInterval defines the interval between rule evaluations.
	// Default: "30s"
	// +kubebuilder:default:="30s"
	// +optional
	EvaluationInterval Duration `json:"evaluationInterval,omitempty"`

	// ruleQueryOffset defines the offset the rule evaluation timestamp of this particular group by the specified duration into the past.
	// It requires Prometheus >= v2.53.0.
	// +optional
	RuleQueryOffset *Duration `json:"ruleQueryOffset,omitempty"`

	// enableAdminAPI defines access to the Prometheus web admin API.
	//
	// WARNING: Enabling the admin APIs enables mutating endpoints, to delete data,
	// shutdown Prometheus, and more. Enabling this should be done with care and the
	// user is advised to add additional authentication authorization via a proxy to
	// ensure only clients authorized to perform these actions can do so.
	//
	// For more information:
	// https://prometheus.io/docs/prometheus/latest/querying/api/#tsdb-admin-apis
	// +optional
	EnableAdminAPI bool `json:"enableAdminAPI,omitempty"`
}

type WhenScaledRetentionType string

var (
	RetainWhenScaledRetentionType WhenScaledRetentionType = "Retain"
	DeleteWhenScaledRetentionType WhenScaledRetentionType = "Delete"
)

type RetainConfig struct {
	// retentionPeriod defines the retentionPeriod for shard retention policy.
	// +required
	RetentionPeriod Duration `json:"retentionPeriod"`
}

type ShardRetentionPolicy struct {
	// whenScaled defines the retention policy when the Prometheus shards are scaled down.
	// * `Delete`, the operator will delete the pods from the scaled-down shard(s).
	// * `Retain`, the operator will keep the pods from the scaled-down shard(s), so the data can still be queried.
	//
	// If not defined, the operator assumes the `Delete` value.
	// +kubebuilder:validation:Enum=Retain;Delete
	// +optional
	WhenScaled *WhenScaledRetentionType `json:"whenScaled,omitempty"`
	// retain defines the config for retention when the retention policy is set to `Retain`.
	// This field is ineffective as of now.
	// +optional
	Retain *RetainConfig `json:"retain,omitempty"`
}

type PrometheusTracingConfig struct {
	// clientType defines the client used to export the traces. Supported values are `http` or `grpc`.
	// +kubebuilder:validation:Enum=http;grpc
	// +optional
	ClientType *string `json:"clientType"`

	// endpoint to send the traces to. Should be provided in format <host>:<port>.
	// +kubebuilder:validation:MinLength:=1
	// +required
	Endpoint string `json:"endpoint"`

	// samplingFraction defines the probability a given trace will be sampled. Must be a float from 0 through 1.
	// +optional
	SamplingFraction *resource.Quantity `json:"samplingFraction"`

	// insecure if disabled, the client will use a secure connection.
	// +optional
	Insecure *bool `json:"insecure"`

	// headers defines the key-value pairs to be used as headers associated with gRPC or HTTP requests.
	// +optional
	Headers map[string]string `json:"headers"`

	// compression key for supported compression types. The only supported value is `gzip`.
	// +kubebuilder:validation:Enum=gzip
	// +optional
	Compression *string `json:"compression"`

	// timeout defines the maximum time the exporter will wait for each batch export.
	// +optional
	Timeout *Duration `json:"timeout"`

	// tlsConfig to use when sending traces.
	// +optional
	TLSConfig *TLSConfig `json:"tlsConfig"`
}

// PrometheusStatus is the most recent observed status of the Prometheus cluster.
// More info:
// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
// +k8s:openapi-gen=true
type PrometheusStatus struct {
	// paused defines whether any actions on the underlying managed objects are
	// being performed. Only delete actions will be performed.
	// +optional
	Paused bool `json:"paused"`
	// replicas defines the total number of non-terminated pods targeted by this Prometheus deployment
	// (their labels match the selector).
	// +optional
	Replicas int32 `json:"replicas"`
	// updatedReplicas defines the total number of non-terminated pods targeted by this Prometheus deployment
	// that have the desired version spec.
	// +optional
	UpdatedReplicas int32 `json:"updatedReplicas"`
	// availableReplicas defines the total number of available pods (ready for at least minReadySeconds)
	// targeted by this Prometheus deployment.
	// +optional
	AvailableReplicas int32 `json:"availableReplicas"`
	// unavailableReplicas defines the total number of unavailable pods targeted by this Prometheus deployment.
	// +optional
	UnavailableReplicas int32 `json:"unavailableReplicas"`
	// conditions defines the current state of the Prometheus deployment.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []Condition `json:"conditions,omitempty"`
	// shardStatuses defines the list has one entry per shard. Each entry provides a summary of the shard status.
	// +listType=map
	// +listMapKey=shardID
	// +optional
	ShardStatuses []ShardStatus `json:"shardStatuses,omitempty"`
	// shards defines the most recently observed number of shards.
	// +optional
	Shards int32 `json:"shards,omitempty"`
	// selector used to match the pods targeted by this Prometheus resource.
	// +optional
	Selector string `json:"selector,omitempty"`
}

// AlertingSpec defines parameters for alerting configuration of Prometheus servers.
// +k8s:openapi-gen=true
type AlertingSpec struct {
	// alertmanagers endpoints where Prometheus should send alerts to.
	// +required
	Alertmanagers []AlertmanagerEndpoints `json:"alertmanagers"`
}

// StorageSpec defines the configured storage for a group Prometheus servers.
// If no storage option is specified, then by default an [EmptyDir](https://kubernetes.io/docs/concepts/storage/volumes/#emptydir) will be used.
//
// If multiple storage options are specified, priority will be given as follows:
//  1. emptyDir
//  2. ephemeral
//  3. volumeClaimTemplate
//
// +k8s:openapi-gen=true
type StorageSpec struct {
	// disableMountSubPath deprecated: subPath usage will be removed in a future release.
	// +optional
	DisableMountSubPath bool `json:"disableMountSubPath,omitempty"`
	// emptyDir to be used by the StatefulSet.
	// If specified, it takes precedence over `ephemeral` and `volumeClaimTemplate`.
	// More info: https://kubernetes.io/docs/concepts/storage/volumes/#emptydir
	// +optional
	EmptyDir *v1.EmptyDirVolumeSource `json:"emptyDir,omitempty"`
	// ephemeral to be used by the StatefulSet.
	// This is a beta field in k8s 1.21 and GA in 1.15.
	// For lower versions, starting with k8s 1.19, it requires enabling the GenericEphemeralVolume feature gate.
	// More info: https://kubernetes.io/docs/concepts/storage/ephemeral-volumes/#generic-ephemeral-volumes
	// +optional
	Ephemeral *v1.EphemeralVolumeSource `json:"ephemeral,omitempty"`
	// volumeClaimTemplate defines the PVC spec to be used by the Prometheus StatefulSets.
	// The easiest way to use a volume that cannot be automatically provisioned
	// is to use a label selector alongside manually created PersistentVolumes.
	// +optional
	VolumeClaimTemplate EmbeddedPersistentVolumeClaim `json:"volumeClaimTemplate,omitempty"`
}

// QuerySpec defines the query command line flags when starting Prometheus.
// +k8s:openapi-gen=true
type QuerySpec struct {
	// lookbackDelta defines the delta difference allowed for retrieving metrics during expression evaluations.
	// +optional
	LookbackDelta *string `json:"lookbackDelta,omitempty"`
	// maxConcurrency defines the number of concurrent queries that can be run at once.
	// +kubebuilder:validation:Minimum:=1
	// +optional
	MaxConcurrency *int32 `json:"maxConcurrency,omitempty"`
	// maxSamples defines the maximum number of samples a single query can load into memory. Note that
	// queries will fail if they would load more samples than this into memory,
	// so this also limits the number of samples a query can return.
	// +optional
	MaxSamples *int32 `json:"maxSamples,omitempty"`
	// timeout defines the maximum time a query may take before being aborted.
	// +optional
	Timeout *Duration `json:"timeout,omitempty"`
}

// PrometheusWebSpec defines the configuration of the Prometheus web server.
// +k8s:openapi-gen=true
type PrometheusWebSpec struct {
	// +optional
	WebConfigFileFields `json:",inline"`
	// pageTitle defines the prometheus web page title.
	// +optional
	PageTitle *string `json:"pageTitle,omitempty"`

	// maxConnections defines the maximum number of simultaneous connections
	// A zero value means that Prometheus doesn't accept any incoming connection.
	// +kubebuilder:validation:Minimum:=0
	// +optional
	MaxConnections *int32 `json:"maxConnections,omitempty"`
}

// ThanosSpec defines the configuration of the Thanos sidecar.
// +k8s:openapi-gen=true
type ThanosSpec struct {
	// image defines the container image name for Thanos. If specified, it takes precedence over
	// the `spec.thanos.baseImage`, `spec.thanos.tag` and `spec.thanos.sha`
	// fields.
	//
	// Specifying `spec.thanos.version` is still necessary to ensure the
	// Prometheus Operator knows which version of Thanos is being configured.
	//
	// If neither `spec.thanos.image` nor `spec.thanos.baseImage` are defined,
	// the operator will use the latest upstream version of Thanos available at
	// the time when the operator was released.
	//
	// +optional
	Image *string `json:"image,omitempty"`

	// version of Thanos being deployed. The operator uses this information
	// to generate the Prometheus StatefulSet + configuration files.
	//
	// If not specified, the operator assumes the latest upstream release of
	// Thanos available at the time when the version of the operator was
	// released.
	//
	// +optional
	Version *string `json:"version,omitempty"`
	// tag is deprecated: use 'image' instead. The image's tag can be specified as as part of the image name.
	// +optional
	Tag *string `json:"tag,omitempty"`
	// sha is deprecated: use 'image' instead.  The image digest can be specified as part of the image name.
	// +optional
	SHA *string `json:"sha,omitempty"`
	// baseImage is deprecated: use 'image' instead.
	// +optional
	BaseImage *string `json:"baseImage,omitempty"`

	// resources defines the resources requests and limits of the Thanos sidecar.
	// +optional
	Resources v1.ResourceRequirements `json:"resources,omitempty"`

	// objectStorageConfig defines the Thanos sidecar's configuration to upload TSDB blocks to object storage.
	//
	// More info: https://thanos.io/tip/thanos/storage.md/
	//
	// objectStorageConfigFile takes precedence over this field.
	// +optional
	ObjectStorageConfig *v1.SecretKeySelector `json:"objectStorageConfig,omitempty"`
	// objectStorageConfigFile defines the Thanos sidecar's configuration file to upload TSDB blocks to object storage.
	//
	// More info: https://thanos.io/tip/thanos/storage.md/
	//
	// This field takes precedence over objectStorageConfig.
	// +optional
	ObjectStorageConfigFile *string `json:"objectStorageConfigFile,omitempty"`

	// listenLocal is deprecated: use `grpcListenLocal` and `httpListenLocal` instead.
	// +optional
	ListenLocal bool `json:"listenLocal,omitempty"`

	// grpcListenLocal defines when true, the Thanos sidecar listens on the loopback interface instead
	// of the Pod IP's address for the gRPC endpoints.
	//
	// It has no effect if `listenLocal` is true.
	// +optional
	GRPCListenLocal bool `json:"grpcListenLocal,omitempty"`

	// httpListenLocal when true, the Thanos sidecar listens on the loopback interface instead
	// of the Pod IP's address for the HTTP endpoints.
	//
	// It has no effect if `listenLocal` is true.
	// +optional
	HTTPListenLocal bool `json:"httpListenLocal,omitempty"`

	// tracingConfig defines the tracing configuration for the Thanos sidecar.
	//
	// `tracingConfigFile` takes precedence over this field.
	//
	// More info: https://thanos.io/tip/thanos/tracing.md/
	//
	// This is an *experimental feature*, it may change in any upcoming release
	// in a breaking way.
	//
	// +optional
	TracingConfig *v1.SecretKeySelector `json:"tracingConfig,omitempty"`
	// tracingConfigFile defines the tracing configuration file for the Thanos sidecar.
	//
	// This field takes precedence over `tracingConfig`.
	//
	// More info: https://thanos.io/tip/thanos/tracing.md/
	//
	// This is an *experimental feature*, it may change in any upcoming release
	// in a breaking way.
	// +optional
	TracingConfigFile string `json:"tracingConfigFile,omitempty"`

	// grpcServerTlsConfig defines the TLS parameters for the gRPC server providing the StoreAPI.
	//
	// Note: Currently only the `caFile`, `certFile`, and `keyFile` fields are supported.
	//
	// +optional
	GRPCServerTLSConfig *TLSConfig `json:"grpcServerTlsConfig,omitempty"`

	// logLevel for the Thanos sidecar.
	// +kubebuilder:validation:Enum="";debug;info;warn;error
	// +optional
	LogLevel string `json:"logLevel,omitempty"`
	// logFormat for the Thanos sidecar.
	// +kubebuilder:validation:Enum="";logfmt;json
	// +optional
	LogFormat string `json:"logFormat,omitempty"`

	// minTime defines the start of time range limit served by the Thanos sidecar's StoreAPI.
	// The field's value should be a constant time in RFC3339 format or a time
	// duration relative to current time, such as -1d or 2h45m. Valid duration
	// units are ms, s, m, h, d, w, y.
	// +optional
	MinTime string `json:"minTime,omitempty"`

	// blockSize controls the size of TSDB blocks produced by Prometheus.
	// The default value is 2h to match the upstream Prometheus defaults.
	//
	// WARNING: Changing the block duration can impact the performance and
	// efficiency of the entire Prometheus/Thanos stack due to how it interacts
	// with memory and Thanos compactors. It is recommended to keep this value
	// set to a multiple of 120 times your longest scrape or rule interval. For
	// example, 30s * 120 = 1h.
	//
	// +kubebuilder:default:="2h"
	// +optional
	BlockDuration Duration `json:"blockSize,omitempty"`

	// readyTimeout defines the maximum time that the Thanos sidecar will wait for
	// Prometheus to start.
	// +optional
	ReadyTimeout Duration `json:"readyTimeout,omitempty"`
	// getConfigInterval defines how often to retrieve the Prometheus configuration.
	// +optional
	GetConfigInterval Duration `json:"getConfigInterval,omitempty"`
	// getConfigTimeout defines the maximum time to wait when retrieving the Prometheus configuration.
	// +optional
	GetConfigTimeout Duration `json:"getConfigTimeout,omitempty"`

	// volumeMounts allows configuration of additional VolumeMounts for Thanos.
	// VolumeMounts specified will be appended to other VolumeMounts in the
	// 'thanos-sidecar' container.
	// +optional
	VolumeMounts []v1.VolumeMount `json:"volumeMounts,omitempty"`

	// additionalArgs allows setting additional arguments for the Thanos container.
	// The arguments are passed as-is to the Thanos container which may cause issues
	// if they are invalid or not supported the given Thanos version.
	// In case of an argument conflict (e.g. an argument which is already set by the
	// operator itself) or when providing an invalid argument, the reconciliation will
	// fail and an error will be logged.
	// +optional
	AdditionalArgs []Argument `json:"additionalArgs,omitempty"`
}

// RemoteWriteSpec defines the configuration to write samples from Prometheus
// to a remote endpoint.
// +k8s:openapi-gen=true
type RemoteWriteSpec struct {
	// url defines the URL of the endpoint to send samples to.
	// +kubebuilder:validation:MinLength=1
	// +required
	URL string `json:"url"`

	// name of the remote write queue, it must be unique if specified. The
	// name is used in metrics and logging in order to differentiate queues.
	//
	// It requires Prometheus >= v2.15.0 or Thanos >= 0.24.0.
	//
	// +optional
	Name *string `json:"name,omitempty"`

	// messageVersion defines the Remote Write message's version to use when writing to the endpoint.
	//
	// `Version1.0` corresponds to the `prometheus.WriteRequest` protobuf message introduced in Remote Write 1.0.
	// `Version2.0` corresponds to the `io.prometheus.write.v2.Request` protobuf message introduced in Remote Write 2.0.
	//
	// When `Version2.0` is selected, Prometheus will automatically be
	// configured to append the metadata of scraped metrics to the WAL.
	//
	// Before setting this field, consult with your remote storage provider
	// what message version it supports.
	//
	// It requires Prometheus >= v2.54.0 or Thanos >= v0.37.0.
	//
	// +optional
	MessageVersion *RemoteWriteMessageVersion `json:"messageVersion,omitempty"`

	// sendExemplars enables sending of exemplars over remote write. Note that
	// exemplar-storage itself must be enabled using the `spec.enableFeatures`
	// option for exemplars to be scraped in the first place.
	//
	// It requires Prometheus >= v2.27.0 or Thanos >= v0.24.0.
	//
	// +optional
	SendExemplars *bool `json:"sendExemplars,omitempty"`

	// sendNativeHistograms enables sending of native histograms, also known as sparse histograms
	// over remote write.
	//
	// It requires Prometheus >= v2.40.0 or Thanos >= v0.30.0.
	//
	// +optional
	SendNativeHistograms *bool `json:"sendNativeHistograms,omitempty"`

	// remoteTimeout defines the timeout for requests to the remote write endpoint.
	// +optional
	RemoteTimeout *Duration `json:"remoteTimeout,omitempty"`

	// headers defines the custom HTTP headers to be sent along with each remote write request.
	// Be aware that headers that are set by Prometheus itself can't be overwritten.
	//
	// It requires Prometheus >= v2.25.0 or Thanos >= v0.24.0.
	//
	// +optional
	Headers map[string]string `json:"headers,omitempty"`

	// writeRelabelConfigs defines the list of remote write relabel configurations.
	// +optional
	WriteRelabelConfigs []RelabelConfig `json:"writeRelabelConfigs,omitempty"`

	// oauth2 configuration for the URL.
	//
	// It requires Prometheus >= v2.27.0 or Thanos >= v0.24.0.
	//
	// Cannot be set at the same time as `sigv4`, `authorization`, `basicAuth`, or `azureAd`.
	// +optional
	OAuth2 *OAuth2 `json:"oauth2,omitempty"`

	// basicAuth configuration for the URL.
	//
	// Cannot be set at the same time as `sigv4`, `authorization`, `oauth2`, or `azureAd`.
	//
	// +optional
	BasicAuth *BasicAuth `json:"basicAuth,omitempty"`

	// bearerTokenFile defines the file from which to read bearer token for the URL.
	//
	// Deprecated: this will be removed in a future release. Prefer using `authorization`.
	// +optional
	BearerTokenFile string `json:"bearerTokenFile,omitempty"`

	// authorization section for the URL.
	//
	// It requires Prometheus >= v2.26.0 or Thanos >= v0.24.0.
	//
	// Cannot be set at the same time as `sigv4`, `basicAuth`, `oauth2`, or `azureAd`.
	//
	// +optional
	Authorization *Authorization `json:"authorization,omitempty"`

	// sigv4 defines the AWS's Signature Verification 4 for the URL.
	//
	// It requires Prometheus >= v2.26.0 or Thanos >= v0.24.0.
	//
	// Cannot be set at the same time as `authorization`, `basicAuth`, `oauth2`, or `azureAd`.
	//
	// +optional
	Sigv4 *Sigv4 `json:"sigv4,omitempty"`

	// azureAd for the URL.
	//
	// It requires Prometheus >= v2.45.0 or Thanos >= v0.31.0.
	//
	// Cannot be set at the same time as `authorization`, `basicAuth`, `oauth2`, or `sigv4`.
	//
	// +optional
	AzureAD *AzureAD `json:"azureAd,omitempty"`

	// bearerToken is deprecated: this will be removed in a future release.
	// *Warning: this field shouldn't be used because the token value appears
	// in clear-text. Prefer using `authorization`.*
	//
	// +optional
	BearerToken string `json:"bearerToken,omitempty"`

	// tlsConfig to use for the URL.
	// +optional
	TLSConfig *TLSConfig `json:"tlsConfig,omitempty"`

	// Optional ProxyConfig.
	// +optional
	ProxyConfig `json:",inline"`

	// followRedirects defines whether HTTP requests follow HTTP 3xx redirects.
	//
	// It requires Prometheus >= v2.26.0 or Thanos >= v0.24.0.
	//
	// +optional
	FollowRedirects *bool `json:"followRedirects,omitempty"`

	// queueConfig allows tuning of the remote write queue parameters.
	// +optional
	QueueConfig *QueueConfig `json:"queueConfig,omitempty"`

	// metadataConfig defines how to send a series metadata to the remote storage.
	//
	// When the field is empty, **no metadata** is sent. But when the field is
	// null, metadata is sent.
	//
	// +optional
	MetadataConfig *MetadataConfig `json:"metadataConfig,omitempty"`

	// enableHTTP2 defines whether to enable HTTP2.
	// +optional
	EnableHttp2 *bool `json:"enableHTTP2,omitempty"`

	// roundRobinDNS controls the DNS resolution behavior for remote-write connections.
	// When enabled:
	//   - The remote-write mechanism will resolve the hostname via DNS.
	//   - It will randomly select one of the resolved IP addresses and connect to it.
	//
	// When disabled (default behavior):
	//   - The Go standard library will handle hostname resolution.
	//   - It will attempt connections to each resolved IP address sequentially.
	//
	// Note: The connection timeout applies to the entire resolution and connection process.
	//
	//	If disabled, the timeout is distributed across all connection attempts.
	//
	// It requires Prometheus >= v3.1.0 or Thanos >= v0.38.0.
	//
	// +optional
	RoundRobinDNS *bool `json:"roundRobinDNS,omitempty"`
}

// +kubebuilder:validation:Enum=V1.0;V2.0
type RemoteWriteMessageVersion string

const (
	// Remote Write message's version 1.0.
	RemoteWriteMessageVersion1_0 = RemoteWriteMessageVersion("V1.0")
	// Remote Write message's version 2.0.
	RemoteWriteMessageVersion2_0 = RemoteWriteMessageVersion("V2.0")
)

// QueueConfig allows the tuning of remote write's queue_config parameters.
// This object is referenced in the RemoteWriteSpec object.
// +k8s:openapi-gen=true
type QueueConfig struct {
	// capacity defines the number of samples to buffer per shard before we start
	// dropping them.
	// +optional
	Capacity int `json:"capacity,omitempty"`
	// minShards defines the minimum number of shards, i.e. amount of concurrency.
	// +optional
	MinShards int `json:"minShards,omitempty"`
	// maxShards defines the maximum number of shards, i.e. amount of concurrency.
	// +optional
	MaxShards int `json:"maxShards,omitempty"`
	// maxSamplesPerSend defines the maximum number of samples per send.
	// +optional
	MaxSamplesPerSend int `json:"maxSamplesPerSend,omitempty"`
	// batchSendDeadline defines the maximum time a sample will wait in buffer.
	// +optional
	BatchSendDeadline *Duration `json:"batchSendDeadline,omitempty"`
	// maxRetries defines the maximum number of times to retry a batch on recoverable errors.
	// +optional
	MaxRetries int `json:"maxRetries,omitempty"`
	// minBackoff defines the initial retry delay. Gets doubled for every retry.
	// +optional
	MinBackoff *Duration `json:"minBackoff,omitempty"`
	// maxBackoff defines the maximum retry delay.
	// +optional
	MaxBackoff *Duration `json:"maxBackoff,omitempty"`
	// retryOnRateLimit defines the retry upon receiving a 429 status code from the remote-write storage.
	//
	// This is an *experimental feature*, it may change in any upcoming release
	// in a breaking way.
	// +optional
	RetryOnRateLimit bool `json:"retryOnRateLimit,omitempty"`
	// sampleAgeLimit drops samples older than the limit.
	// It requires Prometheus >= v2.50.0 or Thanos >= v0.32.0.
	//
	// +optional
	SampleAgeLimit *Duration `json:"sampleAgeLimit,omitempty"`
}

// Sigv4 defines AWS's Signature Verification 4 signing process to
// sign requests.
// +k8s:openapi-gen=true
type Sigv4 struct {
	// region defines the AWS region. If blank, the region from the default credentials chain used.
	// +optional
	Region string `json:"region,omitempty"`
	// accessKey defines the AWS API key. If not specified, the environment variable
	// `AWS_ACCESS_KEY_ID` is used.
	// +optional
	AccessKey *v1.SecretKeySelector `json:"accessKey,omitempty"`
	// secretKey defines the AWS API secret. If not specified, the environment
	// variable `AWS_SECRET_ACCESS_KEY` is used.
	// +optional
	SecretKey *v1.SecretKeySelector `json:"secretKey,omitempty"`
	// profile defines the named AWS profile used to authenticate.
	// +optional
	Profile string `json:"profile,omitempty"`
	// roleArn defines the named AWS profile used to authenticate.
	// +optional
	RoleArn string `json:"roleArn,omitempty"`
	// useFIPSSTSEndpoint defines the FIPS mode for the AWS STS endpoint.
	// It requires Prometheus >= v2.54.0.
	//
	// +optional
	UseFIPSSTSEndpoint *bool `json:"useFIPSSTSEndpoint,omitempty"`
}

// AzureAD defines the configuration for remote write's azuread parameters.
// +k8s:openapi-gen=true
type AzureAD struct {
	// cloud defines the Azure Cloud. Options are 'AzurePublic', 'AzureChina', or 'AzureGovernment'.
	// +kubebuilder:validation:Enum=AzureChina;AzureGovernment;AzurePublic
	// +optional
	Cloud *string `json:"cloud,omitempty"`
	// managedIdentity defines the Azure User-assigned Managed identity.
	// Cannot be set at the same time as `oauth` or `sdk`.
	// +optional
	ManagedIdentity *ManagedIdentity `json:"managedIdentity,omitempty"`
	// oauth defines the oauth config that is being used to authenticate.
	// Cannot be set at the same time as `managedIdentity` or `sdk`.
	//
	// It requires Prometheus >= v2.48.0 or Thanos >= v0.31.0.
	//
	// +optional
	OAuth *AzureOAuth `json:"oauth,omitempty"`
	// sdk defines the Azure SDK config that is being used to authenticate.
	// See https://learn.microsoft.com/en-us/azure/developer/go/azure-sdk-authentication
	// Cannot be set at the same time as `oauth` or `managedIdentity`.
	//
	// It requires Prometheus >= v2.52.0 or Thanos >= v0.36.0.
	// +optional
	SDK *AzureSDK `json:"sdk,omitempty"`
}

// AzureOAuth defines the Azure OAuth settings.
// +k8s:openapi-gen=true
type AzureOAuth struct {
	// clientId defines the clientId of the Azure Active Directory application that is being used to authenticate.
	// +required
	// +kubebuilder:validation:MinLength=1
	ClientID string `json:"clientId"`
	// clientSecret specifies a key of a Secret containing the client secret of the Azure Active Directory application that is being used to authenticate.
	// +required
	ClientSecret v1.SecretKeySelector `json:"clientSecret"`
	// tenantId is the tenant ID of the Azure Active Directory application that is being used to authenticate.
	// +required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern:=^[0-9a-zA-Z-.]+$
	TenantID string `json:"tenantId"`
}

// ManagedIdentity defines the Azure User-assigned Managed identity.
// +k8s:openapi-gen=true
type ManagedIdentity struct {
	// clientId defines the Azure User-assigned Managed identity.
	//
	// For Prometheus >= 3.5.0 and Thanos >= 0.40.0, this field is allowed to be empty to support system-assigned managed identities.
	//
	// +optional
	// +kubebuilder:validation:MinLength:=1
	ClientID *string `json:"clientId"`
}

// AzureSDK is used to store azure SDK config values.
type AzureSDK struct {
	// tenantId defines the tenant ID of the azure active directory application that is being used to authenticate.
	// +optional
	// +kubebuilder:validation:Pattern:=^[0-9a-zA-Z-.]+$
	TenantID *string `json:"tenantId,omitempty"`
}

// RemoteReadSpec defines the configuration for Prometheus to read back samples
// from a remote endpoint.
// +k8s:openapi-gen=true
type RemoteReadSpec struct {
	// url defines the URL of the endpoint to query from.
	// +required
	URL string `json:"url"`

	// name of the remote read queue, it must be unique if specified. The
	// name is used in metrics and logging in order to differentiate read
	// configurations.
	//
	// It requires Prometheus >= v2.15.0.
	//
	// +optional
	Name string `json:"name,omitempty"`

	// requiredMatchers defines an optional list of equality matchers which have to be present
	// in a selector to query the remote read endpoint.
	// +optional
	RequiredMatchers map[string]string `json:"requiredMatchers,omitempty"`

	// remoteTimeout defines the timeout for requests to the remote read endpoint.
	// +optional
	RemoteTimeout *Duration `json:"remoteTimeout,omitempty"`

	// headers defines the custom HTTP headers to be sent along with each remote read request.
	// Be aware that headers that are set by Prometheus itself can't be overwritten.
	// Only valid in Prometheus versions 2.26.0 and newer.
	// +optional
	Headers map[string]string `json:"headers,omitempty"`

	// readRecent defines whether reads should be made for queries for time ranges that
	// the local storage should have complete data for.
	// +optional
	ReadRecent bool `json:"readRecent,omitempty"`

	// oauth2 configuration for the URL.
	//
	// It requires Prometheus >= v2.27.0.
	//
	// Cannot be set at the same time as `authorization`, or `basicAuth`.
	//
	// +optional
	OAuth2 *OAuth2 `json:"oauth2,omitempty"`
	// basicAuth configuration for the URL.
	//
	// Cannot be set at the same time as `authorization`, or `oauth2`.
	//
	// +optional
	BasicAuth *BasicAuth `json:"basicAuth,omitempty"`
	// bearerTokenFile defines the file from which to read the bearer token for the URL.
	//
	// Deprecated: this will be removed in a future release. Prefer using `authorization`.
	// +optional
	BearerTokenFile string `json:"bearerTokenFile,omitempty"`
	// authorization section for the URL.
	//
	// It requires Prometheus >= v2.26.0.
	//
	// Cannot be set at the same time as `basicAuth`, or `oauth2`.
	//
	// +optional
	Authorization *Authorization `json:"authorization,omitempty"`

	// bearerToken is deprecated: this will be removed in a future release.
	// *Warning: this field shouldn't be used because the token value appears
	// in clear-text. Prefer using `authorization`.*
	//
	// +optional
	BearerToken string `json:"bearerToken,omitempty"`

	// tlsConfig to use for the URL.
	// +optional
	TLSConfig *TLSConfig `json:"tlsConfig,omitempty"`

	// Optional ProxyConfig.
	// +optional
	ProxyConfig `json:",inline"`

	// followRedirects defines whether HTTP requests follow HTTP 3xx redirects.
	//
	// It requires Prometheus >= v2.26.0.
	//
	// +optional
	FollowRedirects *bool `json:"followRedirects,omitempty"`

	// filterExternalLabels defines whether to use the external labels as selectors for the remote read endpoint.
	//
	// It requires Prometheus >= v2.34.0.
	//
	// +optional
	FilterExternalLabels *bool `json:"filterExternalLabels,omitempty"`
}

// RelabelConfig allows dynamic rewriting of the label set for targets, alerts,
// scraped samples and remote write samples.
//
// More info: https://prometheus.io/docs/prometheus/latest/configuration/configuration/#relabel_config
//
// +k8s:openapi-gen=true
type RelabelConfig struct {
	// sourceLabels defines the source labels select values from existing labels. Their content is
	// concatenated using the configured Separator and matched against the
	// configured regular expression.
	//
	// +optional
	SourceLabels []LabelName `json:"sourceLabels,omitempty"`

	// separator defines the string between concatenated SourceLabels.
	// +optional
	Separator *string `json:"separator,omitempty"`

	// targetLabel defines the label to which the resulting string is written in a replacement.
	//
	// It is mandatory for `Replace`, `HashMod`, `Lowercase`, `Uppercase`,
	// `KeepEqual` and `DropEqual` actions.
	//
	// Regex capture groups are available.
	// +optional
	TargetLabel string `json:"targetLabel,omitempty"`

	// regex defines the regular expression against which the extracted value is matched.
	// +optional
	Regex string `json:"regex,omitempty"`

	// modulus to take of the hash of the source label values.
	//
	// Only applicable when the action is `HashMod`.
	// +optional
	Modulus uint64 `json:"modulus,omitempty"`

	// replacement value against which a Replace action is performed if the
	// regular expression matches.
	//
	// Regex capture groups are available.
	//
	// +optional
	Replacement *string `json:"replacement,omitempty"`

	// action to perform based on the regex matching.
	//
	// `Uppercase` and `Lowercase` actions require Prometheus >= v2.36.0.
	// `DropEqual` and `KeepEqual` actions require Prometheus >= v2.41.0.
	//
	// Default: "Replace"
	//
	// +kubebuilder:validation:Enum=replace;Replace;keep;Keep;drop;Drop;hashmod;HashMod;labelmap;LabelMap;labeldrop;LabelDrop;labelkeep;LabelKeep;lowercase;Lowercase;uppercase;Uppercase;keepequal;KeepEqual;dropequal;DropEqual
	// +kubebuilder:default=replace
	// +optional
	Action string `json:"action,omitempty"`
}

// APIServerConfig defines how the Prometheus server connects to the Kubernetes API server.
//
// More info: https://prometheus.io/docs/prometheus/latest/configuration/configuration/#kubernetes_sd_config
//
// +k8s:openapi-gen=true
type APIServerConfig struct {
	// host defines the Kubernetes API address consisting of a hostname or IP address followed
	// by an optional port number.
	// +required
	Host string `json:"host"`

	// basicAuth configuration for the API server.
	//
	// Cannot be set at the same time as `authorization`, `bearerToken`, or
	// `bearerTokenFile`.
	//
	// +optional
	BasicAuth *BasicAuth `json:"basicAuth,omitempty"`

	// bearerTokenFile defines the file to read bearer token for accessing apiserver.
	//
	// Cannot be set at the same time as `basicAuth`, `authorization`, or `bearerToken`.
	//
	// Deprecated: this will be removed in a future release. Prefer using `authorization`.
	// +optional
	BearerTokenFile string `json:"bearerTokenFile,omitempty"`

	// tlsConfig to use for the API server.
	//
	// +optional
	TLSConfig *TLSConfig `json:"tlsConfig,omitempty"`

	// authorization section for the API server.
	//
	// Cannot be set at the same time as `basicAuth`, `bearerToken`, or
	// `bearerTokenFile`.
	//
	// +optional
	Authorization *Authorization `json:"authorization,omitempty"`

	// bearerToken is deprecated: this will be removed in a future release.
	//  *Warning: this field shouldn't be used because the token value appears
	// in clear-text. Prefer using `authorization`.*
	//
	// +optional
	BearerToken string `json:"bearerToken,omitempty"`

	// Optional ProxyConfig.
	// +optional
	ProxyConfig `json:",inline"`
}

// +kubebuilder:validation:Enum=v1;V1;v2;V2
type AlertmanagerAPIVersion string

const (
	AlertmanagerAPIVersion1 = AlertmanagerAPIVersion("V1")
	AlertmanagerAPIVersion2 = AlertmanagerAPIVersion("V2")
)

// AlertmanagerEndpoints defines a selection of a single Endpoints object
// containing Alertmanager IPs to fire alerts against.
// +k8s:openapi-gen=true
type AlertmanagerEndpoints struct {
	// namespace of the Endpoints object.
	//
	// If not set, the object will be discovered in the namespace of the
	// Prometheus object.
	//
	// +kubebuilder:validation:MinLength:=1
	// +optional
	Namespace *string `json:"namespace,omitempty"`

	// name of the Endpoints object in the namespace.
	//
	// +kubebuilder:validation:MinLength:=1
	// +required
	Name string `json:"name"`

	// port on which the Alertmanager API is exposed.
	// +required
	Port intstr.IntOrString `json:"port"`

	// scheme defines the HTTP scheme to use when sending alerts.
	//
	// +optional
	Scheme *Scheme `json:"scheme,omitempty"`

	// pathPrefix defines the prefix for the HTTP path alerts are pushed to.
	//
	// +kubebuilder:validation:MinLength=1
	// +optional
	PathPrefix *string `json:"pathPrefix,omitempty"`

	// tlsConfig to use for Alertmanager.
	//
	// +optional
	TLSConfig *TLSConfig `json:"tlsConfig,omitempty"`

	// basicAuth configuration for Alertmanager.
	//
	// Cannot be set at the same time as `bearerTokenFile`, `authorization` or `sigv4`.
	//
	// +optional
	BasicAuth *BasicAuth `json:"basicAuth,omitempty"`

	// bearerTokenFile defines the file to read bearer token for Alertmanager.
	//
	// Cannot be set at the same time as `basicAuth`, `authorization`, or `sigv4`.
	//
	// Deprecated: this will be removed in a future release. Prefer using `authorization`.
	// +optional
	BearerTokenFile string `json:"bearerTokenFile,omitempty"`

	// authorization section for Alertmanager.
	//
	// Cannot be set at the same time as `basicAuth`, `bearerTokenFile` or `sigv4`.
	//
	// +optional
	Authorization *SafeAuthorization `json:"authorization,omitempty"`

	// sigv4 defines AWS's Signature Verification 4 for the URL.
	//
	// It requires Prometheus >= v2.48.0.
	//
	// Cannot be set at the same time as `basicAuth`, `bearerTokenFile` or `authorization`.
	//
	// +optional
	Sigv4 *Sigv4 `json:"sigv4,omitempty"`

	// ProxyConfig
	// +optional
	ProxyConfig `json:",inline"`

	// apiVersion defines the version of the Alertmanager API that Prometheus uses to send alerts.
	// It can be "V1" or "V2".
	// The field has no effect for Prometheus >= v3.0.0 because only the v2 API is supported.
	//
	// +optional
	APIVersion *AlertmanagerAPIVersion `json:"apiVersion,omitempty"`

	// timeout defines a per-target Alertmanager timeout when pushing alerts.
	//
	// +optional
	Timeout *Duration `json:"timeout,omitempty"`

	// enableHttp2 defines whether to enable HTTP2.
	//
	// +optional
	EnableHttp2 *bool `json:"enableHttp2,omitempty"`

	// relabelings defines the relabel configuration applied to the discovered Alertmanagers.
	//
	// +optional
	RelabelConfigs []RelabelConfig `json:"relabelings,omitempty"`

	// alertRelabelings defines the relabeling configs applied before sending alerts to a specific Alertmanager.
	// It requires Prometheus >= v2.51.0.
	//
	// +optional
	AlertRelabelConfigs []RelabelConfig `json:"alertRelabelings,omitempty"`
}

// +k8s:openapi-gen=true
type Rules struct {
	// alert defines the parameters of the Prometheus rules' engine.
	//
	// Any update to these parameters trigger a restart of the pods.
	// +optional
	Alert RulesAlert `json:"alert,omitempty"`
}

// +k8s:openapi-gen=true
type RulesAlert struct {
	// forOutageTolerance defines the max time to tolerate prometheus outage for restoring 'for' state of
	// alert.
	// +optional
	ForOutageTolerance string `json:"forOutageTolerance,omitempty"`

	// forGracePeriod defines the minimum duration between alert and restored 'for' state.
	//
	// This is maintained only for alerts with a configured 'for' time greater
	// than the grace period.
	// +optional
	ForGracePeriod string `json:"forGracePeriod,omitempty"`

	// resendDelay defines the minimum amount of time to wait before resending an alert to
	// Alertmanager.
	// +optional
	ResendDelay string `json:"resendDelay,omitempty"`
}

// MetadataConfig configures the sending of series metadata to the remote storage.
//
// +k8s:openapi-gen=true
type MetadataConfig struct {
	// send defines whether metric metadata is sent to the remote storage or not.
	//
	// +optional
	Send bool `json:"send,omitempty"`

	// sendInterval defines how frequently metric metadata is sent to the remote storage.
	//
	// +optional
	SendInterval Duration `json:"sendInterval,omitempty"`

	// maxSamplesPerSend defines the maximum number of metadata samples per send.
	//
	// It requires Prometheus >= v2.29.0.
	//
	// +optional
	// +kubebuilder:validation:Minimum=-1
	MaxSamplesPerSend *int32 `json:"maxSamplesPerSend,omitempty"`
}

type ShardStatus struct {
	// shardID defines the identifier of the shard.
	// +required
	ShardID string `json:"shardID"`
	// replicas defines the total number of pods targeted by this shard.
	// +required
	Replicas int32 `json:"replicas"`
	// updatedReplicas defines the total number of non-terminated pods targeted by this shard
	// that have the desired spec.
	// +required
	UpdatedReplicas int32 `json:"updatedReplicas"`
	// availableReplicas defines the total number of available pods (ready for at least minReadySeconds)
	// targeted by this shard.
	// +required
	AvailableReplicas int32 `json:"availableReplicas"`
	// unavailableReplicas defines the Total number of unavailable pods targeted by this shard.
	// +required
	UnavailableReplicas int32 `json:"unavailableReplicas"`
}

type TSDBSpec struct {
	// outOfOrderTimeWindow defines how old an out-of-order/out-of-bounds sample can be with
	// respect to the TSDB max time.
	//
	// An out-of-order/out-of-bounds sample is ingested into the TSDB as long as
	// the timestamp of the sample is >= (TSDB.MaxTime - outOfOrderTimeWindow).
	//
	// This is an *experimental feature*, it may change in any upcoming release
	// in a breaking way.
	//
	// It requires Prometheus >= v2.39.0 or PrometheusAgent >= v2.54.0.
	// +optional
	OutOfOrderTimeWindow *Duration `json:"outOfOrderTimeWindow,omitempty"`
}

type Exemplars struct {
	// maxSize defines the maximum number of exemplars stored in memory for all series.
	//
	// exemplar-storage itself must be enabled using the `spec.enableFeature`
	// option for exemplars to be scraped in the first place.
	//
	// If not set, Prometheus uses its default value. A value of zero or less
	// than zero disables the storage.
	//
	// +optional
	MaxSize *int64 `json:"maxSize,omitempty"`
}

// SafeAuthorization specifies a subset of the Authorization struct, that is
// safe for use because it doesn't provide access to the Prometheus container's
// filesystem.
//
// +k8s:openapi-gen=true
type SafeAuthorization struct {
	// type defines the authentication type. The value is case-insensitive.
	//
	// "Basic" is not a supported value.
	//
	// Default: "Bearer"
	// +optional
	Type string `json:"type,omitempty"`

	// credentials defines a key of a Secret in the namespace that contains the credentials for authentication.
	// +optional
	Credentials *v1.SecretKeySelector `json:"credentials,omitempty"`
}

// Validate semantically validates the given Authorization section.
func (c *SafeAuthorization) Validate() error {
	if c == nil {
		return nil
	}

	if strings.ToLower(strings.TrimSpace(c.Type)) == "basic" {
		return errors.New("authorization type cannot be set to \"basic\", use \"basicAuth\" instead")
	}

	if c.Credentials == nil {
		return errors.New("authorization credentials are required")
	}

	return nil
}

type Authorization struct {
	// +optional
	SafeAuthorization `json:",inline"`

	// credentialsFile defines the file to read a secret from, mutually exclusive with `credentials`.
	// +optional
	CredentialsFile string `json:"credentialsFile,omitempty"`
}

// Validate semantically validates the given Authorization section.
func (c *Authorization) Validate() error {
	if c == nil {
		return nil
	}

	if c.Credentials != nil && c.CredentialsFile != "" {
		return errors.New("authorization can not specify both \"credentials\" and \"credentialsFile\"")
	}

	if strings.ToLower(strings.TrimSpace(c.Type)) == "basic" {
		return errors.New("authorization type cannot be set to \"basic\", use \"basicAuth\" instead")
	}

	return nil
}

type ScrapeClass struct {
	// name of the scrape class.
	//
	// +kubebuilder:validation:MinLength=1
	// +required
	Name string `json:"name"`

	// default defines that the scrape applies to all scrape objects that
	// don't configure an explicit scrape class name.
	//
	// Only one scrape class can be set as the default.
	//
	// +optional
	Default *bool `json:"default,omitempty"`

	// fallbackScrapeProtocol defines the protocol to use if a scrape returns blank, unparseable, or otherwise invalid Content-Type.
	// It will only apply if the scrape resource doesn't specify any FallbackScrapeProtocol
	//
	// It requires Prometheus >= v3.0.0.
	// +optional
	FallbackScrapeProtocol *ScrapeProtocol `json:"fallbackScrapeProtocol,omitempty"`

	// tlsConfig defines the TLS settings to use for the scrape. When the
	// scrape objects define their own CA, certificate and/or key, they take
	// precedence over the corresponding scrape class fields.
	//
	// For now only the `caFile`, `certFile` and `keyFile` fields are supported.
	//
	// +optional
	TLSConfig *TLSConfig `json:"tlsConfig,omitempty"`

	// authorization section for the ScrapeClass.
	// It will only apply if the scrape resource doesn't specify any Authorization.
	// +optional
	Authorization *Authorization `json:"authorization,omitempty"`

	// relabelings defines the relabeling rules to apply to all scrape targets.
	//
	// The Operator automatically adds relabelings for a few standard Kubernetes fields
	// like `__meta_kubernetes_namespace` and `__meta_kubernetes_service_name`.
	// Then the Operator adds the scrape class relabelings defined here.
	// Then the Operator adds the target-specific relabelings defined in the scrape object.
	//
	// More info: https://prometheus.io/docs/prometheus/latest/configuration/configuration/#relabel_config
	//
	// +optional
	Relabelings []RelabelConfig `json:"relabelings,omitempty"`

	// metricRelabelings defines the relabeling rules to apply to all samples before ingestion.
	//
	// The Operator adds the scrape class metric relabelings defined here.
	// Then the Operator adds the target-specific metric relabelings defined in ServiceMonitors, PodMonitors, Probes and ScrapeConfigs.
	// Then the Operator adds namespace enforcement relabeling rule, specified in '.spec.enforcedNamespaceLabel'.
	//
	// More info: https://prometheus.io/docs/prometheus/latest/configuration/configuration/#metric_relabel_configs
	//
	// +optional
	MetricRelabelings []RelabelConfig `json:"metricRelabelings,omitempty"`

	// attachMetadata defines additional metadata to the discovered targets.
	// When the scrape object defines its own configuration, it takes
	// precedence over the scrape class configuration.
	//
	// +optional
	AttachMetadata *AttachMetadata `json:"attachMetadata,omitempty"`
}

// TranslationStrategyOption represents a translation strategy option for the OTLP endpoint.
// Supported values are:
// * `NoUTF8EscapingWithSuffixes`
// * `UnderscoreEscapingWithSuffixes`
// * `UnderscoreEscapingWithoutSuffixes`
// * `NoTranslation`
// +kubebuilder:validation:Enum=NoUTF8EscapingWithSuffixes;UnderscoreEscapingWithSuffixes;NoTranslation;UnderscoreEscapingWithoutSuffixes
type TranslationStrategyOption string

const (
	NoUTF8EscapingWithSuffixes     TranslationStrategyOption = "NoUTF8EscapingWithSuffixes"
	UnderscoreEscapingWithSuffixes TranslationStrategyOption = "UnderscoreEscapingWithSuffixes"
	// It requires Prometheus >= v3.4.0.
	NoTranslation TranslationStrategyOption = "NoTranslation"
	// It requires Prometheus >= v3.6.0.
	UnderscoreEscapingWithoutSuffixes TranslationStrategyOption = "UnderscoreEscapingWithoutSuffixes"
)

// OTLPConfig is the configuration for writing to the OTLP endpoint.
//
// +k8s:openapi-gen=true
type OTLPConfig struct {
	// promoteAllResourceAttributes promotes all resource attributes to metric labels except the ones defined in `ignoreResourceAttributes`.
	//
	// Cannot be true when `promoteResourceAttributes` is defined.
	// It requires Prometheus >= v3.5.0.
	// +optional
	PromoteAllResourceAttributes *bool `json:"promoteAllResourceAttributes,omitempty"`

	// ignoreResourceAttributes defines the list of OpenTelemetry resource attributes to ignore when `promoteAllResourceAttributes` is true.
	//
	// It requires `promoteAllResourceAttributes` to be true.
	// It requires Prometheus >= v3.5.0.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:items:MinLength=1
	// +listType=set
	// +optional
	IgnoreResourceAttributes []string `json:"ignoreResourceAttributes,omitempty"`

	// promoteResourceAttributes defines the list of OpenTelemetry Attributes that should be promoted to metric labels, defaults to none.
	// Cannot be defined when `promoteAllResourceAttributes` is true.
	//
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:items:MinLength=1
	// +listType=set
	// +optional
	PromoteResourceAttributes []string `json:"promoteResourceAttributes,omitempty"`

	// translationStrategy defines how the OTLP receiver endpoint translates the incoming metrics.
	//
	// It requires Prometheus >= v3.0.0.
	// +optional
	TranslationStrategy *TranslationStrategyOption `json:"translationStrategy,omitempty"`

	// keepIdentifyingResourceAttributes enables adding `service.name`, `service.namespace` and `service.instance.id`
	// resource attributes to the `target_info` metric, on top of converting them into the `instance` and `job` labels.
	//
	// It requires Prometheus >= v3.1.0.
	// +optional
	KeepIdentifyingResourceAttributes *bool `json:"keepIdentifyingResourceAttributes,omitempty"`

	// convertHistogramsToNHCB defines optional translation of OTLP explicit bucket histograms into native histograms with custom buckets.
	// It requires Prometheus >= v3.4.0.
	// +optional
	ConvertHistogramsToNHCB *bool `json:"convertHistogramsToNHCB,omitempty"`

	// promoteScopeMetadata controls whether to promote OpenTelemetry scope metadata (i.e. name, version, schema URL, and attributes) to metric labels.
	// As per the OpenTelemetry specification, the aforementioned scope metadata should be identifying, i.e. made into metric labels.
	// It requires Prometheus >= v3.6.0.
	// +optional
	PromoteScopeMetadata *bool `json:"promoteScopeMetadata,omitempty"`
}

// Validate semantically validates the given OTLPConfig section.
func (c *OTLPConfig) Validate() error {
	if c == nil {
		return nil
	}

	if len(c.PromoteResourceAttributes) > 0 && c.PromoteAllResourceAttributes != nil && *c.PromoteAllResourceAttributes {
		return fmt.Errorf("'promoteAllResourceAttributes' cannot be set to 'true' simultaneously with 'promoteResourceAttributes'")
	}

	if len(c.IgnoreResourceAttributes) > 0 && (c.PromoteAllResourceAttributes == nil || !*c.PromoteAllResourceAttributes) {
		return fmt.Errorf("'ignoreResourceAttributes' can only be set when 'promoteAllResourceAttributes' is true")
	}

	return nil
}
