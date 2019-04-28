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
	"os"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	clienttest "github.com/rook/rook/pkg/daemon/ceph/client/test"
	testopk8s "github.com/rook/rook/pkg/operator/k8sutil/test"
	"github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCheckHealth(t *testing.T) {

	var deploymentsUpdated *[]*apps.Deployment
	updateDeploymentAndWait, deploymentsUpdated = testopk8s.UpdateDeploymentAndWaitStub()

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
	c := New(context, "ns", "", false, metav1.OwnerReference{})
	setCommonMonProperties(c, 1, cephv1.MonSpec{Count: 3, AllowMultiplePerNode: true}, "myversion")
	logger.Infof("initial mons: %v", c.clusterInfo.Monitors)
	c.waitForStart = false
	defer os.RemoveAll(c.context.ConfigDir)

	c.mapping.Node["f"] = &NodeInfo{
		Name:    "node0",
		Address: "",
	}
	c.mapping.Port["node0"] = DefaultMsgr1Port
	c.maxMonID = 4

	err := c.checkHealth()
	assert.Nil(t, err)
	logger.Infof("mons after checkHealth: %v", c.clusterInfo.Monitors)
	assert.ElementsMatch(t, []string{"rook-ceph-mon-a", "rook-ceph-mon-f"}, testopk8s.DeploymentNamesUpdated(deploymentsUpdated))
	testopk8s.ClearDeploymentsUpdated(deploymentsUpdated)

	err = c.failoverMon("f")
	assert.Nil(t, err)
	assert.ElementsMatch(t, []string{}, testopk8s.DeploymentNamesUpdated(deploymentsUpdated))
	testopk8s.ClearDeploymentsUpdated(deploymentsUpdated)

	newMons := []string{
		"g",
	}
	for _, monName := range newMons {
		_, ok := c.clusterInfo.Monitors[monName]
		assert.True(t, ok, fmt.Sprintf("mon %s not found in monitor list. %v", monName, c.clusterInfo.Monitors))
	}
}

