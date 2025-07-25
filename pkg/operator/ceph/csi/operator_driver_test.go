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
	"fmt"
	"testing"

	csiopv1 "github.com/ceph/ceph-csi-operator/api/v1"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	rookclient "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/rook/rook/pkg/clusterd"
	clienttest "github.com/rook/rook/pkg/daemon/ceph/client/test"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	testop "github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestReconcileCSI_createOrUpdateDriverResources(t *testing.T) {
	ns := "test"
	r := &ReconcileCSI{
		context: &clusterd.Context{
			Clientset:     testop.New(t, 1),
			RookClientset: rookclient.NewSimpleClientset(),
		},
		opManagerContext: context.TODO(),
		opConfig: opcontroller.OperatorConfig{
			OperatorNamespace: "test",
		},
	}
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
				CephFS: cephv1.CSICephFSSpec{
					KernelMountOptions: "ms_mode=crc",
				},
			},
			Network: cephv1.NetworkSpec{
				Connections: &cephv1.ConnectionsSpec{
					Encryption: &cephv1.EncryptionSpec{
						Enabled: true,
					},
				},
			},
		},
	}
	driver := &csiopv1.Driver{}

	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	s.AddKnownTypes(cephv1.SchemeGroupVersion, driver, &cephv1.CephCluster{}, &v1.ConfigMap{})
	object := []runtime.Object{
		cluster,
	}
	cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()
	r.client = cl

	EnableRBD = true
	EnableCephFS = true
	EnableNFS = true

	c := clienttest.CreateTestClusterInfo(3)
	c.Namespace = ns
	c.SetName("testcluster")
	c.NamespacedName()

	err := r.createOrUpdateDriverResources(*cluster, c)
	assert.NoError(t, err)

	err = cl.Get(context.TODO(), types.NamespacedName{Name: fmt.Sprintf("%s.rbd.csi.ceph.com", c.Namespace), Namespace: ns}, driver)
	assert.NoError(t, err)

	err = cl.Get(context.TODO(), types.NamespacedName{Name: fmt.Sprintf("%s.cephfs.csi.ceph.com", c.Namespace), Namespace: ns}, driver)
	assert.NoError(t, err)

	err = cl.Get(context.TODO(), types.NamespacedName{Name: fmt.Sprintf("%s.nfs.csi.ceph.com", c.Namespace), Namespace: ns}, driver)
	assert.NoError(t, err)
}
