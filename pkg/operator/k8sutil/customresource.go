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

// Package kit for Kubernetes operators
package k8sutil

// CustomResource is for creating a Kubernetes TPR/CRD
type CustomResource struct {
	// Name of the custom resource
	Name string

	// Plural of the custom resource in plural
	Plural string

	// Group the custom resource belongs to
	Group string

	// Version which should be defined in a const above
	Version string

	// Kind is the serialized interface of the resource.
	Kind string

	// APIVersion is the full API version name (combine Group and Version)
	APIVersion string
}
