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
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	clienttest "github.com/rook/rook/pkg/daemon/ceph/client/test"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/k8sutil"
	testopk8s "github.com/rook/rook/pkg/operator/k8sutil/test"
	"github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tevino/abool"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func TestCheckHealth(t *testing.T) {
	ctx := context.TODO()
	var deploymentsUpdated *[]*apps.Deployment
	updateDeploymentAndWait, deploymentsUpdated = testopk8s.UpdateDeploymentAndWaitStub()

	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(command string, outFileArg string, args ...string) (string, error) {
			return clienttest.MonInQuorumResponse(), nil
		},
	}
	clientset := test.New(t, 1)
	configDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(configDir)
	context := &clusterd.Context{
		Clientset:                  clientset,
		ConfigDir:                  configDir,
		Executor:                   executor,
		RequestCancelOrchestration: abool.New(),
	}
	c := New(context, "ns", cephv1.ClusterSpec{}, &client.OwnerInfo{}, &sync.Mutex{})
	// clusterInfo is nil so we return err
	err := c.checkHealth()
	assert.NotNil(t, err)

	setCommonMonProperties(c, 1, cephv1.MonSpec{Count: 0, AllowMultiplePerNode: true}, "myversion")
	// mon count is 0 so we return err
	err = c.checkHealth()
	assert.NotNil(t, err)

	c.spec.Mon.Count = 3
	logger.Infof("initial mons: %v", c.ClusterInfo.Monitors)
	c.waitForStart = false
	defer os.RemoveAll(c.context.ConfigDir)

	c.mapping.Schedule["f"] = &MonScheduleInfo{
		Name:    "node0",
		Address: "",
	}
	c.maxMonID = 4

	// mock out the scheduler to return node0
	waitForMonitorScheduling = func(c *Cluster, d *apps.Deployment) (SchedulingResult, error) {
		node, _ := clientset.CoreV1().Nodes().Get(ctx, "node0", metav1.GetOptions{})
		return SchedulingResult{Node: node}, nil
	}

	err = c.checkHealth()
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

	deployments, err := clientset.AppsV1().Deployments(c.Namespace).List(ctx, metav1.ListOptions{})
	assert.NoError(t, err)
	assert.Equal(t, 3, len(deployments.Items))

	// no orphan resources to remove
	c.removeOrphanMonResources()

	// We expect mons to exist: a, g, h
	// Check that their PVCs are not garbage collected after we create fake PVCs
	badMon := "c"
	goodMons := []string{"a", "g", "h"}
	c.spec.Mon.VolumeClaimTemplate = &v1.PersistentVolumeClaim{}
	for _, name := range append(goodMons, badMon) {
		m := &monConfig{ResourceName: "rook-ceph-mon-" + name, DaemonName: name}
		pvc, err := c.makeDeploymentPVC(m, true)
		assert.NoError(t, err)
		_, err = c.context.Clientset.CoreV1().PersistentVolumeClaims(c.Namespace).Create(ctx, pvc, metav1.CreateOptions{})
		assert.NoError(t, err)
	}

	pvcs, err := c.context.Clientset.CoreV1().PersistentVolumeClaims(c.Namespace).List(ctx, metav1.ListOptions{})
	assert.NoError(t, err)
	assert.Equal(t, 4, len(pvcs.Items))

	// pvc "c" should be removed and the others should remain
	c.removeOrphanMonResources()
	pvcs, err = c.context.Clientset.CoreV1().PersistentVolumeClaims(c.Namespace).List(ctx, metav1.ListOptions{})
	assert.NoError(t, err)
	assert.Equal(t, 3, len(pvcs.Items))
	for _, pvc := range pvcs.Items {
		found := false
		for _, name := range goodMons {
			if pvc.Name == "rook-ceph-mon-"+name {
				found = true
				break
			}
		}
		assert.True(t, found, pvc.Name)
	}
}

