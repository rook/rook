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
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	watchtools "k8s.io/client-go/tools/watch"
)

// namespacedDeleter is satisfied by any typed namespaced client (corev1
// ServiceInterface, cephv1 CephObjectStoreInterface, etc.). L is the
// resource's list type, inferred at the call site from the client's List
// return type.
type NamespacedDeleter[T, L runtime.Object] interface {
	Get(ctx context.Context, name string, opts metav1.GetOptions) (T, error) // only used to inference type of T
	List(ctx context.Context, opts metav1.ListOptions) (L, error)
	Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error)
	Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error
}

// AssertDelete deletes a single named resource and blocks until it is gone or
// the timeout elapses. Errors (including timeout) are reported via assert.Fail,
// which marks the test as failed but does not abort it, so a stuck finalizer on
// one resource does not prevent cleanup of the rest. Returns true on success.
//
// The wait is race-free against fast deletions: UntilWithSync performs an
// initial List (scoped to the resource name via a field selector) before
// starting the watch, and the precondition treats an empty store as "already
// gone". Subsequent deletion events are guaranteed to be delivered because the
// watch resumes from the List's resourceVersion.
//
// T is inferred from the client's Get return type and is used as the zero-value
// example object passed to UntilWithSync for internal type assertions.
func AssertDelete[T, L runtime.Object](
	ctx context.Context,
	t *testing.T,
	client NamespacedDeleter[T, L],
	name string,
	timeout time.Duration,
	msgAndArgs ...interface{},
) bool {
	t.Helper()
	var objType T

	fs := fields.OneTermEqualSelector("metadata.name", name).String()
	lw := &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			opts.FieldSelector = fs
			return client.List(ctx, opts)
		},
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			opts.FieldSelector = fs
			return client.Watch(ctx, opts)
		},
	}

	if err := client.Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		return assert.Fail(t, fmt.Sprintf("failed to delete %T: %s (%s)", objType, name, err), msgAndArgs...)
	}

	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	precondition := func(store cache.Store) (bool, error) {
		// lw is name-scoped, so an empty store unambiguously means the
		// object was already gone by the time the reflector's initial
		// list completed.
		return len(store.List()) == 0, nil
	}
	condition := func(ev watch.Event) (bool, error) {
		return ev.Type == watch.Deleted, nil
	}

	if _, err := watchtools.UntilWithSync(waitCtx, lw, objType, precondition, condition); err != nil {
		return assert.Fail(t, fmt.Sprintf("failed to delete %T: %s (%s)", objType, name, err), msgAndArgs...)
	}

	return true
}
