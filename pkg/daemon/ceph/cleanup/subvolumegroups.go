/*
Copyright 2024 The Rook Authors. All rights reserved.

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

package cleanup

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
)

func SubVolumeGroupCleanup(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo, fsName, svg, poolName, csiNamespace string) error {
	logger.Infof("starting clean up cephFS subVolumeGroup resource %q", svg)

	subVolumeList, err := cephclient.ListSubvolumesInGroup(context, clusterInfo, fsName, svg)
	if err != nil {
		return errors.Wrapf(err, "failed to list cephFS subVolumes in subVolumeGroup %q", svg)
	}

	if len(subVolumeList) == 0 {
		logger.Infof("no subvolumes found in cephFS subVolumeGroup %q", svg)
		return nil
	}

	var retErr error
	for _, subVolume := range subVolumeList {
		logger.Infof("starting clean up of subvolume %q", subVolume.Name)
		err := CleanUpOMAPDetails(context, clusterInfo, subVolume.Name, poolName, csiNamespace)
		if err != nil {
			retErr = errors.Wrapf(err, "failed to clean up OMAP details for the subvolume %q.", subVolume.Name)
			logger.Error(retErr)
		}
		subVolumeSnapshots, err := cephclient.ListSubVolumeSnapshots(context, clusterInfo, fsName, subVolume.Name, svg)
		if err != nil {
			retErr = errors.Wrapf(err, "failed to list snapshots for subvolume %q in group %q.", subVolume.Name, svg)
			logger.Error(retErr)
		} else {
			err := CancelPendingClones(context, clusterInfo, subVolumeSnapshots, fsName, subVolume.Name, svg)
			if err != nil {
				retErr = errors.Wrapf(err, "failed to cancel pending clones for subvolume %q in group %q", subVolume.Name, svg)
				logger.Error(retErr)
			}
			err = DeleteSubVolumeSnapshots(context, clusterInfo, subVolumeSnapshots, fsName, subVolume.Name, svg)
			if err != nil {
				retErr = errors.Wrapf(err, "failed to delete snapshots for subvolume %q in group %q", subVolume.Name, svg)
				logger.Error(retErr)
			}
		}
		err = cephclient.DeleteSubVolume(context, clusterInfo, fsName, subVolume.Name, svg)
		if err != nil {
			retErr = errors.Wrapf(err, "failed to delete subvolume group %q.", subVolume.Name)
			logger.Error(retErr)
		}
	}

	if retErr != nil {
		return errors.Wrapf(retErr, "clean up for cephFS subVolumeGroup %q didn't complete successfully.", svg)
	}
	logger.Infof("successfully cleaned up cephFS subVolumeGroup %q", svg)
	return nil
}

func CancelPendingClones(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo, snapshots cephclient.SubVolumeSnapshots, fsName, subvol, svg string) error {
	for _, snapshot := range snapshots {
		logger.Infof("deleting any pending clones of snapshot %q of subvolume %q of group %q", snapshot.Name, subvol, svg)
		pendingClones, err := cephclient.ListSubVolumeSnapshotPendingClones(context, clusterInfo, fsName, subvol, snapshot.Name, svg)
		if err != nil {
			return errors.Wrapf(err, "failed to list all the pending clones for snapshot %q", snapshot.Name)
		}
		for _, pendingClone := range pendingClones.Clones {
			err := cephclient.CancelSnapshotClone(context, clusterInfo, fsName, svg, pendingClone.Name)
			if err != nil {
				return errors.Wrapf(err, "failed to cancel the pending clone %q for snapshot %q", pendingClone.Name, snapshot.Name)
			}
		}
	}
	return nil
}

func DeleteSubVolumeSnapshots(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo, snapshots cephclient.SubVolumeSnapshots, fsName, subvol, svg string) error {
	for _, snapshot := range snapshots {
		logger.Infof("deleting snapshot %q for subvolume %q in group %q", snapshot.Name, subvol, svg)
		err := cephclient.DeleteSubvolumeSnapshot(context, clusterInfo, fsName, subvol, svg, snapshot.Name)
		if err != nil {
			return errors.Wrapf(err, "failed to delete snapshot %q for subvolume %q in group %q. %v", snapshot.Name, subvol, svg, err)
		}
		logger.Infof("successfully deleted snapshot %q for subvolume %q in group %q", snapshot.Name, subvol, svg)
	}
	return nil
}

func CleanUpOMAPDetails(context *clusterd.Context, clusterInfo *cephclient.ClusterInfo, objName, poolName, namespace string) error {
	omapValue := getOMAPValue(objName)
	if omapValue == "" {
		return errors.New(fmt.Sprintf("failed to get OMAP value for object %q", objName))
	}
	logger.Infof("OMAP value for the object %q is %q", objName, omapValue)
	omapKey, err := cephclient.GetOMAPKey(context, clusterInfo, omapValue, poolName, namespace)
	if err != nil {
		return errors.Wrapf(err, "failed to get OMAP key for omapObj %q. %v", omapValue, err)
	}
	logger.Infof("OMAP key for the OIMAP value %q is %q", omapValue, omapKey)

	// delete OMAP details
	err = cephclient.DeleteOmapValue(context, clusterInfo, omapValue, poolName, namespace)
	if err != nil {
		return errors.Wrapf(err, "failed to delete OMAP value %q. %v", omapValue, err)
	}
	if omapKey != "" {
		err = cephclient.DeleteOmapKey(context, clusterInfo, omapKey, poolName, namespace)
		if err != nil {
			return errors.Wrapf(err, "failed to delete OMAP key %q. %v", omapKey, err)
		}
	}
	return nil
}

func getOMAPValue(subVol string) string {
	splitSubvol := strings.SplitAfterN(subVol, "-", 3)
	if len(splitSubvol) < 3 {
		return ""
	}
	subvol_id := splitSubvol[len(splitSubvol)-1]
	omapval := "csi.volume." + subvol_id
	return omapval
}
