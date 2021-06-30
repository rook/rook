/*
Copyright 2021 The Kubernetes Authors.

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
	"reflect"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Based on code from https://github.com/kubernetes/apimachinery/blob/master/pkg/api/meta/conditions.go

func TestSetStatusCondition(t *testing.T) {
	oneHourBefore := metav1.Time{Time: time.Now().Add(-1 * time.Hour)}
	oneHourAfter := metav1.Time{Time: time.Now().Add(1 * time.Hour)}

	tests := []struct {
		name       string
		conditions []Condition
		toAdd      Condition
		expected   []Condition
	}{
		{
			name: "should-add",
			conditions: []Condition{
				{Type: "first"},
				{Type: "third"},
			},
			toAdd: Condition{Type: "second", Status: v1.ConditionTrue, LastTransitionTime: oneHourBefore, LastHeartbeatTime: oneHourBefore, Reason: "reason", Message: "message"},
			expected: []Condition{
				{Type: "first"},
				{Type: "third"},
				{Type: "second", Status: v1.ConditionTrue, LastTransitionTime: oneHourBefore, LastHeartbeatTime: oneHourBefore, Reason: "reason", Message: "message"},
			},
		},
		{
			name: "use-supplied-transition-time",
			conditions: []Condition{
				{Type: "first"},
				{Type: "second", Status: v1.ConditionFalse},
				{Type: "third"},
			},
			toAdd: Condition{Type: "second", Status: v1.ConditionTrue, LastTransitionTime: oneHourBefore, LastHeartbeatTime: oneHourBefore, Reason: "reason", Message: "message"},
			expected: []Condition{
				{Type: "first"},
				{Type: "second", Status: v1.ConditionTrue, LastTransitionTime: oneHourBefore, LastHeartbeatTime: oneHourBefore, Reason: "reason", Message: "message"},
				{Type: "third"},
			},
		},
		{
			name: "update-fields",
			conditions: []Condition{
				{Type: "first"},
				{Type: "second", Status: v1.ConditionTrue, LastTransitionTime: oneHourBefore, LastHeartbeatTime: oneHourBefore},
				{Type: "third"},
			},
			toAdd: Condition{Type: "second", Status: v1.ConditionTrue, LastTransitionTime: oneHourAfter, LastHeartbeatTime: oneHourAfter, Reason: "reason", Message: "message"},
			expected: []Condition{
				{Type: "first"},
				{Type: "second", Status: v1.ConditionTrue, LastTransitionTime: oneHourBefore, LastHeartbeatTime: oneHourAfter, Reason: "reason", Message: "message"},
				{Type: "third"},
			},
		},
		{
			name:       "empty-conditions",
			conditions: []Condition{},
			toAdd:      Condition{Type: "first", Status: v1.ConditionTrue, LastTransitionTime: oneHourBefore, LastHeartbeatTime: oneHourBefore, Reason: "reason", Message: "message"},
			expected: []Condition{
				{Type: "first", Status: v1.ConditionTrue, LastTransitionTime: oneHourBefore, LastHeartbeatTime: oneHourBefore, Reason: "reason", Message: "message"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			SetStatusCondition(&test.conditions, test.toAdd)
			if !reflect.DeepEqual(test.conditions, test.expected) {
				t.Error(test.conditions)
			}
		})
	}
}

func TestFindStatusCondition(t *testing.T) {
	tests := []struct {
		name          string
		conditions    []Condition
		conditionType string
		expected      *Condition
	}{
		{
			name: "not-present",
			conditions: []Condition{
				{Type: "first"},
			},
			conditionType: "second",
			expected:      nil,
		},
		{
			name: "present",
			conditions: []Condition{
				{Type: "first"},
				{Type: "second"},
			},
			conditionType: "second",
			expected:      &Condition{Type: "second"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual := FindStatusCondition(test.conditions, ConditionType(test.conditionType))
			if !reflect.DeepEqual(actual, test.expected) {
				t.Error(actual)
			}
		})
	}
}
