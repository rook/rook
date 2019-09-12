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

package nodedrain

import (
	"testing"

	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
)

func TestCanaryTolerations(t *testing.T) {
	uniqueTolerationsManual := []corev1.Toleration{
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
	for i := range uniqueTolerationsManual {
		tolerationsWithDuplicates = append(tolerationsWithDuplicates, uniqueTolerationsManual[i])

		//append the previous one again if it's within range, else append the last one
		if i > 0 {
			tolerationsWithDuplicates = append(tolerationsWithDuplicates, uniqueTolerationsManual[i-1])
		} else {
			tolerationsWithDuplicates = append(tolerationsWithDuplicates, uniqueTolerationsManual[len(uniqueTolerationsManual)-1])
		}
	}
	uniqueTolerationsMap := make(map[corev1.Toleration]struct{})
	for _, toleration := range tolerationsWithDuplicates {
		uniqueTolerationsMap[toleration] = struct{}{}
	}

	uniqueTolerations := tolerationMapToList(uniqueTolerationsMap)

	assert.Equal(t, len(uniqueTolerationsManual), len(uniqueTolerations))
	for _, tolerationI := range uniqueTolerationsManual {
		found := false
		for _, tolerationJ := range uniqueTolerations {
			if tolerationI == tolerationJ {
				found = true
			}
		}
		assert.True(t, found)
	}
}
