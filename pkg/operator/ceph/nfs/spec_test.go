/*
Copyright 2019 The Rook Authors. All rights reserved.

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

package nfs

import (
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/config"
	optest "github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDeploymentSpec(t *testing.T) {
	nfs := cephv1.CephNFS{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-nfs",
			Namespace: "rook-ceph-test-ns",
		},
		Spec: cephv1.NFSGaneshaSpec{
			RADOS: cephv1.GaneshaRADOSSpec{
				Pool:      "myfs-data0",
				Namespace: "nfs-test-ns",
			},
			Server: cephv1.GaneshaServerSpec{
				Active: 3,
				Resources: v1.ResourceRequirements{
					Limits: v1.ResourceList{
						v1.ResourceCPU:    *resource.NewQuantity(500.0, resource.BinarySI),
						v1.ResourceMemory: *resource.NewQuantity(1024.0, resource.BinarySI),
					},
					Requests: v1.ResourceList{
						v1.ResourceCPU:    *resource.NewQuantity(200.0, resource.BinarySI),
						v1.ResourceMemory: *resource.NewQuantity(512.0, resource.BinarySI),
					},
				},
				PriorityClassName: "my-priority-class",
			},
		},
	}

	clusterInfo := &cephconfig.ClusterInfo{FSID: "myfsid"}
	c := NewCephNFSController(
		clusterInfo,
		&clusterd.Context{Clientset: optest.New(1)},
		"/var/lib/rook",
		"rook-ceph-test-ns",
		"rook/rook:testimage",
		&cephv1.ClusterSpec{CephVersion: cephv1.CephVersionSpec{Image: "ceph/ceph:testversion"}},
		metav1.OwnerReference{},
	)

	id := "i"
	configName := "rook-ceph-nfs-my-nfs-i"
	cfg := daemonConfig{
		ID:              id,
		ConfigConfigMap: configName,
		DataPathMap: &config.DataPathMap{
			HostDataDir:        "",                          // nfs daemon does not store data on host, ...
			ContainerDataDir:   cephconfig.DefaultConfigDir, // does share data in containers using emptyDir, ...
			HostLogAndCrashDir: "",                          // and does not log to /var/log/ceph dir nor creates crash dumps
		},
	}

	d := c.makeDeployment(nfs, cfg)

	// Deployment should have Ceph labels
	optest.AssertLabelsContainRookRequirements(t, d.ObjectMeta.Labels, appName)

	podTemplate := optest.NewPodTemplateSpecTester(t, &d.Spec.Template)
	podTemplate.RunFullSuite(
		appName,
		optest.ResourceLimitExpectations{
			CPUResourceLimit:      "500",
			MemoryResourceLimit:   "1Ki",
			CPUResourceRequest:    "200",
			MemoryResourceRequest: "512",
		},
	)
	assert.Equal(t, "my-priority-class", d.Spec.Template.Spec.PriorityClassName)
}
