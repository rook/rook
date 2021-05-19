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

// Package k8sutil for Kubernetes helpers.
package k8sutil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
)

func getEventsOccurences(channel chan string) map[string]int {

	foundEvents := make(map[string]int)

	for len(channel) > 0 {
		e := <-channel
		foundEvents[e]++
	}

	return foundEvents
}

func TestReportIfNotPresent(t *testing.T) {
	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod1",
			Namespace: "rook-ceph",
		},
	}

	pod2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod2",
			Namespace: "rook-ceph",
		},
	}

	testCases := []struct {
		eventReported                 int
		changeTime                    bool
		ReportAnotherEvent            bool
		ReportEventForDifferentObject bool
	}{
		{
			// verify ReportIfNotPresent is called once and event is reported once
			eventReported: 1,
		},
		{
			// verify ReportIfNotPresent is called twice and event is reported once
			eventReported: 2,
		},
		{
			// verify ReportIfNotPresent report same event again when event is not present on cluster
			eventReported: 1,
			changeTime:    true,
		},
		{
			// verify ReportIfNotPresent report same event again when event is not present on cluster
			eventReported: 2,
			changeTime:    true,
		},
		{
			// verify it report event "a" both the times if events came like "a", "b", "a"
			eventReported:      1,
			ReportAnotherEvent: true,
		},
		{
			// verify it report event "a" both the times if events came like "a", "b", "a"
			eventReported:      2,
			ReportAnotherEvent: true,
		},
		{
			// verify it does not report the same event for same objects if multiple objects come into the picture
			eventReported:                 1,
			ReportEventForDifferentObject: true,
		},
		{
			// verify it does not report the same event for same objects if multiple objects come into the picture
			eventReported:                 2,
			ReportEventForDifferentObject: true,
		},
	}

	for _, tc := range testCases {
		eventType, eventReason, eventMsg := corev1.EventTypeNormal, "Created", "Pod has been created"

		frecorder := record.NewFakeRecorder(1024)
		reporter := NewEventReporter(frecorder)

		for i := 0; i < tc.eventReported; i++ {
			reporter.ReportIfNotPresent(pod1, eventType, eventReason, eventMsg)
		}

		foundEvents := getEventsOccurences(frecorder.Events)
		assert.Equal(t, 1, foundEvents[eventType+" "+eventReason+" "+eventMsg])

		if tc.changeTime {
			nameSpacedName, err := getNameSpacedName(pod1)
			assert.NoError(t, err)
			ftime := reporter.lastReportedEventTime[nameSpacedName].Add(time.Minute * -60)
			reporter.lastReportedEventTime[nameSpacedName] = ftime

			reporter.ReportIfNotPresent(pod1, eventType, eventReason, eventMsg)
			foundEvents := getEventsOccurences(frecorder.Events)
			assert.Equal(t, 1, foundEvents[eventType+" "+eventReason+" "+eventMsg])
		}

		if tc.ReportAnotherEvent {
			reporter.ReportIfNotPresent(pod1, corev1.EventTypeWarning, eventReason, eventMsg)
			foundEvents := getEventsOccurences(frecorder.Events)
			assert.Equal(t, 1, foundEvents[corev1.EventTypeWarning+" "+eventReason+" "+eventMsg])

			reporter.ReportIfNotPresent(pod1, eventType, eventReason, eventMsg)
			foundEvents = getEventsOccurences(frecorder.Events)
			assert.Equal(t, 1, foundEvents[eventType+" "+eventReason+" "+eventMsg])
		}

		if tc.ReportEventForDifferentObject {
			reporter.ReportIfNotPresent(pod2, eventType, eventReason, eventMsg)
			foundEvents := getEventsOccurences(frecorder.Events)
			assert.Equal(t, 1, foundEvents[eventType+" "+eventReason+" "+eventMsg])

			reporter.ReportIfNotPresent(pod1, eventType, eventReason, eventMsg)
			foundEvents = getEventsOccurences(frecorder.Events)
			assert.Equal(t, 0, foundEvents[eventType+" "+eventReason+" "+eventMsg])
		}
	}
}
