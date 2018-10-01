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
	"testing"

	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/kubernetes/pkg/kubelet/apis"
)

func TestPodContainer(t *testing.T) {
	cluster := &Cluster{Namespace: "myosd", Version: "23"}
	c, err := cluster.provisionPodTemplateSpec([]rookalpha.Device{}, rookalpha.Selection{}, v1.ResourceRequirements{}, config.StoreConfig{}, "", "node", "", v1.RestartPolicyAlways)
	assert.NotNil(t, c)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(c.Spec.Containers))
	container := c.Spec.Containers[0]
	assert.Equal(t, "ceph", container.Args[0])
	assert.Equal(t, "osd", container.Args[1])
}

func TestDaemonset(t *testing.T) {
	testPodDevices(t, "", "sda", true)
	testPodDevices(t, "/var/lib/mydatadir", "sdb", false)
	testPodDevices(t, "", "", true)
	testPodDevices(t, "", "", false)
}

func testPodDevices(t *testing.T, dataDir, deviceName string, allDevices bool) {
	storageSpec := rookalpha.StorageScopeSpec{
		Selection: rookalpha.Selection{UseAllDevices: &allDevices, DeviceFilter: deviceName},
		Nodes:     []rookalpha.Node{{Name: "node1"}},
	}
	devices := []rookalpha.Device{
		{Name: deviceName},
	}

	clientset := fake.NewSimpleClientset()
	c := New(&clusterd.Context{Clientset: clientset, ConfigDir: "/var/lib/rook", Executor: &exectest.MockExecutor{}}, "ns", "rook/rook:myversion", "mysa",
		storageSpec, dataDir, rookalpha.Placement{}, false, v1.ResourceRequirements{}, metav1.OwnerReference{})

	devMountNeeded := deviceName != "" || allDevices

	n := c.Storage.ResolveNode(storageSpec.Nodes[0].Name)
	if len(devices) == 0 && len(dataDir) == 0 {
		return
	}
	osd := OSDInfo{
		ID: 0,
	}

	deployment, err := c.makeDeployment(n.Name, devices, n.Selection, v1.ResourceRequirements{}, config.StoreConfig{}, "", n.Location, osd)
	assert.Nil(t, err)
	assert.NotNil(t, deployment)
	assert.Equal(t, "rook-ceph-osd-id-0", deployment.Name)
	assert.Equal(t, c.Namespace, deployment.Namespace)
	assert.Equal(t, "mysa", deployment.Spec.Template.Spec.ServiceAccountName)
	assert.Equal(t, int32(1), *(deployment.Spec.Replicas))
	assert.Equal(t, "node1", deployment.Spec.Template.Spec.NodeSelector[apis.LabelHostname])
	assert.Equal(t, v1.RestartPolicyAlways, deployment.Spec.Template.Spec.RestartPolicy)
	if devMountNeeded && len(dataDir) > 0 {
		assert.Equal(t, 3, len(deployment.Spec.Template.Spec.Volumes))
	}
	if devMountNeeded && len(dataDir) == 0 {
		assert.Equal(t, 2, len(deployment.Spec.Template.Spec.Volumes))
	}
	if !devMountNeeded && len(dataDir) > 0 {
		assert.Equal(t, 2, len(deployment.Spec.Template.Spec.Volumes))
	}

	//assert.Equal(t, "rook-data", deployment.Spec.Template.Spec.Volumes[0].Name)
	assert.Equal(t, "rook-config-override", deployment.Spec.Template.Spec.Volumes[0].Name)

	assert.Equal(t, appName, deployment.Spec.Template.ObjectMeta.Name)
	assert.Equal(t, appName, deployment.Spec.Template.ObjectMeta.Labels["app"])
	assert.Equal(t, c.Namespace, deployment.Spec.Template.ObjectMeta.Labels["rook_cluster"])
	assert.Equal(t, 0, len(deployment.Spec.Template.ObjectMeta.Annotations))

	assert.Equal(t, 1, len(deployment.Spec.Template.Spec.InitContainers))
	initCont := deployment.Spec.Template.Spec.InitContainers[0]
	assert.Equal(t, "rook/rook:myversion", initCont.Image)
	assert.Equal(t, "osd-init-config", initCont.Name)
	assert.Equal(t, 2, len(initCont.VolumeMounts))

	assert.Equal(t, 1, len(deployment.Spec.Template.Spec.Containers))
	cont := deployment.Spec.Template.Spec.Containers[0]
	assert.Equal(t, "rook/rook:myversion", cont.Image)
	if devMountNeeded {
		assert.Equal(t, 3, len(cont.VolumeMounts))
	} else {
		assert.Equal(t, 3, len(cont.VolumeMounts))
	}
	assert.Equal(t, "/tini", cont.Command[0])
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
	storageSpec := rookalpha.StorageScopeSpec{
		Selection: rookalpha.Selection{
			Directories: []rookalpha.Directory{{Path: "/rook/dir2"}},
		},
		Nodes: []rookalpha.Node{
			{
				Name: "node1",
				Selection: rookalpha.Selection{
					Devices:     []rookalpha.Device{{Name: "sda"}},
					Directories: []rookalpha.Directory{{Path: "/rook/dir1"}},
				},
			},
		},
	}

	clientset := fake.NewSimpleClientset()
	c := New(&clusterd.Context{Clientset: clientset, ConfigDir: "/var/lib/rook", Executor: &exectest.MockExecutor{}}, "ns", "rook/rook:myversion", "",
		storageSpec, "/var/lib/rook", rookalpha.Placement{}, false, v1.ResourceRequirements{}, metav1.OwnerReference{})

	n := c.Storage.ResolveNode(storageSpec.Nodes[0].Name)
	osd := OSDInfo{
		ID:          0,
		IsDirectory: true,
		DataPath:    "/my/root/path/osd1",
	}
	deployment, err := c.makeDeployment(n.Name, n.Devices, n.Selection, v1.ResourceRequirements{}, config.StoreConfig{}, "", n.Location, osd)
	assert.NotNil(t, deployment)
	assert.Nil(t, err)
	// pod spec should have a volume for the given dir in the main container and the init container
	podSpec := deployment.Spec.Template.Spec
	assert.Equal(t, 3, len(podSpec.Volumes))
	require.Equal(t, 1, len(podSpec.Containers))
	cont := podSpec.Containers[0]
	assert.Equal(t, 3, len(cont.VolumeMounts))
	assert.Equal(t, "/var/lib/rook", cont.VolumeMounts[0].MountPath)
	assert.Equal(t, "/etc/rook/config", cont.VolumeMounts[1].MountPath)
	assert.Equal(t, "/my/root/path", cont.VolumeMounts[2].MountPath)

	require.Equal(t, 1, len(podSpec.InitContainers))
	initCont := podSpec.InitContainers[0]
	assert.Equal(t, 3, len(initCont.VolumeMounts))
	assert.Equal(t, "/var/lib/rook", initCont.VolumeMounts[0].MountPath)
	assert.Equal(t, "/etc/rook/config", initCont.VolumeMounts[1].MountPath)
	assert.Equal(t, "/my/root/path", initCont.VolumeMounts[2].MountPath)

	// the default osd created on a node will be under /var/lib/rook, which won't need an extra mount
	osd = OSDInfo{
		ID:          1,
		IsDirectory: true,
		DataPath:    "/var/lib/rook/osd1",
	}
	deployment, err = c.makeDeployment(n.Name, n.Devices, n.Selection, v1.ResourceRequirements{}, config.StoreConfig{}, "", n.Location, osd)
	assert.NotNil(t, deployment)
	assert.Nil(t, err)
	// pod spec should have a volume for the given dir in the main container and the init container
	podSpec = deployment.Spec.Template.Spec
	assert.Equal(t, 2, len(podSpec.Volumes))
	require.Equal(t, 1, len(podSpec.Containers))
	cont = podSpec.Containers[0]
	require.Equal(t, 2, len(cont.VolumeMounts))
	assert.Equal(t, "/var/lib/rook", cont.VolumeMounts[0].MountPath)
	assert.Equal(t, "/etc/rook/config", cont.VolumeMounts[1].MountPath)

	require.Equal(t, 1, len(podSpec.InitContainers))
	initCont = podSpec.InitContainers[0]
	require.Equal(t, 2, len(initCont.VolumeMounts))
	assert.Equal(t, "/var/lib/rook", initCont.VolumeMounts[0].MountPath)
	assert.Equal(t, "/etc/rook/config", initCont.VolumeMounts[1].MountPath)
}

