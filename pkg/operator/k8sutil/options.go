/*
Copyright 2019 The Rook Authors. All rights reserved.

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

package k8sutil

import (
	"time"
)

// WaitOptions are a common set of options controlling the behavior of k8sutil operations. If
// WaitOptions are specified, the operation should wait in a loop and verify that the operation
// being performed was successful.
type WaitOptions struct {
	// Wait defines whether the operation should wait in a loop and verify that the operation was
	// successful before returning.
	Wait bool

	// RetryCount defines how many times the operation should retry verification in the wait loop
	// before giving up. If RetryCount is zero, the operation should default to a sane value based
	// on the operation.
	RetryCount uint

	// RetryInterval defines the time the operation will wait before retrying verification. If
	// RetryInterval is zero, the operation should default to a sane value based on the operation.
	RetryInterval time.Duration

	// ErrorOnTimeout defines whether the operation should time out with an error. If ErrorOnTimeout
	// is true and the operation times out, the operation is considered a failure. If ErrorOnTimeout
	// is false and the operation times out, the operation should log a warning but not report
	// failure.
	ErrorOnTimeout bool
}

func (w *WaitOptions) retryCountOrDefault(def uint) uint {
	if w.RetryCount > 0 {
		return w.RetryCount
	}
	return def
}

func (w *WaitOptions) retryIntervalOrDefault(def time.Duration) time.Duration {
	if w.RetryInterval > 0 {
		return w.RetryInterval
	}
	return def
}
