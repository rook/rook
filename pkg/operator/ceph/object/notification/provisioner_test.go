/*
Copyright 2026 The Rook Authors. All rights reserved.

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

package notification

import (
	"testing"

	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/stretchr/testify/assert"
)

// The fallback is documented in the CRD description for
// CephBucketNotification.spec.events and in the bucket notification guide.
// Both are user-facing, so pin the set rather than let it drift silently.
func TestCreateS3EventsFallback(t *testing.T) {
	expected := []s3types.Event{
		s3types.Event("s3:ObjectCreated:*"),
		s3types.Event("s3:ObjectRemoved:*"),
	}

	t.Run("nil list falls back", func(t *testing.T) {
		assert.Equal(t, expected, createS3Events(nil))
	})

	t.Run("empty list falls back", func(t *testing.T) {
		assert.Equal(t, expected, createS3Events([]cephv1.BucketNotificationEvent{}))
	})

	t.Run("explicit events are passed through unchanged", func(t *testing.T) {
		events := []cephv1.BucketNotificationEvent{
			"s3:ObjectLifecycle:Expiration:Current",
			"s3:Replication:Create",
		}
		assert.Equal(t, []s3types.Event{
			s3types.Event("s3:ObjectLifecycle:Expiration:Current"),
			s3types.Event("s3:Replication:Create"),
		}, createS3Events(events))
	})
}
