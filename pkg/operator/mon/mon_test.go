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
	"fmt"
	"io/ioutil"
	"path"
	"strings"
	"testing"

	"os"

	"github.com/rook/rook/pkg/ceph/client"
	clienttest "github.com/rook/rook/pkg/ceph/client/test"
	cephtest "github.com/rook/rook/pkg/ceph/test"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/pkg/api/v1"
)

func TestStartMonPods(t *testing.T) {
	clientset := test.New(3)
	namespace := "ns"
	configDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(configDir)
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(actionName string, command string, args ...string) (string, error) {
			if strings.Contains(command, "ceph-authtool") {
				cephtest.CreateClusterInfo(nil, path.Join(configDir, namespace), nil)
			}
			return "", nil
		},
	}
	context := &clusterd.Context{
		KubeContext: clusterd.KubeContext{Clientset: clientset, MaxRetries: 1},
		Executor:    executor,
		ConfigDir:   configDir,
	}
	c := New(context, namespace, "", "myversion", k8sutil.Placement{})

	// start a basic cluster
	// an error is expected since mocking always creates pods that are not running
	info, err := c.Start()
	assert.NotNil(t, err)
	assert.Nil(t, info)

	validateStart(t, c)

	// starting again should be a no-op, but still results in an error
	info, err = c.Start()
	assert.NotNil(t, err)
	assert.Nil(t, info)

	validateStart(t, c)
}

func validateStart(t *testing.T, c *Cluster) {
	s, err := c.context.Clientset.CoreV1().Secrets(c.Namespace).Get("rook-admin", metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, 1, len(s.StringData))

	s, err = c.context.Clientset.CoreV1().Secrets(c.Namespace).Get(appName, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, 4, len(s.StringData))

	// there is only one pod created. the other two won't be created since the first one doesn't start
	_, err = c.context.Clientset.Extensions().ReplicaSets(c.Namespace).Get("rook-ceph-mon0", metav1.GetOptions{})
	assert.Nil(t, err)
}

func TestSaveMonEndpoints(t *testing.T) {
	clientset := test.New(1)
	configDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(configDir)
	c := New(&clusterd.Context{KubeContext: clusterd.KubeContext{Clientset: clientset}, ConfigDir: configDir}, "ns", "", "myversion", k8sutil.Placement{})
	c.clusterInfo = test.CreateClusterInfo(1)

	// create the initial config map
	err := c.saveMonConfig()
	assert.Nil(t, err)

	cm, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Get("mon-config", metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, "mon1=1.2.3.1:6790", cm.Data["endpoints"])

	// update the config map
	c.clusterInfo.Monitors["mon1"].Endpoint = "2.3.4.5:6790"
	err = c.saveMonConfig()
	assert.Nil(t, err)

	cm, err = c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Get("mon-config", metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, "mon1=2.3.4.5:6790", cm.Data["endpoints"])
}

func TestCheckHealth(t *testing.T) {
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(actionName string, command string, args ...string) (string, error) {
			return clienttest.MonInQuorumResponse(), nil
		},
	}
	clientset := test.New(1)
	configDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(configDir)
	context := &clusterd.Context{
		KubeContext: clusterd.KubeContext{Clientset: clientset, RetryDelay: 1, MaxRetries: 1},
		ConfigDir:   configDir,
		Executor:    executor,
	}
	c := New(context, "ns", "", "myversion", k8sutil.Placement{})
	c.clusterInfo = test.CreateClusterInfo(1)
	c.waitForStart = false
	defer os.RemoveAll(c.context.ConfigDir)

	err := c.CheckHealth()
	assert.Nil(t, err)

	c.maxMonID = 10
	err = c.failoverMon("mon1")
	assert.Nil(t, err)

	cm, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Get("mon-config", metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, "mon11=:6790", cm.Data["endpoints"])
}

func TestMonInQuourm(t *testing.T) {
	entry := client.MonMapEntry{Name: "foo", Rank: 23}
	quorum := []int{}
	// Nothing in quorum
	assert.False(t, monInQuorum(entry, quorum))

	// One or more members in quorum
	quorum = []int{23}
	assert.True(t, monInQuorum(entry, quorum))
	quorum = []int{5, 6, 7, 23, 8}
	assert.True(t, monInQuorum(entry, quorum))

	// Not in quorum
	entry.Rank = 1
	assert.False(t, monInQuorum(entry, quorum))
}

func TestMonID(t *testing.T) {
	// invalid
	id, err := getMonID("m")
	assert.NotNil(t, err)
	assert.Equal(t, -1, id)
	id, err = getMonID("mon")
	assert.NotNil(t, err)
	assert.Equal(t, -1, id)
	id, err = getMonID("monitor0")
	assert.NotNil(t, err)
	assert.Equal(t, -1, id)

	// valid
	id, err = getMonID("mon0")
	assert.Nil(t, err)
	assert.Equal(t, 0, id)
	id, err = getMonID("mon123")
	assert.Nil(t, err)
	assert.Equal(t, 123, id)
}

