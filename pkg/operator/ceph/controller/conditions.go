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

// Package config to provide conditions for CephCluster
package controller

import (
	"context"
	"time"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/ceph/reporting"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// UpdateCondition function will export each condition into the cluster custom resource
func UpdateCondition(c *clusterd.Context, namespaceName types.NamespacedName, conditionType cephv1.ConditionType, status v1.ConditionStatus, reason cephv1.ConditionReason, message string) {
	// use client.Client unit test this more easily with updating statuses which must use the client
	cluster := &cephv1.CephCluster{}
	if err := c.Client.Get(context.TODO(), namespaceName, cluster); err != nil {
		logger.Errorf("failed to get cluster %v to update the conditions. %v", namespaceName, err)
		return
	}

	UpdateClusterCondition(c, cluster, namespaceName, conditionType, status, reason, message, false)
}

// UpdateClusterCondition function will export each condition into the cluster custom resource
func UpdateClusterCondition(c *clusterd.Context, cluster *cephv1.CephCluster, namespaceName types.NamespacedName, conditionType cephv1.ConditionType, status v1.ConditionStatus,
	reason cephv1.ConditionReason, message string, preserveAllConditions bool) {

	// Keep the conditions that already existed if they are in the list of long-term conditions,
	// otherwise discard the temporary conditions
	var currentCondition *cephv1.Condition
	var conditions []cephv1.Condition
	for _, condition := range cluster.Status.Conditions {
		// Only keep conditions in the list if it's a persisted condition such as the cluster creation being completed.
		// The transient conditions are not persisted. However, if the currently requested condition is not expected to
		// reset the transient conditions, they are retained. For example, if the operator is checking for ceph health
		// in the middle of the reconcile, the progress condition should not be reset by the status check update.
		if preserveAllConditions ||
			condition.Reason == cephv1.ClusterCreatedReason ||
			condition.Reason == cephv1.ClusterConnectedReason ||
			condition.Type == cephv1.ConditionDeleting ||
			condition.Type == cephv1.ConditionDeletionIsBlocked {
			if conditionType != condition.Type {
				conditions = append(conditions, condition)
				continue
			}
			// Update the existing condition with the new status
			currentCondition = condition.DeepCopy()
			if currentCondition.Status != status || currentCondition.Message != message {
				// Update the last transition time since the status changed
				currentCondition.LastTransitionTime = metav1.NewTime(time.Now())
			}
			currentCondition.Status = status
			currentCondition.Reason = reason
			currentCondition.Message = message
			currentCondition.LastHeartbeatTime = metav1.NewTime(time.Now())
		}
	}
	if currentCondition == nil {
		// Create a new condition since not found in the existing conditions
		currentCondition = &cephv1.Condition{
			Type:               conditionType,
			Status:             status,
			Reason:             reason,
			Message:            message,
			LastTransitionTime: metav1.NewTime(time.Now()),
			LastHeartbeatTime:  metav1.NewTime(time.Now()),
		}
	}
	conditions = append(conditions, *currentCondition)
	cluster.Status.Conditions = conditions

	// Once the cluster begins deleting, the phase should not revert back to any other phase
	if cluster.Status.Phase != cephv1.ConditionDeleting {
		cluster.Status.Phase = conditionType
		if state := translatePhasetoState(conditionType, status); state != "" {
			cluster.Status.State = state
		}
		cluster.Status.Message = currentCondition.Message
		logger.Debugf("CephCluster %q status: %q. %q", namespaceName.Namespace, cluster.Status.Phase, cluster.Status.Message)
	}

	if err := reporting.UpdateStatus(c.Client, cluster); err != nil {
		logger.Errorf("failed to update cluster condition to %+v. %v", *currentCondition, err)
	}
}

// translatePhasetoState convert the Phases to corresponding State
// 1. We still need to set the State in case someone is still using it
// instead of Phase. If we stopped setting the State it would be a
// breaking change.
// 2. We can't change the enum values of the State since that is also
// a breaking change. Therefore, we translate new phases to the original
// State values
func translatePhasetoState(phase cephv1.ConditionType, status v1.ConditionStatus) cephv1.ClusterState {
	if status == v1.ConditionFalse {
		return cephv1.ClusterStateError
	}
	switch phase {
	case cephv1.ConditionConnecting:
		return cephv1.ClusterStateConnecting
	case cephv1.ConditionConnected:
		return cephv1.ClusterStateConnected
	case cephv1.ConditionProgressing:
		return cephv1.ClusterStateCreating
	case cephv1.ConditionReady:
		return cephv1.ClusterStateCreated
	case cephv1.ConditionDeleting:
		// "Deleting" was not a state before, so just translate the "Deleting" condition directly.
		return cephv1.ClusterState(cephv1.ConditionDeleting)
	default:
		return ""
	}
}
