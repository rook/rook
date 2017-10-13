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
	"strconv"
	"strings"
	"testing"
	"time"

	"os"

	"github.com/rook/rook/pkg/ceph/client"
	clienttest "github.com/rook/rook/pkg/ceph/client/test"
	cephmon "github.com/rook/rook/pkg/ceph/mon"
	cephtest "github.com/rook/rook/pkg/ceph/test"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newTestStartCluster(namespace string) *clusterd.Context {
	clientset := test.New(3)
	configDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(configDir)
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(debug bool, actionName string, command string, args ...string) (string, error) {
			if strings.Contains(command, "ceph-authtool") {
				cephtest.CreateClusterInfo(nil, path.Join(configDir, namespace), nil)
			}
			return "", nil
		},
	}
	return &clusterd.Context{
		Clientset: clientset,
		Executor:  executor,
		ConfigDir: configDir,
	}
}

func newCluster(context *clusterd.Context, namespace string, hostNetwork bool) *Cluster {
	return &Cluster{
		HostNetwork:         true,
		context:             context,
		Namespace:           namespace,
		Version:             "myversion",
		Size:                3,
		maxMonID:            -1,
		waitForStart:        false,
		monPodRetryInterval: 10 * time.Millisecond,
		monPodTimeout:       1 * time.Second,
		monTimeoutList:      map[string]time.Time{},
		mapping: &mapping{
			Node: map[string]*nodeInfo{},
			Port: map[string]int32{},
		},
	}
}

func TestStartMonPods(t *testing.T) {
	namespace := "ns"
	context := newTestStartCluster(namespace)
	c := newCluster(context, namespace, false)

	// start a basic cluster
	err := c.Start()
	assert.Nil(t, err)

	validateStart(t, c)

	// starting again should be a no-op, but still results in an error
	err = c.Start()
	assert.Nil(t, err)

	validateStart(t, c)
}

func TestOperatorRestart(t *testing.T) {
	namespace := "ns"
	context := newTestStartCluster(namespace)
	c := newCluster(context, namespace, false)

	// start a basic cluster
	err := c.Start()
	assert.Nil(t, err)

	validateStart(t, c)

	c = newCluster(context, namespace, false)

	// starting again should be a no-op, but still results in an error
	err = c.Start()
	assert.Nil(t, err)

	validateStart(t, c)
}

// safety check that if hostNetwork is used no changes occur on an operator restart
func TestOperatorRestartHostNetwork(t *testing.T) {
	namespace := "ns"
	context := newTestStartCluster(namespace)
	c := newCluster(context, namespace, true)

	// start a basic cluster
	err := c.Start()
	assert.Nil(t, err)

	validateStart(t, c)

	c = newCluster(context, namespace, true)

	// starting again should be a no-op, but still results in an error
	err = c.Start()
	assert.Nil(t, err)

	validateStart(t, c)
}

func validateStart(t *testing.T, c *Cluster) {
	s, err := c.context.Clientset.CoreV1().Secrets(c.Namespace).Get(appName, metav1.GetOptions{})
	assert.Equal(t, 4, len(s.StringData))

	// there is only one pod created. the other two won't be created since the first one doesn't start
	_, err = c.context.Clientset.Extensions().ReplicaSets(c.Namespace).Get("rook-ceph-mon0", metav1.GetOptions{})
	assert.Nil(t, err)
}

func TestSaveMonEndpoints(t *testing.T) {
	clientset := test.New(1)
	configDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(configDir)
	c := New(&clusterd.Context{Clientset: clientset, ConfigDir: configDir}, "ns", "", "myversion", k8sutil.Placement{}, false)
	c.clusterInfo = test.CreateClusterInfo(1)

	// create the initial config map
	err := c.saveMonConfig()
	assert.Nil(t, err)

	cm, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Get(EndpointConfigMapName, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, "mon1=1.2.3.1:6790", cm.Data[EndpointDataKey])
	assert.Equal(t, `{"node":{},"port":{}}`, cm.Data[MappingKey])
	assert.Equal(t, "-1", cm.Data[MaxMonIDKey])

	// update the config map
	c.clusterInfo.Monitors["mon1"].Endpoint = "2.3.4.5:6790"
	c.maxMonID = 2
	c.mapping.Node["mon1"] = &nodeInfo{
		Name:    "node0",
		Address: "1.1.1.1",
	}
	c.mapping.Port["node0"] = int32(12345)
	err = c.saveMonConfig()
	assert.Nil(t, err)

	cm, err = c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Get(EndpointConfigMapName, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, "mon1=2.3.4.5:6790", cm.Data[EndpointDataKey])
	assert.Equal(t, `{"node":{"mon1":{"Name":"node0","Address":"1.1.1.1"}},"port":{"node0":12345}}`, cm.Data[MappingKey])
	assert.Equal(t, "2", cm.Data[MaxMonIDKey])
}

