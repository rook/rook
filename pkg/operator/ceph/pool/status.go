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
	"time"

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

// updateStatusBucket updates an object with a given status
func (c *mirrorChecker) updateStatusMirroring(mirrorStatus *cephv1.PoolMirroringStatusSummarySpec, mirrorInfo *cephv1.PoolMirroringInfo, snapSchedStatus []cephv1.SnapshotSchedulesSpec, details string) {
	blockPool := &cephv1.CephBlockPool{}
	if err := c.client.Get(c.clusterInfo.Context, c.namespacedName, blockPool); err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("CephBlockPool resource not found. Ignoring since object must be deleted.")
			return
		}
		logger.Warningf("failed to retrieve ceph block pool %q to update mirroring status. %v", c.namespacedName.Name, err)
		return
	}
	if blockPool.Status == nil {
		blockPool.Status = &cephv1.CephBlockPoolStatus{}
	}

	// Update the CephBlockPool CR status field
	blockPool.Status.MirroringStatus, blockPool.Status.MirroringInfo, blockPool.Status.SnapshotScheduleStatus = toCustomResourceStatus(blockPool.Status.MirroringStatus, mirrorStatus, blockPool.Status.MirroringInfo, mirrorInfo, blockPool.Status.SnapshotScheduleStatus, snapSchedStatus, details)
	if err := reporting.UpdateStatus(c.client, blockPool); err != nil {
		logger.Errorf("failed to set ceph block pool %q mirroring status. %v", c.namespacedName.Name, err)
		return
	}

	logger.Debugf("ceph block pool %q mirroring status updated", c.namespacedName.Name)
}

func toCustomResourceStatus(currentStatus *cephv1.MirroringStatusSpec, mirroringStatus *cephv1.PoolMirroringStatusSummarySpec,
	currentInfo *cephv1.MirroringInfoSpec, mirroringInfo *cephv1.PoolMirroringInfo,
	currentSnapSchedStatus *cephv1.SnapshotScheduleStatusSpec, snapSchedStatus []cephv1.SnapshotSchedulesSpec,
	details string) (*cephv1.MirroringStatusSpec, *cephv1.MirroringInfoSpec, *cephv1.SnapshotScheduleStatusSpec) {
	mirroringStatusSpec := &cephv1.MirroringStatusSpec{}
	mirroringInfoSpec := &cephv1.MirroringInfoSpec{}
	snapshotScheduleStatusSpec := &cephv1.SnapshotScheduleStatusSpec{}

	// mirroringStatus will be nil in case of an error to fetch it
	if mirroringStatus != nil {
		mirroringStatusSpec.LastChecked = time.Now().UTC().Format(time.RFC3339)
		mirroringStatusSpec.Summary = mirroringStatus
	}

	// Always display the details, typically an error
	mirroringStatusSpec.Details = details

	if currentStatus != nil {
		mirroringStatusSpec.LastChanged = currentStatus.LastChanged
	}

	// mirroringInfo will be nil in case of an error to fetch it
	if mirroringInfo != nil {
		mirroringInfoSpec.LastChecked = time.Now().UTC().Format(time.RFC3339)
		mirroringInfoSpec.PoolMirroringInfo = mirroringInfo
	}
	// Always display the details, typically an error
	mirroringInfoSpec.Details = details

	if currentInfo != nil {
		mirroringInfoSpec.LastChanged = currentInfo.LastChecked
	}

	// snapSchedStatus will be nil in case of an error to fetch it
	if len(snapSchedStatus) != 0 {
		snapshotScheduleStatusSpec.LastChecked = time.Now().UTC().Format(time.RFC3339)
		snapshotScheduleStatusSpec.SnapshotSchedules = snapSchedStatus
	}
	// Always display the details, typically an error
	snapshotScheduleStatusSpec.Details = details

	if currentSnapSchedStatus != nil {
		snapshotScheduleStatusSpec.LastChanged = currentSnapSchedStatus.LastChecked
	}

	return mirroringStatusSpec, mirroringInfoSpec, snapshotScheduleStatusSpec
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
