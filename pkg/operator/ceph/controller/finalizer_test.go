/*
Copyright 2020 The Rook Authors. All rights reserved.

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

package controller

import (
	"context"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestAddFinalizerIfNotPresent(t *testing.T) {
	fakeObject := &cephv1.CephBlockPool{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test",
			Namespace:  "rook-ceph",
			Finalizers: []string{},
		},
	}

	// Objects to track in the fake client.
	object := []runtime.Object{
		fakeObject,
	}
	s := scheme.Scheme
	s.AddKnownTypes(cephv1.SchemeGroupVersion, fakeObject)
	cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()

	assert.Empty(t, fakeObject.Finalizers)
	generationUpdated, err := AddFinalizerIfNotPresent(context.TODO(), cl, fakeObject)
	assert.NoError(t, err)
	assert.NotEmpty(t, fakeObject.Finalizers)
	assert.False(t, generationUpdated)
}

func TestRemoveFinalizer(t *testing.T) {
	fakeObject := &cephv1.CephBlockPool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "rook-ceph",
			Finalizers: []string{
				"cephblockpool.ceph.rook.io",
			},
		},
		TypeMeta: metav1.TypeMeta{
			Kind: "cephblockpool",
		},
	}

	object := []runtime.Object{
		fakeObject,
	}
	s := scheme.Scheme
	s.AddKnownTypes(cephv1.SchemeGroupVersion, fakeObject)
	cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()

	assert.NotEmpty(t, fakeObject.Finalizers)
	err := RemoveFinalizer(context.TODO(), cl, fakeObject)
	assert.NoError(t, err)
	assert.Empty(t, fakeObject.Finalizers)
}

func TestRemoveFinalizerWithName(t *testing.T) {
	fakeObject := &cephv1.CephBlockPool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "rook-ceph",
			Finalizers: []string{
				"cephblockpool.ceph.rook.io",
			},
		},
		TypeMeta: metav1.TypeMeta{
			Kind: "cephblockpool",
		},
	}

	object := []runtime.Object{
		fakeObject,
	}
	s := scheme.Scheme
	s.AddKnownTypes(cephv1.SchemeGroupVersion, fakeObject)
	cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(object...).Build()

	assert.NotEmpty(t, fakeObject.Finalizers)
	err := RemoveFinalizerWithName(context.TODO(), cl, fakeObject, "cephblockpool.ceph.rook.io")
	assert.NoError(t, err)
	assert.Empty(t, fakeObject.Finalizers)
}
