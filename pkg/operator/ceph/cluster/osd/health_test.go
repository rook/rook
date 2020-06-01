/*
Copyright 2017 The Rook Authors. All rights reserved.

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
package osd

import (
	"fmt"
	"testing"
	"time"

	"github.com/rook/rook/pkg/clusterd"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	testexec "github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
)

func TestOSDHealthCheck(t *testing.T) {
	clientset := testexec.New(t, 2)
	cluster := "fake"

	var execCount = 0
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(command string, outFileArg string, args ...string) (string, error) {
			return "{\"key\":\"mysecurekey\", \"osdid\":3.0}", nil
		},
	}
	executor.MockExecuteCommandWithOutputFile = func(command string, outFileArg string, args ...string) (string, error) {
		logger.Infof("ExecuteCommandWithOutputFile: %s %v", command, args)
		execCount++
		if args[1] == "dump" {
			// Mock executor for OSD Dump command, returning an osd in Down state
			return `{"OSDs": [{"OSD": 0, "Up": 0, "In": 0}]}`, nil
		} else if args[1] == "safe-to-destroy" {
			// Mock executor for OSD Dump command, returning an osd in Down state
			return `{"safe_to_destroy":[0],"active":[],"missing_stats":[],"stored_pgs":[]}`, nil
		}
		return "", nil
	}

	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("ExecuteCommandWithOutput: %s %v", command, args)
		return "", nil
	}

	// Setting up objects needed to create OSD
	context := &clusterd.Context{
		Executor:  executor,
		Clientset: clientset,
	}

	labels := map[string]string{
		k8sutil.AppAttr:     AppName,
		k8sutil.ClusterAttr: cluster,
		OsdIdLabelKey:       "0",
	}

	deployment := &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "osd0",
			Namespace: cluster,
			Labels:    labels,
		},
	}
	if _, err := context.Clientset.AppsV1().Deployments(cluster).Create(deployment); err != nil {
		logger.Errorf("Error creating fake deployment: %v", err)
	}

	// Check if the osd deployment is created
	dp, _ := context.Clientset.AppsV1().Deployments(cluster).List(metav1.ListOptions{LabelSelector: fmt.Sprintf("%v=%d", OsdIdLabelKey, 0)})
	assert.Equal(t, 1, len(dp.Items))

	cephVersion := cephver.CephVersion{
		Major: 14,
	}

	// Initializing an OSD monitoring
	osdMon := NewOSDHealthMonitor(context, cluster, true, cephVersion)

	// Run OSD monitoring routine
	err := osdMon.checkOSDHealth()
	assert.Nil(t, err)
	// After creating an OSD, the dump has 1 mocked cmd and safe to destroy has 1 mocked cmd
	assert.Equal(t, 2, execCount)

	// Check if the osd deployment was deleted
	dp, _ = context.Clientset.AppsV1().Deployments(cluster).List(metav1.ListOptions{LabelSelector: fmt.Sprintf("%v=%d", OsdIdLabelKey, 0)})
	assert.Equal(t, 0, len(dp.Items))
}

func TestMonitorStart(t *testing.T) {
	cephVersion := cephver.CephVersion{
		Major: 14,
	}

	stopCh := make(chan struct{})
	osdMon := NewOSDHealthMonitor(&clusterd.Context{}, "cluster", true, cephVersion)
	logger.Infof("starting osd monitor")
	go osdMon.Start(stopCh)
	close(stopCh)
}

func TestOSDRestartIfStuck(t *testing.T) {
	clientset := testexec.New(t, 1)
	namespace := "test"
	// Setting up objects needed to create OSD
	context := &clusterd.Context{
		Clientset: clientset,
	}

	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "osd0",
			Namespace: namespace,
			Labels: map[string]string{
				"ceph-osd-id": "23",
				"portable":    "true",
			},
		},
	}
	pod.Spec.NodeName = "node0"
	_, err := context.Clientset.CoreV1().Pods(namespace).Create(&pod)
	assert.NoError(t, err)

	m := NewOSDHealthMonitor(context, namespace, false, cephver.CephVersion{})

	assert.NoError(t, k8sutil.ForceDeletePodIfStuck(m.context, pod))

	// The pod should still exist since it wasn't in a deleted state
	p, err := context.Clientset.CoreV1().Pods(namespace).Get(pod.Name, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, p)

	// Add a deletion timestamp to the pod
	pod.DeletionTimestamp = &metav1.Time{Time: time.Now()}
	_, err = context.Clientset.CoreV1().Pods(namespace).Update(&pod)
	assert.NoError(t, err)

	assert.NoError(t, k8sutil.ForceDeletePodIfStuck(m.context, pod))

	// The pod should still exist since the node is ready
	p, err = context.Clientset.CoreV1().Pods(namespace).Get(pod.Name, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, p)

	// Set the node to a not ready state
	nodes, err := context.Clientset.CoreV1().Nodes().List(metav1.ListOptions{})
	assert.NoError(t, err)
	for _, node := range nodes.Items {
		node.Status.Conditions[0].Status = v1.ConditionFalse
		_, err := context.Clientset.CoreV1().Nodes().Update(&node)
		assert.NoError(t, err)
	}

	assert.NoError(t, k8sutil.ForceDeletePodIfStuck(m.context, pod))

	// The pod should be deleted since the pod is marked as deleted and the node is not ready
	_, err = context.Clientset.CoreV1().Pods(namespace).Get(pod.Name, metav1.GetOptions{})
	assert.Error(t, err)
	assert.True(t, errors.IsNotFound(err))
}
