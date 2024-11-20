/*
Copyright 2024 The Rook Authors. All rights reserved.

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

package csi

import (
	"context"
	"testing"

	csiopv1a1 "github.com/ceph/ceph-csi-operator/api/v1alpha1"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	clienttest "github.com/rook/rook/pkg/daemon/ceph/client/test"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCreateUpdateCephConnection(t *testing.T) {
	c := clienttest.CreateTestClusterInfo(3)
	ns := "test"
	c.Namespace = ns
	c.SetName("testcluster")
	c.NamespacedName()
	t.Setenv(k8sutil.PodNamespaceEnvVar, ns)

	cluster := &cephv1.CephCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testCluster",
			Namespace: ns,
		},
		Spec: cephv1.ClusterSpec{
			CSI: cephv1.CSIDriverSpec{
				ReadAffinity: cephv1.ReadAffinitySpec{
					Enabled:             true,
					CrushLocationLabels: []string{"kubernetes.io/hostname"},
				},
			},
		},
	}
	csiCephConnection := &csiopv1a1.CephConnection{}

	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	s.AddKnownTypes(cephv1.SchemeGroupVersion, csiCephConnection, &cephv1.CephCluster{}, &cephv1.CephRBDMirrorList{})
	object := []runtime.Object{
		cluster,
	}

	// Create a fake client to mock API calls.
	cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()
	err := CreateUpdateCephConnection(cl, c, cluster.Spec)
	assert.NoError(t, err)

	// When no RBDMirror is created
	err = cl.Get(context.TODO(), types.NamespacedName{Name: c.NamespacedName().Name, Namespace: c.NamespacedName().Namespace}, csiCephConnection)
	assert.NoError(t, err)
	assert.Equal(t, csiCephConnection.Spec.RbdMirrorDaemonCount, 0)

	rbdMirror := &cephv1.CephRBDMirror{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-mirror",
			Namespace: ns,
		},
		Spec: cephv1.RBDMirroringSpec{
			Count: 1,
		},
	}

	object = []runtime.Object{
		rbdMirror,
		cluster,
	}

	err = cl.Create(context.TODO(), rbdMirror)
	assert.NoError(t, err)
	// Create a fake client to mock API calls.
	cl = fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()
	err = CreateUpdateCephConnection(cl, c, cluster.Spec)
	assert.NoError(t, err)

	// When RBDMirror is created
	err = cl.Get(context.TODO(), types.NamespacedName{Name: c.NamespacedName().Name, Namespace: c.NamespacedName().Namespace}, csiCephConnection)
	assert.NoError(t, err)
	assert.Equal(t, 1, csiCephConnection.Spec.RbdMirrorDaemonCount)
}
