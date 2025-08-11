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
	"strings"
	"testing"

	csiopv1 "github.com/ceph/ceph-csi-operator/api/v1"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	clienttest "github.com/rook/rook/pkg/daemon/ceph/client/test"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCreateUpdateClientProfile(t *testing.T) {
	c := clienttest.CreateTestClusterInfo(3)
	c.CSIDriverSpec = cephv1.CSIDriverSpec{
		CephFS: cephv1.CSICephFSSpec{
			KernelMountOptions: "ms_mode=crc",
		},
	}

	kernelMountKeyVal := strings.Split(c.CSIDriverSpec.CephFS.KernelMountOptions, "=")
	assert.Equal(t, len(kernelMountKeyVal), 2)
	assert.Equal(t, kernelMountKeyVal[0], "ms_mode")
	assert.Equal(t, kernelMountKeyVal[1], "crc")

	ns := "test"
	c.Namespace = ns
	c.SetName("testcluster")
	c.NamespacedName()
	c.SetName(c.Namespace)
	t.Setenv(k8sutil.PodNamespaceEnvVar, ns)

	cephBlockPoolRadosNamespacedName := types.NamespacedName{Namespace: ns, Name: "cephBlockPoolRadosNames"}
	cephSubVolGrpNamespacedName := types.NamespacedName{Namespace: ns, Name: "cephSubVolumeGroupNames"}
	csiOpClientProfile := &csiopv1.ClientProfile{}

	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	s.AddKnownTypes(cephv1.SchemeGroupVersion, csiOpClientProfile)
	object := []runtime.Object{
		csiOpClientProfile,
	}

	// Create a fake client to mock API calls.
	cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()
	err := CreateUpdateClientProfileRadosNamespace(context.TODO(), cl, c, cephBlockPoolRadosNamespacedName.Name, cephBlockPoolRadosNamespacedName.Name)
	assert.NoError(t, err)

	err = CreateUpdateClientProfileSubVolumeGroup(context.TODO(), cl, c, cephSubVolGrpNamespacedName.Name, cephSubVolGrpNamespacedName.Name)
	assert.NoError(t, err)

	err = cl.Get(context.TODO(), cephBlockPoolRadosNamespacedName, csiOpClientProfile)
	assert.NoError(t, err)
	assert.Equal(t, csiOpClientProfile.Spec.Rbd.RadosNamespace, cephBlockPoolRadosNamespacedName.Name)

	err = cl.Get(context.TODO(), cephSubVolGrpNamespacedName, csiOpClientProfile)
	assert.NoError(t, err)
	assert.Equal(t, csiOpClientProfile.Spec.CephFs.SubVolumeGroup, cephSubVolGrpNamespacedName.Name)
	assert.Equal(t, csiOpClientProfile.Spec.CephFs.KernelMountOptions["ms_mode"], kernelMountKeyVal[1])
}
