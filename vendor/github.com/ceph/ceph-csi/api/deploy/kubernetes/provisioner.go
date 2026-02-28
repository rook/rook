/*
Copyright 2024 The Ceph-CSI Authors.

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

package kubernetes

import (
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
)

// CSIProvisionerRBAC describes the interface that is provided by different
// provisioner backends to get details about the required RBAC.
type CSIProvisionerRBAC interface {
	GetServiceAccount() *corev1.ServiceAccount
	GetClusterRole() *rbacv1.ClusterRole
	GetClusterRoleBinding() *rbacv1.ClusterRoleBinding
	GetRole() *rbacv1.Role
	GetRoleBinding() *rbacv1.RoleBinding
}

// CSIProvisionerRBACValues contains values that can be passed to
// NewCSIProvisionerRBAC() functions for different provisioner backends.
type CSIProvisionerRBACValues struct {
	Namespace      string
	ServiceAccount string
}
