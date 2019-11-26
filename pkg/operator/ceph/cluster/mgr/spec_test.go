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
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
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
	clusterInfo := &cephconfig.ClusterInfo{FSID: "myfsid"}
	c := New(
		clusterInfo,
		&clusterd.Context{Clientset: optest.New(1)},
		"ns",
		"rook/rook:myversion",
		cephv1.CephVersionSpec{Image: "ceph/ceph:myceph"},
		rookalpha.Placement{},
		rookalpha.Annotations{},
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
	clusterInfo := &cephconfig.ClusterInfo{FSID: "myfsid"}
	c := New(
		clusterInfo,
		&clusterd.Context{Clientset: optest.New(1)},
		"ns",
		"myversion",
		cephv1.CephVersionSpec{},
		rookalpha.Placement{},
		rookalpha.Annotations{},
		cephv1.NetworkSpec{},
		cephv1.DashboardSpec{},
		cephv1.MonitoringSpec{},
		cephv1.MgrSpec{},
		v1.ResourceRequirements{},
		"my-priority-class",
		metav1.OwnerReference{},
		"/var/lib/rook/",
		false,
		false,
	)

	s := c.makeMetricsService("rook-mgr")
	assert.NotNil(t, s)
	assert.Equal(t, "rook-mgr", s.Name)
	assert.Equal(t, 1, len(s.Spec.Ports))
}

func TestHostNetwork(t *testing.T) {
	clusterInfo := &cephconfig.ClusterInfo{FSID: "myfsid"}
	c := New(
		clusterInfo,
		&clusterd.Context{Clientset: optest.New(1)},
		"ns",
		"myversion",
		cephv1.CephVersionSpec{},
		rookalpha.Placement{},
		rookalpha.Annotations{},
		cephv1.NetworkSpec{HostNetwork: true},
		cephv1.DashboardSpec{Port: 1234},
		cephv1.MonitoringSpec{},
		cephv1.MgrSpec{},
		v1.ResourceRequirements{},
		"my-priority-class",
		metav1.OwnerReference{},
		"/var/lib/rook/",
		false,
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
	clusterInfo := &cephconfig.ClusterInfo{FSID: "myfsid"}
	c := New(
		clusterInfo,
		&clusterd.Context{Clientset: optest.New(1)},
		"ns",
		"myversion",
		cephv1.CephVersionSpec{},
		rookalpha.Placement{},
		rookalpha.Annotations{},
		cephv1.NetworkSpec{},
		cephv1.DashboardSpec{Port: 1234},
		cephv1.MonitoringSpec{},
		cephv1.MgrSpec{},
		v1.ResourceRequirements{},
		"my-priority-class",
		metav1.OwnerReference{},
		"/var/lib/rook/",
		false,
		false,
	)

	mgrTestConfig := mgrConfig{
		DaemonID:     "a",
		ResourceName: "mgr-a",
		DataPathMap:  config.NewStatelessDaemonDataPathMap(config.MgrType, "a", "rook-ceph", "/var/lib/rook/"),
	}

	vers := []struct {
		hasFix bool
		ver    cephver.CephVersion
	}{
		// versions before the fix was introduced
		{hasFix: false, ver: cephver.CephVersion{Major: 11, Minor: 2, Extra: 1}},
		{hasFix: false, ver: cephver.CephVersion{Major: 12, Minor: 2, Extra: 11}},
		{hasFix: false, ver: cephver.CephVersion{Major: 13, Minor: 2, Extra: 5}},
		{hasFix: false, ver: cephver.CephVersion{Major: 14, Minor: 1, Extra: 0}},

		// versions when the fix was introduced
		{hasFix: true, ver: cephver.CephVersion{Major: 13, Minor: 2, Extra: 6}},
		{hasFix: true, ver: cephver.CephVersion{Major: 14, Minor: 1, Extra: 1}},

		// versions after the fix
		{hasFix: true, ver: cephver.CephVersion{Major: 13, Minor: 2, Extra: 7}},
		{hasFix: true, ver: cephver.CephVersion{Major: 14, Minor: 1, Extra: 2}},
		{hasFix: true, ver: cephver.CephVersion{Major: 15, Minor: 2, Extra: 0}},
	}

	for _, test := range vers {
		c.clusterInfo.CephVersion = test.ver

		expectedInitContainers := 1
		if !test.hasFix {
			expectedInitContainers += 2
		}

		d := c.makeDeployment(&mgrTestConfig)
		assert.NotNil(t, d)
		assert.Equal(t, expectedInitContainers,
			len(d.Spec.Template.Spec.InitContainers))
	}
}

func TestApplyPrometheusAnnotations(t *testing.T) {
	c := New(
		&cephconfig.ClusterInfo{FSID: "myfsid"},
		&clusterd.Context{Clientset: optest.New(1)},
		"ns",
		"myversion",
		cephv1.CephVersionSpec{},
		rookalpha.Placement{},
		rookalpha.Annotations{},
		cephv1.NetworkSpec{},
		cephv1.DashboardSpec{},
		cephv1.MonitoringSpec{},
		cephv1.MgrSpec{},
		v1.ResourceRequirements{},
		"my-priority-class",
		metav1.OwnerReference{},
		"/var/lib/rook/",
		false,
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

	fakeAnnotations := rookalpha.Annotations{
		"foo.io/bar": "foobar",
	}
	c.annotations = fakeAnnotations

	c.applyPrometheusAnnotations(&d.ObjectMeta)
	assert.Equal(t, 1, len(c.annotations))
	assert.Equal(t, 0, len(d.ObjectMeta.Annotations))
}
