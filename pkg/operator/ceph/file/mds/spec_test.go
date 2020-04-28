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

package mds

import (
	"testing"

	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/rook/rook/pkg/operator/ceph/config"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"

	cephtest "github.com/rook/rook/pkg/operator/ceph/test"
	testop "github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func testDeploymentObject(t *testing.T, network cephv1.NetworkSpec) (*apps.Deployment, error) {
	fs := cephv1.CephFilesystem{
		ObjectMeta: metav1.ObjectMeta{Name: "myfs", Namespace: "ns"},
		Spec: cephv1.FilesystemSpec{
			MetadataServer: cephv1.MetadataServerSpec{
				ActiveCount:   1,
				ActiveStandby: false,
				Resources: v1.ResourceRequirements{
					Limits: v1.ResourceList{
						v1.ResourceCPU:    *resource.NewQuantity(500.0, resource.BinarySI),
						v1.ResourceMemory: *resource.NewQuantity(4337.0, resource.BinarySI),
					},
					Requests: v1.ResourceList{
						v1.ResourceCPU:    *resource.NewQuantity(250.0, resource.BinarySI),
						v1.ResourceMemory: *resource.NewQuantity(2169.0, resource.BinarySI),
					},
				},
				PriorityClassName: "my-priority-class",
			},
		},
	}
	clusterInfo := &cephconfig.ClusterInfo{
		FSID:        "myfsid",
		CephVersion: cephver.Nautilus,
	}
	clientset := testop.New(t, 1)

	c := NewCluster(
		clusterInfo,
		&clusterd.Context{Clientset: clientset},
		&cephv1.ClusterSpec{
			CephVersion: cephv1.CephVersionSpec{Image: "ceph/ceph:testversion"},
			Network:     network,
		},
		fs,
		&client.CephFilesystemDetails{ID: 15},
		metav1.OwnerReference{},
		"/var/lib/rook/",
		scheme.Scheme,
	)
	mdsTestConfig := &mdsConfig{
		DaemonID:     "myfs-a",
		ResourceName: "rook-ceph-mds-myfs-a",
		DataPathMap:  config.NewStatelessDaemonDataPathMap(config.MdsType, "myfs-a", "rook-ceph", "/var/lib/rook/"),
	}
	return c.makeDeployment(mdsTestConfig), nil
}

func TestPodSpecs(t *testing.T) {
	d, err := testDeploymentObject(t, cephv1.NetworkSpec{HostNetwork: false}) // no host network
	assert.Nil(t, err)

	assert.NotNil(t, d)
	assert.Equal(t, v1.RestartPolicyAlways, d.Spec.Template.Spec.RestartPolicy)

	// Deployment should have Ceph labels
	cephtest.AssertLabelsContainCephRequirements(t, d.ObjectMeta.Labels,
		config.MdsType, "myfs-a", "rook-ceph-mds", "ns")

	podTemplate := cephtest.NewPodTemplateSpecTester(t, &d.Spec.Template)
	podTemplate.RunFullSuite(config.MdsType, "myfs-a", "rook-ceph-mds", "ns", "ceph/ceph:testversion",
		"500", "250", "4337", "2169", /* resources */
		"my-priority-class")
}

func TestHostNetwork(t *testing.T) {
	d, err := testDeploymentObject(t, cephv1.NetworkSpec{HostNetwork: true}) // host network
	assert.Nil(t, err)

	assert.Equal(t, true, d.Spec.Template.Spec.HostNetwork)
	assert.Equal(t, v1.DNSClusterFirstWithHostNet, d.Spec.Template.Spec.DNSPolicy)
}
