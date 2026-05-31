/*
Copyright 2021 The Rook Authors. All rights reserved.

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

import "time"

// Retry executes the function ('f') 'count' times waiting for 'wait' duration between
// each attempt to run the function. Print the 'description' before all test info messages.
// Returns true the first time function 'f' returns true or false if 'f' never returns true.
func Retry(count uint16, wait time.Duration, description string, f func() bool) bool {
	for i := uint16(1); i < count+1; i++ {
		if f() {
			logger.Infof(description+": TRUE on attempt %d", i)
			return true
		}
		logger.Infof(description+": false on attempt %d. waiting %s seconds to retry", i, wait.String())
		time.Sleep(wait)
	}
	logger.Infof(description+": FALSE on all %d attempts", count)
	return false
}
