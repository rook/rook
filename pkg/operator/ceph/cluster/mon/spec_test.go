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
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/ceph/config"
	cephtest "github.com/rook/rook/pkg/operator/ceph/test"
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
		false,
		metav1.OwnerReference{},
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
	monConfig := testGenMonConfig(monID)

	d := c.makeDeployment(monConfig, "node0")
	assert.NotNil(t, d)

	// Deployment should have Ceph labels
	cephtest.AssertLabelsContainCephRequirements(t, d.ObjectMeta.Labels,
		config.MonType, monID, appName, "ns")

	podTemplate := cephtest.NewPodTemplateSpecTester(t, &d.Spec.Template)
	podTemplate.RunFullSuite(config.MonType, monID, appName, "ns", "ceph/ceph:myceph",
		"200", "100", "1337", "500" /* resources */)
}
