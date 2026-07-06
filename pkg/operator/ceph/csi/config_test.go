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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	c.SetName(c.Namespace)
	t.Setenv(k8sutil.PodNamespaceEnvVar, ns)
	c.NamespacedName()
	c.SetName(c.Namespace)
	t.Setenv(k8sutil.PodNamespaceEnvVar, ns)

	cephBlockPoolRadosNamespacedName := types.NamespacedName{Namespace: ns, Name: "cephBlockPoolRadosNames"}
	cephSubVolGrpNamespacedName := types.NamespacedName{Namespace: ns, Name: "cephSubVolumeGroupNames"}
	cephSubVolGrpRadosNamespaceNamespacedName := types.NamespacedName{Namespace: ns, Name: "radosNamespaceName"}
	csiOpClientProfile := &csiopv1.ClientProfile{}
	secretsList := &v1.SecretList{}

	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	s.AddKnownTypes(cephv1.SchemeGroupVersion, csiOpClientProfile, secretsList)
	object := []runtime.Object{
		csiOpClientProfile,
	}

	// Create a fake client to mock API calls.
	cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()
	err := CreateUpdateClientProfileRadosNamespace(context.TODO(), cl, c, cephBlockPoolRadosNamespacedName.Name, cephBlockPoolRadosNamespacedName.Name)
	assert.NoError(t, err)

	err = CreateUpdateClientProfileSubVolumeGroup(context.TODO(), cl, c, cephSubVolGrpNamespacedName.Name, cephSubVolGrpNamespacedName.Name, cephSubVolGrpRadosNamespaceNamespacedName.Name)
	assert.NoError(t, err)

	err = cl.Get(context.TODO(), cephBlockPoolRadosNamespacedName, csiOpClientProfile)
	assert.NoError(t, err)
	assert.Equal(t, csiOpClientProfile.Spec.Rbd.RadosNamespace, cephBlockPoolRadosNamespacedName.Name)

	err = cl.Get(context.TODO(), cephSubVolGrpNamespacedName, csiOpClientProfile)
	assert.NoError(t, err)
	assert.Equal(t, csiOpClientProfile.Spec.CephFs.SubVolumeGroup, cephSubVolGrpNamespacedName.Name)
	assert.Equal(t, csiOpClientProfile.Spec.CephFs.KernelMountOptions["ms_mode"], kernelMountKeyVal[1])
	assert.Equal(t, *csiOpClientProfile.Spec.CephFs.RadosNamespace, cephSubVolGrpRadosNamespaceNamespacedName.Name)
}

func TestGetSecretNameByAnnotation(t *testing.T) {
	ns := "test-ns"
	annotationKey := "csi.rook.io/RBDProvisionerSecret"
	defaultName := "rook-csi-rbd-provisioner"

	t.Run("return matching secret name when annotation value is true", func(t *testing.T) {
		secret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "custom-rbd-secret",
				Namespace: ns,
				Annotations: map[string]string{
					annotationKey: "true",
				},
			},
		}
		cl := fake.NewClientBuilder().WithObjects(secret).Build()

		name, err := getSecretNameByAnnotation(cl, context.TODO(), ns, annotationKey, defaultName)
		assert.NoError(t, err)
		assert.Equal(t, "custom-rbd-secret", name)
	})

	t.Run("return default name when no secret has the annotation", func(t *testing.T) {
		secret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "unrelated-secret",
				Namespace: ns,
			},
		}
		cl := fake.NewClientBuilder().WithObjects(secret).Build()

		name, err := getSecretNameByAnnotation(cl, context.TODO(), ns, annotationKey, defaultName)
		assert.NoError(t, err)
		assert.Equal(t, defaultName, name)
	})

	t.Run("return default name when annotation value is not true", func(t *testing.T) {
		secret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "some-secret",
				Namespace: ns,
				Annotations: map[string]string{
					annotationKey: "false",
				},
			},
		}
		cl := fake.NewClientBuilder().WithObjects(secret).Build()

		name, err := getSecretNameByAnnotation(cl, context.TODO(), ns, annotationKey, defaultName)
		assert.NoError(t, err)
		assert.Equal(t, defaultName, name)
	})

	t.Run("return default name when no secrets exist", func(t *testing.T) {
		cl := fake.NewClientBuilder().Build()

		name, err := getSecretNameByAnnotation(cl, context.TODO(), ns, annotationKey, defaultName)
		assert.NoError(t, err)
		assert.Equal(t, defaultName, name)
	})

	t.Run("return first matching secret when multiple match", func(t *testing.T) {
		secret1 := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "first-secret",
				Namespace: ns,
				Annotations: map[string]string{
					annotationKey: "true",
				},
			},
		}
		secret2 := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "second-secret",
				Namespace: ns,
				Annotations: map[string]string{
					annotationKey: "true",
				},
			},
		}
		cl := fake.NewClientBuilder().WithObjects(secret1, secret2).Build()

		name, err := getSecretNameByAnnotation(cl, context.TODO(), ns, annotationKey, defaultName)
		assert.NoError(t, err)
		assert.Contains(t, []string{"first-secret", "second-secret"}, name)
	})
}

func TestParseMountOptions(t *testing.T) {
	t.Run("single mount option", func(t *testing.T) {
		options := "ms_mode=crc"
		result := parseMountOptions(options)
		assert.Equal(t, 1, len(result))
		assert.Equal(t, "crc", result["ms_mode"])
	})

	t.Run("multiple mount options", func(t *testing.T) {
		options := "ms_mode=prefer-secure,recover_session=clean"
		result := parseMountOptions(options)
		assert.Equal(t, 2, len(result))
		assert.Equal(t, "prefer-secure", result["ms_mode"])
		assert.Equal(t, "clean", result["recover_session"])
	})

	t.Run("multiple mount options with spaces", func(t *testing.T) {
		options := "ms_mode=prefer-secure, recover_session=clean"
		result := parseMountOptions(options)
		assert.Equal(t, 2, len(result))
		assert.Equal(t, "prefer-secure", result["ms_mode"])
		assert.Equal(t, "clean", result["recover_session"])
	})

	t.Run("empty string", func(t *testing.T) {
		options := ""
		result := parseMountOptions(options)
		assert.Equal(t, 0, len(result))
	})
}
