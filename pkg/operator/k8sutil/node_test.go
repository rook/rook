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
	"context"
	"reflect"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	optest "github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func createNode(nodeName string, condition v1.NodeConditionType, clientset *fake.Clientset) error {
	ctx := context.TODO()
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   nodeName,
			Labels: map[string]string{"testLabel": nodeName},
		},
		Status: v1.NodeStatus{
			Conditions: []v1.NodeCondition{
				{
					Type: condition, Status: v1.ConditionTrue,
				},
			},
		},
	}
	_, err := clientset.CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
	return err
}

func TestValidNode(t *testing.T) {
	storage := cephv1.StorageScopeSpec{
		Nodes: []cephv1.Node{
			{
				Name: "nodeA",
			},
			{
				Name: "nodeB",
			},
			{
				Name: "nodeC",
			},
		},
	}
	// set up a fake k8s client set and watcher to generate events that the operator will listen to
	clientset := fake.NewClientset()

	nodeErr := createNode("nodeA", v1.NodeReady, clientset)
	assert.Nil(t, nodeErr)
	nodeErr = createNode("nodeB", v1.NodeNetworkUnavailable, clientset)
	assert.Nil(t, nodeErr)

	t.Run("test valid node", func(t *testing.T) {
		var placement cephv1.Placement
		validNodes := GetValidNodes(context.TODO(), storage, clientset, placement)
		assert.Equal(t, 1, len(validNodes))
		assert.Equal(t, "nodeA", validNodes[0].Name)
	})

	t.Run("test nodes always valid", func(t *testing.T) {
		var placement cephv1.Placement
		storage.ScheduleAlways = true
		validNodes := GetValidNodes(context.TODO(), storage, clientset, placement)
		require.Equal(t, 2, len(validNodes))
		assert.Equal(t, "nodeA", validNodes[0].Name)
		assert.Equal(t, "nodeB", validNodes[1].Name)
	})

	t.Run("test placement", func(t *testing.T) {
		nodeErr = createNode("nodeC", v1.NodeReady, clientset)
		assert.Nil(t, nodeErr)

		placement := cephv1.Placement{
			NodeAffinity: &v1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
					NodeSelectorTerms: []v1.NodeSelectorTerm{
						{
							MatchExpressions: []v1.NodeSelectorRequirement{
								{
									Key:      "testLabel",
									Operator: v1.NodeSelectorOpIn,
									Values:   []string{"nodeC"},
								},
							},
						},
					},
				},
			},
		}
		validNodes := GetValidNodes(context.TODO(), storage, clientset, placement)
		assert.Equal(t, len(validNodes), 1)
		assert.Equal(t, "nodeC", validNodes[0].Name)
	})
}

func testNode(taints []v1.Taint) v1.Node {
	n := v1.Node{}
	n.Spec.Taints = append(n.Spec.Taints, taints...)
	return n
}

func taintReservedForRook() v1.Taint {
	return v1.Taint{Key: "reservedForRook", Effect: v1.TaintEffectNoSchedule}
}

func taintReservedForOther() v1.Taint {
	return v1.Taint{Key: "reservedForNOTRook", Effect: v1.TaintEffectNoSchedule}
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
		{Type: v1.NodeDiskPressure, Status: v1.ConditionTrue},
	}}}))
	// if `Ready` condition is not accompanied by a status, must assume that node is not ready
	assert.False(t, NodeIsReady(v1.Node{Status: v1.NodeStatus{Conditions: []v1.NodeCondition{
		{Type: v1.NodeDiskPressure},
	}}}))
}

