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
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// updateStatus updates a fs CR with the given status
func (r *ReconcileCephFilesystem) updateStatus(client client.Client, namespacedName types.NamespacedName, status cephv1.ConditionType, info map[string]string) {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		fs := &cephv1.CephFilesystem{}
		err := client.Get(r.opManagerContext, namespacedName, fs)
		if err != nil {
			if kerrors.IsNotFound(err) {
				logger.Debug("CephFilesystem resource not found. Ignoring since object must be deleted.")
				return nil
			}
			logger.Warningf("failed to retrieve filesystem %q to update status to %q. %v", namespacedName, status, err)
			return err
		}

		if fs.Status == nil {
			fs.Status = &cephv1.CephFilesystemStatus{}
		}

		fs.Status.Phase = status
		fs.Status.Info = info
		return client.Status().Update(r.opManagerContext, fs)
	})
	if err != nil {
		logger.Errorf("failed to set ceph filesystem %q status to %q. %v", namespacedName.Name, status, err)
	}

	logger.Debugf("filesystem %q status updated to %q", namespacedName.Name, status)
}

// updateStatusBucket updates an object with a given status
func (c *mirrorChecker) updateStatusMirroring(mirrorStatus []cephv1.FilesystemMirroringInfo, snapSchedStatus []cephv1.FilesystemSnapshotSchedulesSpec, details string) {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		fs := &cephv1.CephFilesystem{}
		if err := c.client.Get(c.clusterInfo.Context, c.namespacedName, fs); err != nil {
			if kerrors.IsNotFound(err) {
				logger.Debug("CephFilesystem resource not found. Ignoring since object must be deleted.")
				return nil
			}
			logger.Warningf("failed to retrieve ceph filesystem %q to update mirroring status. %v", c.namespacedName.Name, err)
			return err
		}
		if fs.Status == nil {
			fs.Status = &cephv1.CephFilesystemStatus{}
		}

		// Update the CephFilesystem CR status field
		fs.Status = toCustomResourceStatus(fs.Status, mirrorStatus, snapSchedStatus, details)

		return c.client.Status().Update(c.clusterInfo.Context, fs)
	})
	if err != nil {
		logger.Errorf("failed to update status for ceph filesystem mirror %q. %v", c.namespacedName.Name, err)
	}

	logger.Debugf("ceph filesystem %q mirroring status updated", c.namespacedName.Name)
}

func toCustomResourceStatus(currentStatus *cephv1.CephFilesystemStatus, mirrorStatus []cephv1.FilesystemMirroringInfo, snapSchedStatus []cephv1.FilesystemSnapshotSchedulesSpec, details string) *cephv1.CephFilesystemStatus {
	mirrorStatusSpec := &cephv1.FilesystemMirroringInfoSpec{}
	mirrorSnapScheduleStatusSpec := &cephv1.FilesystemSnapshotScheduleStatusSpec{}
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
	}

	// SNAP SCHEDULE
	if len(snapSchedStatus) != 0 {
		mirrorSnapScheduleStatusSpec.LastChecked = now
		mirrorSnapScheduleStatusSpec.SnapshotSchedules = snapSchedStatus
	}
	// Always display the details, typically an error
	mirrorSnapScheduleStatusSpec.Details = details

	return &cephv1.CephFilesystemStatus{MirroringStatus: mirrorStatusSpec, SnapshotScheduleStatus: mirrorSnapScheduleStatusSpec, Phase: currentStatus.Phase, Info: currentStatus.Info}
}
