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
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

const storedValue = "a-kubernetes-secret-value"

const (
	fakeLogs = "fake logs"
	waitFor  = 5 * time.Second
	pollTick = 10 * time.Millisecond
)

var podsResource = schema.GroupVersionResource{Version: "v1", Resource: "pods"}

type containerState int

const (
	waiting containerState = iota
	running
	terminated
	crashLoopBackOff
)

type testContainer struct {
	name     string
	state    containerState
	restarts int32
}

func (c testContainer) status() corev1.ContainerStatus {
	s := corev1.ContainerStatus{Name: c.name, RestartCount: c.restarts}
	switch c.state {
	case waiting:
		s.State.Waiting = &corev1.ContainerStateWaiting{Reason: "ContainerCreating"}
	case running:
		s.State.Running = &corev1.ContainerStateRunning{}
	case terminated:
		s.State.Terminated = &corev1.ContainerStateTerminated{ExitCode: 0}
	case crashLoopBackOff:
		s.State.Waiting = &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}
		s.LastTerminationState.Terminated = &corev1.ContainerStateTerminated{ExitCode: 1}
	}
	return s
}

func testPod(ns, name string, initContainers, containers []testContainer) *corev1.Pod {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name}}
	for _, c := range initContainers {
		pod.Spec.InitContainers = append(pod.Spec.InitContainers, corev1.Container{Name: c.name})
		pod.Status.InitContainerStatuses = append(pod.Status.InitContainerStatuses, c.status())
	}
	for _, c := range containers {
		pod.Spec.Containers = append(pod.Spec.Containers, corev1.Container{Name: c.name})
		pod.Status.ContainerStatuses = append(pod.Status.ContainerStatuses, c.status())
	}
	return pod
}

type capturedLogs struct {
	mu    sync.Mutex
	lines []string
}

func (c *capturedLogs) logf(format string, args ...any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lines = append(c.lines, fmt.Sprintf(format, args...))
}

func (c *capturedLogs) get() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.lines)
}

func newTestFollower(t *testing.T, cs *fake.Clientset) (*Follower, *capturedLogs, string) {
	t.Helper()
	dir := t.TempDir()
	f := NewFollower(cs, dir)
	captured := &capturedLogs{}
	f.logf = captured.logf
	t.Cleanup(f.Stop)
	return f, captured, dir
}

func logPath(dir, ns, pod, container string, restarts int) string {
	return filepath.Join(dir, "logs", fmt.Sprintf("%s_%s_%s_%d.log", ns, pod, container, restarts))
}

func requireLogFile(t *testing.T, path, want string) {
	t.Helper()
	require.Eventually(t, func() bool {
		got, err := os.ReadFile(path)
		return err == nil && string(got) == want
	}, waitFor, pollTick, "%s never held %q", path, want)
}

func createPod(t *testing.T, cs *fake.Clientset, pod *corev1.Pod) {
	t.Helper()
	_, err := cs.CoreV1().Pods(pod.Namespace).Create(context.Background(), pod, metav1.CreateOptions{})
	require.NoError(t, err)
}

func updatePodStatus(t *testing.T, cs *fake.Clientset, pod *corev1.Pod) {
	t.Helper()
	_, err := cs.CoreV1().Pods(pod.Namespace).UpdateStatus(context.Background(), pod, metav1.UpdateOptions{})
	require.NoError(t, err)
}

func listLogDir(t *testing.T, dir string) map[string]int64 {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(dir, "logs"))
	if errors.Is(err, os.ErrNotExist) {
		return map[string]int64{}
	}
	require.NoError(t, err)
	files := make(map[string]int64, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		require.NoError(t, err)
		files[e.Name()] = info.Size()
	}
	return files
}

