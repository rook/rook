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

package leakguard

import (
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes/fake"
)

const (
	knownValue = "a-secret-in-a-known-leak"
	rgwLog     = "logs/rook-ceph_rook-ceph-rgw-a-1_rgw_0.log"
	// unmarkedText is on a marked line but is no marker, so a report that
	// holds it printed the line.
	unmarkedText = "uid=someone"
)

var rgwAccessLog = KnownLeak{
	Name:    "rgw-access-log",
	Reason:  "RGW logs the admin ops request line.",
	What:    []string{"registered/other", "registered/rgw-*"},
	File:    "logs/*_rgw_*.log",
	Markers: [][]string{{" beast: ", `"PUT /admin/user?`}, {"set_mon_vals "}},
}

// rgwLogLines are a capture of rgwLog: lines 1 and 4 hold the markers of one
// of rgwAccessLog's marker sets, line 2 none and line 3 only one marker of a
// set.
var rgwLogLines = []string{
	`debug 1 beast: 0x1: - "PUT /admin/user?` + unmarkedText + `&secret-key=` + knownValue + `" 200`,
	`secret-key=` + knownValue,
	`debug 1 beast: 0x1: - "GET /admin/user?secret-key=` + knownValue + `" 200`,
	`debug 10 set_mon_vals rgw_keystone_admin_password = ` + knownValue,
}

func writeCapture(t *testing.T, dir, rel string, lines ...string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o700))
	require.NoError(t, os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o600))
}

func lineNumbers(findings []Finding) []int {
	out := make([]int, 0, len(findings))
	for _, f := range findings {
		out = append(out, f.Location.Line)
	}
	return out
}

func knownAt(leak, what, file string, lines ...int) []KnownFinding {
	var known []KnownFinding
	for _, f := range findingsAt(what, file, lines...) {
		known = append(known, KnownFinding{Finding: f, Leak: leak})
	}
	return known
}

// checkKnown checks a guard started with known after planting rgwLogLines,
// whose secret is registered as rgw-key.
func checkKnown(t *testing.T, known ...KnownLeak) (Result, string) {
	t.Helper()
	g, dir := startKnownGuard(t, fake.NewClientset(runningPod("mon")), known, guardNS)
	g.Register("rgw-key", knownValue)
	requireLogFile(t, logPath(dir, guardNS, "mon", "main", 0), fakeLogs)
	writeCapture(t, dir, rgwLog, rgwLogLines...)
	return check(t, g, canned(authLs)), dir
}

func TestKnownLeakNeedsWhatFileAndMarkers(t *testing.T) {
	otherWhat := rgwAccessLog
	otherWhat.What = []string{"registered/other", "registered/rgw"}
	otherFile := rgwAccessLog
	otherFile.File = "logs/*_mon_*.log"

	tests := []struct {
		name      string
		leak      KnownLeak
		wantKnown []int
		wantExact []int
	}{
		{name: "lines with every marker of a set", leak: rgwAccessLog, wantKnown: []int{1, 4}, wantExact: []int{2, 3}},
		{name: "What matching no glob", leak: otherWhat, wantExact: []int{1, 2, 3, 4}},
		{name: "File not matching", leak: otherFile, wantExact: []int{1, 2, 3, 4}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := checkKnown(t, tt.leak)

			require.NoError(t, r.Err)
			assert.Equal(t, knownAt(tt.leak.Name, sourceRegistered+"/rgw-key", rgwLog, tt.wantKnown...), r.Known)
			assert.Equal(t, tt.wantExact, lineNumbers(r.Exact))
			assert.True(t, r.Failed(ModeFail), "a secret on a line without the markers still fails")
		})
	}
}

