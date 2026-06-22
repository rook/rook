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

package utils

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// eventuallyPollInterval is the cadence of Eventually. It is deliberately not
// a parameter: consumers poll cheap idempotent operations (kubectl applies,
// rgw admin / S3 reads), and uniform call sites beat another knob. Add a
// variant if a real need for a different cadence ever appears.
const eventuallyPollInterval = time.Second

// Eventually polls cond on the calling goroutine every second until it
// returns nil, the timeout elapses, or ctx is canceled. Each failed attempt
// is logged via t.Logf so CI output shows liveness attributed to the right
// (sub)test, and the last attempt's error is wrapped into the returned error.
//
// The context passed to cond is derived from ctx with the overall timeout
// applied, so a cond that honors it stops at the deadline instead of
// overrunning it. To additionally bound each attempt, wrap cond with
// AttemptTimeout.
//
// cond must not call assert or require: report transient failures by
// returning an error, and make assertions on captured state after the wait.
// Eventually never runs cond on another goroutine — a hung attempt would keep
// executing concurrently with its retry, and require's FailNow is unsafe off
// the test goroutine (which is also why testify's assert.Eventually is not
// used here).
func Eventually(ctx context.Context, t *testing.T, timeout time.Duration, desc string, cond func(context.Context) error) error {
	t.Helper()

	loopCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for attempt := 1; ; attempt++ {
		err := cond(loopCtx)
		if err == nil {
			return nil
		}
		t.Logf("waiting for %s (attempt %d): %v", desc, attempt, err)

		select {
		case <-loopCtx.Done():
			if cerr := ctx.Err(); cerr != nil {
				return fmt.Errorf("canceled while waiting for %s: %w (last error: %v)", desc, cerr, err)
			}
			return fmt.Errorf("condition not met within %s: %s: %w", timeout, desc, err)
		case <-time.After(eventuallyPollInterval):
		}
	}
}

// AttemptTimeout bounds each attempt of cond with its own deadline, for
// operations that can hang but would succeed if canceled and retried. The
// bound is cooperative: cond only stops early if it honors the context it is
// given (requests built with http.NewRequestWithContext, exec.CommandContext,
// SDK *WithContext calls, ...). A cond that ignores its context is unaffected
// by AttemptTimeout:
//
//	// WRONG: http.Get ignores ctx, so nothing cancels a hung fetch
//	utils.AttemptTimeout(10*time.Second, func(ctx context.Context) error {
//		_, err := http.Get(url)
//		return err
//	})
//
// Operations that cannot honor a context must be bounded at the operation
// level instead (e.g. http.Client{Timeout: ...}).
func AttemptTimeout(d time.Duration, cond func(context.Context) error) func(context.Context) error {
	return func(ctx context.Context) error {
		attemptCtx, cancel := context.WithTimeout(ctx, d)
		defer cancel()
		return cond(attemptCtx)
	}
}
