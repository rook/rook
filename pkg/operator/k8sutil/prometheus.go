/*
Copyright 2020 The Rook Authors. All rights reserved.

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

// Package k8sutil for Kubernetes helpers.
package k8sutil

import (
	"context"
	"fmt"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	monitoringclient "github.com/prometheus-operator/prometheus-operator/pkg/client/versioned"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func getMonitoringClient(context *clusterd.Context) (*monitoringclient.Clientset, error) {
	client, err := monitoringclient.NewForConfig(context.KubeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to get monitoring client. %v", err)
	}
	return client, nil
}

// K8sClusterNameEnvVar is the operator setting used to advertise the name of the
// Kubernetes cluster where Rook runs. When set, the value is applied to every
// metric scraped by Rook's ServiceMonitors as the "k8s_cluster" relabel target.
// The setting can be provided as an environment variable of the operator pod or
// as a key of the rook-ceph-operator-config ConfigMap.
const K8sClusterNameEnvVar = "ROOK_K8S_CLUSTER_NAME"

// monitoringRelabelConfigs returns the relabel configs applied to every metric
// scraped by Rook's ServiceMonitors:
//   - "cluster" holds the Ceph cluster namespace. Ceph's Prometheus alert rules
//     and dashboards reference this label, and Rook has already released with
//     the namespace as its value, so the existing behavior is kept.
//   - "ceph_cluster" always holds the Ceph cluster namespace, as an unambiguous
//     alias for the overloaded "cluster" label.
//   - "k8s_cluster" holds the user-supplied Kubernetes cluster name when
//     ROOK_K8S_CLUSTER_NAME is set. Prometheus applies externalLabels only to
//     samples that do not already carry the label, so the "cluster" label above
//     shadows the external label of the same name in multi-site monitoring
//     setups (Thanos, Mimir, VictoriaMetrics, ...). Grouping and filtering on
//     the explicit "k8s_cluster" label allows metrics from different Kubernetes
//     clusters to be told apart in a centralized monitoring backend.
func monitoringRelabelConfigs(namespace string) []monitoringv1.RelabelConfig {
	configs := []monitoringv1.RelabelConfig{
		{
			TargetLabel: "cluster",
			Replacement: ptr.To(namespace),
		},
		{
			TargetLabel: "ceph_cluster",
			Replacement: ptr.To(namespace),
		},
	}
	if k8sClusterName := GetOperatorSetting(K8sClusterNameEnvVar, ""); k8sClusterName != "" {
		configs = append(configs, monitoringv1.RelabelConfig{
			TargetLabel: "k8s_cluster",
			Replacement: ptr.To(k8sClusterName),
		})
	}
	return configs
}

// GetServiceMonitor creates a serviceMonitor object template
func GetServiceMonitor(name string, namespace string, portName string) *monitoringv1.ServiceMonitor {
	return &monitoringv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"team": "rook",
			},
		},
		Spec: monitoringv1.ServiceMonitorSpec{
			NamespaceSelector: monitoringv1.NamespaceSelector{
				MatchNames: []string{
					namespace,
				},
			},
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":          name,
					"rook_cluster": namespace,
				},
			},
			Endpoints: []monitoringv1.Endpoint{
				{
					Port:           portName,
					Path:           "/metrics",
					Interval:       "10s",
					HonorLabels:    true,
					RelabelConfigs: monitoringRelabelConfigs(namespace),
				},
			},
		},
	}
}

// ApplyMonitoringTiming sets the scrape interval and timeout on the ServiceMonitor endpoint.
// An unset interval keeps the 10s from GetServiceMonitor; an unset timeout is left to Prometheus.
func ApplyMonitoringTiming(monitoring cephv1.MonitoringSpec, serviceMonitor *monitoringv1.ServiceMonitor) {
	if len(serviceMonitor.Spec.Endpoints) == 0 {
		return
	}
	if monitoring.Interval != nil {
		serviceMonitor.Spec.Endpoints[0].Interval = monitoringv1.Duration(monitoring.Interval.Duration.String())
	}
	if monitoring.ScrapeTimeoutSeconds > 0 {
		serviceMonitor.Spec.Endpoints[0].ScrapeTimeout = monitoringv1.Duration(fmt.Sprintf("%ds", monitoring.ScrapeTimeoutSeconds))
	}
}

// CreateOrUpdateServiceMonitor creates or updates a serviceMonitor object and returns it or an error
func CreateOrUpdateServiceMonitor(context *clusterd.Context, ctx context.Context, serviceMonitorDefinition *monitoringv1.ServiceMonitor) (*monitoringv1.ServiceMonitor, error) {
	name := serviceMonitorDefinition.GetName()
	namespace := serviceMonitorDefinition.GetNamespace()
	logger.Debugf("creating servicemonitor %s", name)
	client, err := getMonitoringClient(context)
	if err != nil {
		return nil, fmt.Errorf("failed to get monitoring client. %v", err)
	}
	oldSm, err := client.MonitoringV1().ServiceMonitors(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if kerrors.IsNotFound(err) {
			sm, err := client.MonitoringV1().ServiceMonitors(namespace).Create(ctx, serviceMonitorDefinition, metav1.CreateOptions{})
			if err != nil {
				return nil, fmt.Errorf("failed to create servicemonitor. %v", err)
			}
			return sm, nil
		}
		return nil, fmt.Errorf("failed to retrieve servicemonitor. %v", err)
	}

	oldSm.Spec = serviceMonitorDefinition.Spec
	oldSm.ObjectMeta.Labels = serviceMonitorDefinition.ObjectMeta.Labels
	sm, err := client.MonitoringV1().ServiceMonitors(namespace).Update(ctx, oldSm, metav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to update servicemonitor. %v", err)
	}
	return sm, nil
}
