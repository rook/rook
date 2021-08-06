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
	"strings"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// contains checks if an item exists in a given list.
func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}

	return false
}

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
func AddFinalizerIfNotPresent(client client.Client, obj client.Object) error {
	objectFinalizer := buildFinalizerName(obj.GetObjectKind().GroupVersionKind().Kind)

	accessor, err := meta.Accessor(obj)
	if err != nil {
		return errors.Wrap(err, "failed to get meta information of object")
	}

	if !contains(accessor.GetFinalizers(), objectFinalizer) {
		logger.Infof("adding finalizer %q on %q", objectFinalizer, accessor.GetName())
		accessor.SetFinalizers(append(accessor.GetFinalizers(), objectFinalizer))

		// Update CR with finalizer
		if err := client.Update(context.TODO(), obj); err != nil {
			return errors.Wrapf(err, "failed to add finalizer %q on %q", objectFinalizer, accessor.GetName())
		}
	}

	return nil
}

// RemoveFinalizer removes a finalizer from an object
func RemoveFinalizer(client client.Client, obj client.Object) error {
	err := client.Get(context.TODO(), types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}, obj)
	if err != nil {
		return errors.Wrap(err, "failed to get the latest version of the object")
	}
	objectFinalizer := buildFinalizerName(obj.GetObjectKind().GroupVersionKind().Kind)
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return errors.Wrap(err, "failed to get meta information of object")
	}

	if contains(accessor.GetFinalizers(), objectFinalizer) {
		logger.Infof("removing finalizer %q on %q", objectFinalizer, accessor.GetName())
		accessor.SetFinalizers(remove(accessor.GetFinalizers(), objectFinalizer))
		if err := client.Update(context.TODO(), obj); err != nil {
			return errors.Wrapf(err, "failed to remove finalizer %q on %q", objectFinalizer, accessor.GetName())
		}
	}

	return nil
}

// buildFinalizerName returns the finalizer name
func buildFinalizerName(kind string) string {
	return fmt.Sprintf("%s.%s", strings.ToLower(kind), cephv1.CustomResourceGroup)
}
