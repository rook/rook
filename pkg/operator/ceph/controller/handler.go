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
	"context"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
)

// ObjectToCRMapper returns the list of a given object type metadata
// It is used to trigger a reconcile object Kind A when watching object Kind B
// So we reconcile Kind A instead of Kind B
// For instance, we watch for CephCluster CR changes but want to reconcile CephFilesystem based on a Spec change
func ObjectToCRMapper(ctx context.Context, c client.Client, ro runtime.Object, scheme *runtime.Scheme) (handler.MapFunc, error) {
	if _, ok := ro.(metav1.ListInterface); !ok {
		return nil, errors.Errorf("expected a metav1.ListInterface, got %T instead", ro)
	}

	gvk, err := apiutil.GVKForObject(ro, scheme)
	if err != nil {
		return nil, err
	}

	// return handler.EnqueueRequestsFromMapFunc(func(o client.Object) []ctrl.Request {
	return handler.MapFunc(func(ctx context.Context, o client.Object) []ctrl.Request {
		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(gvk)
		err := c.List(ctx, list)
		if err != nil {
			return nil
		}

		results := []ctrl.Request{}
		for _, obj := range list.Items {
			results = append(results, ctrl.Request{
				NamespacedName: client.ObjectKey{
					Namespace: obj.GetNamespace(),
					Name:      obj.GetName(),
				},
			})
		}
		return results

	}), nil
}
