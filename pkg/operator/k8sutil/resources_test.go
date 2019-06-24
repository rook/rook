/*
Copyright 2017 The Rook Authors. All rights reserved.

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

// Package k8sutil for Kubernetes helpers.
package k8sutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMergeResourceRequirements(t *testing.T) {
	first := v1.ResourceRequirements{}
	second := v1.ResourceRequirements{}
	result := MergeResourceRequirements(first, second)
	// Both are 0 because first and second don't have a value set, so there is nothing to merge
	assert.Equal(t, 0, len(result.Limits))
	assert.Equal(t, 0, len(result.Requests))

	first = v1.ResourceRequirements{}
	second = v1.ResourceRequirements{
		Limits: v1.ResourceList{
			v1.ResourceCPU: *resource.NewQuantity(100.0, resource.BinarySI),
		},
		Requests: v1.ResourceList{
			v1.ResourceMemory: *resource.NewQuantity(1337.0, resource.BinarySI),
		},
	}
	result = MergeResourceRequirements(first, second)
	assert.Equal(t, 1, len(result.Limits))
	assert.Equal(t, 1, len(result.Requests))
	assert.Equal(t, "100", result.Limits.Cpu().String())
	assert.Equal(t, "1337", result.Requests.Memory().String())

	first = v1.ResourceRequirements{
		Limits: v1.ResourceList{
			v1.ResourceCPU: *resource.NewQuantity(42.0, resource.BinarySI),
		},
	}
	second = v1.ResourceRequirements{
		Limits: v1.ResourceList{
			v1.ResourceCPU: *resource.NewQuantity(100.0, resource.BinarySI),
		},
		Requests: v1.ResourceList{
			v1.ResourceCPU:    *resource.NewQuantity(100.0, resource.BinarySI),
			v1.ResourceMemory: *resource.NewQuantity(1337.0, resource.BinarySI),
		},
	}
	result = MergeResourceRequirements(first, second)
	assert.Equal(t, 1, len(result.Limits))
	assert.Equal(t, 2, len(result.Requests))
	assert.Equal(t, "42", result.Limits.Cpu().String())
	assert.Equal(t, "1337", result.Requests.Memory().String())
}

func TestOwnerRefCheck(t *testing.T) {
	namespace := "ns"
	resource := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myresource",
			Namespace: namespace,
		},
		Data: map[string]string{},
	}
	ownerRef := &metav1.OwnerReference{Name: "myref"}

	// test that the ownerref is set
	SetOwnerRef(&resource.ObjectMeta, ownerRef)
	require.Equal(t, 1, len(resource.OwnerReferences))
	assert.Equal(t, ownerRef.Name, resource.OwnerReferences[0].Name)
}
