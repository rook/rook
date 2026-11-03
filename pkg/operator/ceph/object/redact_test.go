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
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Obviously fake stand-ins for credentials a zone document carries. The first two are the
// fields Ceph is known to put them in today; the rest are invented, standing in for the
// credential a newer Ceph adds under a name nothing in rook knows - the case a list of names
// to redact cannot cover, and the reason this filter works the other way round.
const (
	testZoneAccessKey      = "EXAMPLEZONEACCESSKEY"
	testZoneSecretKey      = "EXAMPLEZONESYSTEMSECRETKEY00000000000001"
	testTierSecret         = "EXAMPLETIERREMOTESECRET000000000000000001"
	testUnknownSecret      = "EXAMPLEUNKNOWNTOPLEVELSECRET000000000001"
	testNestedSecret       = "EXAMPLEUNKNOWNPLACEMENTSECRET00000000001"
	testStorageClassSecret = "EXAMPLEUNKNOWNSTORAGECLASSSECRET00000001"
)

// testZoneJSON is a 'radosgw-admin zone get' response in the shape the zone fixtures in
// shared_pools_test.go and zone/controller_test.go use, carrying the credential fields Ceph
// puts in one today and three invented fields in the places a future one could.
const testZoneJSON = `{
    "id": "f539c2c0-e1ed-4c42-9294-41742352eeae",
    "name": "test",
    "domain_root": "test.rgw.meta:root",
    "control_pool": "test.rgw.control",
    "log_pool": "test.rgw.log",
    "system_key": {
        "access_key": "` + testZoneAccessKey + `",
        "secret_key": "` + testZoneSecretKey + `"
    },
    "tier_config": {
        "connection": {
            "access_key": "EXAMPLETIERACCESSKEY",
            "secret": "` + testTierSecret + `",
            "endpoint": "https://remote.example.com"
        }
    },
    "cloud_sync_credentials": "` + testUnknownSecret + `",
    "placement_pools": [
        {
            "key": "default-placement",
            "replication_secret": "` + testNestedSecret + `",
            "val": {
                "index_pool": "test.rgw.buckets.index",
                "storage_classes": {
                    "STANDARD": {
                        "data_pool": "test.rgw.buckets.data",
                        "upload_secret": "` + testStorageClassSecret + `"
                    }
                },
                "data_extra_pool": "test.rgw.buckets.non-ec",
                "index_type": 0,
                "inline_data": true
            }
        }
    ],
    "realm_id": "29e28253-be54-4581-90dd-206020d2fcdd"
}`

func TestRedactZoneJSONWithholdsUnknownFields(t *testing.T) {
	// The justification for filtering by allow list: none of these field names appears
	// anywhere in rook, at the top level or nested inside a placement, and all three are
	// withheld anyway because nothing permits them.
	redacted := RedactZoneJSON(testZoneJSON)

	assert.NotContains(t, redacted, testUnknownSecret, "unknown top level field was logged")
	assert.NotContains(t, redacted, testNestedSecret, "unknown field inside placement_pools was logged")
	assert.NotContains(t, redacted, testStorageClassSecret, "unknown field inside a storage class was logged")
	assert.NotContains(t, redacted, "cloud_sync_credentials")
	assert.NotContains(t, redacted, "replication_secret")
	assert.NotContains(t, redacted, "upload_secret")
}

func TestRedactZoneJSONWithholdsKnownCredentialFields(t *testing.T) {
	// system_key holds the realm system user's live S3 credentials and tier_config a cloud
	// tier's credentials for its remote. Neither is named by the filter: they are withheld
	// because nothing permits them, which is what TestRedactZoneJSONNamesNoCredentialField
	// pins.
	redacted := RedactZoneJSON(testZoneJSON)

	assert.NotContains(t, redacted, testZoneAccessKey)
	assert.NotContains(t, redacted, testZoneSecretKey)
	assert.NotContains(t, redacted, testTierSecret)
	assert.NotContains(t, redacted, "EXAMPLETIERACCESSKEY")
	// the remote's endpoint is not itself a credential, but it is not a field rook reads
	assert.NotContains(t, redacted, "remote.example.com")
	assert.NotContains(t, redacted, "system_key")
	assert.NotContains(t, redacted, "tier_config")
}