func TestFollowerFollowsExistingAndNewPods(t *testing.T) {
	const ns = "rook-ceph"
	cs := fake.NewClientset(testPod(ns, "before",
		[]testContainer{{name: "init", state: terminated}},
		[]testContainer{{name: "main", state: running}}))
	f, captured, dir := newTestFollower(t, cs)

	ctx := context.Background()
	f.Watch(ctx, ns)
	f.Watch(ctx, ns)

	createPod(t, cs, testPod(ns, "after",
		[]testContainer{{name: "setup", state: running}},
		[]testContainer{{name: "main", state: waiting}}))

	for _, p := range []string{
		logPath(dir, ns, "before", "init", 0),
		logPath(dir, ns, "before", "main", 0),
		logPath(dir, ns, "after", "setup", 0),
	} {
		requireLogFile(t, p, fakeLogs)
	}
	f.Stop()

	assert.Equal(t, 3, f.Started(), "the waiting container must not be followed, and a repeated Watch must not double up")
	assert.NoFileExists(t, logPath(dir, ns, "after", "main", 0))
	assert.Empty(t, captured.get())
}

func TestFollowerRestartStartsNewFile(t *testing.T) {
	const ns = "rook-ceph"
	pod := testPod(ns, "osd", nil, []testContainer{{name: "osd", state: running}})
	cs := fake.NewClientset(pod)
	f, captured, dir := newTestFollower(t, cs)

	f.Watch(context.Background(), ns)
	requireLogFile(t, logPath(dir, ns, "osd", "osd", 0), fakeLogs)

	updatePodStatus(t, cs, testPod(ns, "osd", nil, []testContainer{{name: "osd", state: running, restarts: 1}}))
	requireLogFile(t, logPath(dir, ns, "osd", "osd", 1), fakeLogs)

	f.Stop()
	assert.Equal(t, 2, f.Started())
	requireLogFile(t, logPath(dir, ns, "osd", "osd", 0), fakeLogs)
	assert.Empty(t, captured.get())
}

// recordLogRequests returns the options of every GetLogs call made so far.
func recordLogRequests(cs *fake.Clientset) func() []corev1.PodLogOptions {
	var mu sync.Mutex
	var requests []corev1.PodLogOptions
	cs.PrependReactor("get", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if a, ok := action.(k8stesting.GenericAction); ok && action.GetSubresource() == "log" {
			if opts, ok := a.GetValue().(*corev1.PodLogOptions); ok {
				mu.Lock()
				requests = append(requests, *opts)
				mu.Unlock()
			}
		}
		return false, nil, nil
	})
	return func() []corev1.PodLogOptions {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(requests)
	}
}

func TestFollowerFetchesMissedPreviousInstance(t *testing.T) {
	const ns = "rook-ceph"
	cs := fake.NewClientset(testPod(ns, "operator", nil, []testContainer{{name: "operator", state: running, restarts: 2}}))
	requests := recordLogRequests(cs)

	f, captured, dir := newTestFollower(t, cs)
	f.Watch(context.Background(), ns)
	requireLogFile(t, logPath(dir, ns, "operator", "operator", 1), fakeLogs)
	requireLogFile(t, logPath(dir, ns, "operator", "operator", 2), fakeLogs)
	f.Stop()

	assert.Equal(t, 2, f.Started())
	assert.NoFileExists(t, logPath(dir, ns, "operator", "operator", 0), "only the previous instance is recoverable")
	assert.ElementsMatch(t, []corev1.PodLogOptions{
		{Container: "operator", Previous: true},
		{Container: "operator", Follow: true},
	}, requests())
	assert.Empty(t, captured.get())
}

func TestFollowerFetchesInstanceInCrashLoopBackOff(t *testing.T) {
	const ns = "rook-ceph"
	cs := fake.NewClientset(testPod(ns, "prepare", nil, []testContainer{{name: "provision", state: crashLoopBackOff, restarts: 1}}))
	requests := recordLogRequests(cs)

	f, captured, dir := newTestFollower(t, cs)
	f.Watch(context.Background(), ns)
	requireLogFile(t, logPath(dir, ns, "prepare", "provision", 1), fakeLogs)

	updatePodStatus(t, cs, testPod(ns, "prepare", nil, []testContainer{{name: "provision", state: running, restarts: 2}}))
	requireLogFile(t, logPath(dir, ns, "prepare", "provision", 2), fakeLogs)
	f.Stop()

	assert.Equal(t, 2, f.Started(), "the instance fetched in backoff must not be fetched again once the next one runs")
	assert.NoFileExists(t, logPath(dir, ns, "prepare", "provision", 0))
	assert.Equal(t, []corev1.PodLogOptions{
		{Container: "provision", Previous: true},
		{Container: "provision", Follow: true},
	}, requests())
	assert.Empty(t, captured.get())
}

