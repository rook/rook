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

package client

import (
	"context"
	"time"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/ceph/reporting"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var defaultHealthCheckInterval = 1 * time.Minute

type mirrorChecker struct {
	context        *clusterd.Context
	interval       *time.Duration
	client         client.Client
	clusterInfo    *ClusterInfo
	namespacedName types.NamespacedName
	monitoringSpec *cephv1.NamedPoolSpec
	objectType     client.Object
}

// newMirrorChecker creates a new HealthChecker object
func NewMirrorChecker(context *clusterd.Context, client client.Client, clusterInfo *ClusterInfo, namespacedName types.NamespacedName, monitoringSpec *cephv1.NamedPoolSpec, object client.Object) *mirrorChecker {
	c := &mirrorChecker{
		context:        context,
		interval:       &defaultHealthCheckInterval,
		clusterInfo:    clusterInfo,
		namespacedName: namespacedName,
		client:         client,
		monitoringSpec: monitoringSpec,
		objectType:     object,
	}

	// allow overriding the check interval
	checkInterval := monitoringSpec.StatusCheck.Mirror.Interval
	if checkInterval != nil {
		logger.Infof("mirroring status check interval for %q is %q", namespacedName.Name, checkInterval.Duration.String())
		c.interval = &checkInterval.Duration
	}

	return c
}

// checkMirroring periodically checks the health of the cluster
func (c *mirrorChecker) CheckMirroring(context context.Context) {
	// check the mirroring health immediately before starting the loop
	err := c.CheckMirroringHealth()
	if err != nil {
		c.UpdateStatusMirroring(nil, nil, nil, err.Error())
		logger.Debugf("failed to check mirroring status for %q. %v", c.namespacedName.Name, err)
	}

	for {
		select {
		case <-context.Done():
			logger.Infof("stopping monitoring mirroring status for %q", c.namespacedName.Name)
			return

		case <-time.After(*c.interval):
			logger.Debugf("checking mirroring status for %q", c.namespacedName.Name)
			err := c.CheckMirroringHealth()
			if err != nil {
				c.UpdateStatusMirroring(nil, nil, nil, err.Error())
				logger.Debugf("failed to check mirroring status for %q. %v", c.namespacedName.Name, err)
			}
		}
	}
}

func (c *mirrorChecker) CheckMirroringHealth() error {
	// Check mirroring status
	mirrorStatus, err := GetPoolMirroringStatus(c.context, c.clusterInfo, c.monitoringSpec.Name)
	if err != nil {
		c.UpdateStatusMirroring(nil, nil, nil, err.Error())
	}

	// Check mirroring info
	mirrorInfo, err := GetPoolMirroringInfo(c.context, c.clusterInfo, c.monitoringSpec.Name)
	if err != nil {
		c.UpdateStatusMirroring(nil, nil, nil, err.Error())
	}

	// If snapshot scheduling is enabled let's add it to the status
	// snapSchedStatus := cephclient.SnapshotScheduleStatus{}
	snapSchedStatus := []cephv1.SnapshotSchedulesSpec{}
	if c.monitoringSpec.Mirroring.SnapshotSchedulesEnabled() {
		snapSchedStatus, err = ListSnapshotSchedulesRecursively(c.context, c.clusterInfo, c.monitoringSpec.Name)
		if err != nil {
			c.UpdateStatusMirroring(nil, nil, nil, err.Error())
		}
	}

	// On success
	if mirrorStatus != nil {
		c.UpdateStatusMirroring(mirrorStatus.Summary, mirrorInfo, snapSchedStatus, "")
	}
	return nil
}

// updateStatusBucket updates an object with a given status
func (c *mirrorChecker) UpdateStatusMirroring(mirrorStatus *cephv1.MirroringStatusSummarySpec, mirrorInfo *cephv1.MirroringInfo, snapSchedStatus []cephv1.SnapshotSchedulesSpec, details string) {
	switch c.objectType.(type) {
	case *cephv1.CephBlockPool:
		updatePoolStatusMirroring(c, mirrorStatus, mirrorInfo, snapSchedStatus, details)

	case *cephv1.CephBlockPoolRadosNamespace:
		updateRadosNamespaceStatusMirroring(c, mirrorStatus, mirrorInfo, snapSchedStatus, details)
	}
}

func updatePoolStatusMirroring(c *mirrorChecker, mirrorStatus *cephv1.MirroringStatusSummarySpec, mirrorInfo *cephv1.MirroringInfo, snapSchedStatus []cephv1.SnapshotSchedulesSpec, details string) {
	blockPool := &cephv1.CephBlockPool{}
	if err := c.client.Get(c.clusterInfo.Context, c.namespacedName, blockPool); err != nil {
		if kerrors.IsNotFound(err) {
			logger.Debug("CephBlockPool %q resource not found for updating the mirroring status, ignoring.", c.namespacedName)
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

func updateRadosNamespaceStatusMirroring(c *mirrorChecker, mirrorStatus *cephv1.MirroringStatusSummarySpec, mirrorInfo *cephv1.MirroringInfo, snapSchedStatus []cephv1.SnapshotSchedulesSpec, details string) {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		radosNamespace := &cephv1.CephBlockPoolRadosNamespace{}
		if err := c.client.Get(c.clusterInfo.Context, c.namespacedName, radosNamespace); err != nil {
			if kerrors.IsNotFound(err) {
				logger.Debug("CephBlockPoolRadosNamespace %q resource not found for updating the mirroring status, ignoring.", c.namespacedName)
				return nil
			}
			return err
		}
		if radosNamespace.Status == nil {
			radosNamespace.Status = &cephv1.CephBlockPoolRadosNamespaceStatus{}
		}

		blockPool := &cephv1.CephBlockPool{}
		namespaceName := types.NamespacedName{Name: radosNamespace.Spec.BlockPoolName, Namespace: radosNamespace.Namespace}
		if err := c.client.Get(c.clusterInfo.Context, namespaceName, blockPool); err != nil {
			if kerrors.IsNotFound(err) {
				logger.Debug("CephBlockPool %q resource not found for updating the mirroring status, ignoring.", namespaceName)
				return nil
			}
			return err
		}

		if blockPool.Spec.StatusCheck.Mirror.Disabled {
			logger.Debugf("mirroring status check is disabled for %q", c.namespacedName.Name)
			mirrorStatus = nil
			mirrorInfo = nil
			snapSchedStatus = nil
			details = ""
		}

		// Update the CephBlockPoolRadosNamespace CR status field
		radosNamespace.Status.MirroringStatus, radosNamespace.Status.MirroringInfo, radosNamespace.Status.SnapshotScheduleStatus = toCustomResourceStatus(radosNamespace.Status.MirroringStatus, mirrorStatus, radosNamespace.Status.MirroringInfo, mirrorInfo, radosNamespace.Status.SnapshotScheduleStatus, snapSchedStatus, details)
		if err := reporting.UpdateStatus(c.client, radosNamespace); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		logger.Errorf("failed to set ceph block pool rados namespace %q mirroring status after retries. %v", c.namespacedName.Name, err)
		return
	}
}

func toCustomResourceStatus(currentStatus *cephv1.MirroringStatusSpec, mirroringStatus *cephv1.MirroringStatusSummarySpec,
	currentInfo *cephv1.MirroringInfoSpec, mirroringInfo *cephv1.MirroringInfo,
	currentSnapSchedStatus *cephv1.SnapshotScheduleStatusSpec, snapSchedStatus []cephv1.SnapshotSchedulesSpec,
	details string,
) (*cephv1.MirroringStatusSpec, *cephv1.MirroringInfoSpec, *cephv1.SnapshotScheduleStatusSpec) {
	mirroringStatusSpec := &cephv1.MirroringStatusSpec{}
	mirroringInfoSpec := &cephv1.MirroringInfoSpec{}
	snapshotScheduleStatusSpec := &cephv1.SnapshotScheduleStatusSpec{}

	// mirroringStatus will be nil in case of an error to fetch it
	if mirroringStatus != nil {
		mirroringStatusSpec.LastChecked = time.Now().UTC().Format(time.RFC3339)
		mirroringStatusSpec.MirroringStatus.Summary = mirroringStatus
	}

	// Always display the details, typically an error
	mirroringStatusSpec.Details = details

	if currentStatus != nil {
		mirroringStatusSpec.LastChanged = currentStatus.LastChanged
	}

	// mirroringInfo will be nil in case of an error to fetch it
	if mirroringInfo != nil {
		mirroringInfoSpec.LastChecked = time.Now().UTC().Format(time.RFC3339)
		mirroringInfoSpec.MirroringInfo = mirroringInfo
	}
	// Always display the details, typically an error
	mirroringInfoSpec.Details = details

	if currentInfo != nil {
		mirroringInfoSpec.LastChanged = currentInfo.LastChanged
	}

	// snapSchedStatus will be nil in case of an error to fetch it
	if len(snapSchedStatus) != 0 {
		snapshotScheduleStatusSpec.LastChecked = time.Now().UTC().Format(time.RFC3339)
		snapshotScheduleStatusSpec.SnapshotSchedules = snapSchedStatus
	}
	// Always display the details, typically an error
	snapshotScheduleStatusSpec.Details = details

	if currentSnapSchedStatus != nil {
		snapshotScheduleStatusSpec.LastChanged = currentSnapSchedStatus.LastChanged
	}

	return mirroringStatusSpec, mirroringInfoSpec, snapshotScheduleStatusSpec
}
