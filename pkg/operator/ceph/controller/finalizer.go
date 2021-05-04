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

// AddSelfFinalizerIfNotPresent adds a self-referencing finalizer to an object to avoid instant
// deletion of the object without finalizing it: "<object-kind>.ceph.rook.io"
func AddSelfFinalizerIfNotPresent(client client.Client, obj client.Object) error {
	objectFinalizer := buildFinalizerBaseName(obj)

	return addFinalizerIfNotPresent(client, obj, objectFinalizer)
}

// AddNamedSubresourceFinalizerIfNotPresent adds a finalizer for the named Ceph subresource to the
// CephCluster with the subresource's name: "<subresource-kind>.ceph.rook.io/<subresourceName>"
func AddNamedSubresourceFinalizerIfNotPresent(client client.Client, cephClusterObj, subresourceObj client.Object, subresourceName string) error {
	namedFinalizer := buildNamedFinalizer(subresourceObj, subresourceName)

	return addFinalizerIfNotPresent(client, cephClusterObj, namedFinalizer)
}

func addFinalizerIfNotPresent(client client.Client, obj client.Object, finalizer string) error {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return errors.Wrapf(err, "failed to add finalizer %q. failed to get meta information of object", finalizer)
	}

	if !contains(accessor.GetFinalizers(), finalizer) {
		logger.Infof("adding finalizer %q on %q", finalizer, accessor.GetName())
		accessor.SetFinalizers(append(accessor.GetFinalizers(), finalizer))

		// Update CR with finalizer
		if err := client.Update(context.TODO(), obj); err != nil {
			return errors.Wrapf(err, "failed to add finalizer %q on %q", finalizer, accessor.GetName())
		}
	}

	return nil
}

// RemoveSelfFinalizer removes a self-referencing finalizer from an object.
func RemoveSelfFinalizer(client client.Client, obj client.Object) error {
	objectFinalizer := buildFinalizerBaseName(obj)

	return removeFinalizer(client, obj, objectFinalizer)
}

// RemoveNamedSubresourceFinalizer removes a finalizer for the named Ceph subresource.
func RemoveNamedSubresourceFinalizer(client client.Client, cephClusterObj, subresourceObj client.Object, subresourceName string) error {
	namedFinalizer := buildNamedFinalizer(subresourceObj, subresourceName)

	return removeFinalizer(client, cephClusterObj, namedFinalizer)
}

func removeFinalizer(client client.Client, obj client.Object, finalizer string) error {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return errors.Wrapf(err, "failed to remove finalizer %q. failed to get meta information of object", finalizer)
	}

	if contains(accessor.GetFinalizers(), finalizer) {
		logger.Infof("removing finalizer %q on %q", finalizer, accessor.GetName())
		accessor.SetFinalizers(remove(accessor.GetFinalizers(), finalizer))
		if err := client.Update(context.TODO(), obj); err != nil {
			return errors.Wrapf(err, "failed to remove finalizer %q on %q", finalizer, accessor.GetName())
		}
	}

	return nil
}

// FinalizersExist returns a list of non-self-referencing finalizers present on the object.
// TODO: TEST
func GetNonSelfFinalizers(obj client.Object) ([]string, error) {
	objectFinalizer := buildFinalizerBaseName(obj)

	accessor, err := meta.Accessor(obj)
	if err != nil {
		return []string{}, errors.Wrap(err, "failed check finalizers. failed to get meta information of object")
	}

	finalizers := accessor.GetFinalizers()
	nonSelfFinalizers := make([]string, 0, len(finalizers))
	for _, f := range finalizers {
		if f == objectFinalizer {
			continue
		}
		nonSelfFinalizers = append(nonSelfFinalizers, f)
	}

	return nonSelfFinalizers, nil
}

// buildFinalizerBaseName returns the finalizer name: "<object-kind>.ceph.rook.io".
func buildFinalizerBaseName(obj client.Object) string {
	kind := obj.GetObjectKind().GroupVersionKind().Kind
	return fmt.Sprintf("%s.%s", strings.ToLower(kind), cephv1.CustomResourceGroup)
}

// buildNamedFinalizer returns the finalizer name with a name added:
// "<object-kind>.ceph.rook.io/<finalizerName>"
func buildNamedFinalizer(obj client.Object, finalizerName string) string {
	return fmt.Sprintf("%s/%s", buildFinalizerBaseName(obj), finalizerName)
}
