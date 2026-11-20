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

package installer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	rookfake "github.com/rook/rook/pkg/client/clientset/versioned/fake"
	"github.com/rook/rook/tests/framework/leakguard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestToolboxRunnerSplitsCommandFromArgs(t *testing.T) {
	var gotNamespace, gotCommand string
	var gotArgs []string
	run := newToolboxRunner(func(namespace, command string, args []string) (string, error) {
		gotNamespace, gotCommand, gotArgs = namespace, command, args
		return "out", nil
	})

	stdout, err := run(context.Background(), "ns", "ceph", "auth", "ls", "--format", "json")
	require.NoError(t, err)
	assert.Equal(t, "out", stdout)
	assert.Equal(t, "ns", gotNamespace)
	assert.Equal(t, "ceph", gotCommand)
	assert.Equal(t, []string{"auth", "ls", "--format", "json"}, gotArgs)
}

func TestToolboxRunnerPassesErrorThrough(t *testing.T) {
	want := errors.New("exec failed")
	run := newToolboxRunner(func(string, string, []string) (string, error) {
		return "", want
	})

	_, err := run(context.Background(), "ns", "ceph")
	assert.ErrorIs(t, err, want)
}

func TestToolboxRunnerRejectsNoCommand(t *testing.T) {
	called := false
	run := newToolboxRunner(func(string, string, []string) (string, error) {
		called = true
		return "", nil
	})

	_, err := run(context.Background(), "ns")
	assert.Error(t, err)
	assert.False(t, called)
}

func TestToolboxRunnerHonorsCanceledContext(t *testing.T) {
	called := false
	run := newToolboxRunner(func(string, string, []string) (string, error) {
		called = true
		return "", nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := run(ctx, "ns", "ceph")
	assert.ErrorIs(t, err, context.Canceled)
	assert.False(t, called)
}

func TestLeakGuardDirName(t *testing.T) {
	assert.Equal(t, "/out/leakguard-TestCephSmokeSuite", leakGuardDir("/out", "TestCephSmokeSuite"))
	assert.Equal(t, "/out/leakguard-TestSuite_TestMethod", leakGuardDir("/out", "TestSuite/TestMethod"))
}

func TestLeakGuardFailureIsOneLine(t *testing.T) {
	r := leakguard.Result{
		Exact:        []leakguard.Finding{{Detector: "exact", What: "registered/x", Location: leakguard.Location{File: "logs/a.log", Line: 1}}},
		Followers:    1,
		FindingsFile: "/out/leakguard-TestSuite/findings.txt",
	}

	msg := leakGuardFailure(r)
	assert.NotContains(t, msg, "\n")
	assert.Contains(t, msg, r.Summary())
	assert.Contains(t, msg, "/out/leakguard-TestSuite/findings.txt")
	assert.NotContains(t, msg, "logs/a.log", "the locations belong to the logged report")

	r.FindingsFile = ""
	assert.NotContains(t, leakGuardFailure(r), "listed in")
}

// recordingT records the failures assert reports to it.
type recordingT struct {
	failures []string
}

func (r *recordingT) Errorf(format string, args ...any) {
	r.failures = append(r.failures, fmt.Sprintf(format, args...))
}

const leakCheckNS = "rook-ceph"

func runningPod(name string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Namespace: leakCheckNS, Name: name},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "main"}}},
		Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{
			Name:  "main",
			State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
		}}},
	}
}

// toolboxUntil serves `ceph auth ls` until gone is set, as the uninstall
// deletes the toolbox.
func toolboxUntil(gone *atomic.Bool) leakguard.ToolboxRunner {
	return func(context.Context, string, ...string) (string, error) {
		if gone.Load() {
			return "", errors.New(`pods "rook-ceph-tools" not found`)
		}
		return `{"auth_dump":[{"entity":"client.admin","key":"AQBadminkeyexamplevalue012345678=="}]}`, nil
	}
}

type leakCheckFixture struct {
	guard    *leakguard.Guard
	guardCtx context.Context
	t        *recordingT
	check    func()
	dir      string
}

