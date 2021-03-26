/*
Copyright 2020 The Rook Authors. All rights reserved.

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

package controller

import (
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// OwnerMatcher is a struct representing the controller owner reference
// to use for comparison with child objects
type OwnerMatcher struct {
	owner              runtime.Object
	ownerMeta          metav1.Object
	ownerTypeGroupKind schema.GroupKind
	scheme             *runtime.Scheme
}

// NewOwnerReferenceMatcher initializes a new owner reference matcher
func NewOwnerReferenceMatcher(owner runtime.Object, scheme *runtime.Scheme) (*OwnerMatcher, error) {
	m := &OwnerMatcher{
		owner:  owner,
		scheme: scheme,
	}

	meta, _ := meta.Accessor(owner)
	m.ownerMeta = meta
	if err := m.setOwnerTypeGroupKind(); err != nil {
		return m, errors.Wrap(err, "failed to set ownerType %v")
	}

	return m, nil
}

// Match checks whether a given object matches the parent controller owner reference
// It is used in the predicate functions for non-CRD objects to ensure we only watch resources
// that have the parent Kind in its owner reference AND the same UID
//
// So we won't reconcile other object is we have multiple CRs
//
// For example, for CephObjectStore we will only watch "secrets" that have an owner reference
// referencing the 'CephObjectStore' Kind
func (e *OwnerMatcher) Match(object runtime.Object) (bool, metav1.Object, error) {
	o, err := meta.Accessor(object)
	if err != nil {
		return false, o, errors.Wrapf(err, "could not access object meta kind %q", object.GetObjectKind())
	}

	// Iterate over owner reference of the child object
	for _, owner := range e.getOwnersReferences(o) {
		groupVersion, err := schema.ParseGroupVersion(owner.APIVersion)
		if err != nil {
			return false, o, errors.Wrapf(err, "could not parse api version %q", owner.APIVersion)
		}

		if (e.ownerMeta.GetUID() == "" || (e.ownerMeta.GetUID() != "" && owner.UID == e.ownerMeta.GetUID())) && owner.Kind == e.ownerTypeGroupKind.Kind && groupVersion.Group == e.ownerTypeGroupKind.Group {
			return true, o, nil
		}
	}

	return false, o, nil
}

func (e *OwnerMatcher) getOwnersReferences(object metav1.Object) []metav1.OwnerReference {
	if object == nil {
		return nil
	}
	ownerRef := metav1.GetControllerOf(object)
	if ownerRef != nil {
		return []metav1.OwnerReference{*ownerRef}
	}

	return nil
}

func (e *OwnerMatcher) setOwnerTypeGroupKind() error {
	kinds, _, err := e.scheme.ObjectKinds(e.owner)
	if err != nil || len(kinds) < 1 {
		return errors.Wrapf(err, "could not get object kinds %v", e.owner)
	}

	e.ownerTypeGroupKind = schema.GroupKind{Group: kinds[0].Group, Kind: kinds[0].Kind}
	return nil
}
