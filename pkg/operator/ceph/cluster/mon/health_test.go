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
package mon

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"testing"

	"os"

	cephv1beta1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1beta1"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	clienttest "github.com/rook/rook/pkg/daemon/ceph/client/test"
	cephmon "github.com/rook/rook/pkg/daemon/ceph/mon"
	"github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/kubelet/apis"
)

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
	c := New(context, "ns", "", "myversion", cephv1beta1.MonSpec{Count: 3, AllowMultiplePerNode: true},
		rookalpha.Placement{}, false, v1.ResourceRequirements{}, metav1.OwnerReference{})
	c.clusterInfo = test.CreateConfigDir(1)
	logger.Infof("initial mons: %v", c.clusterInfo.Monitors)
	c.waitForStart = false
	defer os.RemoveAll(c.context.ConfigDir)

	c.mapping.Node["rook-ceph-mon-f"] = &NodeInfo{
		Name:    "node0",
		Address: "",
	}
	c.mapping.Port["node0"] = cephmon.DefaultPort
	c.maxMonID = 4

	err := c.checkHealth()
	assert.Nil(t, err)
	logger.Infof("mons after checkHealth: %v", c.clusterInfo.Monitors)

	err = c.failoverMon("f")
	assert.Nil(t, err)

	newMons := []string{
		"rook-ceph-mon-g",
	}
	for _, monName := range newMons {
		_, ok := c.clusterInfo.Monitors[monName]
		assert.True(t, ok, fmt.Sprintf("mon %s not found in monitor list. %v", monName, c.clusterInfo.Monitors))
	}
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
	c := New(context, "ns", "", "myversion", cephv1beta1.MonSpec{Count: 3, AllowMultiplePerNode: true},
		rookalpha.Placement{}, false, v1.ResourceRequirements{}, metav1.OwnerReference{})
	c.clusterInfo = test.CreateConfigDir(2)
	c.waitForStart = false
	defer os.RemoveAll(c.context.ConfigDir)

	c.mapping.Node["rook-ceph-mon-a"] = &NodeInfo{
		Name: "node0",
	}
	c.mapping.Node["rook-ceph-mon-b"] = &NodeInfo{
		Name: "node0",
	}
	c.mapping.Port["node0"] = cephmon.DefaultPort
	c.maxMonID = 4

	c.saveMonConfig()

	// Check if the two mons are found in the configmap
	cm, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Get(EndpointConfigMapName, metav1.GetOptions{})
	assert.Nil(t, err)
	if cm.Data[EndpointDataKey] != "a=1.2.3.1:6790,b=1.2.3.2:6790" {
		assert.Equal(t, "b=1.2.3.2:6790,a=1.2.3.1:6790", cm.Data[EndpointDataKey])
	}

	// Because the mon a isn't in the MonInQuorumResponse() this will create a new mon
	delete(c.mapping.Node, "rook-ceph-mon-a")
	err = c.checkHealth()
	assert.Nil(t, err)

	// recheck that the "not found" mon has been replaced with a new one
	cm, err = c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Get(EndpointConfigMapName, metav1.GetOptions{})
	assert.Nil(t, err)
	if cm.Data[EndpointDataKey] != "b=1.2.3.2:6790,f=:6790" {
		assert.Equal(t, "f=:6790,b=1.2.3.2:6790", cm.Data[EndpointDataKey])
	}
}

