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

package v1

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Based on code from https://github.com/kubernetes/apimachinery/blob/master/pkg/api/meta/conditions.go

// A StatusConditionGetter allows getting a pointer to an object's conditions.
type StatusConditionGetter interface {
	client.Object

	// GetStatusConditions returns a pointer to the object's conditions compatible with
	// SetStatusCondition and FindStatusCondition.
	GetStatusConditions() *[]Condition
}

// SetStatusCondition sets the corresponding condition in conditions to newCondition.
// conditions must be non-nil.
//  1. if the condition of the specified type already exists (all fields of the existing condition are updated to
//     newCondition, LastTransitionTime is set to now if the new status differs from the old status)
//  2. if a condition of the specified type does not exist (LastTransitionTime is set to now() if unset, and newCondition is appended)
func SetStatusCondition(conditions *[]Condition, newCondition Condition) {
	if conditions == nil {
		return
	}

	now := metav1.NewTime(time.Now())

	existingCondition := FindStatusCondition(*conditions, newCondition.Type)
	if existingCondition == nil {
		if newCondition.LastTransitionTime.IsZero() {
			newCondition.LastTransitionTime = now
			newCondition.LastHeartbeatTime = now
		}
		*conditions = append(*conditions, newCondition)
		return
	}

	if existingCondition.Status != newCondition.Status {
		existingCondition.Status = newCondition.Status
		if !newCondition.LastTransitionTime.IsZero() {
			existingCondition.LastTransitionTime = newCondition.LastTransitionTime
		} else {
			existingCondition.LastTransitionTime = now
		}
	}

	existingCondition.Reason = newCondition.Reason
	existingCondition.Message = newCondition.Message
	if !newCondition.LastHeartbeatTime.IsZero() {
		existingCondition.LastHeartbeatTime = newCondition.LastHeartbeatTime
	} else {
		existingCondition.LastHeartbeatTime = now
	}
}

// FindStatusCondition finds the conditionType in conditions.
func FindStatusCondition(conditions []Condition, conditionType ConditionType) *Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}

	return nil
}
