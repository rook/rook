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

package mon

import (
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAvailableMonNodes(t *testing.T) {
	clientset := test.New(1)
	c := New(&clusterd.Context{Clientset: clientset}, "ns", "", "myversion", cephv1.CephVersionSpec{},
		cephv1.MonSpec{Count: 3, AllowMultiplePerNode: true}, rookalpha.Placement{},
		false, v1.ResourceRequirements{}, metav1.OwnerReference{})
	c.clusterInfo = test.CreateConfigDir(0)
	nodes, err := c.getMonNodes()
	assert.Nil(t, err)
	assert.Equal(t, 1, len(nodes))

	// set node to not ready
	conditions := v1.NodeCondition{Type: v1.NodeOutOfDisk}
	nodes[0].Status = v1.NodeStatus{Conditions: []v1.NodeCondition{conditions}}
	clientset.CoreV1().Nodes().Update(&nodes[0])

	// when the node is not ready there should be no nodes returned and an error
	emptyNodes, err := c.getMonNodes()
	assert.Nil(t, err)
	assert.Equal(t, 0, len(emptyNodes))

	// even if AllowMultiplePerNode is true there should be no node returned
	c.AllowMultiplePerNode = true
	emptyNodes, err = c.getMonNodes()
	assert.Nil(t, err)
	assert.Equal(t, 0, len(emptyNodes))
}

func TestAvailableNodesInUse(t *testing.T) {
	clientset := test.New(3)
	c := New(&clusterd.Context{Clientset: clientset}, "ns", "", "myversion", cephv1.CephVersionSpec{},
		cephv1.MonSpec{Count: 3, AllowMultiplePerNode: true}, rookalpha.Placement{},
		false, v1.ResourceRequirements{}, metav1.OwnerReference{})
	c.clusterInfo = test.CreateConfigDir(0)

	// all three nodes are available by default
	nodes, err := c.getMonNodes()
	assert.Nil(t, err)
	assert.Equal(t, 3, len(nodes))

	// start pods on two of the nodes so that only one node will be available
	monIDs := []string{"a", "b"}
	for i := 0; i < 2; i++ {
		pod := c.makeMonPod(testGenMonConfig(monIDs[i]), nodes[i].Name)
		_, err = clientset.CoreV1().Pods(c.Namespace).Create(pod)
		assert.Nil(t, err)
	}
	reducedNodes, err := c.getMonNodes()
	assert.Nil(t, err)
	assert.Equal(t, 1, len(reducedNodes))
	assert.Equal(t, nodes[2].Name, reducedNodes[0].Name)

	// start pods on the remaining node. We expect no nodes to be available for placement
	// since there is no way to place a mon on an unused node.
	pod := c.makeMonPod(testGenMonConfig("c"), nodes[2].Name)
	_, err = clientset.CoreV1().Pods(c.Namespace).Create(pod)
	assert.Nil(t, err)
	nodes, err = c.getMonNodes()
	// no mon nodes is no error, just an empty nodes list
	assert.Nil(t, err)
	assert.Equal(t, 3, len(nodes))

	// no nodes should be returned when AllowMultiplePerNode is false
	c.AllowMultiplePerNode = false
	nodes, err = c.getMonNodes()
	// no mon nodes is no error, just an empty nodes list
	assert.Nil(t, err)
	assert.Equal(t, 0, len(nodes))
}

func TestTaintedNodes(t *testing.T) {
	clientset := test.New(3)
	c := New(&clusterd.Context{Clientset: clientset}, "ns", "", "myversion", cephv1.CephVersionSpec{},
		cephv1.MonSpec{Count: 3, AllowMultiplePerNode: true}, rookalpha.Placement{},
		false, v1.ResourceRequirements{}, metav1.OwnerReference{})
	c.clusterInfo = test.CreateConfigDir(0)

	nodes, err := c.getMonNodes()
	assert.Nil(t, err)
	assert.Equal(t, 3, len(nodes))

	// mark a node as unschedulable
	nodes[0].Spec.Unschedulable = true
	clientset.CoreV1().Nodes().Update(&nodes[0])
	nodesSchedulable, err := c.getMonNodes()
	assert.Nil(t, err)
	assert.Equal(t, 2, len(nodesSchedulable))
	nodes[0].Spec.Unschedulable = false

	// taint nodes so they will not be schedulable for new pods
	nodes[0].Spec.Taints = []v1.Taint{
		{Effect: v1.TaintEffectNoSchedule},
	}
	nodes[1].Spec.Taints = []v1.Taint{
		{Effect: v1.TaintEffectPreferNoSchedule},
	}
	clientset.CoreV1().Nodes().Update(&nodes[0])
	clientset.CoreV1().Nodes().Update(&nodes[1])
	clientset.CoreV1().Nodes().Update(&nodes[2])
	cleanNodes, err := c.getMonNodes()
	assert.Nil(t, err)
	assert.Equal(t, 1, len(cleanNodes))
	assert.Equal(t, nodes[2].Name, cleanNodes[0].Name)
}

func TestNodeAffinity(t *testing.T) {
	clientset := test.New(3)
	c := New(&clusterd.Context{Clientset: clientset}, "ns", "", "myversion", cephv1.CephVersionSpec{},
		cephv1.MonSpec{Count: 3, AllowMultiplePerNode: true}, rookalpha.Placement{},
		false, v1.ResourceRequirements{}, metav1.OwnerReference{})
	c.clusterInfo = test.CreateConfigDir(0)

	nodes, err := c.getMonNodes()
	assert.Nil(t, err)
	assert.Equal(t, 3, len(nodes))

	c.placement.NodeAffinity = &v1.NodeAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
			NodeSelectorTerms: []v1.NodeSelectorTerm{
				{
					MatchExpressions: []v1.NodeSelectorRequirement{
						{
							Key:      "label",
							Operator: v1.NodeSelectorOpIn,
							Values:   []string{"bar", "baz"},
						},
					},
				},
			},
		},
	}

	// label nodes so node0 will not be schedulable for new pods
	nodes[0].Labels = map[string]string{"label": "foo"}
	nodes[1].Labels = map[string]string{"label": "bar"}
	nodes[2].Labels = map[string]string{"label": "baz"}
	clientset.CoreV1().Nodes().Update(&nodes[0])
	clientset.CoreV1().Nodes().Update(&nodes[1])
	clientset.CoreV1().Nodes().Update(&nodes[2])
	cleanNodes, err := c.getMonNodes()

	assert.Nil(t, err)
	assert.Equal(t, 2, len(cleanNodes))
	assert.Equal(t, nodes[1].Name, cleanNodes[0].Name)
	assert.Equal(t, nodes[2].Name, cleanNodes[1].Name)
}

