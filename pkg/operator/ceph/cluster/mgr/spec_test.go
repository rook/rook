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

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookv1 "github.com/rook/rook/pkg/apis/rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/config"
	cephtest "github.com/rook/rook/pkg/operator/ceph/test"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	optest "github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPodSpec(t *testing.T) {
	clientset := optest.New(t, 1)
	clusterInfo := &cephconfig.ClusterInfo{FSID: "myfsid"}
	c := New(
		clusterInfo,
		&clusterd.Context{Clientset: clientset},
		"ns",
		"rook/rook:myversion",
		cephv1.CephVersionSpec{Image: "ceph/ceph:myceph"},
		rookv1.Placement{},
		rookv1.Annotations{},
		cephv1.NetworkSpec{},
		cephv1.DashboardSpec{Port: 1234},
		cephv1.MonitoringSpec{},
		cephv1.MgrSpec{},
		v1.ResourceRequirements{
			Limits: v1.ResourceList{
				v1.ResourceCPU:    *resource.NewQuantity(200.0, resource.BinarySI),
				v1.ResourceMemory: *resource.NewQuantity(500.0, resource.BinarySI),
			},
			Requests: v1.ResourceList{
				v1.ResourceCPU:    *resource.NewQuantity(100.0, resource.BinarySI),
				v1.ResourceMemory: *resource.NewQuantity(250.0, resource.BinarySI),
			},
		},
		"my-priority-class",
		metav1.OwnerReference{},
		"/var/lib/rook/",
		false,
	)

	mgrTestConfig := mgrConfig{
		DaemonID:     "a",
		ResourceName: "rook-ceph-mgr-a",
		DataPathMap:  config.NewStatelessDaemonDataPathMap(config.MgrType, "a", "rook-ceph", "/var/lib/rook/"),
	}

	d := c.makeDeployment(&mgrTestConfig)

	// Deployment should have Ceph labels
	cephtest.AssertLabelsContainCephRequirements(t, d.ObjectMeta.Labels,
		config.MgrType, "a", AppName, "ns")

	podTemplate := cephtest.NewPodTemplateSpecTester(t, &d.Spec.Template)
	podTemplate.Spec().Containers().RequireAdditionalEnvVars(
		"ROOK_OPERATOR_NAMESPACE", "ROOK_CEPH_CLUSTER_CRD_VERSION",
		"ROOK_CEPH_CLUSTER_CRD_NAME")
	podTemplate.RunFullSuite(config.MgrType, "a", AppName, "ns", "ceph/ceph:myceph",
		"200", "100", "500", "250", /* resources */
		"my-priority-class")
	assert.Equal(t, 2, len(d.Spec.Template.Annotations))

}

func TestServiceSpec(t *testing.T) {
	clientset := optest.New(t, 1)
	clusterInfo := &cephconfig.ClusterInfo{FSID: "myfsid"}
	c := New(
		clusterInfo,
		&clusterd.Context{Clientset: clientset},
		"ns",
		"myversion",
		cephv1.CephVersionSpec{},
		rookv1.Placement{},
		rookv1.Annotations{},
		cephv1.NetworkSpec{},
		cephv1.DashboardSpec{},
		cephv1.MonitoringSpec{},
		cephv1.MgrSpec{},
		v1.ResourceRequirements{},
		"my-priority-class",
		metav1.OwnerReference{},
		"/var/lib/rook/",
		false,
	)

	s := c.makeMetricsService("rook-mgr")
	assert.NotNil(t, s)
	assert.Equal(t, "rook-mgr", s.Name)
	assert.Equal(t, 1, len(s.Spec.Ports))
}

