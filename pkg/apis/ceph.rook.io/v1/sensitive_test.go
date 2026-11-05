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

package v1

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// a made-up value; nothing real is involved
const sensitiveTestURI = "amqps://user:EXAMPLEPASSPHRASE0001@broker.example.com/vhost"

// TestSensitiveWithholdsUnderEveryVerb covers the verbs individually because
// the three methods cover different sets: String reaches %v, %s and %q,
// GoString only %#v, and Formatter everything else. %x and %d are the ones a
// String-only type still renders in full.
func TestSensitiveWithholdsUnderEveryVerb(t *testing.T) {
	s := Sensitive(sensitiveTestURI)

	for _, verb := range []string{"%v", "%+v", "%s", "%q", "%#v", "%x", "%X", "%d", "%08s", "%-20s"} {
		out := fmt.Sprintf(verb, s)
		assert.NotContains(t, out, "EXAMPLEPASSPHRASE0001", "verb %s leaked the value", verb)
		assert.NotContains(t, out, "broker.example.com", "verb %s leaked the host", verb)
		assert.Contains(t, out, "_REDACTED-", "verb %s did not render the marker", verb)
	}

	// %T is one of the two verbs no method can intercept: fmt answers it from the
	// argument's type before it looks for a Formatter. It names the type rather
	// than the value, which discloses nothing and tells a reader what this is.
	assert.Equal(t, "v1.Sensitive", fmt.Sprintf("%T", s))
}

// TestSensitiveInStructWithholds covers the shape the spec actually has: the
// value sits in an exported field of a struct that something formats whole.
func TestSensitiveInStructWithholds(t *testing.T) {
	type endpoint struct {
		URI      Sensitive
		Exchange string
	}

	for _, verb := range []string{"%v", "%+v", "%#v", "%s"} {
		out := fmt.Sprintf(verb, endpoint{URI: Sensitive(sensitiveTestURI), Exchange: "ex"})
		assert.NotContains(t, out, "EXAMPLEPASSPHRASE0001", "verb %s leaked through the struct", verb)
		assert.Contains(t, out, "ex", "verb %s dropped the non-sensitive field", verb)
	}
}

// TestSensitiveRoundTripsThroughJSON is the one that keeps this usable in an
// API type. Every off-the-shelf sensitive-string package redacts in
// MarshalJSON, which for a spec field would write the marker back over the
// user's value: the apiserver would store the placeholder, and the next read
// would hand the reconciler a URI it cannot connect to, with no error anywhere.
func TestSensitiveRoundTripsThroughJSON(t *testing.T) {
	type endpoint struct {
		URI Sensitive `json:"uri"`
	}

	encoded, err := json.Marshal(endpoint{URI: Sensitive(sensitiveTestURI)})
	require.NoError(t, err)

	// the stored form is the real value, byte for byte
	assert.JSONEq(t, `{"uri":"`+sensitiveTestURI+`"}`, string(encoded))
	assert.NotContains(t, string(encoded), "_REDACTED-")

	var decoded endpoint
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	assert.Equal(t, sensitiveTestURI, string(decoded.URI))

	// and the value the reconciler reads is usable, while formatting it is not
	assert.Equal(t, sensitiveTestURI, string(decoded.URI))
	assert.NotContains(t, fmt.Sprintf("%v", decoded.URI), "EXAMPLEPASSPHRASE0001")
}

func TestSensitiveEqual(t *testing.T) {
	a := Sensitive(sensitiveTestURI)
	b := Sensitive(sensitiveTestURI)
	c := Sensitive(sensitiveTestURI + "x")

	assert.True(t, a.Equal(b))
	assert.False(t, a.Equal(c))

	// the marker must not be what equality compares
	assert.False(t, Sensitive("aaaa").Equal(Sensitive("bbbb")), "equality must read the value, not the marker")
}

// TestSensitiveMarkerIsStableAndDistinct pins the three properties the marker's
// shape is chosen for. Each one rules out an alternative: a marker that
// described the value would leak its length, a constant one would make go-cmp
// discard the method and print the value in full, and an unstable one would
// make the rendered form useless for telling two log lines apart.
func TestSensitiveMarkerIsStableAndDistinct(t *testing.T) {
	const uri = "amqps://user:EXAMPLEPASSPHRASE0001@broker.example.com/vhost"

	// stable: the same value renders the same marker within a process
	assert.Equal(t, Sensitive(uri).String(), Sensitive(uri).String())

	// distinct: two different values render different markers, including when
	// they are the same length -- the case a length-bearing marker gets wrong,
	// and the case a rotation that keeps the key size produces
	a := Sensitive("amqps://u:AAAAAAAAAAAAAAAA@host/v")
	b := Sensitive("amqps://u:BBBBBBBBBBBBBBBB@host/v")
	require.Equal(t, len(a), len(b), "precondition: the two values are the same length")
	assert.NotEqual(t, a.String(), b.String(), "equal-length values must still render differently")

	// and the marker discloses no length
	long := Sensitive(strings.Repeat("x", 67))
	assert.NotContains(t, long.String(), "67")
	assert.Equal(t, len(Sensitive("x").String()), len(long.String()), "marker length must not vary with the value")

	// an empty value is reported as empty, which is the one property an operator
	// asks of a spec field and which discloses nothing
	assert.Equal(t, "_REDACTED-empty_", Sensitive("").String())
}
