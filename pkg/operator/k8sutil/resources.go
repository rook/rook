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

Some of the code below came from https://github.com/coreos/etcd-operator
which also has the apache 2.0 license.
*/

// Package k8sutil for Kubernetes helpers.
package k8sutil

// MergeResourceRequirements merges two resource requirements together (first overrides second values)
import (
	"k8s.io/api/core/v1"
)

func MergeResourceRequirements(first, second v1.ResourceRequirements) v1.ResourceRequirements {
	// if the first has a value not set check if second has and set it in first
	if _, ok := first.Limits[v1.ResourceCPU]; !ok {
		if first.Limits == nil {
			first.Limits = v1.ResourceList{}
		}
		first.Limits[v1.ResourceCPU] = second.Limits[v1.ResourceCPU]
	}
	if _, ok := first.Limits[v1.ResourceMemory]; !ok {
		if first.Limits == nil {
			first.Limits = v1.ResourceList{}
		}
		first.Limits[v1.ResourceMemory] = second.Limits[v1.ResourceMemory]
	}
	if _, ok := first.Requests[v1.ResourceCPU]; !ok {
		if first.Requests == nil {
			first.Requests = v1.ResourceList{}
		}
		first.Requests[v1.ResourceCPU] = second.Requests[v1.ResourceCPU]
	}
	if _, ok := first.Requests[v1.ResourceMemory]; !ok {
		if first.Requests == nil {
			first.Requests = v1.ResourceList{}
		}
		first.Requests[v1.ResourceMemory] = second.Requests[v1.ResourceMemory]
	}
	return first
}
