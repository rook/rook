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
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
)

// EventReporter is custom events reporter type which allows user to limit the events
type EventReporter struct {
	recorder record.EventRecorder

	// lastReportedEvent will have a last captured event
	lastReportedEvent map[string]string

	// lastReportedEventTime will be the time of lastReportedEvent
	lastReportedEventTime map[string]time.Time
}

// NewEventReporter returns EventReporter object
func NewEventReporter(recorder record.EventRecorder) *EventReporter {
	return &EventReporter{
		recorder:              recorder,
		lastReportedEvent:     make(map[string]string),
		lastReportedEventTime: make(map[string]time.Time),
	}
}

// ReportIfNotPresent will report event if lastReportedEvent is not the same in last 60 minutes
func (rep *EventReporter) ReportIfNotPresent(instance runtime.Object, eventType, eventReason, msg string) {

	nameSpacedName, err := getNameSpacedName(instance)
	if err != nil {
		return
	}

	eventKey := getEventKey(eventType, eventReason, msg)

	if rep.lastReportedEvent[nameSpacedName] != eventKey || rep.lastReportedEventTime[nameSpacedName].Add(time.Minute*60).Before(time.Now()) {
		logger.Info("Reporting Event ", nameSpacedName, " ", eventKey)
		rep.lastReportedEvent[nameSpacedName] = eventKey
		rep.lastReportedEventTime[nameSpacedName] = time.Now()
		rep.recorder.Event(instance, eventType, eventReason, msg)
	} else {
		logger.Debug("Not Reporting Event because event is same as the old one:", nameSpacedName, " ", eventKey)
	}
}

func getNameSpacedName(instance runtime.Object) (string, error) {
	objMeta, err := meta.Accessor(instance)
	if err != nil {
		return "", err
	}
	return objMeta.GetNamespace() + ":" + objMeta.GetName(), nil
}

func getEventKey(eventType, eventReason, msg string) string {
	return fmt.Sprintf("%s:%s:%s", eventType, eventReason, msg)
}
