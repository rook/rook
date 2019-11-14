/*
Copyright 2016 The Rook Authors. All rights reserved.

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

// Package status to provide conditions for CephCluster
package status

import (
	"time"

	"github.com/coreos/pkg/capnslog"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/clusterd"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	logger    = capnslog.NewPackageLogger("github.com/rook/rook", "op-status")
	namespace string
	name      string
)

func ClusterInfo(Namespace, Name string) {
	namespace = Namespace
	name = Name
}

// updates the conditions of the cluster custom resource
func setCondition(context *clusterd.Context, conditions *[]cephv1.RookConditions, newCondition cephv1.RookConditions) {
	if conditions == nil {
		conditions = &[]cephv1.RookConditions{}
	}
	existingCondition := findStatusCondition(*conditions, newCondition.Type)
	if existingCondition == nil {
		newCondition.LastTransitionTime = metav1.NewTime(time.Now())
		newCondition.LastHeartbeatTime = metav1.NewTime(time.Now())
		applyCondition(context, conditions, newCondition)
		return
	}
	if existingCondition.Status != newCondition.Status {
		removeStatusCondition(conditions, newCondition.Type)
		newCondition.LastTransitionTime = metav1.NewTime(time.Now())
		applyCondition(context, conditions, newCondition)
	}
}

func ConditionConnecting(context *clusterd.Context, conditions *[]cephv1.RookConditions, reason, message string) {
	setCondition(context, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionConnecting,
		Status:  v1.ConditionTrue,
		Reason:  reason,
		Message: message,
	})
}

func ConditionConnected(context *clusterd.Context, conditions *[]cephv1.RookConditions, reason, message string) {
	setCondition(context, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionConnected,
		Status:  v1.ConditionTrue,
		Reason:  reason,
		Message: message,
	})
}

func ConditionIgnored(context *clusterd.Context, conditions *[]cephv1.RookConditions, reason, message string) {
	setCondition(context, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionIgnored,
		Status:  v1.ConditionTrue,
		Reason:  reason,
		Message: message,
	})
}

func ConditionUpgrading(context *clusterd.Context, conditions *[]cephv1.RookConditions, reason, message string) {
	setCondition(context, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionNotReady,
		Status:  v1.ConditionTrue,
		Reason:  reason,
		Message: message,
	})
	setCondition(context, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionReady,
		Status:  v1.ConditionFalse,
		Reason:  "",
		Message: "",
	})
	setCondition(context, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionAvailable,
		Status:  v1.ConditionFalse,
		Reason:  "",
		Message: "",
	})
	setCondition(context, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionUpgrading,
		Status:  v1.ConditionTrue,
		Reason:  reason,
		Message: message,
	})
}

func ConditionFailure(context *clusterd.Context, conditions *[]cephv1.RookConditions, reason, message string) {
	setCondition(context, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionReady,
		Status:  v1.ConditionFalse,
		Reason:  "ClusterNotReady",
		Message: "",
	})
	setCondition(context, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionAvailable,
		Status:  v1.ConditionFalse,
		Reason:  "",
		Message: "",
	})
	setCondition(context, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionNotReady,
		Status:  v1.ConditionTrue,
		Reason:  reason,
		Message: message,
	})
	setCondition(context, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionFailure,
		Status:  v1.ConditionTrue,
		Reason:  reason,
		Message: message,
	})
}

func ConditionProgressing(context *clusterd.Context, conditions *[]cephv1.RookConditions, reason, message string) {
	setCondition(context, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionNotReady,
		Status:  v1.ConditionTrue,
		Reason:  "",
		Message: message,
	})
	setCondition(context, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionProgressing,
		Status:  v1.ConditionTrue,
		Reason:  reason,
		Message: message,
	})
}

func ConditionReady(context *clusterd.Context, conditions *[]cephv1.RookConditions, reason, message string) {
	setCondition(context, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionNotReady,
		Status:  v1.ConditionFalse,
		Reason:  "",
		Message: message,
	})
	setCondition(context, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionReady,
		Status:  v1.ConditionTrue,
		Reason:  reason,
		Message: message,
	})
}

func ConditionUpdating(context *clusterd.Context, conditions *[]cephv1.RookConditions, reason, message string) {
	setCondition(context, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionReady,
		Status:  v1.ConditionFalse,
		Reason:  "ClusterNotReady",
		Message: "",
	})
	setCondition(context, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionNotReady,
		Status:  v1.ConditionTrue,
		Reason:  reason,
		Message: message,
	})
}

func ConditionAvailable(context *clusterd.Context, conditions *[]cephv1.RookConditions, reason, message string) {
	setCondition(context, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionAvailable,
		Status:  v1.ConditionTrue,
		Reason:  reason,
		Message: message,
	})
}

func ConditionInitialize(context *clusterd.Context, conditions *[]cephv1.RookConditions, reason, message string) {
	setCondition(context, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionConnecting,
		Status:  v1.ConditionFalse,
		Reason:  "",
		Message: "",
	})
	setCondition(context, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionConnected,
		Status:  v1.ConditionFalse,
		Reason:  "",
		Message: "",
	})
	setCondition(context, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionFailure,
		Status:  v1.ConditionFalse,
		Reason:  "",
		Message: "",
	})
	setCondition(context, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionIgnored,
		Status:  v1.ConditionFalse,
		Reason:  "",
		Message: "",
	})
	setCondition(context, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionUpgrading,
		Status:  v1.ConditionFalse,
		Reason:  "",
		Message: "",
	})
	setCondition(context, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionAvailable,
		Status:  v1.ConditionFalse,
		Reason:  "",
		Message: "",
	})
	setCondition(context, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionExpanding,
		Status:  v1.ConditionFalse,
		Reason:  "",
		Message: "",
	})
	setCondition(context, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionNotReady,
		Status:  v1.ConditionTrue,
		Reason:  reason,
		Message: message,
	})
}

func ConditionExpanding(context *clusterd.Context, conditions *[]cephv1.RookConditions, reason, message string) {
	setCondition(context, conditions, cephv1.RookConditions{
		Type:    cephv1.ConditionExpanding,
		Status:  v1.ConditionTrue,
		Reason:  reason,
		Message: message,
	})
}

// applying the latest condition on to the cluster CRD file
func applyCondition(context *clusterd.Context, conditions *[]cephv1.RookConditions, newCondition cephv1.RookConditions) {
	if name == "" {
		return
	} else {
		cluster, err := context.RookClientset.CephV1().CephClusters(namespace).Get(name, metav1.GetOptions{})
		if err != nil {
			logger.Errorf("failed to get cluster from namespace %s prior to updating its condition to %s. %+v", namespace, newCondition.Type, err)
		}
		*conditions = append(*conditions, newCondition)
		cluster.Status.Condition = *conditions
		cluster.Status.FinalCondition = newCondition.Type
		cluster.Status.Message = newCondition.Message
		if newCondition.Status == v1.ConditionTrue {
			logger.Infof("CephCluster %s status: %s. %s", namespace, cluster.Status.FinalCondition, cluster.Status.Message)
		}
		if _, err := context.RookClientset.CephV1().CephClusters(namespace).Update(cluster); err != nil {
			logger.Errorf("failed to update cluster %s condition: %+v", namespace, err)
		}
	}
}

// removes a condition from the list
func removeStatusCondition(conditions *[]cephv1.RookConditions, conditionType cephv1.ConditionType) {
	if conditions == nil {
		return
	}
	newConditions := []cephv1.RookConditions{}
	for _, condition := range *conditions {
		if condition.Type != conditionType {
			newConditions = append(newConditions, condition)
		}
	}

	*conditions = newConditions
}

//findStatusCondition is used to find the already exisiting Condition Type
func findStatusCondition(conditions []cephv1.RookConditions, conditionType cephv1.ConditionType) *cephv1.RookConditions {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}