func TestRedactZoneJSONNamesNoCredentialField(t *testing.T) {
	// A credential field named in the filter would be a deny list growing back: the names Ceph
	// gives its credential fields are exactly what rook cannot keep up with. The one list the
	// filter carries names fields to keep.
	source, err := os.ReadFile("redact.go")
	require.NoError(t, err)

	for _, field := range []string{"system_key", "tier_config"} {
		assert.NotContains(t, string(source), field,
			"redact.go names credential field %q: fields are withheld by default, not by name", field)
	}
}

func TestRedactZoneJSONKeepsFieldsRookReads(t *testing.T) {
	// What proves the document was filtered rather than suppressed: everything rook reads is
	// still there, with its real values, and each object says how many of its own members were
	// withheld.
	const expected = `{
    "<withheld>": 3,
    "id": "f539c2c0-e1ed-4c42-9294-41742352eeae",
    "name": "test",
    "realm_id": "29e28253-be54-4581-90dd-206020d2fcdd",
    "domain_root": "test.rgw.meta:root",
    "control_pool": "test.rgw.control",
    "log_pool": "test.rgw.log",
    "placement_pools": [
        {
            "<withheld>": 1,
            "key": "default-placement",
            "val": {
                "<withheld>": 1,
                "index_pool": "test.rgw.buckets.index",
                "data_extra_pool": "test.rgw.buckets.non-ec",
                "inline_data": true,
                "storage_classes": {
                    "STANDARD": {
                        "<withheld>": 1,
                        "data_pool": "test.rgw.buckets.data"
                    }
                }
            }
        }
    ]
}`

	assert.JSONEq(t, expected, RedactZoneJSON(testZoneJSON))
}

func TestRedactZoneJSONKeepsEveryZonePoolField(t *testing.T) {
	// The pool fields come from ZoneJsonPoolKeys, the same map that decides which pools rook
	// namespaces, so a pool field added to zonePoolNSSuffix widens the filter with it and this
	// test covers it without being touched.
	zone := map[string]any{"name": "test"}
	poolFields := ZoneJsonPoolKeys()
	require.NotEmpty(t, poolFields)
	for field := range poolFields {
		zone[field] = "test.pool." + field
	}
	zoneJSON, err := json.Marshal(zone)
	require.NoError(t, err)

	redacted := RedactZoneJSON(string(zoneJSON))

	for field := range poolFields {
		assert.Contains(t, redacted, fmt.Sprintf("%q:%q", field, "test.pool."+field))
	}
	// nothing was dropped, so nothing claims it was
	assert.NotContains(t, redacted, withheldMarker)
}

func TestRedactZoneJSONMarksWithheldFieldsOnlyWhenThereAreSome(t *testing.T) {
	tests := []struct {
		name     string
		zoneJSON string
		marked   bool
	}{
		{
			name:     "nothing withheld",
			zoneJSON: `{"id": "zone-id", "name": "test", "realm_id": "realm-id", "log_pool": "test.rgw.log"}`,
			marked:   false,
		},
		{
			name:     "nothing withheld in an empty document",
			zoneJSON: `{}`,
			marked:   false,
		},
		{
			name:     "nothing withheld through an empty placement list",
			zoneJSON: `{"name": "test", "placement_pools": []}`,
			marked:   false,
		},
		{
			name:     "a top level field withheld",
			zoneJSON: `{"name": "test", "sync_token": "` + testUnknownSecret + `"}`,
			marked:   true,
		},
		{
			name:     "a nested field withheld",
			zoneJSON: `{"name": "test", "placement_pools": [{"key": "p", "sync_token": "` + testUnknownSecret + `"}]}`,
			marked:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			redacted := RedactZoneJSON(tc.zoneJSON)

			assert.NotContains(t, redacted, testUnknownSecret)
			if tc.marked {
				assert.Contains(t, redacted, fmt.Sprintf("%q:1", withheldMarker))
			} else {
				assert.NotContains(t, redacted, withheldMarker)
			}
		})
	}
}

