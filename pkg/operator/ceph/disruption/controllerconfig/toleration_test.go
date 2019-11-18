/*
Copyright 2019 The Rook Authors. All rights reserved.

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

package controllerconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
)

func TestTolerationSet(t *testing.T) {
	uniqueTolerationsManualA := []corev1.Toleration{
		// key1
		//   exists
		{
			Key:      "key1",
			Operator: corev1.TolerationOpExists,
			Effect:   corev1.TaintEffectNoSchedule,
		},
		{
			Key:      "key1",
			Operator: corev1.TolerationOpExists,
			Effect:   corev1.TaintEffectPreferNoSchedule,
		},
		//   equals with different values
		{
			Key:      "key1",
			Operator: corev1.TolerationOpEqual,
			Value:    "value1",
			Effect:   corev1.TaintEffectNoSchedule,
		},
		{
			Key:      "key1",
			Operator: corev1.TolerationOpEqual,
			Value:    "value2",
			Effect:   corev1.TaintEffectNoSchedule,
		},

		//   with different effects
		{
			Key:      "key1",
			Operator: corev1.TolerationOpEqual,
			Value:    "value2",
			Effect:   corev1.TaintEffectNoExecute,
		},
		// key2
		{
			Key:      "key2",
			Operator: corev1.TolerationOpExists,
			Effect:   corev1.TaintEffectNoSchedule,
		},
		{
			Key:      "key2",
			Operator: corev1.TolerationOpEqual,
			Value:    "value1",
			Effect:   corev1.TaintEffectNoSchedule,
		},
		{
			Key:      "key2",
			Operator: corev1.TolerationOpEqual,
			Value:    "value2",
			Effect:   corev1.TaintEffectNoSchedule,
		},
		{
			Key:      "key2",
			Operator: corev1.TolerationOpEqual,
			Value:    "value2",
			Effect:   corev1.TaintEffectNoExecute,
		},
	}
	//identical to uniqueTolerationsManualA
	uniqueTolerationsManualB := []corev1.Toleration{
		// key1
		//   exists
		{
			Key:      "key1",
			Operator: corev1.TolerationOpExists,
			Effect:   corev1.TaintEffectNoSchedule,
		},
		{
			Key:      "key1",
			Operator: corev1.TolerationOpExists,
			Effect:   corev1.TaintEffectPreferNoSchedule,
		},
		//   equals with different values
		{
			Key:      "key1",
			Operator: corev1.TolerationOpEqual,
			Value:    "value1",
			Effect:   corev1.TaintEffectNoSchedule,
		},
		{
			Key:      "key1",
			Operator: corev1.TolerationOpEqual,
			Value:    "value2",
			Effect:   corev1.TaintEffectNoSchedule,
		},

		//   with different effects
		{
			Key:      "key1",
			Operator: corev1.TolerationOpEqual,
			Value:    "value2",
			Effect:   corev1.TaintEffectNoExecute,
		},
		// key2
		{
			Key:      "key2",
			Operator: corev1.TolerationOpExists,
			Effect:   corev1.TaintEffectNoSchedule,
		},
		{
			Key:      "key2",
			Operator: corev1.TolerationOpEqual,
			Value:    "value1",
			Effect:   corev1.TaintEffectNoSchedule,
		},
		{
			Key:      "key2",
			Operator: corev1.TolerationOpEqual,
			Value:    "value2",
			Effect:   corev1.TaintEffectNoSchedule,
		},
		{
			Key:      "key2",
			Operator: corev1.TolerationOpEqual,
			Value:    "value2",
			Effect:   corev1.TaintEffectNoExecute,
		},
	}

	tolerationsWithDuplicates := make([]corev1.Toleration, 0)
	for i := range uniqueTolerationsManualA {
		tolerationsWithDuplicates = append(tolerationsWithDuplicates, uniqueTolerationsManualA[i])

		//append the previous one again if it's within range, else append the last one
		if i > 0 {
			tolerationsWithDuplicates = append(tolerationsWithDuplicates, uniqueTolerationsManualB[i-1])
		} else {
			tolerationsWithDuplicates = append(tolerationsWithDuplicates, uniqueTolerationsManualB[len(uniqueTolerationsManualB)-1])
		}
	}
	uniqueTolerationsMap := &TolerationSet{}
	for _, toleration := range tolerationsWithDuplicates {
		uniqueTolerationsMap.Add(toleration)
	}

	uniqueTolerations := uniqueTolerationsMap.ToList()

	assert.Equal(t, len(uniqueTolerationsManualA), len(uniqueTolerations))
	for _, tolerationI := range uniqueTolerationsManualA {
		found := false
		for _, tolerationJ := range uniqueTolerations {
			if tolerationI == tolerationJ {
				found = true
			}
		}
		assert.True(t, found)
	}
}
