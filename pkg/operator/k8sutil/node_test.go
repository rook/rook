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
	optest "github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	scheduler "k8s.io/kubernetes/pkg/scheduler/api"
)

func createNode(nodeName string, condition v1.NodeConditionType, clientset *fake.Clientset) error {
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: nodeName,
		},
		Status: v1.NodeStatus{
			Conditions: []v1.NodeCondition{
				{
					Type: condition, Status: v1.ConditionTrue,
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

func testNode(taints []v1.Taint) v1.Node {
	n := v1.Node{}
	for _, t := range taints {
		n.Spec.Taints = append(n.Spec.Taints, t)
	}
	return n
}

func taintReservedForRook() v1.Taint {
	return v1.Taint{Key: "reservedForRook", Effect: v1.TaintEffectNoSchedule}
}

func taintReservedForOther() v1.Taint {
	return v1.Taint{Key: "reservedForNOTRook", Effect: v1.TaintEffectNoSchedule}
}

func taintCordoned() v1.Taint {
	return v1.Taint{Key: scheduler.TaintNodeUnschedulable, Effect: v1.TaintEffectNoSchedule}
}

func taintAllWellKnown() []v1.Taint {
	taints := []v1.Taint{}
	for _, t := range WellKnownTaints {
		taints = append(taints, v1.Taint{
			// assume the "worst" with NoExecute
			Key: t, Effect: v1.TaintEffectNoExecute,
		})
	}
	return taints
}

func taints(taints ...v1.Taint) []v1.Taint {
	list := []v1.Taint{}
	for _, t := range taints {
		list = append(taints, t)
	}
	return list
}

func tolerateRook() []v1.Toleration {
	return []v1.Toleration{{Key: "reservedForRook"}}
}

func TestNodeIsTolerable(t *testing.T) {
	type args struct {
		node                  v1.Node
		tolerations           []v1.Toleration
		ignoreWellKnownTaints bool
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{name: "tolerate node w/o taints", args: args{
			node:                  v1.Node{},
			tolerations:           tolerateRook(),
			ignoreWellKnownTaints: false,
		}, want: true},
		{name: "tolerate node w/ rook taint", args: args{
			node:                  testNode(taints(taintReservedForRook())),
			tolerations:           tolerateRook(),
			ignoreWellKnownTaints: false,
		}, want: true},
		{name: "do not tolerate rook taint", args: args{
			node:                  testNode(taints(taintReservedForRook())),
			tolerations:           nil,
			ignoreWellKnownTaints: false,
		}, want: false},
		{name: "do not tolerate other taint", args: args{
			node:                  testNode(taints(taintReservedForRook(), taintReservedForOther())),
			tolerations:           tolerateRook(),
			ignoreWellKnownTaints: false,
		}, want: false},
		{name: "do not tolerate node w/ known taints", args: args{
			node:                  testNode(taintAllWellKnown()),
			tolerations:           nil,
			ignoreWellKnownTaints: false,
		}, want: false},
		{name: "do not tolerate node w/ known taints 2", args: args{
			node:                  testNode(taintAllWellKnown()),
			tolerations:           tolerateRook(),
			ignoreWellKnownTaints: false,
		}, want: false},
		{name: "tolerate node w/ known taints and rook taint", args: args{
			node:                  testNode(taintAllWellKnown()),
			tolerations:           tolerateRook(),
			ignoreWellKnownTaints: true,
		}, want: true},
		{name: "do not tolerate node w/ known taints and rook taint", args: args{
			node:                  testNode(append(taintAllWellKnown(), taintReservedForRook())),
			tolerations:           nil,
			ignoreWellKnownTaints: true,
		}, want: false},
		{name: "tolerate node w/ known taints and rook taint", args: args{
			node:                  testNode(append(taintAllWellKnown(), taintReservedForRook())),
			tolerations:           tolerateRook(),
			ignoreWellKnownTaints: true,
		}, want: true},
		{name: "do not tolerate node w/ known and other taints", args: args{
			node:                  testNode(append(taintAllWellKnown(), taintReservedForRook(), taintReservedForOther())),
			tolerations:           tolerateRook(),
			ignoreWellKnownTaints: true,
		}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NodeIsTolerable(tt.args.node, tt.args.tolerations, tt.args.ignoreWellKnownTaints); got != tt.want {
				t.Errorf("NodeIsTolerable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNodeIsReady(t *testing.T) {
	assert.True(t, NodeIsReady(v1.Node{Status: v1.NodeStatus{Conditions: []v1.NodeCondition{
		{Type: v1.NodeReady, Status: v1.ConditionTrue},
	}}}))
	assert.False(t, NodeIsReady(v1.Node{Status: v1.NodeStatus{Conditions: []v1.NodeCondition{
		{Type: v1.NodeReady, Status: v1.ConditionFalse},
	}}}))
	assert.False(t, NodeIsReady(v1.Node{Status: v1.NodeStatus{Conditions: []v1.NodeCondition{
		{Type: v1.NodeReady, Status: v1.ConditionUnknown},
	}}}))
	// if `Ready` condition does not exist, must assume that node is not ready
	assert.False(t, NodeIsReady(v1.Node{Status: v1.NodeStatus{Conditions: []v1.NodeCondition{
		{Type: v1.NodeOutOfDisk, Status: v1.ConditionTrue},
	}}}))
	// if `Ready` condition is not accompanied by a status, must assume that node is not ready
	assert.False(t, NodeIsReady(v1.Node{Status: v1.NodeStatus{Conditions: []v1.NodeCondition{
		{Type: v1.NodeOutOfDisk},
	}}}))
}

func TestGetRookNodesMatchingKubernetesNodes(t *testing.T) {
	clientset := optest.New(3) // create nodes 0, 1, and 2
	rookNodes := []rookalpha.Node{}

	getNode := func(name string) v1.Node {
		n, err := clientset.CoreV1().Nodes().Get(name, metav1.GetOptions{})
		assert.NoError(t, err)
		return *n
	}

	// no rook nodes specified
	nodes, err := GetKubernetesNodesMatchingRookNodes(rookNodes, clientset)
	assert.NoError(t, err)
	assert.Empty(t, nodes)

	// more rook nodes specified than nodes exist
	rookNodes = []rookalpha.Node{
		{Name: "node0"},
		{Name: "node2"},
		{Name: "node5"}}
	nodes, err = GetKubernetesNodesMatchingRookNodes(rookNodes, clientset)
	assert.NoError(t, err)
	assert.Len(t, nodes, 2)
	assert.Contains(t, nodes, getNode("node0"))
	assert.Contains(t, nodes, getNode("node2"))

	// rook nodes match k8s nodes
	rookNodes = []rookalpha.Node{
		{Name: "node0"},
		{Name: "node1"},
		{Name: "node2"}}
	nodes, err = GetKubernetesNodesMatchingRookNodes(rookNodes, clientset)
	assert.NoError(t, err)
	assert.Len(t, nodes, 3)
	assert.Contains(t, nodes, getNode("node0"))
	assert.Contains(t, nodes, getNode("node1"))
	assert.Contains(t, nodes, getNode("node2"))

	// no k8s nodes exist
	clientset = optest.New(0)
	nodes, err = GetKubernetesNodesMatchingRookNodes(rookNodes, clientset)
	assert.NoError(t, err)
	assert.Len(t, nodes, 0)
}

func TestRookNodesMatchingKubernetesNodes(t *testing.T) {
	clientset := optest.New(3) // create nodes 0, 1, and 2

	getNode := func(name string) v1.Node {
		n, err := clientset.CoreV1().Nodes().Get(name, metav1.GetOptions{})
		assert.NoError(t, err)
		return *n
	}
	n0 := getNode("node0")
	n0.Labels = map[string]string{v1.LabelHostname: "node0-hostname"}
	n1 := getNode("node1")
	n2 := getNode("node2")
	n2.Labels = map[string]string{v1.LabelHostname: "node2"}
	k8sNodes := []v1.Node{n0, n1, n2}

	// no rook nodes specified for input
	rookNodes := []rookalpha.Node{}
	retNodes := RookNodesMatchingKubernetesNodes(rookNodes, k8sNodes)
	assert.Len(t, retNodes, 0)

	// all rook nodes specified
	rookNodes = []rookalpha.Node{
		{Name: "node0"},
		{Name: "node1"},
		{Name: "node2"}}
	retNodes = RookNodesMatchingKubernetesNodes(rookNodes, k8sNodes)
	assert.Len(t, retNodes, 3)
	// this should return nodes named by hostname if that is available
	assert.Contains(t, retNodes, rookalpha.Node{Name: "node0-hostname"})
	assert.Contains(t, retNodes, rookalpha.Node{Name: "node1"})
	assert.Contains(t, retNodes, rookalpha.Node{Name: "node2"})

	// more rook nodes specified than exist
	rookNodes = []rookalpha.Node{
		{Name: "node0-hostname"},
		{Name: "node2"},
		{Name: "node5"}}
	retNodes = RookNodesMatchingKubernetesNodes(rookNodes, k8sNodes)
	assert.Len(t, retNodes, 2)
	assert.Contains(t, retNodes, rookalpha.Node{Name: "node0-hostname"})
	assert.Contains(t, retNodes, rookalpha.Node{Name: "node2"})

	// no k8s nodes specified
	retNodes = RookNodesMatchingKubernetesNodes(rookNodes, []v1.Node{})
	assert.Len(t, retNodes, 0)
}

func TestNodeIsInRookList(t *testing.T) {
	rookNodes := []rookalpha.Node{}

	assert.False(t, NodeIsInRookNodeList("node0", rookNodes))

	rookNodes = []rookalpha.Node{
		{Name: "node0-hostname"},
		{Name: "node2"},
		{Name: "node5"}}
	assert.False(t, NodeIsInRookNodeList("node0", rookNodes))
	assert.True(t, NodeIsInRookNodeList("node0-hostname", rookNodes))
}
