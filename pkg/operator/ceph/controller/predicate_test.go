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
	"testing"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	name             = "my-pool"
	namespace        = "rook-ceph"
	oldReplicas uint = 3
	newReplicas uint = 2
)

func TestObjectChanged(t *testing.T) {

	oldObject := &cephv1.CephBlockPool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: cephv1.PoolSpec{
			Replicated: cephv1.ReplicatedSpec{
				Size: oldReplicas,
			},
		},
		Status: &cephv1.Status{
			Phase: "",
		},
	}

	newObject := &cephv1.CephBlockPool{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: cephv1.PoolSpec{
			Replicated: cephv1.ReplicatedSpec{
				Size: oldReplicas,
			},
		},
		Status: &cephv1.Status{
			Phase: "",
		},
	}

	// Identical
	changed, err := objectChanged(oldObject, newObject)
	assert.NoError(t, err)
	assert.False(t, changed)

	// Replica size changed
	oldObject.Spec.Replicated.Size = newReplicas
	changed, err = objectChanged(oldObject, newObject)
	assert.NoError(t, err)
	assert.True(t, changed)
}
