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

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookv1 "github.com/rook/rook/pkg/apis/rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	opconfig "github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/stretchr/testify/assert"
)

func TestPodContainer(t *testing.T) {
	cluster := &Cluster{Namespace: "myosd", rookVersion: "23", cephVersion: cephv1.CephVersionSpec{}, clusterInfo: &cephconfig.ClusterInfo{}}
	osdProps := osdProperties{
		crushHostname: "node",
		devices:       []rookv1.Device{},
		resources:     v1.ResourceRequirements{},
		storeConfig:   config.StoreConfig{},
	}
	dataPathMap := &provisionConfig{
		DataPathMap: opconfig.NewDatalessDaemonDataPathMap(cluster.Namespace, "/var/lib/rook"),
	}
	c, err := cluster.provisionPodTemplateSpec(osdProps, v1.RestartPolicyAlways, dataPathMap)
	assert.NotNil(t, c)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(c.Spec.InitContainers))
	assert.Equal(t, 1, len(c.Spec.Containers))
	container := c.Spec.InitContainers[0]
	logger.Infof("container: %+v", container)
	assert.Equal(t, "copy-binaries", container.Args[0])
	container = c.Spec.Containers[0]
	assert.Equal(t, "/rook/tini", container.Command[0])
	assert.Equal(t, "--", container.Args[0])
	assert.Equal(t, "/rook/rook", container.Args[1])
	assert.Equal(t, "ceph", container.Args[2])
	assert.Equal(t, "osd", container.Args[3])
	assert.Equal(t, "provision", container.Args[4])
}

func TestDaemonset(t *testing.T) {
	testPodDevices(t, "", "sda", true)
	testPodDevices(t, "/var/lib/mydatadir", "sdb", false)
	testPodDevices(t, "", "", true)
	testPodDevices(t, "", "", false)
}

