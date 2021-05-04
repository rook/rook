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
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/k8sutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	cephVersionLabelKey     = "ceph_version"
	DoNotReconcileLabelName = "do_not_reconcile"
)

// WatchControllerPredicate is a special update filter for update events
// do not reconcile if the the status changes, this avoids a reconcile storm loop
//
// returning 'true' means triggering a reconciliation
// returning 'false' means do NOT trigger a reconciliation
func WatchControllerPredicate() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			logger.Debug("create event from a CR")
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			logger.Debug("delete event from a CR")
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			logger.Debug("update event from a CR")
			// resource.Quantity has non-exportable fields, so we use its comparator method
			resourceQtyComparer := cmp.Comparer(func(x, y resource.Quantity) bool { return x.Cmp(y) == 0 })

			switch objOld := e.ObjectOld.(type) {
			case *cephv1.CephObjectStore:
				objNew := e.ObjectNew.(*cephv1.CephObjectStore)
				logger.Debug("update event on CephObjectStore CR")
				// If the labels "do_not_reconcile" is set on the object, let's not reconcile that request
				IsDoNotReconcile := IsDoNotReconcile(objNew.GetLabels())
				if IsDoNotReconcile {
					logger.Debugf("object %q matched on update but %q label is set, doing nothing", DoNotReconcileLabelName, objNew.Name)
					return false
				}
				diff := cmp.Diff(objOld.Spec, objNew.Spec, resourceQtyComparer)
				if diff != "" {
					logger.Infof("CR has changed for %q. diff=%s", objNew.Name, diff)
					return true
				} else if objOld.GetDeletionTimestamp() != objNew.GetDeletionTimestamp() {
					logger.Debugf("CR %q is going to be deleted", objNew.Name)
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
				logger.Debug("update event on CephObjectStoreUser CR")
				// If the labels "do_not_reconcile" is set on the object, let's not reconcile that request
				IsDoNotReconcile := IsDoNotReconcile(objNew.GetLabels())
				if IsDoNotReconcile {
					logger.Debugf("object %q matched on update but %q label is set, doing nothing", DoNotReconcileLabelName, objNew.Name)
					return false
				}
				diff := cmp.Diff(objOld.Spec, objNew.Spec, resourceQtyComparer)
				if diff != "" {
					logger.Infof("CR has changed for %q. diff=%s", objNew.Name, diff)
					return true
				} else if objOld.GetDeletionTimestamp() != objNew.GetDeletionTimestamp() {
					logger.Debugf("CR %q is going to be deleted", objNew.Name)
					return true
				} else if objOld.GetGeneration() != objNew.GetGeneration() {
					logger.Debugf("skipping resource %q update with unchanged spec", objNew.Name)
				}

			case *cephv1.CephObjectRealm:
				objNew := e.ObjectNew.(*cephv1.CephObjectRealm)
				logger.Debug("update event on CephObjectRealm")
				// If the labels "do_not_reconcile" is set on the object, let's not reconcile that request
				IsDoNotReconcile := IsDoNotReconcile(objNew.GetLabels())
				if IsDoNotReconcile {
					logger.Debugf("object %q matched on update but %q label is set, doing nothing", DoNotReconcileLabelName, objNew.Name)
					return false
				}
				diff := cmp.Diff(objOld.Spec, objNew.Spec, resourceQtyComparer)
				if diff != "" {
					logger.Infof("CR has changed for %q. diff=%s", objNew.Name, diff)
					return true
				} else if objOld.GetDeletionTimestamp() != objNew.GetDeletionTimestamp() {
					logger.Debugf("CR %q is going to be deleted", objNew.Name)
					return true
				} else if objOld.GetGeneration() != objNew.GetGeneration() {
					logger.Debugf("skipping resource %q update with unchanged spec", objNew.Name)
				}

			case *cephv1.CephObjectZoneGroup:
				objNew := e.ObjectNew.(*cephv1.CephObjectZoneGroup)
				logger.Debug("update event on CephObjectZoneGroup")
				// If the labels "do_not_reconcile" is set on the object, let's not reconcile that request
				IsDoNotReconcile := IsDoNotReconcile(objNew.GetLabels())
				if IsDoNotReconcile {
					logger.Debugf("object %q matched on update but %q label is set, doing nothing", DoNotReconcileLabelName, objNew.Name)
					return false
				}
				diff := cmp.Diff(objOld.Spec, objNew.Spec, resourceQtyComparer)
				if diff != "" {
					logger.Infof("CR has changed for %q. diff=%s", objNew.Name, diff)
					return true
				} else if objOld.GetDeletionTimestamp() != objNew.GetDeletionTimestamp() {
					logger.Debugf("CR %q is going to be deleted", objNew.Name)
					return true
				} else if objOld.GetGeneration() != objNew.GetGeneration() {
					logger.Debugf("skipping resource %q update with unchanged spec", objNew.Name)
				}

			case *cephv1.CephObjectZone:
				objNew := e.ObjectNew.(*cephv1.CephObjectZone)
				logger.Debug("update event on CephObjectZone")
				// If the labels "do_not_reconcile" is set on the object, let's not reconcile that request
				IsDoNotReconcile := IsDoNotReconcile(objNew.GetLabels())
				if IsDoNotReconcile {
					logger.Debugf("object %q matched on update but %q label is set, doing nothing", DoNotReconcileLabelName, objNew.Name)
					return false
				}
				diff := cmp.Diff(objOld.Spec, objNew.Spec, resourceQtyComparer)
				if diff != "" {
					logger.Infof("CR has changed for %q. diff=%s", objNew.Name, diff)
					return true
				} else if objOld.GetDeletionTimestamp() != objNew.GetDeletionTimestamp() {
					logger.Debugf("CR %q is going to be deleted", objNew.Name)
					return true
				} else if objOld.GetGeneration() != objNew.GetGeneration() {
					logger.Debugf("skipping resource %q update with unchanged spec", objNew.Name)
				}

			case *cephv1.CephBlockPool:
				objNew := e.ObjectNew.(*cephv1.CephBlockPool)
				logger.Debug("update event on CephBlockPool CR")
				// If the labels "do_not_reconcile" is set on the object, let's not reconcile that request
				IsDoNotReconcile := IsDoNotReconcile(objNew.GetLabels())
				if IsDoNotReconcile {
					logger.Debugf("object %q matched on update but %q label is set, doing nothing", DoNotReconcileLabelName, objNew.Name)
					return false
				}
				diff := cmp.Diff(objOld.Spec, objNew.Spec, resourceQtyComparer)
				if diff != "" {
					logger.Infof("CR has changed for %q. diff=%s", objNew.Name, diff)
					return true
				} else if objOld.GetDeletionTimestamp() != objNew.GetDeletionTimestamp() {
					logger.Debugf("CR %q is going to be deleted", objNew.Name)
					return true
				} else if objOld.GetGeneration() != objNew.GetGeneration() {
					logger.Debugf("skipping resource %q update with unchanged spec", objNew.Name)
				}

			case *cephv1.CephFilesystem:
				objNew := e.ObjectNew.(*cephv1.CephFilesystem)
				logger.Debug("update event on CephFilesystem CR")
				// If the labels "do_not_reconcile" is set on the object, let's not reconcile that request
				IsDoNotReconcile := IsDoNotReconcile(objNew.GetLabels())
				if IsDoNotReconcile {
					logger.Debugf("object %q matched on update but %q label is set, doing nothing", DoNotReconcileLabelName, objNew.Name)
					return false
				}
				diff := cmp.Diff(objOld.Spec, objNew.Spec, resourceQtyComparer)
				if diff != "" {
					logger.Infof("CR has changed for %q. diff=%s", objNew.Name, diff)
					return true
				} else if objOld.GetDeletionTimestamp() != objNew.GetDeletionTimestamp() {
					logger.Debugf("CR %q is going to be deleted", objNew.Name)
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
				logger.Debug("update event on CephNFS CR")
				// If the labels "do_not_reconcile" is set on the object, let's not reconcile that request
				IsDoNotReconcile := IsDoNotReconcile(objNew.GetLabels())
				if IsDoNotReconcile {
					logger.Debugf("object %q matched on update but %q label is set, doing nothing", DoNotReconcileLabelName, objNew.Name)
					return false
				}
				diff := cmp.Diff(objOld.Spec, objNew.Spec, resourceQtyComparer)
				if diff != "" {
					logger.Infof("CR has changed for %q. diff=%s", objNew.Name, diff)
					return true
				} else if objOld.GetDeletionTimestamp() != objNew.GetDeletionTimestamp() {
					logger.Debugf("CR %q is going to be deleted", objNew.Name)
					return true
				} else if objOld.GetGeneration() != objNew.GetGeneration() {
					logger.Debugf("skipping resource %q update with unchanged spec", objNew.Name)
				}
				// Handling upgrades
				isUpgrade := isUpgrade(objOld.GetLabels(), objNew.GetLabels())
				if isUpgrade {
					return true
				}

			case *cephv1.CephRBDMirror:
				objNew := e.ObjectNew.(*cephv1.CephRBDMirror)
				logger.Debug("update event on CephRBDMirror CR")
				// If the labels "do_not_reconcile" is set on the object, let's not reconcile that request
				IsDoNotReconcile := IsDoNotReconcile(objNew.GetLabels())
				if IsDoNotReconcile {
					logger.Debugf("object %q matched on update but %q label is set, doing nothing", DoNotReconcileLabelName, objNew.Name)
					return false
				}
				diff := cmp.Diff(objOld.Spec, objNew.Spec, resourceQtyComparer)
				if diff != "" {
					logger.Infof("CR has changed for %q. diff=%s", objNew.Name, diff)
					return true
				} else if objOld.GetDeletionTimestamp() != objNew.GetDeletionTimestamp() {
					logger.Debugf("CR %q is going to be deleted", objNew.Name)
					return true
				} else if objOld.GetGeneration() != objNew.GetGeneration() {
					logger.Debugf("skipping resource %q update with unchanged spec", objNew.Name)
				}
				// Handling upgrades
				isUpgrade := isUpgrade(objOld.GetLabels(), objNew.GetLabels())
				if isUpgrade {
					return true
				}

			case *cephv1.CephClient:
				objNew := e.ObjectNew.(*cephv1.CephClient)
				logger.Debug("update event on CephClient CR")
				// If the labels "do_not_reconcile" is set on the object, let's not reconcile that request
				IsDoNotReconcile := IsDoNotReconcile(objNew.GetLabels())
				if IsDoNotReconcile {
					logger.Debugf("object %q matched on update but %q label is set, doing nothing", DoNotReconcileLabelName, objNew.Name)
					return false
				}
				diff := cmp.Diff(objOld.Spec, objNew.Spec, resourceQtyComparer)
				if diff != "" {
					logger.Infof("CR has changed for %q. diff=%s", objNew.Name, diff)
					return true
				} else if objOld.GetDeletionTimestamp() != objNew.GetDeletionTimestamp() {
					logger.Debugf("CR %q is going to be deleted", objNew.Name)
					return true
				} else if objOld.GetGeneration() != objNew.GetGeneration() {
					logger.Debugf("skipping resource %q update with unchanged spec", objNew.Name)
				}
				// Handling upgrades
				isUpgrade := isUpgrade(objOld.GetLabels(), objNew.GetLabels())
				if isUpgrade {
					return true
				}

			case *cephv1.CephFilesystemMirror:
				objNew := e.ObjectNew.(*cephv1.CephFilesystemMirror)
				logger.Debug("update event on CephFilesystemMirror CR")
				// If the labels "do_not_reconcile" is set on the object, let's not reconcile that request
				IsDoNotReconcile := IsDoNotReconcile(objNew.GetLabels())
				if IsDoNotReconcile {
					logger.Debugf("object %q matched on update but %q label is set, doing nothing", DoNotReconcileLabelName, objNew.Name)
					return false
				}
				diff := cmp.Diff(objOld.Spec, objNew.Spec, resourceQtyComparer)
				if diff != "" {
					logger.Infof("CR has changed for %q. diff=%s", objNew.Name, diff)
					return true
				} else if objOld.GetDeletionTimestamp() != objNew.GetDeletionTimestamp() {
					logger.Debugf("CR %q is going to be deleted", objNew.Name)
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

			return false
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	}
}

// WatchCephClusterPredicate is a predicate used by child controllers such as Filesystem or Object
// It watch for CR changes on the CephCluster object and reconciles if this needs to be propagated
// For instance the logCollector option from the CephCluster spec affects the configuration of rgw pods
// So if it changes we must update the deployment
func WatchCephClusterPredicate() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			logger.Debug("create event from a CR")
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			logger.Debug("delete event from a CR")
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			logger.Debug("update event from a CR")
			// resource.Quantity has non-exportable fields, so we use its comparator method
			resourceQtyComparer := cmp.Comparer(func(x, y resource.Quantity) bool { return x.Cmp(y) == 0 })

			switch objOld := e.ObjectOld.(type) {
			case *cephv1.CephCluster:
				objNew := e.ObjectNew.(*cephv1.CephCluster)
				logger.Debug("update event on CephCluster CR")
				// If the labels "do_not_reconcile" is set on the object, let's not reconcile that request
				isDoNotReconcile := IsDoNotReconcile(objNew.GetLabels())
				if isDoNotReconcile {
					logger.Debugf("object %q matched on update but %q label is set, doing nothing", DoNotReconcileLabelName, objNew.Name)
					return false
				}
				diff := cmp.Diff(objOld.Spec, objNew.Spec, resourceQtyComparer)
				if diff != "" {
					// The image change (upgrade) is being taking care by watchControllerPredicate() in the cluster package
					if objOld.Spec.CephVersion.Image != objNew.Spec.CephVersion.Image {
						return false
					}
					// If the log collector setting changes let's reconcile the child controllers
					if !cmp.Equal(objOld.Spec.LogCollector, objNew.Spec.LogCollector) {
						logger.Debug("log collector option changed, reconciling")
						return true
					}
				} else if objOld.GetGeneration() != objNew.GetGeneration() {
					logger.Debugf("skipping resource %q update with unchanged spec", objNew.Name)
				}
			}

			return false
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	}
}

