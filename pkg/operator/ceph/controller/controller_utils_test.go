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
	"context"
	"testing"
	"time"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util/exec"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kfake "k8s.io/client-go/kubernetes/fake"
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
	clientset := kfake.NewSimpleClientset()
	ctx := context.TODO()
	cm := &v1.ConfigMap{}
	cm.Name = "rook-ceph-operator-config"
	_, err := clientset.CoreV1().ConfigMaps("").Create(ctx, cm, metav1.CreateOptions{})
	assert.NoError(t, err)
	context := &clusterd.Context{Clientset: clientset}

	SetCephCommandsTimeout(context)
	assert.Equal(t, 15*time.Second, exec.CephCommandsTimeout)

	exec.CephCommandsTimeout = 0
	cm.Data = map[string]string{"ROOK_CEPH_COMMANDS_TIMEOUT_SECONDS": "0"}
	_, err = clientset.CoreV1().ConfigMaps("").Update(ctx, cm, metav1.UpdateOptions{})
	assert.NoError(t, err)
	SetCephCommandsTimeout(context)
	assert.Equal(t, 15*time.Second, exec.CephCommandsTimeout)

	exec.CephCommandsTimeout = 0
	cm.Data = map[string]string{"ROOK_CEPH_COMMANDS_TIMEOUT_SECONDS": "1"}
	_, err = clientset.CoreV1().ConfigMaps("").Update(ctx, cm, metav1.UpdateOptions{})
	assert.NoError(t, err)
	SetCephCommandsTimeout(context)
	assert.Equal(t, 1*time.Second, exec.CephCommandsTimeout)
}

func TestIsReadyToReconcile(t *testing.T) {
	scheme := scheme.Scheme
	scheme.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephCluster{}, &cephv1.CephClusterList{})

	controllerName := "testing"
	clusterName := types.NamespacedName{Name: "mycluster", Namespace: "myns"}

	t.Run("non-existent cephcluster", func(t *testing.T) {
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects().Build()
		c, ready, clusterExists, reconcileResult := IsReadyToReconcile(client, clusterName, controllerName)
		assert.NotNil(t, c)
		assert.False(t, ready)
		assert.False(t, clusterExists)
		assert.Equal(t, WaitForRequeueIfCephClusterNotReady, reconcileResult)
	})

	t.Run("valid cephcluster", func(t *testing.T) {
		cephCluster := &cephv1.CephCluster{}
		objects := []runtime.Object{cephCluster}
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objects...).Build()
		c, ready, clusterExists, reconcileResult := IsReadyToReconcile(client, clusterName, controllerName)
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
			},
		}

		objects := []runtime.Object{cephCluster}
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objects...).Build()
		c, ready, clusterExists, reconcileResult := IsReadyToReconcile(client, clusterName, controllerName)
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
		c, ready, clusterExists, _ := IsReadyToReconcile(client, clusterName, controllerName)
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
			},
			Spec: cephv1.ClusterSpec{
				CleanupPolicy: cephv1.CleanupPolicySpec{
					Confirmation: cephv1.DeleteDataDirOnHostsConfirmation,
				},
			}}
		objects := []runtime.Object{cephCluster}
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objects...).Build()
		c, ready, clusterExists, _ := IsReadyToReconcile(client, clusterName, controllerName)
		assert.NotNil(t, c)
		assert.False(t, ready)
		assert.False(t, clusterExists)
	})
}