func TestFollowerToleratesMissingNamespace(t *testing.T) {
	const ns = "not-created-yet"
	cs := fake.NewClientset()
	f, captured, dir := newTestFollower(t, cs)

	ctx, cancel := context.WithTimeout(context.Background(), waitFor)
	defer cancel()
	f.Watch(ctx, ns)
	require.NoError(t, ctx.Err(), "Watch must return once the empty namespace is listed")

	createPod(t, cs, testPod(ns, "operator", nil, []testContainer{{name: "operator", state: running}}))
	requireLogFile(t, logPath(dir, ns, "operator", "operator", 0), fakeLogs)

	f.Stop()
	assert.Equal(t, 1, f.Started())
	assert.Empty(t, captured.get())
}

func TestFollowerPodDeletedQuietly(t *testing.T) {
	const ns = "rook-ceph"
	cs := fake.NewClientset(testPod(ns, "prepare", nil, []testContainer{{name: "provision", state: running}}))

	// The stream of "vanishing" fails because its pod is deleted while the
	// stream is being opened, as happens to an OSD prepare pod. Once the
	// follower asks whether the pod is gone, it has seen the error before
	// Stop, so the error was filtered rather than ended by the cancel.
	var checkedGone atomic.Bool
	cs.PrependReactor("get", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() != "log" {
			if a, ok := action.(k8stesting.GetAction); ok && a.GetName() == "vanishing" {
				checkedGone.Store(true)
			}
			return false, nil, nil
		}
		if a, ok := action.(k8stesting.GenericAction); ok && a.GetNamespace() == ns {
			if opts, ok := a.GetValue().(*corev1.PodLogOptions); ok && opts.Container == "vanishing" {
				assert.NoError(t, cs.Tracker().Delete(podsResource, ns, "vanishing"))
				return true, nil, errors.New("stream closed")
			}
		}
		return false, nil, nil
	})

	f, captured, dir := newTestFollower(t, cs)
	f.Watch(context.Background(), ns)
	path := logPath(dir, ns, "prepare", "provision", 0)
	requireLogFile(t, path, fakeLogs)

	require.NoError(t, cs.CoreV1().Pods(ns).Delete(context.Background(), "prepare", metav1.DeleteOptions{}))

	createPod(t, cs, testPod(ns, "vanishing", nil, []testContainer{{name: "vanishing", state: running}}))
	require.Eventually(t, checkedGone.Load, waitFor, pollTick)

	f.Stop()
	assert.Equal(t, 1, f.Started(), "the stream of vanishing never opened")
	assert.FileExists(t, path)
	requireLogFile(t, path, fakeLogs)
	assert.NoError(t, f.Err())
	assert.Empty(t, captured.get())
}

func TestFollowerReportsStreamErrorWithoutContent(t *testing.T) {
	const ns = "rook-ceph"
	cs := fake.NewClientset(testPod(ns, "mon", nil, []testContainer{{name: "mon", state: running}}))
	cs.PrependReactor("get", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() != "log" {
			return false, nil, nil
		}
		return true, nil, errors.New("connection refused")
	})

	f, captured, _ := newTestFollower(t, cs)
	f.Watch(context.Background(), ns)
	require.Eventually(t, func() bool { return len(captured.get()) == 1 }, waitFor, pollTick)
	f.Stop()

	assert.Equal(t, 0, f.Started(), "a stream that never opened is not followed")
	err := f.Err()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rook-ceph/mon")
	assert.Contains(t, err.Error(), `"mon"`)
	assert.Contains(t, err.Error(), "connection refused")
	lines := captured.get()
	require.Len(t, lines, 1)
	assert.Contains(t, lines[0], err.Error())
}

