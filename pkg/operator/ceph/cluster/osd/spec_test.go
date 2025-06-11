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
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	opconfig "github.com/rook/rook/pkg/operator/ceph/config"
	operatortest "github.com/rook/rook/pkg/operator/ceph/test"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/kubernetes/fake"
)

func TestPodContainer(t *testing.T) {
	cluster := &Cluster{rookVersion: "23", clusterInfo: cephclient.AdminTestClusterInfo("myosd")}
	cluster.clusterInfo.OwnerInfo = cephclient.NewMinimumOwnerInfo(t)
	osdProps := osdProperties{
		crushHostname: "node",
		devices:       []cephv1.Device{},
		resources:     corev1.ResourceRequirements{},
		storeConfig:   config.StoreConfig{},
		schedulerName: "custom-scheduler",
	}
	dataPathMap := &provisionConfig{
		DataPathMap: opconfig.NewDatalessDaemonDataPathMap(cluster.clusterInfo.Namespace, "/var/lib/rook"),
	}
	c, err := cluster.provisionPodTemplateSpec(osdProps, corev1.RestartPolicyAlways, dataPathMap)
	assert.NotNil(t, c)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(c.Spec.InitContainers))
	assert.Equal(t, 1, len(c.Spec.Containers))
	assert.Equal(t, "custom-scheduler", c.Spec.SchedulerName)
	container := c.Spec.InitContainers[0]
	logger.Infof("container: %+v", container)
	assert.Equal(t, "cp", container.Command[0])
	container = c.Spec.Containers[0]
	assert.Equal(t, "/rook/rook", container.Command[0])
	assert.Equal(t, "ceph", container.Args[0])
	assert.Equal(t, "osd", container.Args[1])
	assert.Equal(t, "provision", container.Args[2])

	for _, c := range c.Spec.Containers {
		vars := operatortest.FindDuplicateEnvVars(c)
		assert.Equal(t, 0, len(vars))
	}
}

func TestDaemonset(t *testing.T) {
	t.Run(("device name and all devices"), func(t *testing.T) {
		testPodDevices(t, "", "sda", true)
	})
	t.Run(("data dir and device name"), func(t *testing.T) {
		testPodDevices(t, "/var/lib/mydatadir", "sdb", false)
	})
	t.Run(("all devices"), func(t *testing.T) {
		testPodDevices(t, "", "", true)
	})
	t.Run(("no data dir and device name"), func(t *testing.T) {
		testPodDevices(t, "", "", false)
	})
}

