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
package mon

import (
	"testing"

	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAvailableMonNodes(t *testing.T) {
	clientset := test.New(1)
	c := New(&clusterd.Context{Clientset: clientset}, "ns", "", "myversion", 3, rookalpha.Placement{}, false, v1.ResourceRequirements{}, metav1.OwnerReference{})
	c.clusterInfo = test.CreateConfigDir(0)
	nodes, err := c.getAvailableMonNodes()
	assert.Nil(t, err)
	assert.Equal(t, 1, len(nodes))

	conditions := v1.NodeCondition{Type: v1.NodeOutOfDisk}
	nodes[0].Status = v1.NodeStatus{Conditions: []v1.NodeCondition{conditions}}
	clientset.CoreV1().Nodes().Update(&nodes[0])

	emptyNodes, err := c.getAvailableMonNodes()
	assert.NotNil(t, err)
	assert.Nil(t, emptyNodes)
}
