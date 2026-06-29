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

package wait4

import (
	"bufio"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	watchtools "k8s.io/client-go/tools/watch"

	"github.com/rook/rook/tests/framework/utils"
)

// Default timeouts for waiting on operator reconciliation in the object
// tests. Bespoke waits (e.g. the shared store's multi-minute teardown ladder)
// still pass explicit durations.
const (
	// TimeoutShort covers routine status changes on an already-running
	// operator and rgw: condition waits, deletions, field updates.
	TimeoutShort = 40 * time.Second
	// TimeoutMedium covers resource creation that needs a full reconcile,
	// such as a CephBucketTopic becoming Ready with its ARN set.
	TimeoutMedium = time.Minute
	// TimeoutLong covers first-reconcile waits that may race rgw startup,
	// such as CephObjectStoreUser creation right after the store comes up.
	TimeoutLong = 2 * time.Minute
)

// NamespacedWatcher is satisfied by any typed namespaced client (corev1
// ServiceInterface, cephv1 CephObjectStoreUserInterface, etc.). T is the
// resource's pointer type and L its list type, both inferred at the call site
// from the client's Get/List return types.
type NamespacedWatcher[T, L runtime.Object] interface {
	Get(ctx context.Context, name string, opts metav1.GetOptions) (T, error)
	List(ctx context.Context, opts metav1.ListOptions) (L, error)
	Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error)
}

// nameScopedListWatch returns a ListWatch restricted to a single resource name.
func nameScopedListWatch[T, L runtime.Object](ctx context.Context, client NamespacedWatcher[T, L], name string) *cache.ListWatch {
	fs := fields.OneTermEqualSelector("metadata.name", name).String()
	return &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			opts.FieldSelector = fs
			return client.List(ctx, opts)
		},
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			opts.FieldSelector = fs
			return client.Watch(ctx, opts)
		},
	}
}

// heartbeatInterval is how often a watch-based wait logs progress while it is
// still blocked.
const heartbeatInterval = 15 * time.Second

// heartbeat logs a periodic progress line for a watch-based wait that would
// otherwise block silently, so a slow reconcile shows up in CI output instead
// of looking like a hang. It logs nothing for waits shorter than
// heartbeatInterval; the caller must invoke the returned stop func when the
// wait returns.
func heartbeat(t *testing.T, desc string) (stop func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()
		for elapsed := heartbeatInterval; ; elapsed += heartbeatInterval {
			select {
			case <-done:
				return
			case <-ticker.C:
				t.Logf("still waiting for %s (%s)", desc, elapsed)
			}
		}
	}()
	return func() { close(done) }
}

// waitForAbsent blocks until the named resource is gone or the timeout elapses.
//
// The wait is race-free against fast deletions: UntilWithSync performs an
// initial List (scoped to the resource name via a field selector) before
// starting the watch, and the precondition treats an empty store as "already
// gone". Subsequent deletion events are guaranteed to be delivered because the
// watch resumes from the List's resourceVersion.
func waitForAbsent[T, L runtime.Object](ctx context.Context, t *testing.T, client NamespacedWatcher[T, L], name string, timeout time.Duration) error {
	var objType T
	lw := nameScopedListWatch(ctx, client, name)

	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	precondition := func(store cache.Store) (bool, error) {
		// lw is name-scoped, so an empty store unambiguously means the object
		// was already gone by the time the reflector's initial list completed.
		return len(store.List()) == 0, nil
	}
	condition := func(ev watch.Event) (bool, error) {
		return ev.Type == watch.Deleted, nil
	}

	defer heartbeat(t, fmt.Sprintf("%T %q to be removed", objType, name))()

	if _, err := watchtools.UntilWithSync(waitCtx, lw, objType, precondition, condition); err != nil {
		return fmt.Errorf("%T %q is still present: %w", objType, name, err)
	}
	return nil
}

