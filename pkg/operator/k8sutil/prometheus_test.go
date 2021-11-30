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
	"path"
	"testing"

	"github.com/rook/rook/pkg/util"
	"github.com/stretchr/testify/assert"
)

func TestGetServiceMonitor(t *testing.T) {
	projectRoot := util.PathToProjectRoot()
	filePath := path.Join(projectRoot, "/deploy/examples/monitoring/service-monitor.yaml")
	servicemonitor, err := GetServiceMonitor(filePath)
	assert.Nil(t, err)
	assert.Equal(t, "rook-ceph-mgr", servicemonitor.GetName())
	assert.Equal(t, "rook-ceph", servicemonitor.GetNamespace())
	assert.NotNil(t, servicemonitor.GetLabels())
	assert.NotNil(t, servicemonitor.Spec.NamespaceSelector.MatchNames)
	assert.NotNil(t, servicemonitor.Spec.Endpoints)
}

func TestGetPrometheusRule(t *testing.T) {
	projectRoot := util.PathToProjectRoot()
	filePath := path.Join(projectRoot, "/deploy/examples/monitoring/prometheus-ceph-v14-rules.yaml")
	rules, err := GetPrometheusRule(filePath)
	assert.Nil(t, err)
	assert.Equal(t, "prometheus-ceph-rules", rules.GetName())
	assert.Equal(t, "rook-ceph", rules.GetNamespace())
	// Labels should be present as they are used by prometheus for identifying rules
	assert.NotNil(t, rules.GetLabels())
	assert.NotNil(t, rules.Spec.Groups)
}