func TestCheckHealth(t *testing.T) {
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(debug bool, actionName string, command string, outFileArg string, args ...string) (string, error) {
			return clienttest.MonInQuorumResponse(), nil
		},
	}
	clientset := test.New(1)
	configDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(configDir)
	context := &clusterd.Context{
		Clientset: clientset,
		ConfigDir: configDir,
		Executor:  executor,
	}
	c := New(context, "ns", "", "myversion", k8sutil.Placement{}, false)
	c.Size = 1
	c.clusterInfo = test.CreateClusterInfo(1)
	c.waitForStart = false
	defer os.RemoveAll(c.context.ConfigDir)

	c.mapping.Node["mon1"] = &nodeInfo{
		Name:    "node0",
		Address: "",
	}
	c.mapping.Port["node0"] = cephmon.DefaultPort
	c.maxMonID = 10

	err := c.checkHealth()
	assert.Nil(t, err)

	err = c.failoverMon("mon1")
	assert.Nil(t, err)

	cm, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Get(EndpointConfigMapName, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, "rook-ceph-mon11=:6790", cm.Data[EndpointDataKey])
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
	id, err = getMonID("rook-ceph-monitor0")
	assert.NotNil(t, err)
	assert.Equal(t, -1, id)

	// valid
	id, err = getMonID("rook-ceph-mon0")
	assert.Nil(t, err)
	assert.Equal(t, 0, id)
	id, err = getMonID("rook-ceph-mon123")
	assert.Nil(t, err)
	assert.Equal(t, 123, id)
}

func TestAvailableMonNodes(t *testing.T) {
	clientset := test.New(1)
	c := New(&clusterd.Context{Clientset: clientset}, "ns", "", "myversion", k8sutil.Placement{}, false)
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
	c := New(&clusterd.Context{Clientset: clientset}, "ns", "", "myversion", k8sutil.Placement{}, false)
	c.clusterInfo = test.CreateClusterInfo(0)

	// all three nodes are available by default
	nodes, err := c.getAvailableMonNodes()
	assert.Nil(t, err)
	assert.Equal(t, 3, len(nodes))

	// start pods on two of the nodes so that only one node will be available
	for i := 0; i < 2; i++ {
		pod := c.makeMonPod(&monConfig{Name: fmt.Sprintf("rook-ceph-mon%d", i)}, nodes[i].Name)
		_, err = clientset.CoreV1().Pods(c.Namespace).Create(pod)
		assert.Nil(t, err)
	}
	reducedNodes, err := c.getAvailableMonNodes()
	assert.Nil(t, err)
	assert.Equal(t, 1, len(reducedNodes))
	assert.Equal(t, nodes[2].Name, reducedNodes[0].Name)

	// start pods on the remaining node. We expect all nodes to be available for placement
	// since there is no way to place a mon on an unused node.
	pod := c.makeMonPod(&monConfig{Name: "mon2"}, nodes[2].Name)
	_, err = clientset.CoreV1().Pods(c.Namespace).Create(pod)
	assert.Nil(t, err)
	nodes, err = c.getAvailableMonNodes()
	assert.Nil(t, err)
	assert.Equal(t, 3, len(nodes))
}

func TestTaintedNodes(t *testing.T) {
	clientset := test.New(3)
	c := New(&clusterd.Context{Clientset: clientset}, "ns", "", "myversion", k8sutil.Placement{}, false)
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
		{Effect: v1.TaintEffectNoSchedule},
	}
	nodes[1].Spec.Taints = []v1.Taint{
		{Effect: v1.TaintEffectPreferNoSchedule},
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
	c := New(&clusterd.Context{Clientset: clientset}, "ns", "", "myversion", k8sutil.Placement{}, false)
	c.clusterInfo = test.CreateClusterInfo(0)

	nodes, err := c.getAvailableMonNodes()
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
	cleanNodes, err := c.getAvailableMonNodes()

	assert.Nil(t, err)
	assert.Equal(t, 2, len(cleanNodes))
	assert.Equal(t, nodes[1].Name, cleanNodes[0].Name)
	assert.Equal(t, nodes[2].Name, cleanNodes[1].Name)
}

