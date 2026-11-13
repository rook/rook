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
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Sensitive is a spec string whose value must not reach a log, an error, a
// condition message or a Kubernetes event. A spec field is readable by whoever
// can read the custom resource; the operator log is readable by whoever holds
// pods/log on the operator deployment, which is a different and generally wider
// set, and a value logged on every change discloses both sides of a rotation
// rather than only the value in force. The type exists to keep the second
// audience from seeing what only the first is entitled to.
//
// It is a marker, not a container. Nothing here stops a caller writing
// string(s) wherever it likes, and that conversion is deliberately easy,
// because the reconciler has to use the real value. What the type buys is that
// the careless paths become findable: a go-cmp reporter, a lint rule or a test
// can key on the type, which a bare string gives no way to do.
//
// The validation markers belong on the type rather than on each field that
// uses it: controller-gen resolves a named type's schema separately, so a
// MinLength or MaxLength marker written beside the field is applied before that
// schema exists and is rejected as not textual. LabelValue above carries its
// markers the same way for the same reason.
// +kubebuilder:validation:MinLength=1
// +kubebuilder:validation:MaxLength=2048
type Sensitive string

// markerKey keys the marker's digest. It is generated once per process, so a
// marker is stable for the life of an operator and meaningless outside it.
var markerKey = func() []byte {
	k := make([]byte, sha256.Size)
	// As of Go 1.24 crypto/rand.Read never returns an error; it panics rather
	// than return a short read, so there is no degraded path to handle. That
	// matters here: an unkeyed fallback would be a predictable digest, which is
	// exactly the oracle this key exists to prevent.
	_, _ = rand.Read(k)

	return k
}()

// redactionMarker renders "_REDACTED-<8 hex of a keyed digest>_", or
// "_REDACTED-empty_" for an empty value.
//
// Three properties are wanted and only this shape has all of them.
//
// It must not describe the value. A length tells an attacker which kind of
// credential this is -- 44 bytes is base64 of 32, 40 is hex of 20, 20 is an
// access key id -- and this field holds a whole URI, so anyone who knows the
// scheme, user and host subtracts and learns the passphrase's length exactly.
//
// It must differ between different values, because go-cmp decides whether to
// use a String method by whether the rendered forms differ: it is choosing the
// representation that shows a difference. Two values rendering the same marker
// make the method useless to it, so it discards it and prints both values in
// full. A constant marker therefore leaks under cmp.Diff always, and a
// length-bearing one leaks whenever two lengths coincide -- which is what a
// rotation that keeps the key size does, so it fails in the case that matters.
//
// It should be stable, so a reader can tell that two lines concern the same
// secret, and so that nothing downstream breaks by assuming formatting is pure.
//
// An unkeyed digest would satisfy the first two and fail the third's spirit: it
// is an oracle, letting a guessed value be confirmed offline, and it is a
// permanent handle that correlates the same credential across clusters, tenants
// and log archives. Truncating it does not help, because the width has to
// exceed the value's entropy to avoid collisions and fall below it to avoid
// identifying the value, and a user-chosen passphrase leaves no room between.
//
// Keying it with a per-process secret satisfies all three. Within one operator
// the marker is stable; outside it nothing can compute or correlate it, and the
// key dies with the process. Emptiness is reported because an unset field is
// the question an operator actually asks, and an empty value has nothing to
// disclose.
func (s Sensitive) redactionMarker() string {
	if len(s) == 0 {
		return "_REDACTED-empty_"
	}

	mac := hmac.New(sha256.New, markerKey)
	mac.Write([]byte(s))

	return "_REDACTED-" + hex.EncodeToString(mac.Sum(nil))[:8] + "_"
}

// String withholds the value. fmt consults it for %v, %s and %q, and for a
// struct holding a Sensitive in an exported field by value.
func (s Sensitive) String() string { return s.redactionMarker() }

// GoString withholds it under %#v, which String does not cover.
func (s Sensitive) GoString() string { return s.redactionMarker() }

// Format withholds it under every remaining verb. String and GoString together
// still leave %x, %X and %d, each of which renders a string's bytes in full,
// and fmt consults Formatter ahead of both.
//
// %T and %p are the two verbs no method can intercept: fmt answers them from
// the argument's type and address before it looks for a Formatter. Neither
// discloses the value -- %T renders "v1.Sensitive", which tells a reader what
// they are looking at -- so there is nothing to withhold there.
func (s Sensitive) Format(f fmt.State, verb rune) {
	fmt.Fprint(f, s.redactionMarker())
}

// Equal reports whether two values are the same. github.com/google/go-cmp uses
// it for comparison, so a change to a Sensitive field is still detected even
// though the reporter will not print either side. Reflection would compare it
// correctly without this; it is declared so the comparison path is explicit
// rather than incidental.
func (s Sensitive) Equal(other Sensitive) bool { return string(s) == string(other) }

// Sensitive deliberately does NOT implement json.Marshaler or
// encoding.TextMarshaler. A spec field has to round-trip: the apiserver stores
// what the user applied, the operator reads it back, and the reconciler uses
// it. Every off-the-shelf sensitive-string package surveyed for this redacts in
// MarshalJSON, which is right for a config struct read once at startup and
// wrong here, since it would write an empty or marker value back over the
// user's own URI. Encoding keeps the value; only formatting withholds it.
var _ fmt.Formatter = Sensitive("")