// waitForCondition blocks until the named resource satisfies cond or the timeout
// elapses, returning the matching object. The initial List makes it race-free in
// the same way as waitForAbsent: a resource that already satisfies cond is
// detected immediately, and subsequent updates arrive as watch events.
func waitForCondition[T, L runtime.Object](ctx context.Context, t *testing.T, client NamespacedWatcher[T, L], name string, cond func(T) bool, timeout time.Duration) (T, error) {
	var objType T
	var match T
	lw := nameScopedListWatch(ctx, client, name)

	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	precondition := func(store cache.Store) (bool, error) {
		for _, o := range store.List() {
			if typed, ok := o.(T); ok && cond(typed) {
				match = typed
				return true, nil
			}
		}
		return false, nil
	}
	condition := func(ev watch.Event) (bool, error) {
		switch ev.Type {
		case watch.Added, watch.Modified:
			if typed, ok := ev.Object.(T); ok && cond(typed) {
				match = typed
				return true, nil
			}
		case watch.Deleted:
			return false, fmt.Errorf("%T %q was deleted while waiting for condition", objType, name)
		}
		return false, nil
	}

	defer heartbeat(t, fmt.Sprintf("%T %q to be ready", objType, name))()

	if _, err := watchtools.UntilWithSync(waitCtx, lw, objType, precondition, condition); err != nil {
		return match, fmt.Errorf("%T %q did not satisfy condition: %w", objType, name, err)
	}
	return match, nil
}

// NamespacedDeleter is a NamespacedWatcher that can also delete by name.
type NamespacedDeleter[T, L runtime.Object] interface {
	NamespacedWatcher[T, L]
	Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error
}

func deleteAndWait[T, L runtime.Object](ctx context.Context, t *testing.T, client NamespacedDeleter[T, L], name string, timeout time.Duration) error {
	var objType T
	if err := client.Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		return fmt.Errorf("failed to delete %T %q: %w", objType, name, err)
	}
	return waitForAbsent[T, L](ctx, t, client, name, timeout)
}

// AssertDelete deletes a single named resource and blocks until it is gone,
// reporting failures (including timeout) via assert.Fail (non-fatal). It returns
// true on success. Prefer this for teardown/cleanup, so a stuck finalizer on one
// resource does not prevent cleanup of the rest; use RequireDelete when later
// steps depend on the deletion having completed.
func AssertDelete[T, L runtime.Object](
	ctx context.Context,
	t *testing.T,
	client NamespacedDeleter[T, L],
	name string,
	timeout time.Duration,
	msgAndArgs ...interface{},
) bool {
	t.Helper()
	if err := deleteAndWait(ctx, t, client, name, timeout); err != nil {
		return assert.Fail(t, err.Error(), msgAndArgs...)
	}
	return true
}

// RequireDelete is the fatal counterpart of AssertDelete: it stops the test
// (FailNow) if the resource cannot be deleted or is not gone before the timeout.
func RequireDelete[T, L runtime.Object](
	ctx context.Context,
	t *testing.T,
	client NamespacedDeleter[T, L],
	name string,
	timeout time.Duration,
	msgAndArgs ...interface{},
) {
	t.Helper()
	require.NoError(t, deleteAndWait(ctx, t, client, name, timeout), msgAndArgs...)
}

// AssertAbsent waits for a named resource to disappear without deleting it
// itself, reporting failures via assert.Fail (non-fatal). Use it for resources
// removed as a side effect of another action — e.g. an ObjectBucket that is
// garbage-collected when its ObjectBucketClaim is deleted. To delete and wait,
// use AssertDelete/RequireDelete.
func AssertAbsent[T, L runtime.Object](
	ctx context.Context,
	t *testing.T,
	client NamespacedWatcher[T, L],
	name string,
	timeout time.Duration,
	msgAndArgs ...interface{},
) bool {
	t.Helper()
	if err := waitForAbsent(ctx, t, client, name, timeout); err != nil {
		return assert.Fail(t, err.Error(), msgAndArgs...)
	}
	return true
}

// RequireAbsent is the fatal counterpart of AssertAbsent.
func RequireAbsent[T, L runtime.Object](
	ctx context.Context,
	t *testing.T,
	client NamespacedWatcher[T, L],
	name string,
	timeout time.Duration,
	msgAndArgs ...interface{},
) {
	t.Helper()
	require.NoError(t, waitForAbsent(ctx, t, client, name, timeout), msgAndArgs...)
}