func TestScaleMonDeployment(t *testing.T) {
	ctx := context.TODO()
	clientset := test.New(t, 1)
	context := &clusterd.Context{Clientset: clientset}
	c := New(context, "ns", cephv1.ClusterSpec{}, &client.OwnerInfo{}, &sync.Mutex{})
	setCommonMonProperties(c, 1, cephv1.MonSpec{Count: 0, AllowMultiplePerNode: true}, "myversion")

	name := "a"
	c.spec.Mon.Count = 3
	logger.Infof("initial mons: %v", c.ClusterInfo.Monitors[name])
	monConfig := &monConfig{ResourceName: resourceName(name), DaemonName: name, DataPathMap: &config.DataPathMap{}}
	d, err := c.makeDeployment(monConfig, false)
	require.NoError(t, err)
	_, err = clientset.AppsV1().Deployments(c.Namespace).Create(ctx, d, metav1.CreateOptions{})
	require.NoError(t, err)

	verifyMonReplicas(ctx, t, c, name, 1)
	err = c.updateMonDeploymentReplica(name, false)
	assert.NoError(t, err)
	verifyMonReplicas(ctx, t, c, name, 0)

	err = c.updateMonDeploymentReplica(name, true)
	assert.NoError(t, err)
	verifyMonReplicas(ctx, t, c, name, 1)
}

func verifyMonReplicas(ctx context.Context, t *testing.T, c *Cluster, name string, expected int32) {
	d, err := c.context.Clientset.AppsV1().Deployments(c.Namespace).Get(ctx, resourceName("a"), metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, expected, *d.Spec.Replicas)
}

func TestCheckHealthNotFound(t *testing.T) {
	ctx := context.TODO()
	var deploymentsUpdated *[]*apps.Deployment
	updateDeploymentAndWait, deploymentsUpdated = testopk8s.UpdateDeploymentAndWaitStub()

	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(command string, outFileArg string, args ...string) (string, error) {
			return clienttest.MonInQuorumResponse(), nil
		},
	}
	clientset := test.New(t, 1)
	configDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(configDir)
	context := &clusterd.Context{
		Clientset:                  clientset,
		ConfigDir:                  configDir,
		Executor:                   executor,
		RequestCancelOrchestration: abool.New(),
	}
	c := New(context, "ns", cephv1.ClusterSpec{}, &client.OwnerInfo{}, &sync.Mutex{})
	setCommonMonProperties(c, 2, cephv1.MonSpec{Count: 3, AllowMultiplePerNode: true}, "myversion")
	c.waitForStart = false
	defer os.RemoveAll(c.context.ConfigDir)

	c.mapping.Schedule["a"] = &MonScheduleInfo{
		Name: "node0",
	}
	c.mapping.Schedule["b"] = &MonScheduleInfo{
		Name: "node0",
	}
	c.maxMonID = 4

	err := c.saveMonConfig()
	assert.NoError(t, err)

	// Check if the two mons are found in the configmap
	cm, err := c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Get(ctx, EndpointConfigMapName, metav1.GetOptions{})
	assert.Nil(t, err)
	if cm.Data[EndpointDataKey] != "a=1.2.3.1:6789,b=1.2.3.2:6789" {
		assert.Equal(t, "b=1.2.3.2:6789,a=1.2.3.1:6789", cm.Data[EndpointDataKey])
	}

	// Because the mon a isn't in the MonInQuorumResponse() this will create a new mon
	delete(c.mapping.Schedule, "b")
	err = c.checkHealth()
	assert.Nil(t, err)
	// No updates in unit tests w/ workaround
	assert.ElementsMatch(t, []string{}, testopk8s.DeploymentNamesUpdated(deploymentsUpdated))
	testopk8s.ClearDeploymentsUpdated(deploymentsUpdated)

	// recheck that the "not found" mon has been replaced with a new one
	cm, err = c.context.Clientset.CoreV1().ConfigMaps(c.Namespace).Get(ctx, EndpointConfigMapName, metav1.GetOptions{})
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
		MockExecuteCommandWithOutputFile: func(command string, outFileArg string, args ...string) (string, error) {
			return monQuorumResponse, nil
		},
	}
	clientset := test.New(t, 1)
	configDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(configDir)
	context := &clusterd.Context{
		Clientset:                  clientset,
		ConfigDir:                  configDir,
		Executor:                   executor,
		RequestCancelOrchestration: abool.New(),
	}
	c := New(context, "ns", cephv1.ClusterSpec{}, &client.OwnerInfo{}, &sync.Mutex{})
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
	c := &Cluster{ClusterInfo: &client.ClusterInfo{}}
	c.ClusterInfo = clienttest.CreateTestClusterInfo(1)

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
	c.ClusterInfo = clienttest.CreateTestClusterInfo(3)
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
	c.ClusterInfo = clienttest.CreateTestClusterInfo(1)
	changed, err = c.addOrRemoveExternalMonitor(fakeResp)
	assert.NoError(t, err)
	assert.True(t, changed)
	// ClusterInfo should now have 2 monitors
	assert.Equal(t, 2, len(c.ClusterInfo.Monitors))
}

