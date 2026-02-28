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
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	AlertmanagersKind   = "Alertmanager"
	AlertmanagerName    = "alertmanagers"
	AlertManagerKindKey = "alertmanager"
)

// +genclient
// +k8s:openapi-gen=true
// +kubebuilder:resource:categories="prometheus-operator",shortName="am"
// +kubebuilder:printcolumn:name="Version",type="string",JSONPath=".spec.version",description="The version of Alertmanager"
// +kubebuilder:printcolumn:name="Replicas",type="integer",JSONPath=".spec.replicas",description="The number of desired replicas"
// +kubebuilder:printcolumn:name="Ready",type="integer",JSONPath=".status.availableReplicas",description="The number of ready replicas"
// +kubebuilder:printcolumn:name="Reconciled",type="string",JSONPath=".status.conditions[?(@.type == 'Reconciled')].status"
// +kubebuilder:printcolumn:name="Available",type="string",JSONPath=".status.conditions[?(@.type == 'Available')].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="Paused",type="boolean",JSONPath=".status.paused",description="Whether the resource reconciliation is paused or not",priority=1
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.replicas,selectorpath=.status.selector
// +genclient:method=GetScale,verb=get,subresource=scale,result=k8s.io/api/autoscaling/v1.Scale
// +genclient:method=UpdateScale,verb=update,subresource=scale,input=k8s.io/api/autoscaling/v1.Scale,result=k8s.io/api/autoscaling/v1.Scale

// The `Alertmanager` custom resource definition (CRD) defines a desired [Alertmanager](https://prometheus.io/docs/alerting) setup to run in a Kubernetes cluster. It allows to specify many options such as the number of replicas, persistent storage and many more.
//
// For each `Alertmanager` resource, the Operator deploys a `StatefulSet` in the same namespace. When there are two or more configured replicas, the Operator runs the Alertmanager instances in high-availability mode.
//
// The resource defines via label and namespace selectors which `AlertmanagerConfig` objects should be associated to the deployed Alertmanager instances.
type Alertmanager struct {
	// TypeMeta defines the versioned schema of this representation of an object.
	// +optional
	metav1.TypeMeta `json:",inline"`
	// metadata defines ObjectMeta as the metadata that all persisted resources.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// spec defines the specification of the desired behavior of the Alertmanager cluster. More info:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	// +required
	Spec AlertmanagerSpec `json:"spec"`
	// status defines the most recent observed status of the Alertmanager cluster. Read-only.
	// More info:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	// +optional
	Status AlertmanagerStatus `json:"status,omitempty"`
}

// DeepCopyObject implements the runtime.Object interface.
func (l *Alertmanager) DeepCopyObject() runtime.Object {
	return l.DeepCopy()
}