func testPodDevices(t *testing.T, dataDir, deviceName string, allDevices bool) {
	devices := []cephv1.Device{
		{Name: deviceName},
	}

	clientset := fake.NewSimpleClientset()
	clusterInfo := &cephclient.ClusterInfo{
		Namespace:   "ns",
		CephVersion: cephver.Squid,
	}
	clusterInfo.SetName("test")
	clusterInfo.OwnerInfo = cephclient.NewMinimumOwnerInfo(t)
	context := &clusterd.Context{Clientset: clientset, ConfigDir: "/var/lib/rook", Executor: &exectest.MockExecutor{}}
	spec := cephv1.ClusterSpec{
		CephVersion: cephv1.CephVersionSpec{Image: "quay.io/ceph/ceph:v15"},
		Storage: cephv1.StorageScopeSpec{
			Selection: cephv1.Selection{UseAllDevices: &allDevices, DeviceFilter: deviceName},
			Nodes:     []cephv1.Node{{Name: "node1"}},
		},
		PriorityClassNames: map[cephv1.KeyType]string{
			cephv1.KeyOSD: "my-priority-class",
		},
		Annotations: cephv1.AnnotationsSpec{
			"osd": map[string]string{
				"TCMALLOC_MAX_TOTAL_THREAD_CACHE_BYTES": "134217728",
			},
		},
		DataDirHostPath: "/var/lib/rook/",
	}
	c := New(context, clusterInfo, spec, "rook/rook:myversion")

	devMountNeeded := deviceName != "" || allDevices

	n := c.spec.Storage.ResolveNode(spec.Storage.Nodes[0].Name)
	if len(devices) == 0 && len(dataDir) == 0 {
		return
	}
	osd := &OSDInfo{
		ID:     0,
		CVMode: "raw",
	}

	osdProp := osdProperties{
		crushHostname: n.Name,
		selection:     n.Selection,
		resources:     corev1.ResourceRequirements{},
		storeConfig:   config.StoreConfig{},
		schedulerName: "custom-scheduler",
	}

	dataPathMap := &provisionConfig{
		DataPathMap: opconfig.NewDatalessDaemonDataPathMap(c.clusterInfo.Namespace, "/var/lib/rook"),
	}

	// Test LVM based on OSD on bare metal
	deployment, err := c.makeDeployment(osdProp, osd, dataPathMap)
	assert.Nil(t, err)
	assert.NotNil(t, deployment)
	assert.Equal(t, "rook-ceph-osd-0", deployment.Name)
	assert.Equal(t, c.clusterInfo.Namespace, deployment.Namespace)
	assert.Equal(t, serviceAccountName, deployment.Spec.Template.Spec.ServiceAccountName)
	assert.Equal(t, int32(1), *(deployment.Spec.Replicas))
	assert.Equal(t, "node1", deployment.Spec.Template.Spec.NodeSelector[corev1.LabelHostname])
	assert.Equal(t, corev1.RestartPolicyAlways, deployment.Spec.Template.Spec.RestartPolicy)
	assert.Equal(t, "my-priority-class", deployment.Spec.Template.Spec.PriorityClassName)
	if devMountNeeded && len(dataDir) > 0 {
		assert.Equal(t, 10, len(deployment.Spec.Template.Spec.Volumes))
	}
	if devMountNeeded && len(dataDir) == 0 {
		assert.Equal(t, 10, len(deployment.Spec.Template.Spec.Volumes))
	}
	if !devMountNeeded && len(dataDir) > 0 {
		assert.Equal(t, 1, len(deployment.Spec.Template.Spec.Volumes))
	}
	assert.Equal(t, "custom-scheduler", deployment.Spec.Template.Spec.SchedulerName)

	assert.Equal(t, "rook-data", deployment.Spec.Template.Spec.Volumes[0].Name)

	assert.Equal(t, AppName, deployment.Spec.Template.ObjectMeta.Name)
	assert.Equal(t, AppName, deployment.Spec.Template.ObjectMeta.Labels["app"])
	assert.Equal(t, c.clusterInfo.Namespace, deployment.Spec.Template.ObjectMeta.Labels["rook_cluster"])
	assert.Equal(t, 1, len(deployment.Spec.Template.ObjectMeta.Annotations))

	assert.Equal(t, 4, len(deployment.Spec.Template.Spec.InitContainers))
	initCont := deployment.Spec.Template.Spec.InitContainers[0]
	assert.Equal(t, "quay.io/ceph/ceph:v15", initCont.Image)
	assert.Equal(t, "activate", initCont.Name)
	assert.Equal(t, 5, len(initCont.VolumeMounts))
	initCont = deployment.Spec.Template.Spec.InitContainers[1]
	assert.Equal(t, "quay.io/ceph/ceph:v15", initCont.Image)
	assert.Equal(t, "expand-bluefs", initCont.Name)
	assert.Equal(t, 2, len(initCont.VolumeMounts))
	initCont = deployment.Spec.Template.Spec.InitContainers[2]
	assert.Equal(t, "quay.io/ceph/ceph:v15", initCont.Image)
	assert.Equal(t, "cephx-keyring-update", initCont.Name)
	assert.Equal(t, 3, len(initCont.VolumeMounts))
	initCont = deployment.Spec.Template.Spec.InitContainers[3]
	assert.Equal(t, "quay.io/ceph/ceph:v15", initCont.Image)
	assert.Equal(t, "chown-container-data-dir", initCont.Name)
	assert.Equal(t, 8, len(initCont.VolumeMounts))

	assert.Equal(t, 1, len(deployment.Spec.Template.Spec.Containers))
	cont := deployment.Spec.Template.Spec.Containers[0]
	assert.Equal(t, spec.CephVersion.Image, cont.Image)
	assert.Equal(t, 8, len(cont.VolumeMounts))
	assert.Equal(t, "ceph-osd", cont.Command[0])
	verifyEnvVar(t, cont.Env, "TCMALLOC_MAX_TOTAL_THREAD_CACHE_BYTES", "134217728", true)

	// Test OSD on PVC with LVM
	osdProp = osdProperties{
		crushHostname: n.Name,
		selection:     n.Selection,
		resources:     corev1.ResourceRequirements{},
		storeConfig:   config.StoreConfig{},
		pvc:           corev1.PersistentVolumeClaimVolumeSource{ClaimName: "mypvc"},
	}
	// Not needed when running on PVC
	osd = &OSDInfo{
		ID:     0,
		CVMode: "lvm",
	}

	deployment, err = c.makeDeployment(osdProp, osd, dataPathMap)
	assert.Nil(t, err)
	assert.NotNil(t, deployment)
	assert.Equal(t, 5, len(deployment.Spec.Template.Spec.InitContainers), deployment.Spec.Template.Spec.InitContainers[2].Name)
	assert.Equal(t, "config-init", deployment.Spec.Template.Spec.InitContainers[0].Name)
	assert.Equal(t, "copy-bins", deployment.Spec.Template.Spec.InitContainers[1].Name)
	assert.Equal(t, "blkdevmapper", deployment.Spec.Template.Spec.InitContainers[2].Name)
	assert.Equal(t, "cephx-keyring-update", deployment.Spec.Template.Spec.InitContainers[3].Name)
	assert.Equal(t, "chown-container-data-dir", deployment.Spec.Template.Spec.InitContainers[4].Name)
	assert.Equal(t, 1, len(deployment.Spec.Template.Spec.Containers))
	initCont = deployment.Spec.Template.Spec.InitContainers[0]
	assert.Equal(t, 5, len(initCont.VolumeMounts), initCont.VolumeMounts)
	blkInitCont := deployment.Spec.Template.Spec.InitContainers[2]
	assert.Equal(t, 1, len(blkInitCont.VolumeDevices))
	cont = deployment.Spec.Template.Spec.Containers[0]
	assert.Equal(t, 9, len(cont.VolumeMounts), cont.VolumeMounts)

	// Test OSD on PVC with RAW
	osd = &OSDInfo{
		ID:     0,
		CVMode: "raw",
	}
	deployment, err = c.makeDeployment(osdProp, osd, dataPathMap)
	assert.Nil(t, err)
	assert.NotNil(t, deployment)
	assert.Equal(t, 5, len(deployment.Spec.Template.Spec.InitContainers), deployment.Spec.Template.Spec.InitContainers[2].Name)
	assert.Equal(t, "blkdevmapper", deployment.Spec.Template.Spec.InitContainers[0].Name)
	assert.Equal(t, "activate", deployment.Spec.Template.Spec.InitContainers[1].Name)
	assert.Equal(t, "expand-bluefs", deployment.Spec.Template.Spec.InitContainers[2].Name)
	assert.Equal(t, "cephx-keyring-update", deployment.Spec.Template.Spec.InitContainers[3].Name)
	assert.Equal(t, "chown-container-data-dir", deployment.Spec.Template.Spec.InitContainers[4].Name)
	assert.Equal(t, 1, len(deployment.Spec.Template.Spec.Containers))
	cont = deployment.Spec.Template.Spec.Containers[0]
	assert.Equal(t, 7, len(cont.VolumeMounts), cont.VolumeMounts)

	// Test with encrypted OSD on PVC with RAW
	osdProp.encrypted = true
	deployment, err = c.makeDeployment(osdProp, osd, dataPathMap)
	assert.Nil(t, err)
	assert.NotNil(t, deployment)
	assert.Equal(t, 9, len(deployment.Spec.Template.Spec.InitContainers), deployment.Spec.Template.Spec.InitContainers[2].Name)
	assert.Equal(t, "blkdevmapper", deployment.Spec.Template.Spec.InitContainers[0].Name)
	assert.Equal(t, "encryption-open", deployment.Spec.Template.Spec.InitContainers[1].Name)
	assert.Equal(t, "blkdevmapper-encryption", deployment.Spec.Template.Spec.InitContainers[2].Name)
	assert.Equal(t, "encrypted-block-status", deployment.Spec.Template.Spec.InitContainers[3].Name)
	assert.Equal(t, "expand-encrypted-bluefs", deployment.Spec.Template.Spec.InitContainers[4].Name)
	assert.Equal(t, 2, len(deployment.Spec.Template.Spec.InitContainers[4].VolumeMounts), deployment.Spec.Template.Spec.InitContainers[4].VolumeMounts)
	assert.Equal(t, "dev-mapper", deployment.Spec.Template.Spec.InitContainers[4].VolumeMounts[1].Name, deployment.Spec.Template.Spec.InitContainers[4].VolumeMounts)
	assert.Equal(t, "activate", deployment.Spec.Template.Spec.InitContainers[5].Name)
	assert.Equal(t, "expand-bluefs", deployment.Spec.Template.Spec.InitContainers[6].Name)
	assert.Equal(t, "cephx-keyring-update", deployment.Spec.Template.Spec.InitContainers[7].Name)
	assert.Equal(t, "chown-container-data-dir", deployment.Spec.Template.Spec.InitContainers[8].Name)
	assert.Equal(t, 1, len(deployment.Spec.Template.Spec.Containers))
	cont = deployment.Spec.Template.Spec.Containers[0]
	assert.Equal(t, 8, len(cont.VolumeMounts), cont.VolumeMounts)
	osdProp.encrypted = false
	assert.Equal(t, 11, len(deployment.Spec.Template.Spec.Volumes), deployment.Spec.Template.Spec.Volumes)

	// // Test OSD on PVC with RAW and metadata device
	osd = &OSDInfo{
		ID:     0,
		CVMode: "raw",
	}
	osdProp.metadataPVC = corev1.PersistentVolumeClaimVolumeSource{ClaimName: "mypvc-metadata"}
	deployment, err = c.makeDeployment(osdProp, osd, dataPathMap)
	assert.Nil(t, err)
	assert.NotNil(t, deployment)
	assert.Equal(t, 6, len(deployment.Spec.Template.Spec.InitContainers))
	assert.Equal(t, "blkdevmapper", deployment.Spec.Template.Spec.InitContainers[0].Name)
	assert.Equal(t, "blkdevmapper-metadata", deployment.Spec.Template.Spec.InitContainers[1].Name)
	assert.Equal(t, "activate", deployment.Spec.Template.Spec.InitContainers[2].Name)
	assert.Equal(t, "expand-bluefs", deployment.Spec.Template.Spec.InitContainers[3].Name)
	assert.Equal(t, "cephx-keyring-update", deployment.Spec.Template.Spec.InitContainers[4].Name)
	assert.Equal(t, "chown-container-data-dir", deployment.Spec.Template.Spec.InitContainers[5].Name)
	assert.Equal(t, 1, len(deployment.Spec.Template.Spec.Containers))
	cont = deployment.Spec.Template.Spec.Containers[0]
	assert.Equal(t, 7, len(cont.VolumeMounts), cont.VolumeMounts)
	blkInitCont = deployment.Spec.Template.Spec.InitContainers[1]
	assert.Equal(t, 1, len(blkInitCont.VolumeDevices))
	blkMetaInitCont := deployment.Spec.Template.Spec.InitContainers[2]
	assert.Equal(t, 1, len(blkMetaInitCont.VolumeDevices))
	assert.Equal(t, 11, len(deployment.Spec.Template.Spec.Volumes), deployment.Spec.Template.Spec.Volumes)

	// // Test encrypted OSD on PVC with RAW and metadata device
	osd = &OSDInfo{
		ID:     0,
		CVMode: "raw",
	}
	osdProp.encrypted = true
	osdProp.metadataPVC = corev1.PersistentVolumeClaimVolumeSource{ClaimName: "mypvc-metadata"}
	deployment, err = c.makeDeployment(osdProp, osd, dataPathMap)
	assert.Nil(t, err)
	assert.NotNil(t, deployment)
	assert.Equal(t, 12, len(deployment.Spec.Template.Spec.InitContainers))
	assert.Equal(t, "blkdevmapper", deployment.Spec.Template.Spec.InitContainers[0].Name)
	assert.Equal(t, "blkdevmapper-metadata", deployment.Spec.Template.Spec.InitContainers[1].Name)
	assert.Equal(t, "encryption-open", deployment.Spec.Template.Spec.InitContainers[2].Name)
	assert.Equal(t, "encryption-open-metadata", deployment.Spec.Template.Spec.InitContainers[3].Name)
	assert.Equal(t, "blkdevmapper-encryption", deployment.Spec.Template.Spec.InitContainers[4].Name)
	assert.Equal(t, "blkdevmapper-metadata-encryption", deployment.Spec.Template.Spec.InitContainers[5].Name)
	assert.Equal(t, "encrypted-block-status", deployment.Spec.Template.Spec.InitContainers[6].Name)
	assert.Equal(t, "expand-encrypted-bluefs", deployment.Spec.Template.Spec.InitContainers[7].Name)
	assert.Equal(t, "activate", deployment.Spec.Template.Spec.InitContainers[8].Name)
	assert.Equal(t, "expand-bluefs", deployment.Spec.Template.Spec.InitContainers[9].Name)
	assert.Equal(t, "cephx-keyring-update", deployment.Spec.Template.Spec.InitContainers[10].Name)
	assert.Equal(t, "chown-container-data-dir", deployment.Spec.Template.Spec.InitContainers[11].Name)
	assert.Equal(t, 1, len(deployment.Spec.Template.Spec.Containers))
	cont = deployment.Spec.Template.Spec.Containers[0]
	assert.Equal(t, 8, len(cont.VolumeMounts), cont.VolumeMounts)
	blkInitCont = deployment.Spec.Template.Spec.InitContainers[1]
	assert.Equal(t, 1, len(blkInitCont.VolumeDevices))
	blkMetaInitCont = deployment.Spec.Template.Spec.InitContainers[8]
	assert.Equal(t, 1, len(blkMetaInitCont.VolumeDevices))
	osdProp.encrypted = false
	assert.Equal(t, 13, len(deployment.Spec.Template.Spec.Volumes), deployment.Spec.Template.Spec.Volumes)

	// // Test OSD on PVC with RAW / metadata and wal device
	osd = &OSDInfo{
		ID:     0,
		CVMode: "raw",
	}
	osdProp.metadataPVC = corev1.PersistentVolumeClaimVolumeSource{ClaimName: "mypvc-metadata"}
	osdProp.walPVC = corev1.PersistentVolumeClaimVolumeSource{ClaimName: "mypvc-wal"}
	deployment, err = c.makeDeployment(osdProp, osd, dataPathMap)
	assert.Nil(t, err)
	assert.NotNil(t, deployment)
	assert.Equal(t, 7, len(deployment.Spec.Template.Spec.InitContainers))
	assert.Equal(t, "blkdevmapper", deployment.Spec.Template.Spec.InitContainers[0].Name)
	assert.Equal(t, "blkdevmapper-metadata", deployment.Spec.Template.Spec.InitContainers[1].Name)
	assert.Equal(t, "blkdevmapper-wal", deployment.Spec.Template.Spec.InitContainers[2].Name)
	assert.Equal(t, "activate", deployment.Spec.Template.Spec.InitContainers[3].Name)
	assert.Equal(t, "expand-bluefs", deployment.Spec.Template.Spec.InitContainers[4].Name)
	assert.Equal(t, "cephx-keyring-update", deployment.Spec.Template.Spec.InitContainers[5].Name)
	assert.Equal(t, "chown-container-data-dir", deployment.Spec.Template.Spec.InitContainers[6].Name)
	assert.Equal(t, 1, len(deployment.Spec.Template.Spec.Containers))
	cont = deployment.Spec.Template.Spec.Containers[0]
	assert.Equal(t, 7, len(cont.VolumeMounts), cont.VolumeMounts)
	blkInitCont = deployment.Spec.Template.Spec.InitContainers[1]
	assert.Equal(t, 1, len(blkInitCont.VolumeDevices))
	blkMetaInitCont = deployment.Spec.Template.Spec.InitContainers[2]
	assert.Equal(t, 1, len(blkMetaInitCont.VolumeDevices))
	assert.Equal(t, 13, len(deployment.Spec.Template.Spec.Volumes), deployment.Spec.Template.Spec.Volumes)

	// // Test encrypted OSD on PVC with RAW / metadata and wal device
	osd = &OSDInfo{
		ID:     0,
		CVMode: "raw",
	}
	osdProp.encrypted = true
	osdProp.metadataPVC = corev1.PersistentVolumeClaimVolumeSource{ClaimName: "mypvc-metadata"}
	osdProp.walPVC = corev1.PersistentVolumeClaimVolumeSource{ClaimName: "mypvc-wal"}
	deployment, err = c.makeDeployment(osdProp, osd, dataPathMap)
	assert.Nil(t, err)
	assert.NotNil(t, deployment)
	assert.Equal(t, 15, len(deployment.Spec.Template.Spec.InitContainers))
	assert.Equal(t, "blkdevmapper", deployment.Spec.Template.Spec.InitContainers[0].Name)
	assert.Equal(t, "blkdevmapper-metadata", deployment.Spec.Template.Spec.InitContainers[1].Name)
	assert.Equal(t, "blkdevmapper-wal", deployment.Spec.Template.Spec.InitContainers[2].Name)
	assert.Equal(t, "encryption-open", deployment.Spec.Template.Spec.InitContainers[3].Name)
	assert.Equal(t, "encryption-open-metadata", deployment.Spec.Template.Spec.InitContainers[4].Name)
	assert.Equal(t, "encryption-open-wal", deployment.Spec.Template.Spec.InitContainers[5].Name)
	assert.Equal(t, "blkdevmapper-encryption", deployment.Spec.Template.Spec.InitContainers[6].Name)
	assert.Equal(t, "blkdevmapper-metadata-encryption", deployment.Spec.Template.Spec.InitContainers[7].Name)
	assert.Equal(t, "blkdevmapper-wal-encryption", deployment.Spec.Template.Spec.InitContainers[8].Name)
	assert.Equal(t, "encrypted-block-status", deployment.Spec.Template.Spec.InitContainers[9].Name)
	assert.Equal(t, "expand-encrypted-bluefs", deployment.Spec.Template.Spec.InitContainers[10].Name)
	assert.Equal(t, "activate", deployment.Spec.Template.Spec.InitContainers[11].Name)
	assert.Equal(t, "expand-bluefs", deployment.Spec.Template.Spec.InitContainers[12].Name)
	assert.Equal(t, "cephx-keyring-update", deployment.Spec.Template.Spec.InitContainers[13].Name)
	assert.Equal(t, "chown-container-data-dir", deployment.Spec.Template.Spec.InitContainers[14].Name)
	assert.Equal(t, 1, len(deployment.Spec.Template.Spec.Containers))
	cont = deployment.Spec.Template.Spec.Containers[0]
	assert.Equal(t, 8, len(cont.VolumeMounts), cont.VolumeMounts)
	blkInitCont = deployment.Spec.Template.Spec.InitContainers[1]
	assert.Equal(t, 1, len(blkInitCont.VolumeDevices))
	blkMetaInitCont = deployment.Spec.Template.Spec.InitContainers[11]
	assert.Equal(t, 1, len(blkMetaInitCont.VolumeDevices))
	assert.Equal(t, 15, len(deployment.Spec.Template.Spec.Volumes), deployment.Spec.Template.Spec.Volumes)

	// Test with encrypted OSD on PVC with RAW with KMS
	osdProp.encrypted = true
	osdProp.metadataPVC = corev1.PersistentVolumeClaimVolumeSource{}
	osdProp.walPVC = corev1.PersistentVolumeClaimVolumeSource{}
	c.spec.Security.KeyManagementService.ConnectionDetails = map[string]string{"KMS_PROVIDER": "vault"}
	c.spec.Security.KeyManagementService.TokenSecretName = "vault-token"
	deployment, err = c.makeDeployment(osdProp, osd, dataPathMap)
	assert.Nil(t, err)
	assert.NotNil(t, deployment)
	assert.Equal(t, 10, len(deployment.Spec.Template.Spec.InitContainers), deployment.Spec.Template.Spec.InitContainers)
	assert.Equal(t, "blkdevmapper", deployment.Spec.Template.Spec.InitContainers[0].Name)
	assert.Equal(t, "encryption-kms-get-kek", deployment.Spec.Template.Spec.InitContainers[1].Name)
	assert.Equal(t, "encryption-open", deployment.Spec.Template.Spec.InitContainers[2].Name)
	assert.Equal(t, "blkdevmapper-encryption", deployment.Spec.Template.Spec.InitContainers[3].Name)
	assert.Equal(t, "encrypted-block-status", deployment.Spec.Template.Spec.InitContainers[4].Name)
	assert.Equal(t, "expand-encrypted-bluefs", deployment.Spec.Template.Spec.InitContainers[5].Name)
	assert.Equal(t, "activate", deployment.Spec.Template.Spec.InitContainers[6].Name)
	assert.Equal(t, "expand-bluefs", deployment.Spec.Template.Spec.InitContainers[7].Name)
	assert.Equal(t, "cephx-keyring-update", deployment.Spec.Template.Spec.InitContainers[8].Name)
	assert.Equal(t, "chown-container-data-dir", deployment.Spec.Template.Spec.InitContainers[9].Name)
	assert.Equal(t, 1, len(deployment.Spec.Template.Spec.Containers))
	cont = deployment.Spec.Template.Spec.Containers[0]
	assert.Equal(t, 8, len(cont.VolumeMounts), cont.VolumeMounts)
	assert.Equal(t, 11, len(deployment.Spec.Template.Spec.Volumes), deployment.Spec.Template.Spec.Volumes) // One more than the encryption with k8s for the kek get init container

	// Test with encrypted OSD on PVC with RAW with KMS
	osdProp.encrypted = true
	osdProp.metadataPVC = corev1.PersistentVolumeClaimVolumeSource{}
	osdProp.walPVC = corev1.PersistentVolumeClaimVolumeSource{}
	c.spec.Security.KeyManagementService.ConnectionDetails = map[string]string{"KMS_PROVIDER": "kmip"}
	c.spec.Security.KeyManagementService.TokenSecretName = "kmip-credentials"
	deployment, err = c.makeDeployment(osdProp, osd, dataPathMap)
	assert.Nil(t, err)
	assert.NotNil(t, deployment)
	assert.Equal(t, 10, len(deployment.Spec.Template.Spec.InitContainers), deployment.Spec.Template.Spec.InitContainers)
	assert.Equal(t, "blkdevmapper", deployment.Spec.Template.Spec.InitContainers[0].Name)
	assert.Equal(t, "encryption-kms-get-kek", deployment.Spec.Template.Spec.InitContainers[1].Name)
	assert.Equal(t, "encryption-open", deployment.Spec.Template.Spec.InitContainers[2].Name)
	assert.Equal(t, "blkdevmapper-encryption", deployment.Spec.Template.Spec.InitContainers[3].Name)
	assert.Equal(t, "encrypted-block-status", deployment.Spec.Template.Spec.InitContainers[4].Name)
	assert.Equal(t, "expand-encrypted-bluefs", deployment.Spec.Template.Spec.InitContainers[5].Name)
	assert.Equal(t, "activate", deployment.Spec.Template.Spec.InitContainers[6].Name)
	assert.Equal(t, "expand-bluefs", deployment.Spec.Template.Spec.InitContainers[7].Name)
	assert.Equal(t, "cephx-keyring-update", deployment.Spec.Template.Spec.InitContainers[8].Name)
	assert.Equal(t, "chown-container-data-dir", deployment.Spec.Template.Spec.InitContainers[9].Name)
	assert.Equal(t, 1, len(deployment.Spec.Template.Spec.Containers))
	cont = deployment.Spec.Template.Spec.Containers[0]
	assert.Equal(t, 8, len(cont.VolumeMounts), cont.VolumeMounts)
	assert.Equal(t, 12, len(deployment.Spec.Template.Spec.Volumes), deployment.Spec.Template.Spec.Volumes)

	// Test with encrypted OSD on PVC with RAW with KMS with TLS
	osdProp.encrypted = true
	osdProp.metadataPVC = corev1.PersistentVolumeClaimVolumeSource{}
	osdProp.walPVC = corev1.PersistentVolumeClaimVolumeSource{}
	c.spec.Security.KeyManagementService.ConnectionDetails = map[string]string{"KMS_PROVIDER": "vault", "VAULT_CACERT": "vault-ca-cert-secret", "VAULT_CLIENT_CERT": "vault-client-cert-secret", "VAULT_CLIENT_KEY": "vault-client-key-secret"}
	c.spec.Security.KeyManagementService.TokenSecretName = "vault-token"
	deployment, err = c.makeDeployment(osdProp, osd, dataPathMap)
	assert.Nil(t, err)
	assert.NotNil(t, deployment)
	assert.Equal(t, 12, len(deployment.Spec.Template.Spec.Volumes), deployment.Spec.Template.Spec.Volumes)                                     // One more than the encryption with k8s for the kek get init container
	assert.Equal(t, 3, len(deployment.Spec.Template.Spec.Volumes[8].VolumeSource.Projected.Sources), deployment.Spec.Template.Spec.Volumes[0]) // 3 more since we have the tls secrets
	osdProp.encrypted = false

	// Test tune Fast settings when OSD on PVC
	osdProp.tuneFastDeviceClass = true
	deployment, err = c.makeDeployment(osdProp, osd, dataPathMap)
	assert.NoError(t, err)
	for _, flag := range defaultTuneFastSettings {
		assert.Contains(t, deployment.Spec.Template.Spec.Containers[0].Args, flag)
	}

	// Test tune Slow settings when OSD on PVC
	osdProp.tuneSlowDeviceClass = true
	deployment, err = c.makeDeployment(osdProp, osd, dataPathMap)
	assert.NoError(t, err)
	for _, flag := range defaultTuneSlowSettings {
		assert.Contains(t, deployment.Spec.Template.Spec.Containers[0].Args, flag)
	}

	// Test shareProcessNamespace presence
	// We don't need to use the host PID namespace anymore
	assert.False(t, deployment.Spec.Template.Spec.HostPID)
	if deployment.Spec.Template.Spec.ShareProcessNamespace != nil {
		panic("ShareProcessNamespace should be nil")
	}

	// Turn on log collector
	c.spec.LogCollector.Enabled = true
	deployment, err = c.makeDeployment(osdProp, osd, dataPathMap)
	assert.NoError(t, err)
	assert.False(t, deployment.Spec.Template.Spec.HostPID, deployment.Spec.Template.Spec.HostPID)
	if deployment.Spec.Template.Spec.ShareProcessNamespace == nil {
		panic("ShareProcessNamespace should be nil")
	}

	t.Run(("check hostpid and shareprocessnamespace"), func(t *testing.T) {
		clusterInfo := &cephclient.ClusterInfo{
			Namespace:   "ns",
			CephVersion: cephver.Squid,
		}
		clusterInfo.SetName("test")
		clusterInfo.OwnerInfo = cephclient.NewMinimumOwnerInfo(t)
		c := New(context, clusterInfo, spec, "rook/rook:myversion")
		deployment, err = c.makeDeployment(osdProp, osd, dataPathMap)
		assert.NoError(t, err)
		assert.False(t, deployment.Spec.Template.Spec.HostPID, deployment.Spec.Template.Spec.HostPID)

		// Turn on log collector
		c.spec.LogCollector.Enabled = true
		deployment, err = c.makeDeployment(osdProp, osd, dataPathMap)
		assert.NoError(t, err)
		shareProcessNamespace := *deployment.Spec.Template.Spec.ShareProcessNamespace
		assert.True(t, shareProcessNamespace)
	})

	t.Run(("check osd ConfigureProbe"), func(t *testing.T) {
		c.spec.HealthCheck.StartupProbe = make(map[cephv1.KeyType]*cephv1.ProbeSpec)
		c.spec.HealthCheck.StartupProbe[cephv1.KeyOSD] = &cephv1.ProbeSpec{Disabled: false, Probe: &corev1.Probe{InitialDelaySeconds: 1000}}
		c.spec.HealthCheck.LivenessProbe = make(map[cephv1.KeyType]*cephv1.ProbeSpec)
		c.spec.HealthCheck.LivenessProbe[cephv1.KeyOSD] = &cephv1.ProbeSpec{Disabled: false, Probe: &corev1.Probe{InitialDelaySeconds: 900}}
		deployment, err := c.makeDeployment(osdProp, osd, dataPathMap)
		assert.Nil(t, err)
		assert.NotNil(t, deployment)
		assert.NotNil(t, deployment.Spec.Template.Spec.Containers[0].LivenessProbe)
		assert.NotNil(t, deployment.Spec.Template.Spec.Containers[0].StartupProbe)
		assert.Equal(t, int32(900), deployment.Spec.Template.Spec.Containers[0].LivenessProbe.InitialDelaySeconds)
		assert.Equal(t, int32(1000), deployment.Spec.Template.Spec.Containers[0].StartupProbe.InitialDelaySeconds)
	})

	// test custom topology label
	t.Setenv("ROOK_CUSTOM_HOSTNAME_LABEL", "my_custom_hostname_label")
	deployment, err = c.makeDeployment(osdProp, osd, dataPathMap)
	assert.Nil(t, err)
	assert.NotNil(t, deployment)
	assert.Equal(t, "rook-ceph-osd-0", deployment.Name)
	assert.Equal(t, c.clusterInfo.Namespace, deployment.Namespace)
	assert.Equal(t, serviceAccountName, deployment.Spec.Template.Spec.ServiceAccountName)
	assert.Equal(t, int32(1), *(deployment.Spec.Replicas))
	assert.Equal(t, "node1", deployment.Spec.Template.Spec.NodeSelector["my_custom_hostname_label"])
}

