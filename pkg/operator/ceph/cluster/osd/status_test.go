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
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
)

func TestOrchestrationStatus(t *testing.T) {
	ctx := context.TODO()
	clientset := fake.NewSimpleClientset()
	clusterInfo := &cephclient.ClusterInfo{
		Namespace:   "ns",
		CephVersion: cephver.Octopus,
	}
	context := &clusterd.Context{Clientset: clientset, ConfigDir: "/var/lib/rook", Executor: &exectest.MockExecutor{}}
	spec := cephv1.ClusterSpec{}
	c := New(context, clusterInfo, spec, "myversion")
	ownerInfo := cephclient.NewMinimumOwnerInfoWithOwnerRef()
	kv := k8sutil.NewConfigMapKVStore(c.clusterInfo.Namespace, clientset, ownerInfo)
	nodeName := "mynode"
	cmName := fmt.Sprintf(orchestrationStatusMapName, nodeName)

	// status map should not exist yet
	_, err := c.context.Clientset.CoreV1().ConfigMaps(c.clusterInfo.Namespace).Get(ctx, cmName, metav1.GetOptions{})
	assert.True(t, errors.IsNotFound(err))

	// update the status map with some status
	status := OrchestrationStatus{Status: OrchestrationStatusOrchestrating, Message: "doing work"}
	UpdateNodeOrPVCStatus(ctx, kv, nodeName, status)

	// retrieve the status and verify it
	statusMap, err := c.context.Clientset.CoreV1().ConfigMaps(c.clusterInfo.Namespace).Get(ctx, cmName, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, statusMap)
	retrievedStatus := parseOrchestrationStatus(statusMap.Data)
	assert.NotNil(t, retrievedStatus)
	assert.Equal(t, status, *retrievedStatus)
}

func mockNodeOrchestrationCompletion(c *Cluster, nodeName string, statusMapWatcher *watch.FakeWatcher) {
	ctx := context.TODO()
	// if no valid osd node, don't need to check its status, return immediately
	if len(c.spec.Storage.Nodes) == 0 {
		return
	}
	for {
		// wait for the node's orchestration status to change to "starting"
		cmName := statusConfigMapName(nodeName)
		cm, err := c.context.Clientset.CoreV1().ConfigMaps(c.clusterInfo.Namespace).Get(ctx, cmName, metav1.GetOptions{})
		if err == nil {
			status := parseOrchestrationStatus(cm.Data)
			if status != nil && status.Status == OrchestrationStatusStarting {
				// the node has started orchestration, simulate its completion now by performing 2 tasks:
				// 1) update the config map manually (which doesn't trigger a watch event, see https://github.com/kubernetes/kubernetes/issues/54075#issuecomment-337298950)
				status = &OrchestrationStatus{
					OSDs: []OSDInfo{
						{
							ID:        1,
							UUID:      "000000-0000-00000001",
							Cluster:   "rook",
							CVMode:    "raw",
							BlockPath: "/dev/some/path",
						},
					},
					Status: OrchestrationStatusCompleted,
				}
				UpdateNodeOrPVCStatus(ctx, c.kv, nodeName, *status)

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
	ctx := context.TODO()
	for {
		if *startCompleted {
			break
		}
		cmName := statusConfigMapName(nodeName)
		cm, err := c.context.Clientset.CoreV1().ConfigMaps(c.clusterInfo.Namespace).Get(ctx, cmName, metav1.GetOptions{})
		if err == nil {
			status := parseOrchestrationStatus(cm.Data)
			if status != nil {
				logger.Debugf("start has not completed, status is %+v", status)
			}
		}
		<-time.After(50 * time.Millisecond)
	}
}
