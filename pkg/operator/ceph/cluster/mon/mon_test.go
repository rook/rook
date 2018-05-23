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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path"
	"strconv"
	"strings"
	"testing"
	"time"

	"os"

	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	cephmon "github.com/rook/rook/pkg/daemon/ceph/mon"
	cephtest "github.com/rook/rook/pkg/daemon/ceph/test"
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
				cephtest.CreateConfigDir(path.Join(configDir, namespace))
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

func newCluster(context *clusterd.Context, namespace string, hostNetwork bool, resources v1.ResourceRequirements) *Cluster {
	return &Cluster{
		HostNetwork:         hostNetwork,
		context:             context,
		Namespace:           namespace,
		Version:             "myversion",
		Size:                3,
		maxMonID:            -1,
		waitForStart:        false,
		monPodRetryInterval: 10 * time.Millisecond,
		monPodTimeout:       1 * time.Second,
		monTimeoutList:      map[string]time.Time{},
		mapping: &Mapping{
			Node: map[string]*NodeInfo{},
			Port: map[string]int32{},
		},
		resources: resources,
		ownerRef:  metav1.OwnerReference{},
	}
}

func TestStartMonPods(t *testing.T) {
	namespace := "ns"
	context := newTestStartCluster(namespace)
	c := newCluster(context, namespace, false, v1.ResourceRequirements{})

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
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(debug bool, actionName string, command string, arg ...string) (string, error) {
			if strings.Contains(command, "ceph-authtool") {
				cephtest.CreateConfigDir(path.Join(context.ConfigDir, namespace))
			}
			return "", nil
		},
		MockExecuteCommandWithOutputFile: func(debug bool, actionName string, command string, outFileArg string, args ...string) (string, error) {
			cephtest.CreateConfigDir(path.Join(context.ConfigDir, namespace))
			resp := client.MonStatusResponse{Quorum: []int{0}}
			resp.MonMap.Mons = []client.MonMapEntry{
				{
					Name:    "rook-ceph-mon0",
					Rank:    0,
					Address: "0.0.0.0",
				},
				{
					Name:    "rook-ceph-mon1",
					Rank:    0,
					Address: "0.0.0.0",
				},
				{
					Name:    "rook-ceph-mon2",
					Rank:    0,
					Address: "0.0.0.0",
				},
			}
			serialized, _ := json.Marshal(resp)
			return string(serialized), nil
		},
	}
	context.Executor = executor
	c := newCluster(context, namespace, false, v1.ResourceRequirements{})
	c.clusterInfo = test.CreateConfigDir(1)

	// start a basic cluster
	err := c.Start()
	assert.Nil(t, err)

	validateStart(t, c)

	c = newCluster(context, namespace, false, v1.ResourceRequirements{})

	// starting again should be a no-op, but will not result in an error
	err = c.Start()
	assert.Nil(t, err)

	validateStart(t, c)
}

// safety check that if hostNetwork is used no changes occur on an operator restart
func TestOperatorRestartHostNetwork(t *testing.T) {
	namespace := "ns"
	context := newTestStartCluster(namespace)
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(debug bool, actionName string, command string, arg ...string) (string, error) {
			if strings.Contains(command, "ceph-authtool") {
				cephtest.CreateConfigDir(path.Join(context.ConfigDir, namespace))
			}
			return "", nil
		},
		MockExecuteCommandWithOutputFile: func(debug bool, actionName string, command string, outFileArg string, args ...string) (string, error) {
			cephtest.CreateConfigDir(path.Join(context.ConfigDir, namespace))
			resp := client.MonStatusResponse{Quorum: []int{0}}
			resp.MonMap.Mons = []client.MonMapEntry{
				{
					Name:    "rook-ceph-mon0",
					Rank:    0,
					Address: "0.0.0.0",
				},
				{
					Name:    "rook-ceph-mon1",
					Rank:    0,
					Address: "0.0.0.0",
				},
				{
					Name:    "rook-ceph-mon2",
					Rank:    0,
					Address: "0.0.0.0",
				},
			}
			serialized, _ := json.Marshal(resp)
			return string(serialized), nil
		},
	}
	context.Executor = executor
	c := newCluster(context, namespace, false, v1.ResourceRequirements{})
	c.clusterInfo = test.CreateConfigDir(1)

	// start a basic cluster
	err := c.Start()
	assert.Nil(t, err)

	validateStart(t, c)

	c = newCluster(context, namespace, true, v1.ResourceRequirements{})

	// starting again should be a no-op, but still results in an error
	err = c.Start()
	assert.Nil(t, err)

	validateStart(t, c)
}

func validateStart(t *testing.T, c *Cluster) {
	s, err := c.context.Clientset.CoreV1().Secrets(c.Namespace).Get(appName, metav1.GetOptions{})
	assert.Nil(t, err) // there shouldn't be an error due the secret existing
	assert.Equal(t, 4, len(s.StringData))

	// there is only one pod created. the other two won't be created since the first one doesn't start
	_, err = c.context.Clientset.Extensions().ReplicaSets(c.Namespace).Get("rook-ceph-mon0", metav1.GetOptions{})
	assert.Nil(t, err)
}

