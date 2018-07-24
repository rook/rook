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

// Package k8sutil for Kubernetes helpers.
package k8sutil

import (
	"testing"

	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func createNode(nodeName string, condition v1.NodeConditionType, clientset *fake.Clientset) error {
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: nodeName,
		},
		Status: v1.NodeStatus{
			Conditions: []v1.NodeCondition{
				{
					Type: condition,
				},
			},
		},
	}
	_, err := clientset.CoreV1().Nodes().Create(node)
	return err
}

func TestValidNode(t *testing.T) {
	nodeA := "nodeA"
	nodeB := "nodeB"

	nodes := []rookalpha.Node{
		{
			Name: nodeA,
		},
		{
			Name: nodeB,
		},
	}
	var placement rookalpha.Placement
	// set up a fake k8s client set and watcher to generate events that the operator will listen to
	clientset := fake.NewSimpleClientset()

	nodeErr := createNode(nodeA, v1.NodeReady, clientset)
	assert.Nil(t, nodeErr)
	nodeErr = createNode(nodeB, v1.NodeNetworkUnavailable, clientset)
	assert.Nil(t, nodeErr)
	validNodes := GetValidNodes(nodes, clientset, placement)
	assert.Equal(t, len(validNodes), 1)
}