func verifyEnvVar(t *testing.T, envVars []corev1.EnvVar, expectedName, expectedValue string, expectedFound bool) {
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
	clientset := fake.NewSimpleClientset()
	clusterInfo := &cephclient.ClusterInfo{
		Namespace:   "ns",
		CephVersion: cephver.Squid,
	}
	clusterInfo.SetName("testing")
	clusterInfo.OwnerInfo = cephclient.NewMinimumOwnerInfo(t)
	context := &clusterd.Context{Clientset: clientset, ConfigDir: "/var/lib/rook", Executor: &exectest.MockExecutor{}}
	spec := cephv1.ClusterSpec{
		DataDirHostPath: context.ConfigDir,
		Storage: cephv1.StorageScopeSpec{
			Config: map[string]string{
				"crushRoot": "custom-root",
			},
			Nodes: []cephv1.Node{
				{
					Name: "node1",
					Config: map[string]string{
						"databaseSizeMB": "10",
						"walSizeMB":      "20",
						"metadataDevice": "nvme093",
					},
					Selection: cephv1.Selection{},
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    *resource.NewQuantity(1024.0, resource.BinarySI),
							corev1.ResourceMemory: *resource.NewQuantity(4096.0, resource.BinarySI),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    *resource.NewQuantity(500.0, resource.BinarySI),
							corev1.ResourceMemory: *resource.NewQuantity(2048.0, resource.BinarySI),
						},
					},
				},
			},
		},
	}

	c := New(context, clusterInfo, spec, "rook/rook:myversion")
	n := c.spec.Storage.ResolveNode(spec.Storage.Nodes[0].Name)
	storeConfig := config.ToStoreConfig(spec.Storage.Nodes[0].Config)
	metadataDevice := config.MetadataDevice(spec.Storage.Nodes[0].Config)

	osdProp := osdProperties{
		crushHostname:  n.Name,
		devices:        n.Devices,
		selection:      n.Selection,
		resources:      c.spec.Storage.Nodes[0].Resources,
		storeConfig:    storeConfig,
		metadataDevice: metadataDevice,
	}

	dataPathMap := &provisionConfig{
		DataPathMap: opconfig.NewDatalessDaemonDataPathMap(c.clusterInfo.Namespace, "/var/lib/rook"),
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
	verifyEnvVar(t, container.Env, CrushRootVarName, "custom-root", true)
}

