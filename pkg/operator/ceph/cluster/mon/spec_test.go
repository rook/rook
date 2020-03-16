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
	"sync"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookv1 "github.com/rook/rook/pkg/apis/rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/controller"
	cephtest "github.com/rook/rook/pkg/operator/ceph/test"
	testop "github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPodSpecs(t *testing.T) {
	testPodSpec(t, "a", true)
	testPodSpec(t, "mon0", true)
	testPodSpec(t, "a", false)
	testPodSpec(t, "mon0", false)
}

func testPodSpec(t *testing.T, monID string, pvc bool) {
	clientset := testop.New(t, 1)
	c := New(
		&clusterd.Context{Clientset: clientset, ConfigDir: "/var/lib/rook"},
		"ns",
		"/var/lib/rook",
		cephv1.NetworkSpec{},
		metav1.OwnerReference{},
		&sync.Mutex{},
	)
	setCommonMonProperties(c, 0, cephv1.MonSpec{Count: 3, AllowMultiplePerNode: true}, "rook/rook:myversion")
	c.spec.CephVersion = cephv1.CephVersionSpec{Image: "ceph/ceph:myceph"}
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
	c.spec.PriorityClassNames = map[rookv1.KeyType]string{
		cephv1.KeyMon: "my-priority-class",
	}
	monConfig := testGenMonConfig(monID)

	d := c.makeDeployment(monConfig, false)
	assert.NotNil(t, d)

	if pvc {
		d.Spec.Template.Spec.Volumes = append(
			d.Spec.Template.Spec.Volumes, controller.DaemonVolumesDataPVC("i-am-pvc"))
	} else {
		d.Spec.Template.Spec.Volumes = append(
			d.Spec.Template.Spec.Volumes, controller.DaemonVolumesDataHostPath(monConfig.DataPathMap)...)
	}

	// Deployment should have Ceph labels
	cephtest.AssertLabelsContainCephRequirements(t, d.ObjectMeta.Labels,
		config.MonType, monID, AppName, "ns")

	podTemplate := cephtest.NewPodTemplateSpecTester(t, &d.Spec.Template)
	podTemplate.RunFullSuite(config.MonType, monID, AppName, "ns", "ceph/ceph:myceph",
		"200", "100", "1337", "500", /* resources */
		"my-priority-class")
}

func TestDeploymentPVCSpec(t *testing.T) {
	clientset := testop.New(t, 1)
	c := New(
		&clusterd.Context{Clientset: clientset, ConfigDir: "/var/lib/rook"},
		"ns",
		"/var/lib/rook",
		cephv1.NetworkSpec{},
		metav1.OwnerReference{},
		&sync.Mutex{},
	)
	setCommonMonProperties(c, 0, cephv1.MonSpec{Count: 3, AllowMultiplePerNode: true}, "rook/rook:myversion")
	c.spec.CephVersion = cephv1.CephVersionSpec{Image: "ceph/ceph:myceph"}
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
	c.spec.Mon.VolumeClaimTemplate = &v1.PersistentVolumeClaim{}
	pvc, err := c.makeDeploymentPVC(monConfig, false)
	assert.NoError(t, err)
	defaultReq, err := resource.ParseQuantity(cephMonDefaultStorageRequest)
	assert.NoError(t, err)
	assert.Equal(t, pvc.Spec.Resources.Requests[v1.ResourceStorage], defaultReq)

	// limit is preserved
	req, err := resource.ParseQuantity("22Gi")
	assert.NoError(t, err)
	c.spec.Mon.VolumeClaimTemplate = &v1.PersistentVolumeClaim{
		Spec: v1.PersistentVolumeClaimSpec{
			Resources: v1.ResourceRequirements{
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
	c.spec.Mon.VolumeClaimTemplate = &v1.PersistentVolumeClaim{
		Spec: v1.PersistentVolumeClaimSpec{
			Resources: v1.ResourceRequirements{
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
		&clusterd.Context{},
		"ns",
		"/var/lib/rook",
		cephv1.NetworkSpec{},
		metav1.OwnerReference{},
		&sync.Mutex{},
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