// objectChanged checks whether the object has been updated
func objectChanged(oldObj, newObj runtime.Object, objectName string) (bool, error) {
	var doReconcile bool
	old := oldObj.DeepCopyObject()
	new := newObj.DeepCopyObject()

	// Set resource version
	accessor := meta.NewAccessor()
	currentResourceVersion, err := accessor.ResourceVersion(old)
	if err == nil {
		if err := accessor.SetResourceVersion(new, currentResourceVersion); err != nil {
			return false, errors.Wrapf(err, "failed to set resource version to %s", currentResourceVersion)
		}
	} else {
		return false, errors.Wrap(err, "failed to query current resource version")
	}

	// Calculate diff between old and new object
	diff, err := patch.DefaultPatchMaker.Calculate(old, new)
	if err != nil {
		doReconcile = true
		return doReconcile, errors.Wrap(err, "failed to calculate object diff but let's reconcile just in case")
	} else if diff.IsEmpty() {
		logger.Debugf("object %q diff is empty, nothing to reconcile", objectName)
		return doReconcile, nil
	}

	// Do not leak details of diff if the object contains sensitive data (e.g., it is a Secret)
	isSensitive := false
	if _, ok := new.(*corev1.Secret); ok {
		logger.Debugf("object %q diff is [redacted for Secrets]", objectName)
		isSensitive = true
	} else {
		logger.Debugf("object %q diff is %s", objectName, diff.String())
		isSensitive = false
	}

	return isValidEvent(diff.Patch, objectName, isSensitive), nil
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
	ownerMatcher, err := NewOwnerReferenceMatcher(owner, scheme)
	if err != nil {
		logger.Errorf("failed to initialize owner matcher. %v", err)
	}

	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},

		DeleteFunc: func(e event.DeleteEvent) bool {
			match, object, err := ownerMatcher.Match(e.Object)
			if err != nil {
				logger.Errorf("failed to check if object kind %q matched. %v", e.Object.GetObjectKind(), err)
			}
			objectName := object.GetName()
			if match {
				// If the resource is a CM, we might want to ignore it since some of them are ephemeral
				isCMToIgnoreOnDelete := isCMToIgnoreOnDelete(e.Object)
				if isCMToIgnoreOnDelete {
					return false
				}

				// If the resource is a canary deployment we don't reconcile because it's ephemeral
				isCanary := isCanary(e.Object)
				if isCanary {
					return false
				}

				logger.Infof("object %q matched on delete, reconciling", objectName)
				return true
			}

			logger.Debugf("object %q did not match on delete", objectName)
			return false
		},

		UpdateFunc: func(e event.UpdateEvent) bool {
			match, object, err := ownerMatcher.Match(e.ObjectNew)
			if err != nil {
				logger.Errorf("failed to check if object matched. %v", err)
			}
			objectName := object.GetName()
			if match {
				// If the labels "do_not_reconcile" is set on the object, let's not reconcile that request
				IsDoNotReconcile := IsDoNotReconcile(object.GetLabels())
				if IsDoNotReconcile {
					logger.Debugf("object %q matched on update but %q label is set, doing nothing", DoNotReconcileLabelName, objectName)
					return false
				}

				logger.Debugf("object %q matched on update", objectName)

				// CONFIGMAP WHITELIST
				// Only reconcile on rook-config-override CM changes
				isCMTConfigOverride := isCMTConfigOverride(e.ObjectNew)
				if isCMTConfigOverride {
					logger.Debugf("do reconcile when the cm is %s", k8sutil.ConfigOverrideName)
					return true
				}

				// If the resource is a ConfigMap we don't reconcile
				_, ok := e.ObjectNew.(*corev1.ConfigMap)
				if ok {
					logger.Debugf("do not reconcile on configmap that is not %q", k8sutil.ConfigOverrideName)
					return false
				}

				// SECRETS BLACKLIST
				// If the resource is a Secret, we might want to ignore it
				// We want to reconcile Secrets in case their content gets altered
				isSecretToIgnoreOnUpdate := isSecretToIgnoreOnUpdate(e.ObjectNew)
				if isSecretToIgnoreOnUpdate {
					return false
				}

				// If the resource is a deployment we don't reconcile
				_, ok = e.ObjectNew.(*appsv1.Deployment)
				if ok {
					logger.Debug("do not reconcile deployments updates")
					return false
				}

				// did the object change?
				objectChanged, err := objectChanged(e.ObjectOld, e.ObjectNew, objectName)
				if err != nil {
					logger.Errorf("failed to check if object %q changed. %v", objectName, err)
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

// isValidEvent analyses the diff between two objects events and determines if we should reconcile
// that event or not. The goal is to avoid double-reconcile as much as possible.
// If the patch could contain sensitive data, isValidEvent will not leak the data to logs.
func isValidEvent(patch []byte, objectName string, patchContainsSensitiveData bool) bool {
	var p map[string]interface{}
	err := json.Unmarshal(patch, &p)
	if err != nil {
		logErrorUnlessSensitive("failed to unmarshal patch", err, patchContainsSensitiveData)
		return false
	}
	if !patchContainsSensitiveData {
		logger.Debugf("patch before trimming is %s", string(patch))
	}

	// don't reconcile on status update on an object (e.g. status "creating")
	logger.Debugf("trimming 'status' field from patch")
	delete(p, "status")

	// Do not reconcile on metadata change since managedFields are often updated by the server
	logger.Debugf("trimming 'metadata' field from patch")
	delete(p, "metadata")

	// If the patch is now empty, we don't reconcile
	if len(p) == 0 {
		logger.Debug("patch is empty after trimming")
		return false
	}

	// Re-marshal to get the last diff
	patch, err = json.Marshal(p)
	if err != nil {
		logErrorUnlessSensitive("failed to marshal patch", err, patchContainsSensitiveData)
		return false
	}

	// If after all the filtering there is still something in the patch, we reconcile
	text := string(patch)
	if patchContainsSensitiveData {
		text = "[redacted patch details due to potentially sensitive content]"
	}
	logger.Infof("controller will reconcile resource %q based on patch: %s", objectName, text)

	return true
}

func logErrorUnlessSensitive(msg string, err error, isSensitive bool) {
	if isSensitive {
		logger.Errorf("%s. [redacted error due to potentially sensitive content]", msg)
	} else {
		logger.Errorf("%s. %v", msg, err)
	}
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

func isCanary(obj runtime.Object) bool {
	// If not a deployment, let's not reconcile
	d, ok := obj.(*appsv1.Deployment)
	if !ok {
		return false
	}

	// Get the labels
	labels := d.GetLabels()

	labelVal, labelKeyExist := labels["mon_canary"]
	if labelKeyExist && labelVal == "true" {
		logger.Debugf("do not reconcile %q on monitor canary deployments", d.Name)
		return true
	}

	return false
}

func isCMTConfigOverride(obj runtime.Object) bool {
	// If not a ConfigMap, let's not reconcile
	cm, ok := obj.(*corev1.ConfigMap)
	if !ok {
		return false
	}

	objectName := cm.GetName()
	return objectName == k8sutil.ConfigOverrideName
}

func isCMToIgnoreOnDelete(obj runtime.Object) bool {
	// If not a ConfigMap, let's not reconcile
	cm, ok := obj.(*corev1.ConfigMap)
	if !ok {
		return false
	}

	objectName := cm.GetName()
	// is it the object the temporarily osd config map?
	if strings.HasPrefix(objectName, "rook-ceph-osd-") && strings.HasSuffix(objectName, "-status") {
		logger.Debugf("do not reconcile on %q config map changes", objectName)
		return true
	}

	return false
}

func isSecretToIgnoreOnUpdate(obj runtime.Object) bool {
	s, ok := obj.(*corev1.Secret)
	if !ok {
		return false
	}

	objectName := s.GetName()
	switch objectName {
	case config.StoreName:
		logger.Debugf("do not reconcile on %q secret changes", objectName)
		return true
	}

	return false
}

func IsDoNotReconcile(labels map[string]string) bool {
	value, ok := labels[DoNotReconcileLabelName]

	// Nothing exists
	if ok && value == "true" {
		return true
	}

	return false
}
