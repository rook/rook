/*
Copyright 2021 The Rook Authors. All rights reserved.

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

package mirror

import (
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/test"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/operator/k8sutil"
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
		ResourceName: "rook-ceph-fs-mirror",
		DataPathMap:  config.NewDatalessDaemonDataPathMap("rook-ceph", "/var/lib/rook"),
	}
	cephCluster := &cephv1.CephCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namespace,
			Namespace: namespace,
		},
		Spec: cephv1.ClusterSpec{
			CephVersion: cephv1.CephVersionSpec{
				Image: "quay.io/ceph/ceph:v16",
			},
			DataDirHostPath: "/var/lib/rook/",
		},
	}

	fsMirror := &cephv1.CephFilesystemMirror{
		ObjectMeta: metav1.ObjectMeta{
			Name:      userID,
			Namespace: namespace,
		},
		Spec: cephv1.FilesystemMirroringSpec{
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
	clusterInfo := &cephclient.ClusterInfo{
		CephVersion: cephver.Quincy,
	}
	s := scheme.Scheme
	object := []runtime.Object{fsMirror}
	cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()
	r := &ReconcileFilesystemMirror{client: cl, scheme: s}
	r.cephClusterSpec = &cephCluster.Spec
	r.clusterInfo = clusterInfo

	d, err := r.makeDeployment(&daemonConf, fsMirror)
	assert.NoError(t, err)
	assert.Equal(t, "rook-ceph-fs-mirror", d.Name)
	assert.Equal(t, 5, len(d.Spec.Template.Spec.Volumes))
	assert.Equal(t, 1, len(d.Spec.Template.Spec.Volumes[0].Projected.Sources))
	assert.Equal(t, 5, len(d.Spec.Template.Spec.Containers[0].VolumeMounts))
	assert.Equal(t, k8sutil.DefaultServiceAccount, d.Spec.Template.Spec.ServiceAccountName)

	// Deployment should have Ceph labels
	test.AssertLabelsContainCephRequirements(t, d.ObjectMeta.Labels,
		config.FilesystemMirrorType, userID, AppName, "ns", "fs-mirror", "cephfilesystemmirrors.ceph.rook.io", "ceph-fs-mirror")

	podTemplate := test.NewPodTemplateSpecTester(t, &d.Spec.Template)
	podTemplate.RunFullSuite(config.FilesystemMirrorType, userID, AppName, "ns", "quay.io/ceph/ceph:v16",
		"200", "100", "600", "300", /* resources */
		"my-priority-class", "fs-mirror", "cephfilesystemmirrors.ceph.rook.io", "ceph-fs-mirror")
}
