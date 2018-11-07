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
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

var (
	skipSetOwnerRefEnv bool
	testedSetOwnerRef  bool
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

func SetOwnerRef(clientset kubernetes.Interface, namespace string, object *metav1.ObjectMeta, ownerRef *metav1.OwnerReference) {
	if ownerRef == nil {
		return
	}
	SetOwnerRefs(clientset, namespace, object, []metav1.OwnerReference{*ownerRef})
}

func SetOwnerRefs(clientset kubernetes.Interface, namespace string, object *metav1.ObjectMeta, ownerRefs []metav1.OwnerReference) {
	if !testedSetOwnerRef {
		testSetOwnerRef(clientset, namespace, ownerRefs)
		testedSetOwnerRef = true
	}
	if skipSetOwnerRefEnv {
		return
	}

	// We want to set the owner ref unless we detect if it needs to be skipped.
	object.OwnerReferences = ownerRefs
}

func testSetOwnerRef(clientset kubernetes.Interface, namespace string, ownerRefs []metav1.OwnerReference) {
	// Confirm if we can create a resource with the ownerref set to the cluster CRD.
	// Some versions of OpenShift may not have support for setting the ownerref to a CRD.
	// See https://github.com/kubernetes/kubernetes/pull/62810
	cm := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "rook-test-ownerref",
			Namespace:       namespace,
			OwnerReferences: ownerRefs,
		},
		Data: map[string]string{},
	}
	_, err := clientset.CoreV1().ConfigMaps(namespace).Create(cm)
	if err != nil && !errors.IsAlreadyExists(err) {
		logger.Warningf("OwnerReferences will not be set on resources created by rook. failed to test that it can be set. %+v", err)
		skipSetOwnerRefEnv = true
		return
	}

	logger.Infof("verified the ownerref can be set on resources")
	skipSetOwnerRefEnv = false
}