func TestGetRookNodesMatchingKubernetesNodes(t *testing.T) {
	ctx := context.TODO()
	clientset := optest.New(t, 3) // create nodes 0, 1, and 2
	rookNodes := []cephv1.Node{}

	getNode := func(name string) v1.Node {
		n, err := clientset.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
		assert.NoError(t, err)
		return *n
	}

	// no rook nodes specified
	nodes, err := GetKubernetesNodesMatchingRookNodes(ctx, rookNodes, clientset)
	assert.NoError(t, err)
	assert.Empty(t, nodes)

	// more rook nodes specified than nodes exist
	rookNodes = []cephv1.Node{
		{Name: "node0"},
		{Name: "node2"},
		{Name: "node5"},
	}
	nodes, err = GetKubernetesNodesMatchingRookNodes(ctx, rookNodes, clientset)
	assert.NoError(t, err)
	assert.Len(t, nodes, 2)
	assert.Contains(t, nodes, getNode("node0"))
	assert.Contains(t, nodes, getNode("node2"))

	// rook nodes match k8s nodes
	rookNodes = []cephv1.Node{
		{Name: "node0"},
		{Name: "node1"},
		{Name: "node2"},
	}
	nodes, err = GetKubernetesNodesMatchingRookNodes(ctx, rookNodes, clientset)
	assert.NoError(t, err)
	assert.Len(t, nodes, 3)
	assert.Contains(t, nodes, getNode("node0"))
	assert.Contains(t, nodes, getNode("node1"))
	assert.Contains(t, nodes, getNode("node2"))

	// no k8s nodes exist
	clientset = optest.New(t, 0)
	nodes, err = GetKubernetesNodesMatchingRookNodes(ctx, rookNodes, clientset)
	assert.NoError(t, err)
	assert.Len(t, nodes, 0)
}

