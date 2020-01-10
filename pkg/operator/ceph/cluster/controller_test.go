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
	"os"
	"testing"
	"time"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	rookfake "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/agent/flexvolume/attachment"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/operator/k8sutil"
	testop "github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestClusterDeleteFlexEnabled(t *testing.T) {
	nodeName := "node841"
	clusterName := "cluster684"
	pvName := "pvc-540"
	rookSystemNamespace := "rook-system-6413"

	os.Setenv("ROOK_ENABLE_FLEX_DRIVER", "true")
	os.Setenv(k8sutil.PodNamespaceEnvVar, rookSystemNamespace)
	defer os.Unsetenv("ROOK_ENABLE_FLEX_DRIVER")
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
	addCallbacks := []func(clusterSpec *cephv1.ClusterSpec) error{
		func(clusterSpec *cephv1.ClusterSpec) error {
			logger.Infof("test success callback")
			return nil
		},
	}
	removeCallbacks := []func() error{
		func() error {
			logger.Infof("test success callback")
			return nil
		},
	}

	// create the cluster controller and tell it that the cluster has been deleted
	controller := NewClusterController(context, "", volumeAttachmentController, addCallbacks, removeCallbacks)
	clusterToDelete := &cephv1.CephCluster{ObjectMeta: metav1.ObjectMeta{Namespace: clusterName}}
	controller.handleDelete(clusterToDelete, time.Microsecond)

	// listing of volume attachments should have been called twice.  the first time there were volume attachments
	// that the controller needed to wait on to be cleaned up and the second time they were all cleaned up.
	assert.Equal(t, 2, listCount)
}

func TestClusterDeleteFlexDisabled(t *testing.T) {
	clusterName := "cluster684"
	rookSystemNamespace := "rook-system-6413"

	os.Setenv("ROOK_ENABLE_FLEX_DRIVER", "false")
	os.Setenv(k8sutil.PodNamespaceEnvVar, rookSystemNamespace)
	defer os.Unsetenv("ROOK_ENABLE_FLEX_DRIVER")
	defer os.Unsetenv(k8sutil.PodNamespaceEnvVar)

	clientset := testop.New(3)
	context := &clusterd.Context{
		Clientset: clientset,
	}

	listCount := 0
	volumeAttachmentController := &attachment.MockAttachment{
		MockList: func(namespace string) (*rookalpha.VolumeList, error) {
			listCount++
			return &rookalpha.VolumeList{Items: []rookalpha.Volume{}}, nil

		},
	}
	addCallbacks := []func(clusterSpec *cephv1.ClusterSpec) error{
		func(clusterSpec *cephv1.ClusterSpec) error {
			logger.Infof("test success callback")
			os.Setenv("ROOK_ENABLE_FLEX_DRIVER", "true")
			os.Setenv(k8sutil.PodNamespaceEnvVar, rookSystemNamespace)
			defer os.Unsetenv("ROOK_ENABLE_FLEX_DRIVER")
			return nil
		},
	}
	removeCallbacks := []func() error{
		func() error {
			logger.Infof("test success callback")
			return nil
		},
	}

	// create the cluster controller and tell it that the cluster has been deleted
	controller := NewClusterController(context, "", volumeAttachmentController, addCallbacks, removeCallbacks)
	clusterToDelete := &cephv1.CephCluster{ObjectMeta: metav1.ObjectMeta{Namespace: clusterName}}
	controller.handleDelete(clusterToDelete, time.Microsecond)

	// Ensure that the listing of volume attachments was never called.
	assert.Equal(t, 0, listCount)
}

