/*
Copyright 2019 The Rook Authors. All rights reserved.

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

package noobaa

// This file implements operator methods for synching generic kubernetes resources.

import (
	"encoding/json"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
)

// SyncServiceAccount gets or creates the desired object, and updates desired to match the server
func (o *Operator) SyncServiceAccount(desired *corev1.ServiceAccount) error {
	fullname := desired.Namespace + "/" + desired.Name
	lister := o.ServiceAccountInformer.Lister().ServiceAccounts(desired.Namespace)
	client := o.KubeClient.CoreV1().ServiceAccounts(desired.Namespace)
	result, err := lister.Get(desired.Name)
	if errors.IsNotFound(err) {
		logger.Infof("Operator: Sync %s - Creating ...", fullname)
		result, err = client.Create(desired)
	}
	if err == nil {
		result.DeepCopyInto(desired)
	}
	return err
}

// SyncRole gets or creates the desired object, and updates desired to match the server
func (o *Operator) SyncRole(desired *rbacv1.Role) error {
	fullname := desired.Namespace + "/" + desired.Name
	lister := o.RoleInformer.Lister().Roles(desired.Namespace)
	client := o.KubeClient.RbacV1().Roles(desired.Namespace)
	result, err := lister.Get(desired.Name)
	if err == nil && !same(desired.Rules, result.Rules) {
		logger.Warningf("Operator: Sync %s - WARNING Diff found", fullname)
	}
	if errors.IsNotFound(err) {
		logger.Infof("Operator: Sync %s - Creating ...", fullname)
		result, err = client.Create(desired)
	}
	if err == nil {
		result.DeepCopyInto(desired)
	}
	return err
}

// SyncRoleBinding gets or creates the desired object, and updates desired to match the server
func (o *Operator) SyncRoleBinding(desired *rbacv1.RoleBinding) error {
	fullname := desired.Namespace + "/" + desired.Name
	lister := o.RoleBindingInformer.Lister().RoleBindings(desired.Namespace)
	client := o.KubeClient.RbacV1().RoleBindings(desired.Namespace)
	result, err := lister.Get(desired.Name)
	if err == nil &&
		(!same(desired.RoleRef, result.RoleRef) ||
			!same(desired.Subjects, result.Subjects)) {
		logger.Warningf("Operator: Sync %s - WARNING Diff found", fullname)
	}
	if errors.IsNotFound(err) {
		logger.Infof("Operator: Sync %s - Creating ...", fullname)
		result, err = client.Create(desired)
	}
	if err == nil {
		result.DeepCopyInto(desired)
	}
	return err
}

// SyncService gets or creates the desired object, and updates desired to match the server
func (o *Operator) SyncService(desired *corev1.Service) error {
	fullname := desired.Namespace + "/" + desired.Name
	lister := o.ServiceInformer.Lister().Services(desired.Namespace)
	client := o.KubeClient.CoreV1().Services(desired.Namespace)
	result, err := lister.Get(desired.Name)
	if errors.IsNotFound(err) {
		logger.Infof("Operator: Sync %s - Creating ...", fullname)
		result, err = client.Create(desired)
	}
	if err == nil {
		result.DeepCopyInto(desired)
	}
	return err
}

// SyncStatefulSet gets or creates the desired object, and updates desired to match the server
func (o *Operator) SyncStatefulSet(desired *appsv1.StatefulSet) error {
	fullname := desired.Namespace + "/" + desired.Name
	lister := o.StatefulSetInformer.Lister().StatefulSets(desired.Namespace)
	client := o.KubeClient.AppsV1().StatefulSets(desired.Namespace)
	result, err := lister.Get(desired.Name)
	if errors.IsNotFound(err) {
		logger.Infof("Operator: Sync %s - Creating ...", fullname)
		result, err = client.Create(desired)
	}
	if err == nil {
		result.DeepCopyInto(desired)
	}
	return err
}

// SyncSecret gets or creates the desired object, and updates desired to match the server
func (o *Operator) SyncSecret(desired *corev1.Secret) error {
	fullname := desired.Namespace + "/" + desired.Name
	lister := o.SecretInformer.Lister().Secrets(desired.Namespace)
	client := o.KubeClient.CoreV1().Secrets(desired.Namespace)
	result, err := lister.Get(desired.Name)
	if errors.IsNotFound(err) {
		logger.Infof("Operator: Sync %s - Creating ...", fullname)
		result, err = client.Create(desired)
	}
	if err == nil {
		result.DeepCopyInto(desired)
	}
	return err
}

// JSONPatch is the structure expected for kubernetes client patch of type JSONPatchType
type JSONPatch struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value"`
}

// MakePatch returns a JSON Patch RFC 6902
func MakePatch(patches ...JSONPatch) []byte {
	return MakeJSON(patches)
}

// MakeJSON returns a json encoded buffer assuming there is no error.
func MakeJSON(obj interface{}) []byte {
	patchBytes, _ := json.Marshal(obj)
	return patchBytes
}

// JSONDict is a convenient type for building JSON dictionaries
type JSONDict map[string]interface{}

// same compares two objects with DeepDerivative that only checks non missing fields from desired
func same(desired, existing interface{}) bool {
	return equality.Semantic.DeepDerivative(desired, existing)
}
