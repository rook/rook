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

// Package osd for the Ceph OSDs.
package osd

import (
	"strconv"
	"testing"

	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha1"
	"github.com/rook/rook/pkg/clusterd"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/kubernetes/pkg/kubelet/apis"
)

func TestPodContainer(t *testing.T) {
	cluster := &Cluster{Namespace: "myosd", Version: "23"}
	config := rookalpha.Config{}
	c := cluster.podTemplateSpec([]rookalpha.Device{}, rookalpha.Selection{}, v1.ResourceRequirements{}, config)
	assert.NotNil(t, c)
	assert.Equal(t, 1, len(c.Spec.Containers))
	container := c.Spec.Containers[0]
	assert.Equal(t, "osd", container.Args[0])
}

func TestDaemonset(t *testing.T) {
	testPodDevices(t, "", "sda", true)
	testPodDevices(t, "/var/lib/mydatadir", "sdb", false)
	testPodDevices(t, "", "", true)
	testPodDevices(t, "", "", false)
}

func testPodDevices(t *testing.T, dataDir, deviceFilter string, allDevices bool) {
	storageSpec := rookalpha.StorageSpec{
		Selection: rookalpha.Selection{UseAllDevices: &allDevices, DeviceFilter: deviceFilter},
		Nodes:     []rookalpha.Node{{Name: "node1"}},
	}

	clientset := fake.NewSimpleClientset()
	c := New(&clusterd.Context{Clientset: clientset, ConfigDir: "/var/lib/rook", Executor: &exectest.MockExecutor{}}, "ns", "rook/rook:myversion",
		storageSpec, dataDir, rookalpha.Placement{}, false, v1.ResourceRequirements{}, metav1.OwnerReference{})

	devMountNeeded := deviceFilter != "" || allDevices

	n := c.Storage.ResolveNode(storageSpec.Nodes[0].Name)
	replicaSet := c.makeReplicaSet(n.Name, n.Devices, n.Selection, v1.ResourceRequirements{}, n.Config)
	assert.NotNil(t, replicaSet)
	assert.Equal(t, "rook-ceph-osd-node1", replicaSet.Name)
	assert.Equal(t, c.Namespace, replicaSet.Namespace)
	assert.Equal(t, int32(1), *(replicaSet.Spec.Replicas))
	assert.Equal(t, "node1", replicaSet.Spec.Template.Spec.NodeSelector[apis.LabelHostname])
	assert.Equal(t, v1.RestartPolicyAlways, replicaSet.Spec.Template.Spec.RestartPolicy)
	if devMountNeeded {
		assert.Equal(t, 3, len(replicaSet.Spec.Template.Spec.Volumes))
	} else {
		assert.Equal(t, 2, len(replicaSet.Spec.Template.Spec.Volumes))
	}
	assert.Equal(t, "rook-data", replicaSet.Spec.Template.Spec.Volumes[0].Name)
	assert.Equal(t, "rook-config-override", replicaSet.Spec.Template.Spec.Volumes[1].Name)
	if devMountNeeded {
		assert.Equal(t, "devices", replicaSet.Spec.Template.Spec.Volumes[2].Name)
	}
	if dataDir == "" {
		assert.NotNil(t, replicaSet.Spec.Template.Spec.Volumes[0].EmptyDir)
		assert.Nil(t, replicaSet.Spec.Template.Spec.Volumes[0].HostPath)
	} else {
		assert.Nil(t, replicaSet.Spec.Template.Spec.Volumes[0].EmptyDir)
		assert.Equal(t, dataDir, replicaSet.Spec.Template.Spec.Volumes[0].HostPath.Path)
	}

	assert.Equal(t, appName, replicaSet.Spec.Template.ObjectMeta.Name)
	assert.Equal(t, appName, replicaSet.Spec.Template.ObjectMeta.Labels["app"])
	assert.Equal(t, c.Namespace, replicaSet.Spec.Template.ObjectMeta.Labels["rook_cluster"])
	assert.Equal(t, 0, len(replicaSet.Spec.Template.ObjectMeta.Annotations))

	cont := replicaSet.Spec.Template.Spec.Containers[0]
	assert.Equal(t, "rook/rook:myversion", cont.Image)
	if devMountNeeded {
		assert.Equal(t, 3, len(cont.VolumeMounts))
	} else {
		assert.Equal(t, 2, len(cont.VolumeMounts))
	}
	assert.Equal(t, "osd", cont.Args[0])

	// verify the config dir env var
	verifyEnvVar(t, cont.Env, "ROOK_CONFIG_DIR", "/var/lib/rook", true)

	// verify the osd store type env var uses the default
	verifyEnvVar(t, cont.Env, "ROOK_OSD_STORE", "bluestore", true)

	// verify the device filter env var
	if deviceFilter != "" {
		verifyEnvVar(t, cont.Env, "ROOK_DATA_DEVICE_FILTER", deviceFilter, true)
	} else if allDevices {
		verifyEnvVar(t, cont.Env, "ROOK_DATA_DEVICE_FILTER", "all", true)
	} else {
		verifyEnvVar(t, cont.Env, "ROOK_DATA_DEVICE_FILTER", "", false)
	}
}

func verifyEnvVar(t *testing.T, envVars []v1.EnvVar, expectedName, expectedValue string, expectedFound bool) {
	found := false
	for _, envVar := range envVars {
		if envVar.Name == expectedName {
			assert.Equal(t, expectedValue, envVar.Value)
			found = true
			break
		}
	}

	assert.Equal(t, expectedFound, found)
}

