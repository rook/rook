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
	"context"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/test"
	"github.com/rook/rook/pkg/operator/k8sutil"
	testop "github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestPodSpecs(t *testing.T) {
	testPodSpec(t, "a", true)
	testPodSpec(t, "mon0", true)
	testPodSpec(t, "a", false)
	testPodSpec(t, "mon0", false)
}

func testPodSpec(t *testing.T, monID string, pvc bool) {
	clientset := testop.New(t, 1)
	ownerInfo := cephclient.NewMinimumOwnerInfoWithOwnerRef()
	c := New(
		context.TODO(),
		&clusterd.Context{Clientset: clientset, ConfigDir: "/var/lib/rook"},
		"ns",
		cephv1.ClusterSpec{},
		ownerInfo,
	)
	setCommonMonProperties(c, 0, cephv1.MonSpec{Count: 3, AllowMultiplePerNode: true}, "rook/rook:myversion")
	c.spec.CephVersion = cephv1.CephVersionSpec{Image: "quay.io/ceph/ceph:myceph"}
	c.spec.Resources = map[string]v1.ResourceRequirements{}
	c.spec.DataDirHostPath = "/var/lib/rook"
	c.spec.Resources["mon"] = v1.ResourceRequirements{
		Limits: v1.ResourceList{
			v1.ResourceCPU:    *resource.NewQuantity(200.0, resource.BinarySI),
			v1.ResourceMemory: *resource.NewQuantity(1337.0, resource.BinarySI),
		},
		Requests: v1.ResourceList{
			v1.ResourceCPU:    *resource.NewQuantity(100.0, resource.BinarySI),
			v1.ResourceMemory: *resource.NewQuantity(500.0, resource.BinarySI),
		},
	}
	c.spec.PriorityClassNames = map[cephv1.KeyType]string{
		cephv1.KeyMon: "my-priority-class",
	}
	monConfig := testGenMonConfig(monID)

	d, err := c.makeDeployment(monConfig, false)
	assert.NoError(t, err)
	assert.NotNil(t, d)
	assert.Equal(t, k8sutil.DefaultServiceAccount, d.Spec.Template.Spec.ServiceAccountName)

	if pvc {
		d.Spec.Template.Spec.Volumes = append(
			d.Spec.Template.Spec.Volumes, controller.DaemonVolumesDataPVC("i-am-pvc"))
	} else {
		d.Spec.Template.Spec.Volumes = append(
			d.Spec.Template.Spec.Volumes, controller.DaemonVolumesDataHostPath(monConfig.DataPathMap)...)
	}

	// Deployment should have Ceph labels
	test.AssertLabelsContainCephRequirements(t, d.ObjectMeta.Labels,
		config.MonType, monID, AppName, "ns", "default", "cephclusters.ceph.rook.io", "ceph-mon")

	podTemplate := test.NewPodTemplateSpecTester(t, &d.Spec.Template)
	podTemplate.RunFullSuite(config.MonType, monID, AppName, "ns", "quay.io/ceph/ceph:myceph",
		"200", "100", "1337", "500", /* resources */
		"my-priority-class", "default", "cephclusters.ceph.rook.io", "ceph-mon")

	t.Run(("check mon ConfigureProbe"), func(t *testing.T) {
		c.spec.HealthCheck.StartupProbe = make(map[cephv1.KeyType]*cephv1.ProbeSpec)
		c.spec.HealthCheck.StartupProbe[cephv1.KeyMon] = &cephv1.ProbeSpec{Disabled: false, Probe: &v1.Probe{InitialDelaySeconds: 1000}}
		c.spec.HealthCheck.LivenessProbe = make(map[cephv1.KeyType]*cephv1.ProbeSpec)
		c.spec.HealthCheck.LivenessProbe[cephv1.KeyMon] = &cephv1.ProbeSpec{Disabled: false, Probe: &v1.Probe{InitialDelaySeconds: 900}}
		container := c.makeMonDaemonContainer(monConfig)
		assert.NotNil(t, container.LivenessProbe)
		assert.NotNil(t, container.StartupProbe)
		assert.Equal(t, int32(900), container.LivenessProbe.InitialDelaySeconds)
		assert.Equal(t, int32(1000), container.StartupProbe.InitialDelaySeconds)
	})

	t.Run(("msgr2 not required"), func(t *testing.T) {
		container := c.makeMonDaemonContainer(monConfig)
		checkMsgr2Required(t, container, false)
	})

	t.Run(("require msgr2"), func(t *testing.T) {
		monConfig.Port = DefaultMsgr2Port
		container := c.makeMonDaemonContainer(monConfig)
		checkMsgr2Required(t, container, true)
	})
}

