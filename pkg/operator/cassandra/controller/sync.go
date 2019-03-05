/*
Copyright 2018 The Rook Authors. All rights reserved.

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

package controller

import (
	cassandrav1alpha1 "github.com/rook/rook/pkg/apis/cassandra.rook.io/v1alpha1"
	"github.com/rook/rook/pkg/operator/cassandra/controller/util"
	corev1 "k8s.io/api/core/v1"
)

const (
	// SuccessSynced is used as part of the Event 'reason' when a Cluster is
	// synced.
	SuccessSynced = "Synced"
	// ErrSyncFailed is used as part of the Event 'reason' when a
	// Cluster fails to sync due to a resource of the same name already
	// existing.
	ErrSyncFailed = "ErrSyncFailed"

	MessageRackCreated             = "Rack %s created"
	MessageRackScaledUp            = "Rack %s scaled up to %d members"
	MessageRackScaleDownInProgress = "Rack %s scaling down to %d members"
	MessageRackScaledDown          = "Rack %s scaled down to %d members"

	// Messages to display when experiencing an error.
	MessageHeadlessServiceSyncFailed = "Failed to sync Headless Service for cluster"
	MessageMemberServicesSyncFailed  = "Failed to sync MemberServices for cluster"
	MessageUpdateStatusFailed        = "Failed to update status for cluster"
	MessageCleanupFailed             = "Failed to clean up cluster resources"
	MessageClusterSyncFailed         = "Failed to sync cluster"
)

// Sync attempts to sync the given Cassandra Cluster.
// NOTE: the Cluster Object is a DeepCopy. Modify at will.
func (cc *ClusterController) Sync(c *cassandrav1alpha1.Cluster) error {

	// Before syncing, ensure that all StatefulSets are up-to-date
	stale, err := util.StatefulSetStatusesStale(c, cc.statefulSetLister)
	if err != nil {
		return err
	}
	if stale {
		return nil
	}

	// Cleanup Cluster resources
	if err := cc.cleanup(c); err != nil {
		cc.recorder.Event(
			c,
			corev1.EventTypeWarning,
			ErrSyncFailed,
			MessageCleanupFailed,
		)
	}

	// Sync Headless Service for Cluster
	if err := cc.syncClusterHeadlessService(c); err != nil {
		cc.recorder.Event(
			c,
			corev1.EventTypeWarning,
			ErrSyncFailed,
			MessageHeadlessServiceSyncFailed,
		)
		return err
	}

	// Sync Cluster Member Services
	if err := cc.syncMemberServices(c); err != nil {
		cc.recorder.Event(
			c,
			corev1.EventTypeWarning,
			ErrSyncFailed,
			MessageMemberServicesSyncFailed,
		)
		return err
	}

	// Update Status
	if err := cc.updateStatus(c); err != nil {
		cc.recorder.Event(
			c,
			corev1.EventTypeWarning,
			ErrSyncFailed,
			MessageUpdateStatusFailed,
		)
		return err
	}

	// Sync Cluster
	if err := cc.syncCluster(c); err != nil {
		cc.recorder.Event(
			c,
			corev1.EventTypeWarning,
			ErrSyncFailed,
			MessageClusterSyncFailed,
		)
		return err
	}

	return nil
}
