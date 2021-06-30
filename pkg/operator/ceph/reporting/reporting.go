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

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/dependents"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ReportReconcileResult will report the result of an object's reconcile in 2 ways:
// 1. to the given logger
// 2. as an event on the object (via the given event recorder)
// The results of the object's reconcile should include the object, the reconcile response, and the
// error returned by the reconcile.
// The function is designed to return the appropriate values needed for the controller-runtime
// framework's Reconcile() method.
func ReportReconcileResult(logger *capnslog.PackageLogger, recorder *k8sutil.EventReporter,
	obj client.Object, reconcileResponse reconcile.Result, err error,
) (reconcile.Result, error) {
	kind := obj.GetObjectKind().GroupVersionKind().Kind
	nsName := fmt.Sprintf("%s/%s", obj.GetNamespace(), obj.GetName())

	if err != nil {
		// 1. log
		logger.Errorf("failed to reconcile %s %q. %v", kind, nsName, err)

		// 2. event
		recorder.ReportIfNotPresent(obj, corev1.EventTypeWarning, string(cephv1.ReconcileFailed), err.Error())

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
		recorder.ReportIfNotPresent(obj, corev1.EventTypeNormal, string(cephv1.ReconcileSucceeded), successMsg)
	}

	return reconcileResponse, err
}

// ReportDeletionBlockedDueToDependents reports that deletion of a Rook-Ceph object is blocked due
// to the given dependents in 3 ways:
// 1. to the given logger
// 2. as a condition on the object (added to the object's conditions list given)
// 3. as the returned error which should be included in the FailedReconcile message
func ReportDeletionBlockedDueToDependents(
	logger *capnslog.PackageLogger, client client.Client, obj cephv1.StatusConditionGetter, deps *dependents.DependentList,
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
		if err := client.Get(context.TODO(), nsName, obj); err != nil {
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
	logger *capnslog.PackageLogger, client client.Client, recorder *k8sutil.EventReporter, obj cephv1.StatusConditionGetter,
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
	recorder.ReportIfNotPresent(obj, corev1.EventTypeNormal, string(cephv1.DeletingReason), deletingMsg)

	// 3. condition
	unblockedCond := dependents.DeletionBlockedDueToDependentsCondition(false, safeMsg)
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := client.Get(context.TODO(), nsName, obj); err != nil {
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