func TestFollowerReportsFileError(t *testing.T) {
	const ns = "rook-ceph"
	cs := fake.NewClientset(testPod(ns, "mon", nil, []testContainer{{name: "mon", state: running}}))
	f, _, dir := newTestFollower(t, cs)
	// A file where the log directory belongs makes creating the log fail.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "logs"), nil, 0o600))

	f.Watch(context.Background(), ns)
	require.Eventually(t, func() bool { return f.Err() != nil }, waitFor, pollTick)
	f.Stop()

	assert.Equal(t, 0, f.Started())
	assert.Contains(t, f.Err().Error(), `container "mon" in pod rook-ceph/mon`)
}

func TestFollowerCapsRecordedErrors(t *testing.T) {
	const ns = "rook-ceph"
	extra := 3
	var pods []runtime.Object
	for i := range maxFollowerErrors + extra {
		pods = append(pods, testPod(ns, fmt.Sprintf("mon-%d", i), nil, []testContainer{{name: "mon", state: running}}))
	}
	cs := fake.NewClientset(pods...)
	cs.PrependReactor("get", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() != "log" {
			return false, nil, nil
		}
		return true, nil, errors.New("connection refused")
	})
	f, captured, _ := newTestFollower(t, cs)

	f.Watch(context.Background(), ns)
	require.Eventually(t, func() bool { return len(captured.get()) == len(pods) }, waitFor, pollTick)
	f.Stop()

	lines := strings.Split(f.Err().Error(), "\n")
	require.Len(t, lines, maxFollowerErrors+1)
	assert.Equal(t, fmt.Sprintf("%d more follower errors", extra), lines[maxFollowerErrors])
}

func specPath(dir, ns, pod string) string {
	return filepath.Join(dir, "specs", fmt.Sprintf("%s_%s.txt", ns, pod))
}

// specPod is a pod with command, args and env in each kind of container.
func specPod(uid types.UID, value string) *corev1.Pod {
	pod := testPod("rook-ceph", "cleanup",
		[]testContainer{{name: "setup", state: terminated}},
		[]testContainer{{name: "main", state: running}})
	pod.UID = uid
	pod.Spec.InitContainers[0].Command = []string{"/bin/sh", "-c"}
	pod.Spec.InitContainers[0].Args = []string{"echo " + value}
	pod.Spec.Containers[0].Args = []string{"--secret", value}
	pod.Spec.Containers[0].Env = []corev1.EnvVar{
		{Name: "ROOK_MON_SECRET", Value: value},
		{Name: "FROM_SECRET", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: "creds"}, Key: "token",
		}}},
		{Name: "MULTI", Value: "first\nsecond\n"},
	}
	pod.Spec.EphemeralContainers = []corev1.EphemeralContainer{{EphemeralContainerCommon: corev1.EphemeralContainerCommon{
		Name:    "debug",
		Command: []string{"sleep"},
		Env:     []corev1.EnvVar{{Name: "DEBUG_TOKEN", Value: value}},
	}}}
	return pod
}

func specFile(value string) string {
	return `initContainers.setup.command[0]: /bin/sh
initContainers.setup.command[1]: -c
initContainers.setup.args[0]: echo ` + value + `
containers.main.args[0]: --secret
containers.main.args[1]: ` + value + `
containers.main.env.ROOK_MON_SECRET: ` + value + `
containers.main.env.MULTI: first
second
ephemeralContainers.debug.command[0]: sleep
ephemeralContainers.debug.env.DEBUG_TOKEN: ` + value + `
`
}

