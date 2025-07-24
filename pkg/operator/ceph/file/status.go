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

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/ceph/reporting"
	"github.com/rook/rook/pkg/operator/k8sutil"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
)

// updateStatus updates a fs CR with the given status
func (r *ReconcileCephFilesystem) updateStatus(observedGeneration int64, namespacedName types.NamespacedName, status cephv1.ConditionType, info map[string]string, cephx *cephv1.CephxStatus) (*cephv1.CephFilesystem, error) {
	fs := &cephv1.CephFilesystem{}
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		err := r.client.Get(r.opManagerContext, namespacedName, fs)
		if err != nil {
			if kerrors.IsNotFound(err) {
				logger.Debugf("CephFilesystem resource %q not found. Ignoring since object must be deleted.", namespacedName.String())
				return nil
			}
			return errors.Wrapf(err, "failed to retrieve filesystem %q to update status to %q.", namespacedName.String(), status)
		}

		if fs.Status == nil {
			fs.Status = &cephv1.CephFilesystemStatus{}
		}

		fs.Status.Phase = status
		fs.Status.Info = info
		if observedGeneration != k8sutil.ObservedGenerationNotAvailable {
			fs.Status.ObservedGeneration = observedGeneration
		}

		if cephx != nil {
			fs.Status.Cephx.Daemon = *cephx
		}

		if err := reporting.UpdateStatus(r.client, fs); err != nil {
			return errors.Wrapf(err, "failed to set filesystem %q status to %q.", namespacedName.String(), status)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	logger.Debugf("filesystem %q status updated to %q", namespacedName.String(), status)
	return fs, nil
}

// updateStatusBucket updates an object with a given status
func (c *mirrorChecker) updateStatusMirroring(mirrorStatus []cephv1.FilesystemMirroringInfo, snapSchedStatus []cephv1.FilesystemSnapshotSchedulesSpec, details string) {
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
	fs.Status = toCustomResourceStatus(fs.Status, mirrorStatus, snapSchedStatus, details)
	if err := reporting.UpdateStatus(c.client, fs); err != nil {
		logger.Errorf("failed to set ceph filesystem %q mirroring status. %v", c.namespacedName.Name, err)
		return
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
