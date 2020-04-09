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
	"strings"

	"github.com/banzaicloud/k8s-objectmatcher/patch"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	cephVersionLabelKey = "ceph_version"
)

// WatchControllerPredicate is a special update filter for update events
// do not reconcile if the the status changes, this avoids a reconcile storm loop
//
// returning 'true' means triggering a reconciliation
// returning 'false' means do NOT trigger a reconciliation
func WatchControllerPredicate() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			logger.Debug("create event from the parent object")
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			logger.Debug("delete event from the parent object")
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			logger.Debug("update event from the parent object")
			// resource.Quantity has non-exportable fields, so we use its comparator method
			resourceQtyComparer := cmp.Comparer(func(x, y resource.Quantity) bool { return x.Cmp(y) == 0 })

			switch objOld := e.ObjectOld.(type) {
			case *cephv1.CephObjectStore:
				objNew := e.ObjectNew.(*cephv1.CephObjectStore)
				logger.Debug("update event from the parent object CephObjectStore")
				diff := cmp.Diff(objOld.Spec, objNew.Spec, resourceQtyComparer)
				if diff != "" || objOld.GetDeletionTimestamp() != objNew.GetDeletionTimestamp() {
					// Checking if diff is not empty so we don't print it when the CR gets deleted
					if diff != "" {
						logger.Infof("CR has changed for %q. diff=%s", objNew.Name, diff)
					}
					return true
				} else if objOld.GetGeneration() != objNew.GetGeneration() {
					logger.Debugf("skipping resource %q update with unchanged spec", objNew.Name)
				}
				// Handling upgrades
				isUpgrade := isUpgrade(objOld.GetLabels(), objNew.GetLabels())
				if isUpgrade {
					return true
				}

			case *cephv1.CephObjectStoreUser:
				objNew := e.ObjectNew.(*cephv1.CephObjectStoreUser)
				logger.Debug("update event from the parent object CephObjectStoreUser")
				diff := cmp.Diff(objOld.Spec, objNew.Spec, resourceQtyComparer)
				if diff != "" || objOld.GetDeletionTimestamp() != objNew.GetDeletionTimestamp() {
					// Checking if diff is not empty so we don't print it when the CR gets deleted
					if diff != "" {
						logger.Infof("CR has changed for %q. diff=%s", objNew.Name, diff)
					}
					return true
				} else if objOld.GetGeneration() != objNew.GetGeneration() {
					logger.Debugf("skipping resource %q update with unchanged spec", objNew.Name)
				}

			case *cephv1.CephBlockPool:
				objNew := e.ObjectNew.(*cephv1.CephBlockPool)
				logger.Debug("update event from the parent object CephBlockPool")
				diff := cmp.Diff(objOld.Spec, objNew.Spec, resourceQtyComparer)
				if diff != "" || objOld.GetDeletionTimestamp() != objNew.GetDeletionTimestamp() {
					// Checking if diff is not empty so we don't print it when the CR gets deleted
					if diff != "" {
						logger.Infof("CR has changed for %q. diff=%s", objNew.Name, diff)
					}
					return true
				} else if objOld.GetGeneration() != objNew.GetGeneration() {
					logger.Debugf("skipping resource %q update with unchanged spec", objNew.Name)
				}

			case *cephv1.CephFilesystem:
				objNew := e.ObjectNew.(*cephv1.CephFilesystem)
				logger.Debug("update event from the parent object CephFilesystem")
				diff := cmp.Diff(objOld.Spec, objNew.Spec, resourceQtyComparer)
				if diff != "" || objOld.GetDeletionTimestamp() != objNew.GetDeletionTimestamp() {
					// Checking if diff is not empty so we don't print it when the CR gets deleted
					if diff != "" {
						logger.Infof("CR has changed for %q. diff=%s", objNew.Name, diff)
					}
					return true
				} else if objOld.GetGeneration() != objNew.GetGeneration() {
					logger.Debugf("skipping resource %q update with unchanged spec", objNew.Name)
				}
				// Handling upgrades
				isUpgrade := isUpgrade(objOld.GetLabels(), objNew.GetLabels())
				if isUpgrade {
					return true
				}

			case *cephv1.CephNFS:
				objNew := e.ObjectNew.(*cephv1.CephNFS)
				logger.Debug("update event from the parent object CephNFS")
				diff := cmp.Diff(objOld.Spec, objNew.Spec, resourceQtyComparer)
				if diff != "" || objOld.GetDeletionTimestamp() != objNew.GetDeletionTimestamp() {
					// Checking if diff is not empty so we don't print it when the CR gets deleted
					if diff != "" {
						logger.Infof("CR has changed for %q. diff=%s", objNew.Name, diff)
					}
					return true
				} else if objOld.GetGeneration() != objNew.GetGeneration() {
					logger.Debugf("skipping resource %q update with unchanged spec", objNew.Name)
				}
				// Handling upgrades
				isUpgrade := isUpgrade(objOld.GetLabels(), objNew.GetLabels())
				if isUpgrade {
					return true
				}
			}

			logger.Debug("wont update unknown object")
			return false
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	}
}

