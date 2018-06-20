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

// Package k8sutil for Kubernetes helpers.
package k8sutil

import (
	"os"
	"testing"

	"github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	"k8s.io/api/rbac/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMakeRole(t *testing.T) {
	clientset := test.New(1)
	namespace := "myns"
	name := "myapp"

	rules := []v1beta1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"namespaces", "secrets", "pods", "services", "nodes", "configmaps", "events"},
			Verbs:     []string{"get", "list", "watch", "create", "update"},
		},
		{
			APIGroups: []string{"extensions"},
			Resources: []string{"thirdpartyresources", "deployments", "daemonsets", "replicasets"},
			Verbs:     []string{"get", "list", "create", "delete"},
		},
		{
			APIGroups: []string{"apiextensions.k8s.io"},
			Resources: []string{"customresourcedefinitions"},
			Verbs:     []string{"get", "list", "create"},
		},
		{
			APIGroups: []string{"storage.k8s.io"},
			Resources: []string{"storageclasses"},
			Verbs:     []string{"get", "list"},
		},
	}

	ownerRef := metav1.OwnerReference{}
	err := MakeRole(clientset, namespace, name, rules, &ownerRef)

	role, err := clientset.RbacV1beta1().Roles(namespace).Get(name, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, name, role.Name)
	assert.Equal(t, 4, len(role.Rules))
	account, err := clientset.CoreV1().ServiceAccounts(namespace).Get(name, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, namespace, account.Namespace)
	binding, err := clientset.RbacV1beta1().RoleBindings(namespace).Get(name, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, name, binding.RoleRef.Name)
	assert.Equal(t, "Role", binding.RoleRef.Kind)
	assert.Equal(t, "rbac.authorization.k8s.io", binding.RoleRef.APIGroup)
	assert.Equal(t, name, binding.Subjects[0].Name)
	assert.Equal(t, "ServiceAccount", binding.Subjects[0].Kind)

	// update the rules
	newRules := []v1beta1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"namespaces"},
			Verbs:     []string{"get", "list"},
		},
	}

	err = MakeRole(clientset, namespace, name, newRules, &ownerRef)
	assert.Nil(t, err)
	role, err = clientset.RbacV1beta1().Roles(namespace).Get(name, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, 1, len(role.Rules))
	assert.Equal(t, "", role.Rules[0].APIGroups[0])
	assert.Equal(t, 1, len(role.Rules[0].Resources))
	assert.Equal(t, 2, len(role.Rules[0].Verbs))
}

func TestMakeClusterRole(t *testing.T) {
	clientset := test.New(1)
	namespace := "myns"
	name := "myapp"

	rules := []v1beta1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"pods", "secrets", "configmaps", "persistentvolumes", "nodes/proxy"},
			Verbs:     []string{"get", "list"},
		},
		{
			APIGroups: []string{"storage.k8s.io"},
			Resources: []string{"storageclasses"},
			Verbs:     []string{"get"},
		},
	}

	err := MakeClusterRole(clientset, namespace, name, rules, nil)

	role, err := clientset.RbacV1beta1().ClusterRoles().Get(name, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, name, role.Name)
	assert.Equal(t, 2, len(role.Rules))
	account, err := clientset.CoreV1().ServiceAccounts(namespace).Get(name, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, namespace, account.Namespace)
	binding, err := clientset.RbacV1beta1().ClusterRoleBindings().Get(name, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, name, binding.RoleRef.Name)
	assert.Equal(t, "ClusterRole", binding.RoleRef.Kind)
	assert.Equal(t, "rbac.authorization.k8s.io", binding.RoleRef.APIGroup)
	assert.Equal(t, name, binding.Subjects[0].Name)
	assert.Equal(t, "ServiceAccount", binding.Subjects[0].Kind)

	// update the rules
	newRules := []v1beta1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"namespaces"},
			Verbs:     []string{"get", "list"},
		},
	}

	err = MakeClusterRole(clientset, namespace, name, newRules, nil)
	assert.Nil(t, err)
	role, err = clientset.RbacV1beta1().ClusterRoles().Get(name, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, 1, len(role.Rules))
	assert.Equal(t, "", role.Rules[0].APIGroups[0])
	assert.Equal(t, 1, len(role.Rules[0].Resources))
	assert.Equal(t, 2, len(role.Rules[0].Verbs))
}

func TestMakeRoleRBACDisabled(t *testing.T) {
	os.Setenv(enableRBACEnv, "false")
	defer os.Unsetenv(enableRBACEnv)
	clientset := test.New(1)
	namespace := "myns"
	name := "myapp"

	rules := []v1beta1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"namespaces", "secrets", "pods", "services", "nodes", "configmaps", "events"},
			Verbs:     []string{"get", "list", "watch", "create", "update"},
		},
		{
			APIGroups: []string{"extensions"},
			Resources: []string{"thirdpartyresources", "deployments", "daemonsets", "replicasets"},
			Verbs:     []string{"get", "list", "create", "delete"},
		},
		{
			APIGroups: []string{"apiextensions.k8s.io"},
			Resources: []string{"customresourcedefinitions"},
			Verbs:     []string{"get", "list", "create"},
		},
		{
			APIGroups: []string{"storage.k8s.io"},
			Resources: []string{"storageclasses"},
			Verbs:     []string{"get", "list"},
		},
	}
	ownerRef := metav1.OwnerReference{}
	err := MakeRole(clientset, namespace, name, rules, &ownerRef)

	_, err = clientset.RbacV1beta1().Roles(namespace).Get(name, metav1.GetOptions{})
	assert.NotNil(t, err)
	assert.True(t, errors.IsNotFound(err))

	account, err := clientset.CoreV1().ServiceAccounts(namespace).Get(name, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, namespace, account.Namespace)

	_, err = clientset.RbacV1beta1().RoleBindings(namespace).Get(name, metav1.GetOptions{})
	assert.NotNil(t, err)
	assert.True(t, errors.IsNotFound(err))
}

func TestMakeClusterRoleRBACDisabled(t *testing.T) {
	os.Setenv(enableRBACEnv, "false")
	defer os.Unsetenv(enableRBACEnv)
	clientset := test.New(1)
	namespace := "myns"
	name := "myapp"

	rules := []v1beta1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"pods", "secrets", "configmaps", "persistentvolumes", "nodes/proxy"},
			Verbs:     []string{"get", "list"},
		},
		{
			APIGroups: []string{"storage.k8s.io"},
			Resources: []string{"storageclasses"},
			Verbs:     []string{"get"},
		},
	}

	err := MakeClusterRole(clientset, namespace, name, rules, nil)
	assert.Nil(t, err)
	_, err = clientset.RbacV1beta1().ClusterRoles().Get(name, metav1.GetOptions{})
	assert.NotNil(t, err)
	assert.True(t, errors.IsNotFound(err))

	account, err := clientset.CoreV1().ServiceAccounts(namespace).Get(name, metav1.GetOptions{})
	assert.Nil(t, err)
	assert.Equal(t, namespace, account.Namespace)

	_, err = clientset.RbacV1beta1().ClusterRoleBindings().Get(name, metav1.GetOptions{})
	assert.NotNil(t, err)
	assert.True(t, errors.IsNotFound(err))
}