// AlertmanagerSpec is a specification of the desired behavior of the Alertmanager cluster. More info:
// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
// +k8s:openapi-gen=true
type AlertmanagerSpec struct {
	// podMetadata defines labels and annotations which are propagated to the Alertmanager pods.
	//
	// The following items are reserved and cannot be overridden:
	// * "alertmanager" label, set to the name of the Alertmanager instance.
	// * "app.kubernetes.io/instance" label, set to the name of the Alertmanager instance.
	// * "app.kubernetes.io/managed-by" label, set to "prometheus-operator".
	// * "app.kubernetes.io/name" label, set to "alertmanager".
	// * "app.kubernetes.io/version" label, set to the Alertmanager version.
	// * "kubectl.kubernetes.io/default-container" annotation, set to "alertmanager".
	// +optional
	PodMetadata *EmbeddedObjectMetadata `json:"podMetadata,omitempty"`
	// image if specified has precedence over baseImage, tag and sha
	// combinations. Specifying the version is still necessary to ensure the
	// Prometheus Operator knows what version of Alertmanager is being
	// configured.
	// +optional
	Image *string `json:"image,omitempty"`
	// imagePullPolicy for the 'alertmanager', 'init-config-reloader' and 'config-reloader' containers.
	// See https://kubernetes.io/docs/concepts/containers/images/#image-pull-policy for more details.
	// +kubebuilder:validation:Enum="";Always;Never;IfNotPresent
	// +optional
	ImagePullPolicy v1.PullPolicy `json:"imagePullPolicy,omitempty"`
	// version the cluster should be on.
	// +optional
	Version string `json:"version,omitempty"`
	// tag of Alertmanager container image to be deployed. Defaults to the value of `version`.
	// Version is ignored if Tag is set.
	// Deprecated: use 'image' instead. The image tag can be specified as part of the image URL.
	// +optional
	Tag string `json:"tag,omitempty"`
	// sha of Alertmanager container image to be deployed. Defaults to the value of `version`.
	// Similar to a tag, but the SHA explicitly deploys an immutable container image.
	// Version and Tag are ignored if SHA is set.
	// Deprecated: use 'image' instead. The image digest can be specified as part of the image URL.
	// +optional
	SHA string `json:"sha,omitempty"`
	// baseImage that is used to deploy pods, without tag.
	// Deprecated: use 'image' instead.
	// +optional
	BaseImage string `json:"baseImage,omitempty"`
	// imagePullSecrets An optional list of references to secrets in the same namespace
	// to use for pulling prometheus and alertmanager images from registries
	// see https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/
	// +optional
	ImagePullSecrets []v1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
	// secrets is a list of Secrets in the same namespace as the Alertmanager
	// object, which shall be mounted into the Alertmanager Pods.
	// Each Secret is added to the StatefulSet definition as a volume named `secret-<secret-name>`.
	// The Secrets are mounted into `/etc/alertmanager/secrets/<secret-name>` in the 'alertmanager' container.
	// +optional
	Secrets []string `json:"secrets,omitempty"`
	// configMaps defines a list of ConfigMaps in the same namespace as the Alertmanager
	// object, which shall be mounted into the Alertmanager Pods.
	// Each ConfigMap is added to the StatefulSet definition as a volume named `configmap-<configmap-name>`.
	// The ConfigMaps are mounted into `/etc/alertmanager/configmaps/<configmap-name>` in the 'alertmanager' container.
	// +optional
	ConfigMaps []string `json:"configMaps,omitempty"`
	// configSecret defines the name of a Kubernetes Secret in the same namespace as the
	// Alertmanager object, which contains the configuration for this Alertmanager
	// instance. If empty, it defaults to `alertmanager-<alertmanager-name>`.
	//
	// The Alertmanager configuration should be available under the
	// `alertmanager.yaml` key. Additional keys from the original secret are
	// copied to the generated secret and mounted into the
	// `/etc/alertmanager/config` directory in the `alertmanager` container.
	//
	// If either the secret or the `alertmanager.yaml` key is missing, the
	// operator provisions a minimal Alertmanager configuration with one empty
	// receiver (effectively dropping alert notifications).
	// +optional
	ConfigSecret string `json:"configSecret,omitempty"`
	// logLevel for Alertmanager to be configured with.
	// +kubebuilder:validation:Enum="";debug;info;warn;error
	// +optional
	LogLevel string `json:"logLevel,omitempty"`
	// logFormat for Alertmanager to be configured with.
	// +kubebuilder:validation:Enum="";logfmt;json
	// +optional
	LogFormat string `json:"logFormat,omitempty"`
	// replicas defines the expected size of the alertmanager cluster. The controller will
	// eventually make the size of the running cluster equal to the expected
	// size.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`
	// retention defines the time duration Alertmanager shall retain data for. Default is '120h',
	// and must match the regular expression `[0-9]+(ms|s|m|h)` (milliseconds seconds minutes hours).
	// +kubebuilder:default:="120h"
	// +optional
	Retention GoDuration `json:"retention,omitempty"`
	// storage defines the definition of how storage will be used by the Alertmanager
	// instances.
	// +optional
	Storage *StorageSpec `json:"storage,omitempty"`
	// volumes allows configuration of additional volumes on the output StatefulSet definition.
	// Volumes specified will be appended to other volumes that are generated as a result of
	// StorageSpec objects.
	// +optional
	Volumes []v1.Volume `json:"volumes,omitempty"`
	// volumeMounts allows configuration of additional VolumeMounts on the output StatefulSet definition.
	// VolumeMounts specified will be appended to other VolumeMounts in the alertmanager container,
	// that are generated as a result of StorageSpec objects.
	// +optional
	VolumeMounts []v1.VolumeMount `json:"volumeMounts,omitempty"`
	// persistentVolumeClaimRetentionPolicy controls if and how PVCs are deleted during the lifecycle of a StatefulSet.
	// The default behavior is all PVCs are retained.
	// This is an alpha field from kubernetes 1.23 until 1.26 and a beta field from 1.26.
	// It requires enabling the StatefulSetAutoDeletePVC feature gate.
	//
	// +optional
	PersistentVolumeClaimRetentionPolicy *appsv1.StatefulSetPersistentVolumeClaimRetentionPolicy `json:"persistentVolumeClaimRetentionPolicy,omitempty"`
	// externalUrl defines the URL used to access the Alertmanager web service. This is
	// necessary to generate correct URLs. This is necessary if Alertmanager is not
	// served from root of a DNS name.
	// +optional
	ExternalURL string `json:"externalUrl,omitempty"`
	// routePrefix Alertmanager registers HTTP handlers for. This is useful,
	// if using ExternalURL and a proxy is rewriting HTTP routes of a request,
	// and the actual ExternalURL is still true, but the server serves requests
	// under a different route prefix. For example for use with `kubectl proxy`.
	// +optional
	RoutePrefix string `json:"routePrefix,omitempty"`
	// paused if set to true all actions on the underlying managed objects are not
	// going to be performed, except for delete actions.
	// +optional
	Paused bool `json:"paused,omitempty"`
	// nodeSelector defines which Nodes the Pods are scheduled on.
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// resources defines the resource requests and limits of the Pods.
	// +optional
	Resources v1.ResourceRequirements `json:"resources,omitempty"`
	// affinity defines the pod's scheduling constraints.
	// +optional
	Affinity *v1.Affinity `json:"affinity,omitempty"`
	// tolerations defines the pod's tolerations.
	// +optional
	Tolerations []v1.Toleration `json:"tolerations,omitempty"`
	// topologySpreadConstraints defines the Pod's topology spread constraints.
	// +optional
	TopologySpreadConstraints []v1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`
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
	// enableServiceLinks defines whether information about services should be injected into pod's environment variables
	// +optional
	EnableServiceLinks *bool `json:"enableServiceLinks,omitempty"`
	// serviceName defines the service name used by the underlying StatefulSet(s) as the governing service.
	// If defined, the Service  must be created before the Alertmanager resource in the same namespace and it must define a selector that matches the pod labels.
	// If empty, the operator will create and manage a headless service named `alertmanager-operated` for Alertmanager resources.
	// When deploying multiple Alertmanager resources in the same namespace, it is recommended to specify a different value for each.
	// See https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/#stable-network-id for more details.
	// +optional
	// +kubebuilder:validation:MinLength=1
	ServiceName *string `json:"serviceName,omitempty"`
	// serviceAccountName is the name of the ServiceAccount to use to run the
	// Prometheus Pods.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
	// listenLocal defines the Alertmanager server listen on loopback, so that it
	// does not bind against the Pod IP. Note this is only for the Alertmanager
	// UI, not the gossip communication.
	// +optional
	ListenLocal bool `json:"listenLocal,omitempty"`
	// containers allows injecting additional containers. This is meant to
	// allow adding an authentication proxy to an Alertmanager pod.
	// Containers described here modify an operator generated container if they
	// share the same name and modifications are done via a strategic merge
	// patch. The current container names are: `alertmanager` and
	// `config-reloader`. Overriding containers is entirely outside the scope
	// of what the maintainers will support and by doing so, you accept that
	// this behaviour may break at any time without notice.
	// +optional
	Containers []v1.Container `json:"containers,omitempty"`
	// initContainers allows adding initContainers to the pod definition. Those can be used to e.g.
	// fetch secrets for injection into the Alertmanager configuration from external sources. Any
	// errors during the execution of an initContainer will lead to a restart of the Pod. More info: https://kubernetes.io/docs/concepts/workloads/pods/init-containers/
	// InitContainers described here modify an operator
	// generated init containers if they share the same name and modifications are
	// done via a strategic merge patch. The current init container name is:
	// `init-config-reloader`. Overriding init containers is entirely outside the
	// scope of what the maintainers will support and by doing so, you accept that
	// this behaviour may break at any time without notice.
	// +optional
	InitContainers []v1.Container `json:"initContainers,omitempty"`
	// priorityClassName assigned to the Pods
	// +optional
	PriorityClassName string `json:"priorityClassName,omitempty"`
	// additionalPeers allows injecting a set of additional Alertmanagers to peer with to form a highly available cluster.
	// +optional
	AdditionalPeers []string `json:"additionalPeers,omitempty"`
	// clusterAdvertiseAddress defines the explicit address to advertise in cluster.
	// Needs to be provided for non RFC1918 [1] (public) addresses.
	// [1] RFC1918: https://tools.ietf.org/html/rfc1918
	// +optional
	ClusterAdvertiseAddress string `json:"clusterAdvertiseAddress,omitempty"`
	// clusterGossipInterval defines the interval between gossip attempts.
	// +optional
	ClusterGossipInterval GoDuration `json:"clusterGossipInterval,omitempty"`
	// clusterLabel defines the identifier that uniquely identifies the Alertmanager cluster.
	// You should only set it when the Alertmanager cluster includes Alertmanager instances which are external to this Alertmanager resource. In practice, the addresses of the external instances are provided via the `.spec.additionalPeers` field.
	// +optional
	ClusterLabel *string `json:"clusterLabel,omitempty"`
	// clusterPushpullInterval defines the interval between pushpull attempts.
	// +optional
	ClusterPushpullInterval GoDuration `json:"clusterPushpullInterval,omitempty"`
	// clusterPeerTimeout defines the timeout for cluster peering.
	// +optional
	ClusterPeerTimeout GoDuration `json:"clusterPeerTimeout,omitempty"`
	// portName defines the port's name for the pods and governing service.
	// Defaults to `web`.
	// +kubebuilder:default:="web"
	// +optional
	PortName string `json:"portName,omitempty"`
	// forceEnableClusterMode ensures Alertmanager does not deactivate the cluster mode when running with a single replica.
	// Use case is e.g. spanning an Alertmanager cluster across Kubernetes clusters with a single replica in each.
	// +optional
	ForceEnableClusterMode bool `json:"forceEnableClusterMode,omitempty"`
	// alertmanagerConfigSelector defines the selector to be used for to merge and configure Alertmanager with.
	// +optional
	AlertmanagerConfigSelector *metav1.LabelSelector `json:"alertmanagerConfigSelector,omitempty"`
	// alertmanagerConfigNamespaceSelector defines the namespaces to be selected for AlertmanagerConfig discovery. If nil, only
	// check own namespace.
	// +optional
	AlertmanagerConfigNamespaceSelector *metav1.LabelSelector `json:"alertmanagerConfigNamespaceSelector,omitempty"`

	// alertmanagerConfigMatcherStrategy defines how AlertmanagerConfig objects
	// process incoming alerts.
	// +optional
	AlertmanagerConfigMatcherStrategy AlertmanagerConfigMatcherStrategy `json:"alertmanagerConfigMatcherStrategy,omitempty"`

	// minReadySeconds defines the minimum number of seconds for which a newly created pod should be ready
	// without any of its container crashing for it to be considered available.
	//
	// If unset, pods will be considered available as soon as they are ready.
	//
	// +kubebuilder:validation:Minimum:=0
	// +optional
	MinReadySeconds *int32 `json:"minReadySeconds,omitempty"`
	// hostAliases Pods configuration
	// +listType=map
	// +listMapKey=ip
	// +optional
	HostAliases []HostAlias `json:"hostAliases,omitempty"`
	// web defines the web command line flags when starting Alertmanager.
	// +optional
	Web *AlertmanagerWebSpec `json:"web,omitempty"`
	// limits defines the limits command line flags when starting Alertmanager.
	// +optional
	Limits *AlertmanagerLimitsSpec `json:"limits,omitempty"`
	// clusterTLS defines the mutual TLS configuration for the Alertmanager cluster's gossip protocol.
	//
	// It requires Alertmanager >= 0.24.0.
	// +optional
	ClusterTLS *ClusterTLSConfig `json:"clusterTLS,omitempty"`
	// alertmanagerConfiguration defines the configuration of Alertmanager.
	//
	// If defined, it takes precedence over the `configSecret` field.
	//
	// This is an *experimental feature*, it may change in any upcoming release
	// in a breaking way.
	//
	// +optional
	AlertmanagerConfiguration *AlertmanagerConfiguration `json:"alertmanagerConfiguration,omitempty"`
	// automountServiceAccountToken defines whether a service account token should be automatically mounted in the pod.
	// If the service account has `automountServiceAccountToken: true`, set the field to `false` to opt out of automounting API credentials.
	// +optional
	AutomountServiceAccountToken *bool `json:"automountServiceAccountToken,omitempty"`
	// enableFeatures defines the Alertmanager's feature flags. By default, no features are enabled.
	// Enabling features which are disabled by default is entirely outside the
	// scope of what the maintainers will support and by doing so, you accept
	// that this behaviour may break at any time without notice.
	//
	// It requires Alertmanager >= 0.27.0.
	// +optional
	EnableFeatures []string `json:"enableFeatures,omitempty"`
	// additionalArgs allows setting additional arguments for the 'Alertmanager' container.
	// It is intended for e.g. activating hidden flags which are not supported by
	// the dedicated configuration options yet. The arguments are passed as-is to the
	// Alertmanager container which may cause issues if they are invalid or not supported
	// by the given Alertmanager version.
	// +optional
	AdditionalArgs []Argument `json:"additionalArgs,omitempty"`

	// terminationGracePeriodSeconds defines the Optional duration in seconds the pod needs to terminate gracefully.
	// Value must be non-negative integer. The value zero indicates stop immediately via
	// the kill signal (no opportunity to shut down) which may lead to data corruption.
	//
	// Defaults to 120 seconds.
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

type AlertmanagerConfigMatcherStrategy struct {
	// type defines the strategy used by
	// AlertmanagerConfig objects to match alerts in the routes and inhibition
	// rules.
	//
	// The default value is `OnNamespace`.
	//
	// +kubebuilder:validation:Enum="OnNamespace";"OnNamespaceExceptForAlertmanagerNamespace";"None"
	// +kubebuilder:default:="OnNamespace"
	// +optional
	Type AlertmanagerConfigMatcherStrategyType `json:"type,omitempty"`
}

type AlertmanagerConfigMatcherStrategyType string

const (
	// With `OnNamespace`, the route and inhibition rules of an
	// AlertmanagerConfig object only process alerts that have a `namespace`
	// label equal to the namespace of the object.
	OnNamespaceConfigMatcherStrategyType AlertmanagerConfigMatcherStrategyType = "OnNamespace"

	// With `OnNamespaceExceptForAlertmanagerNamespace`, the route and inhibition rules of an
	// AlertmanagerConfig object only process alerts that have a `namespace`
	// label equal to the namespace of the object, unless the AlertmanagerConfig object
	// is in the same namespace as the Alertmanager object, where it will process all alerts.
	OnNamespaceExceptForAlertmanagerNamespaceConfigMatcherStrategyType AlertmanagerConfigMatcherStrategyType = "OnNamespaceExceptForAlertmanagerNamespace"

	// With `None`, the route and inhibition rules of an AlertmanagerConfig
	// object process all incoming alerts.
	NoneConfigMatcherStrategyType AlertmanagerConfigMatcherStrategyType = "None"
)

// AlertmanagerConfiguration defines the Alertmanager configuration.
// +k8s:openapi-gen=true
type AlertmanagerConfiguration struct {
	// name defines the name of the AlertmanagerConfig custom resource which is used to generate the Alertmanager configuration.
	// It must be defined in the same namespace as the Alertmanager object.
	// The operator will not enforce a `namespace` label for routes and inhibition rules.
	// +kubebuilder:validation:MinLength=1
	// +optional
	Name string `json:"name,omitempty"`
	// global defines the global parameters of the Alertmanager configuration.
	// +optional
	Global *AlertmanagerGlobalConfig `json:"global,omitempty"`
	// templates defines the custom notification templates.
	// +optional
	Templates []SecretOrConfigMap `json:"templates,omitempty"`
}

// AlertmanagerGlobalConfig configures parameters that are valid in all other configuration contexts.
// See https://prometheus.io/docs/alerting/latest/configuration/#configuration-file
type AlertmanagerGlobalConfig struct {
	// smtp defines global SMTP parameters.
	// +optional
	SMTPConfig *GlobalSMTPConfig `json:"smtp,omitempty"`

	// resolveTimeout defines the default value used by alertmanager if the alert does
	// not include EndsAt, after this time passes it can declare the alert as resolved if it has not been updated.
	// This has no impact on alerts from Prometheus, as they always include EndsAt.
	// +optional
	ResolveTimeout Duration `json:"resolveTimeout,omitempty"`

	// httpConfig defines the default HTTP configuration.
	// +optional
	HTTPConfig *HTTPConfig `json:"httpConfig,omitempty"`

	// slackApiUrl defines the default Slack API URL.
	// +optional
	SlackAPIURL *v1.SecretKeySelector `json:"slackApiUrl,omitempty"`

	// opsGenieApiUrl defines the default OpsGenie API URL.
	// +optional
	OpsGenieAPIURL *v1.SecretKeySelector `json:"opsGenieApiUrl,omitempty"`

	// opsGenieApiKey defines the default OpsGenie API Key.
	// +optional
	OpsGenieAPIKey *v1.SecretKeySelector `json:"opsGenieApiKey,omitempty"`

	// pagerdutyUrl defines the default Pagerduty URL.
	// +optional
	PagerdutyURL *URL `json:"pagerdutyUrl,omitempty"`

	// telegram defines the default Telegram config
	// +optional
	TelegramConfig *GlobalTelegramConfig `json:"telegram,omitempty"`

	// jira defines the default configuration for Jira.
	// +optional
	JiraConfig *GlobalJiraConfig `json:"jira,omitempty"`

	// victorops defines the default configuration for VictorOps.
	// +optional
	VictorOpsConfig *GlobalVictorOpsConfig `json:"victorops,omitempty"`

	// rocketChat defines the default configuration for Rocket Chat.
	// +optional
	RocketChatConfig *GlobalRocketChatConfig `json:"rocketChat,omitempty"`

	// webex defines the default configuration for Jira.
	// +optional
	WebexConfig *GlobalWebexConfig `json:"webex,omitempty"`

	// wechat defines the default WeChat Config
	// +optional
	WeChatConfig *GlobalWeChatConfig `json:"wechat,omitempty"`
}

// AlertmanagerStatus is the most recent observed status of the Alertmanager cluster. Read-only.
// More info:
// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
// +k8s:openapi-gen=true
type AlertmanagerStatus struct {
	// paused defines whether any actions on the underlying managed objects are
	// being performed. Only delete actions will be performed.
	// +optional
	Paused bool `json:"paused"`
	// replicas defines the total number of non-terminated pods targeted by this Alertmanager
	// object (their labels match the selector).
	// +optional
	Replicas int32 `json:"replicas"`
	// updatedReplicas defines the total number of non-terminated pods targeted by this Alertmanager
	// object that have the desired version spec.
	// +optional
	UpdatedReplicas int32 `json:"updatedReplicas"`
	// availableReplicas defines the total number of available pods (ready for at least minReadySeconds)
	// targeted by this Alertmanager cluster.
	// +optional
	AvailableReplicas int32 `json:"availableReplicas"`
	// unavailableReplicas defines the total number of unavailable pods targeted by this Alertmanager object.
	// +optional
	UnavailableReplicas int32 `json:"unavailableReplicas"`
	// selector used to match the pods targeted by this Alertmanager object.
	// +optional
	Selector string `json:"selector,omitempty"`
	// conditions defines the current state of the Alertmanager object.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []Condition `json:"conditions,omitempty"`
}

func (a *Alertmanager) ExpectedReplicas() int {
	if a.Spec.Replicas == nil {
		return 1
	}
	return int(*a.Spec.Replicas)
}

func (a *Alertmanager) SetReplicas(i int)            { a.Status.Replicas = int32(i) }
func (a *Alertmanager) SetUpdatedReplicas(i int)     { a.Status.UpdatedReplicas = int32(i) }
func (a *Alertmanager) SetAvailableReplicas(i int)   { a.Status.AvailableReplicas = int32(i) }
func (a *Alertmanager) SetUnavailableReplicas(i int) { a.Status.UnavailableReplicas = int32(i) }

// AlertmanagerWebSpec defines the web command line flags when starting Alertmanager.
// +k8s:openapi-gen=true
type AlertmanagerWebSpec struct {
	WebConfigFileFields `json:",inline"`
	// getConcurrency defines the maximum number of GET requests processed concurrently. This corresponds to the
	// Alertmanager's `--web.get-concurrency` flag.
	// +optional
	GetConcurrency *uint32 `json:"getConcurrency,omitempty"`
	// timeout for HTTP requests. This corresponds to the Alertmanager's
	// `--web.timeout` flag.
	// +optional
	Timeout *uint32 `json:"timeout,omitempty"`
}

// AlertmanagerLimitsSpec defines the limits command line flags when starting Alertmanager.
// +k8s:openapi-gen=true
type AlertmanagerLimitsSpec struct {
	// maxSilences defines the maximum number active and pending silences. This corresponds to the
	// Alertmanager's `--silences.max-silences` flag.
	// It requires Alertmanager >= v0.28.0.
	//
	// +kubebuilder:validation:Minimum:=0
	// +optional
	MaxSilences *int32 `json:"maxSilences,omitempty"`
	// maxPerSilenceBytes defines the maximum size of an individual silence as stored on disk. This corresponds to the Alertmanager's
	// `--silences.max-per-silence-bytes` flag.
	// It requires Alertmanager >= v0.28.0.
	//
	// +optional
	MaxPerSilenceBytes *ByteSize `json:"maxPerSilenceBytes,omitempty"`
}

// GlobalSMTPConfig configures global SMTP parameters.
// See https://prometheus.io/docs/alerting/latest/configuration/#configuration-file
type GlobalSMTPConfig struct {
	// from defines the default SMTP From header field.
	// +optional
	From *string `json:"from,omitempty"`

	// smartHost defines the default SMTP smarthost used for sending emails.
	// +optional
	SmartHost *HostPort `json:"smartHost,omitempty"`

	// hello defines the default hostname to identify to the SMTP server.
	// +optional
	Hello *string `json:"hello,omitempty"`

	// authUsername represents SMTP Auth using CRAM-MD5, LOGIN and PLAIN. If empty, Alertmanager doesn't authenticate to the SMTP server.
	// +optional
	AuthUsername *string `json:"authUsername,omitempty"`

	// authPassword represents SMTP Auth using LOGIN and PLAIN.
	// +optional
	AuthPassword *v1.SecretKeySelector `json:"authPassword,omitempty"`

	// authIdentity represents SMTP Auth using PLAIN
	// +optional
	AuthIdentity *string `json:"authIdentity,omitempty"`

	// authSecret represents SMTP Auth using CRAM-MD5.
	// +optional
	AuthSecret *v1.SecretKeySelector `json:"authSecret,omitempty"`

	// requireTLS defines the default SMTP TLS requirement.
	// Note that Go does not support unencrypted connections to remote SMTP endpoints.
	// +optional
	RequireTLS *bool `json:"requireTLS,omitempty"`

	// tlsConfig defines the default TLS configuration for SMTP receivers
	// +optional
	TLSConfig *SafeTLSConfig `json:"tlsConfig,omitempty"`
}

// GlobalTelegramConfig configures global Telegram parameters.
type GlobalTelegramConfig struct {
	// apiURL defines he default Telegram API URL.
	//
	// It requires Alertmanager >= v0.24.0.
	// +optional
	APIURL *URL `json:"apiURL,omitempty"`
}

// GlobalJiraConfig configures global Jira parameters.
type GlobalJiraConfig struct {
	// apiURL defines the default Jira API URL.
	//
	// It requires Alertmanager >= v0.28.0.
	//
	// +optional
	APIURL *URL `json:"apiURL,omitempty"`
}

// GlobalRocketChatConfig configures global Rocket Chat parameters.
type GlobalRocketChatConfig struct {
	// apiURL defines the default Rocket Chat API URL.
	//
	// It requires Alertmanager >= v0.28.0.
	//
	// +optional
	APIURL *URL `json:"apiURL,omitempty"`

	// token defines the default Rocket Chat token.
	//
	// It requires Alertmanager >= v0.28.0.
	//
	// +optional
	Token *v1.SecretKeySelector `json:"token,omitempty"`

	// tokenID defines the default Rocket Chat Token ID.
	//
	// It requires Alertmanager >= v0.28.0.
	//
	// +optional
	TokenID *v1.SecretKeySelector `json:"tokenID,omitempty"`
}

// GlobalWebexConfig configures global Webex parameters.
// See https://prometheus.io/docs/alerting/latest/configuration/#configuration-file
type GlobalWebexConfig struct {
	// apiURL defines the is the default Webex API URL.
	//
	// It requires Alertmanager >= v0.25.0.
	//
	// +optional
	APIURL *URL `json:"apiURL,omitempty"`
}

type GlobalWeChatConfig struct {
	// apiURL defines he default WeChat API URL.
	// The default value is "https://qyapi.weixin.qq.com/cgi-bin/"
	// +optional
	APIURL *URL `json:"apiURL,omitempty"`

	// apiSecret defines the default WeChat API Secret.
	// +optional
	APISecret *v1.SecretKeySelector `json:"apiSecret,omitempty"`

	// apiCorpID defines the default WeChat API Corporate ID.
	// +optional
	// +kubebuilder:validation:MinLength=1
	APICorpID *string `json:"apiCorpID,omitempty"`
}

// GlobalVictorOpsConfig configures global VictorOps parameters.
type GlobalVictorOpsConfig struct {
	// apiURL defines the default VictorOps API URL.
	//
	// +optional
	APIURL *URL `json:"apiURL,omitempty"`
	// apiKey defines the default VictorOps API Key.
	//
	// +optional
	APIKey *v1.SecretKeySelector `json:"apiKey,omitempty"`
}

// HostPort represents a "host:port" network address.
type HostPort struct {
	// host defines the host's address, it can be a DNS name or a literal IP address.
	// +kubebuilder:validation:MinLength=1
	// +required
	Host string `json:"host"`
	// port defines the host's port, it can be a literal port number or a port name.
	// +kubebuilder:validation:MinLength=1
	// +required
	Port string `json:"port"`
}

// AlertmanagerList is a list of Alertmanagers.
// +k8s:openapi-gen=true
type AlertmanagerList struct {
	// TypeMeta defines the versioned schema of this representation of an object.
	metav1.TypeMeta `json:",inline"`
	// metadata defines ListMeta as metadata for collection responses.
	metav1.ListMeta `json:"metadata,omitempty"`
	// List of Alertmanagers
	Items []Alertmanager `json:"items"`
}

// DeepCopyObject implements the runtime.Object interface.
func (l *AlertmanagerList) DeepCopyObject() runtime.Object {
	return l.DeepCopy()
}

// ClusterTLSConfig defines the mutual TLS configuration for the Alertmanager cluster TLS protocol.
// +k8s:openapi-gen=true
type ClusterTLSConfig struct {
	// server defines the server-side configuration for mutual TLS.
	// +required
	ServerTLS WebTLSConfig `json:"server"`
	// client defines the client-side configuration for mutual TLS.
	// +required
	ClientTLS SafeTLSConfig `json:"client"`
}

// URL represents a valid URL
// +kubebuilder:validation:Pattern:="^(http|https)://.+$"
type URL string
