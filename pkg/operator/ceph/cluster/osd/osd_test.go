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
package osd

import (
	"fmt"
	"os"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	discoverDaemon "github.com/rook/rook/pkg/daemon/discover"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	"github.com/rook/rook/pkg/operator/k8sutil"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/api/core/v1"
	corev1 "k8s.io/api/core/v1"
	extensions "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestStart(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	c := New(&clusterd.Context{Clientset: clientset, ConfigDir: "/var/lib/rook", Executor: &exectest.MockExecutor{}}, "ns", "myversion", cephv1.CephVersionSpec{},
		rookalpha.StorageScopeSpec{}, "", rookalpha.Placement{}, false, v1.ResourceRequirements{}, metav1.OwnerReference{})

	// Start the first time
	err := c.Start()
	assert.Nil(t, err)

	// Should not fail if it already exists
	err = c.Start()
	assert.Nil(t, err)
}

func createDiscoverConfigmap(nodeName, ns string, clientset *fake.Clientset) error {
	data := make(map[string]string, 1)
	data[discoverDaemon.LocalDiskCMData] = `[{"name":"sdx","parent":"","hasChildren":false,"devLinks":"/dev/disk/by-id/scsi-36001405f826bd553d8c4dbf9f41c18be    /dev/disk/by-id/wwn-0x6001405f826bd553d8c4dbf9f41c18be /dev/disk/by-path/ip-127.0.0.1:3260-iscsi-iqn.2016-06.world.srv:storage.target01-lun-1","size":10737418240,"uuid":"","serial":"36001405f826bd553d8c4dbf9f41c18be","type":"disk","rotational":true,"readOnly":false,"ownPartition":true,"filesystem":"","vendor":"LIO-ORG","model":"disk02","wwn":"0x6001405f826bd553","wwnVendorExtension":"0x6001405f826bd553d8c4dbf9f41c18be","empty":true}]`
	cm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "local-device-" + nodeName,
			Namespace: ns,
			Labels: map[string]string{
				k8sutil.AppAttr:         discoverDaemon.AppName,
				discoverDaemon.NodeAttr: nodeName,
			},
		},
		Data: data,
	}
	_, err := clientset.CoreV1().ConfigMaps(ns).Create(cm)
	return err
}

func createNode(nodeName string, condition v1.NodeConditionType, clientset *fake.Clientset) error {
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: nodeName,
		},
		Status: v1.NodeStatus{
			Conditions: []v1.NodeCondition{
				{
					Type: condition,
				},
			},
		},
	}
	_, err := clientset.CoreV1().Nodes().Create(node)
	return err
}

func TestLegacyDeployment(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	c := New(&clusterd.Context{Clientset: clientset}, "ns", "myversion", cephv1.CephVersionSpec{},
		rookalpha.StorageScopeSpec{}, "", rookalpha.Placement{}, false, v1.ResourceRequirements{}, metav1.OwnerReference{})

	osdID := 23
	d := &extensions.Deployment{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf(legacyAppNameFmt, osdID), Namespace: c.Namespace}}
	_, err := clientset.Extensions().Deployments(c.Namespace).Create(d)
	require.Nil(t, err)

	// delete the deployment
	assert.Nil(t, c.deleteDeploymentWithLegacyName(osdID))
	deployments, err := clientset.Extensions().Deployments(c.Namespace).List(metav1.ListOptions{})
	assert.Nil(t, err)
	assert.Equal(t, 0, len(deployments.Items))

	// return success if the deployment doesn't exist
	assert.Nil(t, c.deleteDeploymentWithLegacyName(osdID))

	// don't delete the newer deployment name
	d = &extensions.Deployment{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf(osdAppNameFmt, osdID), Namespace: c.Namespace}}
	_, err = clientset.Extensions().Deployments(c.Namespace).Create(d)
	require.Nil(t, err)
	assert.Nil(t, c.deleteDeploymentWithLegacyName(osdID))
	deployments, err = clientset.Extensions().Deployments(c.Namespace).List(metav1.ListOptions{})
	assert.Nil(t, err)
	assert.Equal(t, 1, len(deployments.Items))
}

