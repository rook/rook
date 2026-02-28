// Copyright 2018 The prometheus-operator Authors
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

package monitoring

import (
	"fmt"
)

const (
	PrometheusesKind = "Prometheus"
	PrometheusName   = "prometheuses"

	PrometheusAgentsKind = "PrometheusAgent"
	PrometheusAgentName  = "prometheusagents"

	AlertmanagersKind = "Alertmanager"
	AlertmanagerName  = "alertmanagers"

	AlertmanagerConfigsKind = "AlertmanagerConfig"
	AlertmanagerConfigName  = "alertmanagerconfigs"

	ServiceMonitorsKind = "ServiceMonitor"
	ServiceMonitorName  = "servicemonitors"

	PodMonitorsKind = "PodMonitor"
	PodMonitorName  = "podmonitors"

	PrometheusRuleKind = "PrometheusRule"
	PrometheusRuleName = "prometheusrules"

	ProbesKind = "Probe"
	ProbeName  = "probes"

	ScrapeConfigsKind = "ScrapeConfig"
	ScrapeConfigName  = "scrapeconfigs"

	ThanosRulersKind = "ThanosRuler"
	ThanosRulerName  = "thanosrulers"
)

var resourceToKindMap = map[string]string{
	PrometheusName:         PrometheusesKind,
	PrometheusAgentName:    PrometheusAgentsKind,
	AlertmanagerName:       AlertmanagersKind,
	AlertmanagerConfigName: AlertmanagerConfigsKind,
	ServiceMonitorName:     ServiceMonitorsKind,
	PodMonitorName:         PodMonitorsKind,
	PrometheusRuleName:     PrometheusRuleKind,
	ProbeName:              ProbesKind,
	ScrapeConfigName:       ScrapeConfigsKind,
	ThanosRulerName:        ThanosRulersKind,
}

var kindToResource = map[string]string{
	PrometheusesKind:        PrometheusName,
	PrometheusAgentsKind:    PrometheusAgentName,
	AlertmanagersKind:       AlertmanagerName,
	AlertmanagerConfigsKind: AlertmanagerConfigName,
	ServiceMonitorsKind:     ServiceMonitorName,
	PodMonitorsKind:         PodMonitorName,
	PrometheusRuleKind:      PrometheusRuleName,
	ProbesKind:              ProbeName,
	ScrapeConfigsKind:       ScrapeConfigName,
	ThanosRulersKind:        ThanosRulerName,
}

// KindToResource returns the resource name corresponding to the given kind.
func KindToResource(k string) string {
	kind, found := kindToResource[k]
	if !found {
		panic(fmt.Sprintf("failed to map kind %q to a resource name", k))
	}
	return kind
}

// ResourceToKind returns the kind corresponding to the given resource name.
func ResourceToKind(r string) string {
	kind, found := resourceToKindMap[r]
	if !found {
		panic(fmt.Sprintf("failed to map resource %q to a kind", r))
	}
	return kind
}