func TestStorageSpecDevicesAndDirectories(t *testing.T) {
	storageSpec := rookalpha.StorageSpec{
		Config: rookalpha.Config{},
		Selection: rookalpha.Selection{
			Directories: []rookalpha.Directory{{Path: "/rook/dir2"}},
		},
		Nodes: []rookalpha.Node{
			{
				Name:    "node1",
				Devices: []rookalpha.Device{{Name: "sda"}},
				Selection: rookalpha.Selection{
					Directories: []rookalpha.Directory{{Path: "/rook/dir1"}},
				},
			},
		},
	}

	clientset := fake.NewSimpleClientset()
	c := New(&clusterd.Context{Clientset: clientset, ConfigDir: "/var/lib/rook", Executor: &exectest.MockExecutor{}}, "ns", "rook/rook:myversion",
		storageSpec, "", rookalpha.Placement{}, false, v1.ResourceRequirements{}, metav1.OwnerReference{})

	n := c.Storage.ResolveNode(storageSpec.Nodes[0].Name)
	replicaSet := c.makeReplicaSet(n.Name, n.Devices, n.Selection, v1.ResourceRequirements{}, n.Config)
	assert.NotNil(t, replicaSet)

	// pod spec should have a volume for the given dir
	podSpec := replicaSet.Spec.Template.Spec
	assert.Equal(t, 4, len(podSpec.Volumes))
	assert.Equal(t, "rook-dir1", podSpec.Volumes[3].Name)
	assert.Equal(t, "/rook/dir1", podSpec.Volumes[3].VolumeSource.HostPath.Path)

	// container should have a volume mount for the given dir
	container := podSpec.Containers[0]
	assert.Equal(t, "rook-dir1", container.VolumeMounts[3].Name)
	assert.Equal(t, "/rook/dir1", container.VolumeMounts[3].MountPath)

	// container command should have the given dir and device
	verifyEnvVar(t, container.Env, "ROOK_DATA_DIRECTORIES", "/rook/dir1", true)
	verifyEnvVar(t, container.Env, "ROOK_DATA_DEVICES", "sda", true)
}

func TestStorageSpecConfig(t *testing.T) {
	storageSpec := rookalpha.StorageSpec{
		Config: rookalpha.Config{},
		Nodes: []rookalpha.Node{
			{
				Name: "node1",
				Config: rookalpha.Config{
					Location: "rack=foo",
					StoreConfig: rookalpha.StoreConfig{
						StoreType:      "bluestore",
						DatabaseSizeMB: 10,
						WalSizeMB:      20,
						JournalSizeMB:  30,
					},
				},
				Resources: v1.ResourceRequirements{
					Limits: v1.ResourceList{
						v1.ResourceCPU: *resource.NewQuantity(100.0, resource.BinarySI),
					},
					Requests: v1.ResourceList{
						v1.ResourceMemory: *resource.NewQuantity(1337.0, resource.BinarySI),
					},
				},
			},
		},
	}

	clientset := fake.NewSimpleClientset()
	c := New(&clusterd.Context{Clientset: clientset, ConfigDir: "/var/lib/rook", Executor: &exectest.MockExecutor{}}, "ns", "rook/rook:myversion",
		storageSpec, "", rookalpha.Placement{}, false, v1.ResourceRequirements{}, metav1.OwnerReference{})

	n := c.Storage.ResolveNode(storageSpec.Nodes[0].Name)
	replicaSet := c.makeReplicaSet(n.Name, n.Devices, n.Selection, c.Storage.Nodes[0].Resources, n.Config)
	assert.NotNil(t, replicaSet)

	container := replicaSet.Spec.Template.Spec.Containers[0]
	assert.NotNil(t, container)
	verifyEnvVar(t, container.Env, "ROOK_OSD_STORE", "bluestore", true)
	verifyEnvVar(t, container.Env, "ROOK_OSD_DATABASE_SIZE", strconv.Itoa(10), true)
	verifyEnvVar(t, container.Env, "ROOK_OSD_WAL_SIZE", strconv.Itoa(20), true)
	verifyEnvVar(t, container.Env, "ROOK_OSD_JOURNAL_SIZE", strconv.Itoa(30), true)
	verifyEnvVar(t, container.Env, "ROOK_LOCATION", "rack=foo", true)

	assert.Equal(t, "100", container.Resources.Limits.Cpu().String())
	assert.Equal(t, "1337", container.Resources.Requests.Memory().String())
}

func TestHostNetwork(t *testing.T) {
	storageSpec := rookalpha.StorageSpec{
		Config: rookalpha.Config{},
		Nodes: []rookalpha.Node{
			{
				Name: "node1",
				Config: rookalpha.Config{
					Location: "rack=foo",
					StoreConfig: rookalpha.StoreConfig{
						StoreType:      "bluestore",
						DatabaseSizeMB: 10,
						WalSizeMB:      20,
						JournalSizeMB:  30,
					},
				},
			},
		},
	}

	clientset := fake.NewSimpleClientset()
	c := New(&clusterd.Context{Clientset: clientset, ConfigDir: "/var/lib/rook", Executor: &exectest.MockExecutor{}}, "ns", "myversion",
		storageSpec, "", rookalpha.Placement{}, true, v1.ResourceRequirements{}, metav1.OwnerReference{})

	n := c.Storage.ResolveNode(storageSpec.Nodes[0].Name)
	r := c.makeReplicaSet(n.Name, n.Devices, n.Selection, v1.ResourceRequirements{}, n.Config)
	assert.NotNil(t, r)

	assert.Equal(t, true, r.Spec.Template.Spec.HostNetwork)
	assert.Equal(t, v1.DNSClusterFirstWithHostNet, r.Spec.Template.Spec.DNSPolicy)
}
