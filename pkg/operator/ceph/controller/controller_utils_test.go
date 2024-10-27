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
	ctx "context"
	"testing"
	"time"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/rook/rook/pkg/util/exec"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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

func TestSetAllowLoopDevices(t *testing.T) {
	SetAllowLoopDevices(map[string]string{})
	assert.False(t, LoopDevicesAllowed())

	SetAllowLoopDevices(map[string]string{"ROOK_CEPH_ALLOW_LOOP_DEVICES": "foo"})
	assert.False(t, LoopDevicesAllowed())

	SetAllowLoopDevices(map[string]string{"ROOK_CEPH_ALLOW_LOOP_DEVICES": "false"})
	assert.False(t, LoopDevicesAllowed())

	SetAllowLoopDevices(map[string]string{"ROOK_CEPH_ALLOW_LOOP_DEVICES": "true"})
	assert.True(t, LoopDevicesAllowed())
}

func TestSetEnforceHostNetwork(t *testing.T) {
	logger.Infof("testing default value for %v", enforceHostNetworkSettingName)
	opConfig := map[string]string{}
	SetEnforceHostNetwork(opConfig)
	assert.False(t, EnforceHostNetwork())

	// test invalid setting
	var value string = "foo"
	logger.Infof("testing invalid value'%v' for %v", value, enforceHostNetworkSettingName)
	opConfig[enforceHostNetworkSettingName] = value
	SetEnforceHostNetwork(opConfig)
	assert.False(t, EnforceHostNetwork())

	// test valid settings
	value = "true"
	logger.Infof("testing valid value'%v' for %v", value, enforceHostNetworkSettingName)
	opConfig[enforceHostNetworkSettingName] = value
	SetEnforceHostNetwork(opConfig)
	assert.True(t, EnforceHostNetwork())

	value = "false"
	logger.Infof("testing valid value'%v' for %v", value, enforceHostNetworkSettingName)
	opConfig[enforceHostNetworkSettingName] = value
	SetEnforceHostNetwork(opConfig)
	assert.False(t, EnforceHostNetwork())
}

func TestSetRevisionHistoryLimit(t *testing.T) {
	opConfig := map[string]string{}
	t.Run("ROOK_REVISION_HISTORY_LIMIT: test default value", func(t *testing.T) {
		SetRevisionHistoryLimit(opConfig)
		assert.Nil(t, RevisionHistoryLimit())
	})

	var value string = "foo"
	t.Run("ROOK_REVISION_HISTORY_LIMIT: test invalid value 'foo'", func(t *testing.T) {
		opConfig[revisionHistoryLimitSettingName] = value
		SetRevisionHistoryLimit(opConfig)
		assert.Nil(t, RevisionHistoryLimit())
	})

	t.Run("ROOK_REVISION_HISTORY_LIMIT: test empty string value", func(t *testing.T) {
		value = ""
		opConfig[revisionHistoryLimitSettingName] = value
		SetRevisionHistoryLimit(opConfig)
		assert.Nil(t, RevisionHistoryLimit())
	})
	t.Run("ROOK_REVISION_HISTORY_LIMIT:  test valig value '10'", func(t *testing.T) {
		value = "10"
		opConfig[revisionHistoryLimitSettingName] = value
		SetRevisionHistoryLimit(opConfig)
		assert.Equal(t, int32(10), *RevisionHistoryLimit())
	})
}
func TestIsReadyToReconcile(t *testing.T) {
	scheme := scheme.Scheme
	scheme.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephCluster{}, &cephv1.CephClusterList{})

	controllerName := "testing"
	clusterName := types.NamespacedName{Name: "mycluster", Namespace: "myns"}

	t.Run("non-existent cephcluster", func(t *testing.T) {
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects().Build()
		c, ready, clusterExists, reconcileResult := IsReadyToReconcile(ctx.TODO(), client, clusterName, controllerName)
		assert.NotNil(t, c)
		assert.False(t, ready)
		assert.False(t, clusterExists)
		assert.Equal(t, WaitForRequeueIfCephClusterNotReady, reconcileResult)
	})

	t.Run("valid cephcluster", func(t *testing.T) {
		cephCluster := &cephv1.CephCluster{}
		objects := []runtime.Object{cephCluster}
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objects...).Build()
		c, ready, clusterExists, reconcileResult := IsReadyToReconcile(ctx.TODO(), client, clusterName, controllerName)
		assert.NotNil(t, c)
		assert.False(t, ready)
		assert.False(t, clusterExists)
		assert.Equal(t, WaitForRequeueIfCephClusterNotReady, reconcileResult)
	})

	t.Run("deleted cephcluster with no cleanup policy", func(t *testing.T) {
		cephCluster := &cephv1.CephCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:              clusterName.Name,
				Namespace:         clusterName.Namespace,
				DeletionTimestamp: &metav1.Time{Time: time.Now()},
				Finalizers:        []string{"test"},
			},
		}

		objects := []runtime.Object{cephCluster}
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objects...).Build()
		c, ready, clusterExists, reconcileResult := IsReadyToReconcile(ctx.TODO(), client, clusterName, controllerName)
		assert.NotNil(t, c)
		assert.False(t, ready)
		assert.True(t, clusterExists)
		assert.Equal(t, WaitForRequeueIfCephClusterNotReady, reconcileResult)
	})

	t.Run("cephcluster with cleanup policy when not deleted", func(t *testing.T) {
		cephCluster := &cephv1.CephCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clusterName.Name,
				Namespace: clusterName.Namespace,
			},
			Spec: cephv1.ClusterSpec{
				CleanupPolicy: cephv1.CleanupPolicySpec{
					Confirmation: cephv1.DeleteDataDirOnHostsConfirmation,
				},
			}}
		objects := []runtime.Object{cephCluster}
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objects...).Build()
		c, ready, clusterExists, _ := IsReadyToReconcile(ctx.TODO(), client, clusterName, controllerName)
		assert.NotNil(t, c)
		assert.False(t, ready)
		assert.True(t, clusterExists)
	})

	t.Run("cephcluster with cleanup policy when deleted", func(t *testing.T) {
		cephCluster := &cephv1.CephCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:              clusterName.Name,
				Namespace:         clusterName.Namespace,
				DeletionTimestamp: &metav1.Time{Time: time.Now()},
				Finalizers:        []string{"test"},
			},
			Spec: cephv1.ClusterSpec{
				CleanupPolicy: cephv1.CleanupPolicySpec{
					Confirmation: cephv1.DeleteDataDirOnHostsConfirmation,
				},
			}}
		objects := []runtime.Object{cephCluster}
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objects...).Build()
		c, ready, clusterExists, _ := IsReadyToReconcile(ctx.TODO(), client, clusterName, controllerName)
		assert.NotNil(t, c)
		assert.False(t, ready)
		assert.False(t, clusterExists)
	})
}
