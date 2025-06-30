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
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"syscall"

	"github.com/banzaicloud/k8s-objectmatcher/patch"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/config/keyring"
	"github.com/rook/rook/pkg/operator/k8sutil"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	DoNotReconcileLabelName = "do_not_reconcile"
)

// Retrieve the GVK (GroupVersionKind) and NamespacedName for the given object.
// Primarily used for logging.
func objectInfo(scheme *runtime.Scheme, obj client.Object) (string, types.NamespacedName) {
	gvk, err := apiutil.GVKForObject(obj, scheme)
	if err != nil {
		logger.Debugf("Unable to get GVK for object %+v", obj)
	}

	nsName := types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}

	return gvk.Kind, nsName
}

// WatchControllerPredicate is a special update filter for update events
// do not reconcile if the status changes, this avoids a reconcile storm loop
//
// returning 'true' means triggering a reconciliation
// returning 'false' means do NOT trigger a reconciliation
func WatchControllerPredicate[T client.Object](scheme *runtime.Scheme) predicate.TypedFuncs[T] {
	return predicate.TypedFuncs[T]{
		CreateFunc: func(e event.TypedCreateEvent[T]) bool {
			kind, nsName := objectInfo(scheme, e.Object)
			logger.Debugf("create event for %q: %q", kind, nsName)
			return true
		},
		DeleteFunc: func(e event.TypedDeleteEvent[T]) bool {
			kind, nsName := objectInfo(scheme, e.Object)
			logger.Debugf("delete event for %q: %q", kind, nsName)
			return true
		},
		UpdateFunc: func(e event.TypedUpdateEvent[T]) bool {
			objOld := e.ObjectOld
			objNew := e.ObjectNew
			kind, nsName := objectInfo(scheme, objNew)

			logger.Debugf("update event for %q: %q", kind, nsName)

			// If the labels "do_not_reconcile" is set on the object, let's not reconcile that request
			IsDoNotReconcile := IsDoNotReconcile(objNew.GetLabels())
			if IsDoNotReconcile {
				logger.Debugf("resource %q: %q had update event but %q label is set, doing nothing", kind, nsName, DoNotReconcileLabelName)
				return false
			}

			// resource.Quantity has non-exportable fields, so we use its comparator method
			resourceQtyComparer := cmp.Comparer(func(x, y resource.Quantity) bool { return x.Cmp(y) == 0 })

			diff := cmp.Diff(GetSpec(objOld), GetSpec(objNew), resourceQtyComparer)
			if diff != "" {
				logger.Infof("resource %q: %q spec has changed. diff=%s", kind, nsName, diff)
				return true
			} else if objectToBeDeleted(objOld, objNew) {
				logger.Debugf("resource %q: %q is going to be deleted", kind, nsName)
				return true
			} else if objOld.GetGeneration() != objNew.GetGeneration() {
				logger.Debugf("reconciling %s %q with changed generation", kind, nsName.String())
				return true
			}
			return false
		},
		GenericFunc: func(e event.TypedGenericEvent[T]) bool {
			kind, nsName := objectInfo(scheme, e.Object)
			logger.Debugf("generic event for %q: %q", kind, nsName)
			return false
		},
	}
}

