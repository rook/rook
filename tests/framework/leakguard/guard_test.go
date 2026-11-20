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
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	rookfake "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

const (
	guardNS     = "rook-ceph"
	emptyAuthLs = `{"auth_dump":[]}`
)

func startTestGuard(t *testing.T, cs *fake.Clientset, namespaces ...string) (*Guard, string) {
	t.Helper()
	dir := t.TempDir()
	g := Start(context.Background(), cs, dir, namespaces...)
	t.Cleanup(g.follower.Stop)
	return g, dir
}

func runningPod(name string) *corev1.Pod {
	return testPod(guardNS, name, nil, []testContainer{{name: "main", state: running}})
}

// check prepares g with run as the toolbox of guardNS, then checks it.
func check(t *testing.T, g *Guard, run ToolboxRunner) Result {
	t.Helper()
	g.Prepare(context.Background(), rookfake.NewSimpleClientset(), run, []string{guardNS})
	return g.Check()
}

func canned(stdout string) ToolboxRunner {
	var ran ranCommand
	return cannedRunner(stdout, nil, &ran)
}

func findingNames(findings []Finding) []string {
	out := make([]string, 0, len(findings))
	for _, f := range findings {
		out = append(out, f.What)
	}
	return out
}

func TestGuardEndToEnd(t *testing.T) {
	cs := fake.NewClientset(
		runningPod("osd-prepare"),
		newSecret(guardNS, "creds", map[string]string{"token": storedValue}),
	)
	g, dir := startTestGuard(t, cs, guardNS)
	g.Register("planted", fakeLogs)
	requireLogFile(t, logPath(dir, guardNS, "osd-prepare", "main", 0), fakeLogs)

	r := check(t, g, canned(authLs))

	require.NoError(t, r.Err)
	assert.Equal(t, 1, r.Followers)
	assert.Equal(t, 5, r.Secrets, "3 cephx keys, 1 Kubernetes Secret value, 1 registered")
	assert.Equal(t, 4, r.ClusterSecrets, "3 cephx keys, 1 Kubernetes Secret value")
	assert.Empty(t, r.EmptyCephx)
	require.Len(t, r.Exact, 1)
	assert.Equal(t, NewExactMatch(nil).Name(), r.Exact[0].Detector)
	assert.Equal(t, sourceRegistered+"/planted", r.Exact[0].What)
	assert.Equal(t, Location{File: "logs/rook-ceph_osd-prepare_main_0.log", Line: 1}, r.Exact[0].Location)
	assert.Empty(t, r.Pattern)
	assert.True(t, r.Failed(ModeFail))
	assert.False(t, r.Failed(ModeReport))

	report := r.Report()
	assert.Contains(t, report, "registered/planted")
	assert.Contains(t, report, "logs/rook-ceph_osd-prepare_main_0.log:1")
	findingsFile := filepath.Join(dir, "findings.txt")
	assert.Equal(t, findingsFile, r.FindingsFile)
	assert.Contains(t, report, "every finding is listed in "+findingsFile)
	written, err := os.ReadFile(findingsFile)
	require.NoError(t, err)
	assert.Contains(t, string(written), "registered/planted")
	assert.Contains(t, string(written), "logs/rook-ceph_osd-prepare_main_0.log:1")
	for _, value := range []string{fakeLogs, storedValue, osd0Key, adminKey, lockboxKey} {
		assert.NotContains(t, report, value)
		assert.NotContains(t, string(written), value)
	}
}

func findingsAt(what string, file string, lines ...int) []Finding {
	findings := make([]Finding, 0, len(lines))
	for _, line := range lines {
		findings = append(findings, Finding{Detector: "exact", What: what, Location: Location{File: file, Line: line}})
	}
	return findings
}

