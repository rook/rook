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

package rbd

import (
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/config"
	cephtest "github.com/rook/rook/pkg/operator/ceph/test"
	optest "github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPodSpec(t *testing.T) {
	c := New(
		&cephconfig.ClusterInfo{FSID: "myfsid"},
		&clusterd.Context{Clientset: optest.New(1)},
		"ns",
		"rook/rook:myversion",
		cephv1.CephVersionSpec{Image: "ceph/ceph:myceph"},
		rookalpha.Placement{},
		rookalpha.Annotations{},
		cephv1.NetworkSpec{},
		cephv1.RBDMirroringSpec{Workers: 2},
		v1.ResourceRequirements{
			Limits: v1.ResourceList{
				v1.ResourceCPU:    *resource.NewQuantity(200.0, resource.BinarySI),
				v1.ResourceMemory: *resource.NewQuantity(600.0, resource.BinarySI),
			},
			Requests: v1.ResourceList{
				v1.ResourceCPU:    *resource.NewQuantity(100.0, resource.BinarySI),
				v1.ResourceMemory: *resource.NewQuantity(300.0, resource.BinarySI),
			},
		},
		"my-priority-class",
		metav1.OwnerReference{},
		"/var/lib/rook/",
		false,
		false,
	)
	daemonConf := daemonConfig{
		DaemonID:     "a",
		ResourceName: "rook-ceph-rbd-mirror-a",
		DataPathMap:  config.NewDatalessDaemonDataPathMap("rook-ceph", "/var/lib/rook"),
	}

	d := c.makeDeployment(&daemonConf)
	assert.Equal(t, "rook-ceph-rbd-mirror-a", d.Name)

	// Deployment should have Ceph labels
	cephtest.AssertLabelsContainCephRequirements(t, d.ObjectMeta.Labels,
		config.RbdMirrorType, "a", AppName, "ns")

	podTemplate := cephtest.NewPodTemplateSpecTester(t, &d.Spec.Template)
	podTemplate.RunFullSuite(config.RbdMirrorType, "a", AppName, "ns", "ceph/ceph:myceph",
		"200", "100", "600", "300", /* resources */
		"my-priority-class")
}