func TestHostNetwork(t *testing.T) {
	clientset := test.New(3)
	c := New(&clusterd.Context{Clientset: clientset}, "ns", "", "myversion", k8sutil.Placement{}, false)
	c.clusterInfo = test.CreateClusterInfo(0)

	c.HostNetwork = true

	nodes, err := c.getAvailableMonNodes()
	assert.Nil(t, err)
	assert.Equal(t, 3, len(nodes))

	pod := c.makeMonPod(&monConfig{Name: "mon2"}, nodes[2].Name)
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

	c := New(&clusterd.Context{Clientset: clientset}, "ns", "", "myversion", k8sutil.Placement{}, true)
	c.clusterInfo = test.CreateClusterInfo(0)

	var info *nodeInfo
	info, err = getNodeInfoFromNode(*node)
	assert.Nil(t, err)

	assert.Equal(t, "1.1.1.1", info.Address)
}

func TestHostNetworkPortIncrease(t *testing.T) {
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

	configDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(configDir)
	c := New(&clusterd.Context{
		Clientset: clientset,
		ConfigDir: configDir,
	}, "ns", "", "myversion", k8sutil.Placement{}, true)
	c.clusterInfo = test.CreateClusterInfo(0)

	mons := []*monConfig{
		{
			Name: "mon1",
			Port: cephmon.DefaultPort,
		},
		{
			Name: "mon2",
			Port: cephmon.DefaultPort,
		},
	}

	err = c.assignMons(mons)
	assert.Nil(t, err)

	err = c.initMonIPs(mons)
	assert.Nil(t, err)

	assert.Equal(t, node.Name, c.mapping.Node["mon1"].Name)
	assert.Equal(t, node.Name, c.mapping.Node["mon2"].Name)

	sEndpoint := strings.Split(c.clusterInfo.Monitors["mon1"].Endpoint, ":")
	assert.Equal(t, strconv.Itoa(cephmon.DefaultPort), sEndpoint[1])
	sEndpoint = strings.Split(c.clusterInfo.Monitors["mon2"].Endpoint, ":")
	assert.Equal(t, strconv.Itoa(cephmon.DefaultPort+1), sEndpoint[1])
}

func TestCheckHealthNotFound(t *testing.T) {
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(debug bool, actionName string, command string, outFileArg string, args ...string) (string, error) {
			return clienttest.MonInQuorumResponse(), nil
		},
	}
	clientset := test.New(1)
	configDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(configDir)
	context := &clusterd.Context{
		Clientset: clientset,
		ConfigDir: configDir,
		Executor:  executor,
	}
	c := New(context, "ns", "", "myversion", k8sutil.Placement{}, false)
	c.Size = 2
	c.clusterInfo = test.CreateClusterInfo(2)
	c.waitForStart = false
	defer os.RemoveAll(c.context.ConfigDir)
	fmt.Printf("TEST: %+v\n", c.clusterInfo)

	c.mapping.Node["mon1"] = &nodeInfo{
		Name:    "node0",
		Address: "",
	}
	c.mapping.Node["mon2"] = &nodeInfo{
		Name:    "node0",
		Address: "",
	}
	c.mapping.Port["node0"] = cephmon.DefaultPort
	c.maxMonID = 10

	c.saveMonConfig()

	// Check if the two mons are found in the configmap
	cm, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Get(EndpointConfigMapName, metav1.GetOptions{})
	assert.Nil(t, err)
	if cm.Data[EndpointDataKey] == "mon1=1.2.3.1:6790,mon2=1.2.3.2:6790" {
		assert.Equal(t, "mon1=1.2.3.1:6790,mon2=1.2.3.2:6790", cm.Data[EndpointDataKey])
	} else {
		assert.Equal(t, "mon2=1.2.3.2:6790,mon1=1.2.3.1:6790", cm.Data[EndpointDataKey])
	}

	// Because mon2 isn't in the MonInQuorumResponse() this will create a mon11
	err = c.checkHealth()
	assert.Nil(t, err)

	// recheck that the "not found" mon has been replaced with a new one
	cm, err = c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Get(EndpointConfigMapName, metav1.GetOptions{})
	assert.Nil(t, err)
	if cm.Data[EndpointDataKey] == "mon1=1.2.3.1:6790,rook-ceph-mon11=:6790" {
		assert.Equal(t, "mon1=1.2.3.1:6790,rook-ceph-mon11=:6790", cm.Data[EndpointDataKey])
	} else {
		assert.Equal(t, "rook-ceph-mon11=:6790,mon1=1.2.3.1:6790", cm.Data[EndpointDataKey])
	}
}
