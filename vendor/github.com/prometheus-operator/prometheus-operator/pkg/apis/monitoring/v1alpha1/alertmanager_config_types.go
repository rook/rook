// Copyright 2020 The prometheus-operator Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v1alpha1

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"regexp"
	"strings"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"

	v1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	Version = "v1alpha1"

	AlertmanagerConfigKind    = "AlertmanagerConfig"
	AlertmanagerConfigName    = "alertmanagerconfigs"
	AlertmanagerConfigKindKey = "alertmanagerconfig"
)

// +genclient
// +k8s:openapi-gen=true
// +kubebuilder:resource:categories="prometheus-operator",shortName="amcfg"
// +kubebuilder:storageversion

// AlertmanagerConfig configures the Prometheus Alertmanager,
// specifying how alerts should be grouped, inhibited and notified to external systems.
type AlertmanagerConfig struct {
	// TypeMeta defines the versioned schema of this representation of an object.
	metav1.TypeMeta `json:",inline"`
	// metadata defines ObjectMeta as the metadata that all persisted resources.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// spec defines the specification of AlertmanagerConfigSpec
	// +required
	Spec AlertmanagerConfigSpec `json:"spec"`
}

// AlertmanagerConfigList is a list of AlertmanagerConfig.
// +k8s:openapi-gen=true
type AlertmanagerConfigList struct {
	// TypeMeta defines the versioned schema of this representation of an object.
	metav1.TypeMeta `json:",inline"`
	// metadata defines ListMeta as metadata for collection responses.
	metav1.ListMeta `json:"metadata,omitempty"`
	// List of AlertmanagerConfig
	Items []AlertmanagerConfig `json:"items"`
}

// AlertmanagerConfigSpec is a specification of the desired behavior of the
// Alertmanager configuration.
// By default, the Alertmanager configuration only applies to alerts for which
// the `namespace` label is equal to the namespace of the AlertmanagerConfig
// resource (see the `.spec.alertmanagerConfigMatcherStrategy` field of the
// Alertmanager CRD).
type AlertmanagerConfigSpec struct {
	// route defines the Alertmanager route definition for alerts matching the resource's
	// namespace. If present, it will be added to the generated Alertmanager
	// configuration as a first-level route.
	// +optional
	Route *Route `json:"route"`
	// receivers defines the list of receivers.
	// +optional
	Receivers []Receiver `json:"receivers"`
	// inhibitRules defines the list of inhibition rules. The rules will only apply to alerts matching
	// the resource's namespace.
	// +optional
	InhibitRules []InhibitRule `json:"inhibitRules,omitempty"`
	// muteTimeIntervals defines the list of MuteTimeInterval specifying when the routes should be muted.
	// +optional
	MuteTimeIntervals []MuteTimeInterval `json:"muteTimeIntervals,omitempty"`
}

// Route defines a node in the routing tree.
type Route struct {
	// receiver defines the name of the receiver for this route. If not empty, it should be listed in
	// the `receivers` field.
	// +optional
	Receiver string `json:"receiver"`
	// groupBy defines the list of labels to group by.
	// Labels must not be repeated (unique list).
	// Special label "..." (aggregate by all possible labels), if provided, must be the only element in the list.
	// +optional
	GroupBy []string `json:"groupBy,omitempty"`
	// groupWait defines how long to wait before sending the initial notification.
	// Must match the regular expression`^(([0-9]+)y)?(([0-9]+)w)?(([0-9]+)d)?(([0-9]+)h)?(([0-9]+)m)?(([0-9]+)s)?(([0-9]+)ms)?$`
	// Example: "30s"
	// +optional
	GroupWait string `json:"groupWait,omitempty"`
	// groupInterval defines how long to wait before sending an updated notification.
	// Must match the regular expression`^(([0-9]+)y)?(([0-9]+)w)?(([0-9]+)d)?(([0-9]+)h)?(([0-9]+)m)?(([0-9]+)s)?(([0-9]+)ms)?$`
	// Example: "5m"
	// +optional
	GroupInterval string `json:"groupInterval,omitempty"`
	// repeatInterval defines how long to wait before repeating the last notification.
	// Must match the regular expression`^(([0-9]+)y)?(([0-9]+)w)?(([0-9]+)d)?(([0-9]+)h)?(([0-9]+)m)?(([0-9]+)s)?(([0-9]+)ms)?$`
	// Example: "4h"
	// +optional
	RepeatInterval string `json:"repeatInterval,omitempty"`
	// matchers defines the list of matchers that the alert's labels should match. For the first
	// level route, the operator removes any existing equality and regexp
	// matcher on the `namespace` label and adds a `namespace: <object
	// namespace>` matcher.
	// +optional
	Matchers []Matcher `json:"matchers,omitempty"`
	// continue defines the boolean indicating whether an alert should continue matching subsequent
	// sibling nodes. It will always be overridden to true for the first-level
	// route by the Prometheus operator.
	// +optional
	Continue bool `json:"continue,omitempty"`
	// routes defines the child routes.
	// +optional
	Routes []apiextensionsv1.JSON `json:"routes,omitempty"`
	// Note: this comment applies to the field definition above but appears
	// below otherwise it gets included in the generated manifest.
	// CRD schema doesn't support self-referential types for now (see
	// https://github.com/kubernetes/kubernetes/issues/62872). We have to use
	// an alternative type to circumvent the limitation. The downside is that
	// the Kube API can't validate the data beyond the fact that it is a valid
	// JSON representation.

	// muteTimeIntervals is a list of MuteTimeInterval names that will mute this route when matched,
	// +optional
	MuteTimeIntervals []string `json:"muteTimeIntervals,omitempty"`
	// activeTimeIntervals is a list of MuteTimeInterval names when this route should be active.
	// +optional
	ActiveTimeIntervals []string `json:"activeTimeIntervals,omitempty"`
}

