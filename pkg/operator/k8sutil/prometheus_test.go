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
	"testing"
	"time"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetServiceMonitor(t *testing.T) {
	name := "rook-ceph-mgr"
	namespace := "rook-ceph"
	port := "http-metrics"
	interval := monitoringv1.Duration("10s")
	servicemonitor := GetServiceMonitor(name, namespace, port)
	assert.Equal(t, name, servicemonitor.GetName())
	assert.Equal(t, namespace, servicemonitor.GetNamespace())
	assert.Equal(t, port, servicemonitor.Spec.Endpoints[0].Port)
	assert.Equal(t, interval, servicemonitor.Spec.Endpoints[0].Interval)
	assert.NotNil(t, servicemonitor.GetLabels())
	assert.NotNil(t, servicemonitor.Spec.NamespaceSelector.MatchNames)
	assert.NotNil(t, servicemonitor.Spec.Selector.MatchLabels)
	assert.NotNil(t, servicemonitor.Spec.Endpoints)
	assert.Equal(t, "cluster", servicemonitor.Spec.Endpoints[0].RelabelConfigs[0].TargetLabel)
	assert.Equal(t, namespace, *servicemonitor.Spec.Endpoints[0].RelabelConfigs[0].Replacement)
	assert.Equal(t, "ceph_cluster", servicemonitor.Spec.Endpoints[0].RelabelConfigs[1].TargetLabel)
	assert.Equal(t, namespace, *servicemonitor.Spec.Endpoints[0].RelabelConfigs[1].Replacement)
}

func TestServiceMonitorRelabelConfigs(t *testing.T) {
	namespace := "rook-ceph"

	findRelabel := func(sm *monitoringv1.ServiceMonitor, target string) *monitoringv1.RelabelConfig {
		for i := range sm.Spec.Endpoints[0].RelabelConfigs {
			if sm.Spec.Endpoints[0].RelabelConfigs[i].TargetLabel == target {
				return &sm.Spec.Endpoints[0].RelabelConfigs[i]
			}
		}
		return nil
	}

	t.Run("cluster and ceph_cluster are always set to the namespace", func(t *testing.T) {
		sm := GetServiceMonitor("rook-ceph-mgr", namespace, "http-metrics")

		relabels := sm.Spec.Endpoints[0].RelabelConfigs
		assert.Len(t, relabels, 2)
		assert.Equal(t, "cluster", relabels[0].TargetLabel)
		assert.Equal(t, namespace, *relabels[0].Replacement)
		assert.Equal(t, "ceph_cluster", relabels[1].TargetLabel)
		assert.Equal(t, namespace, *relabels[1].Replacement)
		assert.Nil(t, findRelabel(sm, "k8s_cluster"))
	})

	t.Run("k8s_cluster is set when ROOK_K8S_CLUSTER_NAME is configured", func(t *testing.T) {
		t.Setenv(K8sClusterNameEnvVar, "cluster-a.example.com")

		sm := GetServiceMonitor("rook-ceph-mgr", namespace, "http-metrics")

		relabels := sm.Spec.Endpoints[0].RelabelConfigs
		assert.Len(t, relabels, 3)
		k8sCluster := findRelabel(sm, "k8s_cluster")
		assert.NotNil(t, k8sCluster)
		assert.Equal(t, "cluster-a.example.com", *k8sCluster.Replacement)
		// the existing labels are unchanged by the Kubernetes cluster name
		assert.Equal(t, namespace, *findRelabel(sm, "cluster").Replacement)
		assert.Equal(t, namespace, *findRelabel(sm, "ceph_cluster").Replacement)
	})

	t.Run("empty ROOK_K8S_CLUSTER_NAME is ignored", func(t *testing.T) {
		t.Setenv(K8sClusterNameEnvVar, "")

		sm := GetServiceMonitor("rook-ceph-exporter", namespace, "http-metrics")

		assert.Len(t, sm.Spec.Endpoints[0].RelabelConfigs, 2)
		assert.Nil(t, findRelabel(sm, "k8s_cluster"))
	})

	t.Run("k8s_cluster follows the configured Kubernetes cluster name", func(t *testing.T) {
		t.Setenv(K8sClusterNameEnvVar, "cluster-b.example.com")

		sm := GetServiceMonitor("rook-ceph-exporter", namespace, "http-metrics")

		assert.Equal(t, "cluster-b.example.com", *findRelabel(sm, "k8s_cluster").Replacement)
	})
}

func TestApplyMonitoringTiming(t *testing.T) {
	newServiceMonitor := func() *monitoringv1.ServiceMonitor {
		return &monitoringv1.ServiceMonitor{Spec: monitoringv1.ServiceMonitorSpec{
			Endpoints: []monitoringv1.Endpoint{{}},
		}}
	}

	t.Run("both unset leaves Prometheus defaults in place", func(t *testing.T) {
		sm := newServiceMonitor()
		ApplyMonitoringTiming(cephv1.MonitoringSpec{}, sm)
		assert.Empty(t, sm.Spec.Endpoints[0].Interval)
		assert.Empty(t, sm.Spec.Endpoints[0].ScrapeTimeout)
	})

	t.Run("interval and scrape timeout are both applied", func(t *testing.T) {
		monitoring := cephv1.MonitoringSpec{
			Interval:             &metav1.Duration{Duration: 60 * time.Second},
			ScrapeTimeoutSeconds: 30,
		}

		sm := newServiceMonitor()
		ApplyMonitoringTiming(monitoring, sm)
		assert.Equal(t, monitoringv1.Duration("1m0s"), sm.Spec.Endpoints[0].Interval)
		assert.Equal(t, monitoringv1.Duration("30s"), sm.Spec.Endpoints[0].ScrapeTimeout)
	})

	t.Run("scrape timeout is independent of interval", func(t *testing.T) {
		monitoring := cephv1.MonitoringSpec{ScrapeTimeoutSeconds: 15}

		sm := newServiceMonitor()
		ApplyMonitoringTiming(monitoring, sm)
		assert.Empty(t, sm.Spec.Endpoints[0].Interval)
		assert.Equal(t, monitoringv1.Duration("15s"), sm.Spec.Endpoints[0].ScrapeTimeout)
	})

	t.Run("a ServiceMonitor with no endpoints is left alone", func(t *testing.T) {
		monitoring := cephv1.MonitoringSpec{ScrapeTimeoutSeconds: 15}

		sm := &monitoringv1.ServiceMonitor{}
		ApplyMonitoringTiming(monitoring, sm)
		assert.Empty(t, sm.Spec.Endpoints)
	})
}
