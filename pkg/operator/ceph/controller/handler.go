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

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ObjectToCRMapper returns the list of a given object type metadata
// It is used to trigger a reconcile object Kind A when watching object Kind B
// So we reconcile Kind A instead of Kind B
// For instance, we watch for CephCluster CR changes but want to reconcile CephFilesystem based on a Spec change
func ObjectToCRMapper[List client.ObjectList, T runtime.Object](ctx context.Context, c client.Client, list List, scheme *runtime.Scheme) (handler.TypedMapFunc[T, reconcile.Request], error) {
	gvk, err := apiutil.GVKForObject(list, scheme)
	if err != nil {
		return nil, err
	}

	return func(ctx context.Context, obj T) []reconcile.Request {
		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(gvk)
		err := c.List(ctx, list)
		if err != nil {
			return nil
		}

		results := []reconcile.Request{}
		for _, obj := range list.Items {
			results = append(results, reconcile.Request{
				NamespacedName: client.ObjectKey{
					Namespace: obj.GetNamespace(),
					Name:      obj.GetName(),
				},
			})
		}
		return results
	}, nil
}

func ConfigFromSecretToClusterMapper(ctx context.Context, c client.Client, list cephv1.CephClusterList, scheme *runtime.Scheme) (handler.TypedMapFunc[*corev1.Secret, reconcile.Request], error) {
	return func(ctx context.Context, secret *corev1.Secret) []reconcile.Request {
		logger.Info("Inside mapper func")
		err := c.List(ctx, &list)
		if err != nil {
			return nil
		}
		results := []reconcile.Request{}
		for _, obj := range list.Items {
			for _, secretKeyMap := range obj.Spec.CephConfigFromSecret {
				for _, keyselector := range secretKeyMap {
					if secret.Name == keyselector.Name {
						results = append(results, reconcile.Request{
							NamespacedName: client.ObjectKey{
								Namespace: obj.Namespace,
								Name:      obj.Name,
							},
						})
					}
				}
			}
		}
		return results
	}, nil
}