func TestCheckHealthNotFound(t *testing.T) {
	var deploymentsUpdated *[]*apps.Deployment
	updateDeploymentAndWait, deploymentsUpdated = testopk8s.UpdateDeploymentAndWaitStub()

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
	c := New(context, "ns", "", false, metav1.OwnerReference{})
	setCommonMonProperties(c, 2, cephv1.MonSpec{Count: 3, AllowMultiplePerNode: true}, "myversion")
	c.waitForStart = false
	defer os.RemoveAll(c.context.ConfigDir)

	c.mapping.Node["a"] = &NodeInfo{
		Name: "node0",
	}
	c.mapping.Node["b"] = &NodeInfo{
		Name: "node0",
	}
	c.mapping.Port["node0"] = DefaultMsgr1Port
	c.maxMonID = 4

	c.saveMonConfig()

	// Check if the two mons are found in the configmap
	cm, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Get(EndpointConfigMapName, metav1.GetOptions{})
	assert.Nil(t, err)
	if cm.Data[EndpointDataKey] != "a=1.2.3.1:6789,b=1.2.3.2:6789" {
		assert.Equal(t, "b=1.2.3.2:6789,a=1.2.3.1:6789", cm.Data[EndpointDataKey])
	}

	// Because the mon a isn't in the MonInQuorumResponse() this will create a new mon
	delete(c.mapping.Node, "b")
	err = c.checkHealth()
	assert.Nil(t, err)
	// No updates in unit tests w/ workaround
	assert.ElementsMatch(t, []string{}, testopk8s.DeploymentNamesUpdated(deploymentsUpdated))
	testopk8s.ClearDeploymentsUpdated(deploymentsUpdated)

	// recheck that the "not found" mon has been replaced with a new one
	cm, err = c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Get(EndpointConfigMapName, metav1.GetOptions{})
	assert.Nil(t, err)
	if cm.Data[EndpointDataKey] != "a=1.2.3.1:6789,f=:6789" {
		assert.Equal(t, "f=:6789,a=1.2.3.1:6789", cm.Data[EndpointDataKey])
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

	c := New(context, "ns", "", false, metav1.OwnerReference{})
	setCommonMonProperties(c, 2, cephv1.MonSpec{Count: 3, AllowMultiplePerNode: true}, "myversion")
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
		monConfig := testGenMonConfig(monNames[i])
		d := c.makeDeployment(monConfig, "node0")
		_, err := clientset.AppsV1().Deployments(c.Namespace).Create(d)
		assert.Nil(t, err)
		po := c.makeMonPod(monConfig, "node0")
		_, err = clientset.CoreV1().Pods(c.Namespace).Create(po)
		assert.Nil(t, err)
	}

	// initial health check should already see that there is more than one mon on one node (node0)
	_, err := c.checkMonsOnSameNode(3)
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
					Type: v1.NodeReady, Status: v1.ConditionTrue,
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

	_, err = c.checkMonsOnSameNode(3)
	assert.Nil(t, err)

	// check that mon c exists
	logger.Infof("mapping: %+v", c.mapping.Node)
	assert.NotNil(t, c.mapping.Node["c"])
	assert.Equal(t, "node2", c.mapping.Node["c"].Name)

	// check if mon b has been deleted
	var dlist *apps.DeploymentList
	dlist, err = clientset.AppsV1().Deployments(c.Namespace).List(metav1.ListOptions{})
	assert.Nil(t, err)
	deleted := true
	for _, d := range dlist.Items {
		if d.Name == "b" {
			deleted = false
		}
	}
	assert.Equal(t, true, deleted, "mon b not failed over or deleted after health check")

	// enable different ceph mon map output
	executorNextMons = true
	_, err = c.checkMonsOnSameNode(3)
	assert.Nil(t, err)

	// check that nothing has changed
	dlist, err = clientset.AppsV1().Deployments(c.Namespace).List(metav1.ListOptions{})
	assert.Nil(t, err)

	for _, d := range dlist.Items {
		// both mons should always be on the same node as in this test due to the order
		// the mons are processed in the loop
		if (d.Name == "rook-ceph-mon-a" && d.Spec.Template.Spec.NodeSelector[v1.LabelHostname] == "node1") || (d.Name != "rook-ceph-mon-c" && d.Spec.Template.Spec.NodeSelector[v1.LabelHostname] == "node2") {
			assert.Fail(t, fmt.Sprintf("mon %s shouldn't exist", d.Name))
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
	c := New(context, "ns", "", false, metav1.OwnerReference{})
	setCommonMonProperties(c, 1, cephv1.MonSpec{Count: 3, AllowMultiplePerNode: true}, "myversion")
	c.waitForStart = false
	defer os.RemoveAll(c.context.ConfigDir)

	// add two mons to the mapping on node0
	c.mapping.Node["a"] = &NodeInfo{
		Name:    "node0",
		Address: "0.0.0.0",
	}
	c.mapping.Node["b"] = &NodeInfo{
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
						Type: v1.NodeReady, Status: v1.ConditionTrue,
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
	assert.Equal(t, "node0", c.mapping.Node["a"].Name)
	assert.Equal(t, "node1", c.mapping.Node["b"].Name)

	// set node1 unschedulable and check that mon2 gets failovered to be mon3 to node2
	node0, err := c.context.Clientset.CoreV1().Nodes().Get("node0", metav1.GetOptions{})
	assert.Nil(t, err)
	node0.Spec.Unschedulable = true
	_, err = c.context.Clientset.CoreV1().Nodes().Update(node0)
	assert.Nil(t, err)

	// add the pods so the getNodesInUse() works correctly
	monNames := []string{"a", "b", "c"}
	for i := 1; i <= 2; i++ { // 1=b, 2=c
		po := c.makeMonPod(testGenMonConfig(monNames[i]), fmt.Sprintf("node%d", i-1))
		_, err = clientset.CoreV1().Pods(c.Namespace).Create(po)
		assert.Nil(t, err)
	}

	_, err = c.checkMonsOnValidNodes()
	assert.Nil(t, err)

	assert.Len(t, c.mapping.Node, 2)
	logger.Infof("mapping: %+v", c.mapping)
	assert.Nil(t, c.mapping.Node["a"])
	// the new mon should always be on the empty node2
	// the failovered mon's name is "rook-ceph-mon-a"
	assert.Equal(t, "node2", c.mapping.Node["c"].Name)
	assert.Equal(t, "node1", c.mapping.Node["b"].Name)
}

func TestAddRemoveMons(t *testing.T) {
	var deploymentsUpdated *[]*apps.Deployment
	updateDeploymentAndWait, deploymentsUpdated = testopk8s.UpdateDeploymentAndWaitStub()

	monQuorumResponse := clienttest.MonInQuorumResponse()
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(debug bool, actionName string, command string, outFileArg string, args ...string) (string, error) {
			return monQuorumResponse, nil
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
	c := New(context, "ns", "", false, metav1.OwnerReference{})
	setCommonMonProperties(c, 0, cephv1.MonSpec{Count: 5, AllowMultiplePerNode: true}, "myversion")
	c.maxMonID = 0 // "a" is max mon id
	c.waitForStart = false
	defer os.RemoveAll(c.context.ConfigDir)

	// checking the health will increase the mons as desired all in one go
	err := c.checkHealth()
	assert.Nil(t, err)
	assert.Equal(t, 5, len(c.clusterInfo.Monitors), fmt.Sprintf("mons: %v", c.clusterInfo.Monitors))
	assert.ElementsMatch(t, []string{
		// b is created first, no updates
		"rook-ceph-mon-b",                    // b updated when c created
		"rook-ceph-mon-b", "rook-ceph-mon-c", // b and c updated when d created
		"rook-ceph-mon-b", "rook-ceph-mon-c", "rook-ceph-mon-d", // etc.
		"rook-ceph-mon-b", "rook-ceph-mon-c", "rook-ceph-mon-d", "rook-ceph-mon-e"},
		testopk8s.DeploymentNamesUpdated(deploymentsUpdated))
	testopk8s.ClearDeploymentsUpdated(deploymentsUpdated)

	// reducing the mon count to 3 will reduce the mon count once each time we call checkHealth
	monQuorumResponse = clienttest.MonInQuorumResponseFromMons(c.clusterInfo.Monitors)
	c.spec.Mon.Count = 3
	err = c.checkHealth()
	assert.Nil(t, err)
	assert.Equal(t, 4, len(c.clusterInfo.Monitors))
	// No updates in unit tests w/ workaround
	assert.ElementsMatch(t, []string{}, testopk8s.DeploymentNamesUpdated(deploymentsUpdated))
	testopk8s.ClearDeploymentsUpdated(deploymentsUpdated)

	// after the second call we will be down to the expected count of 3
	monQuorumResponse = clienttest.MonInQuorumResponseFromMons(c.clusterInfo.Monitors)
	err = c.checkHealth()
	assert.Nil(t, err)
	assert.Equal(t, 3, len(c.clusterInfo.Monitors))
	// No updates in unit tests w/ workaround
	assert.ElementsMatch(t, []string{}, testopk8s.DeploymentNamesUpdated(deploymentsUpdated))
	testopk8s.ClearDeploymentsUpdated(deploymentsUpdated)

	// now attempt to reduce the mons down to quorum size 1
	monQuorumResponse = clienttest.MonInQuorumResponseFromMons(c.clusterInfo.Monitors)
	c.spec.Mon.Count = 1
	err = c.checkHealth()
	assert.Nil(t, err)
	assert.Equal(t, 2, len(c.clusterInfo.Monitors))
	// No updates in unit tests w/ workaround
	assert.ElementsMatch(t, []string{}, testopk8s.DeploymentNamesUpdated(deploymentsUpdated))
	testopk8s.ClearDeploymentsUpdated(deploymentsUpdated)

	// cannot reduce from quorum size of 2 to 1
	monQuorumResponse = clienttest.MonInQuorumResponseFromMons(c.clusterInfo.Monitors)
	err = c.checkHealth()
	assert.Nil(t, err)
	assert.Equal(t, 2, len(c.clusterInfo.Monitors))
	// No updates in unit tests w/ workaround
	assert.ElementsMatch(t, []string{}, testopk8s.DeploymentNamesUpdated(deploymentsUpdated))
	testopk8s.ClearDeploymentsUpdated(deploymentsUpdated)
}
