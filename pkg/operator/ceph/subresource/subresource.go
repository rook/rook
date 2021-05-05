/*
Copyright 2021 The Rook Authors. All rights reserved.

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

package subresource

import (
	"github.com/rook/rook/pkg/clusterd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Subresource interface {
	// Kind is the Kubernetes Kind of the dependent subresource. e.g. "CephFilesystem" or "CephNFS"
	Kind() string

	// DependentsOf returns the name of each instance of the subresource that has been created that
	// depends on the given object. The Object should be a fully-filled Kubernetes object so that
	// the dependent subresource can check whether it is a dependent of the object.
	DependentsOf(context *clusterd.Context, obj client.Object) ([]string, error)
}