// objectChanged checks whether the object has been updated
func objectChanged(oldObj, newObj runtime.Object) (bool, error) {
	var doReconcile bool
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
		doReconcile = true
		return doReconcile, errors.Wrap(err, "failed to calculate object diff")
	} else if diff.IsEmpty() {
		return doReconcile, nil
	}

	return isValidEvent(diff.Patch), nil
}

// WatchPredicateForNonCRDObject is a special filter for create events
// It only applies to non-CRD objects, meaning, for instance a cephv1.CephBlockPool{}
// object will not have this filter
// Only for objects like &v1.Secret{} etc...
//
// We return 'false' on a create event so we don't overstep with the main watcher on cephv1.CephBlockPool{}
// This avoids a double reconcile when the secret gets deleted.
func WatchPredicateForNonCRDObject(owner runtime.Object, scheme *runtime.Scheme) predicate.Funcs {
	// Initialize the Owner Matcher, which is the main controller object: e.g. cephv1.CephBlockPool{}
	ownerMatcher := NewOwnerReferenceMatcher(owner, scheme)

	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			match, object, err := ownerMatcher.Match(e.Object)
			if err != nil {
				logger.Errorf("failed to check if object kind %q matched. %v", e.Object.GetObjectKind(), err)
			}
			if match {
				logger.Debugf("object %q matched on delete", object.GetName())
				return true
			}

			logger.Debugf("object %q did not match on delete", object.GetName())
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			match, object, err := ownerMatcher.Match(e.ObjectNew)
			if err != nil {
				logger.Errorf("failed to check if object matched. %v", err)
			}
			if match {
				logger.Debugf("object %q matched on update", object.GetName())
				objectChanged, err := objectChanged(e.ObjectOld, e.ObjectNew)
				if err != nil {
					logger.Errorf("failed to check if object %q changed. %v", object.GetName(), err)
				}
				return objectChanged
			}

			return false
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	}
}

// isValidEvent analyses the diff between two objects events and determines
// if we should reconcile that event or not
// The goal is to avoid double-reconcile as much as possible
func isValidEvent(patch []byte) bool {
	patchString := string(patch)

	// Seem a bit 'weak' but since we can't get a real struct of the object
	// (unless we use the unstructured package, but that over complicates things)
	// That's probably the most straightforward approach for now...
	//
	// The downscale only shows a "deletionTimestamp" which is not appropriate to catch
	if strings.Contains(patchString, "Created new replica set") {
		logger.Debug("don't reconcile on replicaset addition")
		return false
	}

	// It looks like there is a diff
	// if the status changed, we do nothing
	var p map[string]interface{}
	json.Unmarshal(patch, &p)
	delete(p, "status")
	if len(p) == 0 {
		return false
	}

	logger.Infof("will reconcile based on patch %s", patchString)
	return true
}

func isUpgrade(oldLabels, newLabels map[string]string) bool {
	oldLabelVal, oldLabelKeyExist := oldLabels[cephVersionLabelKey]
	newLabelVal, newLabelKeyExist := newLabels[cephVersionLabelKey]

	// Nothing exists
	if !oldLabelKeyExist && !newLabelKeyExist {
		return false
	}

	// The new object has the label key so we reconcile
	if !oldLabelKeyExist && newLabelKeyExist {
		return true
	}

	// Both objects have the label and values are different so we reconcile
	if (oldLabelKeyExist && newLabelKeyExist) && oldLabelVal != newLabelVal {
		return true
	}

	return false
}
