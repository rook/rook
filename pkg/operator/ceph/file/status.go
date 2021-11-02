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

// Package file manages a CephFS filesystem and the required daemons.
package file

import (
	"time"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/ceph/reporting"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// updateStatus updates a fs CR with the given status
func (r *ReconcileCephFilesystem) updateStatus(client client.Client, namespacedName types.NamespacedName, status cephv1.ConditionType, info map[string]string) {
	fs := &cephv1.CephFilesystem{}
	err := client.Get(r.opManagerContext, namespacedName, fs)
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("CephFilesystem resource not found. Ignoring since object must be deleted.")
			return
		}
		logger.Warningf("failed to retrieve filesystem %q to update status to %q. %v", namespacedName, status, err)
		return
	}

	if fs.Status == nil {
		fs.Status = &cephv1.CephFilesystemStatus{}
	}

	fs.Status.Phase = status
	fs.Status.Info = info
	if err := reporting.UpdateStatus(client, fs); err != nil {
		logger.Warningf("failed to set filesystem %q status to %q. %v", fs.Name, status, err)
		return
	}
	logger.Debugf("filesystem %q status updated to %q", fs.Name, status)
}

// updateStatusMirroring updates an object with a given status
func (c *mirrorChecker) updateStatusMirroring(mirrorStatus []cephv1.FilesystemMirroringInfo,
	snapSchedStatus []cephv1.FilesystemSnapshotSchedulesSpec,
	perfStats *cephv1.FilesystemStats,
	details string,
) {
	fs := &cephv1.CephFilesystem{}
	if err := c.client.Get(c.clusterInfo.Context, c.namespacedName, fs); err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("CephFilesystem resource not found. Ignoring since object must be deleted.")
			return
		}
		logger.Warningf("failed to retrieve ceph filesystem %q to update mirroring status. %v", c.namespacedName.Name, err)
		return
	}
	if fs.Status == nil {
		fs.Status = &cephv1.CephFilesystemStatus{}
	}

	// Update the CephFilesystem CR status field
	fs.Status = toCustomResourceStatus(fs.Status, mirrorStatus, snapSchedStatus, perfStats, details)
	if err := reporting.UpdateStatus(c.client, fs); err != nil {
		logger.Errorf("failed to set ceph filesystem %q mirroring status. %v", c.namespacedName.Name, err)
		return
	}

	logger.Debugf("ceph filesystem %q mirroring status updated", c.namespacedName.Name)
}

func toCustomResourceStatus(currentStatus *cephv1.CephFilesystemStatus,
	mirrorStatus []cephv1.FilesystemMirroringInfo,
	snapSchedStatus []cephv1.FilesystemSnapshotSchedulesSpec,
	perfStats *cephv1.FilesystemStats,
	details string,
) *cephv1.CephFilesystemStatus {
	mirrorStatusSpec := &cephv1.FilesystemMirroringInfoSpec{}
	mirrorSnapScheduleStatusSpec := &cephv1.FilesystemSnapshotScheduleStatusSpec{}
	perfStatsSpec := &cephv1.FilesystemStatsSpec{}
	now := time.Now().UTC().Format(time.RFC3339)

	// MIRROR
	if len(mirrorStatus) != 0 {
		mirrorStatusSpec.LastChecked = now
		mirrorStatusSpec.FilesystemMirroringAllInfo = mirrorStatus
	}
	// Always display the details, typically an error
	mirrorStatusSpec.Details = details

	if currentStatus != nil {
		if currentStatus.MirroringStatus != nil {
			mirrorStatusSpec.LastChanged = currentStatus.MirroringStatus.LastChanged
		}
		if currentStatus.SnapshotScheduleStatus != nil {
			mirrorStatusSpec.LastChanged = currentStatus.SnapshotScheduleStatus.LastChanged
		}
		if currentStatus.PerfStats != nil {
			mirrorStatusSpec.LastChanged = currentStatus.PerfStats.LastChanged
		}
	}

	// SNAP SCHEDULE
	if len(snapSchedStatus) != 0 {
		mirrorSnapScheduleStatusSpec.LastChecked = now
		mirrorSnapScheduleStatusSpec.SnapshotSchedules = snapSchedStatus
	}
	// Always display the details, typically an error
	mirrorSnapScheduleStatusSpec.Details = details

	// Performance metrics
	if perfStats != nil {
		perfStatsSpec.LastChecked = now
		perfStatsSpec.FilesystemStats = perfStats
	}
	// Always display the details, typically an error
	perfStatsSpec.Details = details

	return &cephv1.CephFilesystemStatus{
		MirroringStatus:        mirrorStatusSpec,
		SnapshotScheduleStatus: mirrorSnapScheduleStatusSpec,
		PerfStats:              perfStatsSpec,
		Phase:                  currentStatus.Phase,
		Info:                   currentStatus.Info,
	}
}
