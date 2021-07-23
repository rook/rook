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
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/config"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	optest "github.com/rook/rook/pkg/operator/test"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestDeploymentSpec(t *testing.T) {
	nfs := &cephv1.CephNFS{
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

	clientset := optest.New(t, 1)
	c := &clusterd.Context{
		Executor:      &exectest.MockExecutor{},
		RookClientset: rookclient.NewSimpleClientset(),
		Clientset:     clientset,
	}

	s := scheme.Scheme
	object := []runtime.Object{&cephv1.CephNFS{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: cephv1.NFSGaneshaSpec{
			RADOS: cephv1.GaneshaRADOSSpec{
				Pool:      "foo",
				Namespace: namespace,
			},
			Server: cephv1.GaneshaServerSpec{
				Active: 1,
			},
		},
		TypeMeta: controllerTypeMeta,
	},
	}
	cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()

	r := &ReconcileCephNFS{
		client:  cl,
		scheme:  scheme.Scheme,
		context: c,
		clusterInfo: &cephclient.ClusterInfo{
			FSID:        "myfsid",
			CephVersion: cephver.Nautilus,
		},
		cephClusterSpec: &cephv1.ClusterSpec{
			CephVersion: cephv1.CephVersionSpec{
				Image: "quay.io/ceph/ceph:v15",
			},
		},
	}

	id := "i"
	configName := "rook-ceph-nfs-my-nfs-i"
	cfg := daemonConfig{
		ID:              id,
		ConfigConfigMap: configName,
		DataPathMap: &config.DataPathMap{
			HostDataDir:        "",                          // nfs daemon does not store data on host, ...
			ContainerDataDir:   cephclient.DefaultConfigDir, // does share data in containers using emptyDir, ...
			HostLogAndCrashDir: "",                          // and does not log to /var/log/ceph dir nor creates crash dumps
		},
	}

	d, err := r.makeDeployment(nfs, cfg)
	assert.NoError(t, err)

	// Deployment should have Ceph labels
	optest.AssertLabelsContainRookRequirements(t, d.ObjectMeta.Labels, AppName)

	podTemplate := optest.NewPodTemplateSpecTester(t, &d.Spec.Template)
	podTemplate.RunFullSuite(
		AppName,
		optest.ResourceLimitExpectations{
			CPUResourceLimit:      "500",
			MemoryResourceLimit:   "1Ki",
			CPUResourceRequest:    "200",
			MemoryResourceRequest: "512",
		},
	)
	assert.Equal(t, "my-priority-class", d.Spec.Template.Spec.PriorityClassName)
}