func checkMsgr2Required(t *testing.T, container v1.Container, expectedRequireMsgr2 bool) {
	foundDisabledMsgr1 := false
	foundMsgr2Port := false
	for _, arg := range container.Args {
		if arg == "--ms-bind-msgr1=false" {
			foundDisabledMsgr1 = true
		}
		if arg == "--public-bind-addr=$(ROOK_POD_IP):3300" {
			foundMsgr2Port = true
		}
	}
	assert.Equal(t, expectedRequireMsgr2, foundDisabledMsgr1)
	assert.Equal(t, expectedRequireMsgr2, foundMsgr2Port)
}

func TestDeploymentPVCSpec(t *testing.T) {
	clientset := testop.New(t, 1)
	ownerInfo := cephclient.NewMinimumOwnerInfoWithOwnerRef()
	c := New(
		context.TODO(),
		&clusterd.Context{Clientset: clientset, ConfigDir: "/var/lib/rook"},
		"ns",
		cephv1.ClusterSpec{},
		ownerInfo,
	)
	setCommonMonProperties(c, 0, cephv1.MonSpec{Count: 3, AllowMultiplePerNode: true}, "rook/rook:myversion")
	c.spec.CephVersion = cephv1.CephVersionSpec{Image: "quay.io/ceph/ceph:myceph"}
	c.spec.Resources = map[string]v1.ResourceRequirements{}
	c.spec.Resources["mon"] = v1.ResourceRequirements{
		Limits: v1.ResourceList{
			v1.ResourceCPU:    *resource.NewQuantity(200.0, resource.BinarySI),
			v1.ResourceMemory: *resource.NewQuantity(1337.0, resource.BinarySI),
		},
		Requests: v1.ResourceList{
			v1.ResourceCPU:    *resource.NewQuantity(100.0, resource.BinarySI),
			v1.ResourceMemory: *resource.NewQuantity(500.0, resource.BinarySI),
		},
	}
	monConfig := testGenMonConfig("a")

	// configured with default storage request
	c.spec.Mon.VolumeClaimTemplate = &cephv1.VolumeClaimTemplate{}
	pvc, err := c.makeDeploymentPVC(monConfig, false)
	assert.NoError(t, err)
	defaultReq, err := resource.ParseQuantity(cephMonDefaultStorageRequest)
	assert.NoError(t, err)
	assert.Equal(t, pvc.Spec.Resources.Requests[v1.ResourceStorage], defaultReq)

	// limit is preserved
	req, err := resource.ParseQuantity("22Gi")
	assert.NoError(t, err)
	c.spec.Mon.VolumeClaimTemplate = &cephv1.VolumeClaimTemplate{
		Spec: v1.PersistentVolumeClaimSpec{
			Resources: v1.VolumeResourceRequirements{
				Limits: v1.ResourceList{v1.ResourceStorage: req},
			},
		},
	}
	pvc, err = c.makeDeploymentPVC(monConfig, false)
	assert.NoError(t, err)
	assert.Equal(t, pvc.Spec.Resources.Limits[v1.ResourceStorage], req)

	// request is preserved
	req, err = resource.ParseQuantity("23Gi")
	assert.NoError(t, err)
	c.spec.Mon.VolumeClaimTemplate = &cephv1.VolumeClaimTemplate{
		Spec: v1.PersistentVolumeClaimSpec{
			Resources: v1.VolumeResourceRequirements{
				Requests: v1.ResourceList{v1.ResourceStorage: req},
			},
		},
	}
	pvc, err = c.makeDeploymentPVC(monConfig, false)
	assert.NoError(t, err)
	assert.Equal(t, pvc.Spec.Resources.Requests[v1.ResourceStorage], req)
}

func testRequiredDuringScheduling(t *testing.T, hostNetwork, allowMultiplePerNode, required bool) {
	c := New(
		context.TODO(),
		&clusterd.Context{},
		"ns",
		cephv1.ClusterSpec{},
		&k8sutil.OwnerInfo{},
	)

	c.spec.Network.HostNetwork = hostNetwork
	c.spec.Mon.AllowMultiplePerNode = allowMultiplePerNode
	assert.Equal(t, required, requiredDuringScheduling(&c.spec))
}

func TestRequiredDuringScheduling(t *testing.T) {
	testRequiredDuringScheduling(t, false, false, true)
	testRequiredDuringScheduling(t, true, false, true)
	testRequiredDuringScheduling(t, true, true, true)
	testRequiredDuringScheduling(t, false, true, false)
}
