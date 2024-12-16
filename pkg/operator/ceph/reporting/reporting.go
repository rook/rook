/*
Copyright 2021 The Rook Authors. All rights reserved.

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

// Reporting focuses on reporting Events, Status Conditions, and the like to users.
package reporting

import (
	"context"
	"fmt"
	"reflect"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/util/dependents"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const unknownKind = "<UnknownObjectKind>"

// Based on code from https://github.com/kubernetes/apimachinery/blob/master/pkg/api/meta/conditions.go

// A statusConditionGetter allows getting a pointer to an object's conditions.
type statusConditionGetter interface {
	client.Object

	// GetStatusConditions returns a pointer to the object's conditions compatible with
	// SetStatusCondition and FindStatusCondition.
	GetStatusConditions() *[]cephv1.Condition
}

// an object of a given type that has a nil reference is not the same as obj==nil (untyped nil)
// (e.g., var cluster cephv1.CephCluster = nil ), so we must also check for nil via reflection
func objIsNil(obj client.Object) bool {
	return obj == nil || reflect.ValueOf(obj).IsNil()
}

// get the kind through the object API, but if that is empty, make a best guess via golang reflection
func objKindOrBestGuess(obj client.Object) string {
	// can't get any type info from an untyped nil
	if obj == nil {
		return unknownKind
	}

	if !reflect.ValueOf(obj).IsNil() {
		kind := obj.GetObjectKind().GroupVersionKind().Kind
		if kind != "" {
			return kind
		}
	}

	// typed nil, or object's typemeta is empty
	t := reflect.TypeOf(obj)
	if t.Kind() == reflect.Ptr {
		return t.Elem().Name()
	}
	return t.Name()
}

func copyObject(obj client.Object) client.Object {
	if obj == nil {
		return nil // cannot copy nil object
	}

	if !reflect.ValueOf(obj).IsNil() {
		return obj.DeepCopyObject().(client.Object) // deep copy the object if it's non-nil
	}

	// Otherwise, it is a nil object, but it has a type. We can use reflection to make an empty
	// object to use as the copy in this case.
	var nonNilCopy client.Object
	t := reflect.TypeOf(obj)
	if t.Kind() == reflect.Ptr {
		innerType := t.Elem()
		newObj := reflect.New(innerType)
		nonNilCopy = newObj.Interface().(client.Object)
	} else {
		newObj := reflect.Zero(t)
		nonNilCopy = newObj.Interface().(client.Object)
	}

	return nonNilCopy
}

// ReportReconcileResult will report the result of an object's reconcile in 2 ways:
// 1. to the given logger
// 2. as an event on the object (via the given event recorder)
//
// The results of the object's reconcile should include the object (NEVER NIL),
// the reconcile response, and the error returned by the reconcile.
//
// The reconcile request is used to reference a given object that doesn't have a name.
//
// The function is designed to return the appropriate values needed for the controller-runtime
// framework's Reconcile() method.
func ReportReconcileResult(
	logger *capnslog.PackageLogger,
	recorder record.EventRecorder,
	reconcileRequest reconcile.Request,
	obj client.Object,
	reconcileResponse reconcile.Result,
	err error,
) (reconcile.Result, error) {
	kind := objKindOrBestGuess(obj)

	if objIsNil(obj) {
		logger.Errorf("object associated with reconcile request %s %q should not be nil", kind, reconcileRequest)
	}

	objCopy := copyObject(obj)

	// If object is empty, this may be because (a) the object was deleted and so the reconciler only
	// had an empty object, (b)) the api server didn't give the object, or (c)the reconciler
	// returned nil accidentally. The object needs full metadata in order to have an event
	// associated with it, but even with an empty object we can at least create an event that
	// references the namespaced name of the object, even if event won't show up in the output of
	// `kubectl describe object`.
	if objCopy != nil && objCopy.GetName() == "" {
		objCopy.SetName(reconcileRequest.Name)
		objCopy.SetNamespace(reconcileRequest.Namespace)
	}

	nsName := reconcileRequest.NamespacedName.String()

	if err != nil {
		errorMsg := fmt.Sprintf("failed to reconcile %s %q. %v", kind, nsName, err)

		// 1. log
		logger.Errorf("%s", errorMsg)

		// 2. event
		recorder.Event(objCopy, corev1.EventTypeWarning, string(cephv1.ReconcileFailed), errorMsg)

		if !reconcileResponse.IsZero() {
			// The framework will requeue immediately if there is an error. If we get an error with
			// a non-empty reconcile response, just return the response with the error now logged as
			// an event so that the framework can pause before the next reconcile per the response's
			// intent.
			return reconcileResponse, nil
		}
	} else {
		successMsg := fmt.Sprintf("successfully configured %s %q", kind, nsName)

		// 1. log
		logger.Debug(successMsg)

		// 2. event
		recorder.Event(objCopy, corev1.EventTypeNormal, string(cephv1.ReconcileSucceeded), successMsg)
	}

	return reconcileResponse, err
}

// ReportDeletionBlockedDueToDependents reports that deletion of a Rook-Ceph object is blocked due
// to the given dependents in 3 ways:
// 1. to the given logger
// 2. as a condition on the object (added to the object's conditions list given)
// 3. as the returned error which should be included in the FailedReconcile message
func ReportDeletionBlockedDueToDependents(
	ctx context.Context, logger *capnslog.PackageLogger, client client.Client, obj statusConditionGetter, deps *dependents.DependentList,
) error {
	kind := obj.GetObjectKind().GroupVersionKind().Kind
	nsName := types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}
	blockedMsg := deps.StringWithHeader("%s %q will not be deleted until all dependents are removed", kind, nsName.String())

	// 1. log
	logger.Info(blockedMsg)

	// 2. condition
	blockedCond := dependents.DeletionBlockedDueToDependentsCondition(true, blockedMsg)
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := client.Get(ctx, nsName, obj); err != nil {
			return errors.Wrapf(err, "failed to get latest %s %q", kind, nsName.String())
		}
		if err := UpdateStatusCondition(client, obj, blockedCond); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return errors.Wrapf(err, "on condition %s", blockedMsg)
	}

	// 3. error for later FailedReconcile message
	return errors.New(blockedMsg)
}

// ReportDeletionNotBlockedDueToDependents reports that deletion of a Rook-Ceph object is proceeding
// and NOT blocked due to dependents in 3 ways:
// 1. to the given logger
// 2. as an event on the object (via the given event recorder)
// 3. as a condition on the object (added to the object's conditions list given)
func ReportDeletionNotBlockedDueToDependents(
	ctx context.Context, logger *capnslog.PackageLogger, client client.Client, recorder record.EventRecorder, obj statusConditionGetter,
) {
	kind := obj.GetObjectKind().GroupVersionKind().Kind
	nsName := types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}
	safeMsg := fmt.Sprintf("%s %q can be deleted safely", kind, nsName.String())
	deletingMsg := fmt.Sprintf("deleting %s %q", kind, nsName.String())

	// 1. log
	logger.Infof("%s. %s", safeMsg, deletingMsg)

	// 2. event
	recorder.Event(obj, corev1.EventTypeNormal, string(cephv1.DeletingReason), deletingMsg)

	// 3. condition
	unblockedCond := dependents.DeletionBlockedDueToDependentsCondition(false, safeMsg)
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := client.Get(ctx, nsName, obj); err != nil {
			return errors.Wrapf(err, "failed to get latest %s %q", kind, nsName.String())
		}
		if err := UpdateStatusCondition(client, obj, unblockedCond); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		logger.Warningf("continuing deletion of %s %q without setting the condition. %v", kind, nsName.String(), err)
	}
}