func TestReportGroupsFindings(t *testing.T) {
	var exact []Finding
	exact = append(exact, findingsAt("registered/noisy", "logs/a.log", 1, 2, 3, 4, 5)...)
	exact = append(exact, findingsAt("registered/noisy", "logs/b.log", 9)...)
	// One value held by two sources is found on the same lines under each.
	for _, line := range []int{7, 8} {
		exact = append(exact, findingsAt("k8s-secret/rook-ceph/rook-ceph-mon:admin-secret", "logs/c.log", line)...)
		exact = append(exact, findingsAt("cephx/client.admin", "logs/c.log", line)...)
	}
	exact = append(exact, findingsAt("cephx/osd.0", "logs/c.log", 8)...)
	r := Result{
		Exact:          exact,
		Followers:      1,
		ClusterSecrets: 1,
		FindingsFile:   "/out/findings.txt",
	}

	report := r.Report()
	assert.Equal(t, `leak guard: 1 container logs followed, 0 secrets searched for, 1 of them collected from the cluster
exact-match findings, failing unless LEAK_GUARD_MODE=report: 11
  cephx/client.admin, k8s-secret/rook-ceph/rook-ceph-mon:admin-secret: 2 lines in 1 file
    at logs/c.log:7
    at logs/c.log:8
  cephx/osd.0: 1 line in 1 file
    at logs/c.log:8
  registered/noisy: 6 lines in 2 files
    at logs/a.log:1
    at logs/a.log:2
    at logs/a.log:3
    and 3 more
pattern findings, reported only: 0
every finding is listed in /out/findings.txt
`, report)

	full := r.render(-1)
	for _, loc := range []string{"logs/a.log:4", "logs/a.log:5", "logs/b.log:9"} {
		assert.Contains(t, full, "at "+loc)
	}
	assert.NotContains(t, full, "more")
}

func TestReportOmitsFindingsFileWithoutFindings(t *testing.T) {
	r := Result{Followers: 1, ClusterSecrets: 1, FindingsFile: "/out/findings.txt"}
	assert.NotContains(t, r.Report(), "findings.txt")
}

func TestSummary(t *testing.T) {
	r := Result{
		Exact:   findingsAt("registered/x", "logs/a.log", 1, 2),
		Pattern: findingsAt("rook-cephx-key", "logs/a.log", 3),
	}
	assert.Equal(t, "leak guard: 2 exact-match findings, 1 pattern findings, 2 liveness failures", r.Summary())
	r.Err = errors.New("listing failed")
	assert.Equal(t, "leak guard: 2 exact-match findings, 1 pattern findings, 2 liveness failures, guard errors", r.Summary())
}

func TestGuardLivenessFailsWithoutFollowersOrSecrets(t *testing.T) {
	t.Run("no followers", func(t *testing.T) {
		g, _ := startTestGuard(t, fake.NewClientset(), guardNS)

		r := check(t, g, canned(authLs))

		require.NoError(t, r.Err)
		assert.Equal(t, 0, r.Followers)
		assert.Equal(t, 3, r.ClusterSecrets)
		assert.True(t, r.Failed(ModeFail))
		assert.True(t, r.Failed(ModeReport))
		assert.Contains(t, r.Report(), "no container log was followed")
	})
	t.Run("no cluster secrets", func(t *testing.T) {
		g, _ := startTestGuard(t, fake.NewClientset(runningPod("mon")), guardNS)
		g.Register("registered", "a registered value")
		g.Register("too short to search for", "short")

		g.Prepare(context.Background(), rookfake.NewSimpleClientset(), canned(authLs), nil)
		r := g.Check()

		require.NoError(t, r.Err)
		assert.Equal(t, 1, r.Followers)
		assert.Equal(t, 1, r.Secrets, "only the registered value long enough to search for")
		assert.Equal(t, 0, r.ClusterSecrets, "registered secrets are not collected from the cluster")
		assert.True(t, r.Failed(ModeFail))
		assert.True(t, r.Failed(ModeReport))
		assert.Contains(t, r.Report(), "no secret was collected from the cluster")
	})
	t.Run("toolbox without cephx keys", func(t *testing.T) {
		cs := fake.NewClientset(
			runningPod("mon"),
			newSecret(guardNS, "creds", map[string]string{"token": storedValue}),
		)
		g, _ := startTestGuard(t, cs, guardNS)

		r := check(t, g, canned(emptyAuthLs))

		require.NoError(t, r.Err)
		assert.Equal(t, 1, r.ClusterSecrets)
		assert.Equal(t, []string{guardNS}, r.EmptyCephx)
		assert.True(t, r.Failed(ModeFail))
		assert.True(t, r.Failed(ModeReport))
		assert.Contains(t, r.Report(), "ceph auth ls in namespace rook-ceph returned no keys")
	})
}

