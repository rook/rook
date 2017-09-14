// /*
// Copyright 2017 The Rook Authors. All rights reserved.

// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at

// 	http://www.apache.org/licenses/LICENSE-2.0

// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Some of the code below came from https://github.com/coreos/etcd-operator
// which also has the apache 2.0 license.
// */

package crd

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/kubernetes/pkg/api"
)

func RegisterFakeAPI() *runtime.Scheme {
	scheme := runtime.NewScheme()
	api.SchemeBuilder.AddToScheme(scheme)
	api.SchemeBuilder.Register(addKnownTypes)
	return scheme
}
