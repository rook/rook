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
	"context"
	"strings"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	clienttest "github.com/rook/rook/pkg/daemon/ceph/client/test"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNodeAffinity(t *testing.T) {
	ctx := context.TODO()
	clientset := test.New(t, 4)
	c := New(ctx, &clusterd.Context{Clientset: clientset}, "ns", cephv1.ClusterSpec{}, &k8sutil.OwnerInfo{})
	setCommonMonProperties(c, 0, cephv1.MonSpec{Count: 3, AllowMultiplePerNode: true}, "myversion")

	c.spec.Placement = map[cephv1.KeyType]cephv1.Placement{}
	c.spec.Placement[cephv1.KeyMon] = cephv1.Placement{NodeAffinity: &v1.NodeAffinity{
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
	node, _ := clientset.CoreV1().Nodes().Get(ctx, "node0", metav1.GetOptions{})
	node.Labels = map[string]string{"label": "foo"}
	_, err := clientset.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
	assert.NoError(t, err)

	node, _ = clientset.CoreV1().Nodes().Get(ctx, "node1", metav1.GetOptions{})
	node.Labels = map[string]string{"label": "bar"}
	_, err = clientset.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
	assert.NoError(t, err)

	node, _ = clientset.CoreV1().Nodes().Get(ctx, "node2", metav1.GetOptions{})
	node.Labels = map[string]string{"label": "baz"}
	_, err = clientset.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
	assert.NoError(t, err)
}

// this tests can 3 mons with hostnetworking on the same host is rejected
func TestHostNetworkSameNode(t *testing.T) {
	namespace := "ns"
	context, err := newTestStartCluster(t, namespace)
	assert.NoError(t, err)
	// cluster host networking
	c := newCluster(context, namespace, true, v1.ResourceRequirements{})
	c.spec.Network.HostNetwork = true
	c.ClusterInfo = clienttest.CreateTestClusterInfo(1)

	// start a basic cluster
	_, err = c.Start(c.ClusterInfo, c.rookVersion, cephver.Octopus, c.spec)
	assert.Error(t, err)
}

func TestPodMemory(t *testing.T) {
	namespace := "ns"
	context, err := newTestStartCluster(t, namespace)
	assert.NoError(t, err)
	// Test memory limit alone
	r := v1.ResourceRequirements{
		Limits: v1.ResourceList{
			v1.ResourceMemory: *resource.NewQuantity(536870912, resource.BinarySI), // size in Bytes
		},
	}

	c := newCluster(context, namespace, true, r)
	c.ClusterInfo = clienttest.CreateTestClusterInfo(1)
	// start a basic cluster
	_, err = c.Start(c.ClusterInfo, c.rookVersion, cephver.Octopus, c.spec)
	assert.NoError(t, err)

	// Test REQUEST == LIMIT
	r = v1.ResourceRequirements{
		Limits: v1.ResourceList{
			v1.ResourceMemory: *resource.NewQuantity(536870912, resource.BinarySI), // size in Bytes
		},
		Requests: v1.ResourceList{
			v1.ResourceMemory: *resource.NewQuantity(536870912, resource.BinarySI), // size in Bytes
		},
	}

	c = newCluster(context, namespace, true, r)
	c.ClusterInfo = clienttest.CreateTestClusterInfo(1)
	// start a basic cluster
	_, err = c.Start(c.ClusterInfo, c.rookVersion, cephver.Octopus, c.spec)
	assert.NoError(t, err)

	// Test LIMIT != REQUEST but obviously LIMIT > REQUEST
	r = v1.ResourceRequirements{
		Limits: v1.ResourceList{
			v1.ResourceMemory: *resource.NewQuantity(536870912, resource.BinarySI), // size in Bytes
		},
		Requests: v1.ResourceList{
			v1.ResourceMemory: *resource.NewQuantity(236870912, resource.BinarySI), // size in Bytes
		},
	}

	c = newCluster(context, namespace, true, r)
	c.ClusterInfo = clienttest.CreateTestClusterInfo(1)
	// start a basic cluster
	_, err = c.Start(c.ClusterInfo, c.rookVersion, cephver.Octopus, c.spec)
	assert.NoError(t, err)

	// Test valid case where pod resource is set appropriately
	r = v1.ResourceRequirements{
		Limits: v1.ResourceList{
			v1.ResourceMemory: *resource.NewQuantity(1073741824, resource.BinarySI), // size in Bytes
		},
		Requests: v1.ResourceList{
			v1.ResourceMemory: *resource.NewQuantity(236870912, resource.BinarySI), // size in Bytes
		},
	}

	c = newCluster(context, namespace, true, r)
	c.ClusterInfo = clienttest.CreateTestClusterInfo(1)
	// start a basic cluster
	_, err = c.Start(c.ClusterInfo, c.rookVersion, cephver.Octopus, c.spec)
	assert.NoError(t, err)

	// Test no resources were specified on the pod
	r = v1.ResourceRequirements{}
	c = newCluster(context, namespace, true, r)
	c.ClusterInfo = clienttest.CreateTestClusterInfo(1)
	// start a basic cluster
	_, err = c.Start(c.ClusterInfo, c.rookVersion, cephver.Octopus, c.spec)
	assert.NoError(t, err)

}

func TestHostNetwork(t *testing.T) {
	clientset := test.New(t, 3)
	c := New(context.TODO(), &clusterd.Context{Clientset: clientset}, "ns", cephv1.ClusterSpec{}, &k8sutil.OwnerInfo{})
	setCommonMonProperties(c, 0, cephv1.MonSpec{Count: 3, AllowMultiplePerNode: true}, "myversion")

	c.spec.Network.HostNetwork = true

	monConfig := testGenMonConfig("c")
	pod, err := c.makeMonPod(monConfig, false)
	assert.NoError(t, err)
	assert.NotNil(t, pod)
	assert.Equal(t, true, pod.Spec.HostNetwork)
	assert.Equal(t, v1.DNSClusterFirstWithHostNet, pod.Spec.DNSPolicy)
	val, message := extractArgValue(pod.Spec.Containers[0].Args, "--public-addr")
	assert.Equal(t, "2.4.6.3", val, message)
	val, message = extractArgValue(pod.Spec.Containers[0].Args, "--public-bind-addr")
	assert.Equal(t, "", val)
	assert.Equal(t, "arg not found: --public-bind-addr", message)

	monConfig.Port = 6790
	pod, err = c.makeMonPod(monConfig, false)
	assert.NoError(t, err)
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
	ctx := context.TODO()
	clientset := test.New(t, 1)
	node, err := clientset.CoreV1().Nodes().Get(ctx, "node0", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, node)

	node.Status = v1.NodeStatus{}
	node.Status.Addresses = []v1.NodeAddress{}

	var info *opcontroller.MonScheduleInfo
	_, err = getNodeInfoFromNode(*node)
	assert.NotNil(t, err)

	// With internalIP and externalIP
	node.Status.Addresses = []v1.NodeAddress{
		{
			Type:    v1.NodeExternalIP,
			Address: "1.1.1.1",
		},
		{
			Type:    v1.NodeInternalIP,
			Address: "172.17.0.1",
		},
	}
	info, err = getNodeInfoFromNode(*node)
	assert.NoError(t, err)
	assert.Equal(t, "172.17.0.1", info.Address) // Must return the internalIP

	// With externalIP only
	node.Status.Addresses = []v1.NodeAddress{
		{
			Type:    v1.NodeExternalIP,
			Address: "1.2.3.4",
		},
	}
	info, err = getNodeInfoFromNode(*node)
	assert.NoError(t, err)
	assert.Equal(t, "1.2.3.4", info.Address)
}
