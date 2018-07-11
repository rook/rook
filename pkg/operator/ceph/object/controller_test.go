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

// Package rgw to manage a rook object store.
package object

import (
	"testing"

	cephv1beta1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1beta1"
	rookv1alpha1 "github.com/rook/rook/pkg/apis/rook.io/v1alpha1"
	rookv1alpha2 "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	rookfake "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/pkg/clusterd"
	testop "github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestObjectStoreChanged(t *testing.T) {
	old := cephv1beta1.ObjectStoreSpec{Gateway: cephv1beta1.GatewaySpec{Port: 80, SecurePort: 443, Instances: 1, AllNodes: false, SSLCertificateRef: ""}}
	new := cephv1beta1.ObjectStoreSpec{Gateway: cephv1beta1.GatewaySpec{Port: 80, SecurePort: 443, Instances: 1, AllNodes: false, SSLCertificateRef: ""}}
	// nothing changed
	assert.False(t, storeChanged(old, new))

	// there was a change
	new = cephv1beta1.ObjectStoreSpec{Gateway: cephv1beta1.GatewaySpec{Port: 81, SecurePort: 443, Instances: 1, AllNodes: false, SSLCertificateRef: ""}}
	assert.True(t, storeChanged(old, new))

	new = cephv1beta1.ObjectStoreSpec{Gateway: cephv1beta1.GatewaySpec{Port: 80, SecurePort: 444, Instances: 1, AllNodes: false, SSLCertificateRef: ""}}
	assert.True(t, storeChanged(old, new))

	new = cephv1beta1.ObjectStoreSpec{Gateway: cephv1beta1.GatewaySpec{Port: 80, SecurePort: 443, Instances: 2, AllNodes: false, SSLCertificateRef: ""}}
	assert.True(t, storeChanged(old, new))

	new = cephv1beta1.ObjectStoreSpec{Gateway: cephv1beta1.GatewaySpec{Port: 80, SecurePort: 443, Instances: 1, AllNodes: true, SSLCertificateRef: ""}}
	assert.True(t, storeChanged(old, new))

	new = cephv1beta1.ObjectStoreSpec{Gateway: cephv1beta1.GatewaySpec{Port: 80, SecurePort: 443, Instances: 1, AllNodes: false, SSLCertificateRef: "mysecret"}}
	assert.True(t, storeChanged(old, new))
}

func TestGetObjectStoreObject(t *testing.T) {
	// get a current version objectstore object, should return with no error and no migration needed
	objectstore, migrationNeeded, err := getObjectStoreObject(&cephv1beta1.ObjectStore{})
	assert.NotNil(t, objectstore)
	assert.False(t, migrationNeeded)
	assert.Nil(t, err)

	// get a legacy version objectstore object, should return with no error and yes migration needed
	objectstore, migrationNeeded, err = getObjectStoreObject(&rookv1alpha1.ObjectStore{})
	assert.NotNil(t, objectstore)
	assert.True(t, migrationNeeded)
	assert.Nil(t, err)

	// try to get an object that isn't a objectstore, should return with an error
	objectstore, migrationNeeded, err = getObjectStoreObject(&map[string]string{})
	assert.Nil(t, objectstore)
	assert.False(t, migrationNeeded)
	assert.NotNil(t, err)
}

func TestMigrateObjectStoreObject(t *testing.T) {
	// create a legacy objectstore that will get migrated
	legacyObjectStore := &rookv1alpha1.ObjectStore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "legacy-objectstore-861",
			Namespace: "rook-267",
		},
	}

	// create fake core and rook clientsets and a objectstore controller
	clientset := testop.New(3)
	context := &clusterd.Context{
		Clientset:     clientset,
		RookClientset: rookfake.NewSimpleClientset(legacyObjectStore),
	}
	controller := NewObjectStoreController(context, "", false, metav1.OwnerReference{})

	// convert the legacy objectstore object in memory and assert that a migration is needed
	convertedObjectStore, migrationNeeded, err := getObjectStoreObject(legacyObjectStore)
	assert.NotNil(t, convertedObjectStore)
	assert.True(t, migrationNeeded)
	assert.Nil(t, err)

	// perform the migration of the converted legacy objectstore
	err = controller.migrateObjectStoreObject(convertedObjectStore, legacyObjectStore)
	assert.Nil(t, err)

	// assert that a current objectstore object was created via the migration
	migratedObjectStore, err := context.RookClientset.CephV1beta1().ObjectStores(legacyObjectStore.Namespace).Get(
		legacyObjectStore.Name, metav1.GetOptions{})
	assert.NotNil(t, migratedObjectStore)
	assert.Nil(t, err)

	// assert that the legacy objectstore object was deleted
	_, err = context.RookClientset.RookV1alpha1().ObjectStores(legacyObjectStore.Namespace).Get(legacyObjectStore.Name, metav1.GetOptions{})
	assert.NotNil(t, err)
	assert.True(t, errors.IsNotFound(err))
}

func TestConvertLegacyObjectStore(t *testing.T) {
	legacyObjectStore := rookv1alpha1.ObjectStore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "legacy-objectstore-3545",
			Namespace: "rook-96874",
		},
		Spec: rookv1alpha1.ObjectStoreSpec{
			MetadataPool: rookv1alpha1.PoolSpec{
				FailureDomain: "fd1",
				Replicated:    rookv1alpha1.ReplicatedSpec{Size: 5},
			},
			DataPool: rookv1alpha1.PoolSpec{
				CrushRoot: "root329",
				ErasureCoded: rookv1alpha1.ErasureCodedSpec{
					CodingChunks: 5,
					DataChunks:   10,
					Algorithm:    "ec-algorithm-367",
				},
			},
			Gateway: rookv1alpha1.GatewaySpec{
				Port:              3093,
				SecurePort:        2022,
				Instances:         2,
				AllNodes:          true,
				SSLCertificateRef: "my-ssl-cert",
				Placement: rookv1alpha1.Placement{
					PodAntiAffinity: &v1.PodAntiAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
							{LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"label": "value"}}},
						},
					},
				},
				Resources: v1.ResourceRequirements{Limits: v1.ResourceList{v1.ResourceMemory: resource.MustParse("100Mi")}},
			},
		},
	}

	expectedObjectStore := cephv1beta1.ObjectStore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "legacy-objectstore-3545",
			Namespace: "rook-96874",
		},
		Spec: cephv1beta1.ObjectStoreSpec{
			MetadataPool: cephv1beta1.PoolSpec{
				FailureDomain: "fd1",
				Replicated:    cephv1beta1.ReplicatedSpec{Size: 5},
			},
			DataPool: cephv1beta1.PoolSpec{
				CrushRoot: "root329",
				ErasureCoded: cephv1beta1.ErasureCodedSpec{
					CodingChunks: 5,
					DataChunks:   10,
					Algorithm:    "ec-algorithm-367",
				},
			},
			Gateway: cephv1beta1.GatewaySpec{
				Port:              3093,
				SecurePort:        2022,
				Instances:         2,
				AllNodes:          true,
				SSLCertificateRef: "my-ssl-cert",
				Placement: rookv1alpha2.Placement{
					PodAntiAffinity: &v1.PodAntiAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
							{LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"label": "value"}}},
						},
					},
				},
				Resources: v1.ResourceRequirements{Limits: v1.ResourceList{v1.ResourceMemory: resource.MustParse("100Mi")}},
			},
		},
	}

	assert.Equal(t, expectedObjectStore, *convertRookLegacyObjectStore(&legacyObjectStore))
}