func TestPatternFindingsNeverFail(t *testing.T) {
	cs := fake.NewClientset(
		runningPod("mon"),
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Namespace: guardNS, Name: "leaky"},
			Data:       map[string]string{"keyring": fakeCephxKey(t, 1, 7)},
		},
	)
	g, _ := startTestGuard(t, cs, guardNS)

	r := check(t, g, canned(authLs))

	require.NoError(t, r.Err)
	assert.Empty(t, r.Exact)
	require.Len(t, r.Pattern, 1)
	assert.Equal(t, "rook-cephx-key", r.Pattern[0].What)
	assert.Equal(t, Location{File: "snapshots/rook-ceph_ConfigMap_leaky.txt", Line: 1}, r.Pattern[0].Location)
	assert.False(t, r.Failed(ModeFail))
	assert.False(t, r.Failed(ModeReport))
	assert.Contains(t, r.Report(), "rook-cephx-key")
	assert.NotContains(t, r.Report(), fakeCephxKey(t, 1, 7))
}

func TestExpectExcludesRegisteredFinding(t *testing.T) {
	g, dir := startTestGuard(t, fake.NewClientset(runningPod("mon")), guardNS)
	g.Register("x", fakeLogs)
	g.Register("y", fakeLogs[1:])
	g.Expect("x")
	requireLogFile(t, logPath(dir, guardNS, "mon", "main", 0), fakeLogs)

	r := check(t, g, canned(emptyAuthLs))

	require.NoError(t, r.Err)
	assert.Equal(t, []string{sourceRegistered + "/y"}, findingNames(r.Exact))
}

func TestCheckNowKeepsFollowing(t *testing.T) {
	cs := fake.NewClientset(runningPod("first"))
	g, dir := startTestGuard(t, cs, guardNS)
	g.Register("planted", fakeLogs)
	requireLogFile(t, logPath(dir, guardNS, "first", "main", 0), fakeLogs)

	r := g.CheckNow(context.Background(), rookfake.NewSimpleClientset(), canned(emptyAuthLs), []string{guardNS})
	require.NoError(t, r.Err)
	assert.Equal(t, 1, r.Followers)
	assert.Len(t, r.Exact, 1)

	createPod(t, cs, runningPod("second"))
	requireLogFile(t, logPath(dir, guardNS, "second", "main", 0), fakeLogs)

	r = check(t, g, canned(emptyAuthLs))
	require.NoError(t, r.Err)
	assert.Equal(t, 2, r.Followers)
	assert.Len(t, r.Exact, 2)
}

func TestGuardFollowsAddedNamespace(t *testing.T) {
	const ns = "external"
	cs := fake.NewClientset()
	g, dir := startTestGuard(t, cs, guardNS)
	g.AddNamespace(ns)
	pod := testPod(ns, "mon", nil, []testContainer{{name: "main", state: running}})
	pod.Spec.Containers[0].Env = []corev1.EnvVar{{Name: "TOKEN", Value: storedValue}}
	createPod(t, cs, pod)
	_, err := cs.CoreV1().Secrets(ns).Create(context.Background(),
		newSecret(ns, "creds", map[string]string{"token": storedValue}), metav1.CreateOptions{})
	require.NoError(t, err)

	requireLogFile(t, logPath(dir, ns, "mon", "main", 0), fakeLogs)
	requireSecret(t, g.follower, ns+"/creds:token", storedValue)
	r := check(t, g, canned(authLs))

	require.NoError(t, r.Err)
	require.Equal(t, []string{"k8s-secret/external/creds:token"}, findingNames(r.Exact))
	assert.Equal(t, Location{File: "specs/external_mon.txt", Line: 1}, r.Exact[0].Location)
}

// teardownToolbox is a toolbox that runs until gone is called, as the
// toolbox is deleted by the uninstall between Prepare and Check.
type teardownToolbox struct {
	mu   sync.Mutex
	gone bool
	runs int
}

