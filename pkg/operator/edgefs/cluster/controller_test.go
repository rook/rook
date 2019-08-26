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
package cluster

import (
	"os"
	"testing"
	"time"

	edgefsv1 "github.com/rook/rook/pkg/apis/edgefs.rook.io/v1"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	rookfake "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	testop "github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestClusterDelete(t *testing.T) {
	//nodeName := "node841"
	clusterName := "cluster684"
	//pvName := "pvc-540"
	rookSystemNamespace := "rook-system-6413"

	os.Setenv(k8sutil.PodNamespaceEnvVar, rookSystemNamespace)
	defer os.Unsetenv(k8sutil.PodNamespaceEnvVar)

	clientset := testop.New(3)
	context := &clusterd.Context{
		Clientset: clientset,
	}

	// create the cluster controller and tell it that the cluster has been deleted
	controller := NewClusterController(context, "")
	clusterToDelete := &edgefsv1.Cluster{ObjectMeta: metav1.ObjectMeta{Namespace: clusterName}}
	controller.handleDelete(clusterToDelete, time.Microsecond)
}

func TestClusterChanged(t *testing.T) {
	// a new node added, should be a change
	old := edgefsv1.ClusterSpec{
		Storage: rookalpha.StorageScopeSpec{
			Nodes: []rookalpha.Node{
				{Name: "node1", Selection: rookalpha.Selection{Devices: []rookalpha.Device{{Name: "sda"}}}},
			},
		},
	}
	new := edgefsv1.ClusterSpec{
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
	controller := NewClusterController(context, "")

	// *****************************************
	// start with a current version edgefs cluster
	// *****************************************
	cluster := &edgefsv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "cluster-1893",
			Namespace:  "namespace-6551",
			Finalizers: []string{finalizerName},
		},
	}

	// create the cluster initially so it exists in the k8s api
	cluster, err := context.RookClientset.EdgefsV1().Clusters(cluster.Namespace).Create(cluster)
	assert.NoError(t, err)
	assert.Len(t, cluster.Finalizers, 1)

	// remove the finalizer from the cluster object
	controller.removeFinalizer(cluster)

	// verify the finalier was removed
	cluster, err = context.RookClientset.EdgefsV1().Clusters(cluster.Namespace).Get(cluster.Name, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, cluster)
	assert.Len(t, cluster.Finalizers, 0)
}
