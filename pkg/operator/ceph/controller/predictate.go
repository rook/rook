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
	"encoding/json"

	"github.com/banzaicloud/k8s-objectmatcher/patch"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// WatchUpdatePredicate is a special update filter for update events
// do not reconcile if the the status changes, this avoids a reconcile storm loop
//
// returning 'true' means triggering a reconciliation
// returning 'false' means do NOT trigger a reconciliation
func WatchUpdatePredicate() predicate.Funcs {
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			objectChanged, err := objectChanged(e.ObjectOld, e.ObjectNew)
			if err != nil {
				logger.Errorf("failed to check if object changed. %v", err)
			}
			if !objectChanged {
				return false
			}

			return true
		},
	}
}

// objectChanged checks whether the object has been updated
func objectChanged(oldObj, newObj runtime.Object) (bool, error) {
	old := oldObj.DeepCopyObject()
	new := newObj.DeepCopyObject()

	// Set resource version
	accessor := meta.NewAccessor()
	currentResourceVersion, err := accessor.ResourceVersion(old)
	if err == nil {
		accessor.SetResourceVersion(new, currentResourceVersion)
	}

	// Calculate diff between old and new object
	diff, err := patch.DefaultPatchMaker.Calculate(old, new)
	if err != nil {
		return true, errors.Wrap(err, "failed to calculate object diff")
	} else if diff.IsEmpty() {
		return false, nil
	}
	logger.Debugf("%v", diff.String())

	// It looks like there is a diff
	// if the status changed, we do nothing
	var patch map[string]interface{}
	json.Unmarshal(diff.Patch, &patch)
	delete(patch, "status")
	if len(patch) == 0 {
		return false, nil
	}

	// Get object meta
	objectMeta, err := meta.Accessor(newObj)
	if err != nil {
		return true, errors.Wrap(err, "failed to get object meta")
	}

	// This handles the case where the object is deleted
	spec := patch["spec"]
	if spec != nil {
		logger.Infof("object spec %q changed with %v", objectMeta.GetName(), spec)
	}

	return true, nil
}

// WatchPredicateForNonCRDObject is a special filter for create events
// It only applies to non-CRD objects, meaning, for instance a cephv1.CephBlockPool{}
// object will not have this filter
// Only for objects like &v1.Secret{} or &v1.Pod{} etc...
//
// We return 'false' on a create event so we don't overstep with the main watcher on cephv1.CephBlockPool{}
// This avoids a double reconcile when the secret gets deleted.
func WatchPredicateForNonCRDObject() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
	}
}
