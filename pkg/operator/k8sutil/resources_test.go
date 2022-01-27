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
	v1 "k8s.io/api/core/v1"
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
			v1.ResourceCPU:     *resource.NewQuantity(100.0, resource.BinarySI),
			v1.ResourceStorage: *resource.NewQuantity(50.0, resource.BinarySI),
		},
		Requests: v1.ResourceList{
			v1.ResourceName("foo"): *resource.NewQuantity(23.0, resource.BinarySI),
		},
	}
	result = MergeResourceRequirements(first, second)
	assert.Equal(t, 2, len(result.Limits))
	assert.Equal(t, 1, len(result.Requests))
	assert.Equal(t, "100", result.Limits.Cpu().String())
	assert.Equal(t, "50", result.Limits.Storage().String())

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

func TestYamlToContainerResource(t *testing.T) {
	var validData string = `
- name: rbdplugin
  resource:
    requests:
      memory: 512Mi
      cpu: 250m
    limits:
      memory: 512Mi
      cpu: 250m
- name: rbdplugin
  resource:
    requests:
      memory: 512Mi
      cpu: 250m
    limits:
      memory: 512Mi
      cpu: 250m`
	res, err := YamlToContainerResource(validData)
	assert.Len(t, res, 2)
	assert.NoError(t, err)

	var invalidData string = `
	invalid:
	  data: 512Mi
	invalid:
	  memry: 512Mi
	  cpu: 250m`
	res, err = YamlToContainerResource(invalidData)
	assert.Len(t, res, 0)
	assert.Error(t, err)
}

func TestValidateOwner(t *testing.T) {
	// global-scoped owner
	ownerRef := &metav1.OwnerReference{}
	ownerInfo := NewOwnerInfoWithOwnerRef(ownerRef, "")
	object := &v1.ConfigMap{}
	err := ownerInfo.validateOwner(object)
	assert.NoError(t, err)

	// namespaced owner
	ownerInfo = NewOwnerInfoWithOwnerRef(ownerRef, "test-ns")
	object = &v1.ConfigMap{}
	err = ownerInfo.validateOwner(object)
	assert.Error(t, err)
	object = &v1.ConfigMap{}
	object.Namespace = "test-ns"
	err = ownerInfo.validateOwner(object)
	assert.NoError(t, err)
	object.Namespace = "different-ns"
	err = ownerInfo.validateOwner(object)
	assert.Error(t, err)
}

func TestValidateController(t *testing.T) {
	controllerRef := &metav1.OwnerReference{UID: "test-id"}
	ownerInfo := NewOwnerInfoWithOwnerRef(controllerRef, "")
	object := &v1.ConfigMap{}
	err := ownerInfo.validateController(object)
	assert.NoError(t, err)
	err = ownerInfo.SetControllerReference(object)
	assert.NoError(t, err)
	err = ownerInfo.validateController(object)
	assert.NoError(t, err)
	err = ownerInfo.SetControllerReference(object)
	assert.NoError(t, err)
	newControllerRef := &metav1.OwnerReference{UID: "different-id"}
	newOwnerInfo := NewOwnerInfoWithOwnerRef(newControllerRef, "")
	err = newOwnerInfo.validateController(object)
	assert.Error(t, err)
}

func TestSetOwnerReference(t *testing.T) {
	info := OwnerInfo{
		ownerRef: &metav1.OwnerReference{Name: "test-id"},
	}
	object := v1.ConfigMap{}
	err := info.SetOwnerReference(&object)
	assert.NoError(t, err)
	assert.Equal(t, object.GetOwnerReferences(), []metav1.OwnerReference{*info.ownerRef})

	err = info.SetOwnerReference(&object)
	assert.NoError(t, err)
	assert.Equal(t, object.GetOwnerReferences(), []metav1.OwnerReference{*info.ownerRef})

	info2 := OwnerInfo{
		ownerRef: &metav1.OwnerReference{Name: "test-id-2"},
	}
	err = info2.SetOwnerReference(&object)
	assert.NoError(t, err)
	assert.Equal(t, object.GetOwnerReferences(), []metav1.OwnerReference{*info.ownerRef, *info2.ownerRef})
}
