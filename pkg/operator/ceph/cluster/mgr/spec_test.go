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
	"testing"

	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/stretchr/testify/assert"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	cephtest "github.com/rook/rook/pkg/operator/ceph/test"
	optest "github.com/rook/rook/pkg/operator/test"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPodSpec(t *testing.T) {
	c := New(
		&clusterd.Context{Clientset: optest.New(1)},
		"ns",
		"rook/rook:myversion",
		cephv1.CephVersionSpec{Image: "ceph/ceph:myceph"},
		rookalpha.Placement{},
		false,
		cephv1.DashboardSpec{},
		v1.ResourceRequirements{
			Limits: v1.ResourceList{
				v1.ResourceCPU: *resource.NewQuantity(100.0, resource.BinarySI),
			},
			Requests: v1.ResourceList{
				v1.ResourceMemory: *resource.NewQuantity(1337.0, resource.BinarySI),
			},
		},
		metav1.OwnerReference{},
		optest.CreateConfigDir(1),
	)

	mgrTestConfig := mgrConfig{
		DaemonID:      "a",
		ResourceName:  "rook-ceph-mgr-a",
		DashboardPort: 1234,
		DataPathMap:   config.NewStatelessDaemonDataPathMap(config.MgrType, "a"),
	}

	d := c.makeDeployment(&mgrTestConfig)

	// Deployment should have Ceph labels
	cephtest.AssertLabelsContainCephRequirements(t, d.ObjectMeta.Labels,
		config.MgrType, "a", appName, "ns")

	podTemplate := cephtest.NewPodTemplateSpecTester(t, &d.Spec.Template)
	podTemplate.RunFullSuite(config.MgrType, "a", appName, "ns", "ceph/ceph:myceph",
		"100", "1337" /* resources */)

}

func TestServiceSpec(t *testing.T) {
	c := New(
		&clusterd.Context{Clientset: optest.New(1)},
		"ns",
		"myversion",
		cephv1.CephVersionSpec{},
		rookalpha.Placement{},
		false,
		cephv1.DashboardSpec{},
		v1.ResourceRequirements{},
		metav1.OwnerReference{},
		optest.CreateConfigDir(1),
	)

	s := c.makeMetricsService("rook-mgr")
	assert.NotNil(t, s)
	assert.Equal(t, "rook-mgr", s.Name)
	assert.Equal(t, 1, len(s.Spec.Ports))
}

func TestHostNetwork(t *testing.T) {
	c := New(
		&clusterd.Context{Clientset: optest.New(1)},
		"ns",
		"myversion",
		cephv1.CephVersionSpec{},
		rookalpha.Placement{},
		true,
		cephv1.DashboardSpec{},
		v1.ResourceRequirements{},
		metav1.OwnerReference{},
		optest.CreateConfigDir(1),
	)

	mgrTestConfig := mgrConfig{
		DaemonID:      "a",
		ResourceName:  "mgr-a",
		DashboardPort: 1234,
		DataPathMap:   config.NewStatelessDaemonDataPathMap(config.MgrType, "a"),
	}

	d := c.makeDeployment(&mgrTestConfig)
	assert.NotNil(t, d)

	assert.Equal(t, true, d.Spec.Template.Spec.HostNetwork)
	assert.Equal(t, v1.DNSClusterFirstWithHostNet, d.Spec.Template.Spec.DNSPolicy)
}
