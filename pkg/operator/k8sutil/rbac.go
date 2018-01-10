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
*/

// Package k8sutil for Kubernetes helpers.
package k8sutil

import (
	"fmt"
	"os"

	"k8s.io/api/core/v1"
	"k8s.io/api/rbac/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	enableRBACEnv = "RBAC_ENABLED"
)

func MakeRole(clientset kubernetes.Interface, namespace, name string, rules []v1beta1.PolicyRule, ownerRef metav1.OwnerReference) error {

	err := makeServiceAccount(clientset, namespace, name, &ownerRef)
	if err != nil {
		return err
	}

	if !isRBACEnabled() {
		return nil
	}

	// Create the role if it doesn't yet exist.
	// If the role already exists we have to update it. Otherwise if the permissions change during an upgrade,
	// the create will fail with an error that we're changing the permissions.
	role := &v1beta1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			OwnerReferences: []metav1.OwnerReference{ownerRef},
		},
		Rules: rules,
	}
	_, err = clientset.RbacV1beta1().Roles(namespace).Get(role.Name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		logger.Infof("creating role %s in namespace %s", name, namespace)
		_, err = clientset.RbacV1beta1().Roles(namespace).Create(role)
	} else if err == nil {
		logger.Infof("role %s already exists in namespace %s. updating if needed.", name, namespace)
		_, err = clientset.RbacV1beta1().Roles(namespace).Update(role)
	}
	if err != nil {
		return fmt.Errorf("failed to create/update role %s in namespace %s. %+v", name, namespace, err)
	}

	binding := &v1beta1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			OwnerReferences: []metav1.OwnerReference{ownerRef},
		},
		RoleRef: v1beta1.RoleRef{
			Name:     name,
			Kind:     "Role",
			APIGroup: "rbac.authorization.k8s.io",
		},
		Subjects: []v1beta1.Subject{
			{Kind: "ServiceAccount", Name: name, Namespace: namespace},
		},
	}
	_, err = clientset.RbacV1beta1().RoleBindings(namespace).Create(binding)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create %s role binding in namespace %s. %+v", name, namespace, err)
	}
	return nil
}

func MakeClusterRole(clientset kubernetes.Interface, namespace, name string, rules []v1beta1.PolicyRule, ownerRef *metav1.OwnerReference) error {

	err := makeServiceAccount(clientset, namespace, name, ownerRef)
	if err != nil {
		return err
	}

	if !isRBACEnabled() {
		return nil
	}

	// Create the cluster scoped role if it doesn't yet exist.
	// If the role already exists we have to update it. Otherwise if the permissions change during an upgrade,
	// the create will fail with an error that we're changing the permissions.
	role := &v1beta1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Rules: rules,
	}
	if ownerRef != nil {
		role.OwnerReferences = []metav1.OwnerReference{*ownerRef}
	}

	_, err = clientset.RbacV1beta1().ClusterRoles().Get(role.Name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		logger.Infof("creating cluster role %s", name)
		_, err = clientset.RbacV1beta1().ClusterRoles().Create(role)
	} else if err == nil {
		logger.Infof("cluster role %s already exists. Updating if needed.", name)
		_, err = clientset.RbacV1beta1().ClusterRoles().Update(role)
	}
	if err != nil {
		return fmt.Errorf("failed to create/update cluster role %s. %+v", name, err)
	}

	binding := &v1beta1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		RoleRef: v1beta1.RoleRef{
			Name:     name,
			Kind:     "ClusterRole",
			APIGroup: "rbac.authorization.k8s.io",
		},
		Subjects: []v1beta1.Subject{
			{Kind: "ServiceAccount", Name: name, Namespace: namespace},
		},
	}
	if ownerRef != nil {
		binding.OwnerReferences = []metav1.OwnerReference{*ownerRef}
	}

	_, err = clientset.RbacV1beta1().ClusterRoleBindings().Create(binding)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create cluster role binding %s. %+v", name, err)
	}
	return nil
}

func makeServiceAccount(clientset kubernetes.Interface, namespace, name string, ownerRef *metav1.OwnerReference) error {
	account := &v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
	if ownerRef != nil {
		account.OwnerReferences = []metav1.OwnerReference{*ownerRef}
	}

	_, err := clientset.CoreV1().ServiceAccounts(namespace).Create(account)
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create %s service account in namespace %s. %+v", name, namespace, err)
	}
	return nil
}

func isRBACEnabled() bool {
	r := os.Getenv(enableRBACEnv)
	if r == "false" {
		return false
	}
	return true
}
