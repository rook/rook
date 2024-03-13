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
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/stretchr/testify/assert"
	"testing"
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
}
