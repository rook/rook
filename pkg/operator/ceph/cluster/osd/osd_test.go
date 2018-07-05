/*
Copyright 2016 The Rook Authors. All rights reserved.

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
	"encoding/json"
	"fmt"
	"testing"
	"time"

	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	"github.com/rook/rook/pkg/operator/k8sutil"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestStart(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	c := New(&clusterd.Context{Clientset: clientset, ConfigDir: "/var/lib/rook", Executor: &exectest.MockExecutor{}}, "ns", "myversion", "",
		rookalpha.StorageScopeSpec{}, "", rookalpha.Placement{}, false, v1.ResourceRequirements{}, metav1.OwnerReference{})

	// Start the first time
	err := c.Start()
	assert.Nil(t, err)

	// Should not fail if it already exists
	err = c.Start()
	assert.Nil(t, err)
}

func TestAddRemoveNode(t *testing.T) {
	// create a storage spec with the given nodes/devices/dirs
	nodeName := "node8230"
	storageSpec := rookalpha.StorageScopeSpec{
		Nodes: []rookalpha.Node{
			{
				Name: nodeName,
				Selection: rookalpha.Selection{
					Devices:     []rookalpha.Device{{Name: "sdx"}},
					Directories: []rookalpha.Directory{{Path: "/rook/storage1"}},
				},
			},
		},
	}

	// set up a fake k8s client set and watcher to generate events that the operator will listen to
	clientset := fake.NewSimpleClientset()
	statusMapWatcher := watch.NewFake()
	clientset.PrependWatchReactor("configmaps", k8stesting.DefaultWatchReactor(statusMapWatcher, nil))

	c := New(&clusterd.Context{Clientset: clientset, ConfigDir: "/var/lib/rook", Executor: &exectest.MockExecutor{}}, "ns-add-remove", "myversion", "",
		storageSpec, "", rookalpha.Placement{}, false, v1.ResourceRequirements{}, metav1.OwnerReference{})

	// kick off the start of the orchestration in a goroutine
	var startErr error
	startCompleted := false
	go func() {
		startErr = c.Start()
		startCompleted = true
	}()

	// simulate the completion of the nodes orchestration
	mockNodeOrchestrationCompletion(c, nodeName, statusMapWatcher)

	// wait for orchestration to complete
	waitForOrchestrationCompletion(c, nodeName, &startCompleted)

	// verify orchestration for adding the node succeeded
	assert.True(t, startCompleted)
	assert.Nil(t, startErr)

	// Now let's get ready for testing the removal of the node we just added.  We first need to simulate/mock some things:

	// simulate the node having created an OSD dir map
	kvstore := k8sutil.NewConfigMapKVStore(c.Namespace, c.context.Clientset, metav1.OwnerReference{})
	config.SaveOSDDirMap(kvstore, nodeName, map[string]int{"/rook/storage1": 0})

	// simulate the OSD pod having been created
	osdPod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name:            "osdPod",
		Labels:          map[string]string{k8sutil.AppAttr: appName},
		OwnerReferences: []metav1.OwnerReference{{Name: "rook-ceph-osd-node8230"}},
	}}
	c.context.Clientset.CoreV1().Pods(c.Namespace).Create(osdPod)

	// mock the ceph calls that will be called during remove node
	mockExec := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(debug bool, actionName, command, outputFile string, args ...string) (string, error) {
			logger.Infof("Command: %s %v", command, args)
			if args[0] == "status" {
				return `{"pgmap":{"num_pgs":100,"pgs_by_state":[{"state_name":"active+clean","count":100}]}}`, nil
			}
			if args[0] == "osd" && args[1] == "df" {
				return `{"nodes":[{"id":0,"name":"osd.0","kb_used":1}]}`, nil
			}
			if args[0] == "df" && args[1] == "detail" {
				return `{"stats":{"total_bytes":4096,"total_used_bytes":1024,"total_avail_bytes":3072}}`, nil
			}
			if args[0] == "osd" && (args[1] == "set" || args[1] == "unset") {
				return "", nil
			}
			return "", fmt.Errorf("unexpected ceph command '%v'", args)
		},
	}

	// modify the storage spec to remove the node from the cluster
	storageSpec.Nodes = []rookalpha.Node{}
	c = New(&clusterd.Context{Clientset: clientset, ConfigDir: "/var/lib/rook", Executor: mockExec}, "ns-add-remove", "myversion", "",
		storageSpec, "", rookalpha.Placement{}, false, v1.ResourceRequirements{}, metav1.OwnerReference{})

	// reset the orchestration status watcher
	statusMapWatcher = watch.NewFake()
	clientset.PrependWatchReactor("configmaps", k8stesting.DefaultWatchReactor(statusMapWatcher, nil))

	// kick off the start of the removal orchestration in a goroutine
	startErr = nil
	startCompleted = false
	go func() {
		startErr = c.Start()
		startCompleted = true
	}()

	// simulate the completion of the removed nodes orchestration
	mockNodeOrchestrationCompletion(c, nodeName, statusMapWatcher)

	// wait for orchestration to complete
	waitForOrchestrationCompletion(c, nodeName, &startCompleted)

	// verify orchestration for removing the node succeeded
	assert.True(t, startCompleted)
	assert.Nil(t, startErr)
}

func TestAddNodeFailure(t *testing.T) {
	// create a storage spec with the given nodes/devices/dirs
	nodeName := "node1672"
	storageSpec := rookalpha.StorageScopeSpec{
		Nodes: []rookalpha.Node{
			{
				Name: nodeName,
				Selection: rookalpha.Selection{
					Devices:     []rookalpha.Device{{Name: "sdx"}},
					Directories: []rookalpha.Directory{{Path: "/rook/storage1"}},
				},
			},
		},
	}

	// create a fake clientset that will return an error when the operator tries to create a replica set
	clientset := fake.NewSimpleClientset()
	clientset.PrependReactor("create", "replicasets", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, fmt.Errorf("mock failed to create replica set")
	})

	c := New(&clusterd.Context{Clientset: clientset, ConfigDir: "/var/lib/rook", Executor: &exectest.MockExecutor{}}, "ns-add-remove", "myversion", "",
		storageSpec, "", rookalpha.Placement{}, false, v1.ResourceRequirements{}, metav1.OwnerReference{})

	// kick off the start of the orchestration in a goroutine
	var startErr error
	startCompleted := false
	go func() {
		startErr = c.Start()
		startCompleted = true
	}()

	// wait for orchestration to complete
	waitForOrchestrationCompletion(c, nodeName, &startCompleted)

	// verify orchestration failed (because the operator failed to create a replica set)
	assert.True(t, startCompleted)
	assert.NotNil(t, startErr)
}

func TestOrchestrationStatus(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	c := New(&clusterd.Context{Clientset: clientset, ConfigDir: "/var/lib/rook", Executor: &exectest.MockExecutor{}}, "ns", "myversion", "",
		rookalpha.StorageScopeSpec{}, "", rookalpha.Placement{}, false, v1.ResourceRequirements{}, metav1.OwnerReference{})

	// status map should not exist yet
	_, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Get(OrchestrationStatusMapName, metav1.GetOptions{})
	assert.True(t, errors.IsNotFound(err))

	// make the initial status map
	err = makeOrchestrationStatusMap(c.context.Clientset, c.Namespace, nil)
	assert.Nil(t, err)

	// the status map should exist now
	statusMap, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Get(OrchestrationStatusMapName, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, statusMap)

	// update the status map with some status
	nodeName := "node09238"
	status := OrchestrationStatus{Status: OrchestrationStatusOrchestrating, Message: "doing work"}
	err = UpdateOrchestrationStatusMap(c.context.Clientset, c.Namespace, nodeName, status)
	assert.Nil(t, err)

	// retrieve the status and verify it
	statusMap, err = c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Get(OrchestrationStatusMapName, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, statusMap)
	retrievedStatus := parseOrchestrationStatus(statusMap.Data, nodeName)
	assert.NotNil(t, retrievedStatus)
	assert.Equal(t, status, *retrievedStatus)
}

func mockNodeOrchestrationCompletion(c *Cluster, nodeName string, statusMapWatcher *watch.FakeWatcher) {
	for {
		// wait for the node's orchestration status to change to "starting"
		cm, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Get(OrchestrationStatusMapName, metav1.GetOptions{})
		if err == nil {
			status := parseOrchestrationStatus(cm.Data, nodeName)
			if status != nil && status.Status == OrchestrationStatusStarting {
				// the node has started orchestration, simulate its completion now by performing 2 tasks:
				// 1) update the config map manually (which doesn't trigger a watch event, see https://github.com/kubernetes/kubernetes/issues/54075#issuecomment-337298950)
				status = &OrchestrationStatus{Status: OrchestrationStatusCompleted}
				UpdateOrchestrationStatusMap(c.context.Clientset, c.Namespace, nodeName, *status)

				// 2) call modify on the fake watcher so a watch event will get triggered
				s, _ := json.Marshal(status)
				cm.Data[nodeName] = string(s)
				statusMapWatcher.Modify(cm)
				break
			} else {
				logger.Infof("waiting for node %s orchestration to start. status: %+v", nodeName, *status)
			}
		} else {
			logger.Warningf("failed to get node %s orchestration status, will try again: %+v", nodeName, err)
		}
		<-time.After(50 * time.Millisecond)
	}
}

func waitForOrchestrationCompletion(c *Cluster, nodeName string, startCompleted *bool) {
	for {
		if *startCompleted {
			break
		}
		cm, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Get(OrchestrationStatusMapName, metav1.GetOptions{})
		if err == nil {
			status := parseOrchestrationStatus(cm.Data, nodeName)
			if status != nil {
				logger.Infof("start has not completed, status is %+v", status)
			}
		}
		<-time.After(50 * time.Millisecond)
	}
}
