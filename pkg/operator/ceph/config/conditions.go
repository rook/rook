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
	"context"
	"time"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// ConditionExport function will export each condition into the cluster custom resource
func ConditionExport(context *clusterd.Context, namespaceName types.NamespacedName, conditionType cephv1.ConditionType, status v1.ConditionStatus, reason, message string) {
	setCondition(context, namespaceName, cephv1.Condition{
		Type:    conditionType,
		Status:  status,
		Reason:  reason,
		Message: message,
	})
}

// setCondition updates the conditions of the cluster custom resource
func setCondition(c *clusterd.Context, namespaceName types.NamespacedName, newCondition cephv1.Condition) {
	cluster, err := getCluster(c, namespaceName)
	if err != nil {
		logger.Errorf("failed to get cluster to set condition. %v", err)
		return
	}
	conditionMap := conditionMapping(cluster.Status.Conditions)
	if newCondition.Type == cephv1.ConditionReady {
		checkConditionFalse(c, namespaceName, conditionMap)
		cluster, err = getCluster(c, namespaceName)
		if err != nil {
			logger.Errorf("failed to get updated cluster to set later condition. %v", err)
		}
	}
	conditions := &cluster.Status.Conditions
	conditionMap[newCondition.Type] = newCondition.Status
	existingCondition := findStatusCondition(*conditions, newCondition.Type)
	if existingCondition == nil {
		newCondition.LastTransitionTime = metav1.NewTime(time.Now())
		newCondition.LastHeartbeatTime = metav1.NewTime(time.Now())
		*conditions = append(*conditions, newCondition)

	} else if existingCondition.Status != newCondition.Status || existingCondition.Message != newCondition.Message {
		newCondition.LastTransitionTime = metav1.NewTime(time.Now())
	}
	if existingCondition != nil {
		existingCondition.Status = newCondition.Status
		existingCondition.Reason = newCondition.Reason
		existingCondition.Message = newCondition.Message
		existingCondition.LastHeartbeatTime = metav1.NewTime(time.Now())
	}
	cluster.Status.Conditions = *conditions

	if newCondition.Status == v1.ConditionTrue {
		cluster.Status.Phase = newCondition.Type
		if state := translatePhasetoState(newCondition.Type); state != "" {
			cluster.Status.State = state
		}
		cluster.Status.Message = newCondition.Message
		logger.Debugf("CephCluster %q status: %q. %q", namespaceName.Namespace, cluster.Status.Phase, cluster.Status.Message)
	}

	err = c.Client.Status().Update(context.TODO(), cluster)
	if err != nil {
		logger.Errorf("failed to update cluster condition to %+v. %v", newCondition, err)
	}
}

// get the CephCluster with error logging
func getCluster(c *clusterd.Context, namespaceName types.NamespacedName) (*cephv1.CephCluster, error) {
	ctx := context.TODO()
	cluster, err := c.RookClientset.CephV1().CephClusters(namespaceName.Namespace).Get(ctx, namespaceName.Name, metav1.GetOptions{})
	if err != nil {
		if kerrors.IsNotFound(err) {
			logger.Errorf("no CephCluster could not be found. %+v", err)
			return nil, err
		}
		logger.Errorf("failed to get CephCluster object. %+v", err)
		return nil, err
	}
	return cluster, nil
}

// conditionMapping maps the condition type to its status
func conditionMapping(conditions []cephv1.Condition) (conditionMap map[cephv1.ConditionType]v1.ConditionStatus) {
	conditionMap = make(map[cephv1.ConditionType]v1.ConditionStatus)
	for i := range conditions {
		conditionType := conditions[i].Type
		conditionMap[conditionType] = conditions[i].Status
	}
	return conditionMap
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

// Updating the status of Progressing, Updating or Upgrading to False once cluster is Ready
func checkConditionFalse(context *clusterd.Context, namespaceName types.NamespacedName, conditionMap map[cephv1.ConditionType]v1.ConditionStatus) {
	tempConditionList := []cephv1.ConditionType{cephv1.ConditionUpdating, cephv1.ConditionUpgrading, cephv1.ConditionProgressing}
	var tempCondition cephv1.ConditionType
	for _, conditionType := range tempConditionList {
		if conditionMap[conditionType] == v1.ConditionTrue {
			tempCondition = conditionType
		}
	}
	reason := ""
	message := ""
	if tempCondition == cephv1.ConditionUpdating {
		reason = "UpdateCompleted"
		message = "Cluster updating is completed"
	} else if tempCondition == cephv1.ConditionUpgrading {
		reason = "UpgradeCompleted"
		message = "Cluster upgrading is completed"
	} else {
		tempCondition = cephv1.ConditionProgressing
		reason = "ProgressingCompleted"
		message = "Cluster progression is completed"
	}
	ConditionExport(context, namespaceName, tempCondition, v1.ConditionFalse, reason, message)
}

// ConditionInitialize initializes some of the conditions at the beginning of cluster creation
func ConditionInitialize(context *clusterd.Context, namespaceName types.NamespacedName) {
	setCondition(context, namespaceName, cephv1.Condition{
		Type:    cephv1.ConditionFailure,
		Status:  v1.ConditionFalse,
		Reason:  "",
		Message: "",
	})
	setCondition(context, namespaceName, cephv1.Condition{
		Type:    cephv1.ConditionIgnored,
		Status:  v1.ConditionFalse,
		Reason:  "",
		Message: "",
	})
	setCondition(context, namespaceName, cephv1.Condition{
		Type:    cephv1.ConditionUpgrading,
		Status:  v1.ConditionFalse,
		Reason:  "",
		Message: "",
	})
}

// CheckConditionReady checks whether the cluster is Ready and returns the message for the Progressing ConditionType
func CheckConditionReady(c *clusterd.Context, namespaceName types.NamespacedName) string {
	cluster, err := getCluster(c, namespaceName)
	if err != nil {
		if cluster.Status.Conditions != nil {
			conditionMap := conditionMapping(cluster.Status.Conditions)
			if conditionMap[cephv1.ConditionReady] == v1.ConditionTrue {
				return "Cluster is checking if updates are needed"
			}
		}
	}
	return "Cluster is creating"
}

//findStatusCondition is used to find the already existing Condition Type
func findStatusCondition(conditions []cephv1.Condition, conditionType cephv1.ConditionType) *cephv1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}