func TestFollowerWritesPodSpecOncePerUID(t *testing.T) {
	const ns = "rook-ceph"
	cs := fake.NewClientset(specPod("uid-1", "first-literal"))
	f, captured, dir := newTestFollower(t, cs)
	path := specPath(dir, ns, "cleanup")

	f.Watch(context.Background(), ns)
	requireLogFile(t, path, specFile("first-literal"))

	restarted := specPod("uid-1", "first-literal")
	restarted.Status.ContainerStatuses[0].RestartCount = 1
	updatePodStatus(t, cs, restarted)
	requireLogFile(t, logPath(dir, ns, "cleanup", "main", 1), fakeLogs)
	requireLogFile(t, path, specFile("first-literal"))

	// A pod recreated under the same name is appended, as its logs are.
	require.NoError(t, cs.CoreV1().Pods(ns).Delete(context.Background(), "cleanup", metav1.DeleteOptions{}))
	createPod(t, cs, specPod("uid-2", "second-literal"))
	requireLogFile(t, path, specFile("first-literal")+specFile("second-literal"))
	f.Stop()

	for p, mode := range map[string]os.FileMode{path: 0o600, filepath.Dir(path): 0o700 | os.ModeDir} {
		info, err := os.Stat(p)
		require.NoError(t, err)
		assert.Equal(t, mode, info.Mode(), p)
	}
	require.NoError(t, f.Err())
	assert.Empty(t, captured.get())
}

func TestFollowerWritesSpecOfPodBeingDeleted(t *testing.T) {
	const ns = "rook-ceph"
	pod := specPod("uid-1", "deleted-literal")
	pod.Status = corev1.PodStatus{}
	pod.DeletionTimestamp = &metav1.Time{Time: time.Now()}
	pod.Finalizers = []string{"example.com/hold"}
	cs := fake.NewClientset()
	f, _, dir := newTestFollower(t, cs)

	f.Watch(context.Background(), ns)
	createPod(t, cs, pod)
	requireLogFile(t, specPath(dir, ns, "cleanup"), specFile("deleted-literal"))
	f.Stop()

	assert.Equal(t, 0, f.Started())
	require.NoError(t, f.Err())
}

func TestFollowerReportsSpecWriteError(t *testing.T) {
	const ns = "rook-ceph"
	cs := fake.NewClientset(specPod("uid-1", "unwritten-literal"))
	f, _, dir := newTestFollower(t, cs)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "specs"), nil, 0o600))

	f.Watch(context.Background(), ns)
	f.Stop()

	err := f.Err()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "writing the spec of pod rook-ceph/cleanup")
	assert.NotContains(t, err.Error(), "unwritten-literal")
}

// hasSecret reports whether secrets holds value under name.
func hasSecret(secrets []Secret, name, value string) bool {
	return slices.ContainsFunc(secrets, func(s Secret) bool { return s.Name == name && s.Value == value })
}

func requireSecret(t *testing.T, f *Follower, name, value string) {
	t.Helper()
	require.Eventually(t, func() bool { return hasSecret(f.Secrets(), name, value) }, waitFor, pollTick,
		"the follower never collected a value of %s", name)
}