func TestSaveMonEndpoints(t *testing.T) {
	clientset := test.New(1)
	configDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(configDir)
	c := New(&clusterd.Context{Clientset: clientset, ConfigDir: configDir}, "ns", "", "myversion", 3, rookalpha.Placement{}, false, v1.ResourceRequirements{}, metav1.OwnerReference{})
	c.clusterInfo = test.CreateConfigDir(1)

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
	c.mapping.Node["mon1"] = &NodeInfo{
		Name:     "node0",
		Address:  "1.1.1.1",
		Hostname: "myhost",
	}
	c.mapping.Port["node0"] = int32(12345)
	err = c.saveMonConfig()
	assert.Nil(t, err)

	cm, err = c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Get(EndpointConfigMapName, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, "mon1=2.3.4.5:6790", cm.Data[EndpointDataKey])
	assert.Equal(t, `{"node":{"mon1":{"Name":"node0","Hostname":"myhost","Address":"1.1.1.1"}},"port":{"node0":12345}}`, cm.Data[MappingKey])
	assert.Equal(t, "2", cm.Data[MaxMonIDKey])
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
	c := New(&clusterd.Context{Clientset: clientset}, "ns", "", "myversion", 3, rookalpha.Placement{}, false, v1.ResourceRequirements{}, metav1.OwnerReference{})
	c.clusterInfo = test.CreateConfigDir(0)
	nodes, err := c.getMonNodes()
	assert.Nil(t, err)
	assert.Equal(t, 1, len(nodes))

	conditions := v1.NodeCondition{Type: v1.NodeOutOfDisk}
	nodes[0].Status = v1.NodeStatus{Conditions: []v1.NodeCondition{conditions}}
	clientset.CoreV1().Nodes().Update(&nodes[0])

	emptyNodes, err := c.getMonNodes()
	assert.NotNil(t, err)
	assert.Nil(t, emptyNodes)
}

func TestAvailableNodesInUse(t *testing.T) {
	clientset := test.New(3)
	c := New(&clusterd.Context{Clientset: clientset}, "ns", "", "myversion", 3, rookalpha.Placement{}, false, v1.ResourceRequirements{}, metav1.OwnerReference{})
	c.clusterInfo = test.CreateConfigDir(0)

	// all three nodes are available by default
	nodes, err := c.getMonNodes()
	assert.Nil(t, err)
	assert.Equal(t, 3, len(nodes))

	// start pods on two of the nodes so that only one node will be available
	for i := 0; i < 2; i++ {
		pod := c.makeMonPod(&monConfig{Name: fmt.Sprintf("rook-ceph-mon%d", i)}, nodes[i].Name)
		_, err = clientset.CoreV1().Pods(c.Namespace).Create(pod)
		assert.Nil(t, err)
	}
	reducedNodes, err := c.getMonNodes()
	assert.Nil(t, err)
	assert.Equal(t, 1, len(reducedNodes))
	assert.Equal(t, nodes[2].Name, reducedNodes[0].Name)

	// start pods on the remaining node. We expect all nodes to be available for placement
	// since there is no way to place a mon on an unused node.
	pod := c.makeMonPod(&monConfig{Name: "mon2"}, nodes[2].Name)
	_, err = clientset.CoreV1().Pods(c.Namespace).Create(pod)
	assert.Nil(t, err)
	nodes, err = c.getMonNodes()
	assert.Nil(t, err)
	assert.Equal(t, 3, len(nodes))
}

func TestTaintedNodes(t *testing.T) {
	clientset := test.New(3)
	c := New(&clusterd.Context{Clientset: clientset}, "ns", "", "myversion", 3, rookalpha.Placement{}, false, v1.ResourceRequirements{}, metav1.OwnerReference{})
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
	c := New(&clusterd.Context{Clientset: clientset}, "ns", "", "myversion", 3, rookalpha.Placement{}, false, v1.ResourceRequirements{}, metav1.OwnerReference{})
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

func TestHostNetwork(t *testing.T) {
	clientset := test.New(3)
	c := New(&clusterd.Context{Clientset: clientset}, "ns", "", "myversion", 3, rookalpha.Placement{}, false, v1.ResourceRequirements{}, metav1.OwnerReference{})
	c.clusterInfo = test.CreateConfigDir(0)

	c.HostNetwork = true

	nodes, err := c.getMonNodes()
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

	c := New(&clusterd.Context{Clientset: clientset}, "ns", "", "myversion", 3, rookalpha.Placement{}, true, v1.ResourceRequirements{}, metav1.OwnerReference{})
	c.clusterInfo = test.CreateConfigDir(0)

	var info *NodeInfo
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
	}, "ns", "", "myversion", 3, rookalpha.Placement{}, true, v1.ResourceRequirements{}, metav1.OwnerReference{})
	c.clusterInfo = test.CreateConfigDir(0)

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
