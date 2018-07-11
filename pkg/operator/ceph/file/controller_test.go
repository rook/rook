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

// Package mds to manage a rook file system.
package file

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

func TestFilesystemChanged(t *testing.T) {
	// no change
	old := cephv1beta1.FilesystemSpec{MetadataServer: cephv1beta1.MetadataServerSpec{ActiveCount: 1, ActiveStandby: true}}
	new := cephv1beta1.FilesystemSpec{MetadataServer: cephv1beta1.MetadataServerSpec{ActiveCount: 1, ActiveStandby: true}}
	changed := filesystemChanged(old, new)
	assert.False(t, changed)

	// changed properties
	new = cephv1beta1.FilesystemSpec{MetadataServer: cephv1beta1.MetadataServerSpec{ActiveCount: 2, ActiveStandby: true}}
	assert.True(t, filesystemChanged(old, new))

	new = cephv1beta1.FilesystemSpec{MetadataServer: cephv1beta1.MetadataServerSpec{ActiveCount: 1, ActiveStandby: false}}
	assert.True(t, filesystemChanged(old, new))
}

func TestGetFilesystemObject(t *testing.T) {
	// get a current version filesystem object, should return with no error and no migration needed
	filesystem, migrationNeeded, err := getFilesystemObject(&cephv1beta1.Filesystem{})
	assert.NotNil(t, filesystem)
	assert.False(t, migrationNeeded)
	assert.Nil(t, err)

	// get a legacy version filesystem object, should return with no error and yes migration needed
	filesystem, migrationNeeded, err = getFilesystemObject(&rookv1alpha1.Filesystem{})
	assert.NotNil(t, filesystem)
	assert.True(t, migrationNeeded)
	assert.Nil(t, err)

	// try to get an object that isn't a filesystem, should return with an error
	filesystem, migrationNeeded, err = getFilesystemObject(&map[string]string{})
	assert.Nil(t, filesystem)
	assert.False(t, migrationNeeded)
	assert.NotNil(t, err)
}

func TestMigrateFilesystemObject(t *testing.T) {
	// create a legacy filesystem that will get migrated
	legacyFilesystem := &rookv1alpha1.Filesystem{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "legacy-filesystem-861",
			Namespace: "rook-267",
		},
	}

	// create fake core and rook clientsets and a filesystem controller
	clientset := testop.New(3)
	context := &clusterd.Context{
		Clientset:     clientset,
		RookClientset: rookfake.NewSimpleClientset(legacyFilesystem),
	}
	controller := NewFilesystemController(context, "", false, metav1.OwnerReference{})

	// convert the legacy filesystem object in memory and assert that a migration is needed
	convertedFilesystem, migrationNeeded, err := getFilesystemObject(legacyFilesystem)
	assert.NotNil(t, convertedFilesystem)
	assert.True(t, migrationNeeded)
	assert.Nil(t, err)

	// perform the migration of the converted legacy filesystem
	err = controller.migrateFilesystemObject(convertedFilesystem, legacyFilesystem)
	assert.Nil(t, err)

	// assert that a current filesystem object was created via the migration
	migratedFilesystem, err := context.RookClientset.CephV1beta1().Filesystems(legacyFilesystem.Namespace).Get(
		legacyFilesystem.Name, metav1.GetOptions{})
	assert.NotNil(t, migratedFilesystem)
	assert.Nil(t, err)

	// assert that the legacy filesystem object was deleted
	_, err = context.RookClientset.RookV1alpha1().Filesystems(legacyFilesystem.Namespace).Get(legacyFilesystem.Name, metav1.GetOptions{})
	assert.NotNil(t, err)
	assert.True(t, errors.IsNotFound(err))
}

func TestConvertLegacyFilesystem(t *testing.T) {
	legacyFilesystem := rookv1alpha1.Filesystem{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "legacy-filesystem-3215",
			Namespace: "rook-6468",
		},
		Spec: rookv1alpha1.FilesystemSpec{
			MetadataPool: rookv1alpha1.PoolSpec{
				FailureDomain: "fd1",
				Replicated:    rookv1alpha1.ReplicatedSpec{Size: 5}},
			DataPools: []rookv1alpha1.PoolSpec{
				{
					CrushRoot: "root32",
					ErasureCoded: rookv1alpha1.ErasureCodedSpec{
						CodingChunks: 5,
						DataChunks:   10,
						Algorithm:    "ec-algorithm-048",
					},
				},
			},
			MetadataServer: rookv1alpha1.MetadataServerSpec{
				ActiveCount:   2,
				ActiveStandby: true,
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

	expectedFilesystem := cephv1beta1.Filesystem{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "legacy-filesystem-3215",
			Namespace: "rook-6468",
		},
		Spec: cephv1beta1.FilesystemSpec{
			MetadataPool: cephv1beta1.PoolSpec{
				FailureDomain: "fd1",
				Replicated:    cephv1beta1.ReplicatedSpec{Size: 5}},
			DataPools: []cephv1beta1.PoolSpec{
				{
					CrushRoot: "root32",
					ErasureCoded: cephv1beta1.ErasureCodedSpec{
						CodingChunks: 5,
						DataChunks:   10,
						Algorithm:    "ec-algorithm-048",
					},
				},
			},
			MetadataServer: cephv1beta1.MetadataServerSpec{
				ActiveCount:   2,
				ActiveStandby: true,
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

	assert.Equal(t, expectedFilesystem, *convertRookLegacyFilesystem(&legacyFilesystem))
}
