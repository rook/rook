/*
Copyright 2016 The Kubernetes Authors.

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

package utils

// Source: https://github.com/kubernetes/kubernetes/blob/v1.21.1/pkg/apis/storage/v1/util/helpers.go

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// isDefaultStorageClassAnnotation represents a StorageClass annotation that
// marks a class as the default StorageClass
const isDefaultStorageClassAnnotation = "storageclass.kubernetes.io/is-default-class"

// betaIsDefaultStorageClassAnnotation is the beta version of IsDefaultStorageClassAnnotation.
// TODO: remove Beta when no longer used
const betaIsDefaultStorageClassAnnotation = "storageclass.beta.kubernetes.io/is-default-class"

// isDefaultAnnotation returns a boolean if
// the annotation is set
// TODO: remove Beta when no longer needed
func isDefaultAnnotation(obj metav1.ObjectMeta) bool {
	if obj.Annotations[isDefaultStorageClassAnnotation] == "true" {
		return true
	}
	if obj.Annotations[betaIsDefaultStorageClassAnnotation] == "true" {
		return true
	}
	return false
}