func TestAvailableMonNodes(t *testing.T) {
	clientset := test.New(1)
	c := New(&clusterd.Context{KubeContext: clusterd.KubeContext{Clientset: clientset}}, "ns", "", "myversion", k8sutil.Placement{})
	c.clusterInfo = test.CreateClusterInfo(0)
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

func TestAvailableNodesInUse(t *testing.T) {
	clientset := test.New(3)
	c := New(&clusterd.Context{KubeContext: clusterd.KubeContext{Clientset: clientset}}, "ns", "", "myversion", k8sutil.Placement{})
	c.clusterInfo = test.CreateClusterInfo(0)

	// all three nodes are available by default
	nodes, err := c.getAvailableMonNodes()
	assert.Nil(t, err)
	assert.Equal(t, 3, len(nodes))

	// start pods on two of the nodes so that only one node will be available
	for i := 0; i < 2; i++ {
		pod := c.makeMonPod(&MonConfig{Name: fmt.Sprintf("rook-ceph-mon%d", i)}, nodes[i].Name)
		_, err := clientset.CoreV1().Pods(c.Namespace).Create(pod)
		assert.Nil(t, err)
	}
	reducedNodes, err := c.getAvailableMonNodes()
	assert.Nil(t, err)
	assert.Equal(t, 1, len(reducedNodes))
	assert.Equal(t, nodes[2].Name, reducedNodes[0].Name)

	// start pods on the remaining node. We expect all nodes to be available for placement
	// since there is no way to place a mon on an unused node.
	pod := c.makeMonPod(&MonConfig{Name: "mon2"}, nodes[2].Name)
	_, err = clientset.CoreV1().Pods(c.Namespace).Create(pod)
	assert.Nil(t, err)
	nodes, err = c.getAvailableMonNodes()
	assert.Nil(t, err)
	assert.Equal(t, 3, len(nodes))
}

func TestTaintedNodes(t *testing.T) {
	clientset := test.New(3)
	c := New(&clusterd.Context{KubeContext: clusterd.KubeContext{Clientset: clientset}}, "ns", "", "myversion", k8sutil.Placement{})
	c.clusterInfo = test.CreateClusterInfo(0)

	nodes, err := c.getAvailableMonNodes()
	assert.Nil(t, err)
	assert.Equal(t, 3, len(nodes))

	// mark a node as unschedulable
	nodes[0].Spec.Unschedulable = true
	clientset.CoreV1().Nodes().Update(&nodes[0])
	nodesSchedulable, err := c.getAvailableMonNodes()
	assert.Nil(t, err)
	assert.Equal(t, 2, len(nodesSchedulable))
	nodes[0].Spec.Unschedulable = false

	// taint nodes so they will not be schedulable for new pods
	nodes[0].Spec.Taints = []v1.Taint{
		v1.Taint{Effect: v1.TaintEffectNoSchedule},
	}
	nodes[1].Spec.Taints = []v1.Taint{
		v1.Taint{Effect: v1.TaintEffectPreferNoSchedule},
	}
	clientset.CoreV1().Nodes().Update(&nodes[0])
	clientset.CoreV1().Nodes().Update(&nodes[1])
	clientset.CoreV1().Nodes().Update(&nodes[2])
	cleanNodes, err := c.getAvailableMonNodes()
	assert.Nil(t, err)
	assert.Equal(t, 1, len(cleanNodes))
	assert.Equal(t, nodes[2].Name, cleanNodes[0].Name)
}

func TestNodeAffinity(t *testing.T) {
	clientset := test.New(3)
	c := New(&clusterd.Context{KubeContext: clusterd.KubeContext{Clientset: clientset}}, "ns", "", "myversion", k8sutil.Placement{})
	c.clusterInfo = test.CreateClusterInfo(0)

	nodes, err := c.getAvailableMonNodes()
	assert.Nil(t, err)
	assert.Equal(t, 3, len(nodes))

	c.placement.NodeAffinity = &v1.NodeAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
			NodeSelectorTerms: []v1.NodeSelectorTerm{
				v1.NodeSelectorTerm{
					MatchExpressions: []v1.NodeSelectorRequirement{
						v1.NodeSelectorRequirement{
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
	cleanNodes, err := c.getAvailableMonNodes()

	assert.Nil(t, err)
	assert.Equal(t, 2, len(cleanNodes))
	assert.Equal(t, nodes[1].Name, cleanNodes[0].Name)
	assert.Equal(t, nodes[2].Name, cleanNodes[1].Name)
}
