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
	"strconv"
	"strings"
	"testing"

	cephosd "github.com/rook/rook/pkg/ceph/osd"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/pkg/api/v1"
)

func TestStartDaemonset(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	c := New(&clusterd.Context{KubeContext: clusterd.KubeContext{Clientset: clientset}}, "ns", "myversion", StorageSpec{}, "", k8sutil.Placement{})

	// Start the first time
	err := c.Start()
	assert.Nil(t, err)

	// Should not fail if it already exists
	err = c.Start()
	assert.Nil(t, err)
}

func TestPodContainer(t *testing.T) {
	cluster := &Cluster{Namespace: "myosd", Version: "23"}
	config := Config{}
	c := cluster.podTemplateSpec([]Device{}, []Directory{}, Selection{}, config)
	assert.NotNil(t, c)
	assert.Equal(t, 1, len(c.Spec.Containers))
	container := c.Spec.Containers[0]
	assert.Equal(t, 7, len(container.Env))
	assert.True(t, strings.Contains(container.Command[2], `echo $(HOSTNAME) | sed "s/\./_/g" > /etc/hostname; hostname -F /etc/hostname`))
	assert.True(t, strings.Contains(container.Command[2], "/usr/local/bin/rookd osd"))
}

func TestDaemonset(t *testing.T) {
	testPodDevices(t, "", "sda", true)
	testPodDevices(t, "/var/lib/mydatadir", "sdb", false)
	testPodDevices(t, "", "", true)
	testPodDevices(t, "", "", false)
}

func testPodDevices(t *testing.T, dataDir, deviceFilter string, allDevices bool) {
	storageSpec := StorageSpec{
		Selection: Selection{UseAllDevices: &allDevices, DeviceFilter: deviceFilter},
		Nodes:     []Node{{Name: "node1"}},
	}

	clientset := fake.NewSimpleClientset()
	c := New(&clusterd.Context{KubeContext: clusterd.KubeContext{Clientset: clientset}}, "ns", "myversion", storageSpec, dataDir, k8sutil.Placement{})

	n := c.Storage.resolveNode(storageSpec.Nodes[0].Name)
	replicaSet := c.makeReplicaSet(n.Name, n.Devices, n.Directories, n.Selection, n.Config)
	assert.NotNil(t, replicaSet)
	assert.Equal(t, "rook-ceph-osd-node1", replicaSet.Name)
	assert.Equal(t, c.Namespace, replicaSet.Namespace)
	assert.Equal(t, int32(1), *(replicaSet.Spec.Replicas))
	assert.Equal(t, "node1", replicaSet.Spec.Template.Spec.NodeSelector[metav1.LabelHostname])
	assert.Equal(t, v1.RestartPolicyAlways, replicaSet.Spec.Template.Spec.RestartPolicy)
	assert.Equal(t, 3, len(replicaSet.Spec.Template.Spec.Volumes))
	assert.Equal(t, "rook-data", replicaSet.Spec.Template.Spec.Volumes[0].Name)
	assert.Equal(t, "devices", replicaSet.Spec.Template.Spec.Volumes[1].Name)
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
	assert.Equal(t, "quay.io/rook/rookd:myversion", cont.Image)
	assert.Equal(t, 3, len(cont.VolumeMounts))

	expectedCommand := "/usr/local/bin/rookd osd"
	assert.NotEqual(t, -1, strings.Index(cont.Command[2], expectedCommand), cont.Command[2])

	// verify the config dir env var
	verifyEnvVar(t, cont.Env, "ROOKD_CONFIG_DIR", "/var/lib/rook", true)

	// verify the osd store type env var uses the default
	verifyEnvVar(t, cont.Env, "ROOKD_OSD_STORE", cephosd.DefaultStore, true)

	// verify the device filter env var
	if deviceFilter != "" {
		verifyEnvVar(t, cont.Env, "ROOKD_DATA_DEVICE_FILTER", deviceFilter, true)
	} else if allDevices {
		verifyEnvVar(t, cont.Env, "ROOKD_DATA_DEVICE_FILTER", "all", true)
	} else {
		verifyEnvVar(t, cont.Env, "ROOKD_DATA_DEVICE_FILTER", "", false)
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
	storageSpec := StorageSpec{
		Config: Config{},
		Nodes: []Node{
			{
				Name:        "node1",
				Devices:     []Device{{Name: "sda"}},
				Directories: []Directory{{Path: "/rook/dir1"}},
			},
		},
	}

	clientset := fake.NewSimpleClientset()
	c := New(&clusterd.Context{KubeContext: clusterd.KubeContext{Clientset: clientset}}, "ns", "myversion", storageSpec, "", k8sutil.Placement{})

	n := c.Storage.resolveNode(storageSpec.Nodes[0].Name)
	replicaSet := c.makeReplicaSet(n.Name, n.Devices, n.Directories, n.Selection, n.Config)
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
	verifyEnvVar(t, container.Env, "ROOKD_DATA_DIRECTORIES", "/rook/dir1", true)
	verifyEnvVar(t, container.Env, "ROOKD_DATA_DEVICES", "sda", true)
}

func TestStorageSpecConfig(t *testing.T) {
	storageSpec := StorageSpec{
		Config: Config{},
		Nodes: []Node{
			{
				Name: "node1",
				Config: Config{
					Location: "rack=foo",
					StoreConfig: cephosd.StoreConfig{
						StoreType:      cephosd.Bluestore,
						DatabaseSizeMB: 10,
						WalSizeMB:      20,
						JournalSizeMB:  30,
					},
				},
			},
		},
	}

	clientset := fake.NewSimpleClientset()
	c := New(&clusterd.Context{KubeContext: clusterd.KubeContext{Clientset: clientset}}, "ns", "myversion", storageSpec, "", k8sutil.Placement{})

	n := c.Storage.resolveNode(storageSpec.Nodes[0].Name)
	replicaSet := c.makeReplicaSet(n.Name, n.Devices, n.Directories, n.Selection, n.Config)
	assert.NotNil(t, replicaSet)

	container := replicaSet.Spec.Template.Spec.Containers[0]
	assert.NotNil(t, container)
	verifyEnvVar(t, container.Env, "ROOKD_OSD_STORE", cephosd.Bluestore, true)
	verifyEnvVar(t, container.Env, "ROOKD_OSD_DATABASE_SIZE", strconv.Itoa(10), true)
	verifyEnvVar(t, container.Env, "ROOKD_OSD_WAL_SIZE", strconv.Itoa(20), true)
	verifyEnvVar(t, container.Env, "ROOKD_OSD_JOURNAL_SIZE", strconv.Itoa(30), true)
	verifyEnvVar(t, container.Env, "ROOKD_LOCATION", "rack=foo", true)
}
