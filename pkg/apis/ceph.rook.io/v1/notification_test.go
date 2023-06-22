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
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestValidateNotificationSpec(t *testing.T) {
	notification := &CephBucketNotification{
		ObjectMeta: metav1.ObjectMeta{
			Name: "notification1",
		},
		Spec: BucketNotificationSpec{
			Topic:  "fish-topic",
			Events: []BucketNotificationEvent{BucketNotificationEvent("s3:ObjectCreated:*")},
			Filter: &NotificationFilterSpec{
				KeyFilters: []NotificationKeyFilterRule{
					{Name: "prefix", Value: "hello"},
					{Name: "suffix", Value: ".txt"},
					{Name: "regex", Value: "he[a-zA-Z0-9]o"},
				},
				MetadataFilters: []NotificationFilterRule{
					{Name: "x-amz-m1", Value: "value1"},
					{Name: "x-amz-v2", Value: "value2"},
					{Name: "x-amz-v3", Value: "value3"},
				},
				TagFilters: []NotificationFilterRule{
					{Name: "color", Value: "blue"},
					{Name: "application", Value: "streaming"},
					{Name: "city", Value: "chicago"},
				},
			},
		},
	}

	t.Run("valid", func(t *testing.T) {
		err := ValidateNotificationSpec(notification)
		assert.NoError(t, err)
	})
}

func TestPublicNotificationValidation(t *testing.T) {
	notification := &CephBucketNotification{
		ObjectMeta: metav1.ObjectMeta{
			Name: "notification1",
		},
		Spec: BucketNotificationSpec{
			Topic:  "fish-topic",
			Events: []BucketNotificationEvent{},
		},
	}

	t.Run("create", func(t *testing.T) {
		_, err := notification.ValidateCreate()
		assert.NoError(t, err)
	})

	t.Run("update", func(t *testing.T) {
		_, err := notification.ValidateUpdate(notification)
		assert.NoError(t, err)
	})

	t.Run("delete", func(t *testing.T) {
		_, err := notification.ValidateDelete()
		assert.NoError(t, err)
	})
}
