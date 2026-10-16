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
