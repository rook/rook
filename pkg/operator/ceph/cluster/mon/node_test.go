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

func TestGetNodeMonUsageValidNode(t *testing.T) {
	clientset := test.New(2)
	c := New(&clusterd.Context{Clientset: clientset}, "ns", "", false, metav1.OwnerReference{})
	setCommonMonProperties(c, 0, cephv1.MonSpec{Count: 3, AllowMultiplePerNode: true}, "myversion")

	node, err := clientset.CoreV1().Nodes().Get("node0", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, node)
	conditions := v1.NodeCondition{Type: v1.NodeOutOfDisk}
	node.Status = v1.NodeStatus{Conditions: []v1.NodeCondition{conditions}}
	clientset.CoreV1().Nodes().Update(node)

	// nodes are reported with correct valid flag
	nodeZones, err := c.getNodeMonUsage()
	assert.NoError(t, err)
	assert.False(t, nodeZones[0][0].MonValid)
	assert.True(t, nodeZones[0][1].MonValid)
}

func TestGetNodeMonUsageMonCount(t *testing.T) {
	clientset := test.New(2)
	c := New(&clusterd.Context{Clientset: clientset}, "ns", "", false, metav1.OwnerReference{})
	setCommonMonProperties(c, 0, cephv1.MonSpec{Count: 3, AllowMultiplePerNode: true}, "myversion")

	// 3 mons on node0
	node, err := clientset.CoreV1().Nodes().Get("node0", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, node)

	monIDs := []string{"a", "b", "c"}
	for i := 0; i < 3; i++ {
		pod := c.makeMonPod(testGenMonConfig(monIDs[i]), node.Name)
		_, err = clientset.CoreV1().Pods(c.Namespace).Create(pod)
		assert.Nil(t, err)
	}

	// 2 mons on node1
	node, err = clientset.CoreV1().Nodes().Get("node1", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, node)

	monIDs = []string{"d", "e"}
	for i := 0; i < 2; i++ {
		pod := c.makeMonPod(testGenMonConfig(monIDs[i]), node.Name)
		_, err = clientset.CoreV1().Pods(c.Namespace).Create(pod)
		assert.Nil(t, err)
	}

	nodeZones, err := c.getNodeMonUsage()
	assert.NoError(t, err)
	assert.Equal(t, 1, len(nodeZones))
	if nodeZones[0][0].Node.Name == "node0" {
		assert.Equal(t, 3, nodeZones[0][0].MonCount)
		assert.Equal(t, 2, nodeZones[0][1].MonCount)
	} else {
		assert.Equal(t, 2, nodeZones[0][1].MonCount)
		assert.Equal(t, 3, nodeZones[0][0].MonCount)
	}
}

func TestTaintedNodes(t *testing.T) {
	clientset := test.New(4)
	c := New(&clusterd.Context{Clientset: clientset}, "ns", "", false, metav1.OwnerReference{})
	setCommonMonProperties(c, 0, cephv1.MonSpec{Count: 3, AllowMultiplePerNode: true}, "myversion")

	// mark a node as unschedulable
	node, err := clientset.CoreV1().Nodes().Get("node0", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, node)
	node.Spec.Unschedulable = true
	clientset.CoreV1().Nodes().Update(node)

	// taint
	node, err = clientset.CoreV1().Nodes().Get("node1", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, node)
	node.Spec.Taints = []v1.Taint{
		{Effect: v1.TaintEffectNoSchedule},
	}
	clientset.CoreV1().Nodes().Update(node)

	// taint
	node, err = clientset.CoreV1().Nodes().Get("node2", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, node)
	node.Spec.Taints = []v1.Taint{
		{Effect: v1.TaintEffectPreferNoSchedule},
	}
	clientset.CoreV1().Nodes().Update(node)

	nodeZones, err := c.getNodeMonUsage()
	assert.NoError(t, err)
	assert.Equal(t, 1, len(nodeZones))
	validCount := 0
	for zi := range nodeZones {
		for ni := range nodeZones[zi] {
			if nodeZones[zi][ni].MonValid {
				validCount++
			}
		}
	}
	assert.Equal(t, 1, validCount)
}