func TestHostNetwork(t *testing.T) {
	clientset := optest.New(t, 1)
	clusterInfo := &cephconfig.ClusterInfo{FSID: "myfsid"}
	c := New(
		clusterInfo,
		&clusterd.Context{Clientset: clientset},
		"ns",
		"myversion",
		cephv1.CephVersionSpec{},
		rookv1.Placement{},
		rookv1.Annotations{},
		cephv1.NetworkSpec{HostNetwork: true},
		cephv1.DashboardSpec{Port: 1234},
		cephv1.MonitoringSpec{},
		cephv1.MgrSpec{},
		v1.ResourceRequirements{},
		"my-priority-class",
		metav1.OwnerReference{},
		"/var/lib/rook/",
		false,
	)

	mgrTestConfig := mgrConfig{
		DaemonID:     "a",
		ResourceName: "mgr-a",
		DataPathMap:  config.NewStatelessDaemonDataPathMap(config.MgrType, "a", "rook-ceph", "/var/lib/rook/"),
	}

	d := c.makeDeployment(&mgrTestConfig)
	assert.NotNil(t, d)

	assert.Equal(t, true, c.Network.IsHost())
	assert.Equal(t, v1.DNSClusterFirstWithHostNet, d.Spec.Template.Spec.DNSPolicy)
}

func TestHttpBindFix(t *testing.T) {
	clientset := optest.New(t, 1)
	clusterInfo := &cephconfig.ClusterInfo{FSID: "myfsid"}
	c := New(
		clusterInfo,
		&clusterd.Context{Clientset: clientset},
		"ns",
		"myversion",
		cephv1.CephVersionSpec{},
		rookv1.Placement{},
		rookv1.Annotations{},
		cephv1.NetworkSpec{},
		cephv1.DashboardSpec{Port: 1234},
		cephv1.MonitoringSpec{},
		cephv1.MgrSpec{},
		v1.ResourceRequirements{},
		"my-priority-class",
		metav1.OwnerReference{},
		"/var/lib/rook/",
		false,
	)

	mgrTestConfig := mgrConfig{
		DaemonID:     "a",
		ResourceName: "mgr-a",
		DataPathMap:  config.NewStatelessDaemonDataPathMap(config.MgrType, "a", "rook-ceph", "/var/lib/rook/"),
	}

	c.clusterInfo.CephVersion = cephver.Nautilus
	expectedInitContainers := 3
	d := c.makeDeployment(&mgrTestConfig)
	assert.NotNil(t, d)
	assert.Equal(t, expectedInitContainers,
		len(d.Spec.Template.Spec.InitContainers))

}

func TestApplyPrometheusAnnotations(t *testing.T) {
	clientset := optest.New(t, 1)
	c := New(
		&cephconfig.ClusterInfo{FSID: "myfsid"},
		&clusterd.Context{Clientset: clientset},
		"ns",
		"myversion",
		cephv1.CephVersionSpec{},
		rookv1.Placement{},
		rookv1.Annotations{},
		cephv1.NetworkSpec{},
		cephv1.DashboardSpec{},
		cephv1.MonitoringSpec{},
		cephv1.MgrSpec{},
		v1.ResourceRequirements{},
		"my-priority-class",
		metav1.OwnerReference{},
		"/var/lib/rook/",
		false,
	)

	mgrTestConfig := mgrConfig{
		DaemonID:     "a",
		ResourceName: "rook-ceph-mgr-a",
		DataPathMap:  config.NewStatelessDaemonDataPathMap(config.MgrType, "a", "rook-ceph", "/var/lib/rook/"),
	}

	d := c.makeDeployment(&mgrTestConfig)

	// Test without annotations
	c.applyPrometheusAnnotations(&d.ObjectMeta)
	assert.Equal(t, 2, len(d.ObjectMeta.Annotations))

	// Test with existing annotations
	// applyPrometheusAnnotations() shouldn't do anything
	// re-initialize "d"
	d = c.makeDeployment(&mgrTestConfig)

	fakeAnnotations := rookv1.Annotations{
		"foo.io/bar": "foobar",
	}
	c.annotations = fakeAnnotations

	c.applyPrometheusAnnotations(&d.ObjectMeta)
	assert.Equal(t, 1, len(c.annotations))
	assert.Equal(t, 0, len(d.ObjectMeta.Annotations))
}