func TestForceDeleteFailedMon(t *testing.T) {
	ctx := context.TODO()
	clientset := test.New(t, 1)
	context := &clusterd.Context{
		Clientset: clientset,
	}
	c := &Cluster{
		context:   context,
		Namespace: "ns",
	}

	failedMonName := "a"
	monToFail := createTestMonPod(ctx, t, clientset, c.Namespace, failedMonName)
	createTestMonPod(ctx, t, clientset, c.Namespace, "b")
	createTestMonPod(ctx, t, clientset, c.Namespace, "c")

	assert.NoError(t, c.restartMonIfStuckTerminating(failedMonName))

	// The mon should still exist since it wasn't in a deleted state
	p, err := context.Clientset.CoreV1().Pods(c.Namespace).Get(ctx, monToFail.Name, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, p)

	// Add a deletion timestamp to the pod
	monToFail.DeletionTimestamp = &metav1.Time{Time: time.Now()}
	_, err = context.Clientset.CoreV1().Pods(c.Namespace).Update(ctx, &monToFail, metav1.UpdateOptions{})
	assert.NoError(t, err)

	assert.NoError(t, c.restartMonIfStuckTerminating(failedMonName))

	// The pod should still exist since the node is ready
	p, err = context.Clientset.CoreV1().Pods(c.Namespace).Get(ctx, monToFail.Name, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, p)

	// Set the node to a not ready state
	nodes, err := context.Clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	assert.NoError(t, err)
	for _, node := range nodes.Items {
		node.Status.Conditions[0].Status = v1.ConditionFalse
		localnode := node
		_, err := context.Clientset.CoreV1().Nodes().Update(ctx, &localnode, metav1.UpdateOptions{})
		assert.NoError(t, err)
	}

	assert.NoError(t, c.restartMonIfStuckTerminating(failedMonName))

	// The pod should be deleted since the pod is marked as deleted and the node is not ready
	_, err = context.Clientset.CoreV1().Pods(c.Namespace).Get(ctx, monToFail.Name, metav1.GetOptions{})
	assert.Error(t, err)
	assert.True(t, kerrors.IsNotFound(err))

	// mons b and c should still exist
	_, err = context.Clientset.CoreV1().Pods(c.Namespace).Get(ctx, "b-test", metav1.GetOptions{})
	assert.NoError(t, err)
	_, err = context.Clientset.CoreV1().Pods(c.Namespace).Get(ctx, "c-test", metav1.GetOptions{})
	assert.NoError(t, err)
}

func createTestMonPod(ctx context.Context, t *testing.T, clientset kubernetes.Interface, namespace, name string) v1.Pod {
	pod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name + "-test",
			Namespace: namespace,
			Labels: map[string]string{
				k8sutil.AppAttr: AppName,
				"mon":           name,
			},
		},
	}
	pod.Spec.NodeName = "node0"
	_, err := clientset.CoreV1().Pods(namespace).Create(ctx, &pod, metav1.CreateOptions{})
	assert.NoError(t, err)
	return pod
}

func TestNewHealthChecker(t *testing.T) {
	c := &Cluster{spec: cephv1.ClusterSpec{HealthCheck: cephv1.CephClusterHealthCheckSpec{}}}
	time10s, _ := time.ParseDuration("10s")
	c10s := &Cluster{spec: cephv1.ClusterSpec{HealthCheck: cephv1.CephClusterHealthCheckSpec{DaemonHealth: cephv1.DaemonHealthSpec{Monitor: cephv1.HealthCheckSpec{Interval: "10s"}}}}}

	type args struct {
		monCluster *Cluster
	}
	tests := []struct {
		name string
		args args
		want *HealthChecker
	}{
		{"default-interval", args{c}, &HealthChecker{c, HealthCheckInterval}},
		{"10s-interval", args{c10s}, &HealthChecker{c10s, time10s}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewHealthChecker(tt.args.monCluster); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewHealthChecker() = %v, want %v", got, tt.want)
			}
		})
	}
}