func TestRedactZoneJSONCountsWithheldFieldsItself(t *testing.T) {
	// the marker reports what the filter withheld and nothing the document says: every name in
	// the output comes from the allow list, which does not name the marker, so a document
	// claiming its own count loses it
	redacted := RedactZoneJSON(fmt.Sprintf(`{"name": "test", %q: 99, "sync_token": %q}`, withheldMarker, testUnknownSecret))

	assert.NotContains(t, redacted, testUnknownSecret)
	assert.NotContains(t, redacted, "99")
	assert.JSONEq(t, fmt.Sprintf(`{"name": "test", %q: 2}`, withheldMarker), redacted)
}

func TestRedactZoneJSONWithholdsUnexpectedShapes(t *testing.T) {
	// a field the filter permits as a pool name, arriving as an object, is a shape the allow
	// list cannot vouch for - a credential hides in a shape as readily as under a name
	tests := []struct {
		name     string
		zoneJSON string
	}{
		{
			name:     "a permitted pool field holding an object",
			zoneJSON: `{"name": "test", "domain_root": {"secret_key": "` + testUnknownSecret + `"}}`,
		},
		{
			name:     "a permitted pool field holding an array",
			zoneJSON: `{"name": "test", "log_pool": ["` + testUnknownSecret + `"]}`,
		},
		{
			name:     "placement_pools holding an object",
			zoneJSON: `{"name": "test", "placement_pools": {"secret_key": "` + testUnknownSecret + `"}}`,
		},
		{
			name:     "a placement_pools element that is not an object",
			zoneJSON: `{"name": "test", "placement_pools": ["` + testUnknownSecret + `"]}`,
		},
		{
			name:     "a storage class holding a string",
			zoneJSON: `{"name": "test", "placement_pools": [{"val": {"storage_classes": {"STANDARD": "` + testUnknownSecret + `"}}}]}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var redacted string
			require.NotPanics(t, func() { redacted = RedactZoneJSON(tc.zoneJSON) })

			assert.NotContains(t, redacted, testUnknownSecret)
			assert.Contains(t, redacted, withheldMarker)
			// the rest of the document survives the one field that did not
			assert.Contains(t, redacted, `"name":"test"`)
		})
	}
}

func TestRedactZoneJSONIsTotal(t *testing.T) {
	// A payload that does not decode as a JSON object is never passed through. Truncation in
	// particular can end part way through a credential, where a textual match would miss it.
	tests := []struct {
		name    string
		payload string
	}{
		{
			name:    "truncated mid document",
			payload: testZoneJSON[:strings.Index(testZoneJSON, testZoneSecretKey)+10],
		},
		{
			name:    "truncated mid credential",
			payload: `{"name": "test", "system_key": {"secret_key": "` + testZoneSecretKey,
		},
		{
			name:    "trailing comma",
			payload: `{"name": "test", "system_key": {"secret_key": "` + testZoneSecretKey + `"},}`,
		},
		{
			name:    "stderr appended to the document",
			payload: testZoneJSON + "\n2026-11-02 00:00:00.000 7f0 -1 failed to connect to the cluster",
		},
		{
			name:    "stderr prepended to the document",
			payload: "2026-11-02 00:00:00.000 7f0 -1 unable to load config\n" + testZoneJSON,
		},
		{
			name:    "an array instead of an object",
			payload: `["` + testZoneSecretKey + `"]`,
		},
		{
			name:    "a bare null",
			payload: "null",
		},
		{
			name:    "a bare string",
			payload: `"` + testZoneSecretKey + `"`,
		},
		{
			name:    "not json at all",
			payload: "failed to connect to the cluster: " + testZoneSecretKey,
		},
		{
			name:    "empty",
			payload: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var redacted string
			require.NotPanics(t, func() { redacted = RedactZoneJSON(tc.payload) })

			assert.NotContains(t, redacted, testZoneSecretKey)
			assert.NotContains(t, redacted, testZoneAccessKey)
			assert.NotContains(t, redacted, testTierSecret)
			assert.NotContains(t, redacted, testUnknownSecret)
			// a prefix of the secret is still a lead on it
			assert.NotContains(t, redacted, testZoneSecretKey[:10])
			// the size is the only context left
			assert.Contains(t, redacted, fmt.Sprintf("(%d bytes)", len(tc.payload)))
		})
	}
}
