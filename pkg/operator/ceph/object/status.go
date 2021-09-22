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

package object

import (
	"context"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/ceph/reporting"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func (r *ReconcileCephObjectStore) setFailedStatus(name types.NamespacedName, errMessage string, err error) (reconcile.Result, error) {
	updateStatus(r.client, name, cephv1.ConditionFailure, map[string]string{})
	return reconcile.Result{}, errors.Wrapf(err, "%s", errMessage)
}

// updateStatus updates an object with a given status
func updateStatus(client client.Client, namespacedName types.NamespacedName, status cephv1.ConditionType, info map[string]string) {
	// Updating the status is important to users, but we can still keep operating if there is a
	// failure. Retry a few times to give it our best effort attempt.
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		objectStore := &cephv1.CephObjectStore{}
		if err := client.Get(context.TODO(), namespacedName, objectStore); err != nil {
			if kerrors.IsNotFound(err) {
				logger.Debug("CephObjectStore resource not found. Ignoring since object must be deleted.")
				return nil
			}
			return errors.Wrapf(err, "failed to retrieve object store %q to update status to %q", namespacedName.String(), status)
		}
		if objectStore.Status == nil {
			objectStore.Status = &cephv1.ObjectStoreStatus{}
		}

		if objectStore.Status.Phase == cephv1.ConditionDeleting {
			logger.Debugf("object store %q status not updated to %q because it is deleting", namespacedName.String(), status)
			return nil // do not transition to other statuses once deletion begins
		}

		objectStore.Status.Phase = status
		objectStore.Status.Info = info

		if err := reporting.UpdateStatus(client, objectStore); err != nil {
			return errors.Wrapf(err, "failed to set object store %q status to %q", namespacedName.String(), status)
		}
		return nil
	})
	if err != nil {
		logger.Error(err)
	}

	logger.Debugf("object store %q status updated to %q", namespacedName.String(), status)
}

// updateStatusBucket updates an object with a given status
func updateStatusBucket(client client.Client, name types.NamespacedName, status cephv1.ConditionType, details string) {
	// Updating the status is important to users, but we can still keep operating if there is a
	// failure. Retry a few times to give it our best effort attempt.
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		objectStore := &cephv1.CephObjectStore{}
		if err := client.Get(context.TODO(), name, objectStore); err != nil {
			if kerrors.IsNotFound(err) {
				logger.Debug("CephObjectStore resource not found. Ignoring since object must be deleted.")
				return nil
			}
			return errors.Wrapf(err, "failed to retrieve object store %q to update status to %v", name.String(), status)
		}
		if objectStore.Status == nil {
			objectStore.Status = &cephv1.ObjectStoreStatus{}
		}
		objectStore.Status.BucketStatus = toCustomResourceStatus(objectStore.Status.BucketStatus, details, status)

		// do not transition to other statuses once deletion begins
		if objectStore.Status.Phase != cephv1.ConditionDeleting {
			objectStore.Status.Phase = status
		}

		// but we still need to update the health checker status
		if err := reporting.UpdateStatus(client, objectStore); err != nil {
			return errors.Wrapf(err, "failed to set object store %q status to %v", name.String(), status)
		}
		return nil
	})
	if err != nil {
		logger.Error(err)
	}

	logger.Debugf("object store %q status updated to %v", name.String(), status)
}

func buildStatusInfo(cephObjectStore *cephv1.CephObjectStore) map[string]string {
	m := make(map[string]string)

	if cephObjectStore.Spec.Gateway.SecurePort != 0 && cephObjectStore.Spec.Gateway.Port != 0 {
		m["secureEndpoint"] = BuildDNSEndpoint(BuildDomainName(cephObjectStore.Name, cephObjectStore.Namespace), cephObjectStore.Spec.Gateway.SecurePort, true)
		m["endpoint"] = BuildDNSEndpoint(BuildDomainName(cephObjectStore.Name, cephObjectStore.Namespace), cephObjectStore.Spec.Gateway.Port, false)
	} else if cephObjectStore.Spec.Gateway.SecurePort != 0 {
		m["endpoint"] = BuildDNSEndpoint(BuildDomainName(cephObjectStore.Name, cephObjectStore.Namespace), cephObjectStore.Spec.Gateway.SecurePort, true)
	} else {
		m["endpoint"] = BuildDNSEndpoint(BuildDomainName(cephObjectStore.Name, cephObjectStore.Namespace), cephObjectStore.Spec.Gateway.Port, false)
	}

	return m
}