// NamespacedReadyWaiter is a NamespacedWatcher that can also create.
type NamespacedReadyWaiter[T, L runtime.Object] interface {
	NamespacedWatcher[T, L]
	Create(ctx context.Context, obj T, opts metav1.CreateOptions) (T, error)
}

// createAndWaitReady creates obj and, when ready is non-nil, blocks until ready
// reports true for the live object or the timeout elapses, returning the live
// object. Pass a nil ready predicate for resources that are usable as soon as
// they are created (Namespaces, Services, Secrets, ...).
//
// Unlike deletion, "ready" has no universal representation across resource
// types, so the readiness check is supplied by the caller.
func createAndWaitReady[T, L runtime.Object](
	ctx context.Context,
	t *testing.T,
	client NamespacedReadyWaiter[T, L],
	obj T,
	ready func(T) bool,
	timeout time.Duration,
) (T, error) {
	var zero T

	created, err := client.Create(ctx, obj, metav1.CreateOptions{})
	if err != nil {
		return zero, fmt.Errorf("failed to create %T: %w", obj, err)
	}
	if ready == nil {
		return created, nil
	}

	accessor, err := meta.Accessor(created)
	if err != nil {
		return zero, fmt.Errorf("failed to read metadata of %T: %w", created, err)
	}

	return waitForCondition[T, L](ctx, t, client, accessor.GetName(), ready, timeout)
}

// AssertCreate creates obj and waits for the ready predicate, reporting failures
// (including timeout) via assert.Fail (non-fatal) like AssertDelete. It returns
// the live object and true on success. Prefer this for cleanup/best-effort
// creation where the test should continue on failure; for setup preconditions
// use RequireCreate.
func AssertCreate[T, L runtime.Object](
	ctx context.Context,
	t *testing.T,
	client NamespacedReadyWaiter[T, L],
	obj T,
	ready func(T) bool,
	timeout time.Duration,
	msgAndArgs ...interface{},
) (T, bool) {
	t.Helper()
	live, err := createAndWaitReady(ctx, t, client, obj, ready, timeout)
	if err != nil {
		var zero T
		assert.Fail(t, err.Error(), msgAndArgs...)
		return zero, false
	}
	return live, true
}

// RequireCreate is the fatal counterpart of AssertCreate: it stops the test
// (FailNow) if the resource cannot be created or does not become ready, and
// returns the live object. This is the common case for setup preconditions, so
// the call site needs no follow-up require.True.
func RequireCreate[T, L runtime.Object](
	ctx context.Context,
	t *testing.T,
	client NamespacedReadyWaiter[T, L],
	obj T,
	ready func(T) bool,
	timeout time.Duration,
	msgAndArgs ...interface{},
) T {
	t.Helper()
	live, err := createAndWaitReady(ctx, t, client, obj, ready, timeout)
	require.NoError(t, err, msgAndArgs...)
	return live
}

// AssertCondition waits for the named resource to satisfy cond, reporting
// failures (including timeout) via assert.Fail (non-fatal). It returns the
// matching object and true on success. Use it to wait on an already-created
// resource — e.g. an OBC reaching Bound, or a status field changing after an
// update; for the create-and-wait case use AssertCreate/RequireCreate.
func AssertCondition[T, L runtime.Object](
	ctx context.Context,
	t *testing.T,
	client NamespacedWatcher[T, L],
	name string,
	cond func(T) bool,
	timeout time.Duration,
	msgAndArgs ...interface{},
) (T, bool) {
	t.Helper()
	match, err := waitForCondition(ctx, t, client, name, cond, timeout)
	if err != nil {
		var zero T
		assert.Fail(t, err.Error(), msgAndArgs...)
		return zero, false
	}
	return match, true
}

// RequireCondition is the fatal counterpart of AssertCondition.
func RequireCondition[T, L runtime.Object](
	ctx context.Context,
	t *testing.T,
	client NamespacedWatcher[T, L],
	name string,
	cond func(T) bool,
	timeout time.Duration,
	msgAndArgs ...interface{},
) T {
	t.Helper()
	match, err := waitForCondition(ctx, t, client, name, cond, timeout)
	require.NoError(t, err, msgAndArgs...)
	return match
}

