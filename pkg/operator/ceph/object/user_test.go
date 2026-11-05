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
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/coreos/pkg/capnslog"
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

// captureLogsAtLevel redirects the capnslog sink into a buffer at the given level
// for the duration of the test, restoring the package default afterwards so later
// tests still log to stderr and never at TRACE.
func captureLogsAtLevel(t *testing.T, level capnslog.LogLevel) *bytes.Buffer {
	t.Helper()

	logBuf := bytes.NewBuffer([]byte{})
	capnslog.SetFormatter(capnslog.NewLogFormatter(logBuf, "", 0))
	capnslog.SetGlobalLogLevel(level)
	t.Cleanup(func() {
		capnslog.SetFormatter(capnslog.NewDefaultFormatter(os.Stderr))
		capnslog.SetGlobalLogLevel(capnslog.INFO)
	})

	return logBuf
}

func TestDecodeUserLogsRawResponseOnlyUnderTrace(t *testing.T) {
	// the raw response is the only way to tell a truncated document from a schema
	// change, but it carries the user's S3 keys, so it is logged only at the TRACE
	// level that ROOK_LOG_LEVEL=TRACE_INSECURE unlocks.
	const secretKey = "EXAMPLEUSERSECRETKEY0000000000000000000001"
	badJSON := `{"user_id": "my-user", "keys": [{"secret_key": "` + secretKey + `"}]`

	for _, tc := range []struct {
		level  capnslog.LogLevel
		logged bool
	}{
		{level: capnslog.DEBUG, logged: false},
		{level: capnslog.TRACE, logged: true},
	} {
		t.Run(tc.level.String(), func(t *testing.T) {
			logBuf := captureLogsAtLevel(t, tc.level)

			_, _, err := decodeUser(badJSON)

			require.Error(t, err)
			assert.NotContains(t, err.Error(), secretKey)
			if tc.logged {
				assert.Contains(t, logBuf.String(), secretKey)
			} else {
				assert.NotContains(t, logBuf.String(), secretKey)
			}
		})
	}
}
