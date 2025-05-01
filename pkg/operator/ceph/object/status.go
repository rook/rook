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
	"fmt"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/ceph/reporting"
	"github.com/rook/rook/pkg/operator/k8sutil"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func (r *ReconcileCephObjectStore) setFailedStatus(observedGeneration int64, name types.NamespacedName, errMessage string, err error) (reconcile.Result, error) {
	updateStatus(r.opManagerContext, observedGeneration, r.client, name, cephv1.ConditionFailure, map[string]string{}, nil)
	return reconcile.Result{}, errors.Wrapf(err, "%s", errMessage)
}

// updateStatus updates an object with a given status
func updateStatus(ctx context.Context, observedGeneration int64, client client.Client, namespacedName types.NamespacedName, status cephv1.ConditionType, info map[string]string, cephx *cephv1.CephxStatus) {
	// Updating the status is important to users, but we can still keep operating if there is a
	// failure. Retry a few times to give it our best effort attempt.
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		objectStore := &cephv1.CephObjectStore{}
		if err := client.Get(ctx, namespacedName, objectStore); err != nil {
			if kerrors.IsNotFound(err) {
				logger.Debug("CephObjectStore resource not found. Ignoring since object must be deleted.")
				return nil
			}
			return errors.Wrapf(err, "failed to retrieve object store %q to update status to %q", namespacedName.String(), status)
		}
		if objectStore.Status == nil {
			objectStore.Status = &cephv1.ObjectStoreStatus{
				Endpoints: cephv1.ObjectEndpoints{
					Insecure: []string{},
					Secure:   []string{},
				},
			}
		}

		if objectStore.Status.Phase == cephv1.ConditionDeleting {
			logger.Debugf("object store %q status not updated to %q because it is deleting", namespacedName.String(), status)
			return nil // do not transition to other statuses once deletion begins
		}

		objectStore.Status.Phase = status
		objectStore.Status.Info = info
		if observedGeneration != k8sutil.ObservedGenerationNotAvailable {
			objectStore.Status.ObservedGeneration = observedGeneration
		}

		insecurePort := objectStore.Spec.Gateway.Port
		if insecurePort > 0 {
			objectStore.Status.Endpoints.Insecure = getAllDNSEndpoints(objectStore, insecurePort, false)
		}
		securePort := objectStore.Spec.Gateway.SecurePort
		if securePort > 0 {
			objectStore.Status.Endpoints.Secure = getAllDNSEndpoints(objectStore, securePort, true)
		}

		if cephx != nil {
			objectStore.Status.Cephx.Daemon = *cephx
		}

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

func buildStatusInfo(cephObjectStore *cephv1.CephObjectStore) map[string]string {
	nsName := fmt.Sprintf("%s/%s", cephObjectStore.Namespace, cephObjectStore.Name)

	m := make(map[string]string)

	advertiseEndpoint, err := cephObjectStore.GetAdvertiseEndpointUrl()
	if err != nil {
		// lots of validation happens before this point, so this should be nearly impossible
		logger.Errorf("failed to get advertise endpoint for CephObjectStore %q to record on status; continuing without this. %v", nsName, err)
	}

	if cephObjectStore.AdvertiseEndpointIsSet() {
		// if the advertise endpoint is explicitly set, it takes precedence as the only endpoint
		m["endpoint"] = advertiseEndpoint
		return m
	}

	if cephObjectStore.Spec.Gateway.Port != 0 && cephObjectStore.Spec.Gateway.SecurePort != 0 {
		// by definition, advertiseEndpoint should prefer HTTPS, so the inverse arrangement doesn't apply
		m["secureEndpoint"] = advertiseEndpoint
		m["endpoint"] = BuildDNSEndpoint(GetStableDomainName(cephObjectStore), cephObjectStore.Spec.Gateway.Port, false)
	} else {
		m["endpoint"] = advertiseEndpoint
	}

	return m
}