func TestKnownFindingsNeverFail(t *testing.T) {
	g, dir := startKnownGuard(t, fake.NewClientset(runningPod("mon")), []KnownLeak{rgwAccessLog}, guardNS)
	g.Register("rgw-key", knownValue)
	requireLogFile(t, logPath(dir, guardNS, "mon", "main", 0), fakeLogs)
	writeCapture(t, dir, rgwLog, rgwLogLines[0], rgwLogLines[3])

	r := check(t, g, canned(authLs))

	require.NoError(t, r.Err)
	assert.Empty(t, r.Exact)
	assert.Equal(t, knownAt(rgwAccessLog.Name, sourceRegistered+"/rgw-key", rgwLog, 1, 2), r.Known)
	assert.False(t, r.Failed(ModeFail))
	assert.False(t, r.Failed(ModeReport))
}

func TestUnusedKnownLeaksAreReportedNotFailing(t *testing.T) {
	unused := rgwAccessLog
	unused.Name = "fixed-leak"
	unused.File = "logs/*_mgr_*.log"
	g, dir := startKnownGuard(t, fake.NewClientset(runningPod("mon")), []KnownLeak{unused, rgwAccessLog}, guardNS)
	g.Register("rgw-key", knownValue)
	requireLogFile(t, logPath(dir, guardNS, "mon", "main", 0), fakeLogs)
	writeCapture(t, dir, rgwLog, rgwLogLines[0])

	r := check(t, g, canned(authLs))

	require.NoError(t, r.Err)
	assert.Len(t, r.Known, 1)
	assert.False(t, r.Failed(ModeFail))
	assert.Contains(t, r.Report(), "unused known leaks, reported only: 1\n  fixed-leak\npattern findings")
}

func TestKnownFindingsAreListedWithoutValues(t *testing.T) {
	r, dir := checkKnown(t, rgwAccessLog)

	require.NoError(t, r.Err)
	findingsFile := filepath.Join(dir, "findings.txt")
	written, err := os.ReadFile(findingsFile)
	require.NoError(t, err)
	report := r.Report()
	for _, out := range []string{report, string(written)} {
		assert.Contains(t, out, "known-leak findings, reported only: 2\n  rgw-access-log: "+rgwAccessLog.Reason+"\n")
		assert.Contains(t, out, "at "+rgwLog+":1\n")
		assert.Contains(t, out, "at "+rgwLog+":4\n")
		assert.Contains(t, out, "unused known leaks, reported only: 0\n")
		assert.NotContains(t, out, knownValue)
		assert.NotContains(t, out, unmarkedText)
	}
	assert.Contains(t, report, "every finding is listed in "+findingsFile)
}

func TestReportGroupsKnownFindings(t *testing.T) {
	other := KnownLeak{Name: "other-leak", Reason: "Another reason."}
	unused := KnownLeak{Name: "unused-leak", Reason: "Fixed."}
	var known []KnownFinding
	known = append(known, knownAt(rgwAccessLog.Name, "registered/rgw-key", "logs/r.log", 1, 2, 3, 4)...)
	known = append(known, knownAt(other.Name, "registered/other", "logs/o.log", 1)...)
	r := Result{
		Known:          known,
		KnownLeaks:     []KnownLeak{rgwAccessLog, other, unused},
		Followers:      1,
		ClusterSecrets: 1,
		FindingsFile:   "/out/findings.txt",
	}

	assert.Equal(t, `leak guard: 1 container logs followed, 0 secrets searched for, 1 of them collected from the cluster
exact-match findings, failing unless LEAK_GUARD_MODE=report: 0
known-leak findings, reported only: 5
  rgw-access-log: RGW logs the admin ops request line.
    registered/rgw-key: 4 lines in 1 file
      at logs/r.log:1
      at logs/r.log:2
      at logs/r.log:3
      and 1 more
  other-leak: Another reason.
    registered/other: 1 line in 1 file
      at logs/o.log:1
unused known leaks, reported only: 1
  unused-leak
pattern findings, reported only: 0
every finding is listed in /out/findings.txt
`, r.Report())
	assert.Contains(t, r.render(-1), "      at logs/r.log:4\n")
}

