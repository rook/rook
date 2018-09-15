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

package mon

import (
	"fmt"
	"testing"

	cephv1beta1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1beta1"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	mondaemon "github.com/rook/rook/pkg/daemon/ceph/mon"
	test_opceph "github.com/rook/rook/pkg/operator/ceph/test"
	"github.com/rook/rook/pkg/operator/k8sutil"
	testop "github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPodSpecs(t *testing.T) {
	testPodSpec(t, "")
	testPodSpec(t, "/var/lib/mydatadir")
}

func monCommonExpectedArgs(name string, c *Cluster) [][]string {
	return [][]string{
		{"--name", fmt.Sprintf("mon.%s", name)},
		{"--mon-data", mondaemon.GetMonDataDirPath(c.context.ConfigDir, name)},
	}
}

func testPodSpec(t *testing.T, dataDir string) {
	clientset := testop.New(1)
	c := New(
		&clusterd.Context{Clientset: clientset, ConfigDir: "/var/lib/rook"},
		"ns",
		dataDir,
		"rook/rook:myversion",
		cephv1beta1.MonSpec{Count: 3, AllowMultiplePerNode: true},
		rookalpha.Placement{},
		false,
		v1.ResourceRequirements{
			Limits: v1.ResourceList{
				v1.ResourceCPU: *resource.NewQuantity(100.0, resource.BinarySI),
			},
			Requests: v1.ResourceList{
				v1.ResourceMemory: *resource.NewQuantity(1337.0, resource.BinarySI),
			},
		},
		metav1.OwnerReference{},
	)
	c.clusterInfo = testop.CreateConfigDir(0)
	name := "a"
	config := &monConfig{ResourceName: name, DaemonName: name, Port: 6790, PublicIP: "2.4.6.1"}

	pod := c.makeMonPod(config, "foo")
	assert.NotNil(t, pod)
	assert.Equal(t, "a", pod.Name)
	assert.Equal(t, v1.RestartPolicyAlways, pod.Spec.RestartPolicy)
	assert.Equal(t, 3, len(pod.Spec.Volumes))
	assert.Nil(t, testop.VolumeExists("rook-data", pod.Spec.Volumes))
	assert.Nil(t, testop.VolumeExists(k8sutil.ConfigOverrideName, pod.Spec.Volumes))
	if dataDir == "" {
		assert.Nil(t, testop.VolumeIsEmptyDir(k8sutil.DataDirVolume, pod.Spec.Volumes))
	} else {
		assert.Nil(t, testop.VolumeIsHostPath(k8sutil.DataDirVolume, dataDir, pod.Spec.Volumes))
	}

	assert.Equal(t, "a", pod.ObjectMeta.Name)
	assert.Equal(t, appName, pod.ObjectMeta.Labels["app"])
	assert.Equal(t, c.Namespace, pod.ObjectMeta.Labels["mon_cluster"])

	assert.Equal(t, 3, len(pod.Spec.InitContainers))
	assert.Equal(t, 1, len(pod.Spec.Containers))

	// All containers have the same privilege
	isPrivileged := false

	// config w/ rook binary init container
	configImage := "rook/rook:myversion"
	configEnvs := 7
	configContDev := test_opceph.ContainerTestDefinition{
		Image:   &configImage,
		Command: []string{}, // no command
		Args: [][]string{
			{"ceph"},
			{mondaemon.InitCommand},
			{"--config-dir=/var/lib/rook"},
			{fmt.Sprintf("--name=%s", name)},
			{"--port=6790"},
			{fmt.Sprintf("--fsid=%s", c.clusterInfo.FSID)}},
		InOrderArgs: map[int]string{
			0: "ceph",                 // ceph must be first arg
			1: mondaemon.InitCommand}, // mgr init command must be second arg
		VolumeMountNames: []string{
			"rook-data",
			cephconfig.DefaultConfigMountName,
			k8sutil.ConfigOverrideName},
		EnvCount:     &configEnvs,
		Ports:        []v1.ContainerPort{},
		IsPrivileged: &isPrivileged,
	}
	cont := &pod.Spec.InitContainers[0]
	configContDev.TestContainer(t, "config init", cont, logger)
	assert.Equal(t, "100", cont.Resources.Limits.Cpu().String())
	assert.Equal(t, "1337", cont.Resources.Requests.Memory().String())

	// All ceph images have the same image, no envs, and the same volume mounts
	cephImage := "rook/rook:myversion"
	cephEnvs := 0
	cephVolumeMountNames := []string{
		"rook-data",
		cephconfig.DefaultConfigMountName}

	// monmap init container
	monmapContDev := test_opceph.ContainerTestDefinition{
		Image: &cephImage,
		Command: []string{
			"/usr/bin/monmaptool"},
		Args: [][]string{
			{"/var/lib/rook/mon-a/monmap"},
			{"--create"},
			{"--clobber"},
			{"--fsid", c.clusterInfo.FSID}},
		VolumeMountNames: cephVolumeMountNames,
		EnvCount:         &cephEnvs,
		Ports:            []v1.ContainerPort{},
		IsPrivileged:     &isPrivileged,
	}
	cont = &pod.Spec.InitContainers[1]
	monmapContDev.TestContainer(t, "monmap init", cont, logger)
	assert.Equal(t, "100", cont.Resources.Limits.Cpu().String())
	assert.Equal(t, "1337", cont.Resources.Requests.Memory().String())

	// mon fs init container
	monFsContDev := test_opceph.ContainerTestDefinition{
		Image: &cephImage,
		Command: []string{
			"ceph-mon"},
		Args: append(
			monCommonExpectedArgs(name, c),
			[]string{"--mkfs"},
			[]string{"--monmap", "/var/lib/rook/mon-a/monmap"}),
		VolumeMountNames: cephVolumeMountNames,
		EnvCount:         &cephEnvs,
		Ports:            []v1.ContainerPort{},
		IsPrivileged:     &isPrivileged,
	}
	cont = &pod.Spec.InitContainers[2]
	monFsContDev.TestContainer(t, "monmap init", cont, logger)
	assert.Equal(t, "100", cont.Resources.Limits.Cpu().String())
	assert.Equal(t, "1337", cont.Resources.Requests.Memory().String())

	// main mon daemon container
	monDaemonContDev := test_opceph.ContainerTestDefinition{
		Image: &cephImage,
		Command: []string{
			"ceph-mon"},
		Args: append(
			monCommonExpectedArgs(name, c),
			[]string{"--foreground"},
			[]string{"--public-addr", "2.4.6.1:6790"}),
		VolumeMountNames: cephVolumeMountNames,
		EnvCount:         &cephEnvs,
		Ports: []v1.ContainerPort{
			{ContainerPort: config.Port,
				Protocol: v1.ProtocolTCP}},
		IsPrivileged: &isPrivileged,
	}
	cont = &pod.Spec.Containers[0]
	monDaemonContDev.TestContainer(t, "monmap init", cont, logger)
	assert.Equal(t, "100", cont.Resources.Limits.Cpu().String())
	assert.Equal(t, "1337", cont.Resources.Requests.Memory().String())

	// Verify that all the mounts have volumes and that there are no extraneous volumes
	volsMountsTestDef := testop.VolumesAndMountsTestDefinition{
		VolumesSpec: &testop.VolumesSpec{Moniker: "mon pod volumes", Volumes: pod.Spec.Volumes},
		MountsSpecItems: []*testop.MountsSpec{
			{Moniker: "mon config init mounts", Mounts: pod.Spec.InitContainers[0].VolumeMounts},
			{Moniker: "mon monmap init mounts", Mounts: pod.Spec.InitContainers[1].VolumeMounts},
			{Moniker: "mon fs init mounts", Mounts: pod.Spec.InitContainers[2].VolumeMounts},
			{Moniker: "mon daemon mounts", Mounts: pod.Spec.Containers[0].VolumeMounts}},
	}
	volsMountsTestDef.TestMountsMatchVolumes(t)

	assert.Equal(t, "100", cont.Resources.Limits.Cpu().String())
	assert.Equal(t, "1337", cont.Resources.Requests.Memory().String())
}
