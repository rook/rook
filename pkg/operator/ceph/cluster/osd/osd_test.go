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
	"os"
	"testing"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookv1 "github.com/rook/rook/pkg/apis/rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	discoverDaemon "github.com/rook/rook/pkg/daemon/discover"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	opconfig "github.com/rook/rook/pkg/operator/ceph/config"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestStart(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	clusterInfo := &cephconfig.ClusterInfo{
		CephVersion: cephver.Nautilus,
	}
	c := New(clusterInfo, &clusterd.Context{Clientset: clientset, ConfigDir: "/var/lib/rook", Executor: &exectest.MockExecutor{}}, "ns", "myversion", cephv1.CephVersionSpec{},
		rookv1.StorageScopeSpec{}, "", rookv1.Placement{}, rookv1.Annotations{}, cephv1.NetworkSpec{}, v1.ResourceRequirements{}, v1.ResourceRequirements{}, "my-priority-class", metav1.OwnerReference{}, false, false)

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
					Type: condition, Status: v1.ConditionTrue,
				},
			},
		},
	}
	_, err := clientset.CoreV1().Nodes().Create(node)
	return err
}

func TestAddRemoveNode(t *testing.T) {
	// create a storage spec with the given nodes/devices/dirs
	nodeName := "node8230"
	storageSpec := rookv1.StorageScopeSpec{
		Nodes: []rookv1.Node{
			{
				Name: nodeName,
				Selection: rookv1.Selection{
					Devices: []rookv1.Device{{Name: "sdx"}},
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

	clusterInfo := &cephconfig.ClusterInfo{
		CephVersion: cephver.Nautilus,
	}
	generateKey := "expected key"
	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(debug bool, actionName string, command string, outFileArg string, args ...string) (string, error) {
			return "{\"key\": \"" + generateKey + "\"}", nil
		},
	}

	c := New(clusterInfo, &clusterd.Context{Clientset: clientset, ConfigDir: "/var/lib/rook", Executor: executor}, "ns-add-remove", "myversion", cephv1.CephVersionSpec{},
		storageSpec, "/foo", rookv1.Placement{}, rookv1.Annotations{}, cephv1.NetworkSpec{}, v1.ResourceRequirements{}, v1.ResourceRequirements{}, "my-priority-class", metav1.OwnerReference{}, false, false)

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

	// simulate the OSD pod having been created
	osdPod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name:   "osdPod",
		Labels: map[string]string{k8sutil.AppAttr: AppName}}}
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
				return `{}`, nil
			}
			if args[0] == "auth" && args[1] == "del" {
				assert.Equal(t, "osd.1", args[2])
				return "", nil
			}
			return "", errors.Errorf("unexpected ceph command %q", args)
		},
	}

	// modify the storage spec to remove the node from the cluster
	storageSpec.Nodes = []rookv1.Node{}
	c = New(clusterInfo, &clusterd.Context{Clientset: clientset, ConfigDir: "/var/lib/rook", Executor: mockExec}, "ns-add-remove", "myversion", cephv1.CephVersionSpec{},
		storageSpec, "", rookv1.Placement{}, rookv1.Annotations{}, cephv1.NetworkSpec{}, v1.ResourceRequirements{}, v1.ResourceRequirements{}, "my-priority-class", metav1.OwnerReference{}, false, false)

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

func TestAddNodeFailure(t *testing.T) {
	// create a storage spec with the given nodes/devices/dirs
	nodeName := "node1672"
	storageSpec := rookv1.StorageScopeSpec{
		Nodes: []rookv1.Node{
			{
				Name: nodeName,
				Selection: rookv1.Selection{
					Devices: []rookv1.Device{{Name: "sdx"}},
				},
			},
		},
	}

	// create a fake clientset that will return an error when the operator tries to create a job
	clientset := fake.NewSimpleClientset()
	clientset.PrependReactor("create", "jobs", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, errors.New("mock failed to create jobs")
	})
	nodeErr := createNode(nodeName, v1.NodeReady, clientset)
	assert.Nil(t, nodeErr)

	os.Setenv(k8sutil.PodNamespaceEnvVar, "rook-system")
	defer os.Unsetenv(k8sutil.PodNamespaceEnvVar)

	cmErr := createDiscoverConfigmap(nodeName, "rook-system", clientset)
	assert.Nil(t, cmErr)

	clusterInfo := &cephconfig.ClusterInfo{
		CephVersion: cephver.Nautilus,
	}
	c := New(clusterInfo, &clusterd.Context{Clientset: clientset, ConfigDir: "/var/lib/rook", Executor: &exectest.MockExecutor{}}, "ns-add-remove", "myversion", cephv1.CephVersionSpec{},
		storageSpec, "/foo", rookv1.Placement{}, rookv1.Annotations{}, cephv1.NetworkSpec{}, v1.ResourceRequirements{}, v1.ResourceRequirements{}, "my-priority-class", metav1.OwnerReference{}, false, false)

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

func TestGetOSDInfo(t *testing.T) {
	c := New(&cephconfig.ClusterInfo{}, &clusterd.Context{}, "ns", "myversion", cephv1.CephVersionSpec{},
		rookv1.StorageScopeSpec{}, "", rookv1.Placement{}, rookv1.Annotations{}, cephv1.NetworkSpec{},
		v1.ResourceRequirements{}, v1.ResourceRequirements{}, "my-priority-class", metav1.OwnerReference{}, false, false)

	node := "n1"
	location := "root=default host=myhost zone=myzone"
	osd1 := OSDInfo{ID: 3, UUID: "osd-uuid", BlockPath: "dev/logical-volume-path", Location: location}
	osd2 := OSDInfo{ID: 3, UUID: "osd-uuid", BlockPath: ""}
	osdProp := osdProperties{
		crushHostname: node,
		pvc:           v1.PersistentVolumeClaimVolumeSource{ClaimName: "pvc"},
		selection:     rookv1.Selection{},
		resources:     v1.ResourceRequirements{},
		storeConfig:   config.StoreConfig{},
	}
	dataPathMap := &provisionConfig{
		DataPathMap: opconfig.NewDatalessDaemonDataPathMap(c.Namespace, c.dataDirHostPath),
	}
	d1, _ := c.makeDeployment(osdProp, osd1, dataPathMap)
	osds1, _ := c.getOSDInfo(d1)
	assert.Equal(t, 1, len(osds1))
	assert.Equal(t, osd1.ID, osds1[0].ID)
	assert.Equal(t, osd1.BlockPath, osds1[0].BlockPath)
	assert.Equal(t, location, osds1[0].Location)

	d2, _ := c.makeDeployment(osdProp, osd2, dataPathMap)
	osds2, err := c.getOSDInfo(d2)
	assert.Equal(t, 0, len(osds2))
	assert.NotNil(t, err)
}
