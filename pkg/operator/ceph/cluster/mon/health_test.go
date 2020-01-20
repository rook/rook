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
	"fmt"
	"io/ioutil"
	"os"
	"sync"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	clienttest "github.com/rook/rook/pkg/daemon/ceph/client/test"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	testopk8s "github.com/rook/rook/pkg/operator/k8sutil/test"
	"github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	apps "k8s.io/api/apps/v1"
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
	c := New(context, "ns", "", cephv1.NetworkSpec{}, metav1.OwnerReference{}, &sync.Mutex{}, false)
	setCommonMonProperties(c, 1, cephv1.MonSpec{Count: 3, AllowMultiplePerNode: true}, "myversion")
	logger.Infof("initial mons: %v", c.ClusterInfo.Monitors)
	c.waitForStart = false
	defer os.RemoveAll(c.context.ConfigDir)

	c.mapping.Node["f"] = &NodeInfo{
		Name:    "node0",
		Address: "",
	}
	c.maxMonID = 4

	// mock out the scheduler to return node0
	scheduleMonitor = func(c *Cluster, mon *monConfig) (SchedulingResult, error) {
		node, _ := clientset.CoreV1().Nodes().Get("node0", metav1.GetOptions{})
		return SchedulingResult{Node: node}, nil
	}

	err := c.checkHealth()
	assert.Nil(t, err)
	logger.Infof("mons after checkHealth: %v", c.ClusterInfo.Monitors)
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
		_, ok := c.ClusterInfo.Monitors[monName]
		assert.True(t, ok, fmt.Sprintf("mon %s not found in monitor list. %v", monName, c.ClusterInfo.Monitors))
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
	c := New(context, "ns", "", cephv1.NetworkSpec{}, metav1.OwnerReference{}, &sync.Mutex{}, false)
	setCommonMonProperties(c, 2, cephv1.MonSpec{Count: 3, AllowMultiplePerNode: true}, "myversion")
	c.waitForStart = false
	defer os.RemoveAll(c.context.ConfigDir)

	c.mapping.Node["a"] = &NodeInfo{
		Name: "node0",
	}
	c.mapping.Node["b"] = &NodeInfo{
		Name: "node0",
	}
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
	c := New(context, "ns", "", cephv1.NetworkSpec{}, metav1.OwnerReference{}, &sync.Mutex{}, false)
	setCommonMonProperties(c, 0, cephv1.MonSpec{Count: 5, AllowMultiplePerNode: true}, "myversion")
	c.maxMonID = 0 // "a" is max mon id
	c.waitForStart = false
	defer os.RemoveAll(c.context.ConfigDir)

	// checking the health will increase the mons as desired all in one go
	err := c.checkHealth()
	assert.Nil(t, err)
	assert.Equal(t, 5, len(c.ClusterInfo.Monitors), fmt.Sprintf("mons: %v", c.ClusterInfo.Monitors))
	assert.ElementsMatch(t, []string{
		// b is created first, no updates
		"rook-ceph-mon-b",                    // b updated when c created
		"rook-ceph-mon-b", "rook-ceph-mon-c", // b and c updated when d created
		"rook-ceph-mon-b", "rook-ceph-mon-c", "rook-ceph-mon-d", // etc.
		"rook-ceph-mon-b", "rook-ceph-mon-c", "rook-ceph-mon-d", "rook-ceph-mon-e"},
		testopk8s.DeploymentNamesUpdated(deploymentsUpdated))
	testopk8s.ClearDeploymentsUpdated(deploymentsUpdated)

	// reducing the mon count to 3 will reduce the mon count once each time we call checkHealth
	monQuorumResponse = clienttest.MonInQuorumResponseFromMons(c.ClusterInfo.Monitors)
	c.spec.Mon.Count = 3
	err = c.checkHealth()
	assert.Nil(t, err)
	assert.Equal(t, 4, len(c.ClusterInfo.Monitors))
	// No updates in unit tests w/ workaround
	assert.ElementsMatch(t, []string{}, testopk8s.DeploymentNamesUpdated(deploymentsUpdated))
	testopk8s.ClearDeploymentsUpdated(deploymentsUpdated)

	// after the second call we will be down to the expected count of 3
	monQuorumResponse = clienttest.MonInQuorumResponseFromMons(c.ClusterInfo.Monitors)
	err = c.checkHealth()
	assert.Nil(t, err)
	assert.Equal(t, 3, len(c.ClusterInfo.Monitors))
	// No updates in unit tests w/ workaround
	assert.ElementsMatch(t, []string{}, testopk8s.DeploymentNamesUpdated(deploymentsUpdated))
	testopk8s.ClearDeploymentsUpdated(deploymentsUpdated)

	// now attempt to reduce the mons down to quorum size 1
	monQuorumResponse = clienttest.MonInQuorumResponseFromMons(c.ClusterInfo.Monitors)
	c.spec.Mon.Count = 1
	err = c.checkHealth()
	assert.Nil(t, err)
	assert.Equal(t, 2, len(c.ClusterInfo.Monitors))
	// No updates in unit tests w/ workaround
	assert.ElementsMatch(t, []string{}, testopk8s.DeploymentNamesUpdated(deploymentsUpdated))
	testopk8s.ClearDeploymentsUpdated(deploymentsUpdated)

	// cannot reduce from quorum size of 2 to 1
	monQuorumResponse = clienttest.MonInQuorumResponseFromMons(c.ClusterInfo.Monitors)
	err = c.checkHealth()
	assert.Nil(t, err)
	assert.Equal(t, 2, len(c.ClusterInfo.Monitors))
	// No updates in unit tests w/ workaround
	assert.ElementsMatch(t, []string{}, testopk8s.DeploymentNamesUpdated(deploymentsUpdated))
	testopk8s.ClearDeploymentsUpdated(deploymentsUpdated)
}

