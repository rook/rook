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

package clients

import (
	"strings"
	"time"

	"github.com/rook/rook/tests/framework/installer"
	"github.com/rook/rook/tests/framework/utils"
)

// TopicOperation is a wrapper for rook notification operations
type NotificationOperation struct {
	k8sh      *utils.K8sHelper
	manifests installer.CephManifests
}

// CreateTopicOperation creates a new topic client
func CreateNotificationOperation(k8sh *utils.K8sHelper, manifests installer.CephManifests) *NotificationOperation {
	return &NotificationOperation{k8sh, manifests}
}

func (n *NotificationOperation) CreateNotification(notificationName string, topicName string) error {
	return n.k8sh.ResourceOperation("create", n.manifests.GetBucketNotification(notificationName, topicName))
}

func (n *NotificationOperation) DeleteNotification(notificationName string, topicName string) error {
	return n.k8sh.ResourceOperation("delete", n.manifests.GetBucketNotification(notificationName, topicName))
}

func (n *NotificationOperation) UpdateNotification(notificationName string, topicName string) error {
	return n.k8sh.ResourceOperation("apply", n.manifests.GetBucketNotification(notificationName, topicName))
}

// CheckNotification if notification was set
func (t *NotificationOperation) CheckNotificationCR(notificationName string) bool {
	// TODO: return result based on reconcile status of the CR
	const resourceName = "cephbucketnotification"
	_, err := t.k8sh.GetResource(resourceName, notificationName)
	if err != nil {
		logger.Infof("%q %q does not exist", resourceName, notificationName)
		return false
	}

	return true
}

const (
	notificationWaitAttempts = 12
	notificationWaitInterval = 5 * time.Second
)

// notificationRecords returns the http server log lines emitted strictly
// after since. The endpoint pod is shared by every scenario in the suite
// (including the other TLS pass), so an unbounded sample can match stale
// records; --timestamps gives per-line runtime stamps for a precise client
// side boundary, while --since-time only prefilters at second granularity.
func (t *NotificationOperation) notificationRecords(appLabel string, since time.Time) ([]string, error) {
	l, err := t.k8sh.Kubectl("logs",
		"--selector="+appLabel,
		"--timestamps",
		"--since-time="+since.Add(-2*time.Second).UTC().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}

	var records []string
	for _, line := range strings.Split(l, "\n") {
		stamp, record, found := strings.Cut(line, " ")
		if !found {
			continue
		}
		when, err := time.Parse(time.RFC3339Nano, stamp)
		if err != nil {
			continue
		}
		if when.After(since) {
			records = append(records, record)
		}
	}
	return records, nil
}

// recordMatches reports whether a single record line carries both eventName
// and fileName. Records are single-line JSON; matching the concatenated log
// would let two unrelated records satisfy the substrings independently.
func recordMatches(records []string, eventName, fileName string) bool {
	for _, record := range records {
		if strings.Contains(record, eventName) && strings.Contains(record, fileName) {
			return true
		}
	}
	return false
}

// CheckNotificationFromHTTPEndPoint polls the http endpoint logs for an
// eventName/fileName record emitted after since, for up to a minute —
// delivery lags well past a single fixed sleep on loaded CI runners.
func (t *NotificationOperation) CheckNotificationFromHTTPEndPoint(appLabel, eventName, fileName string, since time.Time) (bool, error) {
	var lastErr error
	for i := 0; i < notificationWaitAttempts; i++ {
		// wait for the notification to reach http-server
		time.Sleep(notificationWaitInterval)
		records, err := t.notificationRecords(appLabel, since)
		if err != nil {
			lastErr = err
			continue
		}
		lastErr = nil
		if recordMatches(records, eventName, fileName) {
			return true, nil
		}
	}
	return false, lastErr
}

// CheckNotificationAbsentFromHTTPEndPoint reports whether no eventName/
// fileName record was emitted after since: one settle interval, then a
// single sample. Callers pair it with a positive check that already proved
// delivery works, so polling longer would only slow the suite down.
func (t *NotificationOperation) CheckNotificationAbsentFromHTTPEndPoint(appLabel, eventName, fileName string, since time.Time) (bool, error) {
	time.Sleep(notificationWaitInterval)
	records, err := t.notificationRecords(appLabel, since)
	if err != nil {
		return false, err
	}
	return !recordMatches(records, eventName, fileName), nil
}
