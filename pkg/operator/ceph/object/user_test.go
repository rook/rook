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

package object

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeUserDoesNotLeakKeys(t *testing.T) {
	// decodeUser is handed radosgw-admin user output, which carries the user's S3
	// keys. Its error reaches a Kubernetes event and the CephObjectStore status by
	// way of the admin-ops and dashboard user provisioning.
	const (
		accessKey = "EXAMPLEACCESSKEYID01"
		secretKey = "EXAMPLEUSERSECRETKEY0000000000000000000001"
	)
	keys := `"keys": [{"user": "my-user", "access_key": "` + accessKey + `", "secret_key": "` + secretKey + `"}]`

	tests := []struct {
		name string
		json string
		// the field a type error names; empty when the error is a syntax error,
		// which reports only its own message
		namesField string
	}{
		{
			// max_buckets is an int in admin.User, so a string is the shape a schema
			// change across Ceph versions would take
			name:       "type error",
			json:       `{"user_id": "my-user", "max_buckets": "not-an-int", ` + keys + `}`,
			namesField: "max_buckets",
		},
		{
			name: "truncated response",
			json: `{"user_id": "my-user", ` + keys,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, code, err := decodeUser(tc.json)

			require.Error(t, err)
			assert.Equal(t, RGWErrorParse, code)
			assert.NotContains(t, err.Error(), secretKey)
			assert.NotContains(t, err.Error(), accessKey)
			// the response size is the only context a syntax error leaves
			assert.Contains(t, err.Error(), fmt.Sprintf("(%d bytes)", len(tc.json)))
			if tc.namesField != "" {
				assert.Contains(t, err.Error(), tc.namesField)
			}
		})
	}
}
