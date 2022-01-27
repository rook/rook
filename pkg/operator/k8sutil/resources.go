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

// MergeResourceRequirements merges two resource requirements together (first overrides second values)
import (
	"encoding/json"
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// OwnerInfo is to set owner references. Only one of owner and ownerRef must be valid.
type OwnerInfo struct {
	owner             metav1.Object
	scheme            *runtime.Scheme
	ownerRef          *metav1.OwnerReference
	ownerRefNamespace string
}

// NewOwnerInfo create a new ownerInfo to set ownerReference by controllerutil
func NewOwnerInfo(owner metav1.Object, scheme *runtime.Scheme) *OwnerInfo {
	return &OwnerInfo{owner: owner, scheme: scheme}
}

// NewOwnerInfoWithOwnerRef create a new ownerInfo to set ownerReference by rook itself
func NewOwnerInfoWithOwnerRef(ownerRef *metav1.OwnerReference, namespace string) *OwnerInfo {
	return &OwnerInfo{ownerRef: ownerRef, ownerRefNamespace: namespace}
}

func (info *OwnerInfo) validateOwner(object metav1.Object) error {
	if info.ownerRefNamespace == "" {
		return nil
	}
	objectNamespace := object.GetNamespace()
	if objectNamespace == "" {
		return fmt.Errorf("cluster-scoped resource %q must not have a namespaced resource %q in namespace %q",
			object.GetName(), info.ownerRef.Name, info.ownerRefNamespace)

	}
	if info.ownerRefNamespace != objectNamespace {
		return fmt.Errorf("cross-namespaced owner references are disallowed. resource %q is in namespace %q, owner %q is in %q",
			object.GetName(), object.GetNamespace(), info.ownerRef.Name, info.ownerRefNamespace)
	}
	return nil
}

func (info *OwnerInfo) validateController(object metav1.Object) error {
	existingController := metav1.GetControllerOf(object)
	if existingController != nil && existingController.UID != info.ownerRef.UID {
		return fmt.Errorf("%q already set its controller %q", object.GetName(), info.ownerRef.Name)
	}
	return nil
}

// SetOwnerReference set the owner reference of object
func (info *OwnerInfo) SetOwnerReference(object metav1.Object) error {
	if info.owner != nil {
		return controllerutil.SetOwnerReference(info.owner, object, info.scheme)
	}
	if info.ownerRef == nil {
		return nil
	}
	err := info.validateOwner(object)
	if err != nil {
		return err
	}
	ownerRefs := object.GetOwnerReferences()
	for _, v := range ownerRefs {
		if referSameObject(v, *info.ownerRef) {
			return nil
		}
	}
	ownerRefs = append(ownerRefs, *info.ownerRef)
	object.SetOwnerReferences(ownerRefs)
	return nil
}

// The original code is in https://github.com/kubernetes-sigs/controller-runtime/blob/a905949b9040084f0c6d2a27ec70e77c3c5c0931/pkg/controller/controllerutil/controllerutil.go#L160
func referSameObject(a, b metav1.OwnerReference) bool {
	groupVersionA, err := schema.ParseGroupVersion(a.APIVersion)
	if err != nil {
		return false
	}

	groupVersionB, err := schema.ParseGroupVersion(b.APIVersion)
	if err != nil {
		return false
	}

	return groupVersionA.Group == groupVersionB.Group && a.Kind == b.Kind && a.Name == b.Name
}

// SetControllerReference set the controller reference of object
func (info *OwnerInfo) SetControllerReference(object metav1.Object) error {
	if info.owner != nil {
		return controllerutil.SetControllerReference(info.owner, object, info.scheme)
	}
	if info.ownerRef == nil {
		return nil
	}
	err := info.validateOwner(object)
	if err != nil {
		return err
	}
	err = info.validateController(object)
	if err != nil {
		return err
	}

	// Do not override the BlockOwnerDeletion is already set
	if info.ownerRef.BlockOwnerDeletion == nil {
		blockOwnerDeletion := true
		info.ownerRef.BlockOwnerDeletion = &blockOwnerDeletion
	}

	controller := true
	info.ownerRef.Controller = &controller
	ownerRefs := append(object.GetOwnerReferences(), *info.ownerRef)
	object.SetOwnerReferences(ownerRefs)
	return nil
}

// GetUID gets the UID of the owner
func (info *OwnerInfo) GetUID() types.UID {
	return info.owner.GetUID()
}

func MergeResourceRequirements(first, second v1.ResourceRequirements) v1.ResourceRequirements {
	// if the first has no limits set, apply the second limits if any are specified
	if len(first.Limits) == 0 {
		if len(second.Limits) > 0 {
			first.Limits = second.Limits
		}
	}
	// if the first has no requests set, apply the second requests if any are specified
	if len(first.Requests) == 0 {
		if len(second.Requests) > 0 {
			first.Requests = second.Requests
		}
	}
	return first
}

func SetOwnerRefsWithoutBlockOwner(object metav1.Object, ownerRefs []metav1.OwnerReference) {
	if ownerRefs == nil {
		return
	}
	newOwnerRefs := []metav1.OwnerReference{}
	for _, ownerRef := range ownerRefs {
		// Make a new copy of the owner ref so we don't impact existing references to it
		// but don't add the Controller or BlockOwnerDeletion properties
		newOwnerRef := metav1.OwnerReference{
			APIVersion: ownerRef.APIVersion,
			Kind:       ownerRef.Kind,
			Name:       ownerRef.Name,
			UID:        ownerRef.UID,
		}
		newOwnerRefs = append(newOwnerRefs, newOwnerRef)
	}
	object.SetOwnerReferences(newOwnerRefs)
}

type ContainerResource struct {
	Name     string                  `json:"name"`
	Resource v1.ResourceRequirements `json:"resource"`
}

// YamlToContainerResource takes raw YAML string and converts it to array of
// ContainerResource
func YamlToContainerResource(raw string) ([]ContainerResource, error) {
	resources := []ContainerResource{}
	if raw == "" {
		return resources, nil
	}
	rawJSON, err := yaml.ToJSON([]byte(raw))
	if err != nil {
		return resources, err
	}
	err = json.Unmarshal(rawJSON, &resources)
	if err != nil {
		return resources, err
	}
	return resources, nil
}
