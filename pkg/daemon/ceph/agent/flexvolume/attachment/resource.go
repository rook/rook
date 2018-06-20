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

// Package attachment to manage Kubernetes storage attach events.
package attachment

import (
	"reflect"

	opkit "github.com/rook/operator-kit"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha2"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
)

const (
	CustomResourceName       = "volume"
	CustomResourceNamePlural = "volumes"
)

// VolumeResource represents the Volume custom resource object
var VolumeResource = opkit.CustomResource{
	Name:    CustomResourceName,
	Plural:  CustomResourceNamePlural,
	Group:   rookalpha.CustomResourceGroup,
	Version: rookalpha.Version,
	Scope:   apiextensionsv1beta1.NamespaceScoped,
	Kind:    reflect.TypeOf(rookalpha.Volume{}).Name(),
}
