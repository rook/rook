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

// ConditionExport function will export each condition into the cluster custom resource
func ConditionExport(context *clusterd.Context, namespace, name string, conditionType cephv1.ConditionType, status v1.ConditionStatus, reason, message string) {
	setCondition(context, namespace, name, cephv1.Condition{
		Type:    conditionType,
		Status:  status,
		Reason:  reason,
		Message: message,
	})
}

// setCondition updates the conditions of the cluster custom resource
func setCondition(context *clusterd.Context, namespace, name string, newCondition cephv1.Condition) {
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
	conditionMap[newCondition.Type] = newCondition.Status
	existingCondition := findStatusCondition(*conditions, newCondition.Type)
	if existingCondition == nil {
		newCondition.LastTransitionTime = metav1.NewTime(time.Now())
		newCondition.LastHeartbeatTime = metav1.NewTime(time.Now())
		*conditions = append(*conditions, newCondition)

	} else if existingCondition.Status != newCondition.Status || existingCondition.Message != newCondition.Message {
		newCondition.LastTransitionTime = metav1.NewTime(time.Now())
		existingCondition.Status = newCondition.Status
		existingCondition.Reason = newCondition.Reason
		existingCondition.Message = newCondition.Message
		existingCondition.LastHeartbeatTime = metav1.NewTime(time.Now())
	}
	cluster.Status.Conditions = *conditions

	if newCondition.Status == v1.ConditionTrue {
		cluster.Status.Phase = newCondition.Type
		cluster.Status.Message = newCondition.Message
		logger.Infof("CephCluster %q status: %q. %q", namespace, cluster.Status.Phase, cluster.Status.Message)
	}

	if _, err := context.RookClientset.CephV1().CephClusters(namespace).Update(cluster); err != nil {
		logger.Errorf("failed to update cluster condition %v", err)
	}
	if newCondition.Type == cephv1.ConditionReady {
		checkConditionFalse(context, namespace, name)
	}
}

// Updating the status of Progressing, Updating or Upgrading to False once cluster is Ready
func checkConditionFalse(context *clusterd.Context, namespace, name string) {
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
		reason = "ProgressingCompleted"
		message = "Cluster progression is completed"
	}
	ConditionExport(context, namespace, name, tempCondition, v1.ConditionFalse, reason, message)
}

// ConditionInitialize initializes some of the conditions at the beginning of cluster creation
func ConditionInitialize(context *clusterd.Context, namespace, name string) {
	setCondition(context, namespace, name, cephv1.Condition{
		Type:    cephv1.ConditionFailure,
		Status:  v1.ConditionFalse,
		Reason:  "",
		Message: "",
	})
	setCondition(context, namespace, name, cephv1.Condition{
		Type:    cephv1.ConditionIgnored,
		Status:  v1.ConditionFalse,
		Reason:  "",
		Message: "",
	})
	setCondition(context, namespace, name, cephv1.Condition{
		Type:    cephv1.ConditionUpgrading,
		Status:  v1.ConditionFalse,
		Reason:  "",
		Message: "",
	})
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

//findStatusCondition is used to find the already existing Condition Type
func findStatusCondition(conditions []cephv1.Condition, conditionType cephv1.ConditionType) *cephv1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}
