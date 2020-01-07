/*
Copyright 2017 The Rook Authors. All rights reserved.

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
	"fmt"
	"time"
)

// Retry will attempt the given function until it succeeds, up to the given maximum amount of retries,
// sleeping for the given duration in between attempts.
func Retry(maxRetries int, delay time.Duration, f func() error) error {
	tries := 0
	for {
		err := f()
		if err == nil {
			// function succeeded, all done
			return nil
		}

		tries++
		if tries > maxRetries {
			return fmt.Errorf("max retries exceeded, last err: %+v", err)
		}

		logger.Infof("retrying after %v, last error: %v", delay, err)
		<-time.After(delay)
	}
}
