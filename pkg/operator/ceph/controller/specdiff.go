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
	"fmt"
	"reflect"
	"strings"

	"github.com/google/go-cmp/cmp"
	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
)

var sensitiveType = reflect.TypeOf(cephv1.Sensitive(""))

// specDiffReporter renders which spec fields changed, withholding the value of
// any field typed cephv1.Sensitive.
//
// It exists because cmp.Diff cannot be made to withhold anything. go-cmp's own
// reporter walks the value tree and formats each leaf itself, consulting no
// String, GoString or Format method, so a type that redacts under fmt is
// printed in full by cmp.Diff regardless. The options that do suppress a field
// -- cmp.Transformer and cmpopts.IgnoreFields -- suppress it from the
// comparison as well, which here would be a correctness bug rather than a
// formatting choice: the caller uses inequality to decide whether to
// reconcile, so a hidden field would stop a credential rotation from
// triggering one. Replacing the reporter leaves equality untouched.
type specDiffReporter struct {
	path  cmp.Path
	lines []string
}

func (r *specDiffReporter) PushStep(ps cmp.PathStep) { r.path = append(r.path, ps) }

func (r *specDiffReporter) PopStep() { r.path = r.path[:len(r.path)-1] }

func (r *specDiffReporter) Report(rs cmp.Result) {
	if rs.Equal() {
		return
	}

	vx, vy := r.path.Last().Values()
	r.lines = append(r.lines, fmt.Sprintf("%s: %s => %s", r.path, renderSpecValue(vx), renderSpecValue(vy)))
}

// String reports the changed fields, one per line, or a fixed note when the
// values compared unequal without the reporter being given a leaf to name --
// so that an empty result never reads as "nothing changed".
func (r *specDiffReporter) String() string {
	if len(r.lines) == 0 {
		return "(changed, no field reported)"
	}

	return strings.Join(r.lines, "; ")
}

// renderSpecValue formats one side of a difference. A Sensitive is reported by
// length alone.
//
// The type test is deliberate rather than relying on Sensitive's own Format
// method: it keeps the reporter correct for a value reached in a way fmt would
// not protect, and states at this one site which types are withheld.
//
// %v is also deliberate. Under %s or %q, fmt rejects a pointer, sets its
// erroring flag and then renders the subtree with every Format and String
// method skipped, so a Sensitive behind a pointer field would print in the
// clear. %v accepts pointers and consults methods normally.
func renderSpecValue(v reflect.Value) string {
	if !v.IsValid() {
		return "<absent>"
	}

	if v.Type() == sensitiveType {
		// the type's own marker, so one shape appears in every channel
		return cephv1.Sensitive(v.String()).String()
	}

	return fmt.Sprintf("%v", v)
}

// SpecChanged reports whether two specs differ and, when they do, a rendering
// of what changed that no sensitive field's value reaches.
func SpecChanged(old, new any, opts ...cmp.Option) (bool, string) {
	var reporter specDiffReporter
	equal := cmp.Equal(old, new, append(opts, cmp.Reporter(&reporter))...)
	if equal {
		return false, ""
	}

	return true, reporter.String()
}