func TestCheckHealthTwoMonsOneNode(t *testing.T) {
	executorNextMons := false
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(debug bool, actionName string, command string, outFileArg string, args ...string) (string, error) {
			if executorNextMons {
				resp := client.MonStatusResponse{Quorum: []int{0}}
				resp.MonMap.Mons = []client.MonMapEntry{
					{
						Name:    "a",
						Rank:    0,
						Address: "1.2.3.4",
					},
					{
						Name:    "c",
						Rank:    0,
						Address: "1.2.3.4",
					},
				}
				serialized, _ := json.Marshal(resp)
				return string(serialized), nil
			}
			return clienttest.MonInQuorumResponseMany(2), nil
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
	c := New(context, "ns", "", "myversion", cephv1beta1.MonSpec{Count: 3, AllowMultiplePerNode: true},
		rookalpha.Placement{}, false, v1.ResourceRequirements{}, metav1.OwnerReference{})
	c.clusterInfo = test.CreateConfigDir(2)
	c.waitForStart = false
	defer os.RemoveAll(c.context.ConfigDir)

	// add two mons to the mapping on node0
	c.mapping.Node["a"] = &NodeInfo{
		Name:     "node0",
		Hostname: "mynode0",
		Address:  "0.0.0.0",
	}
	c.mapping.Node["b"] = &NodeInfo{
		Name:     "node0",
		Hostname: "mynode0",
		Address:  "0.0.0.0",
	}
	c.maxMonID = 1
	c.saveMonConfig()

	monNames := []string{"a", "b"}
	for i := 0; i < len(monNames); i++ {
		prefix := appName + "-"
		name := monNames[i]
		rs := c.makeReplicaSet(&monConfig{Name: prefix + name, DaemonName: name}, "node0")
		_, err := clientset.ExtensionsV1beta1().ReplicaSets(c.Namespace).Create(rs)
		assert.Nil(t, err)
		po := c.makeMonPod(&monConfig{Name: prefix + name, DaemonName: name}, "node0")
		_, err = clientset.CoreV1().Pods(c.Namespace).Create(po)
		assert.Nil(t, err)
	}

	// initial health check should already see that there is more than one mon on one node (node1)
	_, err := c.checkMonsOnSameNode()
	assert.Nil(t, err)
	assert.Equal(t, "node0", c.mapping.Node["a"].Name)
	assert.Equal(t, "node0", c.mapping.Node["b"].Name)
	assert.Equal(t, "mynode0", c.mapping.Node["a"].Hostname)
	assert.Equal(t, "mynode0", c.mapping.Node["b"].Hostname)

	// add new node and check if the second mon gets failovered to it
	n := &v1.Node{
		Status: v1.NodeStatus{
			Conditions: []v1.NodeCondition{
				{
					Type: v1.NodeReady,
				},
			},
			Addresses: []v1.NodeAddress{
				{
					Type:    v1.NodeExternalIP,
					Address: "2.2.2.2",
				},
			},
		},
	}
	n.Name = "node2"
	clientset.CoreV1().Nodes().Create(n)

	_, err = c.checkMonsOnSameNode()
	assert.Nil(t, err)

	// check that mon rook-ceph-mon-c exists
	logger.Infof("mapping: %+v", c.mapping.Node)
	assert.NotNil(t, c.mapping.Node["rook-ceph-mon-c"])
	assert.Equal(t, "node2", c.mapping.Node["rook-ceph-mon-c"].Name)

	// check if mon2 has been deleted
	var rsses *v1beta1.ReplicaSetList
	rsses, err = clientset.ExtensionsV1beta1().ReplicaSets(c.Namespace).List(metav1.ListOptions{})
	assert.Nil(t, err)

	deleted := false
	for _, rs := range rsses.Items {
		if rs.Name == "rook-ceph-mon-a" || rs.Name == "rook-ceph-mon-c" {
			deleted = true
		} else {
			deleted = false
		}
	}
	assert.Equal(t, true, deleted, "rook-ceph-mon-b not failovered/deleted after health check")

	// enable different ceph mon map output
	executorNextMons = true
	_, err = c.checkMonsOnSameNode()
	assert.Nil(t, err)

	// check that nothing has changed
	rsses, err = clientset.ExtensionsV1beta1().ReplicaSets(c.Namespace).List(metav1.ListOptions{})
	assert.Nil(t, err)

	for _, rs := range rsses.Items {
		// both mons should always be on the same node as in this test due to the order
		//the mons are processed in the loop
		if (rs.Name == "rook-ceph-mon-a" && rs.Spec.Template.Spec.NodeSelector[apis.LabelHostname] == "node1") || (rs.Name != "rook-ceph-mon-c" && rs.Spec.Template.Spec.NodeSelector[apis.LabelHostname] == "node2") {
			assert.Fail(t, fmt.Sprintf("mon %s shouldn't exist", rs.Name))
		}
	}
}

func TestCheckMonsValid(t *testing.T) {
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
	c := New(context, "ns", "", "myversion", cephv1beta1.MonSpec{Count: 3, AllowMultiplePerNode: true},
		rookalpha.Placement{}, false, v1.ResourceRequirements{}, metav1.OwnerReference{})
	c.clusterInfo = test.CreateConfigDir(1)
	c.waitForStart = false
	defer os.RemoveAll(c.context.ConfigDir)

	// add two mons to the mapping on node0
	c.mapping.Node["rook-ceph-mon-a"] = &NodeInfo{
		Name:    "node0",
		Address: "0.0.0.0",
	}
	c.mapping.Node["rook-ceph-mon-b"] = &NodeInfo{
		Name:    "node1",
		Address: "0.0.0.0",
	}
	c.maxMonID = 1

	// add three nodes
	for i := 0; i < 3; i++ {
		n := &v1.Node{
			Status: v1.NodeStatus{
				Conditions: []v1.NodeCondition{
					{
						Type: v1.NodeReady,
					},
				},
				Addresses: []v1.NodeAddress{
					{
						Type:    v1.NodeExternalIP,
						Address: "0.0.0.0",
					},
				},
			},
		}
		n.Name = fmt.Sprintf("node%d", i)
		clientset.CoreV1().Nodes().Create(n)
	}

	_, err := c.checkMonsOnValidNodes()
	assert.Nil(t, err)
	assert.Equal(t, "node0", c.mapping.Node["rook-ceph-mon-a"].Name)
	assert.Equal(t, "node1", c.mapping.Node["rook-ceph-mon-b"].Name)

	// set node1 unschedulable and check that mon2 gets failovered to be mon3 to node2
	node0, err := c.context.Clientset.CoreV1().Nodes().Get("node0", metav1.GetOptions{})
	assert.Nil(t, err)
	node0.Spec.Unschedulable = true
	_, err = c.context.Clientset.CoreV1().Nodes().Update(node0)
	assert.Nil(t, err)

	// add the pods so the getNodesInUse() works correctly
	for i := 1; i <= 2; i++ {
		name := fmt.Sprintf("mon%d", i)
		po := c.makeMonPod(&monConfig{Name: name, DaemonName: name}, fmt.Sprintf("node%d", i-1))
		_, err = clientset.CoreV1().Pods(c.Namespace).Create(po)
		assert.Nil(t, err)
	}

	_, err = c.checkMonsOnValidNodes()
	assert.Nil(t, err)

	assert.Len(t, c.mapping.Node, 2)
	logger.Infof("mapping: %+v", c.mapping)
	assert.Nil(t, c.mapping.Node["rook-ceph-mon-a"])
	// the new mon should always be on the empty node2
	// the failovered mon's name is "rook-ceph-mon-a"
	assert.Equal(t, "node2", c.mapping.Node["rook-ceph-mon-c"].Name)
	assert.Equal(t, "node1", c.mapping.Node["rook-ceph-mon-b"].Name)
}

func TestCheckLessMonsStartNewMons(t *testing.T) {
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
	c := New(context, "ns", "", "myversion", cephv1beta1.MonSpec{Count: 5, AllowMultiplePerNode: true},
		rookalpha.Placement{}, false, v1.ResourceRequirements{}, metav1.OwnerReference{})
	c.maxMonID = 0
	c.clusterInfo = test.CreateConfigDir(0)
	c.waitForStart = false
	defer os.RemoveAll(c.context.ConfigDir)

	err := c.checkHealth()
	assert.Nil(t, err)
	assert.Equal(t, 5, len(c.clusterInfo.Monitors), fmt.Sprintf("mons: %v", c.clusterInfo.Monitors))
}