// prepareTestLeakCheck starts a guard over cs and prepares its check, with a
// toolbox that is gone once the check is prepared.
func prepareTestLeakCheck(t *testing.T, cs *fake.Clientset) leakCheckFixture {
	t.Helper()
	t.Setenv("LEAK_GUARD_MODE", "fail")
	f := leakCheckFixture{t: &recordingT{}, dir: t.TempDir()}
	guardCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	f.guardCtx = guardCtx
	f.guard = leakguard.Start(guardCtx, cs, f.dir, leakCheckNS)
	var gone atomic.Bool
	f.check = prepareGuardCheck(f.t, f.guard, cancel, rookfake.NewSimpleClientset(), toolboxUntil(&gone), []string{leakCheckNS})
	gone.Store(true)
	return f
}

func requireFile(t *testing.T, path string) {
	t.Helper()
	require.Eventually(t, func() bool {
		_, err := os.Stat(path)
		return err == nil
	}, 5*time.Second, 10*time.Millisecond, "%s was never written", path)
}

func TestLeakCheckCoversTheTeardown(t *testing.T) {
	const leaked = "a-value-leaked-during-the-teardown"
	cs := fake.NewClientset(runningPod("mon"))
	f := prepareTestLeakCheck(t, cs)
	f.guard.Register("teardown", leaked)

	cleanup := runningPod("cleanup")
	cleanup.Spec.Containers[0].Env = []corev1.EnvVar{{Name: "TOKEN", Value: leaked}}
	_, err := cs.CoreV1().Pods(leakCheckNS).Create(context.Background(), cleanup, metav1.CreateOptions{})
	require.NoError(t, err)
	requireFile(t, filepath.Join(f.dir, "specs", leakCheckNS+"_cleanup.txt"))
	assert.Empty(t, f.t.failures, "nothing fails before the check")
	require.NoError(t, f.guardCtx.Err(), "the guard keeps running until the check")

	f.check()

	require.Len(t, f.t.failures, 1)
	assert.Contains(t, f.t.failures[0], "leak guard: 1 exact-match findings, 0 pattern findings, 0 liveness failures")
	assert.NotContains(t, f.t.failures[0], leaked)
	assert.ErrorIs(t, f.guardCtx.Err(), context.Canceled)
}

func TestLeakCheckPassesACleanTeardown(t *testing.T) {
	f := prepareTestLeakCheck(t, fake.NewClientset(runningPod("mon")))
	requireFile(t, filepath.Join(f.dir, "logs", leakCheckNS+"_mon_main_0.log"))

	f.check()

	assert.Empty(t, f.t.failures)
	assert.ErrorIs(t, f.guardCtx.Err(), context.Canceled)
}

func TestPrepareLeakCheckWithoutAGuardDoesNothing(t *testing.T) {
	h := &CephInstaller{}
	assert.NotPanics(t, h.prepareLeakCheck([]string{leakCheckNS}))
}

// TestGuardCheckRunsWhenPrepareDoesNotReturn covers a toolbox exec that panics,
// or ends its goroutine as require does, while the check is being prepared.
func TestGuardCheckRunsWhenPrepareDoesNotReturn(t *testing.T) {
	for _, tc := range []struct {
		name      string
		abort     func()
		wantPanic any
	}{
		{name: "panic", abort: func() { panic("toolbox exec panicked") }, wantPanic: "toolbox exec panicked"},
		{name: "Goexit", abort: runtime.Goexit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LEAK_GUARD_MODE", "fail")
			guardCtx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			guard := leakguard.Start(guardCtx, fake.NewClientset(runningPod("mon")), t.TempDir(), leakCheckNS)
			rt := &recordingT{}
			aborting := func(context.Context, string, ...string) (string, error) {
				tc.abort()
				return "", nil
			}

			returned := false
			recovered := make(chan any)
			go func() {
				defer func() { recovered <- recover() }()
				prepareGuardCheck(rt, guard, cancel, rookfake.NewSimpleClientset(), aborting, []string{leakCheckNS})
				returned = true
			}()

			assert.Equal(t, tc.wantPanic, <-recovered)
			assert.False(t, returned)
			require.Len(t, rt.failures, 1, "the check runs once, failing on the missing Prepare")
			assert.Contains(t, rt.failures[0], "guard errors")
			assert.ErrorIs(t, guardCtx.Err(), context.Canceled)
		})
	}
}
