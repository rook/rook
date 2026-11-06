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

package controller

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/resource"
)

// a made-up passphrase; nothing real is involved
const specDiffSecret = "EXAMPLEPASSPHRASE0001"

func topicSpec(uri, exchange string) cephv1.BucketTopicSpec {
	return cephv1.BucketTopicSpec{
		ObjectStoreName: "my-store",
		Endpoint: cephv1.TopicEndpointSpec{
			AMQP: &cephv1.AMQPEndpointSpec{
				URI:      cephv1.Sensitive(uri),
				Exchange: exchange,
			},
		},
	}
}

var quantityComparer = cmp.Comparer(func(x, y resource.Quantity) bool { return x.Cmp(y) == 0 })

// TestSpecChangedWithholdsSensitiveValues is the case the reporter exists for:
// the endpoint URI carries a passphrase, it sits behind a pointer field, and a
// rotation changes it on both sides.
func TestSpecChangedWithholdsSensitiveValues(t *testing.T) {
	old := topicSpec("amqps://user:"+specDiffSecret+"@broker/vhost", "ex")
	new := topicSpec("amqps://user:"+specDiffSecret+"9@broker/vhost", "ex")

	changed, diff := SpecChanged(old, new, quantityComparer)

	// the change must still be seen, or the operator would not reconcile a rotation
	assert.True(t, changed, "a changed URI must still register as a spec change")
	assert.NotContains(t, diff, specDiffSecret, "the passphrase reached the log line")
	assert.NotContains(t, diff, "broker", "the rest of the URI reached the log line")
	assert.Contains(t, diff, "URI", "the reader cannot tell which field changed")
	assert.Contains(t, diff, "_REDACTED-", "no marker was rendered")
}

// TestSpecChangedKeepsOrdinaryValues checks the reporter did not buy safety by
// making every diff useless.
func TestSpecChangedKeepsOrdinaryValues(t *testing.T) {
	uri := "amqps://user:" + specDiffSecret + "@broker/vhost"
	changed, diff := SpecChanged(topicSpec(uri, "before"), topicSpec(uri, "after"), quantityComparer)

	assert.True(t, changed)
	assert.Contains(t, diff, "before")
	assert.Contains(t, diff, "after")
	assert.NotContains(t, diff, specDiffSecret)
}

// TestSpecChangedReportsNoChange pins the half the reconcile decision rests on.
func TestSpecChangedReportsNoChange(t *testing.T) {
	s := topicSpec("amqps://user:"+specDiffSecret+"@broker/vhost", "ex")

	changed, diff := SpecChanged(s, s, quantityComparer)

	assert.False(t, changed, "identical specs must not trigger a reconcile")
	assert.Empty(t, diff)
}

// TestSpecChangedDetectsSecretOnlyChange is the regression that cmp.Transformer
// and cmpopts.IgnoreFields would both fail: they withhold the field from the
// comparison as well as the report, so a rotation of nothing but the secret
// compares equal and no reconcile is queued.
func TestSpecChangedDetectsSecretOnlyChange(t *testing.T) {
	old := topicSpec("amqps://user:"+specDiffSecret+"@broker/vhost", "ex")
	new := topicSpec("amqps://user:"+specDiffSecret+"-rotated@broker/vhost", "ex")

	changed, diff := SpecChanged(old, new, quantityComparer)

	assert.True(t, changed, "a secret-only rotation must trigger a reconcile")
	assert.NotContains(t, diff, specDiffSecret)
	// both sides are withheld, and their lengths differ, so a reader can still
	// tell that the value changed rather than only that the field was touched
	assert.Contains(t, diff, "=>")
}

// TestPlainCmpDiffDoesNotLeakSensitive pins the property the marker's shape
// exists for, on the path that bypasses SpecChanged entirely.
//
// go-cmp's reporter uses a String method only when the rendered forms differ,
// because it is choosing the representation that shows a difference. That makes
// the marker's shape load-bearing for every cmp.Diff in the tree, not just this
// package's: a constant marker is discarded and the value printed in full, and a
// length-bearing one is discarded whenever two lengths coincide -- which is what
// a rotation that keeps the key size produces. A keyed digest always differs, so
// the method is always used.
//
// If this test ever fails, the marker has been made constant or value-derived
// and every unguarded cmp.Diff over a spec has started leaking.
func TestPlainCmpDiffDoesNotLeakSensitive(t *testing.T) {
	// deliberately the same length, which is the case that defeats a
	// length-bearing marker. Both are built from the fixture constant rather
	// than written as literals, because gosec reads a password inside a URL
	// literal as a hardcoded credential.
	a := "amqps://user:" + specDiffSecret + "A@broker/vhost"
	b := "amqps://user:" + specDiffSecret + "B@broker/vhost"
	require.Equal(t, len(a), len(b), "precondition: the two values are the same length")

	out := cmp.Diff(topicSpec(a, "ex"), topicSpec(b, "ex"), quantityComparer)

	assert.NotContains(t, out, "AAAAAAAAAAAAAAAA", "plain cmp.Diff leaked the old value")
	assert.NotContains(t, out, "BBBBBBBBBBBBBBBB", "plain cmp.Diff leaked the new value")
	assert.NotEmpty(t, out, "the change must still be visible to cmp.Diff")
}

// TestSpecChangedOnNilToSetEndpoint covers the shape where the differing leaf is
// the pointer itself rather than the string: the whole endpoint appears or goes
// away, so the struct is formatted rather than the Sensitive field alone.
func TestSpecChangedOnNilToSetEndpoint(t *testing.T) {
	old := cephv1.BucketTopicSpec{ObjectStoreName: "my-store"}
	new := topicSpec("amqps://user:"+specDiffSecret+"@broker/vhost", "ex")

	changed, diff := SpecChanged(old, new, quantityComparer)

	assert.True(t, changed)
	assert.NotContains(t, diff, specDiffSecret, "the passphrase leaked when the whole endpoint was added")
}