func TestHostNetwork(t *testing.T) {
	storageSpec := cephv1.StorageScopeSpec{
		Nodes: []cephv1.Node{
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
	clusterInfo := &cephclient.ClusterInfo{
		Namespace:   "ns",
		CephVersion: cephver.Squid,
	}
	clusterInfo.SetName("test")

	clusterInfo.OwnerInfo = cephclient.NewMinimumOwnerInfo(t)
	context := &clusterd.Context{Clientset: clientset, ConfigDir: "/var/lib/rook", Executor: &exectest.MockExecutor{}}
	spec := cephv1.ClusterSpec{
		Storage: storageSpec,
		Network: cephv1.NetworkSpec{HostNetwork: true},
	}
	c := New(context, clusterInfo, spec, "rook/rook:myversion")

	n := c.spec.Storage.ResolveNode(storageSpec.Nodes[0].Name)
	osd := &OSDInfo{
		ID:     0,
		CVMode: "raw",
	}

	osdProp := osdProperties{
		crushHostname: n.Name,
		devices:       n.Devices,
		selection:     n.Selection,
		resources:     c.spec.Storage.Nodes[0].Resources,
		storeConfig:   config.StoreConfig{},
	}

	dataPathMap := &provisionConfig{
		DataPathMap: opconfig.NewDatalessDaemonDataPathMap(c.clusterInfo.Namespace, "/var/lib/rook"),
	}

	r, err := c.makeDeployment(osdProp, osd, dataPathMap)
	assert.NotNil(t, r)
	assert.Nil(t, err)

	assert.Equal(t, "rook-ceph-osd-0", r.ObjectMeta.Name)
	assert.Equal(t, true, r.Spec.Template.Spec.HostNetwork)
	assert.Equal(t, corev1.DNSClusterFirstWithHostNet, r.Spec.Template.Spec.DNSPolicy)
}

func TestOsdPrepareResources(t *testing.T) {
	clientset := fake.NewSimpleClientset()

	context := &clusterd.Context{Clientset: clientset, ConfigDir: "/var/lib/rook", Executor: &exectest.MockExecutor{}}
	clusterInfo := &cephclient.ClusterInfo{Namespace: "ns"}
	clusterInfo.SetName("test")
	clusterInfo.OwnerInfo = cephclient.NewMinimumOwnerInfo(t)
	spec := cephv1.ClusterSpec{
		Resources: map[string]corev1.ResourceRequirements{
			"prepareosd": {
				Limits: corev1.ResourceList{
					corev1.ResourceCPU: *resource.NewQuantity(2000.0, resource.BinarySI),
				},
				Requests: corev1.ResourceList{
					corev1.ResourceMemory: *resource.NewQuantity(250.0, resource.BinarySI),
				},
			},
		},
	}
	c := New(context, clusterInfo, spec, "rook/rook:myversion")

	r := cephv1.GetPrepareOSDResources(c.spec.Resources)
	assert.Equal(t, "2000", r.Limits.Cpu().String())
	assert.Equal(t, "0", r.Requests.Cpu().String())
	assert.Equal(t, "0", r.Limits.Memory().String())
	assert.Equal(t, "250", r.Requests.Memory().String())
}

func TestClusterGetPVCEncryptionOpenInitContainerActivate(t *testing.T) {
	c := New(&clusterd.Context{}, &cephclient.ClusterInfo{OwnerInfo: &k8sutil.OwnerInfo{}}, cephv1.ClusterSpec{}, "rook/rook:myversion")
	osdProperties := osdProperties{
		pvc: corev1.PersistentVolumeClaimVolumeSource{
			ClaimName: "pvc1",
		},
	}
	mountPath := "/var/lib/ceph/osd/ceph-0"

	// No metadata PVC
	containers := c.getPVCEncryptionOpenInitContainerActivate(mountPath, osdProperties)
	assert.Equal(t, 1, len(containers))

	// With metadata PVC
	osdProperties.metadataPVC.ClaimName = "pvcDB"
	containers = c.getPVCEncryptionOpenInitContainerActivate(mountPath, osdProperties)
	assert.Equal(t, 2, len(containers))

	// With wal PVC
	osdProperties.walPVC.ClaimName = "pvcWal"
	containers = c.getPVCEncryptionOpenInitContainerActivate(mountPath, osdProperties)
	assert.Equal(t, 3, len(containers))
}

func TestClusterGetPVCEncryptionInitContainerActivate(t *testing.T) {
	c := New(&clusterd.Context{}, &cephclient.ClusterInfo{OwnerInfo: &k8sutil.OwnerInfo{}}, cephv1.ClusterSpec{}, "rook/rook:myversion")
	osdProperties := osdProperties{
		pvc: corev1.PersistentVolumeClaimVolumeSource{
			ClaimName: "pvc1",
		},
		resources: corev1.ResourceRequirements{},
	}
	mountPath := "/var/lib/ceph/osd/ceph-0"

	// No metadata PVC
	containers := c.getPVCEncryptionInitContainerActivate(mountPath, osdProperties)
	assert.Equal(t, 1, len(containers))

	// With metadata PVC
	osdProperties.metadataPVC.ClaimName = "pvcDB"
	containers = c.getPVCEncryptionInitContainerActivate(mountPath, osdProperties)
	assert.Equal(t, 2, len(containers))

	// With wal PVC
	osdProperties.walPVC.ClaimName = "pvcWal"
	containers = c.getPVCEncryptionInitContainerActivate(mountPath, osdProperties)
	assert.Equal(t, 3, len(containers))
}

// WARNING! modifies c.deviceSets
func getDummyDeploymentOnPVC(clientset *fake.Clientset, c *Cluster, pvcName string, osdID int) *appsv1.Deployment {
	osd := &OSDInfo{
		ID:        osdID,
		UUID:      "some-uuid",
		BlockPath: "/some/path",
		CVMode:    "raw",
		Store:     "bluestore",
	}
	c.deviceSets = append(c.deviceSets, deviceSet{
		Name: pvcName,
		PVCSources: map[string]corev1.PersistentVolumeClaimVolumeSource{
			bluestorePVCData: {ClaimName: pvcName},
		},
		Portable: true,
	})
	config := c.newProvisionConfig()
	d, err := deploymentOnPVC(c, osd, pvcName, config)
	if err != nil {
		panic(err)
	}
	return d
}

// WARNING! modifies c.ValidStorage
func getDummyDeploymentOnNode(clientset *fake.Clientset, c *Cluster, nodeName string, osdID int) *appsv1.Deployment {
	osd := &OSDInfo{
		ID:        osdID,
		UUID:      "some-uuid",
		BlockPath: "/dev/vda",
		CVMode:    "raw",
		Store:     "bluestore",
	}
	c.ValidStorage.Nodes = append(c.ValidStorage.Nodes, cephv1.Node{Name: nodeName})
	config := c.newProvisionConfig()
	d, err := deploymentOnNode(c, osd, nodeName, config)
	if err != nil {
		panic(err)
	}
	return d
}

func TestOSDPlacement(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	clusterInfo := &cephclient.ClusterInfo{
		Namespace:   "ns",
		CephVersion: cephver.Squid,
	}
	clusterInfo.SetName("testing")
	clusterInfo.OwnerInfo = cephclient.NewMinimumOwnerInfo(t)
	context := &clusterd.Context{Clientset: clientset, ConfigDir: "/var/lib/rook", Executor: &exectest.MockExecutor{}}

	spec := cephv1.ClusterSpec{
		Placement: cephv1.PlacementSpec{
			"all": {
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{{
									Key:      "role",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"storage-node1"},
								}},
							},
						},
					},
				},
			},
			"osd": {
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{{
									Key:      "role",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"storage-node1"},
								}},
							},
						},
					},
				},
			},
			"prepareosd": {
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{{
									Key:      "role",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"storage-node1"},
								}},
							},
						},
					},
				},
			},
		},
		Storage: cephv1.StorageScopeSpec{
			OnlyApplyOSDPlacement: false,
		},
	}

	osdProps := osdProperties{
		pvc: corev1.PersistentVolumeClaimVolumeSource{
			ClaimName: "pvc1",
		},
	}
	osdProps.placement = cephv1.Placement{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{
					{
						MatchExpressions: []corev1.NodeSelectorRequirement{
							{
								Key:      "role",
								Operator: corev1.NodeSelectorOpIn,
								Values:   []string{"storage-node3"},
							},
						},
					},
				},
			},
		},
	}

	osdProps.preparePlacement = &cephv1.Placement{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{
					{
						MatchExpressions: []corev1.NodeSelectorRequirement{
							{
								Key:      "role",
								Operator: corev1.NodeSelectorOpIn,
								Values:   []string{"storage-node3"},
							},
						},
					},
				},
			},
		},
	}

	c := New(context, clusterInfo, spec, "rook/rook:myversion")
	osd := &OSDInfo{
		ID:     0,
		CVMode: "raw",
	}

	dataPathMap := &provisionConfig{
		DataPathMap: opconfig.NewDatalessDaemonDataPathMap(c.clusterInfo.Namespace, "/var/lib/rook"),
	}

	// For OSD daemon
	// When OnlyApplyOSDPlacement false, in case of PVC
	r, err := c.makeDeployment(osdProps, osd, dataPathMap)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(r.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions))

	// For OSD-prepare job
	job, err := c.makeJob(osdProps, dataPathMap)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(job.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions))

	// When OnlyApplyOSDPlacement true, in case of PVC
	spec.Storage.OnlyApplyOSDPlacement = true
	c = New(context, clusterInfo, spec, "rook/rook:myversion")
	r, err = c.makeDeployment(osdProps, osd, dataPathMap)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(r.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions))

	// For OSD-prepare job
	job, err = c.makeJob(osdProps, dataPathMap)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(job.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions))

	// When OnlyApplyOSDPlacement false, in case of non-PVC
	spec.Storage.OnlyApplyOSDPlacement = false
	osdProps = osdProperties{}
	c = New(context, clusterInfo, spec, "rook/rook:myversion")
	r, err = c.makeDeployment(osdProps, osd, dataPathMap)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(r.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions))

	// For OSD-prepare job
	job, err = c.makeJob(osdProps, dataPathMap)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(job.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions))

	// When OnlyApplyOSDPlacement true, in case of non-PVC
	spec.Storage.OnlyApplyOSDPlacement = true
	c = New(context, clusterInfo, spec, "rook/rook:myversion")
	r, err = c.makeDeployment(osdProps, osd, dataPathMap)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(r.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions))

	// For OSD-prepare job
	job, err = c.makeJob(osdProps, dataPathMap)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(job.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions))
}

