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
	"reflect"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestObjectToCRMapper(t *testing.T) {
	fs := &cephv1.CephFilesystem{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind: reflect.TypeOf(cephv1.CephFilesystem{}).Name(),
		},
	}

	// Objects to track in the fake client.
	objects := []runtime.Object{
		&cephv1.CephFilesystemList{},
		fs,
	}

	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephFilesystemList{})
	s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephFilesystem{})
	s.AddKnownTypes(cephv1.SchemeGroupVersion, &cephv1.CephCluster{})

	// Create a fake client to mock API calls.
	cl := fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objects...).Build()

	// Fake reconcile request
	fakeRequest := []ctrl.Request{
		{NamespacedName: client.ObjectKey{Name: "my-pool", Namespace: "rook-ceph"}},
	}

	handlerFunc, err := ObjectToCRMapper(context.TODO(), cl, objects[0], s)
	assert.NoError(t, err)
	assert.ElementsMatch(t, fakeRequest, handlerFunc(context.TODO(), fs))
}
