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
	"reflect"

	"k8s.io/apimachinery/pkg/api/meta"

	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CreateOrUpdateObject updates an object with a given status
func CreateOrUpdateObject(ctx context.Context, client client.Client, obj client.Object) error {
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return errors.Wrap(err, "failed to get meta information of object")
	}
	objName := accessor.GetName()

	// Somehow meta.TypeAccessor returns an empty string for the type name so using reflection instead
	objType := reflect.TypeOf(obj)

	err = client.Create(ctx, obj)
	if err != nil {
		if kerrors.IsAlreadyExists(err) {
			err = client.Update(ctx, obj)
			if err != nil {
				return errors.Wrapf(err, "failed to update ceph %q object %q", objType, objName)
			}

			logger.Infof("updated ceph %q object %q", objType, objName)
			return nil
		}
		return errors.Wrapf(err, "failed to create ceph %v object %q", objType, objName)
	}

	logger.Infof("created ceph %v object %q", objType, objName)
	return nil
}