func (tb *teardownToolbox) run(context.Context, string, ...string) (string, error) {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.runs++
	if tb.gone {
		return "", errors.New("pods \"rook-ceph-tools\" not found")
	}
	return authLs, nil
}

func (tb *teardownToolbox) remove() {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.gone = true
}

func TestGuardCheckScansWhatPrepareCollected(t *testing.T) {
	ctx := context.Background()
	cs := fake.NewClientset(
		runningPod("mon"),
		newSecret(guardNS, "creds", map[string]string{"token": storedValue}),
		&corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Namespace: guardNS, Name: "leaky"},
			Data:       map[string]string{"config": storedValue},
		},
	)
	g, _ := startTestGuard(t, cs, guardNS)
	toolbox := &teardownToolbox{}

	g.Prepare(ctx, rookfake.NewSimpleClientset(), toolbox.run, []string{guardNS})
	toolbox.remove()
	require.NoError(t, cs.CoreV1().ConfigMaps(guardNS).Delete(ctx, "leaky", metav1.DeleteOptions{}))
	require.NoError(t, cs.CoreV1().Secrets(guardNS).Delete(ctx, "creds", metav1.DeleteOptions{}))
	r := g.Check()

	require.NoError(t, r.Err)
	assert.Equal(t, 1, toolbox.runs, "only Prepare reads the cephx keys")
	assert.Empty(t, r.EmptyCephx)
	assert.Equal(t, 4, r.ClusterSecrets, "3 cephx keys, 1 Kubernetes Secret value")
	require.Equal(t, []string{"k8s-secret/rook-ceph/creds:token"}, findingNames(r.Exact))
	assert.Equal(t, Location{File: "snapshots/rook-ceph_ConfigMap_leaky.txt", Line: 1}, r.Exact[0].Location)
}

// TestGuardCheckScansWhatRanAfterPrepare is the teardown: pods started and
// Secrets rotated or deleted after Prepare are still scanned and searched for.
func TestGuardCheckScansWhatRanAfterPrepare(t *testing.T) {
	const (
		first  = "a-first-value-of-the-token"
		second = "b-second-value-of-the-token"
	)
	ctx := context.Background()
	cs := fake.NewClientset(runningPod("mon"))
	g, dir := startTestGuard(t, cs, guardNS)
	secrets := cs.CoreV1().Secrets(guardNS)

	g.Prepare(ctx, rookfake.NewSimpleClientset(), canned(authLs), []string{guardNS})
	_, err := secrets.Create(ctx, newSecret(guardNS, "rotated", map[string]string{"token": first}), metav1.CreateOptions{})
	require.NoError(t, err)
	requireSecret(t, g.follower, "rook-ceph/rotated:token", first)
	_, err = secrets.Update(ctx, newSecret(guardNS, "rotated", map[string]string{"token": second}), metav1.UpdateOptions{})
	require.NoError(t, err)
	requireSecret(t, g.follower, "rook-ceph/rotated:token", second)
	require.NoError(t, secrets.Delete(ctx, "rotated", metav1.DeleteOptions{}))
	cleanup := runningPod("cleanup")
	cleanup.Spec.Containers[0].Env = []corev1.EnvVar{{Name: "OLD_TOKEN", Value: first}}
	createPod(t, cs, cleanup)
	requireLogFile(t, logPath(dir, guardNS, "cleanup", "main", 0), fakeLogs)
	r := g.Check()

	require.NoError(t, r.Err)
	assert.Equal(t, 2, r.Followers)
	assert.Equal(t, 5, r.ClusterSecrets, "3 cephx keys, 2 values of one Kubernetes Secret")
	require.Equal(t, []string{"k8s-secret/rook-ceph/rotated:token"}, findingNames(r.Exact))
	assert.Equal(t, Location{File: "specs/rook-ceph_cleanup.txt", Line: 1}, r.Exact[0].Location)
	for _, value := range []string{first, second} {
		assert.NotContains(t, r.Report(), value)
	}
}