func TestAddRemoveNode(t *testing.T) {
	// create a storage spec with the given nodes/devices/dirs
	nodeName := "node8230"
	storageSpec := rookalpha.StorageScopeSpec{
		Nodes: []rookalpha.Node{
			{
				Name: nodeName,
				Selection: rookalpha.Selection{
					Devices:     []rookalpha.Device{{Name: "sdx"}},
					Directories: []rookalpha.Directory{{Path: "/rook/storage1"}},
				},
			},
		},
	}

	// set up a fake k8s client set and watcher to generate events that the operator will listen to
	clientset := fake.NewSimpleClientset()
	os.Setenv(k8sutil.PodNamespaceEnvVar, "rook-system")
	defer os.Unsetenv(k8sutil.PodNamespaceEnvVar)

	nodeErr := createNode(nodeName, v1.NodeReady, clientset)
	assert.Nil(t, nodeErr)
	cmErr := createDiscoverConfigmap(nodeName, "rook-system", clientset)
	assert.Nil(t, cmErr)

	statusMapWatcher := watch.NewFake()
	clientset.PrependWatchReactor("configmaps", k8stesting.DefaultWatchReactor(statusMapWatcher, nil))

	c := New(&clusterd.Context{Clientset: clientset, ConfigDir: "/var/lib/rook", Executor: &exectest.MockExecutor{}}, "ns-add-remove", "myversion", cephv1.CephVersionSpec{},
		storageSpec, "", rookalpha.Placement{}, false, v1.ResourceRequirements{}, metav1.OwnerReference{})

	// kick off the start of the orchestration in a goroutine
	var startErr error
	startCompleted := false
	go func() {
		startErr = c.Start()
		startCompleted = true
	}()

	// simulate the completion of the nodes orchestration
	mockNodeOrchestrationCompletion(c, nodeName, statusMapWatcher)

	// wait for orchestration to complete
	waitForOrchestrationCompletion(c, nodeName, &startCompleted)

	// verify orchestration for adding the node succeeded
	assert.True(t, startCompleted)
	assert.Nil(t, startErr)

	// Now let's get ready for testing the removal of the node we just added.  We first need to simulate/mock some things:

	// simulate the node having created an OSD dir map
	kvstore := k8sutil.NewConfigMapKVStore(c.Namespace, c.context.Clientset, metav1.OwnerReference{})
	config.SaveOSDDirMap(kvstore, nodeName, map[string]int{"/rook/storage1": 0})

	// simulate the OSD pod having been created
	osdPod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name:   "osdPod",
		Labels: map[string]string{k8sutil.AppAttr: appName}}}
	c.context.Clientset.CoreV1().Pods(c.Namespace).Create(osdPod)

	// mock the ceph calls that will be called during remove node
	mockExec := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(debug bool, actionName, command, outputFile string, args ...string) (string, error) {
			logger.Infof("Command: %s %v", command, args)
			if args[0] == "status" {
				return `{"pgmap":{"num_pgs":100,"pgs_by_state":[{"state_name":"active+clean","count":100}]}}`, nil
			}
			if args[0] == "osd" {
				if args[1] == "df" {
					return `{"nodes":[{"id":0,"name":"osd.0","kb_used":0},{"id":1,"name":"osd.1","kb_used":0}]}`, nil
				}
				if args[1] == "set" {
					return "", nil
				}
				if args[1] == "unset" {
					return "", nil
				}
				if args[1] == "crush" {
					if args[2] == "reweight" {
						return "", nil
					}
					if args[2] == "rm" {
						return "", nil
					}
				}
				if args[1] == "out" {
					return "", nil
				}
				if args[1] == "rm" {
					assert.Equal(t, "1", args[2])
					return "", nil
				}
				if args[1] == "find" {
					return `{"crush_location":{"host":"my-host"}}`, nil
				}
			}
			if args[0] == "df" && args[1] == "detail" {
				return `{"stats":{"total_bytes":0,"total_used_bytes":0,"total_avail_bytes":3072}}`, nil
			}
			if args[0] == "pg" && args[1] == "dump" {
				return `[]`, nil
			}
			if args[0] == "auth" && args[1] == "del" {
				assert.Equal(t, "osd.1", args[2])
				return "", nil
			}
			return "", fmt.Errorf("unexpected ceph command '%v'", args)
		},
	}

	// modify the storage spec to remove the node from the cluster
	storageSpec.Nodes = []rookalpha.Node{}
	c = New(&clusterd.Context{Clientset: clientset, ConfigDir: "/var/lib/rook", Executor: mockExec}, "ns-add-remove", "myversion", cephv1.CephVersionSpec{},
		storageSpec, "", rookalpha.Placement{}, false, v1.ResourceRequirements{}, metav1.OwnerReference{})

	// reset the orchestration status watcher
	statusMapWatcher = watch.NewFake()
	clientset.PrependWatchReactor("configmaps", k8stesting.DefaultWatchReactor(statusMapWatcher, nil))

	// kick off the start of the removal orchestration in a goroutine
	startErr = nil
	startCompleted = false
	go func() {
		startErr = c.Start()
		startCompleted = true
	}()

	// simulate the completion of the nodes orchestration
	mockNodeOrchestrationCompletion(c, nodeName, statusMapWatcher)

	// wait for orchestration to complete
	waitForOrchestrationCompletion(c, nodeName, &startCompleted)

	// verify orchestration for removing the node succeeded
	assert.True(t, startCompleted)
	assert.Nil(t, startErr)
}

