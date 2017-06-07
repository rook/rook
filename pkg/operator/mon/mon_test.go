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
	"testing"

	"os"

	"github.com/rook/rook/pkg/cephmgr/client"
	testclient "github.com/rook/rook/pkg/cephmgr/client/test"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/pkg/api/v1"
)

func TestStartMonPods(t *testing.T) {
	clientset := test.New(3)
	factory := &testclient.MockConnectionFactory{Fsid: "fsid", SecretKey: "mysecret"}
	c := New(&k8sutil.Context{Clientset: clientset, Factory: factory, MaxRetries: 1}, "myname", "ns", "", "myversion")

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

	s, err = c.context.Clientset.CoreV1().Secrets(c.Namespace).Get("mon", metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, 4, len(s.StringData))

	// there is only one pod created. the other two won't be created since the first one doesn't start
	p, err := c.context.Clientset.Extensions().ReplicaSets(c.Namespace).Get("mon0", metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, "mon0", p.Name)
}

func TestSaveMonEndpoints(t *testing.T) {
	clientset := test.New(1)
	c := New(&k8sutil.Context{Clientset: clientset}, "myname", "ns", "", "myversion")
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
	clientset := test.New(1)
	factory := &testclient.MockConnectionFactory{Fsid: "fsid", SecretKey: "mysecret"}
	c := New(&k8sutil.Context{Clientset: clientset, Factory: factory, RetryDelay: 1, MaxRetries: 1}, "myname", "ns", "", "myversion")
	c.clusterInfo = test.CreateClusterInfo(1)
	c.configDir = "/tmp/healthtest"
	c.waitForStart = false
	defer os.RemoveAll(c.configDir)

	err := c.CheckHealth()
	assert.Nil(t, err)

	c.maxMonID = 10
	conn, err := factory.NewConnWithClusterAndUser(c.Namespace, "admin")
	defer conn.Shutdown()
	err = c.failoverMon(conn, "mon1")
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
	c := New(&k8sutil.Context{Clientset: clientset}, "myname", "ns", "", "myversion")
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
	c := New(&k8sutil.Context{Clientset: clientset}, "myname", "ns", "", "myversion")
	c.clusterInfo = test.CreateClusterInfo(0)

	// all three nodes are available by default
	nodes, err := c.getAvailableMonNodes()
	assert.Nil(t, err)
	assert.Equal(t, 3, len(nodes))

	// start pods on two of the nodes so that only one node will be available
	for i := 0; i < 2; i++ {
		pod := c.makeMonPod(&MonConfig{Name: fmt.Sprintf("mon%d", i)}, nodes[i].Name)
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
	c := New(&k8sutil.Context{Clientset: clientset}, "myname", "ns", "", "myversion")
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
	nodes[2].Spec.Taints = []v1.Taint{
		v1.Taint{Effect: v1.TaintEffectNoExecute},
	}
	clientset.CoreV1().Nodes().Update(&nodes[0])
	clientset.CoreV1().Nodes().Update(&nodes[1])
	clientset.CoreV1().Nodes().Update(&nodes[2])
	assert.Nil(t, err)
	cleanNodes, err := c.getAvailableMonNodes()
	assert.Nil(t, err)
	assert.Equal(t, 1, len(cleanNodes))
	assert.Equal(t, nodes[2].Name, cleanNodes[0].Name)
}
