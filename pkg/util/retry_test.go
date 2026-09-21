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

package util

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRetryWithContext(t *testing.T) {
	t.Run("succeeds on first attempt", func(t *testing.T) {
		calls := 0
		err := RetryWithContext(context.Background(), 5, time.Millisecond, func() error {
			calls++
			return nil
		})
		assert.NoError(t, err)
		assert.Equal(t, 1, calls)
	})

	t.Run("succeeds after some retries", func(t *testing.T) {
		calls := 0
		err := RetryWithContext(context.Background(), 5, time.Millisecond, func() error {
			calls++
			if calls < 3 {
				return errors.New("not yet")
			}
			return nil
		})
		assert.NoError(t, err)
		assert.Equal(t, 3, calls)
	})

	t.Run("returns last error if retries are exhausted", func(t *testing.T) {
		calls := 0
		wantErr := errors.New("always fails")
		err := RetryWithContext(context.Background(), 3, time.Millisecond, func() error {
			calls++
			return wantErr
		})
		assert.Error(t, err)
		// initial attempt + 3 retries
		assert.Equal(t, 4, calls)
		assert.True(t, errors.Is(err, wantErr))
	})

	t.Run("returns immediately when context is already cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		calls := 0
		err := RetryWithContext(ctx, 5, time.Minute, func() error {
			calls++
			return errors.New("should not be called")
		})
		assert.ErrorIs(t, err, context.Canceled)
		assert.Equal(t, 0, calls)
	})

	t.Run("aborts during delay instead of consuming full budget", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		calls := 0
		// Cancel during the first attempt. The "delay" gets skipped because
		// the context is already cancelled, so the call returns at once.
		err := RetryWithContext(ctx, 10, time.Minute, func() error {
			calls++
			cancel()
			return errors.New("busy")
		})
		assert.ErrorIs(t, err, context.Canceled)
		// loop returned after first attempt because the context was cancelled during the first attempt itself.
		assert.Equal(t, 1, calls)
	})
}
