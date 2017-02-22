/*
Copyright 2016 The Rook Authors. All rights reserved.

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
package k8sutil

import (
	"net/http"

	apierrors "k8s.io/client-go/pkg/api/errors"
	"k8s.io/client-go/pkg/api/unversioned"
)

const (
	Namespace        = "rook"
	DefaultNamespace = "default"
	DataDirVolume    = "rook-data"
	DataDir          = "/var/lib/rook"
	RookType         = "kubernetes.io/rook"
	RbdType          = "kubernetes.io/rbd"
)

func IsKubernetesResourceAlreadyExistError(err error) bool {
	se, ok := err.(*apierrors.StatusError)
	if !ok {
		return false
	}
	if se.Status().Code == http.StatusConflict && se.Status().Reason == unversioned.StatusReasonAlreadyExists {
		return true
	}
	return false
}

func IsKubernetesResourceNotFoundError(err error) bool {
	se, ok := err.(*apierrors.StatusError)
	if !ok {
		return false
	}
	if se.Status().Code == http.StatusNotFound && se.Status().Reason == unversioned.StatusReasonNotFound {
		return true
	}
	return false
}
