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
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/ceph/go-ceph/rgw/admin"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/operator/ceph/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestGetUserCredentials(t *testing.T) {
	const owner = "alice"

	newAdminOpsContext := func(t *testing.T, userJSON string) *object.AdminOpsContext {
		t.Helper()
		mockClient := &object.MockClient{
			MockDo: func(req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodGet && req.URL.Path == "rgw.test/admin/user" && req.URL.Query().Get("uid") == owner {
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader(userJSON)),
					}, nil
				}
				return nil, fmt.Errorf("unexpected request: %s %s?%s", req.Method, req.URL.Path, req.URL.RawQuery)
			},
		}
		adminClient, err := admin.New("rgw.test", "admin-access", "admin-secret", mockClient)
		require.NoError(t, err)
		return &object.AdminOpsContext{AdminOpsClient: adminClient}
	}

	// radosgw-admin key rm can leave a user with no S3 keys at all
	t.Run("user without keys is an error", func(t *testing.T) {
		adminOpsCtx := newAdminOpsContext(t, `{"user_id": "alice", "keys": [], "swift_keys": []}`)

		accessKey, secretKey, err := getUserCredentials(adminOpsCtx, t.Context(), owner)
		assert.EqualError(t, err, `ceph user "alice" has no access keys`)
		assert.Empty(t, accessKey)
		assert.Empty(t, secretKey)
	})

	t.Run("user with a key yields its credentials", func(t *testing.T) {
		adminOpsCtx := newAdminOpsContext(t, `{"user_id": "alice", "keys": [{"user": "alice", "access_key": "alice-access", "secret_key": "alice-secret"}]}`)

		accessKey, secretKey, err := getUserCredentials(adminOpsCtx, t.Context(), owner)
		assert.NoError(t, err)
		assert.Equal(t, "alice-access", accessKey)
		assert.Equal(t, "alice-secret", secretKey)
	})
}
