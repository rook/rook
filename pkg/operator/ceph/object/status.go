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
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func (r *ReconcileCephObjectStore) setFailedStatus(name types.NamespacedName, errMessage string, err error) (reconcile.Result, error) {
	updateStatus(r.client, name, cephv1.ConditionFailure, map[string]string{})
	return reconcile.Result{}, errors.Wrapf(err, "%s", errMessage)
}

// updateStatus updates an object with a given status
func updateStatus(client client.Client, namespacedName types.NamespacedName, status cephv1.ConditionType, info map[string]string) {
	objectStore := &cephv1.CephObjectStore{}
	if err := client.Get(context.TODO(), namespacedName, objectStore); err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("CephObjectStore resource not found. Ignoring since object must be deleted.")
			return
		}
		logger.Warningf("failed to retrieve object store %q to update status to %q. %v", namespacedName, status, err)
		return
	}
	if objectStore.Status == nil {
		objectStore.Status = &cephv1.ObjectStoreStatus{}
	}

	objectStore.Status.Phase = status
	objectStore.Status.Info = info

	if err := opcontroller.UpdateStatus(client, objectStore); err != nil {
		logger.Errorf("failed to set object store %q status to %q. %v", namespacedName, status, err)
		return
	}
	logger.Debugf("object store %q status updated to %q", namespacedName, status)
}

// updateStatusBucket updates an object with a given status
func updateStatusBucket(client client.Client, name types.NamespacedName, phase cephv1.ConditionType, details string) {
	objectStore := &cephv1.CephObjectStore{}
	if err := client.Get(context.TODO(), name, objectStore); err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("CephObjectStore resource not found. Ignoring since object must be deleted.")
			return
		}
		logger.Warningf("failed to retrieve object store %q to update status to %v. %v", name, phase, err)
		return
	}
	if objectStore.Status == nil {
		objectStore.Status = &cephv1.ObjectStoreStatus{}
	}
	objectStore.Status.BucketStatus = toCustomResourceStatus(objectStore.Status.BucketStatus, details, phase)
	objectStore.Status.Phase = phase
	if err := opcontroller.UpdateStatus(client, objectStore); err != nil {
		logger.Errorf("failed to set object store %q status to %v. %v", name, phase, err)
		return
	}

	logger.Debugf("object store %q status updated to %v", name, phase)
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