func TestGuardCheckWithoutPrepareFails(t *testing.T) {
	g, dir := startTestGuard(t, fake.NewClientset(runningPod("mon")), guardNS)
	requireLogFile(t, logPath(dir, guardNS, "mon", "main", 0), fakeLogs)

	r := g.Check()

	require.Error(t, r.Err)
	assert.Contains(t, r.Err.Error(), "checked without Prepare")
	assert.True(t, r.Failed(ModeReport))
}

func TestGuardErrorsNeverCarryValues(t *testing.T) {
	g, _ := startTestGuard(t, fake.NewClientset(runningPod("mon")), guardNS)
	g.Register("registered", "a registered value")
	failing := func(context.Context, string, ...string) (string, error) {
		return "", errors.New("command failed, output: " + osd0Key)
	}

	r := check(t, g, failing)

	require.Error(t, r.Err)
	assert.NotContains(t, r.Err.Error(), osd0Key)
	assert.NotContains(t, r.Report(), osd0Key)
	assert.Contains(t, r.Report(), "ceph auth ls")
	assert.True(t, r.Failed(ModeFail))
	assert.True(t, r.Failed(ModeReport), "a guard that could not collect every source is broken")
}

func TestGuardScansPodSpecs(t *testing.T) {
	pod := runningPod("cleanup")
	pod.Spec.Containers[0].Env = []corev1.EnvVar{{Name: "ROOK_MON_SECRET", Value: storedValue}}
	cs := fake.NewClientset(pod, newSecret(guardNS, "creds", map[string]string{"token": storedValue}))
	g, dir := startTestGuard(t, cs, guardNS)
	requireLogFile(t, logPath(dir, guardNS, "cleanup", "main", 0), fakeLogs)

	r := check(t, g, canned(emptyAuthLs))

	require.NoError(t, r.Err)
	require.Len(t, r.Exact, 1)
	assert.Equal(t, "k8s-secret/rook-ceph/creds:token", r.Exact[0].What)
	assert.Equal(t, Location{File: "specs/rook-ceph_cleanup.txt", Line: 1}, r.Exact[0].Location)
	assert.NotContains(t, r.Report(), storedValue)
}

func TestGuardFailsOnFollowerError(t *testing.T) {
	cs := fake.NewClientset(
		runningPod("mon"),
		newSecret(guardNS, "creds", map[string]string{"token": storedValue}),
	)
	cs.PrependReactor("get", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() != "log" {
			return false, nil, nil
		}
		return true, nil, errors.New("connection refused")
	})
	g, _ := startTestGuard(t, cs, guardNS)
	require.Eventually(t, func() bool { return g.follower.Err() != nil }, waitFor, pollTick)

	r := check(t, g, canned(authLs))

	require.Error(t, r.Err)
	assert.Contains(t, r.Err.Error(), `following the log of container "main" in pod rook-ceph/mon: `)
	assert.Contains(t, r.Err.Error(), "connection refused")
	assert.Equal(t, 0, r.Followers, "a stream that never opened is not followed")
	assert.True(t, r.Failed(ModeFail))
	assert.True(t, r.Failed(ModeReport))
	assert.Contains(t, r.Report(), "guard errors:")
}

func TestGuardDedupesNamespaces(t *testing.T) {
	cs := fake.NewClientset(
		runningPod("mon"),
		newSecret(guardNS, "creds", map[string]string{"token": storedValue}),
	)
	g, _ := startTestGuard(t, cs, guardNS, guardNS)
	g.AddNamespace(guardNS)
	var mu sync.Mutex
	calls := 0
	counting := func(context.Context, string, ...string) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		calls++
		return emptyAuthLs, nil
	}

	g.Prepare(context.Background(), rookfake.NewSimpleClientset(), counting, []string{guardNS, guardNS})
	r := g.Check()

	require.NoError(t, r.Err)
	assert.Equal(t, 1, calls)
	assert.Equal(t, 1, r.Secrets)
}

func TestModeFromEnv(t *testing.T) {
	for value, want := range map[string]Mode{
		"":       ModeFail,
		"fail":   ModeFail,
		"report": ModeReport,
		"REPORT": ModeFail,
		"bogus":  ModeFail,
	} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("LEAK_GUARD_MODE", value)
			assert.Equal(t, want, ModeFromEnv())
		})
	}
}
