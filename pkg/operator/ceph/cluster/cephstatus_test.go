/*
Copyright 2019 The Rook Authors. All rights reserved.

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

// Package cluster to manage Kubernetes storage.
package cluster

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	optest "github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCephStatus(t *testing.T) {
	newStatus := &cephclient.CephStatus{
		Health: cephclient.HealthStatus{Status: "HEALTH_OK"},
	}

	// Empty initial status will have no previous health
	currentStatus := cephv1.ClusterStatus{}
	aggregateStatus := toCustomResourceStatus(currentStatus, newStatus)
	assert.NotNil(t, aggregateStatus)
	assert.Equal(t, "HEALTH_OK", aggregateStatus.Health)
	assert.NotEqual(t, "", aggregateStatus.LastChecked)
	assert.Equal(t, "", aggregateStatus.LastChanged)
	assert.Equal(t, "", aggregateStatus.PreviousHealth)
	assert.Equal(t, 0, len(aggregateStatus.Details))

	// Set the current status to the same as the new status and there will be no previous health
	currentStatus.CephStatus = &cephv1.CephStatus{
		Health: "HEALTH_OK",
	}
	aggregateStatus = toCustomResourceStatus(currentStatus, newStatus)
	assert.NotNil(t, aggregateStatus)
	assert.Equal(t, "HEALTH_OK", aggregateStatus.Health)
	assert.NotEqual(t, "", aggregateStatus.LastChecked)
	assert.Equal(t, "", aggregateStatus.LastChanged)
	assert.Equal(t, "", aggregateStatus.PreviousHealth)
	assert.Equal(t, 0, len(aggregateStatus.Details))

	// Set the new status to something different and we should get a previous health
	// Simulate the previous check a minute ago.
	previousTime := formatTime(time.Now().Add(-time.Minute).UTC())
	currentStatus.CephStatus.LastChecked = previousTime
	newStatus.Health.Status = "HEALTH_WARN"
	aggregateStatus = toCustomResourceStatus(currentStatus, newStatus)
	assert.NotNil(t, aggregateStatus)
	assert.Equal(t, "HEALTH_WARN", aggregateStatus.Health)
	assert.NotEqual(t, "", aggregateStatus.LastChecked)
	assert.Equal(t, aggregateStatus.LastChecked, aggregateStatus.LastChanged)
	assert.Equal(t, "HEALTH_OK", aggregateStatus.PreviousHealth)
	assert.Equal(t, 0, len(aggregateStatus.Details))

	// Add some details to the warning
	osdDownMsg := cephclient.CheckMessage{Severity: "HEALTH_WARN"}
	osdDownMsg.Summary.Message = "1 osd down"
	pgAvailMsg := cephclient.CheckMessage{Severity: "HEALTH_ERR"}
	pgAvailMsg.Summary.Message = "'Reduced data availability: 100 pgs stale'"
	newStatus.Health.Checks = map[string]cephclient.CheckMessage{
		"OSD_DOWN":        osdDownMsg,
		"PG_AVAILABILITY": pgAvailMsg,
	}
	newStatus.Health.Status = "HEALTH_ERR"
	aggregateStatus = toCustomResourceStatus(currentStatus, newStatus)
	assert.NotNil(t, aggregateStatus)
	assert.Equal(t, "HEALTH_ERR", aggregateStatus.Health)
	assert.NotEqual(t, "", aggregateStatus.LastChecked)
	assert.Equal(t, aggregateStatus.LastChecked, aggregateStatus.LastChanged)
	assert.Equal(t, "HEALTH_OK", aggregateStatus.PreviousHealth)
	assert.Equal(t, 2, len(aggregateStatus.Details))
	assert.Equal(t, osdDownMsg.Summary.Message, aggregateStatus.Details["OSD_DOWN"].Message)
	assert.Equal(t, osdDownMsg.Severity, aggregateStatus.Details["OSD_DOWN"].Severity)
	assert.Equal(t, pgAvailMsg.Summary.Message, aggregateStatus.Details["PG_AVAILABILITY"].Message)
	assert.Equal(t, pgAvailMsg.Severity, aggregateStatus.Details["PG_AVAILABILITY"].Severity)

	// Test for storage capacity of the ceph cluster when there is no disk
	newStatus = &cephclient.CephStatus{
		PgMap: cephclient.PgMap{TotalBytes: 0},
	}
	aggregateStatus = toCustomResourceStatus(currentStatus, newStatus)
	assert.Equal(t, 0, int(aggregateStatus.Capacity.TotalBytes))
	assert.Equal(t, "", aggregateStatus.Capacity.LastUpdated)

	// Test for storage capacity of the ceph cluster when the disk of size 1024 bytes attached
	newStatus = &cephclient.CephStatus{
		PgMap: cephclient.PgMap{TotalBytes: 1024},
	}
	aggregateStatus = toCustomResourceStatus(currentStatus, newStatus)
	assert.Equal(t, 1024, int(aggregateStatus.Capacity.TotalBytes))
	assert.Equal(t, formatTime(time.Now().UTC()), aggregateStatus.Capacity.LastUpdated)

	// Test for storage capacity of the ceph cluster when initially there is a disk of size
	// 1024 bytes attached and then the disk is removed or newStatus.PgMap.TotalBytes is 0.
	currentStatus.CephStatus.Capacity.TotalBytes = 1024
	newStatus = &cephclient.CephStatus{
		PgMap: cephclient.PgMap{TotalBytes: 0},
	}

	aggregateStatus = toCustomResourceStatus(currentStatus, newStatus)
	assert.Equal(t, 1024, int(aggregateStatus.Capacity.TotalBytes))
	assert.Equal(t, formatTime(time.Now().Add(-time.Minute).UTC()), formatTime(time.Now().Add(-time.Minute).UTC()))
}

func TestNewCephStatusChecker(t *testing.T) {
	clusterInfo := cephclient.AdminClusterInfo("ns")
	c := &clusterd.Context{}
	time10s, err := time.ParseDuration("10s")
	assert.NoError(t, err)

	type args struct {
		context     *clusterd.Context
		clusterInfo *cephclient.ClusterInfo
		clusterSpec *cephv1.ClusterSpec
	}
	tests := []struct {
		name string
		args args
		want *cephStatusChecker
	}{
		{"default-interval", args{c, clusterInfo, &cephv1.ClusterSpec{}}, &cephStatusChecker{c, clusterInfo, &defaultStatusCheckInterval, c.Client, false}},
		{"10s-interval", args{c, clusterInfo, &cephv1.ClusterSpec{HealthCheck: cephv1.CephClusterHealthCheckSpec{DaemonHealth: cephv1.DaemonHealthSpec{Status: cephv1.HealthCheckSpec{Interval: &metav1.Duration{Duration: time10s}}}}}}, &cephStatusChecker{c, clusterInfo, &time10s, c.Client, false}},
		{"10s-interval-external", args{c, clusterInfo, &cephv1.ClusterSpec{External: cephv1.ExternalSpec{Enable: true}, HealthCheck: cephv1.CephClusterHealthCheckSpec{DaemonHealth: cephv1.DaemonHealthSpec{Status: cephv1.HealthCheckSpec{Interval: &metav1.Duration{Duration: time10s}}}}}}, &cephStatusChecker{c, clusterInfo, &time10s, c.Client, true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := newCephStatusChecker(tt.args.context, tt.args.clusterInfo, tt.args.clusterSpec); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("newCephStatusChecker() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfigureHealthSettings(t *testing.T) {
	c := &cephStatusChecker{
		context:     &clusterd.Context{},
		clusterInfo: cephclient.AdminClusterInfo("ns"),
	}
	setGlobalIDReclaim := false
	c.context.Executor = &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(command string, args ...string) (string, error) {
			logger.Infof("Command: %s %v", command, args)
			if args[0] == "config" && args[3] == "auth_allow_insecure_global_id_reclaim" {
				if args[1] == "set" {
					setGlobalIDReclaim = true
					return "", nil
				}
			}
			return "", errors.New("mock error to simulate failure of mon store config")
		},
	}
	noActionOneWarningStatus := cephclient.CephStatus{
		Health: cephclient.HealthStatus{
			Checks: map[string]cephclient.CheckMessage{
				"MDS_ALL_DOWN": {
					Severity: "HEALTH_WARN",
					Summary: cephclient.Summary{
						Message: "MDS_ALL_DOWN",
					},
				},
			},
		},
	}
	disableInsecureGlobalIDStatus := cephclient.CephStatus{
		Health: cephclient.HealthStatus{
			Checks: map[string]cephclient.CheckMessage{
				"AUTH_INSECURE_GLOBAL_ID_RECLAIM_ALLOWED": {
					Severity: "HEALTH_WARN",
					Summary: cephclient.Summary{
						Message: "foo",
					},
				},
			},
		},
	}
	noDisableInsecureGlobalIDStatus := cephclient.CephStatus{
		Health: cephclient.HealthStatus{
			Checks: map[string]cephclient.CheckMessage{
				"AUTH_INSECURE_GLOBAL_ID_RECLAIM_ALLOWED": {
					Severity: "HEALTH_WARN",
					Summary: cephclient.Summary{
						Message: "foo",
					},
				},
				"AUTH_INSECURE_GLOBAL_ID_RECLAIM": {
					Severity: "HEALTH_WARN",
					Summary: cephclient.Summary{
						Message: "bar",
					},
				},
			},
		},
	}

	type args struct {
		status                     cephclient.CephStatus
		expectedSetGlobalIDSetting bool
	}
	tests := []struct {
		name string
		args args
	}{
		{"no-warnings", args{cephclient.CephStatus{}, false}},
		{"no-action-one-warning", args{noActionOneWarningStatus, false}},
		{"disable-insecure-global-id", args{disableInsecureGlobalIDStatus, true}},
		{"no-disable-insecure-global-id", args{noDisableInsecureGlobalIDStatus, false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setGlobalIDReclaim = false
			c.configureHealthSettings(tt.args.status)
			assert.Equal(t, tt.args.expectedSetGlobalIDSetting, setGlobalIDReclaim)
		})
	}
}

func TestForceDeleteStuckRookPodsOnNotReadyNodes(t *testing.T) {
	ctx := context.TODO()
	clientset := optest.New(t, 1)
	clusterInfo := cephclient.NewClusterInfo("test", "test")
	clusterName := clusterInfo.NamespacedName()

	context := &clusterd.Context{
		Clientset: clientset,
	}

	c := newCephStatusChecker(context, clusterInfo, &cephv1.ClusterSpec{})

	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "stuck-pod",
			Namespace: clusterName.Namespace,
			Labels: map[string]string{
				"app": "rook-ceph-osd",
			},
		},
	}
	pod.Spec.NodeName = "node0"
	_, err := context.Clientset.CoreV1().Pods(clusterName.Namespace).Create(ctx, &pod, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Create a non matching pod
	notDeletePod := pod
	notDeletePod.ObjectMeta.Labels = map[string]string{"app": "not-to-be-deleted"}
	notDeletePod.ObjectMeta.Name = "not-to-be-deleted"
	notDeletePod.DeletionTimestamp = &metav1.Time{Time: time.Now()}
	_, err = context.Clientset.CoreV1().Pods(clusterName.Namespace).Create(ctx, &notDeletePod, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Set the node to NotReady state
	nodes, err := context.Clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	assert.NoError(t, err)
	for _, node := range nodes.Items {
		node.Status.Conditions[0].Status = v1.ConditionFalse
		localnode := node
		_, err := context.Clientset.CoreV1().Nodes().Update(ctx, &localnode, metav1.UpdateOptions{})
		assert.NoError(t, err)
	}

	// There should be no error
	err = c.forceDeleteStuckRookPodsOnNotReadyNodes()
	assert.NoError(t, err)

	// The pod should still exist since its not deleted.
	p, err := context.Clientset.CoreV1().Pods(clusterInfo.Namespace).Get(ctx, pod.Name, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, p)

	// Add a deletion timestamp to the pod
	pod.DeletionTimestamp = &metav1.Time{Time: time.Now()}
	_, err = clientset.CoreV1().Pods(clusterName.Namespace).Update(ctx, &pod, metav1.UpdateOptions{})
	assert.NoError(t, err)

	// There should be no error as the pod is deleted
	err = c.forceDeleteStuckRookPodsOnNotReadyNodes()
	assert.NoError(t, err)

	// The pod should be deleted since the pod is marked as deleted and the node is in NotReady state
	_, err = clientset.CoreV1().Pods(clusterName.Namespace).Get(ctx, pod.Name, metav1.GetOptions{})
	assert.Error(t, err)
	assert.True(t, kerrors.IsNotFound(err))

	// The pod should not be deleted as it does not have the matching labels
	_, err = clientset.CoreV1().Pods(clusterName.Namespace).Get(ctx, notDeletePod.Name, metav1.GetOptions{})
	assert.NoError(t, err)
}

func TestGetRookPodsOnNode(t *testing.T) {
	ctx := context.TODO()
	clientset := optest.New(t, 1)
	clusterInfo := cephclient.NewClusterInfo("test", "test")
	clusterName := clusterInfo.NamespacedName()
	context := &clusterd.Context{
		Clientset: clientset,
	}

	c := newCephStatusChecker(context, clusterInfo, &cephv1.ClusterSpec{})
	labels := []map[string]string{
		{"app": "rook-ceph-osd"},
		{"app": "csi-rbdplugin-provisioner"},
		{"app": "csi-rbdplugin"},
		{"app": "csi-cephfsplugin-provisioner"},
		{"app": "csi-cephfsplugin"},
		{"app": "rook-ceph-operator"},
		{"app": "rook-ceph-crashcollector"},
		{"app": "rook-ceph-mgr"},
		{"app": "rook-ceph-mds"},
		{"app": "rook-ceph-rgw"},
		{"app": "user-app"},
		{"app": "rook-ceph-mon"},
	}

	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-with-no-label",
			Namespace: clusterName.Namespace,
		},
	}
	pod.Spec.NodeName = "node0"
	_, err := context.Clientset.CoreV1().Pods(clusterName.Namespace).Create(ctx, &pod, metav1.CreateOptions{})
	assert.NoError(t, err)

	expectedPodNames := []string{}
	for i, label := range labels {
		pod.ObjectMeta.Name = fmt.Sprintf("pod-%d", i)
		pod.ObjectMeta.Namespace = clusterName.Namespace
		pod.ObjectMeta.Labels = label
		if label["app"] != "user-app" {
			expectedPodNames = append(expectedPodNames, pod.Name)
		}
		_, err := context.Clientset.CoreV1().Pods(clusterName.Namespace).Create(ctx, &pod, metav1.CreateOptions{})
		assert.NoError(t, err)
	}

	pods, err := c.getRookPodsOnNode("node0")
	assert.NoError(t, err)
	// A pod is having two matching labels and its returned only once
	assert.Equal(t, 11, len(pods))

	podNames := []string{}
	for _, pod := range pods {
		// Check if the pods has labels
		assert.NotEmpty(t, pod.Labels)
		podNames = append(podNames, pod.Name)
	}

	sort.Strings(expectedPodNames)
	sort.Strings(podNames)
	assert.Equal(t, expectedPodNames, podNames)
}