// this tests can 3 mons with hostnetworking on the same host is rejected
func TestHostNetworkSameNode(t *testing.T) {
	namespace := "ns"
	context := newTestStartCluster(namespace)

	// cluster host networking
	c := newCluster(context, namespace, true, true, v1.ResourceRequirements{})
	c.clusterInfo = test.CreateConfigDir(1)

	// start a basic cluster
	_, err := c.Start()
	assert.Error(t, err)
}

func TestHostNetwork(t *testing.T) {
	clientset := test.New(3)
	c := New(&clusterd.Context{Clientset: clientset}, "ns", "", "myversion", cephv1.CephVersionSpec{},
		cephv1.MonSpec{Count: 3, AllowMultiplePerNode: false}, rookalpha.Placement{},
		false, v1.ResourceRequirements{}, metav1.OwnerReference{})
	c.clusterInfo = test.CreateConfigDir(0)

	c.HostNetwork = true

	nodes, err := c.getMonNodes()
	assert.Nil(t, err)
	assert.Equal(t, 3, len(nodes))

	pod := c.makeMonPod(testGenMonConfig("c"), nodes[2].Name)
	assert.NotNil(t, pod)

	assert.Equal(t, true, pod.Spec.HostNetwork)
	assert.Equal(t, v1.DNSClusterFirstWithHostNet, pod.Spec.DNSPolicy)
}

func TestGetNodeInfoFromNode(t *testing.T) {
	clientset := test.New(1)
	node, err := clientset.CoreV1().Nodes().Get("node0", metav1.GetOptions{})
	assert.Nil(t, err)
	assert.NotNil(t, node)

	node.Status = v1.NodeStatus{}
	node.Status.Addresses = []v1.NodeAddress{
		{
			Type:    v1.NodeExternalIP,
			Address: "1.1.1.1",
		},
	}

	c := New(&clusterd.Context{Clientset: clientset}, "ns", "", "myversion", cephv1.CephVersionSpec{},
		cephv1.MonSpec{Count: 3, AllowMultiplePerNode: true}, rookalpha.Placement{},
		true, v1.ResourceRequirements{}, metav1.OwnerReference{})
	c.clusterInfo = test.CreateConfigDir(0)

	var info *NodeInfo
	info, err = getNodeInfoFromNode(*node)
	assert.Nil(t, err)

	assert.Equal(t, "1.1.1.1", info.Address)
}
