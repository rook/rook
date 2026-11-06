/*
Copyright 2026 The Rook Authors. All rights reserved.

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

package test

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// NewFakeClientWithKind returns a controller-runtime fake client that keeps the kind of the objects
// it reads and writes, as the operator's client does. The fake client clears the kind of typed
// objects, but Rook derives finalizer names from it.
func NewFakeClientWithKind(s *runtime.Scheme, objects ...runtime.Object) client.WithWatch {
	setKind := func(c client.Client, obj client.Object) error {
		gvk, err := apiutil.GVKForObject(obj, c.Scheme())
		if err != nil {
			return err
		}
		obj.GetObjectKind().SetGroupVersionKind(gvk)
		return nil
	}

	return fake.NewClientBuilder().WithScheme(s).WithRuntimeObjects(objects...).WithInterceptorFuncs(interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			if err := c.Get(ctx, key, obj, opts...); err != nil {
				return err
			}
			return setKind(c, obj)
		},
		Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
			if err := c.Update(ctx, obj, opts...); err != nil {
				return err
			}
			return setKind(c, obj)
		},
		Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
			if err := c.Patch(ctx, obj, patch, opts...); err != nil {
				return err
			}
			return setKind(c, obj)
		},
		SubResourceUpdate: func(ctx context.Context, c client.Client, subResourceName string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
			if err := c.SubResource(subResourceName).Update(ctx, obj, opts...); err != nil {
				return err
			}
			return setKind(c, obj)
		},
		SubResourcePatch: func(ctx context.Context, c client.Client, subResourceName string, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
			if err := c.SubResource(subResourceName).Patch(ctx, obj, patch, opts...); err != nil {
				return err
			}
			return setKind(c, obj)
		},
	}).Build()
}
