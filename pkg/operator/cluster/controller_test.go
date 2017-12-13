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

Some of the code below came from https://github.com/coreos/etcd-operator
which also has the apache 2.0 license.
*/
package cluster

import (
	"fmt"
	"os"
	"testing"
	"time"

	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/agent/flexvolume/attachment"
	"github.com/rook/rook/pkg/operator/k8sutil"
	testop "github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCreateInitialCrushMap(t *testing.T) {
	clientset := testop.New(3)
	executor := &exectest.MockExecutor{}
	context := &clusterd.Context{Clientset: clientset, Executor: executor}
	c := newCluster(&rookalpha.Cluster{}, context)
	c.Namespace = "rook294"

	// create the initial crush map and verify that a configmap value was created that says the crush map was created
	err := c.createInitialCrushMap()
	assert.Nil(t, err)
	cm, err := clientset.CoreV1().ConfigMaps(c.Namespace).Get(crushConfigMapName, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, cm)
	assert.Equal(t, "1", cm.Data[crushmapCreatedKey])

	// the crushmap should NOT get created again
	executor.MockExecuteCommandWithOutputFile = func(debug bool, actionName string, command string, outFileArg string, args ...string) (string, error) {
		return "", fmt.Errorf("crushmap was already created, we shouldn't be calling this again")
	}
	err = c.createInitialCrushMap()
	assert.Nil(t, err)
}

func TestClusterChanged(t *testing.T) {
	old := rookalpha.ClusterSpec{MonCount: 1, HostNetwork: false}
	new := rookalpha.ClusterSpec{MonCount: 3, HostNetwork: true}

	// no changes supported yet
	assert.False(t, clusterChanged(old, new))
}

func TestClusterDelete(t *testing.T) {
	nodeName := "node841"
	clusterName := "cluster684"
	pvName := "pvc-540"
	rookSystemNamespace := "rook-system-6413"

	os.Setenv(k8sutil.PodNamespaceEnvVar, rookSystemNamespace)
	defer os.Unsetenv(k8sutil.PodNamespaceEnvVar)

	clientset := testop.New(3)
	context := &clusterd.Context{
		Clientset: clientset,
	}

	listCount := 0
	volumeAttachmentController := &attachment.MockAttachment{
		MockList: func(namespace string) (*rookalpha.VolumeAttachmentList, error) {
			listCount++
			if listCount == 1 {
				// first listing returns an existing volume attachment, so the controller should wait
				return &rookalpha.VolumeAttachmentList{
					Items: []rookalpha.VolumeAttachment{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      pvName,
								Namespace: rookSystemNamespace,
							},
							Attachments: []rookalpha.Attachment{
								{
									Node:        nodeName,
									ClusterName: clusterName,
								},
							},
						},
					},
				}, nil
			} else {
				// subsequent listings should return no volume attachments, meaning that they have all
				// been cleaned up and the controller can move on.
				return &rookalpha.VolumeAttachmentList{Items: []rookalpha.VolumeAttachment{}}, nil
			}
		},
	}

	// create the cluster controller and tell it that the cluster has been deleted
	controller := NewClusterController(context, "", volumeAttachmentController)
	clusterToDelete := &rookalpha.Cluster{ObjectMeta: metav1.ObjectMeta{Namespace: clusterName}}
	controller.handleDelete(clusterToDelete, time.Microsecond)

	// listing of volume attachments should have been called twice.  the first time there were volume attachments
	// that the controller needed to wait on to be cleaned up and the second time they were all cleaned up.
	assert.Equal(t, 2, listCount)
}
