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

package reporting

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// UpdateStatus updates an object with a given status. The object is updated with the latest version
// from the server on a successful update.
func UpdateStatus(client client.Client, obj client.Object) error {
	nsName := types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}

	// Try to update the status
	err := client.Status().Update(context.Background(), obj)
	// If the object doesn't exist yet, we need to initialize it
	if kerrors.IsNotFound(err) {
		err = client.Update(context.Background(), obj)
	}
	if err != nil {
		return errors.Wrapf(err, "failed to update object %q status", nsName.String())
	}

	return nil
}

// UpdateStatusCondition updates (or adds to) the status condition to the given object. The object
// is updated with the latest version from the server on a successful update.
func UpdateStatusCondition(
	client client.Client, obj cephv1.StatusConditionGetter, newCond cephv1.Condition,
) error {
	kind := obj.GetObjectKind().GroupVersionKind().Kind
	nsName := fmt.Sprintf("%s/%s", obj.GetNamespace(), obj.GetName())

	cephv1.SetStatusCondition(obj.GetStatusConditions(), newCond)
	if err := UpdateStatus(client, obj); err != nil {
		return errors.Wrapf(err, "failed to update %s %q status condition %s=%s", kind, nsName, newCond.Type, newCond.Status)
	}

	return nil
}