func TestStorageSpecConfig(t *testing.T) {
	storageSpec := rookalpha.StorageScopeSpec{
		Nodes: []rookalpha.Node{
			{
				Name:     "node1",
				Location: "rack=foo",
				Config: map[string]string{
					"storeType":      "bluestore",
					"databaseSizeMB": "10",
					"walSizeMB":      "20",
					"journalSizeMB":  "30",
					"metadataDevice": "nvme093",
				},
				Selection: rookalpha.Selection{
					Directories: []rookalpha.Directory{{Path: "/rook/storageDir472"}},
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
	c := New(&clusterd.Context{Clientset: clientset, ConfigDir: "/var/lib/rook", Executor: &exectest.MockExecutor{}}, "ns", "rook/rook:myversion", "",
		storageSpec, "", rookalpha.Placement{}, false, v1.ResourceRequirements{}, metav1.OwnerReference{})

	n := c.Storage.ResolveNode(storageSpec.Nodes[0].Name)
	storeConfig := config.ToStoreConfig(storageSpec.Nodes[0].Config)
	metadataDevice := config.MetadataDevice(storageSpec.Nodes[0].Config)

	job, err := c.makeJob(n.Name, n.Devices, n.Selection, c.Storage.Nodes[0].Resources, storeConfig, metadataDevice, n.Location)
	assert.NotNil(t, job)
	assert.Nil(t, err)
	assert.Equal(t, "rook-ceph-osd-prepare-node1", job.ObjectMeta.Name)
	container := job.Spec.Template.Spec.Containers[0]
	assert.NotNil(t, container)
	verifyEnvVar(t, container.Env, "ROOK_OSD_STORE", "bluestore", true)
	verifyEnvVar(t, container.Env, "ROOK_OSD_DATABASE_SIZE", "10", true)
	verifyEnvVar(t, container.Env, "ROOK_OSD_WAL_SIZE", "20", true)
	verifyEnvVar(t, container.Env, "ROOK_OSD_JOURNAL_SIZE", "30", true)
	verifyEnvVar(t, container.Env, "ROOK_LOCATION", "rack=foo", true)
	verifyEnvVar(t, container.Env, "ROOK_METADATA_DEVICE", "nvme093", true)

	assert.Equal(t, "100", container.Resources.Limits.Cpu().String())
	assert.Equal(t, "1337", container.Resources.Requests.Memory().String())

	// verify that osd config can be discovered from the container and matches the original config from the spec
	discoveredConfig := getConfigFromContainer(container)
	assert.Equal(t, n.Config, discoveredConfig)
	discoveredDirs := getDirectoriesFromContainer(container)
	assert.Equal(t, n.Directories, discoveredDirs)
}

func TestHostNetwork(t *testing.T) {
	storageSpec := rookalpha.StorageScopeSpec{
		Nodes: []rookalpha.Node{
			{
				Name:     "node1",
				Location: "rack=foo",
				Config: map[string]string{
					"storeType":      "bluestore",
					"databaseSizeMB": "10",
					"walSizeMB":      "20",
					"journalSizeMB":  "30",
				},
			},
		},
	}

	clientset := fake.NewSimpleClientset()
	c := New(&clusterd.Context{Clientset: clientset, ConfigDir: "/var/lib/rook", Executor: &exectest.MockExecutor{}}, "ns", "myversion", "",
		storageSpec, "", rookalpha.Placement{}, true, v1.ResourceRequirements{}, metav1.OwnerReference{})

	n := c.Storage.ResolveNode(storageSpec.Nodes[0].Name)
	osd := OSDInfo{
		ID: 0,
	}
	r, err := c.makeDeployment(n.Name, n.Devices, n.Selection, v1.ResourceRequirements{}, config.StoreConfig{}, "", n.Location, osd)
	assert.NotNil(t, r)
	assert.Nil(t, err)

	assert.Equal(t, "rook-ceph-osd-id-0", r.ObjectMeta.Name)
	assert.Equal(t, true, r.Spec.Template.Spec.HostNetwork)
	assert.Equal(t, v1.DNSClusterFirstWithHostNet, r.Spec.Template.Spec.DNSPolicy)
}
