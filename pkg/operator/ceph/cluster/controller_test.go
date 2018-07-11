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
package cluster

import (
	"fmt"
	"os"
	"testing"
	"time"

	cephv1beta1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1beta1"
	rookv1alpha1 "github.com/rook/rook/pkg/apis/rook.io/v1alpha1"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	rookfake "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/agent/flexvolume/attachment"
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
	c := newCluster(&cephv1beta1.Cluster{}, context)
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
		MockList: func(namespace string) (*rookalpha.VolumeList, error) {
			listCount++
			if listCount == 1 {
				// first listing returns an existing volume attachment, so the controller should wait
				return &rookalpha.VolumeList{
					Items: []rookalpha.Volume{
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
			}

			// subsequent listings should return no volume attachments, meaning that they have all
			// been cleaned up and the controller can move on.
			return &rookalpha.VolumeList{Items: []rookalpha.Volume{}}, nil

		},
	}

	// create the cluster controller and tell it that the cluster has been deleted
	controller := NewClusterController(context, "", volumeAttachmentController)
	clusterToDelete := &cephv1beta1.Cluster{ObjectMeta: metav1.ObjectMeta{Namespace: clusterName}}
	controller.handleDelete(clusterToDelete, time.Microsecond)

	// listing of volume attachments should have been called twice.  the first time there were volume attachments
	// that the controller needed to wait on to be cleaned up and the second time they were all cleaned up.
	assert.Equal(t, 2, listCount)
}

func TestClusterChanged(t *testing.T) {
	// a new node added, should be a change
	old := cephv1beta1.ClusterSpec{
		Storage: rookalpha.StorageScopeSpec{
			Nodes: []rookalpha.Node{
				{Name: "node1", Selection: rookalpha.Selection{Devices: []rookalpha.Device{{Name: "sda"}}}},
			},
		},
	}
	new := cephv1beta1.ClusterSpec{
		Storage: rookalpha.StorageScopeSpec{
			Nodes: []rookalpha.Node{
				{Name: "node1", Selection: rookalpha.Selection{Devices: []rookalpha.Device{{Name: "sda"}}}},
				{Name: "node2", Selection: rookalpha.Selection{Devices: []rookalpha.Device{{Name: "sda"}}}},
			},
		},
	}
	assert.True(t, clusterChanged(old, new))

	// a node was removed, should be a change
	old.Storage.Nodes = []rookalpha.Node{
		{Name: "node1", Selection: rookalpha.Selection{Devices: []rookalpha.Device{{Name: "sda"}}}},
		{Name: "node2", Selection: rookalpha.Selection{Devices: []rookalpha.Device{{Name: "sda"}}}},
	}
	new.Storage.Nodes = []rookalpha.Node{
		{Name: "node1", Selection: rookalpha.Selection{Devices: []rookalpha.Device{{Name: "sda"}}}},
	}
	assert.True(t, clusterChanged(old, new))

	// the nodes being in a different order should not be a change
	old.Storage.Nodes = []rookalpha.Node{
		{Name: "node1", Selection: rookalpha.Selection{Devices: []rookalpha.Device{{Name: "sda"}}}},
		{Name: "node2", Selection: rookalpha.Selection{Devices: []rookalpha.Device{{Name: "sda"}}}},
	}
	new.Storage.Nodes = []rookalpha.Node{
		{Name: "node2", Selection: rookalpha.Selection{Devices: []rookalpha.Device{{Name: "sda"}}}},
		{Name: "node1", Selection: rookalpha.Selection{Devices: []rookalpha.Device{{Name: "sda"}}}},
	}
	assert.False(t, clusterChanged(old, new))
}

func TestRemoveFinalizer(t *testing.T) {
	clientset := testop.New(3)
	context := &clusterd.Context{
		Clientset:     clientset,
		RookClientset: rookfake.NewSimpleClientset(),
	}
	controller := NewClusterController(context, "", &attachment.MockAttachment{})

	// *****************************************
	// start with a current version ceph cluster
	// *****************************************
	cluster := &cephv1beta1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "cluster-1893",
			Namespace:  "namespace-6551",
			Finalizers: []string{finalizerName},
		},
	}

	// create the cluster initially so it exists in the k8s api
	cluster, err := context.RookClientset.CephV1beta1().Clusters(cluster.Namespace).Create(cluster)
	assert.NoError(t, err)
	assert.Len(t, cluster.Finalizers, 1)

	// remove the finalizer from the cluster object
	controller.removeFinalizer(cluster)

	// verify the finalier was removed
	cluster, err = context.RookClientset.CephV1beta1().Clusters(cluster.Namespace).Get(cluster.Name, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, cluster)
	assert.Len(t, cluster.Finalizers, 0)

	// ****************************************
	// now try with with a legacy rook cluster
	// ****************************************
	legacyRookCluster := &rookv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "cluster-6842",
			Namespace:  "namespace-9523",
			Finalizers: []string{finalizerNameRookLegacy},
		},
	}

	// create the cluster initially so it exists in the k8s api
	legacyRookCluster, err = context.RookClientset.RookV1alpha1().Clusters(legacyRookCluster.Namespace).Create(legacyRookCluster)
	assert.NoError(t, err)
	assert.Len(t, legacyRookCluster.Finalizers, 1)

	// remove the finalizer from the cluster object
	controller.removeFinalizer(legacyRookCluster)

	// verify the finalier was removed
	legacyRookCluster, err = context.RookClientset.RookV1alpha1().Clusters(legacyRookCluster.Namespace).Get(legacyRookCluster.Name, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, legacyRookCluster)
	assert.Len(t, legacyRookCluster.Finalizers, 0)
}
