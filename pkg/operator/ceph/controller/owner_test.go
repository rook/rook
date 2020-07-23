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
	"fmt"
	"reflect"
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/client/clientset/versioned/scheme"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMatch(t *testing.T) {
	isController := true

	// Setup controller object
	fakeObject := &cephv1.CephObjectStore{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "rook-ceph",
			UID:       "ce6807a0-7270-4874-9e9f-ae493d48b814",
			Name:      "my-store",
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       reflect.TypeOf(cephv1.CephObjectStore{}).Name(),
			APIVersion: fmt.Sprintf("%s/%s", cephv1.CustomResourceGroup, cephv1.Version),
		},
	}

	// Setup child object
	fakeChildObject := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rgw-secret",
			Namespace: "rook-ceph",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "ceph.rook.io/v1",
					Kind:       "wrong kind",
					Name:       "my-store",
					UID:        "wrong-uid",
					Controller: &isController,
				},
			},
		},
	}

	// Setup scheme
	scheme := scheme.Scheme
	scheme.AddKnownTypes(cephv1.SchemeGroupVersion, fakeObject)

	// Wrong Kind
	ownerMatcher, err := NewOwnerReferenceMatcher(fakeObject, scheme)
	assert.NoError(t, err)
	match, _, err := ownerMatcher.Match(fakeChildObject)
	assert.NoError(t, err)
	assert.False(t, match)

	// Good Kind but wrong UID
	fakeChildObject.OwnerReferences[0].Kind = "CephObjectStore"
	match, _, err = ownerMatcher.Match(fakeChildObject)
	assert.NoError(t, err)
	assert.False(t, match)

	// Good Kind AND good UID
	fakeChildObject.OwnerReferences[0].UID = "ce6807a0-7270-4874-9e9f-ae493d48b814"
	match, _, err = ownerMatcher.Match(fakeChildObject)
	assert.NoError(t, err)
	assert.True(t, match)
}
