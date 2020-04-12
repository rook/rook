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
package config

import (
	"time"

	"github.com/pkg/errors"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	conditions   *[]cephv1.Condition
	conditionMap = make(map[cephv1.ConditionType]v1.ConditionStatus)
)

// SetCondition updates the conditions of the cluster custom resource
func SetCondition(context *clusterd.Context, namespace, name string, conditionType cephv1.ConditionType, status v1.ConditionStatus, reason, message string) {
	condition := cephv1.Condition{
		Type:    conditionType,
		Status:  status,
		Reason:  reason,
		Message: message,
	}
	cluster, err := context.RookClientset.CephV1().CephClusters(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		logger.Errorf("failed to get cluster %v", err)
	}
	conditionMap[condition.Type] = condition.Status
	existingCondition := findStatusCondition(*conditions, condition.Type)
	if existingCondition == nil {
		condition.LastTransitionTime = metav1.NewTime(time.Now())
		condition.LastHeartbeatTime = metav1.NewTime(time.Now())
		*conditions = append(*conditions, condition)

	} else if existingCondition.Status != condition.Status || existingCondition.Message != condition.Message {
		condition.LastTransitionTime = metav1.NewTime(time.Now())
		existingCondition.Status = condition.Status
		existingCondition.Reason = condition.Reason
		existingCondition.Message = condition.Message
		existingCondition.LastHeartbeatTime = metav1.NewTime(time.Now())
	}
	cluster.Status.Conditions = *conditions

	if condition.Status == v1.ConditionTrue {
		cluster.Status.Phase = condition.Type
		if state := translatePhasetoState(condition.Type); state != "" {
			cluster.Status.State = state
		}
		cluster.Status.Message = condition.Message
		logger.Infof("CephCluster %q status: %q. %q", namespace, cluster.Status.Phase, cluster.Status.Message)
	}

	if _, err := context.RookClientset.CephV1().CephClusters(namespace).Update(cluster); err != nil {
		logger.Errorf("failed to update cluster condition %v", err)
	}
	if condition.Type == cephv1.ConditionReady {
		MarkProgressConditionsCompleted(context, namespace, name)
	}
}

// translatePhasetoState convert the Phases to corresponding State
// 1. We still need to set the State in case someone is still using it
// instead of Phase. If we stopped setting the State it would be a
// breaking change.
// 2. We can't change the enum values of the State since that is also
// a breaking change. Therefore, we translate new phases to the original
// State values
func translatePhasetoState(phase cephv1.ConditionType) cephv1.ClusterState {
	switch phase {
	case cephv1.ConditionConnecting:
		return cephv1.ClusterStateConnecting
	case cephv1.ConditionConnected:
		return cephv1.ClusterStateConnected
	case cephv1.ConditionFailure:
		return cephv1.ClusterStateError
	case cephv1.ConditionIgnored:
		return cephv1.ClusterStateError
	case cephv1.ConditionProgressing:
		return cephv1.ClusterStateCreating
	case cephv1.ConditionReady:
		return cephv1.ClusterStateCreated
	case cephv1.ConditionUpgrading:
		return cephv1.ClusterStateUpdating
	case cephv1.ConditionUpdating:
		return cephv1.ClusterStateUpdating
	default:
		return ""
	}
}

// MarkProgressConditionsCompleted Updates the status of Progressing, Updating or Upgrading to False once cluster is Ready
func MarkProgressConditionsCompleted(context *clusterd.Context, namespace, name string) {
	conditionsToUpdate := []cephv1.ConditionType{cephv1.ConditionUpdating, cephv1.ConditionUpgrading, cephv1.ConditionProgressing}
	for _, conditionType := range conditionsToUpdate {
		if conditionMap[conditionType] == v1.ConditionTrue {
			reason := ""
			message := ""
			if conditionType == cephv1.ConditionUpdating {
				reason = "UpdateCompleted"
				message = "Cluster updating is completed"
			} else if conditionType == cephv1.ConditionUpgrading {
				reason = "UpgradeCompleted"
				message = "Cluster upgrading is completed"
			} else if conditionType == cephv1.ConditionProgressing {
				reason = "ProgressingCompleted"
				message = "Cluster progression is completed"
			}
			if reason != "" {
				SetCondition(context, namespace, name, conditionType, v1.ConditionFalse, reason, message)
			}
		}
	}
}

// ConditionInitialize initializes some of the conditions at the beginning of cluster creation
func ConditionInitialize(context *clusterd.Context, namespace, name string) {
	cluster, err := context.RookClientset.CephV1().CephClusters(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		logger.Errorf("failed to get cluster %v", err)
	}
	if conditions == nil {
		conditions = &cluster.Status.Conditions
		if cluster.Status.Conditions != nil {
			conditionMapping(*conditions)
		}
	}

	if conditionMap[cephv1.ConditionReady] != v1.ConditionTrue {
		conditions = &[]cephv1.Condition{
			{
				Type:               cephv1.ConditionFailure,
				Status:             v1.ConditionFalse,
				Reason:             "",
				Message:            "",
				LastHeartbeatTime:  metav1.NewTime(time.Now()),
				LastTransitionTime: metav1.NewTime(time.Now()),
			}, {
				Type:               cephv1.ConditionIgnored,
				Status:             v1.ConditionFalse,
				Reason:             "",
				Message:            "",
				LastHeartbeatTime:  metav1.NewTime(time.Now()),
				LastTransitionTime: metav1.NewTime(time.Now()),
			}, {
				Type:               cephv1.ConditionUpgrading,
				Status:             v1.ConditionFalse,
				Reason:             "",
				Message:            "",
				LastHeartbeatTime:  metav1.NewTime(time.Now()),
				LastTransitionTime: metav1.NewTime(time.Now()),
			},
		}
		conditionMap[cephv1.ConditionFailure] = v1.ConditionFalse
		conditionMap[cephv1.ConditionIgnored] = v1.ConditionFalse
		conditionMap[cephv1.ConditionUpgrading] = v1.ConditionFalse
	}
}

// conditionMapping maps the condition type to its status
func conditionMapping(conditions []cephv1.Condition) {
	for i := range conditions {
		conditionType := conditions[i].Type
		conditionMap[conditionType] = conditions[i].Status
	}
}

// CheckConditionReady checks whether the cluster is Ready and returns the message for the Progressing ConditionType
func CheckConditionReady(context *clusterd.Context, namespace, name string) string {
	cluster, err := context.RookClientset.CephV1().CephClusters(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		logger.Errorf("failed to get cluster %v", err)
	}
	if cluster.Status.Conditions != nil && len(conditionMap) == 0 {
		conditionMapping(cluster.Status.Conditions)
	}
	if conditionMap[cephv1.ConditionReady] == v1.ConditionTrue {
		return "Cluster is checking if updates are needed"
	}
	return "Cluster is creating"
}

// ErrorMapping iterate through the Condition Map to see if Failure is True or False
func ErrorMapping() error {
	if conditionMap[cephv1.ConditionFailure] == v1.ConditionTrue {
		return errors.New("failed to initialize the cluster")
	}
	return nil
}

// findStatusCondition is used to find the already existing Condition Type
func findStatusCondition(conditions []cephv1.Condition, conditionType cephv1.ConditionType) *cephv1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}
