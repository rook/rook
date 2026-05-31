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
	"fmt"
	"slices"
	"strings"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/util/log"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// remove removes any element from a list
func remove(list []string, s string) []string {
	for i, v := range list {
		if v == s {
			list = append(list[:i], list[i+1:]...)
		}
	}

	return list
}

// AddFinalizerIfNotPresent adds a finalizer an object to avoid instant deletion
// of the object without finalizing it.
func AddFinalizerIfNotPresent(ctx context.Context, client client.Client, obj client.Object) (bool, error) {
	objectFinalizer := buildFinalizerName(obj.GetObjectKind().GroupVersionKind().Kind)
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return false, errors.Wrap(err, "failed to get meta information of object")
	}

	if !slices.Contains(accessor.GetFinalizers(), objectFinalizer) {
		log.NamedInfo(NsName(obj.GetNamespace(), obj.GetName()), logger, "adding finalizer %q", objectFinalizer)
		accessor.SetFinalizers(append(accessor.GetFinalizers(), objectFinalizer))
		originalGeneration := obj.GetGeneration()

		// Update CR with finalizer
		if err := client.Update(ctx, obj); err != nil {
			return false, errors.Wrapf(err, "failed to add finalizer %q on %q", objectFinalizer, accessor.GetName())
		}
		newGeneration := obj.GetGeneration()
		log.NamedDebug(NsName(obj.GetNamespace(), obj.GetName()), logger, "when adding finalizer, original generation %d, new generation %d", originalGeneration, newGeneration)
		return originalGeneration != newGeneration, nil
	}

	return false, nil
}

// RemoveFinalizer removes a finalizer from an object
func RemoveFinalizer(ctx context.Context, client client.Client, obj client.Object) error {
	finalizerName := buildFinalizerName(obj.GetObjectKind().GroupVersionKind().Kind)
	return RemoveFinalizerWithName(ctx, client, obj, finalizerName)
}

// RemoveFinalizerWithName removes finalizer passed as an argument from an object
func RemoveFinalizerWithName(ctx context.Context, client client.Client, obj client.Object, finalizerName string) error {
	err := client.Get(ctx, types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}, obj)
	if err != nil {
		return errors.Wrap(err, "failed to get the latest version of the object")
	}
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return errors.Wrap(err, "failed to get meta information of object")
	}

	if slices.Contains(accessor.GetFinalizers(), finalizerName) {
		log.NamedInfo(NsName(obj.GetNamespace(), obj.GetName()), logger, "removing finalizer %q", finalizerName)
		accessor.SetFinalizers(remove(accessor.GetFinalizers(), finalizerName))
		if err := client.Update(ctx, obj); err != nil {
			return errors.Wrapf(err, "failed to remove finalizer %q on %q", finalizerName, accessor.GetName())
		}
	}

	return nil
}

// buildFinalizerName returns the finalizer name
func buildFinalizerName(kind string) string {
	return fmt.Sprintf("%s.%s", strings.ToLower(kind), cephv1.CustomResourceGroup)
}