func TestRookNodesMatchingKubernetesNodes(t *testing.T) {
	ctx := context.TODO()
	clientset := optest.New(t, 3) // create nodes 0, 1, and 2

	getNode := func(name string) v1.Node {
		n, err := clientset.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
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
	rookStorage := cephv1.StorageScopeSpec{
		Nodes: []cephv1.Node{},
	}
	retNodes := RookNodesMatchingKubernetesNodes(rookStorage, k8sNodes)
	assert.Len(t, retNodes, 0)

	// all rook nodes specified
	rookStorage.Nodes = []cephv1.Node{
		{Name: "node0"},
		{Name: "node1"},
		{Name: "node2"},
	}
	retNodes = RookNodesMatchingKubernetesNodes(rookStorage, k8sNodes)
	assert.Len(t, retNodes, 3)
	// this should return nodes named by hostname if that is available
	assert.Contains(t, retNodes, cephv1.Node{Name: "node0-hostname"})
	assert.Contains(t, retNodes, cephv1.Node{Name: "node1"})
	assert.Contains(t, retNodes, cephv1.Node{Name: "node2"})

	// more rook nodes specified than exist
	rookStorage.Nodes = []cephv1.Node{
		{Name: "node0-hostname"},
		{Name: "node2"},
		{Name: "node5"},
	}
	retNodes = RookNodesMatchingKubernetesNodes(rookStorage, k8sNodes)
	assert.Len(t, retNodes, 2)
	assert.Contains(t, retNodes, cephv1.Node{Name: "node0-hostname"})
	assert.Contains(t, retNodes, cephv1.Node{Name: "node2"})

	// no k8s nodes specified
	retNodes = RookNodesMatchingKubernetesNodes(rookStorage, []v1.Node{})
	assert.Len(t, retNodes, 0)

	// custom node hostname label
	t.Setenv("ROOK_CUSTOM_HOSTNAME_LABEL", "my_custom_hostname_label")
	n0.Labels["my_custom_hostname_label"] = "node0-custom-hostname"
	k8sNodes[0] = n0

	rookStorage.Nodes = []cephv1.Node{
		{Name: "node0"},
		{Name: "node1"},
		{Name: "node2"},
	}
	retNodes = RookNodesMatchingKubernetesNodes(rookStorage, k8sNodes)
	assert.Len(t, retNodes, 3)
	// this should return nodes named by hostname if that is available
	assert.Contains(t, retNodes, cephv1.Node{Name: "node0-custom-hostname"})
	assert.Contains(t, retNodes, cephv1.Node{Name: "node1"})
	assert.Contains(t, retNodes, cephv1.Node{Name: "node2"})
}

func TestGenerateNodeAffinity(t *testing.T) {
	type args struct {
		nodeAffinity string
	}
	tests := []struct {
		name    string
		args    args
		want    *v1.NodeAffinity
		wantErr bool
	}{
		{
			name: "GenerateNodeAffinity",
			args: args{
				nodeAffinity: "rook.io/ceph=true",
			},
			want: &v1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
					NodeSelectorTerms: []v1.NodeSelectorTerm{
						{
							MatchExpressions: []v1.NodeSelectorRequirement{
								{
									Key:      "rook.io/ceph",
									Operator: v1.NodeSelectorOpIn,
									Values:   []string{"true"},
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "FailGenerateNodeAffinity",
			args: args{
				nodeAffinity: "rook.io/ceph,cassandra=true",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "GenerateNodeAffinityWithKeyOnly",
			args: args{
				nodeAffinity: "rook.io/ceph",
			},
			want: &v1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
					NodeSelectorTerms: []v1.NodeSelectorTerm{
						{
							MatchExpressions: []v1.NodeSelectorRequirement{
								{
									Key:      "rook.io/ceph",
									Operator: v1.NodeSelectorOpExists,
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "GenerateNodeAffinityWithJSONInputUsingDoesNotExistOperator",
			args: args{
				nodeAffinity: `{"requiredDuringSchedulingIgnoredDuringExecution":{"nodeSelectorTerms":[{"matchExpressions":[{"key":"myKey","operator":"DoesNotExist"}]}]}}`,
			},
			want: &v1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
					NodeSelectorTerms: []v1.NodeSelectorTerm{
						{
							MatchExpressions: []v1.NodeSelectorRequirement{
								{
									Key:      "myKey",
									Operator: v1.NodeSelectorOpDoesNotExist,
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "GenerateNodeAffinityWithJSONInputUsingNotInOperator",
			args: args{
				nodeAffinity: `{"requiredDuringSchedulingIgnoredDuringExecution":{"nodeSelectorTerms":[{"matchExpressions":[{"key":"myKey","operator":"NotIn","values":["myValue"]}]}]}}`,
			},
			want: &v1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
					NodeSelectorTerms: []v1.NodeSelectorTerm{
						{
							MatchExpressions: []v1.NodeSelectorRequirement{
								{
									Key:      "myKey",
									Operator: v1.NodeSelectorOpNotIn,
									Values: []string{
										"myValue",
									},
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "GenerateNodeAffinityWithYAMLInputUsingDoesNotExistOperator",
			args: args{
				nodeAffinity: `
---
requiredDuringSchedulingIgnoredDuringExecution:
  nodeSelectorTerms:
    -
      matchExpressions:
        -
          key: myKey
          operator: DoesNotExist`,
			},
			want: &v1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
					NodeSelectorTerms: []v1.NodeSelectorTerm{
						{
							MatchExpressions: []v1.NodeSelectorRequirement{
								{
									Key:      "myKey",
									Operator: v1.NodeSelectorOpDoesNotExist,
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "GenerateNodeAffinityWithYAMLInputUsingNotInOperator",
			args: args{
				nodeAffinity: `
---
requiredDuringSchedulingIgnoredDuringExecution:
  nodeSelectorTerms:
    -
      matchExpressions:
        -
          key: myKey
          operator: NotIn
          values:
            - myValue`,
			},
			want: &v1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
					NodeSelectorTerms: []v1.NodeSelectorTerm{
						{
							MatchExpressions: []v1.NodeSelectorRequirement{
								{
									Key:      "myKey",
									Operator: v1.NodeSelectorOpNotIn,
									Values: []string{
										"myValue",
									},
								},
							},
						},
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GenerateNodeAffinity(tt.args.nodeAffinity)
			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateNodeAffinity() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GenerateNodeAffinity() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetNotReadyKubernetesNodes(t *testing.T) {
	ctx := context.TODO()
	clientset := optest.New(t, 0)

	// when there is no node
	nodes, err := GetNotReadyKubernetesNodes(ctx, clientset)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(nodes))

	// when all the nodes are in ready state
	clientset = optest.New(t, 2)
	nodes, err = GetNotReadyKubernetesNodes(ctx, clientset)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(nodes))

	// when there is a not ready node
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "failed",
		},
		Status: v1.NodeStatus{
			Conditions: []v1.NodeCondition{
				{
					Type: v1.NodeReady, Status: v1.ConditionFalse,
				},
			},
		},
	}
	_, err = clientset.CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
	assert.NoError(t, err)
	nodes, err = GetNotReadyKubernetesNodes(ctx, clientset)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(nodes))

	// when all the nodes are not ready
	allNodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	assert.NoError(t, err)
	for _, n := range allNodes.Items {
		n.Status.Conditions[0].Status = v1.ConditionFalse
		updateNode := n
		_, err := clientset.CoreV1().Nodes().Update(ctx, &updateNode, metav1.UpdateOptions{})
		assert.NoError(t, err)
	}
	nodes, err = GetNotReadyKubernetesNodes(ctx, clientset)
	assert.NoError(t, err)
	assert.Equal(t, 3, len(nodes))
}
