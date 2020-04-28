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
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/config"
	cephtest "github.com/rook/rook/pkg/operator/ceph/test"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestPodSpec(t *testing.T) {
	namespace := "ns"
	daemonConf := daemonConfig{
		DaemonID:     "a",
		ResourceName: "rook-ceph-rbd-mirror-a",
		DataPathMap:  config.NewDatalessDaemonDataPathMap("rook-ceph", "/var/lib/rook"),
		namespace:    namespace,
	}
	cephCluster := &cephv1.CephCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namespace,
			Namespace: namespace,
		},
		Spec: cephv1.ClusterSpec{
			CephVersion: cephv1.CephVersionSpec{
				Image: "ceph/ceph:v14",
			},
		},
	}

	rbdMirror := &cephv1.CephRBDMirror{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "a",
			Namespace: namespace,
		},
		Spec: cephv1.RBDMirroringSpec{
			Count: 1,
			Resources: v1.ResourceRequirements{
				Limits: v1.ResourceList{
					v1.ResourceCPU:    *resource.NewQuantity(200.0, resource.BinarySI),
					v1.ResourceMemory: *resource.NewQuantity(600.0, resource.BinarySI),
				},
				Requests: v1.ResourceList{
					v1.ResourceCPU:    *resource.NewQuantity(100.0, resource.BinarySI),
					v1.ResourceMemory: *resource.NewQuantity(300.0, resource.BinarySI),
				},
			},
			PriorityClassName: "my-priority-class",
		},
		TypeMeta: controllerTypeMeta,
	}
	clusterInfo := &cephconfig.ClusterInfo{
		CephVersion: cephver.Nautilus,
	}
	s := scheme.Scheme
	object := []runtime.Object{rbdMirror}
	cl := fake.NewFakeClientWithScheme(s, object...)
	r := &ReconcileCephRBDMirror{client: cl, scheme: s}
	r.cephClusterSpec = &cephCluster.Spec
	r.clusterInfo = clusterInfo

	d := r.makeDeployment(&daemonConf, rbdMirror)
	assert.Equal(t, "rook-ceph-rbd-mirror-a", d.Name)

	// Deployment should have Ceph labels
	cephtest.AssertLabelsContainCephRequirements(t, d.ObjectMeta.Labels,
		config.RbdMirrorType, "a", AppName, "ns")

	podTemplate := cephtest.NewPodTemplateSpecTester(t, &d.Spec.Template)
	podTemplate.RunFullSuite(config.RbdMirrorType, "a", AppName, "ns", "ceph/ceph:myceph",
		"200", "100", "600", "300", /* resources */
		"my-priority-class")
}
