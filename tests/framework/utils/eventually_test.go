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
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventuallySucceedsImmediately(t *testing.T) {
	calls := 0
	err := Eventually(context.Background(), t, time.Minute, "immediate success", func(context.Context) error {
		calls++
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
}

func TestEventuallyRetriesUntilSuccess(t *testing.T) {
	calls := 0
	err := Eventually(context.Background(), t, time.Minute, "second attempt succeeds", func(context.Context) error {
		calls++
		if calls < 2 {
			return errors.New("not yet")
		}
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 2, calls)
}

func TestEventuallyTimeoutWrapsLastError(t *testing.T) {
	cause := errors.New("still broken")
	err := Eventually(context.Background(), t, 10*time.Millisecond, "never succeeds", func(context.Context) error {
		return cause
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, cause)
	assert.Contains(t, err.Error(), "never succeeds")
}

func TestEventuallyParentCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := Eventually(ctx, t, time.Minute, "canceled parent", func(context.Context) error {
		return errors.New("not yet")
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestEventuallyCondSeesOverallDeadline(t *testing.T) {
	err := Eventually(context.Background(), t, 10*time.Millisecond, "cond honors ctx", func(ctx context.Context) error {
		// a cooperative operation blocks only until the loop deadline
		<-ctx.Done()
		return ctx.Err()
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestAttemptTimeoutCancelsHungAttempt(t *testing.T) {
	calls := 0
	err := Eventually(context.Background(), t, time.Minute, "hung attempt is canceled and retried",
		AttemptTimeout(10*time.Millisecond, func(ctx context.Context) error {
			calls++
			if calls < 2 {
				// simulate an operation that hangs until canceled
				<-ctx.Done()
				return ctx.Err()
			}
			return nil
		}))
	require.NoError(t, err)
	assert.Equal(t, 2, calls)
}
