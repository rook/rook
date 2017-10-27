/*
Copyright 2017 The Kubernetes Authors.

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

// Package mds to manage a rook file system.
package mds

import (
	"reflect"

	opkit "github.com/rook/operator-kit"
	"github.com/rook/rook/pkg/operator/k8sutil"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	schemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	addToScheme   = schemeBuilder.AddToScheme
)

// FilesystemResource represents the file system custom resource
var FilesystemResource = opkit.CustomResource{
	Name:    "filesystem",
	Plural:  "filesystems",
	Group:   k8sutil.CustomResourceGroup,
	Version: k8sutil.V1Alpha1,
	Scope:   apiextensionsv1beta1.NamespaceScoped,
	Kind:    reflect.TypeOf(Filesystem{}).Name(),
}

// Kind takes an unqualified kind and returns back a Group qualified GroupKind
func Kind(kind string) schema.GroupKind {
	return schemeGroupVersion.WithKind(kind).GroupKind()
}

// Resource takes an unqualified resource and returns back a Group qualified GroupResource
func Resource(resource string) schema.GroupResource {
	return schemeGroupVersion.WithResource(resource).GroupResource()
}

// Adds the list of known types to api.Scheme.
func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(schemeGroupVersion,
		&Filesystem{},
		&FilesystemList{},
	)
	metav1.AddToGroupVersion(scheme, schemeGroupVersion)
	return nil
}
