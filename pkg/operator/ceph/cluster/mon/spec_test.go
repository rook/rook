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
	"path"
	"strings"
	"testing"

	"github.com/rook/rook/pkg/operator/ceph/config"

	"github.com/rook/rook/pkg/operator/ceph/spec"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	test_opceph "github.com/rook/rook/pkg/operator/ceph/test"
	testop "github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPodSpecs(t *testing.T) {
	testPodSpec(t, "a")
	testPodSpec(t, "mon0")
}

func testPodSpec(t *testing.T, monID string) {
	clientset := testop.New(1)
	c := New(
		&clusterd.Context{Clientset: clientset, ConfigDir: "/var/lib/rook"},
		"ns",
		"/var/lib/rook",
		"rook/rook:myversion",
		cephv1.CephVersionSpec{Image: "ceph/ceph:myceph"},
		cephv1.MonSpec{Count: 3, AllowMultiplePerNode: true},
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
	monConfig := testGenMonConfig(monID)

	pod := c.makeMonPod(monConfig, "foo")
	assert.NotNil(t, pod)
	assert.Equal(t, monConfig.ResourceName, pod.Name)
	assert.Equal(t, v1.RestartPolicyAlways, pod.Spec.RestartPolicy)
	assert.Equal(t, 3, len(pod.Spec.Volumes))
	assert.Nil(t, testop.VolumeExists("rook-ceph-config", pod.Spec.Volumes))       // ceph.conf
	assert.Nil(t, testop.VolumeExists("rook-ceph-mons-keyring", pod.Spec.Volumes)) // mon shared keyring
	if strings.HasPrefix(monID, "mon") {
		// is legacy mon id (mon0, mon1, ...)
		assert.Nil(t, testop.VolumeIsHostPath("ceph-daemon-data", path.Join("/var/lib/rook/", monID, "data"), pod.Spec.Volumes))
	} else {
		// is new mon id (a, b, c, ...)
		assert.Nil(t, testop.VolumeIsHostPath("ceph-daemon-data", path.Join("/var/lib/rook/", "mon-"+monID, "data"), pod.Spec.Volumes))
	}

	assert.Equal(t, monConfig.ResourceName, pod.ObjectMeta.Name)
	assert.Nil(t, test_opceph.VerifyPodLabels("rook-ceph-mon", "ns", "mon", monConfig.DaemonName, pod.ObjectMeta.Labels))
	assert.Equal(t, c.Namespace, pod.ObjectMeta.Labels["mon_cluster"])

	assert.Equal(t, 1, len(pod.Spec.InitContainers))
	assert.Equal(t, 1, len(pod.Spec.Containers))

	// All containers have the same privilege
	isPrivileged := false
	// All ceph images have the same image, basic envs, and the same volume mounts
	cephImage := "ceph/ceph:myceph"
	envVars := len(spec.DaemonEnvVars())
	cephVolumeMountNames := []string{
		"rook-ceph-config",
		"rook-ceph-mons-keyring",
		"ceph-daemon-data"}
	commonFlags := [][]string{}
	for _, f := range spec.DaemonFlags(c.clusterInfo, config.MonType, monID) {
		commonFlags = append(commonFlags, []string{f})
	}
	fmt.Println(commonFlags)

	// mon fs init container
	monFsInitContDef := test_opceph.ContainerTestDefinition{
		Image:   &cephImage,
		Command: []string{"ceph-mon"},
		Args: append(commonFlags,
			[]string{"--public-addr=2.4.6.1"},
			[]string{"--mkfs"}),
		VolumeMountNames: cephVolumeMountNames,
		EnvCount:         &envVars,
		Ports:            []v1.ContainerPort{},
		IsPrivileged:     &isPrivileged,
	}
	cont := &pod.Spec.InitContainers[0]
	monFsInitContDef.TestContainer(t, "config init", cont, logger)
	assert.Equal(t, "100", cont.Resources.Limits.Cpu().String())
	assert.Equal(t, "1337", cont.Resources.Requests.Memory().String())

	// main mon daemon container
	monDaemonEnvs := len(spec.DaemonEnvVars()) + 1
	monDaemonContDef := test_opceph.ContainerTestDefinition{
		Image: &cephImage,
		Command: []string{
			"ceph-mon"},
		Args: append(commonFlags,
			[]string{"--foreground"},
			[]string{"--public-addr=2.4.6.1"},
			[]string{"--public-bind-addr=$(ROOK_PRIVATE_IP)"}),
		VolumeMountNames: cephVolumeMountNames,
		EnvCount:         &monDaemonEnvs,
		Ports: []v1.ContainerPort{
			{ContainerPort: monConfig.Port,
				Protocol: v1.ProtocolTCP}},
		IsPrivileged: &isPrivileged,
	}
	cont = &pod.Spec.Containers[0]
	monDaemonContDef.TestContainer(t, "mon", cont, logger)
	assert.Equal(t, "100", cont.Resources.Limits.Cpu().String())
	assert.Equal(t, "1337", cont.Resources.Requests.Memory().String())

	// Verify that all the mounts have volumes and that there are no extraneous volumes
	volsMountsTestDef := testop.VolumesAndMountsTestDefinition{
		VolumesSpec: &testop.VolumesSpec{Moniker: "mon pod volumes", Volumes: pod.Spec.Volumes},
		MountsSpecItems: []*testop.MountsSpec{
			{Moniker: "mon fs init mounts", Mounts: pod.Spec.InitContainers[0].VolumeMounts},
			{Moniker: "mon daemon mounts", Mounts: pod.Spec.Containers[0].VolumeMounts}},
	}
	volsMountsTestDef.TestMountsMatchVolumes(t)

	assert.Equal(t, "100", cont.Resources.Limits.Cpu().String())
	assert.Equal(t, "1337", cont.Resources.Requests.Memory().String())
}
