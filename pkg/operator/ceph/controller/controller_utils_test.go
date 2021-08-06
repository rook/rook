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

package controller

import (
	"testing"
	"time"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/util/exec"
	"github.com/stretchr/testify/assert"
)

func CreateTestClusterFromStatusDetails(details map[string]cephv1.CephHealthMessage) cephv1.CephCluster {
	return cephv1.CephCluster{
		Status: cephv1.ClusterStatus{
			CephStatus: &cephv1.CephStatus{
				Details: details,
			},
		},
	}
}

func TestCanIgnoreHealthErrStatusInReconcile(t *testing.T) {
	var cluster = CreateTestClusterFromStatusDetails(map[string]cephv1.CephHealthMessage{
		"MDS_ALL_DOWN": {
			Severity: "HEALTH_ERR",
			Message:  "MDS_ALL_DOWN",
		},
		"TEST_OTHER": {
			Severity: "HEALTH_WARN",
			Message:  "TEST_OTHER",
		},
		"TEST_ANOTHER": {
			Severity: "HEALTH_OK",
			Message:  "TEST_ANOTHER",
		},
	})
	assert.True(t, canIgnoreHealthErrStatusInReconcile(cluster, "controller"))

	cluster = CreateTestClusterFromStatusDetails(map[string]cephv1.CephHealthMessage{
		"MDS_ALL_DOWN": {
			Severity: "HEALTH_ERR",
			Message:  "MDS_ALL_DOWN",
		},
		"TEST_UNIGNORABLE": {
			Severity: "HEALTH_ERR",
			Message:  "TEST_UNIGNORABLE",
		},
	})
	assert.False(t, canIgnoreHealthErrStatusInReconcile(cluster, "controller"))

	cluster = CreateTestClusterFromStatusDetails(map[string]cephv1.CephHealthMessage{
		"TEST_UNIGNORABLE": {
			Severity: "HEALTH_ERR",
			Message:  "TEST_UNIGNORABLE",
		},
	})
	assert.False(t, canIgnoreHealthErrStatusInReconcile(cluster, "controller"))
}

func TestSetCephCommandsTimeout(t *testing.T) {
	SetCephCommandsTimeout(map[string]string{})
	assert.Equal(t, 15*time.Second, exec.CephCommandsTimeout)

	exec.CephCommandsTimeout = 0
	SetCephCommandsTimeout(map[string]string{"ROOK_CEPH_COMMANDS_TIMEOUT_SECONDS": "0"})
	assert.Equal(t, 15*time.Second, exec.CephCommandsTimeout)

	exec.CephCommandsTimeout = 0
	SetCephCommandsTimeout(map[string]string{"ROOK_CEPH_COMMANDS_TIMEOUT_SECONDS": "1"})
	assert.Equal(t, 1*time.Second, exec.CephCommandsTimeout)
}