func TestCreateOSDService(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	context := &clusterd.Context{Clientset: clientset, ConfigDir: "/var/lib/rook", Executor: &exectest.MockExecutor{}}
	clusterInfo := &cephclient.ClusterInfo{Namespace: "ns"}
	clusterInfo.SetName("test")
	clusterInfo.OwnerInfo = cephclient.NewMinimumOwnerInfo(t)
	spec := cephv1.ClusterSpec{
		Network: cephv1.NetworkSpec{Connections: &cephv1.ConnectionsSpec{RequireMsgr2: false}},
	}
	c := New(context, clusterInfo, spec, "rook/rook:myversion")
	osd := OSDInfo{
		ID:     0,
		CVMode: "raw",
	}
	service, err := c.createOSDService(osd, map[string]string{})
	assert.NoError(t, err)
	// the clusterIP will not be set in a mock service
	assert.Equal(t, "", service.Spec.ClusterIP)
	assert.Equal(t, 2, len(service.Spec.Ports))

	c.spec.Network.Connections.RequireMsgr2 = true
	service, err = c.createOSDService(osd, map[string]string{})
	assert.NoError(t, err)
	// only 6800 port is added if RequireMsgr2 is enabled
	assert.Equal(t, 1, len(service.Spec.Ports))
	assert.Equal(t, int32(osdPortv2), service.Spec.Ports[0].Port)
}
