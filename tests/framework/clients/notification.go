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

func (t *NotificationOperation) CheckNotificationFromHTTPEndPoint(appLabel, eventName, fileName string) (bool, error) {
	// wait for the notification to reach http-server
	time.Sleep(5 * time.Second)
	selectorName := "--selector=" + appLabel
	l, err := t.k8sh.Kubectl("logs", selectorName, "--tail", "5")
	if err != nil {
		return false, err
	}
	if strings.Contains(l, eventName) && strings.Contains(l, fileName) {
		return true, nil
	}
	return false, nil
}
