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
	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
	"github.com/rook/rook/pkg/operator/ceph/reporting"
	"github.com/rook/rook/pkg/operator/k8sutil"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

// updateStatus updates a pool CR with the given status
func (r *ReconcileCephBlockPool) updateStatus(poolName types.NamespacedName, status cephv1.ConditionType, observedGeneration int64) error {
	pool := &cephv1.CephBlockPool{}
	err := r.client.Get(r.opManagerContext, poolName, pool)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("CephBlockPool resource not found. Ignoring since object must be deleted.")
			return nil
		}
		logger.Warningf("failed to retrieve pool %q to update status to %q. %v", poolName, status, err)
		return errors.Wrapf(err, "failed to retrieve pool %q to update status to %q", poolName, status)
	}

	if pool.Status == nil {
		pool.Status = &cephv1.CephBlockPoolStatus{}
	}

	// add pool ID to the status
	if status == cephv1.ConditionReady && pool.Status.PoolID == 0 {
		r.updatePoolID(pool)
	}

	pool.Status.Phase = status
	updateStatusInfo(pool)
	if observedGeneration != k8sutil.ObservedGenerationNotAvailable {
		pool.Status.ObservedGeneration = observedGeneration
	}
	if err := reporting.UpdateStatus(r.client, pool); err != nil {
		logger.Warningf("failed to set pool %q status to %q. %v", pool.Name, status, err)
		return errors.Wrapf(err, "failed to set pool %q status to %q", pool.Name, status)
	}
	logger.Debugf("pool %q status updated to %q", poolName, status)
	return nil
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

func (r *ReconcileCephBlockPool) updatePoolID(cephBlockPool *cephv1.CephBlockPool) {
	poolName := cephBlockPool.ToNamedPoolSpec().Name
	poolDetails, err := cephclient.GetPoolDetails(r.context, r.clusterInfo, poolName)
	if err != nil {
		logger.Warningf("failed to get pool details for cephBlockPool %q", poolName)
		return
	}
	logger.Infof("set pool ID %d to cephBlockPool %q status", poolDetails.Number, poolName)
	cephBlockPool.Status.PoolID = poolDetails.Number
}
