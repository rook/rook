/*
Copyright 2018 The Rook Authors. All rights reserved.

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

package mon

import (
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/k8sutil"
	v1 "k8s.io/api/core/v1"
)

// PodNamespaceEnvVar is the cluster namespace environment var
func PodNamespaceEnvVar(namespace string) v1.EnvVar {
	return v1.EnvVar{Name: k8sutil.PodNamespaceEnvVar, Value: namespace}
}

// EndpointEnvVar is the mon endpoint environment var
func EndpointEnvVar() v1.EnvVar {
	ref := &v1.ConfigMapKeySelector{LocalObjectReference: v1.LocalObjectReference{Name: EndpointConfigMapName}, Key: EndpointDataKey}
	return v1.EnvVar{Name: "ROOK_MON_ENDPOINTS", ValueFrom: &v1.EnvVarSource{ConfigMapKeyRef: ref}}
}

// CephUsernameEnvVar is the ceph username environment var
func CephUsernameEnvVar() v1.EnvVar {
	ref := &v1.SecretKeySelector{LocalObjectReference: v1.LocalObjectReference{Name: AppName}, Key: opcontroller.CephUsernameKey}
	return v1.EnvVar{Name: "ROOK_CEPH_USERNAME", ValueFrom: &v1.EnvVarSource{SecretKeyRef: ref}}
}
