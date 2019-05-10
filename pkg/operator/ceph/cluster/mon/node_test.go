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
	"strings"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAvailableMonNodes(t *testing.T) {
	clientset := test.New(1)
	c := New(&clusterd.Context{Clientset: clientset}, "ns", "", false, metav1.OwnerReference{})
	setCommonMonProperties(c, 0, cephv1.MonSpec{Count: 3, AllowMultiplePerNode: true}, "myversion")

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
	c.spec.Mon.AllowMultiplePerNode = true
	emptyNodes, err = c.getMonNodes()
	assert.Nil(t, err)
	assert.Equal(t, 0, len(emptyNodes))
}

func TestAvailableNodesInUse(t *testing.T) {
	clientset := test.New(3)
	c := New(&clusterd.Context{Clientset: clientset}, "ns", "", false, metav1.OwnerReference{})
	setCommonMonProperties(c, 0, cephv1.MonSpec{Count: 3, AllowMultiplePerNode: true}, "myversion")

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
	c.spec.Mon.AllowMultiplePerNode = false
	nodes, err = c.getMonNodes()
	// no mon nodes is no error, just an empty nodes list
	assert.Nil(t, err)
	assert.Equal(t, 0, len(nodes))
}

func TestTaintedNodes(t *testing.T) {
	clientset := test.New(3)
	c := New(&clusterd.Context{Clientset: clientset}, "ns", "", false, metav1.OwnerReference{})
	setCommonMonProperties(c, 0, cephv1.MonSpec{Count: 3, AllowMultiplePerNode: true}, "myversion")

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
	c := New(&clusterd.Context{Clientset: clientset}, "ns", "", false, metav1.OwnerReference{})
	setCommonMonProperties(c, 0, cephv1.MonSpec{Count: 3, AllowMultiplePerNode: true}, "myversion")

	nodes, err := c.getMonNodes()
	assert.Nil(t, err)
	assert.Equal(t, 3, len(nodes))

	c.spec.Placement = map[rookalpha.KeyType]rookalpha.Placement{}
	c.spec.Placement[cephv1.KeyMon] = rookalpha.Placement{NodeAffinity: &v1.NodeAffinity{
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
	_, err := c.Start(c.clusterInfo, c.rookVersion, cephver.Mimic, c.spec)
	assert.Error(t, err)
}

func TestPodMemory(t *testing.T) {
	namespace := "ns"
	context := newTestStartCluster(namespace)

	// Test memory limit alone
	r := v1.ResourceRequirements{
		Limits: v1.ResourceList{
			v1.ResourceMemory: *resource.NewQuantity(536870912, resource.BinarySI), // size in Bytes
		},
	}

	c := newCluster(context, namespace, false, true, r)
	c.clusterInfo = test.CreateConfigDir(1)
	// start a basic cluster
	_, err := c.Start(c.clusterInfo, c.rookVersion, cephver.Mimic, c.spec)
	assert.Error(t, err)

	// Test REQUEST == LIMIT
	r = v1.ResourceRequirements{
		Limits: v1.ResourceList{
			v1.ResourceMemory: *resource.NewQuantity(536870912, resource.BinarySI), // size in Bytes
		},
		Requests: v1.ResourceList{
			v1.ResourceMemory: *resource.NewQuantity(536870912, resource.BinarySI), // size in Bytes
		},
	}

	c = newCluster(context, namespace, false, true, r)
	c.clusterInfo = test.CreateConfigDir(1)
	// start a basic cluster
	_, err = c.Start(c.clusterInfo, c.rookVersion, cephver.Mimic, c.spec)
	assert.Error(t, err)

	// Test LIMIT != REQUEST but obviously LIMIT > REQUEST
	r = v1.ResourceRequirements{
		Limits: v1.ResourceList{
			v1.ResourceMemory: *resource.NewQuantity(536870912, resource.BinarySI), // size in Bytes
		},
		Requests: v1.ResourceList{
			v1.ResourceMemory: *resource.NewQuantity(236870912, resource.BinarySI), // size in Bytes
		},
	}

	c = newCluster(context, namespace, false, true, r)
	c.clusterInfo = test.CreateConfigDir(1)
	// start a basic cluster
	_, err = c.Start(c.clusterInfo, c.rookVersion, cephver.Mimic, c.spec)
	assert.Error(t, err)

	// Test valid case where pod resource is set approprietly
	r = v1.ResourceRequirements{
		Limits: v1.ResourceList{
			v1.ResourceMemory: *resource.NewQuantity(1073741824, resource.BinarySI), // size in Bytes
		},
		Requests: v1.ResourceList{
			v1.ResourceMemory: *resource.NewQuantity(236870912, resource.BinarySI), // size in Bytes
		},
	}

	c = newCluster(context, namespace, false, true, r)
	c.clusterInfo = test.CreateConfigDir(1)
	// start a basic cluster
	_, err = c.Start(c.clusterInfo, c.rookVersion, cephver.Mimic, c.spec)
	assert.Nil(t, err)

	// Test no resources were specified on the pod
	r = v1.ResourceRequirements{}
	c = newCluster(context, namespace, false, true, r)
	c.clusterInfo = test.CreateConfigDir(1)
	// start a basic cluster
	_, err = c.Start(c.clusterInfo, c.rookVersion, cephver.Mimic, c.spec)
	assert.Nil(t, err)

}

func TestHostNetwork(t *testing.T) {
	clientset := test.New(3)
	c := New(&clusterd.Context{Clientset: clientset}, "ns", "", false, metav1.OwnerReference{})
	setCommonMonProperties(c, 0, cephv1.MonSpec{Count: 3, AllowMultiplePerNode: true}, "myversion")

	c.HostNetwork = true

	nodes, err := c.getMonNodes()
	assert.Nil(t, err)
	assert.Equal(t, 3, len(nodes))

	monConfig := testGenMonConfig("c")
	pod := c.makeMonPod(monConfig, nodes[2].Name)
	assert.NotNil(t, pod)
	assert.Equal(t, true, pod.Spec.HostNetwork)
	assert.Equal(t, v1.DNSClusterFirstWithHostNet, pod.Spec.DNSPolicy)
	val, message := extractArgValue(pod.Spec.Containers[0].Args, "--public-addr")
	assert.Equal(t, "2.4.6.3", val, message)
	val, message = extractArgValue(pod.Spec.Containers[0].Args, "--public-bind-addr")
	assert.Equal(t, "", val)
	assert.Equal(t, "arg not found: --public-bind-addr", message)

	monConfig.Port = 6790
	pod = c.makeMonPod(monConfig, nodes[2].Name)
	val, message = extractArgValue(pod.Spec.Containers[0].Args, "--public-addr")
	assert.Equal(t, "2.4.6.3:6790", val, message)
	assert.NotNil(t, pod)
}

func extractArgValue(args []string, name string) (string, string) {
	for _, arg := range args {
		if strings.Contains(arg, name) {
			vals := strings.Split(arg, "=")
			if len(vals) != 2 {
				return "", "cannot split arg: " + arg
			}
			return vals[1], "value: " + vals[1]
		}
	}
	return "", "arg not found: " + name
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

	var info *NodeInfo
	info, err = getNodeInfoFromNode(*node)
	assert.Nil(t, err)

	assert.Equal(t, "1.1.1.1", info.Address)
}

func TestTargetMonCount(t *testing.T) {
	// only a single node and min 1
	spec := cephv1.MonSpec{Count: 1, PreferredCount: 0, AllowMultiplePerNode: false}
	nodes := 1
	target, msg := calcTargetMonCount(nodes, spec)
	assert.Equal(t, 1, target)
	logger.Infof(msg)

	// preferred 3 mons, but only one node available
	spec.PreferredCount = 3
	target, msg = calcTargetMonCount(nodes, spec)
	assert.Equal(t, 1, target)
	logger.Infof(msg)

	// preferred 3 mons, and 3 nodes available
	nodes = 3
	target, msg = calcTargetMonCount(nodes, spec)
	assert.Equal(t, 3, target)
	logger.Infof(msg)

	// get an intermediate odd number since we have several options between the min and the node count
	nodes = 4
	spec.Count = 1
	spec.PreferredCount = 5
	target, msg = calcTargetMonCount(nodes, spec)
	assert.Equal(t, 3, target)
	logger.Infof(msg)
	nodes = 5
	spec.PreferredCount = 6
	target, msg = calcTargetMonCount(nodes, spec)
	assert.Equal(t, 5, target)
	logger.Infof(msg)
	nodes = 6
	spec.PreferredCount = 7
	target, msg = calcTargetMonCount(nodes, spec)
	assert.Equal(t, 5, target)
	logger.Infof(msg)
	nodes = 7
	spec.PreferredCount = 7
	target, msg = calcTargetMonCount(nodes, spec)
	assert.Equal(t, 7, target)
	logger.Infof(msg)

	// multiple allowed per node, so we always returned preferred
	nodes = 1
	spec.PreferredCount = 7
	spec.AllowMultiplePerNode = true
	target, msg = calcTargetMonCount(nodes, spec)
	assert.Equal(t, 7, target)
	logger.Infof(msg)
}