func TestNodeAffinity(t *testing.T) {
	clientset := test.New(4)
	c := New(&clusterd.Context{Clientset: clientset}, "ns", "", false, metav1.OwnerReference{})
	setCommonMonProperties(c, 0, cephv1.MonSpec{Count: 3, AllowMultiplePerNode: true}, "myversion")

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

	// label nodes so they appear as not scheduable / invalid
	node, err := clientset.CoreV1().Nodes().Get("node0", metav1.GetOptions{})
	node.Labels = map[string]string{"label": "foo"}
	clientset.CoreV1().Nodes().Update(node)

	node, err = clientset.CoreV1().Nodes().Get("node1", metav1.GetOptions{})
	node.Labels = map[string]string{"label": "bar"}
	clientset.CoreV1().Nodes().Update(node)

	node, err = clientset.CoreV1().Nodes().Get("node2", metav1.GetOptions{})
	node.Labels = map[string]string{"label": "baz"}
	clientset.CoreV1().Nodes().Update(node)

	nodeZones, err := c.getNodeMonUsage()
	assert.NoError(t, err)
	assert.Equal(t, 1, len(nodeZones))
	validCount := 0
	for zi := range nodeZones {
		for ni := range nodeZones[zi] {
			if nodeZones[zi][ni].MonValid {
				validCount++
			}
		}
	}
	assert.Equal(t, 2, validCount)
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

	nodes := []v1.Node{}
	nodeZones, err := c.getNodeMonUsage()
	assert.Nil(t, err)

	for zi := range nodeZones {
		for _, nodeUsage := range nodeZones[zi] {
			if nodeUsage.MonCount == 0 || c.spec.Mon.AllowMultiplePerNode {
				nodes = append(nodes, *nodeUsage.Node)
			}
		}
	}

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

// mon node usage should return no zones if there are no nodes
func TestGetNodeMonUsageNoNodes(t *testing.T) {
	clientset := test.New(0)
	c := New(&clusterd.Context{Clientset: clientset}, "ns", "", false, metav1.OwnerReference{})
	setCommonMonProperties(c, 0, cephv1.MonSpec{Count: 3, AllowMultiplePerNode: true}, "myversion")

	nodeZones, err := c.getNodeMonUsage()
	assert.NoError(t, err)
	assert.Equal(t, len(nodeZones), 0)
}

// all nodes without zone annotations are in the same zone
func TestGetNodeMonUsageNoZoneLabels(t *testing.T) {
	for i := 1; i < 5; i++ {
		clientset := test.New(i)
		c := New(&clusterd.Context{Clientset: clientset}, "ns", "", false, metav1.OwnerReference{})
		setCommonMonProperties(c, 0, cephv1.MonSpec{Count: 3, AllowMultiplePerNode: true}, "myversion")

		nodeZones, err := c.getNodeMonUsage()
		assert.NoError(t, err)
		assert.Equal(t, len(nodeZones), 1)
		assert.Equal(t, len(nodeZones[0]), i)
	}
}

// nodes are partitioned into separate zones
func TestGetNodeMonUsageZoneSpread(t *testing.T) {
	clientset := test.New(5)
	c := New(&clusterd.Context{Clientset: clientset}, "ns", "", false, metav1.OwnerReference{})
	setCommonMonProperties(c, 0, cephv1.MonSpec{Count: 3, AllowMultiplePerNode: true}, "myversion")

	// 1 node labeled -> 1 zone + the rest in the unlabeled zone
	node, err := clientset.CoreV1().Nodes().Get("node0", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, node)
	node.Labels = map[string]string{"failure-domain.beta.kubernetes.io/zone": "z0"}
	clientset.CoreV1().Nodes().Update(node)

	// the zones are unlabeled and sort order isn't guaranteed. so instead we
	// assert on expected count of zones and collection of zone sizes.
	zoneCounts := map[int]int{}

	nodeZones, err := c.getNodeMonUsage()
	assert.NoError(t, err)
	assert.Equal(t, 2, len(nodeZones))
	for _, zones := range nodeZones {
		zoneSize := len(zones)
		if _, ok := zoneCounts[zoneSize]; ok {
			zoneCounts[zoneSize]++
		} else {
			zoneCounts[zoneSize] = 1
		}
	}
	assert.Equal(t, map[int]int{1: 1, 4: 1}, zoneCounts)

	// 2 nodes with same label, 3 unlabeled
	node, err = clientset.CoreV1().Nodes().Get("node1", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, node)
	node.Labels = map[string]string{"failure-domain.beta.kubernetes.io/zone": "z0"}
	clientset.CoreV1().Nodes().Update(node)

	// reset
	zoneCounts = map[int]int{}

	nodeZones, err = c.getNodeMonUsage()
	assert.NoError(t, err)
	assert.Equal(t, 2, len(nodeZones))
	for _, zones := range nodeZones {
		zoneSize := len(zones)
		if _, ok := zoneCounts[zoneSize]; ok {
			zoneCounts[zoneSize]++
		} else {
			zoneCounts[zoneSize] = 1
		}
	}
	assert.Equal(t, map[int]int{2: 1, 3: 1}, zoneCounts)

	// 2 nodes with same label, 2 with distinct labels, 1 unlabeled
	node, err = clientset.CoreV1().Nodes().Get("node2", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, node)
	node.Labels = map[string]string{"failure-domain.beta.kubernetes.io/zone": "z1"}
	clientset.CoreV1().Nodes().Update(node)

	node, err = clientset.CoreV1().Nodes().Get("node3", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, node)
	node.Labels = map[string]string{"failure-domain.beta.kubernetes.io/zone": "z2"}
	clientset.CoreV1().Nodes().Update(node)

	// reset
	zoneCounts = map[int]int{}

	nodeZones, err = c.getNodeMonUsage()
	assert.NoError(t, err)
	assert.Equal(t, 4, len(nodeZones))
	for _, zones := range nodeZones {
		zoneSize := len(zones)
		if _, ok := zoneCounts[zoneSize]; ok {
			zoneCounts[zoneSize]++
		} else {
			zoneCounts[zoneSize] = 1
		}
	}
	assert.Equal(t, map[int]int{2: 1, 1: 3}, zoneCounts)

	// no unlabeled zones
	node, err = clientset.CoreV1().Nodes().Get("node4", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, node)
	node.Labels = map[string]string{"failure-domain.beta.kubernetes.io/zone": "z1"}
	clientset.CoreV1().Nodes().Update(node)

	// reset
	zoneCounts = map[int]int{}

	nodeZones, err = c.getNodeMonUsage()
	assert.NoError(t, err)
	assert.Equal(t, 3, len(nodeZones))
	for _, zones := range nodeZones {
		zoneSize := len(zones)
		if _, ok := zoneCounts[zoneSize]; ok {
			zoneCounts[zoneSize]++
		} else {
			zoneCounts[zoneSize] = 1
		}
	}
	assert.Equal(t, map[int]int{2: 2, 1: 1}, zoneCounts)
}
