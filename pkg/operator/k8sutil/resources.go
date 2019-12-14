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

// MergeResourceRequirements merges two resource requirements together (first overrides second values)
import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func MergeResourceRequirements(first, second v1.ResourceRequirements) v1.ResourceRequirements {
	// if the first has a value not set check if second has and set it in first
	if _, ok := first.Limits[v1.ResourceCPU]; !ok {
		if _, ok = second.Limits[v1.ResourceCPU]; ok {
			if first.Limits == nil {
				first.Limits = v1.ResourceList{}
			}
			first.Limits[v1.ResourceCPU] = second.Limits[v1.ResourceCPU]
		}
	}
	if _, ok := first.Limits[v1.ResourceMemory]; !ok {
		if _, ok = second.Limits[v1.ResourceMemory]; ok {
			if first.Limits == nil {
				first.Limits = v1.ResourceList{}
			}
			first.Limits[v1.ResourceMemory] = second.Limits[v1.ResourceMemory]
		}
	}
	if _, ok := first.Requests[v1.ResourceCPU]; !ok {
		if _, ok = second.Requests[v1.ResourceCPU]; ok {
			if first.Requests == nil {
				first.Requests = v1.ResourceList{}
			}
			first.Requests[v1.ResourceCPU] = second.Requests[v1.ResourceCPU]
		}
	}
	if _, ok := first.Requests[v1.ResourceMemory]; !ok {
		if _, ok = second.Requests[v1.ResourceMemory]; ok {
			if first.Requests == nil {
				first.Requests = v1.ResourceList{}
			}
			first.Requests[v1.ResourceMemory] = second.Requests[v1.ResourceMemory]
		}
	}
	return first
}

func SetOwnerRef(object *metav1.ObjectMeta, ownerRef *metav1.OwnerReference) {
	if ownerRef == nil {
		return
	}
	SetOwnerRefs(object, []metav1.OwnerReference{*ownerRef})
}

func SetOwnerRefsWithoutBlockOwner(object *metav1.ObjectMeta, ownerRefs []metav1.OwnerReference) {
	if ownerRefs == nil {
		return
	}
	newOwners := []metav1.OwnerReference{}
	for _, ownerRef := range ownerRefs {
		// Make a new copy of the owner ref so we don't impact existing references to it
		// but don't add the Controller or BlockOwnerDeletion properties
		newRef := metav1.OwnerReference{
			APIVersion: ownerRef.APIVersion,
			Kind:       ownerRef.Kind,
			Name:       ownerRef.Name,
			UID:        ownerRef.UID,
		}
		newOwners = append(newOwners, newRef)
	}
	SetOwnerRefs(object, newOwners)
}

func SetOwnerRefs(object *metav1.ObjectMeta, ownerRefs []metav1.OwnerReference) {
	object.SetOwnerReferences(ownerRefs)
}