func TestKnownLeakValidate(t *testing.T) {
	require.NoError(t, rgwAccessLog.Validate())

	tests := []struct {
		name       string
		edit       func(*KnownLeak)
		badPattern bool
	}{
		{name: "no name", edit: func(k *KnownLeak) { k.Name = "" }},
		{name: "no reason", edit: func(k *KnownLeak) { k.Reason = "" }},
		{name: "no What", edit: func(k *KnownLeak) { k.What = nil }},
		{name: "malformed What", edit: func(k *KnownLeak) { k.What = []string{"registered/[rgw"} }, badPattern: true},
		{name: "no File", edit: func(k *KnownLeak) { k.File = "" }},
		{name: "malformed File", edit: func(k *KnownLeak) { k.File = "logs/[rgw" }, badPattern: true},
		{name: "no markers", edit: func(k *KnownLeak) { k.Markers = nil }},
		{name: "empty marker set", edit: func(k *KnownLeak) { k.Markers = [][]string{{" beast: "}, {}} }},
		{name: "empty marker", edit: func(k *KnownLeak) { k.Markers = [][]string{{" beast: ", ""}} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := rgwAccessLog
			tt.edit(&k)

			err := k.Validate()

			require.Error(t, err)
			assert.Equal(t, tt.badPattern, errors.Is(err, path.ErrBadPattern))
		})
	}
}

func TestGuardFailsOnInvalidKnownLeak(t *testing.T) {
	invalid := rgwAccessLog
	invalid.File = "logs/[rgw"

	r, _ := checkKnown(t, invalid)

	require.Error(t, r.Err)
	assert.ErrorIs(t, r.Err, path.ErrBadPattern)
	assert.Contains(t, r.Err.Error(), rgwAccessLog.Name)
	assert.Empty(t, r.KnownLeaks)
	assert.Empty(t, r.Known)
	assert.Equal(t, []int{1, 2, 3, 4}, lineNumbers(r.Exact))
}

func TestGuardFailsOnDuplicateKnownLeak(t *testing.T) {
	duplicate := rgwAccessLog
	duplicate.File = "logs/*_mon_*.log"

	r, _ := checkKnown(t, rgwAccessLog, duplicate)

	require.Error(t, r.Err)
	assert.ErrorContains(t, r.Err, `known leak "rgw-access-log" is listed more than once`)
	require.Len(t, r.KnownLeaks, 1)
	assert.Equal(t, rgwAccessLog.File, r.KnownLeaks[0].File, "the first entry of a name is kept")
	assert.Equal(t, knownAt(rgwAccessLog.Name, sourceRegistered+"/rgw-key", rgwLog, 1, 4), r.Known)
}

func TestKnownLeakUnreadableLineStaysExact(t *testing.T) {
	g := &Guard{dir: t.TempDir(), known: []KnownLeak{rgwAccessLog}}
	findings := findingsAt(sourceRegistered+"/rgw-key", rgwLog, 1)

	exact, known, err := g.matchKnown(findings)

	require.Error(t, err)
	assert.ErrorContains(t, err, "re-read "+rgwLog+": ")
	assert.Equal(t, findings, exact)
	assert.Empty(t, known)
}

func TestKnownLeakMatches(t *testing.T) {
	f := findingsAt(sourceRegistered+"/rgw-key", rgwLog, 1)[0]
	assert.True(t, rgwAccessLog.Matches(f, []byte(rgwLogLines[0])))
	assert.True(t, rgwAccessLog.Matches(f, []byte(rgwLogLines[3])))
	assert.False(t, rgwAccessLog.Matches(f, []byte(rgwLogLines[1])))
	assert.False(t, rgwAccessLog.Matches(f, []byte(rgwLogLines[2])))
	f.Location.File = "logs/rook-ceph_mon_main_0.log"
	assert.False(t, rgwAccessLog.Matches(f, []byte(rgwLogLines[0])))
}