func TestGetIDFromDeployment(t *testing.T) {
	d := &extensions.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "foo"}}
	d.Labels = map[string]string{"ceph-osd-id": "0"}
	assert.Equal(t, 0, getIDFromDeployment(d))

	d.Labels = map[string]string{}
	assert.Equal(t, -1, getIDFromDeployment(d))

	d.Labels = map[string]string{"ceph-osd-id": "101"}
	assert.Equal(t, 101, getIDFromDeployment(d))
}

func TestDiscoverOSDs(t *testing.T) {
	c := New(&clusterd.Context{}, "ns", "myversion", cephv1.CephVersionSpec{},
		rookalpha.StorageScopeSpec{}, "", rookalpha.Placement{}, false, v1.ResourceRequirements{}, metav1.OwnerReference{})
	node1 := "n1"
	node2 := "n2"

	osd1 := OSDInfo{ID: 0, IsDirectory: true, IsFileStore: true, DataPath: "/rook/path"}
	d1, err := c.makeDeployment(node1, []rookalpha.Device{}, rookalpha.Selection{}, v1.ResourceRequirements{}, config.StoreConfig{}, "", "", osd1)
	assert.Nil(t, err)
	assert.NotNil(t, d1)

	osd2 := OSDInfo{ID: 101, IsDirectory: true, IsFileStore: true, DataPath: "/rook/path"}
	d2, err := c.makeDeployment(node1, []rookalpha.Device{}, rookalpha.Selection{}, v1.ResourceRequirements{}, config.StoreConfig{}, "", "", osd2)
	assert.Nil(t, err)
	assert.NotNil(t, d2)

	osd3 := OSDInfo{ID: 23, IsDirectory: true, IsFileStore: true, DataPath: "/rook/path"}
	d3, err := c.makeDeployment(node2, []rookalpha.Device{}, rookalpha.Selection{}, v1.ResourceRequirements{}, config.StoreConfig{}, "", "", osd3)
	assert.Nil(t, err)
	assert.NotNil(t, d3)

	clientset := fake.NewSimpleClientset(d1, d2, d3)
	c.context.Clientset = clientset

	discovered, err := c.discoverStorageNodes()
	require.Nil(t, err)
	assert.Equal(t, 2, len(discovered))

	assert.Equal(t, 2, len(discovered[node1]))
	if discovered[node1][0].Name == "rook-ceph-osd-0" {
		assert.Equal(t, "rook-ceph-osd-101", discovered[node1][1].Name)
	} else {
		assert.Equal(t, "rook-ceph-osd-101", discovered[node1][0].Name)
		assert.Equal(t, "rook-ceph-osd-0", discovered[node1][1].Name)
	}

	assert.Equal(t, 1, len(discovered[node2]))
	assert.Equal(t, "rook-ceph-osd-23", discovered[node2][0].Name)
}

func TestAddNodeFailure(t *testing.T) {
	// create a storage spec with the given nodes/devices/dirs
	nodeName := "node1672"
	storageSpec := rookalpha.StorageScopeSpec{
		Nodes: []rookalpha.Node{
			{
				Name: nodeName,
				Selection: rookalpha.Selection{
					Devices:     []rookalpha.Device{{Name: "sdx"}},
					Directories: []rookalpha.Directory{{Path: "/rook/storage1"}},
				},
			},
		},
	}

	// create a fake clientset that will return an error when the operator tries to create a job
	clientset := fake.NewSimpleClientset()
	clientset.PrependReactor("create", "jobs", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, fmt.Errorf("mock failed to create jobs")
	})
	nodeErr := createNode(nodeName, v1.NodeReady, clientset)
	assert.Nil(t, nodeErr)

	os.Setenv(k8sutil.PodNamespaceEnvVar, "rook-system")
	defer os.Unsetenv(k8sutil.PodNamespaceEnvVar)

	cmErr := createDiscoverConfigmap(nodeName, "rook-system", clientset)
	assert.Nil(t, cmErr)

	c := New(&clusterd.Context{Clientset: clientset, ConfigDir: "/var/lib/rook", Executor: &exectest.MockExecutor{}}, "ns-add-remove", "myversion", cephv1.CephVersionSpec{},
		storageSpec, "", rookalpha.Placement{}, false, v1.ResourceRequirements{}, metav1.OwnerReference{})

	// kick off the start of the orchestration in a goroutine
	var startErr error
	startCompleted := false
	go func() {
		startErr = c.Start()
		startCompleted = true
	}()

	// wait for orchestration to complete
	waitForOrchestrationCompletion(c, nodeName, &startCompleted)

	// verify orchestration failed (because the operator failed to create a job)
	assert.True(t, startCompleted)
	assert.NotNil(t, startErr)
}