func objectToBeDeleted(oldObj, newObj client.Object) bool {
	return !oldObj.GetDeletionTimestamp().Equal(newObj.GetDeletionTimestamp())
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
	if s, ok := new.(*corev1.Secret); ok {
		logger.Debugf("object %q diff is [redacted for Secrets]", objectName)
		isSensitive = true

		// keyring secrets are a special case. rook updates these during reconciliation as needed,
		// and daemon pods automatically get updated keys with no need for another reconcile
		if _, ok := s.ObjectMeta.Annotations[keyring.KeyringAnnotation]; ok {
			logger.Debugf("not reconciling update to cephx keyring secret %q", objectName)
			return false, nil
		}
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
func WatchPredicateForNonCRDObject[T client.Object](owner runtime.Object, scheme *runtime.Scheme) predicate.TypedFuncs[T] {
	// Initialize the Owner Matcher, which is the main controller object: e.g. cephv1.CephBlockPool{}
	ownerMatcher, err := NewOwnerReferenceMatcher(owner, scheme)
	if err != nil {
		logger.Errorf("failed to initialize owner matcher. %v", err)
	}

	return predicate.TypedFuncs[T]{
		CreateFunc: func(e event.TypedCreateEvent[T]) bool {
			return false
		},

		DeleteFunc: func(e event.TypedDeleteEvent[T]) bool {
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

				// If the resource is a canary, crash collector, or exporter we don't reconcile because it's ephemeral
				if isCanary(e.Object) || isCrashCollector(e.Object) || isExporter(e.Object) {
					return false
				}

				logger.Infof("object %q matched on delete, reconciling", objectName)
				return true
			}

			logger.Debugf("object %q did not match on delete", objectName)
			return false
		},

		UpdateFunc: func(e event.TypedUpdateEvent[T]) bool {
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
				// Only reconcile on rook-config-override CM changes if the configmap changed
				shouldReconcileCM := shouldReconcileCM(e.ObjectOld, e.ObjectNew)
				if shouldReconcileCM {
					logger.Infof("reconcile due to updated configmap %s", k8sutil.ConfigOverrideName)
					return true
				}

				switch newObjCopy := any(e.ObjectNew).(type) {
				case *corev1.ConfigMap:
					// If the resource is a ConfigMap we don't reconcile
					logger.Debugf("do not reconcile on configmap %q", objectName)
					return false

				case *corev1.Secret:
					// SECRETS BLACKLIST
					// If the resource is a Secret, we might want to ignore it
					// We want to reconcile Secrets in case their content gets altered
					if isSecretToIgnoreOnUpdate(newObjCopy) {
						return false
					}

				case *appsv1.Deployment:
					// If the resource is a deployment we don't reconcile
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

		GenericFunc: func(e event.TypedGenericEvent[T]) bool {
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

func isCrashCollector(obj runtime.Object) bool {
	return isDeployment(obj, "rook-ceph-crashcollector")
}

func isExporter(obj runtime.Object) bool {
	return isDeployment(obj, "rook-ceph-exporter")
}

func isDeployment(obj runtime.Object, appName string) bool {
	// If not a deployment, let's not reconcile
	d, ok := obj.(*appsv1.Deployment)
	if !ok {
		return false
	}

	// Get the labels
	labels := d.GetLabels()

	labelVal, labelKeyExist := labels["app"]
	if labelKeyExist && labelVal == appName {
		logger.Debugf("do not reconcile %q on %s", d.Name, appName)
		return true
	}

	return false
}

func shouldReconcileCM(objOld runtime.Object, objNew runtime.Object) bool {
	// If not a ConfigMap, let's not reconcile
	cmNew, ok := objNew.(*corev1.ConfigMap)
	if !ok {
		return false
	}

	// If not a ConfigMap, let's not reconcile
	cmOld, ok := objOld.(*corev1.ConfigMap)
	if !ok {
		return false
	}

	objectName := cmNew.GetName()
	if objectName != k8sutil.ConfigOverrideName {
		return false
	}
	if !reflect.DeepEqual(cmNew.Data, cmOld.Data) {
		return true
	}
	return false
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

func isSecretToIgnoreOnUpdate(secret *corev1.Secret) bool {
	switch secret.GetName() {
	case config.StoreName:
		logger.Debugf("do not reconcile on %q secret changes", secret.GetName())
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

func ReloadManager() {
	p, _ := os.FindProcess(os.Getpid())
	_ = p.Signal(syscall.SIGHUP)
}

// DuplicateCephClusters determine whether a similar object exists in the same namespace
// mainly used for the CephCluster which we only support a single instance per namespace
func DuplicateCephClusters(ctx context.Context, c client.Client, object *cephv1.CephCluster, log bool) bool {
	cephClusterList := &cephv1.CephClusterList{}
	listOpts := []client.ListOption{
		client.InNamespace(object.GetNamespace()),
	}
	err := c.List(ctx, cephClusterList, listOpts...)
	if err != nil {
		logger.Errorf("failed to list ceph clusters, assuming there is none, not reconciling. %v", err)
		return true
	}

	logger.Debugf("found %d ceph clusters in namespace %q", len(cephClusterList.Items), object.GetNamespace())

	// This check is needed when the operator is down and a cluster was created
	if len(cephClusterList.Items) > 1 {
		// Since multiple predicate are using this function we don't want all of them to log the
		// same message, so one predicate can log and the other cannot
		if log {
			logger.Errorf("found more than one ceph cluster in namespace %q. not reconciling. only one ceph cluster per namespace.", object.GetNamespace())
			for _, cluster := range cephClusterList.Items {
				logger.Errorf("found ceph cluster %q in namespace %q", cluster.Name, cluster.Namespace)
			}
		}
		return true
	}

	return false
}

func GetSpec(obj client.Object) interface{} {
	val := reflect.ValueOf(obj)

	// If obj is a pointer, get the element
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	spec := val.FieldByName("Spec")
	if !spec.IsValid() {
		logger.Warningf("No Spec field found for object %+v. This should not happen.", obj)
		return nil
	}

	return spec.Interface()
}

func WatchPeerTokenSecretPredicate[T *corev1.Secret]() predicate.TypedFuncs[T] {
	return predicate.TypedFuncs[T]{
		CreateFunc: func(e event.TypedCreateEvent[T]) bool {
			newSecret := (*corev1.Secret)(e.Object)
			// reconcile when secret is created
			if strings.Contains(newSecret.GetName(), clusterMirrorBootstrapPeerSecretName) {
				logger.Debugf("peer token create event for secret %q in the namespace %q", newSecret.GetName(), newSecret.GetNamespace())
				return true
			}
			return false
		},
		UpdateFunc: func(e event.TypedUpdateEvent[T]) bool {
			newSecret := (*corev1.Secret)(e.ObjectNew)
			oldSecret := (*corev1.Secret)(e.ObjectOld)
			if !strings.Contains(newSecret.GetName(), clusterMirrorBootstrapPeerSecretName) {
				return false
			}
			// reconcile if the peer token data has changed
			newData := newSecret.Data["token"]
			oldData := oldSecret.Data["token"]
			if string(newData) != string(oldData) {
				logger.Debugf("peer token update event for secret %q in the namespace %q", newSecret.GetName(), newSecret.GetNamespace())
				return true
			}
			return false
		},
		DeleteFunc: func(e event.TypedDeleteEvent[T]) bool {
			// Do not reconcile when secret is deleted
			return false
		},
	}
}
