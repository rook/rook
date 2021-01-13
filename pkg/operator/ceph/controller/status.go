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

	"k8s.io/apimachinery/pkg/api/meta"

	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// UpdateStatus updates an object with a given status
func UpdateStatus(client client.Client, obj client.Object) error {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return errors.Wrap(err, "failed to get meta information of object")
	}

	// Try to update the status
	err = client.Status().Update(context.Background(), obj)
	// If the object doesn't exist yet, we need to initialize it
	if kerrors.IsNotFound(err) {
		err = client.Update(context.Background(), obj)
	}
	if err != nil {
		if !kerrors.IsConflict(err) || !kerrors.IsInvalid(err) {
			return errors.Wrapf(err, "failed to update object %q status", accessor.GetName())
		}

		err = client.Status().Update(context.Background(), obj)
		if kerrors.IsNotFound(err) {
			err = client.Update(context.Background(), obj)
		}
		if err != nil {
			return errors.Wrapf(err, "failed to update object %q status", accessor.GetName())
		}
	}

	return nil
}