func TestAddOrRemoveExternalMonitor(t *testing.T) {
	var changed bool
	var err error

	// populate fake monmap
	fakeResp := client.MonStatusResponse{Quorum: []int{0}}

	fakeResp.MonMap.Mons = []client.MonMapEntry{
		{
			Name: "a",
		},
	}
	fakeResp.MonMap.Mons[0].PublicAddr = "172.17.0.4:3300"

	// populate fake ClusterInfo
	c := &Cluster{ClusterInfo: &cephconfig.ClusterInfo{}}
	c.ClusterInfo = test.CreateConfigDir(1)

	//
	// TEST 1
	//
	// both clusterInfo and mon map are identical so nil is expected
	changed, err = c.addOrRemoveExternalMonitor(fakeResp)
	assert.NoError(t, err)
	assert.False(t, changed)
	assert.Equal(t, 1, len(c.ClusterInfo.Monitors))

	//
	// TEST 2
	//
	// Now let's test the case where mon disappeared from the external cluster
	// ClusterInfo still has them but they are gone from the monmap.
	// Thus they should be removed from ClusterInfo
	c.ClusterInfo = test.CreateConfigDir(3)
	changed, err = c.addOrRemoveExternalMonitor(fakeResp)
	assert.NoError(t, err)
	assert.True(t, changed)
	// ClusterInfo should shrink to 1
	assert.Equal(t, 1, len(c.ClusterInfo.Monitors))

	//
	// TEST 3
	//
	// Now let's add a new mon in the external cluster
	// ClusterInfo should be updated with this new monitor
	fakeResp.MonMap.Mons = []client.MonMapEntry{
		{
			Name: "a",
		},
		{
			Name: "b",
		},
	}
	fakeResp.MonMap.Mons[1].PublicAddr = "172.17.0.5:3300"
	c.ClusterInfo = test.CreateConfigDir(1)
	changed, err = c.addOrRemoveExternalMonitor(fakeResp)
	assert.NoError(t, err)
	assert.True(t, changed)
	// ClusterInfo should now have 2 monitors
	assert.Equal(t, 2, len(c.ClusterInfo.Monitors))
}
