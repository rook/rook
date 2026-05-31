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

	"github.com/pkg/errors"
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
			return fmt.Errorf("max retries exceeded, last err: %v", err)
		}

		logger.Infof("retrying after %v, last error: %v", delay, err)
		<-time.After(delay)
	}
}

// RetryFunc is a function that returns true when it is done and it should be retried no longer.
// It should return error if there has been an error. The error will be logged if done==false
// (should keep retrying). The error will be returned by the calling function if done==true (should
// stop retrying).
type RetryFunc func() (done bool, err error)

// RetryWithTimeout retries the RetryFunc until the timeout occurs. It will retry once more after
// the timeout to avoid race conditions at the expense of running for slightly longer than timeout
// in the timeout/error case.
// The given description will be output in log/error messages as "... waiting for <description>..."
func RetryWithTimeout(f RetryFunc, period time.Duration, timeout time.Duration, description string) error {
	tt := time.After(timeout)
	var done bool
	var err error
	for {
		done, err = f()
		if done {
			break
		}
		if err != nil {
			logger.Errorf("error occurred waiting for %s. retrying after %v. %v", description, period, err)
		}
		logger.Debugf("waiting for %s. retrying after %v", description, period)

		select {
		case <-time.After(period):
			// go back to start of loop
		case <-tt:
			// timeout; try one last time
			done, err = f()
			if !done {
				if err != nil {
					return errors.Wrapf(err, "timed out waiting for %s", description)
				}
				return fmt.Errorf("timed out waiting for %s", description)
			}
			return err
		}
	}

	return err
}
