/*
Copyright 2018 The Rook Authors. All rights reserved.

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

package test

import (
	"fmt"
	"os"
	"testing"

	optest "github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
)

// AssertLabelsContainCephRequirements asserts that the labels under test contain the labels
// which all Ceph pods should have. This can be used with labels for Kubernetes Deployments,
// DaemonSets, etc.
func AssertLabelsContainCephRequirements(
	t *testing.T, labels map[string]string,
	daemonType, daemonID, appName, namespace, parentName, resourceKind, appBinaryName string,
) {
	optest.AssertLabelsContainRookRequirements(t, labels, appName)

	resourceLabels := []string{}
	for k, v := range labels {
		resourceLabels = append(resourceLabels, fmt.Sprintf("%s=%s", k, v))
	}
	expectedLabels := []string{
		"app.kubernetes.io/created-by=rook-ceph-operator",
		"app.kubernetes.io/component=" + resourceKind,
		"app.kubernetes.io/instance=" + daemonID,
		"app.kubernetes.io/name=" + appBinaryName,
		"app.kubernetes.io/managed-by=rook-ceph-operator",
		"app.kubernetes.io/part-of=" + parentName,
		"ceph_daemon_id=" + daemonID,
		string(daemonType) + "=" + daemonID,
		"rook.io/operator-namespace=" + os.Getenv("POD_NAMESPACE"),
		"rook_cluster" + "=" + namespace,
	}
	assert.Subset(t, resourceLabels, expectedLabels,
		"labels on resource do not match Ceph requirements", labels)
}