func testPodDevices(t *testing.T, dataDir, deviceName string, allDevices bool) {
	storageSpec := rookv1.StorageScopeSpec{
		Selection: rookv1.Selection{UseAllDevices: &allDevices, DeviceFilter: deviceName},
		Nodes:     []rookv1.Node{{Name: "node1"}},
	}
	devices := []rookv1.Device{
		{Name: deviceName},
	}

	clientset := fake.NewSimpleClientset()
	cephVersion := cephv1.CephVersionSpec{Image: "ceph/ceph:v12.2.8"}
	clusterInfo := &cephconfig.ClusterInfo{
		CephVersion: cephver.Nautilus,
	}
	c := New(clusterInfo, &clusterd.Context{Clientset: clientset, ConfigDir: "/var/lib/rook", Executor: &exectest.MockExecutor{}}, "ns", "rook/rook:myversion", cephVersion,
		storageSpec, dataDir, rookv1.Placement{}, rookv1.Annotations{}, cephv1.NetworkSpec{}, v1.ResourceRequirements{}, v1.ResourceRequirements{}, "my-priority-class", metav1.OwnerReference{}, false, false)

	devMountNeeded := deviceName != "" || allDevices

	n := c.DesiredStorage.ResolveNode(storageSpec.Nodes[0].Name)
	if len(devices) == 0 && len(dataDir) == 0 {
		return
	}
	osd := OSDInfo{
		ID: 0,
	}

	osdProp := osdProperties{
		crushHostname: n.Name,
		selection:     n.Selection,
		resources:     v1.ResourceRequirements{},
		storeConfig:   config.StoreConfig{},
	}

	dataPathMap := &provisionConfig{
		DataPathMap: opconfig.NewDatalessDaemonDataPathMap(c.Namespace, "/var/lib/rook"),
	}

	// Test LVM based on OSD on bare metal
	deployment, err := c.makeDeployment(osdProp, osd, dataPathMap)
	assert.Nil(t, err)
	assert.NotNil(t, deployment)
	assert.Equal(t, "rook-ceph-osd-0", deployment.Name)
	assert.Equal(t, c.Namespace, deployment.Namespace)
	assert.Equal(t, serviceAccountName, deployment.Spec.Template.Spec.ServiceAccountName)
	assert.Equal(t, int32(1), *(deployment.Spec.Replicas))
	assert.Equal(t, "node1", deployment.Spec.Template.Spec.NodeSelector[v1.LabelHostname])
	assert.Equal(t, v1.RestartPolicyAlways, deployment.Spec.Template.Spec.RestartPolicy)
	assert.Equal(t, "my-priority-class", deployment.Spec.Template.Spec.PriorityClassName)
	if devMountNeeded && len(dataDir) > 0 {
		assert.Equal(t, 6, len(deployment.Spec.Template.Spec.Volumes))
	}
	if devMountNeeded && len(dataDir) == 0 {
		assert.Equal(t, 6, len(deployment.Spec.Template.Spec.Volumes))
	}
	if !devMountNeeded && len(dataDir) > 0 {
		assert.Equal(t, 1, len(deployment.Spec.Template.Spec.Volumes))
	}

	assert.Equal(t, "rook-data", deployment.Spec.Template.Spec.Volumes[0].Name)

	assert.Equal(t, AppName, deployment.Spec.Template.ObjectMeta.Name)
	assert.Equal(t, AppName, deployment.Spec.Template.ObjectMeta.Labels["app"])
	assert.Equal(t, c.Namespace, deployment.Spec.Template.ObjectMeta.Labels["rook_cluster"])
	assert.Equal(t, 0, len(deployment.Spec.Template.ObjectMeta.Annotations))

	assert.Equal(t, 2, len(deployment.Spec.Template.Spec.InitContainers))
	initCont := deployment.Spec.Template.Spec.InitContainers[0]
	assert.Equal(t, "ceph/ceph:v12.2.8", initCont.Image)
	assert.Equal(t, "activate", initCont.Name)
	assert.Equal(t, 3, len(initCont.VolumeMounts))

	assert.Equal(t, 1, len(deployment.Spec.Template.Spec.Containers))
	cont := deployment.Spec.Template.Spec.Containers[0]
	assert.Equal(t, cephVersion.Image, cont.Image)
	assert.Equal(t, 6, len(cont.VolumeMounts))
	assert.Equal(t, "ceph-osd", cont.Command[0])

	// Test OSD on PVC with LVM
	osdProp = osdProperties{
		crushHostname: n.Name,
		selection:     n.Selection,
		resources:     v1.ResourceRequirements{},
		storeConfig:   config.StoreConfig{},
		pvc:           v1.PersistentVolumeClaimVolumeSource{ClaimName: "mypvc"},
	}
	// Not needed when running on PVC
	osd = OSDInfo{
		ID:     0,
		CVMode: "lvm",
	}

	deployment, err = c.makeDeployment(osdProp, osd, dataPathMap)
	assert.Nil(t, err)
	assert.NotNil(t, deployment)
	assert.Equal(t, 4, len(deployment.Spec.Template.Spec.InitContainers), deployment.Spec.Template.Spec.InitContainers[2].Name)
	assert.Equal(t, "config-init", deployment.Spec.Template.Spec.InitContainers[0].Name)
	assert.Equal(t, "copy-bins", deployment.Spec.Template.Spec.InitContainers[1].Name)
	assert.Equal(t, "blkdevmapper", deployment.Spec.Template.Spec.InitContainers[2].Name)
	assert.Equal(t, "chown-container-data-dir", deployment.Spec.Template.Spec.InitContainers[3].Name)
	assert.Equal(t, 1, len(deployment.Spec.Template.Spec.Containers))
	initCont = deployment.Spec.Template.Spec.InitContainers[0]
	assert.Equal(t, 4, len(initCont.VolumeMounts), initCont.VolumeMounts)
	blkInitCont := deployment.Spec.Template.Spec.InitContainers[2]
	assert.Equal(t, 1, len(blkInitCont.VolumeDevices))
	cont = deployment.Spec.Template.Spec.Containers[0]
	assert.Equal(t, 8, len(cont.VolumeMounts), cont.VolumeMounts)

	// Test OSD on PVC with RAW
	osd = OSDInfo{
		ID:     0,
		CVMode: "raw",
	}
	deployment, err = c.makeDeployment(osdProp, osd, dataPathMap)
	assert.Nil(t, err)
	assert.NotNil(t, deployment)
	assert.Equal(t, 4, len(deployment.Spec.Template.Spec.InitContainers), deployment.Spec.Template.Spec.InitContainers[2].Name)
	assert.Equal(t, "blkdevmapper", deployment.Spec.Template.Spec.InitContainers[0].Name)
	assert.Equal(t, "activate", deployment.Spec.Template.Spec.InitContainers[1].Name)
	assert.Equal(t, "expand-bluefs", deployment.Spec.Template.Spec.InitContainers[2].Name)
	assert.Equal(t, "chown-container-data-dir", deployment.Spec.Template.Spec.InitContainers[3].Name)
	assert.Equal(t, 1, len(deployment.Spec.Template.Spec.Containers))
	cont = deployment.Spec.Template.Spec.Containers[0]
	assert.Equal(t, 5, len(cont.VolumeMounts), cont.VolumeMounts)

	// Test OSD on PVC with RAW and metadata device
	osd = OSDInfo{
		ID:     0,
		CVMode: "raw",
	}
	osdProp.metadataPVC = v1.PersistentVolumeClaimVolumeSource{ClaimName: "mypvc-metadata"}
	deployment, err = c.makeDeployment(osdProp, osd, dataPathMap)
	assert.Nil(t, err)
	assert.NotNil(t, deployment)
	assert.Equal(t, 5, len(deployment.Spec.Template.Spec.InitContainers))
	assert.Equal(t, "blkdevmapper", deployment.Spec.Template.Spec.InitContainers[0].Name)
	assert.Equal(t, "blkdevmapper-metadata", deployment.Spec.Template.Spec.InitContainers[1].Name)
	assert.Equal(t, "activate", deployment.Spec.Template.Spec.InitContainers[2].Name)
	assert.Equal(t, "expand-bluefs", deployment.Spec.Template.Spec.InitContainers[3].Name)
	assert.Equal(t, "chown-container-data-dir", deployment.Spec.Template.Spec.InitContainers[4].Name)
	assert.Equal(t, 1, len(deployment.Spec.Template.Spec.Containers))
	cont = deployment.Spec.Template.Spec.Containers[0]
	assert.Equal(t, 5, len(cont.VolumeMounts), cont.VolumeMounts)
	blkInitCont = deployment.Spec.Template.Spec.InitContainers[1]
	assert.Equal(t, 1, len(blkInitCont.VolumeDevices))
	blkMetaInitCont := deployment.Spec.Template.Spec.InitContainers[2]
	assert.Equal(t, 1, len(blkMetaInitCont.VolumeDevices))
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

func TestStorageSpecConfig(t *testing.T) {
	storageSpec := rookv1.StorageScopeSpec{
		Nodes: []rookv1.Node{
			{
				Name: "node1",
				Config: map[string]string{
					"databaseSizeMB": "10",
					"walSizeMB":      "20",
					"metadataDevice": "nvme093",
				},
				Selection: rookv1.Selection{},
				Resources: v1.ResourceRequirements{
					Limits: v1.ResourceList{
						v1.ResourceCPU:    *resource.NewQuantity(1024.0, resource.BinarySI),
						v1.ResourceMemory: *resource.NewQuantity(4096.0, resource.BinarySI),
					},
					Requests: v1.ResourceList{
						v1.ResourceCPU:    *resource.NewQuantity(500.0, resource.BinarySI),
						v1.ResourceMemory: *resource.NewQuantity(2048.0, resource.BinarySI),
					},
				},
			},
		},
	}

	clientset := fake.NewSimpleClientset()
	clusterInfo := &cephconfig.ClusterInfo{
		CephVersion: cephver.Nautilus,
	}
	c := New(clusterInfo, &clusterd.Context{Clientset: clientset, ConfigDir: "/var/lib/rook", Executor: &exectest.MockExecutor{}}, "ns", "rook/rook:myversion", cephv1.CephVersionSpec{},
		storageSpec, "", rookv1.Placement{}, rookv1.Annotations{}, cephv1.NetworkSpec{}, v1.ResourceRequirements{}, v1.ResourceRequirements{}, "my-priority-class", metav1.OwnerReference{}, false, false)

	n := c.DesiredStorage.ResolveNode(storageSpec.Nodes[0].Name)
	storeConfig := config.ToStoreConfig(storageSpec.Nodes[0].Config)
	metadataDevice := config.MetadataDevice(storageSpec.Nodes[0].Config)

	osdProp := osdProperties{
		crushHostname:  n.Name,
		devices:        n.Devices,
		selection:      n.Selection,
		resources:      c.DesiredStorage.Nodes[0].Resources,
		storeConfig:    storeConfig,
		metadataDevice: metadataDevice,
	}

	dataPathMap := &provisionConfig{
		DataPathMap: opconfig.NewDatalessDaemonDataPathMap(c.Namespace, "/var/lib/rook"),
	}

	job, err := c.makeJob(osdProp, dataPathMap)
	assert.NotNil(t, job)
	assert.Nil(t, err)
	assert.Equal(t, "rook-ceph-osd-prepare-node1", job.ObjectMeta.Name)
	container := job.Spec.Template.Spec.InitContainers[0]
	assert.NotNil(t, container)
	container = job.Spec.Template.Spec.Containers[0]
	assert.NotNil(t, container)
	verifyEnvVar(t, container.Env, "ROOK_OSD_DATABASE_SIZE", "10", true)
	verifyEnvVar(t, container.Env, "ROOK_OSD_WAL_SIZE", "20", true)
	verifyEnvVar(t, container.Env, "ROOK_METADATA_DEVICE", "nvme093", true)
}

func TestHostNetwork(t *testing.T) {
	storageSpec := rookv1.StorageScopeSpec{
		Nodes: []rookv1.Node{
			{
				Name: "node1",
				Config: map[string]string{
					"databaseSizeMB": "10",
					"walSizeMB":      "20",
				},
			},
		},
	}

	clientset := fake.NewSimpleClientset()
	clusterInfo := &cephconfig.ClusterInfo{
		CephVersion: cephver.Nautilus,
	}
	c := New(clusterInfo, &clusterd.Context{Clientset: clientset, ConfigDir: "/var/lib/rook", Executor: &exectest.MockExecutor{}}, "ns", "myversion", cephv1.CephVersionSpec{},
		storageSpec, "", rookv1.Placement{}, rookv1.Annotations{}, cephv1.NetworkSpec{HostNetwork: true}, v1.ResourceRequirements{}, v1.ResourceRequirements{}, "my-priority-class", metav1.OwnerReference{}, false, false)

	n := c.DesiredStorage.ResolveNode(storageSpec.Nodes[0].Name)
	osd := OSDInfo{
		ID: 0,
	}

	osdProp := osdProperties{
		crushHostname: n.Name,
		devices:       n.Devices,
		selection:     n.Selection,
		resources:     c.DesiredStorage.Nodes[0].Resources,
		storeConfig:   config.StoreConfig{},
	}

	dataPathMap := &provisionConfig{
		DataPathMap: opconfig.NewDatalessDaemonDataPathMap(c.Namespace, "/var/lib/rook"),
	}

	r, err := c.makeDeployment(osdProp, osd, dataPathMap)
	assert.NotNil(t, r)
	assert.Nil(t, err)

	assert.Equal(t, "rook-ceph-osd-0", r.ObjectMeta.Name)
	assert.Equal(t, true, r.Spec.Template.Spec.HostNetwork)
	assert.Equal(t, v1.DNSClusterFirstWithHostNet, r.Spec.Template.Spec.DNSPolicy)
}

func TestOsdOnSDNFlag(t *testing.T) {
	network := cephv1.NetworkSpec{}

	args := osdOnSDNFlag(network)
	assert.NotEmpty(t, args)

	network.Provider = "host"
	args = osdOnSDNFlag(network)
	assert.Empty(t, args)
}

func TestOsdPrepareResources(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	c := New(&cephconfig.ClusterInfo{}, &clusterd.Context{Clientset: clientset, ConfigDir: "/var/lib/rook", Executor: &exectest.MockExecutor{}}, "ns", "myversion", cephv1.CephVersionSpec{},
		rookv1.StorageScopeSpec{}, "", rookv1.Placement{}, rookv1.Annotations{}, cephv1.NetworkSpec{}, v1.ResourceRequirements{}, v1.ResourceRequirements{}, "my-priority-class", metav1.OwnerReference{}, false, false)

	// TEST 2: NOT running on PVC and some prepareResources are specificied
	rr := v1.ResourceRequirements{
		Limits: v1.ResourceList{
			v1.ResourceCPU: *resource.NewQuantity(2000.0, resource.BinarySI),
		},
		Requests: v1.ResourceList{
			v1.ResourceMemory: *resource.NewQuantity(250.0, resource.BinarySI),
		},
	}

	c.prepareResources = rr
	r := c.prepareResources
	assert.Equal(t, "2000", r.Limits.Cpu().String(), rr.Limits.Cpu().String())
	assert.Equal(t, "0", r.Requests.Cpu().String())
	assert.Equal(t, "0", r.Limits.Memory().String())
	assert.Equal(t, "250", r.Requests.Memory().String())
}

func TestCephVolumeEnvVar(t *testing.T) {
	cvEnv := cephVolumeEnvVar()
	assert.Equal(t, "CEPH_VOLUME_DEBUG", cvEnv[0].Name)
	assert.Equal(t, "1", cvEnv[0].Value)
	assert.Equal(t, "CEPH_VOLUME_SKIP_RESTORECON", cvEnv[1].Name)
	assert.Equal(t, "1", cvEnv[1].Value)
	assert.Equal(t, "DM_DISABLE_UDEV", cvEnv[2].Name)
	assert.Equal(t, "1", cvEnv[1].Value)
}

func TestOsdActivateEnvVar(t *testing.T) {
	osdActivateEnv := osdActivateEnvVar()
	assert.Equal(t, 5, len(osdActivateEnv))
	assert.Equal(t, "CEPH_VOLUME_DEBUG", osdActivateEnv[0].Name)
	assert.Equal(t, "1", osdActivateEnv[0].Value)
	assert.Equal(t, "CEPH_VOLUME_SKIP_RESTORECON", osdActivateEnv[1].Name)
	assert.Equal(t, "1", osdActivateEnv[1].Value)
	assert.Equal(t, "DM_DISABLE_UDEV", osdActivateEnv[2].Name)
	assert.Equal(t, "1", osdActivateEnv[1].Value)
	assert.Equal(t, "ROOK_CEPH_MON_HOST", osdActivateEnv[3].Name)
	assert.Equal(t, "CEPH_ARGS", osdActivateEnv[4].Name)
	assert.Equal(t, "-m $(ROOK_CEPH_MON_HOST)", osdActivateEnv[4].Value)
}