// AssertEventually polls cond until it returns nil or the timeout elapses,
// reporting failure via assert.Fail (non-fatal). It is for state the
// Kubernetes API cannot watch (the rgw admin API, S3, SNS); k8s resources
// should use the watch-based helpers above. The poll loop is utils.Eventually;
// this adds the assert-flavored, non-fatal reporting that pairs with the
// AssertCreate/AssertDelete/AssertCondition family.
//
// cond must not call assert or require: report transient failures by returning
// an error (which is surfaced in the timeout message), and make assertions on
// captured state after the wait. cond receives a context bounded by the wait
// deadline; thread it into the reads it performs.
func AssertEventually(ctx context.Context, t *testing.T, timeout time.Duration, desc string, cond func(context.Context) error) bool {
	t.Helper()
	if err := utils.Eventually(ctx, t, timeout, desc, cond); err != nil {
		return assert.Fail(t, err.Error())
	}
	return true
}

// RequireEventually is the fatal counterpart of AssertEventually: it stops
// the test (FailNow) if cond does not succeed before the timeout. Use it
// when subsequent statements consume the polled result.
func RequireEventually(ctx context.Context, t *testing.T, timeout time.Duration, desc string, cond func(context.Context) error) {
	t.Helper()
	require.NoError(t, utils.Eventually(ctx, t, timeout, desc, cond))
}

// matchPodLog follows the logs of the first pod matching selector until match
// returns true for a line, the stream ends, or the wait deadline passes.
// Reaching the end of the stream without a match is an error: following the
// stream (rather than scanning a snapshot) is what lets the wait distinguish
// "absent" from "not logged yet".
func matchPodLog(ctx context.Context, t *testing.T, k8sh *utils.K8sHelper, namespace string, selector labels.Selector, timeout time.Duration, match func(line string) bool) error {
	t.Helper()

	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	pods, err := k8sh.Clientset.CoreV1().Pods(namespace).List(waitCtx, metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return fmt.Errorf("failed to list pods: %w", err)
	}
	if len(pods.Items) == 0 {
		return fmt.Errorf("no pods found with labels %v in namespace %q", selector, namespace)
	}

	// if there are multiple pods, just pick the first one
	pod := pods.Items[0]
	t.Logf("matching against logs of pod %q (first match for labels %v)", pod.Name, selector)

	req := k8sh.Clientset.CoreV1().Pods(namespace).GetLogs(pod.Name, &corev1.PodLogOptions{Follow: true})
	logStream, err := req.Stream(waitCtx)
	if err != nil {
		return fmt.Errorf("failed to stream logs from pod %q: %w", pod.Name, err)
	}
	defer logStream.Close()

	scanner := bufio.NewScanner(logStream)
	for scanner.Scan() {
		if match(scanner.Text()) {
			return nil
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("no matching log line from pod %q: %w", pod.Name, err)
	}
	return fmt.Errorf("log stream from pod %q ended without a matching line", pod.Name)
}

// AssertPodLog follows the logs of the first pod matching selector and reports
// failure via assert.Fail (non-fatal) if no line satisfies match before the
// timeout. It is for log-based state the Kubernetes API cannot watch (e.g. an
// operator logging a reconcile error); for a setup precondition use
// RequirePodLog.
func AssertPodLog(
	ctx context.Context,
	t *testing.T,
	k8sh *utils.K8sHelper,
	namespace string,
	selector labels.Selector,
	timeout time.Duration,
	match func(line string) bool,
	msgAndArgs ...interface{},
) bool {
	t.Helper()
	if err := matchPodLog(ctx, t, k8sh, namespace, selector, timeout, match); err != nil {
		return assert.Fail(t, err.Error(), msgAndArgs...)
	}
	return true
}

// RequirePodLog is the fatal counterpart of AssertPodLog.
func RequirePodLog(
	ctx context.Context,
	t *testing.T,
	k8sh *utils.K8sHelper,
	namespace string,
	selector labels.Selector,
	timeout time.Duration,
	match func(line string) bool,
	msgAndArgs ...interface{},
) {
	t.Helper()
	require.NoError(t, matchPodLog(ctx, t, k8sh, namespace, selector, timeout, match), msgAndArgs...)
}