func TestFollowerKeepsEverySecretValue(t *testing.T) {
	const (
		ns     = "rook-ceph"
		first  = "a-first-value-of-the-token"
		second = "b-second-value-of-the-token"
		late   = "a-value-created-after-the-watch"
		marker = "a-value-that-orders-the-events"
	)
	ctx := context.Background()
	cs := fake.NewClientset(
		newSecret(ns, "creds", map[string]string{"token": first, "fsid": "9b6a1c1e-3f0e-4d8a-9c55-2f1f0a3e7b21"}),
		// #nosec G101 -- fake test credential
		newSecret("unwatched", "other", map[string]string{"token": "a-value-in-an-unwatched-namespace"}),
	)
	f, captured, _ := newTestFollower(t, cs)
	secrets := cs.CoreV1().Secrets(ns)

	f.Watch(ctx, ns)
	assert.True(t, hasSecret(f.Secrets(), "rook-ceph/creds:token", first), "Watch returns once the existing Secrets are seen")

	// Rotated, then relabeled, then deleted: each value seen is kept, once.
	_, err := secrets.Update(ctx, newSecret(ns, "creds", map[string]string{"token": second}), metav1.UpdateOptions{})
	require.NoError(t, err)
	requireSecret(t, f, "rook-ceph/creds:token", second)
	relabeled := newSecret(ns, "creds", map[string]string{"token": second})
	relabeled.Labels = map[string]string{"rotated": "true"}
	_, err = secrets.Update(ctx, relabeled, metav1.UpdateOptions{})
	require.NoError(t, err)
	require.NoError(t, secrets.Delete(ctx, "creds", metav1.DeleteOptions{}))
	_, err = secrets.Create(ctx, newSecret(ns, "late", map[string]string{"token": late}), metav1.CreateOptions{})
	require.NoError(t, err)
	// One informer delivers events in order, so once the marker is seen
	// every earlier event has been handled.
	_, err = secrets.Create(ctx, newSecret(ns, "marker", map[string]string{"token": marker}), metav1.CreateOptions{})
	require.NoError(t, err)
	requireSecret(t, f, "rook-ceph/marker:token", marker)
	f.Stop()

	got := f.Secrets()
	want := []struct{ name, value string }{
		{"rook-ceph/creds:token", first},
		{"rook-ceph/creds:token", second},
		{"rook-ceph/late:token", late},
		{"rook-ceph/marker:token", marker},
	}
	wantNames := make([]string, len(want))
	for i, w := range want {
		wantNames[i] = w.name
	}
	require.Equal(t, wantNames, secretNames(got), "ordered by name, then value")
	for i, w := range want {
		assert.Equal(t, sourceK8sSecret, got[i].Source, "secret %s source", w.name)
		assert.True(t, got[i].Value == w.value, "secret %d (%s) has the wrong value", i, w.name)
	}
	require.NoError(t, f.Err())
	assert.Empty(t, captured.get())
}

func TestFollowerSecretsOfAnAddedNamespace(t *testing.T) {
	const ns = "not-created-yet"
	cs := fake.NewClientset()
	f, _, _ := newTestFollower(t, cs)

	f.Watch(context.Background(), ns)
	_, err := cs.CoreV1().Secrets(ns).Create(context.Background(),
		newSecret(ns, "obc", map[string]string{"AWS_SECRET_ACCESS_KEY": storedValue}), metav1.CreateOptions{})
	require.NoError(t, err)

	requireSecret(t, f, ns+"/obc:AWS_SECRET_ACCESS_KEY", storedValue)
}

func TestStopWaits(t *testing.T) {
	const ns = "rook-ceph"
	cs := fake.NewClientset(testPod(ns, "mgr", nil, []testContainer{{name: "mgr", state: running}}))
	f, captured, dir := newTestFollower(t, cs)

	f.Watch(context.Background(), ns)
	requireLogFile(t, logPath(dir, ns, "mgr", "mgr", 0), fakeLogs)

	f.Stop()
	started := f.Started()
	before := listLogDir(t, dir)

	createPod(t, cs, testPod(ns, "late", nil, []testContainer{{name: "late", state: running}}))
	updatePodStatus(t, cs, testPod(ns, "mgr", nil, []testContainer{{name: "mgr", state: running, restarts: 1}}))
	_, err := cs.CoreV1().Secrets(ns).Create(context.Background(),
		newSecret(ns, "late", map[string]string{"token": storedValue}), metav1.CreateOptions{})
	require.NoError(t, err)
	f.Watch(context.Background(), "other")
	_, err = cs.CoreV1().Secrets("other").Create(context.Background(),
		newSecret("other", "late", map[string]string{"token": storedValue}), metav1.CreateOptions{})
	require.NoError(t, err)
	f.Stop()

	// Nothing should happen, so the only way to observe it is to give a
	// stray goroutine time to act.
	time.Sleep(200 * time.Millisecond)
	assert.Equal(t, started, f.Started())
	assert.Equal(t, before, listLogDir(t, dir))
	assert.NoFileExists(t, specPath(dir, ns, "late"))
	assert.Empty(t, secretNames(f.Secrets()))
	assert.Empty(t, captured.get())
}