// ChildRoutes extracts the child routes.
func (r *Route) ChildRoutes() ([]Route, error) {
	out := make([]Route, len(r.Routes))

	for i, v := range r.Routes {
		dec := json.NewDecoder(bytes.NewBuffer(v.Raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&out[i]); err != nil {
			return nil, fmt.Errorf("route[%d]: %w", i, err)
		}
	}

	return out, nil
}

// Receiver defines one or more notification integrations.
type Receiver struct {
	// name defines the name of the receiver. Must be unique across all items from the list.
	// +kubebuilder:validation:MinLength=1
	// +required
	Name string `json:"name"`
	// opsgenieConfigs defines the list of OpsGenie configurations.
	// +optional
	OpsGenieConfigs []OpsGenieConfig `json:"opsgenieConfigs,omitempty"`
	// pagerdutyConfigs defines the List of PagerDuty configurations.
	// +optional
	PagerDutyConfigs []PagerDutyConfig `json:"pagerdutyConfigs,omitempty"`
	// discordConfigs defines the list of Slack configurations.
	// +optional
	DiscordConfigs []DiscordConfig `json:"discordConfigs,omitempty"`
	// slackConfigs defines the list of Slack configurations.
	// +optional
	SlackConfigs []SlackConfig `json:"slackConfigs,omitempty"`
	// webhookConfigs defines the List of webhook configurations.
	// +optional
	WebhookConfigs []WebhookConfig `json:"webhookConfigs,omitempty"`
	// wechatConfigs defines the list of WeChat configurations.
	// +optional
	WeChatConfigs []WeChatConfig `json:"wechatConfigs,omitempty"`
	// emailConfigs defines the list of Email configurations.
	// +optional
	EmailConfigs []EmailConfig `json:"emailConfigs,omitempty"`
	// victoropsConfigs defines the list of VictorOps configurations.
	// +optional
	VictorOpsConfigs []VictorOpsConfig `json:"victoropsConfigs,omitempty"`
	// pushoverConfigs defines the list of Pushover configurations.
	// +optional
	PushoverConfigs []PushoverConfig `json:"pushoverConfigs,omitempty"`
	// snsConfigs defines the list of SNS configurations
	// +optional
	SNSConfigs []SNSConfig `json:"snsConfigs,omitempty"`
	// telegramConfigs defines the list of Telegram configurations.
	// +optional
	TelegramConfigs []TelegramConfig `json:"telegramConfigs,omitempty"`
	// webexConfigs defines the list of Webex configurations.
	// +optional
	WebexConfigs []WebexConfig `json:"webexConfigs,omitempty"`
	// msteamsConfigs defines the list of MSTeams configurations.
	// It requires Alertmanager >= 0.26.0.
	// +optional
	MSTeamsConfigs []MSTeamsConfig `json:"msteamsConfigs,omitempty"`
	// msteamsv2Configs defines the list of MSTeamsV2 configurations.
	// It requires Alertmanager >= 0.28.0.
	// +optional
	MSTeamsV2Configs []MSTeamsV2Config `json:"msteamsv2Configs,omitempty"`
	// rocketchatConfigs defines the list of RocketChat configurations.
	// It requires Alertmanager >= 0.28.0.
	// +optional
	RocketChatConfigs []RocketChatConfig `json:"rocketchatConfigs,omitempty"`
}

// PagerDutyConfig configures notifications via PagerDuty.
// See https://prometheus.io/docs/alerting/latest/configuration/#pagerduty_config
type PagerDutyConfig struct {
	// sendResolved defines whether or not to notify about resolved alerts.
	// +optional
	SendResolved *bool `json:"sendResolved,omitempty"`
	// routingKey defines the secret's key that contains the PagerDuty integration key (when using
	// Events API v2). Either this field or `serviceKey` needs to be defined.
	// The secret needs to be in the same namespace as the AlertmanagerConfig
	// object and accessible by the Prometheus Operator.
	// +optional
	RoutingKey *v1.SecretKeySelector `json:"routingKey,omitempty"`
	// serviceKey defines the secret's key that contains the PagerDuty service key (when using
	// integration type "Prometheus"). Either this field or `routingKey` needs to
	// be defined.
	// The secret needs to be in the same namespace as the AlertmanagerConfig
	// object and accessible by the Prometheus Operator.
	// +optional
	ServiceKey *v1.SecretKeySelector `json:"serviceKey,omitempty"`
	// url defines the URL to send requests to.
	// +optional
	URL string `json:"url,omitempty"`
	// client defines the client identification.
	// +optional
	Client string `json:"client,omitempty"`
	// clientURL defines the backlink to the sender of notification.
	// +optional
	ClientURL string `json:"clientURL,omitempty"`
	// description of the incident.
	// +optional
	Description string `json:"description,omitempty"`
	// severity of the incident.
	// +optional
	Severity string `json:"severity,omitempty"`
	// class defines the class/type of the event.
	// +optional
	Class string `json:"class,omitempty"`
	// group defines a cluster or grouping of sources.
	// +optional
	Group string `json:"group,omitempty"`
	// component defines the part or component of the affected system that is broken.
	// +optional
	Component string `json:"component,omitempty"`
	// details defines the arbitrary key/value pairs that provide further detail about the incident.
	// +optional
	Details []KeyValue `json:"details,omitempty"`
	// pagerDutyImageConfigs defines a list of image details to attach that provide further detail about an incident.
	// +optional
	PagerDutyImageConfigs []PagerDutyImageConfig `json:"pagerDutyImageConfigs,omitempty"`
	// pagerDutyLinkConfigs defines a list of link details to attach that provide further detail about an incident.
	// +optional
	PagerDutyLinkConfigs []PagerDutyLinkConfig `json:"pagerDutyLinkConfigs,omitempty"`
	// httpConfig defines the HTTP client configuration.
	// +optional
	HTTPConfig *HTTPConfig `json:"httpConfig,omitempty"`
	// source defines the unique location of the affected system.
	// +optional
	Source *string `yaml:"source,omitempty" json:"source,omitempty"`
}

// PagerDutyImageConfig attaches images to an incident
type PagerDutyImageConfig struct {
	// src of the image being attached to the incident
	// +optional
	Src string `json:"src,omitempty"`
	// href defines the optional URL; makes the image a clickable link.
	// +optional
	Href string `json:"href,omitempty"`
	// alt is the optional alternative text for the image.
	// +optional
	Alt string `json:"alt,omitempty"`
}

// PagerDutyLinkConfig attaches text links to an incident
type PagerDutyLinkConfig struct {
	// href defines the URL of the link to be attached
	// +optional
	Href string `json:"href,omitempty"`
	// alt defines the text that describes the purpose of the link, and can be used as the link's text.
	// +optional
	Text string `json:"alt,omitempty"`
}

// DiscordConfig configures notifications via Discord.
// See https://prometheus.io/docs/alerting/latest/configuration/#discord_config
type DiscordConfig struct {
	// sendResolved defines whether or not to notify about resolved alerts.
	// +optional
	SendResolved *bool `json:"sendResolved,omitempty"`
	// apiURL defines the secret's key that contains the Discord webhook URL.
	// The secret needs to be in the same namespace as the AlertmanagerConfig
	// object and accessible by the Prometheus Operator.
	// +required
	APIURL v1.SecretKeySelector `json:"apiURL"`
	// title defines the template of the message's title.
	// +optional
	Title *string `json:"title,omitempty"`
	// message defines the template of the message's body.
	// +optional
	Message *string `json:"message,omitempty"`
	// content defines the template of the content's body.
	// +optional
	// +kubebuilder:validation:MinLength=1
	Content *string `json:"content,omitempty"`
	// username defines the username of the message sender.
	// +optional
	// +kubebuilder:validation:MinLength=1
	Username *string `json:"username,omitempty"`
	// avatarURL defines the avatar url of the message sender.
	// +optional
	AvatarURL *URL `json:"avatarURL,omitempty"`
	// httpConfig defines the HTTP client configuration.
	// +optional
	HTTPConfig *HTTPConfig `json:"httpConfig,omitempty"`
}

// SlackConfig configures notifications via Slack.
// See https://prometheus.io/docs/alerting/latest/configuration/#slack_config
type SlackConfig struct {
	// sendResolved defines whether or not to notify about resolved alerts.
	// +optional
	SendResolved *bool `json:"sendResolved,omitempty"`
	// apiURL defines the secret's key that contains the Slack webhook URL.
	// The secret needs to be in the same namespace as the AlertmanagerConfig
	// object and accessible by the Prometheus Operator.
	// +optional
	APIURL *v1.SecretKeySelector `json:"apiURL,omitempty"`
	// channel defines the channel or user to send notifications to.
	// +optional
	Channel string `json:"channel,omitempty"`
	// username defines the slack bot user name.
	// +optional
	Username string `json:"username,omitempty"`
	// color defines the color of the left border of the Slack message attachment.
	// Can be a hex color code (e.g., "#ff0000") or a predefined color name.
	// +optional
	Color string `json:"color,omitempty"`
	// title defines the title text displayed in the Slack message attachment.
	// +optional
	Title string `json:"title,omitempty"`
	// titleLink defines the URL that the title will link to when clicked.
	// +optional
	TitleLink string `json:"titleLink,omitempty"`
	// pretext defines optional text that appears above the message attachment block.
	// +optional
	Pretext string `json:"pretext,omitempty"`
	// text defines the main text content of the Slack message attachment.
	// +optional
	Text string `json:"text,omitempty"`
	// fields defines a list of Slack fields that are sent with each notification.
	// +optional
	Fields []SlackField `json:"fields,omitempty"`
	// shortFields determines whether fields are displayed in a compact format.
	// When true, fields are shown side by side when possible.
	// +optional
	ShortFields bool `json:"shortFields,omitempty"`
	// footer defines small text displayed at the bottom of the message attachment.
	// +optional
	Footer string `json:"footer,omitempty"`
	// fallback defines a plain-text summary of the attachment for clients that don't support attachments.
	// +optional
	Fallback string `json:"fallback,omitempty"`
	// callbackId defines an identifier for the message used in interactive components.
	// +optional
	CallbackID string `json:"callbackId,omitempty"`
	// iconEmoji defines the emoji to use as the bot's avatar (e.g., ":ghost:").
	// +optional
	IconEmoji string `json:"iconEmoji,omitempty"`
	// iconURL defines the URL to an image to use as the bot's avatar.
	// +optional
	IconURL string `json:"iconURL,omitempty"`
	// imageURL defines the URL to an image file that will be displayed inside the message attachment.
	// +optional
	ImageURL string `json:"imageURL,omitempty"`
	// thumbURL defines the URL to an image file that will be displayed as a thumbnail
	// on the right side of the message attachment.
	// +optional
	ThumbURL string `json:"thumbURL,omitempty"`
	// linkNames enables automatic linking of channel names and usernames in the message.
	// When true, @channel and @username will be converted to clickable links.
	// +optional
	LinkNames bool `json:"linkNames,omitempty"`
	// mrkdwnIn defines which fields should be parsed as Slack markdown.
	// Valid values include "pretext", "text", and "fields".
	// +optional
	MrkdwnIn []string `json:"mrkdwnIn,omitempty"`
	// actions defines a list of Slack actions that are sent with each notification.
	// +optional
	Actions []SlackAction `json:"actions,omitempty"`
	// httpConfig defines the HTTP client configuration.
	// +optional
	HTTPConfig *HTTPConfig `json:"httpConfig,omitempty"`
}

// Validate ensures SlackConfig is valid.
func (sc *SlackConfig) Validate() error {
	for _, action := range sc.Actions {
		if err := action.Validate(); err != nil {
			return err
		}
	}

	for _, field := range sc.Fields {
		if err := field.Validate(); err != nil {
			return err
		}
	}

	return nil
}

// SlackAction configures a single Slack action that is sent with each
// notification.
// See https://api.slack.com/docs/message-attachments#action_fields and
// https://api.slack.com/docs/message-buttons for more information.
type SlackAction struct {
	// type defines the type of interactive component.
	// Common values include "button" for clickable buttons and "select" for dropdown menus.
	// +kubebuilder:validation:MinLength=1
	// +required
	Type string `json:"type"`
	// text defines the user-visible label displayed on the action element.
	// For buttons, this is the button text. For select menus, this is the placeholder text.
	// +kubebuilder:validation:MinLength=1
	// +required
	Text string `json:"text"`
	// url defines the URL to open when the action is triggered.
	// Only applicable for button-type actions. When set, clicking the button opens this URL.
	// +optional
	URL string `json:"url,omitempty"`
	// style defines the visual appearance of the action element.
	// Valid values include "default", "primary" (green), and "danger" (red).
	// +optional
	Style string `json:"style,omitempty"`
	// name defines a unique identifier for the action within the message.
	// This value is sent back to your application when the action is triggered.
	// +optional
	Name string `json:"name,omitempty"`
	// value defines the payload sent when the action is triggered.
	// This data is included in the callback sent to your application.
	// +optional
	Value string `json:"value,omitempty"`
	// confirm defines an optional confirmation dialog that appears before the action is executed.
	// When set, users must confirm their intent before the action proceeds.
	// +optional
	ConfirmField *SlackConfirmationField `json:"confirm,omitempty"`
}

// Validate ensures SlackAction is valid.
func (sa *SlackAction) Validate() error {
	if sa.Type == "" {
		return errors.New("missing type in Slack action configuration")
	}

	if sa.Text == "" {
		return errors.New("missing text in Slack action configuration")
	}

	if sa.URL == "" && sa.Name == "" {
		return errors.New("missing name or url in Slack action configuration")
	}

	if sa.ConfirmField != nil {
		if err := sa.ConfirmField.Validate(); err != nil {
			return err
		}
	}

	return nil
}

// SlackConfirmationField protect users from destructive actions or
// particularly distinguished decisions by asking them to confirm their button
// click one more time.
// See https://api.slack.com/docs/interactive-message-field-guide#confirmation_fields
// for more information.
type SlackConfirmationField struct {
	// text defines the main message displayed in the confirmation dialog.
	// This should be a clear question or statement asking the user to confirm their action.
	// +kubebuilder:validation:MinLength=1
	// +required
	Text string `json:"text"`
	// title defines the title text displayed at the top of the confirmation dialog.
	// When not specified, a default title will be used.
	// +optional
	Title string `json:"title,omitempty"`
	// okText defines the label for the confirmation button in the dialog.
	// When not specified, defaults to "Okay". This button proceeds with the action.
	// +optional
	OkText string `json:"okText,omitempty"`
	// dismissText defines the label for the cancel button in the dialog.
	// When not specified, defaults to "Cancel". This button cancels the action.
	// +optional
	DismissText string `json:"dismissText,omitempty"`
}

// Validate ensures SlackConfirmationField is valid.
func (scf *SlackConfirmationField) Validate() error {
	if scf.Text == "" {
		return errors.New("missing text in Slack confirmation configuration")
	}
	return nil
}

// SlackField configures a single Slack field that is sent with each notification.
// Each field must contain a title, value, and optionally, a boolean value to indicate if the field
// is short enough to be displayed next to other fields designated as short.
// See https://api.slack.com/docs/message-attachments#fields for more information.
type SlackField struct {
	// title defines the label or header text displayed for this field.
	// This appears as bold text above the field value in the Slack message.
	// +kubebuilder:validation:MinLength=1
	// +required
	Title string `json:"title"`
	// value defines the content or data displayed for this field.
	// This appears below the title and can contain plain text or Slack markdown.
	// +kubebuilder:validation:MinLength=1
	// +required
	Value string `json:"value"`
	// short determines whether this field can be displayed alongside other short fields.
	// When true, Slack may display this field side by side with other short fields.
	// When false or not specified, the field takes the full width of the message.
	// +optional
	Short *bool `json:"short,omitempty"`
}

// Validate ensures SlackField is valid
func (sf *SlackField) Validate() error {
	if sf.Title == "" {
		return errors.New("missing title in Slack field configuration")
	}

	if sf.Value == "" {
		return errors.New("missing value in Slack field configuration")
	}

	return nil
}

// WebhookConfig configures notifications via a generic receiver supporting the webhook payload.
// See https://prometheus.io/docs/alerting/latest/configuration/#webhook_config
type WebhookConfig struct {
	// sendResolved defines whether or not to notify about resolved alerts.
	// +optional
	SendResolved *bool `json:"sendResolved,omitempty"`
	// url defines the URL to send HTTP POST requests to.
	// urlSecret takes precedence over url. One of urlSecret and url should be defined.
	// +optional
	URL *string `json:"url,omitempty"`
	// urlSecret defines the secret's key that contains the webhook URL to send HTTP requests to.
	// urlSecret takes precedence over url. One of urlSecret and url should be defined.
	// The secret needs to be in the same namespace as the AlertmanagerConfig
	// object and accessible by the Prometheus Operator.
	// +optional
	URLSecret *v1.SecretKeySelector `json:"urlSecret,omitempty"`
	// httpConfig defines the HTTP client configuration for webhook requests.
	// +optional
	HTTPConfig *HTTPConfig `json:"httpConfig,omitempty"`
	// maxAlerts defines the maximum number of alerts to be sent per webhook message.
	// When 0, all alerts are included in the webhook payload.
	// +optional
	// +kubebuilder:validation:Minimum=0
	MaxAlerts int32 `json:"maxAlerts,omitempty"`
	// timeout defines the maximum time to wait for a webhook request to complete,
	// before failing the request and allowing it to be retried.
	// It requires Alertmanager >= v0.28.0.
	// +optional
	Timeout *monitoringv1.Duration `json:"timeout,omitempty"`
}

// OpsGenieConfig configures notifications via OpsGenie.
// See https://prometheus.io/docs/alerting/latest/configuration/#opsgenie_config
type OpsGenieConfig struct {
	// sendResolved defines whether or not to notify about resolved alerts.
	// +optional
	SendResolved *bool `json:"sendResolved,omitempty"`
	// apiKey defines the secret's key that contains the OpsGenie API key.
	// The secret needs to be in the same namespace as the AlertmanagerConfig
	// object and accessible by the Prometheus Operator.
	// +optional
	APIKey *v1.SecretKeySelector `json:"apiKey,omitempty"`
	// apiURL defines the URL to send OpsGenie API requests to.
	// When not specified, defaults to the standard OpsGenie API endpoint.
	// +optional
	APIURL string `json:"apiURL,omitempty"`
	// message defines the alert text limited to 130 characters.
	// This appears as the main alert title in OpsGenie.
	// +optional
	Message string `json:"message,omitempty"`
	// description defines the detailed description of the incident.
	// This provides additional context beyond the message field.
	// +optional
	Description string `json:"description,omitempty"`
	// source defines the backlink to the sender of the notification.
	// This helps identify where the alert originated from.
	// +optional
	Source string `json:"source,omitempty"`
	// tags defines a comma separated list of tags attached to the notifications.
	// These help categorize and filter alerts within OpsGenie.
	// +optional
	Tags string `json:"tags,omitempty"`
	// note defines an additional alert note.
	// This provides supplementary information about the alert.
	// +optional
	Note string `json:"note,omitempty"`
	// priority defines the priority level of alert.
	// Possible values are P1, P2, P3, P4, and P5, where P1 is highest priority.
	// +optional
	Priority string `json:"priority,omitempty"`
	// updateAlerts defines Whether to update message and description of the alert in OpsGenie if it already exists
	// By default, the alert is never updated in OpsGenie, the new message only appears in activity log.
	// +optional
	UpdateAlerts *bool `json:"updateAlerts,omitempty"`
	// details defines a set of arbitrary key/value pairs that provide further detail about the incident.
	// These appear as additional fields in the OpsGenie alert.
	// +optional
	Details []KeyValue `json:"details,omitempty"`
	// responders defines the list of responders responsible for notifications.
	// These determine who gets notified when the alert is created.
	// +optional
	Responders []OpsGenieConfigResponder `json:"responders,omitempty"`
	// httpConfig defines the HTTP client configuration for OpsGenie API requests.
	// +optional
	HTTPConfig *HTTPConfig `json:"httpConfig,omitempty"`
	// entity defines an optional field that can be used to specify which domain alert is related to.
	// This helps group related alerts together in OpsGenie.
	// +optional
	Entity string `json:"entity,omitempty"`
	// actions defines a comma separated list of actions that will be available for the alert.
	// These appear as action buttons in the OpsGenie interface.
	// +optional
	Actions string `json:"actions,omitempty"`
}

// Validate ensures OpsGenieConfig is valid
func (o *OpsGenieConfig) Validate() error {
	for _, responder := range o.Responders {
		if err := responder.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// OpsGenieConfigResponder defines a responder to an incident.
// One of `id`, `name` or `username` has to be defined.
type OpsGenieConfigResponder struct {
	// id defines the unique identifier of the responder.
	// This corresponds to the responder's ID within OpsGenie.
	// +optional
	ID string `json:"id,omitempty"`
	// name defines the display name of the responder.
	// This is used when the responder is identified by name rather than ID.
	// +optional
	Name string `json:"name,omitempty"`
	// username defines the username of the responder.
	// This is typically used for user-type responders when identifying by username.
	// +optional
	Username string `json:"username,omitempty"`
	// type defines the type of responder.
	// Valid values include "user", "team", "schedule", and "escalation".
	// This determines how OpsGenie interprets the other identifier fields.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Enum=team;teams;user;escalation;schedule
	// +required
	Type string `json:"type"`
}

const opsgenieValidTypesRe = `^(team|teams|user|escalation|schedule)$`

var opsgenieTypeMatcher = regexp.MustCompile(opsgenieValidTypesRe)

// Validate ensures OpsGenieConfigResponder is valid.
func (r *OpsGenieConfigResponder) Validate() error {
	if r.ID == "" && r.Name == "" && r.Username == "" {
		return errors.New("responder must have at least an ID, a Name or an Username defined")
	}

	if strings.Contains(r.Type, "{{") {
		_, err := template.New("").Parse(r.Type)
		if err != nil {
			return fmt.Errorf("responder %v type is not a valid template: %w", r, err)
		}
		return nil
	}

	if opsgenieTypeMatcher.MatchString(strings.ToLower(r.Type)) {
		return nil
	}
	return fmt.Errorf("opsGenieConfig responder %v type does not match valid options %s", r, opsgenieValidTypesRe)
}

// HTTPConfig defines a client HTTP configuration.
// See https://prometheus.io/docs/alerting/latest/configuration/#http_config
type HTTPConfig struct {
	// authorization defines the authorization header configuration for the client.
	// This is mutually exclusive with BasicAuth and is only available starting from Alertmanager v0.22+.
	// +optional
	Authorization *monitoringv1.SafeAuthorization `json:"authorization,omitempty"`
	// basicAuth defines the basic authentication credentials for the client.
	// This is mutually exclusive with Authorization. If both are defined, BasicAuth takes precedence.
	// +optional
	BasicAuth *monitoringv1.BasicAuth `json:"basicAuth,omitempty"`
	// oauth2 defines the OAuth2 client credentials used to fetch a token for the targets.
	// This enables OAuth2 authentication flow for HTTP requests.
	// +optional
	OAuth2 *monitoringv1.OAuth2 `json:"oauth2,omitempty"`
	// bearerTokenSecret defines the secret's key that contains the bearer token to be used by the client
	// for authentication.
	// The secret needs to be in the same namespace as the AlertmanagerConfig
	// object and accessible by the Prometheus Operator.
	// +optional
	BearerTokenSecret *v1.SecretKeySelector `json:"bearerTokenSecret,omitempty"`
	// tlsConfig defines the TLS configuration for the client.
	// This includes settings for certificates, CA validation, and TLS protocol options.
	// +optional
	TLSConfig *monitoringv1.SafeTLSConfig `json:"tlsConfig,omitempty"`

	// proxyURL defines an optional proxy URL for HTTP requests.
	// If defined, this field takes precedence over `proxyUrl`.
	//
	// +optional
	ProxyURLOriginal *string `json:"proxyURL,omitempty"`

	monitoringv1.ProxyConfig `json:",inline"`

	// followRedirects specifies whether the client should follow HTTP 3xx redirects.
	// When true, the client will automatically follow redirect responses.
	// +optional
	FollowRedirects *bool `json:"followRedirects,omitempty"`

	// enableHttp2 can be used to disable HTTP2.
	//
	// +optional
	EnableHTTP2 *bool `json:"enableHttp2,omitempty"`
}

// WebexConfig configures notification via Cisco Webex
// See https://prometheus.io/docs/alerting/latest/configuration/#webex_config
type WebexConfig struct {
	// sendResolved defines whether or not to notify about resolved alerts.
	// +optional
	SendResolved *bool `json:"sendResolved,omitempty"`

	// apiURL defines the Webex Teams API URL i.e. https://webexapis.com/v1/messages
	// +optional
	APIURL *URL `json:"apiURL,omitempty"`

	// httpConfig defines the HTTP client's configuration.
	// +optional
	HTTPConfig *HTTPConfig `json:"httpConfig,omitempty"`

	// message defines the message template
	// +optional
	Message *string `json:"message,omitempty"`

	// roomID defines the ID of the Webex Teams room where to send the messages.
	// +kubebuilder:validation:MinLength=1
	// +required
	RoomID string `json:"roomID"`
}

// WeChatConfig configures notifications via WeChat.
// See https://prometheus.io/docs/alerting/latest/configuration/#wechat_config
type WeChatConfig struct {
	// sendResolved defines whether or not to notify about resolved alerts.
	// +optional
	SendResolved *bool `json:"sendResolved,omitempty"`
	// apiSecret defines the secret's key that contains the WeChat API key.
	// The secret needs to be in the same namespace as the AlertmanagerConfig
	// object and accessible by the Prometheus Operator.
	// +optional
	APISecret *v1.SecretKeySelector `json:"apiSecret,omitempty"`
	// apiURL defines the WeChat API URL.
	// When not specified, defaults to the standard WeChat Work API endpoint.
	// +optional
	APIURL string `json:"apiURL,omitempty"`
	// corpID defines the corp id for authentication.
	// This is the unique identifier for your WeChat Work organization.
	// +optional
	CorpID string `json:"corpID,omitempty"`
	// agentID defines the application agent ID within WeChat Work.
	// This identifies which WeChat Work application will send the notifications.
	// +optional
	AgentID string `json:"agentID,omitempty"`
	// toUser defines the target user(s) to receive the notification.
	// Can be a single user ID or multiple user IDs separated by '|'.
	// +optional
	ToUser string `json:"toUser,omitempty"`
	// toParty defines the target department(s) to receive the notification.
	// Can be a single department ID or multiple department IDs separated by '|'.
	// +optional
	ToParty string `json:"toParty,omitempty"`
	// toTag defines the target tag(s) to receive the notification.
	// Can be a single tag ID or multiple tag IDs separated by '|'.
	// +optional
	ToTag string `json:"toTag,omitempty"`
	// message defines the API request data as defined by the WeChat API.
	// This contains the actual notification content to be sent.
	// +optional
	Message string `json:"message,omitempty"`
	// messageType defines the type of message to send.
	// Valid values include "text", "markdown", and other WeChat Work supported message types.
	// +optional
	MessageType string `json:"messageType,omitempty"`
	// httpConfig defines the HTTP client configuration for WeChat API requests.
	// +optional
	HTTPConfig *HTTPConfig `json:"httpConfig,omitempty"`
}

// EmailConfig configures notifications via Email.
type EmailConfig struct {
	// sendResolved defines whether or not to notify about resolved alerts.
	// +optional
	SendResolved *bool `json:"sendResolved,omitempty"`
	// to defines the email address to send notifications to.
	// This is the recipient address for alert notifications.
	// +optional
	To string `json:"to,omitempty"`
	// from defines the sender address for email notifications.
	// This appears as the "From" field in the email header.
	// +optional
	From string `json:"from,omitempty"`
	// hello defines the hostname to identify to the SMTP server.
	// This is used in the SMTP HELO/EHLO command during the connection handshake.
	// +optional
	Hello string `json:"hello,omitempty"`
	// smarthost defines the SMTP host and port through which emails are sent.
	// Format should be "hostname:port", e.g. "smtp.example.com:587".
	// +optional
	Smarthost string `json:"smarthost,omitempty"`
	// authUsername defines the username to use for SMTP authentication.
	// This is used for SMTP AUTH when the server requires authentication.
	// +optional
	AuthUsername string `json:"authUsername,omitempty"`
	// authPassword defines the secret's key that contains the password to use for authentication.
	// The secret needs to be in the same namespace as the AlertmanagerConfig
	// object and accessible by the Prometheus Operator.
	// +optional
	AuthPassword *v1.SecretKeySelector `json:"authPassword,omitempty"`
	// authSecret defines the secret's key that contains the CRAM-MD5 secret.
	// This is used for CRAM-MD5 authentication mechanism.
	// The secret needs to be in the same namespace as the AlertmanagerConfig
	// object and accessible by the Prometheus Operator.
	// +optional
	AuthSecret *v1.SecretKeySelector `json:"authSecret,omitempty"`
	// authIdentity defines the identity to use for SMTP authentication.
	// This is typically used with PLAIN authentication mechanism.
	// +optional
	AuthIdentity string `json:"authIdentity,omitempty"`
	// headers defines additional email header key/value pairs.
	// These override any headers previously set by the notification implementation.
	// +optional
	Headers []KeyValue `json:"headers,omitempty"`
	// html defines the HTML body of the email notification.
	// This allows for rich formatting in the email content.
	// +optional
	HTML *string `json:"html,omitempty"`
	// text defines the plain text body of the email notification.
	// This provides a fallback for email clients that don't support HTML.
	// +optional
	Text *string `json:"text,omitempty"`
	// requireTLS defines the SMTP TLS requirement.
	// Note that Go does not support unencrypted connections to remote SMTP endpoints.
	// +optional
	RequireTLS *bool `json:"requireTLS,omitempty"`
	// tlsConfig defines the TLS configuration for SMTP connections.
	// This includes settings for certificates, CA validation, and TLS protocol options.
	// +optional
	TLSConfig *monitoringv1.SafeTLSConfig `json:"tlsConfig,omitempty"`
}

// VictorOpsConfig configures notifications via VictorOps.
// See https://prometheus.io/docs/alerting/latest/configuration/#victorops_config
type VictorOpsConfig struct {
	// sendResolved defines whether or not to notify about resolved alerts.
	// +optional
	SendResolved *bool `json:"sendResolved,omitempty"`
	// apiKey defines the secret's key that contains the API key to use when talking to the VictorOps API.
	// The secret needs to be in the same namespace as the AlertmanagerConfig
	// object and accessible by the Prometheus Operator.
	// +optional
	APIKey *v1.SecretKeySelector `json:"apiKey,omitempty"`
	// apiUrl defines the VictorOps API URL.
	// When not specified, defaults to the standard VictorOps API endpoint.
	// +optional
	APIURL string `json:"apiUrl,omitempty"`
	// routingKey defines a key used to map the alert to a team.
	// This determines which VictorOps team will receive the alert notification.
	// +optional
	RoutingKey string `json:"routingKey"`
	// messageType describes the behavior of the alert.
	// Valid values are "CRITICAL", "WARNING", and "INFO".
	// +optional
	MessageType string `json:"messageType,omitempty"`
	// entityDisplayName contains a summary of the alerted problem.
	// This appears as the main title or identifier for the incident.
	// +optional
	EntityDisplayName string `json:"entityDisplayName,omitempty"`
	// stateMessage contains a long explanation of the alerted problem.
	// This provides detailed context about the incident.
	// +optional
	StateMessage string `json:"stateMessage,omitempty"`
	// monitoringTool defines the monitoring tool the state message is from.
	// This helps identify the source system that generated the alert.
	// +optional
	MonitoringTool string `json:"monitoringTool,omitempty"`
	// customFields defines additional custom fields for notification.
	// These provide extra metadata that will be included with the VictorOps incident.
	// +optional
	CustomFields []KeyValue `json:"customFields,omitempty"`
	// httpConfig defines the HTTP client's configuration for VictorOps API requests.
	// +optional
	HTTPConfig *HTTPConfig `json:"httpConfig,omitempty"`
}

// PushoverConfig configures notifications via Pushover.
// See https://prometheus.io/docs/alerting/latest/configuration/#pushover_config
type PushoverConfig struct {
	// sendResolved defines whether or not to notify about resolved alerts.
	// +optional
	SendResolved *bool `json:"sendResolved,omitempty"`
	// userKey defines the secret's key that contains the recipient user's user key.
	// The secret needs to be in the same namespace as the AlertmanagerConfig
	// object and accessible by the Prometheus Operator.
	// Either `userKey` or `userKeyFile` is required.
	// +optional
	UserKey *v1.SecretKeySelector `json:"userKey,omitempty"`
	// userKeyFile defines the user key file that contains the recipient user's user key.
	// Either `userKey` or `userKeyFile` is required.
	// It requires Alertmanager >= v0.26.0.
	// +optional
	UserKeyFile *string `json:"userKeyFile,omitempty"`
	// token defines the secret's key that contains the registered application's API token.
	// See https://pushover.net/apps for application registration.
	// The secret needs to be in the same namespace as the AlertmanagerConfig
	// object and accessible by the Prometheus Operator.
	// Either `token` or `tokenFile` is required.
	// +optional
	Token *v1.SecretKeySelector `json:"token,omitempty"`
	// tokenFile defines the token file that contains the registered application's API token.
	// See https://pushover.net/apps for application registration.
	// Either `token` or `tokenFile` is required.
	// It requires Alertmanager >= v0.26.0.
	// +optional
	TokenFile *string `json:"tokenFile,omitempty"`
	// title defines the notification title displayed in the Pushover message.
	// This appears as the bold header text in the notification.
	// +optional
	Title string `json:"title,omitempty"`
	// message defines the notification message content.
	// This is the main body text of the Pushover notification.
	// +optional
	Message string `json:"message,omitempty"`
	// url defines a supplementary URL shown alongside the message.
	// This creates a clickable link within the Pushover notification.
	// +optional
	URL string `json:"url,omitempty"`
	// urlTitle defines a title for the supplementary URL.
	// If not specified, the raw URL is shown instead.
	// +optional
	URLTitle string `json:"urlTitle,omitempty"`
	// ttl defines the time to live for the alert notification.
	// This determines how long the notification remains active before expiring.
	// +optional
	TTL *monitoringv1.Duration `json:"ttl,omitempty"`
	// device defines the name of a specific device to send the notification to.
	// If not specified, the notification is sent to all user's devices.
	// +optional
	Device *string `json:"device,omitempty"`
	// sound defines the name of one of the sounds supported by device clients.
	// This overrides the user's default sound choice for this notification.
	// +optional
	Sound string `json:"sound,omitempty"`
	// priority defines the notification priority level.
	// See https://pushover.net/api#priority for valid values and behavior.
	// +optional
	Priority string `json:"priority,omitempty"`
	// retry defines how often the Pushover servers will send the same notification to the user.
	// Must be at least 30 seconds. Only applies to priority 2 notifications.
	// +kubebuilder:validation:Pattern=`^(([0-9]+)y)?(([0-9]+)w)?(([0-9]+)d)?(([0-9]+)h)?(([0-9]+)m)?(([0-9]+)s)?(([0-9]+)ms)?$`
	// +optional
	Retry string `json:"retry,omitempty"`
	// expire defines how long your notification will continue to be retried for,
	// unless the user acknowledges the notification. Only applies to priority 2 notifications.
	// +kubebuilder:validation:Pattern=`^(([0-9]+)y)?(([0-9]+)w)?(([0-9]+)d)?(([0-9]+)h)?(([0-9]+)m)?(([0-9]+)s)?(([0-9]+)ms)?$`
	// +optional
	Expire string `json:"expire,omitempty"`
	// html defines whether notification message is HTML or plain text.
	// When true, the message can include HTML formatting tags.
	// +optional
	HTML *bool `json:"html,omitempty"`
	// monospace optional HTML/monospace formatting for the message, see https://pushover.net/api#html
	// html and monospace formatting are mutually exclusive.
	// +optional
	Monospace *bool `json:"monospace,omitempty"`
	// httpConfig defines the HTTP client configuration for Pushover API requests.
	// +optional
	HTTPConfig *HTTPConfig `json:"httpConfig,omitempty"`
}

// SNSConfig configures notifications via AWS SNS.
// See https://prometheus.io/docs/alerting/latest/configuration/#sns_configs
type SNSConfig struct {
	// sendResolved defines whether or not to notify about resolved alerts.
	// +optional
	SendResolved *bool `json:"sendResolved,omitempty"`
	// apiURL defines the SNS API URL, e.g. https://sns.us-east-2.amazonaws.com.
	// If not specified, the SNS API URL from the SNS SDK will be used.
	// +optional
	ApiURL string `json:"apiURL,omitempty"`
	// sigv4 configures AWS's Signature Verification 4 signing process to sign requests.
	// This includes AWS credentials and region configuration for authentication.
	// +optional
	Sigv4 *monitoringv1.Sigv4 `json:"sigv4,omitempty"`
	// topicARN defines the SNS topic ARN, e.g. arn:aws:sns:us-east-2:698519295917:My-Topic.
	// If you don't specify this value, you must specify a value for the PhoneNumber or TargetARN.
	// +optional
	TopicARN string `json:"topicARN,omitempty"`
	// subject defines the subject line when the message is delivered to email endpoints.
	// This field is only used when sending to email subscribers of an SNS topic.
	// +optional
	Subject string `json:"subject,omitempty"`
	// phoneNumber defines the phone number if message is delivered via SMS in E.164 format.
	// If you don't specify this value, you must specify a value for the TopicARN or TargetARN.
	// +optional
	PhoneNumber string `json:"phoneNumber,omitempty"`
	// targetARN defines the mobile platform endpoint ARN if message is delivered via mobile notifications.
	// If you don't specify this value, you must specify a value for the TopicARN or PhoneNumber.
	// +optional
	TargetARN string `json:"targetARN,omitempty"`
	// message defines the message content of the SNS notification.
	// This is the actual notification text that will be sent to subscribers.
	// +optional
	Message string `json:"message,omitempty"`
	// attributes defines SNS message attributes as key-value pairs.
	// These provide additional metadata that can be used for message filtering and routing.
	// +optional
	Attributes map[string]string `json:"attributes,omitempty"`
	// httpConfig defines the HTTP client configuration for SNS API requests.
	// +optional
	HTTPConfig *HTTPConfig `json:"httpConfig,omitempty"`
}

// TelegramConfig configures notifications via Telegram.
// See https://prometheus.io/docs/alerting/latest/configuration/#telegram_config
type TelegramConfig struct {
	// sendResolved defines whether or not to notify about resolved alerts.
	// +optional
	SendResolved *bool `json:"sendResolved,omitempty"`
	// apiURL defines the Telegram API URL, e.g. https://api.telegram.org.
	// If not specified, the default Telegram API URL will be used.
	// +optional
	APIURL string `json:"apiURL,omitempty"`
	// botToken defines the Telegram bot token. It is mutually exclusive with `botTokenFile`.
	// The secret needs to be in the same namespace as the AlertmanagerConfig
	// object and accessible by the Prometheus Operator.
	// Either `botToken` or `botTokenFile` is required.
	// +optional
	BotToken *v1.SecretKeySelector `json:"botToken,omitempty"`
	// botTokenFile defines the file to read the Telegram bot token from.
	// It is mutually exclusive with `botToken`.
	// Either `botToken` or `botTokenFile` is required.
	// It requires Alertmanager >= v0.26.0.
	// +optional
	BotTokenFile *string `json:"botTokenFile,omitempty"`
	// chatID defines the Telegram chat ID where messages will be sent.
	// This can be a user ID, group ID, or channel ID (with @ prefix for public channels).
	// +required
	ChatID int64 `json:"chatID,omitempty"`
	// messageThreadID defines the Telegram Group Topic ID for threaded messages.
	// This allows sending messages to specific topics within Telegram groups.
	// It requires Alertmanager >= 0.26.0.
	// +optional
	MessageThreadID *int64 `json:"messageThreadID,omitempty"`
	// message defines the message template for the Telegram notification.
	// This is the content that will be sent to the specified chat.
	// +optional
	Message string `json:"message,omitempty"`
	// disableNotifications controls whether Telegram notifications are sent silently.
	// When true, users will receive the message without notification sounds.
	// +optional
	DisableNotifications *bool `json:"disableNotifications,omitempty"`
	// parseMode defines the parse mode for telegram message formatting.
	// Valid values are "MarkdownV2", "Markdown", and "HTML".
	// This determines how text formatting is interpreted in the message.
	//+kubebuilder:validation:Enum=MarkdownV2;Markdown;HTML
	// +optional
	ParseMode string `json:"parseMode,omitempty"`
	// httpConfig defines the HTTP client configuration for Telegram API requests.
	// +optional
	HTTPConfig *HTTPConfig `json:"httpConfig,omitempty"`
}

// MSTeamsConfig configures notifications via Microsoft Teams.
// It requires Alertmanager >= 0.26.0.
type MSTeamsConfig struct {
	// sendResolved defines whether or not to notify about resolved alerts.
	// +optional
	SendResolved *bool `json:"sendResolved,omitempty"`
	// webhookUrl defines the MSTeams webhook URL for sending notifications.
	// This is the incoming webhook URL configured in your Teams channel.
	// +required
	WebhookURL v1.SecretKeySelector `json:"webhookUrl"`
	// title defines the message title template for Teams notifications.
	// This appears as the main heading of the Teams message card.
	// +optional
	Title *string `json:"title,omitempty"`
	// summary defines the message summary template for Teams notifications.
	// This provides a brief overview that appears in Teams notification previews.
	// It requires Alertmanager >= 0.27.0.
	// +optional
	Summary *string `json:"summary,omitempty"`
	// text defines the message body template for Teams notifications.
	// This contains the detailed content of the Teams message.
	// +optional
	Text *string `json:"text,omitempty"`
	// httpConfig defines the HTTP client configuration for Teams webhook requests.
	// +optional
	HTTPConfig *HTTPConfig `json:"httpConfig,omitempty"`
}

// MSTeamsV2Config configures notifications via Microsoft Teams using the new message format with adaptive cards as required by flows.
// See https://prometheus.io/docs/alerting/latest/configuration/#msteamsv2_config
// It requires Alertmanager >= 0.28.0.
type MSTeamsV2Config struct {
	// sendResolved defines whether or not to notify about resolved alerts.
	// +optional
	SendResolved *bool `json:"sendResolved,omitempty"`
	// webhookURL defines the MSTeams incoming webhook URL for adaptive card notifications.
	// This webhook must support the newer adaptive cards format required by Teams flows.
	// +optional
	WebhookURL *v1.SecretKeySelector `json:"webhookURL,omitempty"`
	// title defines the message title template for adaptive card notifications.
	// This appears as the main heading in the Teams adaptive card.
	// +kubebuilder:validation:MinLength=1
	// +optional
	Title *string `json:"title,omitempty"`
	// text defines the message body template for adaptive card notifications.
	// This contains the detailed content displayed in the Teams adaptive card format.
	// +kubebuilder:validation:MinLength=1
	// +optional
	Text *string `json:"text,omitempty"`
	// httpConfig defines the HTTP client configuration for Teams webhook requests.
	// +optional
	HTTPConfig *HTTPConfig `json:"httpConfig,omitempty"`
}

// RocketChatConfig configures notifications via RocketChat.
// It requires Alertmanager >= 0.28.0.
type RocketChatConfig struct {
	// sendResolved defines whether or not to notify about resolved alerts.
	// +optional
	SendResolved *bool `json:"sendResolved,omitempty"`
	// apiURL defines the API URL for RocketChat.
	// Defaults to https://open.rocket.chat/ if not specified.
	// +optional
	APIURL *URL `json:"apiURL,omitempty"`
	// channel defines the channel to send alerts to.
	// This can be a channel name (e.g., "#alerts") or a direct message recipient.
	// +kubebuilder:validation:MinLength=1
	// +optional
	Channel *string `json:"channel,omitempty"`
	// token defines the sender token for RocketChat authentication.
	// This is the personal access token or bot token used to authenticate API requests.
	// +required
	Token v1.SecretKeySelector `json:"token,omitempty"`
	// tokenID defines the sender token ID for RocketChat authentication.
	// This is the user ID associated with the token used for API requests.
	// +required
	TokenID v1.SecretKeySelector `json:"tokenID,omitempty"`
	// color defines the message color displayed in RocketChat.
	// This appears as a colored bar alongside the message.
	// +kubebuilder:validation:MinLength=1
	// +optional
	Color *string `json:"color,omitempty"`
	// emoji defines the emoji to be displayed as an avatar.
	// If provided, this emoji will be used instead of the default avatar or iconURL.
	// +kubebuilder:validation:MinLength=1
	// +optional
	Emoji *string `json:"emoji,omitempty"`
	// iconURL defines the icon URL for the message avatar.
	// This displays a custom image as the message sender's avatar.
	// +optional
	IconURL *URL `json:"iconURL,omitempty"`
	// text defines the message text to send.
	// This is optional because attachments can be used instead of or alongside text.
	// +kubebuilder:validation:MinLength=1
	// +optional
	Text *string `json:"text,omitempty"`
	// title defines the message title displayed prominently in the message.
	// This appears as bold text at the top of the message attachment.
	// +kubebuilder:validation:MinLength=1
	// +optional
	Title *string `json:"title,omitempty"`
	// titleLink defines the URL that the title will link to when clicked.
	// This makes the message title clickable in the RocketChat interface.
	// +kubebuilder:validation:MinLength=1
	// +optional
	TitleLink *string `json:"titleLink,omitempty"`
	// fields defines additional fields for the message attachment.
	// These appear as structured key-value pairs within the message.
	// +kubebuilder:validation:MinItems=1
	// +optional
	Fields []RocketChatFieldConfig `json:"fields,omitempty"`
	// shortFields defines whether to use short fields in the message layout.
	// When true, fields may be displayed side by side to save space.
	// +optional
	ShortFields *bool `json:"shortFields,omitempty"`
	// imageURL defines the image URL to display within the message.
	// This embeds an image directly in the message attachment.
	// +optional
	ImageURL *URL `json:"imageURL,omitempty"`
	// thumbURL defines the thumbnail URL for the message.
	// This displays a small thumbnail image alongside the message content.
	// +optional
	ThumbURL *URL `json:"thumbURL,omitempty"`
	// linkNames defines whether to enable automatic linking of usernames and channels.
	// When true, @username and #channel references become clickable links.
	// +optional
	LinkNames *bool `json:"linkNames,omitempty"`
	// actions defines interactive actions to include in the message.
	// These appear as buttons that users can click to trigger responses.
	// +kubebuilder:validation:MinItems=1
	// +optional
	Actions []RocketChatActionConfig `json:"actions,omitempty"`
	// httpConfig defines the HTTP client configuration for RocketChat API requests.
	// +optional
	HTTPConfig *HTTPConfig `json:"httpConfig,omitempty"`
}

// RocketChatFieldConfig defines additional fields for RocketChat messages.
type RocketChatFieldConfig struct {
	// title defines the title of this field.
	// This appears as bold text labeling the field content.
	// +kubebuilder:validation:MinLength=1
	// +optional
	Title *string `json:"title,omitempty"`
	// value defines the value of this field, displayed underneath the title.
	// This contains the actual data or content for the field.
	// +kubebuilder:validation:MinLength=1
	// +optional
	Value *string `json:"value,omitempty"`
	// short defines whether this field should be a short field.
	// When true, the field may be displayed inline with other short fields to save space.
	// +optional
	Short *bool `json:"short,omitempty"`
}

// RocketChatActionConfig defines actions for RocketChat messages.
type RocketChatActionConfig struct {
	// text defines the button text displayed to users.
	// This is the label that appears on the interactive button.
	// +kubebuilder:validation:MinLength=1
	// +optional
	Text *string `json:"text,omitempty"`
	// url defines the URL the button links to when clicked.
	// This creates a clickable button that opens the specified URL.
	// +optional
	URL *URL `json:"url,omitempty"`
	// msg defines the message to send when the button is clicked.
	// This allows the button to post a predefined message to the channel.
	// +kubebuilder:validation:MinLength=1
	// +optional
	Msg *string `json:"msg,omitempty"`
}

// InhibitRule defines an inhibition rule that allows to mute alerts when other
// alerts are already firing.
// See https://prometheus.io/docs/alerting/latest/configuration/#inhibit_rule
type InhibitRule struct {
	// targetMatch defines matchers that have to be fulfilled in the alerts to be muted.
	// The operator enforces that the alert matches the resource's namespace.
	// When these conditions are met, matching alerts will be inhibited (silenced).
	// +optional
	TargetMatch []Matcher `json:"targetMatch,omitempty"`
	// sourceMatch defines matchers for which one or more alerts have to exist for the inhibition
	// to take effect. The operator enforces that the alert matches the resource's namespace.
	// These are the "trigger" alerts that cause other alerts to be inhibited.
	// +optional
	SourceMatch []Matcher `json:"sourceMatch,omitempty"`
	// equal defines labels that must have an equal value in the source and target alert
	// for the inhibition to take effect. This ensures related alerts are properly grouped.
	// +optional
	Equal []string `json:"equal,omitempty"`
}

// KeyValue defines a (key, value) tuple.
type KeyValue struct {
	// key defines the key of the tuple.
	// This is the identifier or name part of the key-value pair.
	// +kubebuilder:validation:MinLength=1
	// +required
	Key string `json:"key"`
	// value defines the value of the tuple.
	// This is the data or content associated with the key.
	// +required
	Value string `json:"value"`
}

// Matcher defines how to match on alert's labels.
type Matcher struct {
	// name defines the label to match.
	// This specifies which alert label should be evaluated.
	// +kubebuilder:validation:MinLength=1
	// +required
	Name string `json:"name"`
	// value defines the label value to match.
	// This is the expected value for the specified label.
	// +optional
	Value string `json:"value"`
	// matchType defines the match operation available with AlertManager >= v0.22.0.
	// Takes precedence over Regex (deprecated) if non-empty.
	// Valid values: "=" (equality), "!=" (inequality), "=~" (regex match), "!~" (regex non-match).
	// +kubebuilder:validation:Enum=!=;=;=~;!~
	// +optional
	MatchType MatchType `json:"matchType,omitempty"`
	// regex defines whether to match on equality (false) or regular-expression (true).
	// Deprecated: for AlertManager >= v0.22.0, `matchType` should be used instead.
	// +optional
	Regex bool `json:"regex,omitempty"`
}

// String returns Matcher as a string
// Use only for MatchType Matcher
func (in Matcher) String() string {
	return fmt.Sprintf(`%s%s"%s"`, in.Name, in.MatchType, openMetricsEscape(in.Value))
}

// Validate the Matcher returns an error if the matcher is invalid
// Validates only non-deprecated matching fields
func (in Matcher) Validate() error {
	// nothing to do
	if in.MatchType == "" {
		return nil
	}

	if !in.MatchType.Valid() {
		return fmt.Errorf("invalid 'matchType' '%s' provided'", in.MatchType)
	}

	if strings.TrimSpace(in.Name) == "" {
		return errors.New("matcher 'name' is required")
	}

	return nil
}

// MatchType is a comparison operator on a Matcher
type MatchType string

// Valid MatchType returns true if the operator is acceptable
func (mt MatchType) Valid() bool {
	_, ok := validMatchTypes[mt]
	return ok
}

// DeepCopyObject implements the runtime.Object interface.
func (l *AlertmanagerConfig) DeepCopyObject() runtime.Object {
	return l.DeepCopy()
}

// DeepCopyObject implements the runtime.Object interface.
func (l *AlertmanagerConfigList) DeepCopyObject() runtime.Object {
	return l.DeepCopy()
}

const (
	MatchEqual     MatchType = "="
	MatchNotEqual  MatchType = "!="
	MatchRegexp    MatchType = "=~"
	MatchNotRegexp MatchType = "!~"
)

var validMatchTypes = map[MatchType]bool{
	MatchEqual:     true,
	MatchNotEqual:  true,
	MatchRegexp:    true,
	MatchNotRegexp: true,
}

// openMetricsEscape is similar to the usual string escaping, but more
// restricted. It merely replaces a new-line character with '\n', a double-quote
// character with '\"', and a backslash with '\\', which is the escaping used by
// OpenMetrics.
// * Copied from alertmanager codebase pkg/labels *
func openMetricsEscape(s string) string {
	r := strings.NewReplacer(
		`\`, `\\`,
		"\n", `\n`,
		`"`, `\"`,
	)
	return r.Replace(s)
}

// MuteTimeInterval specifies the periods in time when notifications will be muted
type MuteTimeInterval struct {
	// name of the time interval
	// +required
	Name string `json:"name,omitempty"`
	// timeIntervals defines a list of TimeInterval
	// +optional
	TimeIntervals []TimeInterval `json:"timeIntervals,omitempty"`
}

// TimeInterval describes intervals of time
type TimeInterval struct {
	// times defines a list of TimeRange
	// +optional
	Times []TimeRange `json:"times,omitempty"`
	// weekdays defines a list of WeekdayRange
	// +optional
	Weekdays []WeekdayRange `json:"weekdays,omitempty"`
	// daysOfMonth defines a list of DayOfMonthRange
	// +optional
	DaysOfMonth []DayOfMonthRange `json:"daysOfMonth,omitempty"`
	// months defines a list of MonthRange
	// +optional
	Months []MonthRange `json:"months,omitempty"`
	// years defines a list of YearRange
	// +optional
	Years []YearRange `json:"years,omitempty"`
}

// Time defines a time in 24hr format
// +kubebuilder:validation:Pattern=`^((([01][0-9])|(2[0-3])):[0-5][0-9])$|(^24:00$)`
type Time string

// TimeRange defines a start and end time in 24hr format
type TimeRange struct {
	// startTime defines the start time in 24hr format.
	// +optional
	StartTime Time `json:"startTime,omitempty"`
	// endTime defines the end time in 24hr format.
	// +optional
	EndTime Time `json:"endTime,omitempty"`
}

// WeekdayRange is an inclusive range of days of the week beginning on Sunday
// Days can be specified by name (e.g 'Sunday') or as an inclusive range (e.g 'Monday:Friday')
// +kubebuilder:validation:Pattern=`^((?i)sun|mon|tues|wednes|thurs|fri|satur)day(?:((:(sun|mon|tues|wednes|thurs|fri|satur)day)$)|$)`
type WeekdayRange string

// DayOfMonthRange is an inclusive range of days of the month beginning at 1
type DayOfMonthRange struct {
	// start of the inclusive range
	// +kubebuilder:validation:Minimum=-31
	// +kubebuilder:validation:Maximum=31
	// +optional
	Start int `json:"start,omitempty"`
	// end of the inclusive range
	// +kubebuilder:validation:Minimum=-31
	// +kubebuilder:validation:Maximum=31
	// +optional
	End int `json:"end,omitempty"`
}

// MonthRange is an inclusive range of months of the year beginning in January
// Months can be specified by name (e.g 'January') by numerical month (e.g '1') or as an inclusive range (e.g 'January:March', '1:3', '1:March')
// +kubebuilder:validation:Pattern=`^((?i)january|february|march|april|may|june|july|august|september|october|november|december|1[0-2]|[1-9])(?:((:((?i)january|february|march|april|may|june|july|august|september|october|november|december|1[0-2]|[1-9]))$)|$)`
type MonthRange string

// YearRange is an inclusive range of years
// +kubebuilder:validation:Pattern=`^2\d{3}(?::2\d{3}|$)`
type YearRange string

// Weekday is day of the week
type Weekday string

const (
	Sunday    Weekday = "sunday"
	Monday    Weekday = "monday"
	Tuesday   Weekday = "tuesday"
	Wednesday Weekday = "wednesday"
	Thursday  Weekday = "thursday"
	Friday    Weekday = "friday"
	Saturday  Weekday = "saturday"
)

var daysOfWeek = map[Weekday]int{
	Sunday:    0,
	Monday:    1,
	Tuesday:   2,
	Wednesday: 3,
	Thursday:  4,
	Friday:    5,
	Saturday:  6,
}

var daysOfWeekInv = map[int]Weekday{
	0: Sunday,
	1: Monday,
	2: Tuesday,
	3: Wednesday,
	4: Thursday,
	5: Friday,
	6: Saturday,
}

// Month of the year
type Month string

const (
	January   Month = "january"
	February  Month = "february"
	March     Month = "march"
	April     Month = "april"
	May       Month = "may"
	June      Month = "june"
	July      Month = "july"
	August    Month = "august"
	September Month = "september"
	October   Month = "october"
	November  Month = "november"
	December  Month = "december"
)

var months = map[Month]int{
	January:   1,
	February:  2,
	March:     3,
	April:     4,
	May:       5,
	June:      6,
	July:      7,
	August:    8,
	September: 9,
	October:   10,
	November:  11,
	December:  12,
}

var monthsInv = map[int]Month{
	1:  January,
	2:  February,
	3:  March,
	4:  April,
	5:  May,
	6:  June,
	7:  July,
	8:  August,
	9:  September,
	10: October,
	11: November,
	12: December,
}

// URL represents a valid URL
// +kubebuilder:validation:Pattern=`^https?://.+$`
type URL string
