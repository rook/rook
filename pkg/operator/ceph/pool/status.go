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

// Package pool to manage a rook pool.
package pool

import (
	"context"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/reporting"
	"github.com/rook/rook/pkg/operator/k8sutil"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// updateStatus updates a pool CR with the given status
func updateStatus(ctx context.Context, client client.Client, poolName types.NamespacedName, status cephv1.ConditionType, observedGeneration int64) {
	pool := &cephv1.CephBlockPool{}
	err := client.Get(ctx, poolName, pool)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("CephBlockPool resource not found. Ignoring since object must be deleted.")
			return
		}
		logger.Warningf("failed to retrieve pool %q to update status to %q. %v", poolName, status, err)
		return
	}

	if pool.Status == nil {
		pool.Status = &cephv1.CephBlockPoolStatus{}
	}

	pool.Status.Phase = status
	updateStatusInfo(pool)
	if observedGeneration != k8sutil.ObservedGenerationNotAvailable {
		pool.Status.ObservedGeneration = observedGeneration
	}
	if err := reporting.UpdateStatus(client, pool); err != nil {
		logger.Warningf("failed to set pool %q status to %q. %v", pool.Name, status, err)
		return
	}
	logger.Debugf("pool %q status updated to %q", poolName, status)
}

func updateStatusInfo(cephBlockPool *cephv1.CephBlockPool) {
	m := make(map[string]string)
	if cephBlockPool.Status.Phase == cephv1.ConditionReady && cephBlockPool.Spec.Mirroring.Enabled {
		mirroringInfo := opcontroller.GenerateStatusInfo(cephBlockPool)
		for key, value := range mirroringInfo {
			m[key] = value
		}
	}

	if cephBlockPool.Spec.IsReplicated() {
		m["type"] = "Replicated"
	} else {
		m["type"] = "Erasure Coded"
	}

	if cephBlockPool.Spec.FailureDomain != "" {
		m["failureDomain"] = cephBlockPool.Spec.FailureDomain
	} else {
		m["failureDomain"] = cephv1.DefaultFailureDomain
	}

	cephBlockPool.Status.Info = m
}
