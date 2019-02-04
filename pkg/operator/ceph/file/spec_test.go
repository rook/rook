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

package file

import (
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	mdsdaemon "github.com/rook/rook/pkg/daemon/ceph/mds"
	cephtest "github.com/rook/rook/pkg/operator/ceph/test"
	"github.com/rook/rook/pkg/operator/k8sutil"
	testop "github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	apps "k8s.io/api/apps/v1"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func testDeploymentObject(hostNetwork bool) *apps.Deployment {
	fs := cephv1.CephFilesystem{
		ObjectMeta: metav1.ObjectMeta{Name: "myfs", Namespace: "ns"},
		Spec: cephv1.FilesystemSpec{
			MetadataServer: cephv1.MetadataServerSpec{
				ActiveCount:   1,
				ActiveStandby: false,
				Resources: v1.ResourceRequirements{
					Limits: v1.ResourceList{
						v1.ResourceCPU: *resource.NewQuantity(100.0, resource.BinarySI),
					},
					Requests: v1.ResourceList{
						v1.ResourceMemory: *resource.NewQuantity(1337.0, resource.BinarySI),
					},
				}}}}
	c := newCluster(
		&clusterd.Context{Clientset: testop.New(1)},
		"rook/rook:myversion",
		cephv1.CephVersionSpec{Image: "ceph/ceph:testversion"},
		hostNetwork,
		fs,
		&client.CephFilesystemDetails{ID: 15},
		[]metav1.OwnerReference{{}},
	)
	mdsTestConfig := &mdsConfig{
		DaemonName:   "myfs-a",
		ResourceName: "rook-ceph-mds-myfs-a",
	}
	return c.makeDeployment(mdsTestConfig)
}

func TestPodSpecs(t *testing.T) {
	d := testDeploymentObject(false) // no host network

	assert.NotNil(t, d)
	assert.Equal(t, "rook-ceph-mds-myfs-a", d.Name)
	assert.Equal(t, "rook-ceph-mds-myfs-a", d.ObjectMeta.Name)
	assert.Equal(t, v1.RestartPolicyAlways, d.Spec.Template.Spec.RestartPolicy)
	assert.Equal(t, 0, len(d.ObjectMeta.Annotations))

	pod := d.Spec.Template

	assert.Nil(t, cephtest.VerifyPodLabels("rook-ceph-mds", "ns", "mds", "myfs-a", pod.ObjectMeta.Labels))
	assert.Equal(t, "myfs", pod.ObjectMeta.Labels["rook_file_system"])
	assert.Equal(t, 0, len(d.ObjectMeta.Annotations))
	assert.Nil(t, testop.VolumeExists("rook-data", pod.Spec.Volumes))
	assert.Nil(t, testop.VolumeExists(cephconfig.DefaultConfigMountName, pod.Spec.Volumes))
	assert.Nil(t, testop.VolumeExists(k8sutil.ConfigOverrideName, pod.Spec.Volumes))

	assert.Equal(t, 1, len(pod.Spec.InitContainers))
	assert.Equal(t, 1, len(pod.Spec.Containers))

	configImage := "rook/rook:myversion"
	configEnvs := 8
	configContainerDefinition := cephtest.ContainerTestDefinition{
		Image:   &configImage,
		Command: []string{}, // no command
		Args: [][]string{
			{"ceph"},
			{mdsdaemon.InitCommand},
			{"--config-dir", "/var/lib/rook"},
			{"--mds-name", "myfs-a"},
			{"--filesystem-id", "15"},
			{"--active-standby", "false"}},
		InOrderArgs: map[int]string{
			0: "ceph",                 // ceph must be first arg
			1: mdsdaemon.InitCommand}, // mds init command must be second arg
		VolumeMountNames: []string{
			"rook-data",
			cephconfig.DefaultConfigMountName,
			k8sutil.ConfigOverrideName},
		EnvCount:     &configEnvs,
		Ports:        []v1.ContainerPort{},
		IsPrivileged: nil, // not set in spec
	}
	cont := &pod.Spec.InitContainers[0]
	configContainerDefinition.TestContainer(t, "config init", cont, logger)
	assert.Equal(t, "100", cont.Resources.Limits.Cpu().String())
	assert.Equal(t, "1337", cont.Resources.Requests.Memory().String())

	daemonImage := "ceph/ceph:testversion"
	daemonEnvs := len(k8sutil.ClusterDaemonEnvVars())
	daemonContainerDefinition := cephtest.ContainerTestDefinition{
		Image: &daemonImage,
		Command: []string{
			"ceph-mds"},
		Args: [][]string{
			{"--foreground"},
			{"--id", "myfs-a"}},
		VolumeMountNames: []string{
			"rook-data",
			cephconfig.DefaultConfigMountName},
		EnvCount:     &daemonEnvs,
		Ports:        []v1.ContainerPort{},
		IsPrivileged: nil, // not set in spec
	}
	keyringMount, err := testop.GetEnv("ROOK_MDS_KEYRING", cont.Env)
	assert.Nil(t, err) // keyring should get the secret named by the filesystem without daemon id
	assert.Equal(t, "rook-ceph-mds-myfs-a", keyringMount.ValueFrom.SecretKeyRef.LocalObjectReference.Name)

	cont = &pod.Spec.Containers[0]
	daemonContainerDefinition.TestContainer(t, "main mon daemon", cont, logger)
	assert.Equal(t, "100", cont.Resources.Limits.Cpu().String())
	assert.Equal(t, "1337", cont.Resources.Requests.Memory().String())

	// Verify that all the mounts have volumes and that there are no extraneous volumes
	volsMountsTestDef := testop.VolumesAndMountsTestDefinition{
		VolumesSpec: &testop.VolumesSpec{Moniker: "mon pod volumes", Volumes: pod.Spec.Volumes},
		MountsSpecItems: []*testop.MountsSpec{
			{Moniker: "mds config init mounts", Mounts: pod.Spec.InitContainers[0].VolumeMounts},
			{Moniker: "mds daemon mounts", Mounts: pod.Spec.Containers[0].VolumeMounts}},
	}
	volsMountsTestDef.TestMountsMatchVolumes(t)
}

func TestHostNetwork(t *testing.T) {
	d := testDeploymentObject(true) // host network

	assert.Equal(t, true, d.Spec.Template.Spec.HostNetwork)
	assert.Equal(t, v1.DNSClusterFirstWithHostNet, d.Spec.Template.Spec.DNSPolicy)
}
