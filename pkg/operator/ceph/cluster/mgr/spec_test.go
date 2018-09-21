/*
Copyright 2018 The Rook Authors. All rights reserved.

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

package mgr

import (
	"fmt"
	"strconv"
	"testing"

	cephv1beta1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1beta1"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	mgrdaemon "github.com/rook/rook/pkg/daemon/ceph/mgr"
	cephtest "github.com/rook/rook/pkg/operator/ceph/test"
	"github.com/rook/rook/pkg/operator/k8sutil"
	optest "github.com/rook/rook/pkg/operator/test"
	testop "github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPodSpec(t *testing.T) {
	c := New(
		&clusterd.Context{Clientset: testop.New(1)},
		"ns",
		"rook/rook:myversion",
		rookalpha.Placement{},
		false,
		cephv1beta1.DashboardSpec{},
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

	mgrTestConfig := mgrConfig{
		DaemonName:   "a",
		ResourceName: "mgr-a",
	}

	d := c.makeDeployment(&mgrTestConfig)

	assert.NotNil(t, d)
	assert.Equal(t, "mgr-a", d.Name)
	assert.Equal(t, "mgr-a", d.ObjectMeta.Name)
	assert.Equal(t, 0, len(d.ObjectMeta.Annotations))

	pod := d.Spec.Template
	assert.Equal(t, appName, pod.ObjectMeta.Labels["app"])
	assert.Equal(t, c.Namespace, pod.ObjectMeta.Labels["rook_cluster"])
	assert.Equal(t, 2, len(pod.ObjectMeta.Annotations))
	assert.Equal(t, "true", pod.ObjectMeta.Annotations["prometheus.io/scrape"])
	assert.Equal(t, strconv.Itoa(metricsPort), pod.ObjectMeta.Annotations["prometheus.io/port"])
	assert.Equal(t, v1.RestartPolicyAlways, pod.Spec.RestartPolicy)
	assert.Nil(t, optest.VolumeExists("rook-data", pod.Spec.Volumes))
	assert.Nil(t, optest.VolumeExists(cephconfig.DefaultConfigMountName, pod.Spec.Volumes))
	assert.Nil(t, optest.VolumeExists(k8sutil.ConfigOverrideName, pod.Spec.Volumes))

	assert.Equal(t, 1, len(pod.Spec.InitContainers))
	assert.Equal(t, 1, len(pod.Spec.Containers))

	configImage := "rook/rook:myversion"
	configEnvs := 8
	configContainerDefinition := cephtest.ContainerTestDefinition{
		Image:   &configImage,
		Command: []string{}, // no command
		Args: [][]string{
			{"ceph"},
			{mgrdaemon.InitCommand},
			{"--config-dir=/var/lib/rook"},
			{fmt.Sprintf("--mgr-name=%s", mgrTestConfig.DaemonName)}},
		InOrderArgs: map[int]string{
			0: "ceph",                 // ceph must be first arg
			1: mgrdaemon.InitCommand}, // mgr init command must be second arg
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

	daemonImage := "rook/rook:myversion"
	daemonEnvs := 0
	daemonContainerDefinition := cephtest.ContainerTestDefinition{
		Image: &daemonImage,
		Command: []string{
			"ceph-mgr"},
		Args: [][]string{
			{"--foreground"},
			{"--id", mgrTestConfig.DaemonName}},
		VolumeMountNames: []string{
			"rook-data",
			cephconfig.DefaultConfigMountName},
		EnvCount: &daemonEnvs,
		Ports: []v1.ContainerPort{
			{ContainerPort: int32(6800),
				Protocol: v1.ProtocolTCP},
			{ContainerPort: int32(metricsPort),
				Protocol: v1.ProtocolTCP},
			{ContainerPort: int32(dashboardPort),
				Protocol: v1.ProtocolTCP}},
		IsPrivileged: nil, // not set in spec
	}
	cont = &pod.Spec.Containers[0]
	daemonContainerDefinition.TestContainer(t, "main mon daemon", cont, logger)
	assert.Equal(t, "100", cont.Resources.Limits.Cpu().String())
	assert.Equal(t, "1337", cont.Resources.Requests.Memory().String())

	// Verify that all the mounts have volumes and that there are no extraneous volumes
	volsMountsTestDef := optest.VolumesAndMountsTestDefinition{
		VolumesSpec: &optest.VolumesSpec{Moniker: "mon pod volumes", Volumes: pod.Spec.Volumes},
		MountsSpecItems: []*optest.MountsSpec{
			{Moniker: "mgr config init mounts", Mounts: pod.Spec.InitContainers[0].VolumeMounts},
			{Moniker: "mgr daemon mounts", Mounts: pod.Spec.Containers[0].VolumeMounts}},
	}
	volsMountsTestDef.TestMountsMatchVolumes(t)
}

func TestServiceSpec(t *testing.T) {
	c := New(&clusterd.Context{}, "ns", "myversion", rookalpha.Placement{}, false, cephv1beta1.DashboardSpec{}, v1.ResourceRequirements{}, metav1.OwnerReference{})

	s := c.makeMetricsService("rook-mgr")
	assert.NotNil(t, s)
	assert.Equal(t, "rook-mgr", s.Name)
	assert.Equal(t, 1, len(s.Spec.Ports))
}

func TestHostNetwork(t *testing.T) {
	c := New(
		&clusterd.Context{Clientset: testop.New(1)},
		"ns",
		"myversion",
		rookalpha.Placement{},
		true,
		cephv1beta1.DashboardSpec{},
		v1.ResourceRequirements{},
		metav1.OwnerReference{},
	)

	mgrTestConfig := mgrConfig{
		DaemonName:   "a",
		ResourceName: "mgr-a",
	}

	d := c.makeDeployment(&mgrTestConfig)
	assert.NotNil(t, d)

	assert.Equal(t, true, d.Spec.Template.Spec.HostNetwork)
	assert.Equal(t, v1.DNSClusterFirstWithHostNet, d.Spec.Template.Spec.DNSPolicy)
}
