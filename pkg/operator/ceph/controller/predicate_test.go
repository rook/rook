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

func TestIsUpgrade(t *testing.T) {
	oldLabel := make(map[string]string)
	newLabel := map[string]string{
		"foo": "bar",
	}

	// no value do nothing
	b := isUpgrade(oldLabel, newLabel)
	assert.False(t, b)

	// different value do something
	newLabel["ceph_version"] = "15.2.0-octopus"
	b = isUpgrade(oldLabel, newLabel)
	assert.True(t, b, fmt.Sprintf("%v,%v", oldLabel, newLabel))

	// same value do nothing
	oldLabel["ceph_version"] = "15.2.0-octopus"
	newLabel["ceph_version"] = "15.2.0-octopus"
	b = isUpgrade(oldLabel, newLabel)
	assert.False(t, b, fmt.Sprintf("%v,%v", oldLabel, newLabel))

	// different value do something
	newLabel["ceph_version"] = "15.2.1-octopus"
	b = isUpgrade(oldLabel, newLabel)
	assert.True(t, b, fmt.Sprintf("%v,%v", oldLabel, newLabel))
}
