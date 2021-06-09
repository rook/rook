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

package k8sutil

import (
	"context"
	"reflect"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Patcher is a utility for patching of objects and their status.
// Portion of this file is coming from https://github.com/kubernetes-sigs/cluster-api/blob/master/util/patch/patch.go
type Patcher struct {
	client      client.Client
	hasStatus   bool
	old         map[string]interface{}
	oldStatus   interface{}
	objectPatch client.Patch
	statusPatch client.Patch
}

func NewPatcher(object client.Object, crclient client.Client) (*Patcher, error) {
	if object == nil {
		return nil, errors.Errorf("expected non-nil resource")
	}

	if _, ok := object.(runtime.Unstructured); ok {
		object = object.DeepCopyObject().(client.Object)
	}

	old, err := runtime.DefaultUnstructuredConverter.ToUnstructured(object)
	if err != nil {
		return nil, err
	}

	hasStatus := false
	oldStatus, ok, err := unstructured.NestedFieldCopy(old, "status")
	if err != nil {
		return nil, err
	}
	if ok {
		hasStatus = true
		unstructured.RemoveNestedField(old, "status")
	}

	return &Patcher{
		client:      crclient,
		hasStatus:   hasStatus,
		old:         old,
		oldStatus:   oldStatus,
		objectPatch: client.MergeFrom(object.DeepCopyObject().(client.Object)),
		statusPatch: client.MergeFrom(object.DeepCopyObject().(client.Object)),
	}, nil
}

func (p *Patcher) Patch(ctx context.Context, object client.Object) error {
	if object == nil {
		return errors.Errorf("expected non-nil resource")
	}

	newObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(object)
	if err != nil {
		return err
	}

	hasStatus := false
	newStatus, ok, err := unstructured.NestedFieldCopy(newObj, "status")
	if err != nil {
		return err
	}

	if ok {
		hasStatus = true
		unstructured.RemoveNestedField(newObj, "status")
	}

	var errs []error
	if !reflect.DeepEqual(p.old, newObj) {
		if err := p.client.Patch(ctx, object, p.objectPatch); err != nil {
			errs = append(errs, err)
		}
	}

	if (p.hasStatus || hasStatus) && !reflect.DeepEqual(p.oldStatus, newStatus) {
		if err := p.client.Status().Patch(ctx, object, p.statusPatch); err != nil {
			errs = append(errs, err)
		}
	}

	return kerrors.NewAggregate(errs)
}