func TestClusterChanged(t *testing.T) {
	// a new node added, should be a change
	old := cephv1.ClusterSpec{
		Storage: rookalpha.StorageScopeSpec{
			Nodes: []rookalpha.Node{
				{Name: "node1", Selection: rookalpha.Selection{Devices: []rookalpha.Device{{Name: "sda"}}}},
			},
		},
	}
	new := cephv1.ClusterSpec{
		Storage: rookalpha.StorageScopeSpec{
			Nodes: []rookalpha.Node{
				{Name: "node1", Selection: rookalpha.Selection{Devices: []rookalpha.Device{{Name: "sda"}}}},
				{Name: "node2", Selection: rookalpha.Selection{Devices: []rookalpha.Device{{Name: "sda"}}}},
			},
		},
	}
	c := &cluster{Spec: &cephv1.ClusterSpec{}, mons: &mon.Cluster{}}
	changed, diff := clusterChanged(old, new, c)
	assert.True(t, changed)
	assert.NotEqual(t, diff, "")
	assert.Equal(t, 0, c.Spec.Mon.Count)

	// a node was removed, should be a change
	old.Storage.Nodes = []rookalpha.Node{
		{Name: "node1", Selection: rookalpha.Selection{Devices: []rookalpha.Device{{Name: "sda"}}}},
		{Name: "node2", Selection: rookalpha.Selection{Devices: []rookalpha.Device{{Name: "sda"}}}},
	}
	new.Storage.Nodes = []rookalpha.Node{
		{Name: "node1", Selection: rookalpha.Selection{Devices: []rookalpha.Device{{Name: "sda"}}}},
	}
	changed, diff = clusterChanged(old, new, c)
	assert.True(t, changed)
	assert.NotEqual(t, diff, "")

	// the nodes being in a different order should not be a change
	old.Storage.Nodes = []rookalpha.Node{
		{Name: "node1", Selection: rookalpha.Selection{Devices: []rookalpha.Device{{Name: "sda"}}}},
		{Name: "node2", Selection: rookalpha.Selection{Devices: []rookalpha.Device{{Name: "sda"}}}},
	}
	new.Storage.Nodes = []rookalpha.Node{
		{Name: "node2", Selection: rookalpha.Selection{Devices: []rookalpha.Device{{Name: "sda"}}}},
		{Name: "node1", Selection: rookalpha.Selection{Devices: []rookalpha.Device{{Name: "sda"}}}},
	}
	changed, diff = clusterChanged(old, new, c)
	assert.False(t, changed)
	assert.Equal(t, 0, c.Spec.Mon.Count)
	assert.Equal(t, "", diff)

	// If the number of mons changes, the cluster would be updated
	new.Mon.Count = 3
	new.Mon.AllowMultiplePerNode = true
	changed, diff = clusterChanged(old, new, c)
	assert.True(t, changed)
	assert.NotEqual(t, diff, "")
}

func TestRemoveFinalizer(t *testing.T) {
	clientset := testop.New(3)
	context := &clusterd.Context{
		Clientset:     clientset,
		RookClientset: rookfake.NewSimpleClientset(),
	}
	addCallbacks := []func(clusterSpec *cephv1.ClusterSpec) error{
		func(clusterSpec *cephv1.ClusterSpec) error {
			logger.Infof("test success callback")
			return errors.New("test failed callback")
		},
	}
	removeCallbacks := []func() error{
		func() error {
			logger.Infof("test success callback")
			return nil
		},
	}

	controller := NewClusterController(context, "", &attachment.MockAttachment{}, addCallbacks, removeCallbacks)

	// *****************************************
	// start with a current version ceph cluster
	// *****************************************
	cluster := &cephv1.CephCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "cluster-1893",
			Namespace:  "namespace-6551",
			Finalizers: []string{finalizerName},
		},
	}

	// create the cluster initially so it exists in the k8s api
	cluster, err := context.RookClientset.CephV1().CephClusters(cluster.Namespace).Create(cluster)
	assert.NoError(t, err)
	assert.Len(t, cluster.Finalizers, 1)

	// remove the finalizer from the cluster object
	controller.removeFinalizer(cluster)

	// verify the finalizer was removed
	cluster, err = context.RookClientset.CephV1().CephClusters(cluster.Namespace).Get(cluster.Name, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, cluster)
	assert.Len(t, cluster.Finalizers, 0)
}

func TestValidateExternalClusterSpec(t *testing.T) {
	c := &cluster{Spec: &cephv1.ClusterSpec{}, mons: &mon.Cluster{}}
	err := validateExternalClusterSpec(c)
	assert.Error(t, err)

	c.Spec.DataDirHostPath = "path"
	err = validateExternalClusterSpec(c)
	assert.NoError(t, err, err)

	c.Spec.CephVersion.Image = "ceph/ceph:v14.2.6"
	err = validateExternalClusterSpec(c)
	assert.NoError(t, err)
}
