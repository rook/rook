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
package osd

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
)

func TestOrchestrationStatus(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	clusterInfo := &cephconfig.ClusterInfo{
		CephVersion: cephver.Nautilus,
	}
	c := New(clusterInfo, &clusterd.Context{Clientset: clientset, ConfigDir: "/var/lib/rook", Executor: &exectest.MockExecutor{}}, "ns", "myversion", cephv1.CephVersionSpec{},
		rookalpha.StorageScopeSpec{}, "", rookalpha.Placement{}, rookalpha.Annotations{}, cephv1.NetworkSpec{}, v1.ResourceRequirements{}, v1.ResourceRequirements{}, "my-priority-class", metav1.OwnerReference{}, false, false, false)
	kv := k8sutil.NewConfigMapKVStore(c.Namespace, clientset, metav1.OwnerReference{})
	nodeName := "mynode"
	cmName := fmt.Sprintf(orchestrationStatusMapName, nodeName)

	// status map should not exist yet
	_, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Get(cmName, metav1.GetOptions{})
	assert.True(t, errors.IsNotFound(err))

	// update the status map with some status
	status := OrchestrationStatus{Status: OrchestrationStatusOrchestrating, Message: "doing work"}
	err = UpdateNodeStatus(kv, nodeName, status)
	assert.Nil(t, err)

	// retrieve the status and verify it
	statusMap, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Get(cmName, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, statusMap)
	retrievedStatus := parseOrchestrationStatus(statusMap.Data)
	assert.NotNil(t, retrievedStatus)
	assert.Equal(t, status, *retrievedStatus)
}

func mockNodeOrchestrationCompletion(c *Cluster, nodeName string, statusMapWatcher *watch.FakeWatcher) {
	// if no valid osd node, don't need to check its status, return immediately
	if len(c.DesiredStorage.Nodes) == 0 {
		return
	}
	for {
		// wait for the node's orchestration status to change to "starting"
		cmName := fmt.Sprintf(orchestrationStatusMapName, nodeName)
		cm, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Get(cmName, metav1.GetOptions{})
		if err == nil {
			status := parseOrchestrationStatus(cm.Data)
			if status != nil && status.Status == OrchestrationStatusStarting {
				// the node has started orchestration, simulate its completion now by performing 2 tasks:
				// 1) update the config map manually (which doesn't trigger a watch event, see https://github.com/kubernetes/kubernetes/issues/54075#issuecomment-337298950)
				status = &OrchestrationStatus{
					OSDs: []OSDInfo{
						{
							ID:          1,
							DataPath:    "/tmp",
							Config:      "/foo/bar/ceph.conf",
							Cluster:     "rook",
							KeyringPath: "/foo/bar/key",
						},
					},
					Status: OrchestrationStatusCompleted,
				}
				UpdateNodeStatus(c.kv, nodeName, *status)

				// 2) call modify on the fake watcher so a watch event will get triggered
				s, _ := json.Marshal(status)
				cm.Data[orchestrationStatusKey] = string(s)
				statusMapWatcher.Modify(cm)
				break
			} else {
				logger.Debugf("waiting for node %s orchestration to start. status: %+v", nodeName, *status)
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
		cm, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Get(orchestrationStatusMapName, metav1.GetOptions{})
		if err == nil {
			status := parseOrchestrationStatus(cm.Data)
			if status != nil {
				logger.Debugf("start has not completed, status is %+v", status)
			}
		}
		<-time.After(50 * time.Millisecond)
	}
}
