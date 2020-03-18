/*
Copyright 2016 The Rook Authors. All rights reserved.

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
package exec

import (
	"os/exec"
	"syscall"
)

func ExitStatus(err error) (int, bool) {
	exitErr, ok := err.(*exec.ExitError)
	if ok {
		waitStatus, ok := exitErr.ProcessState.Sys().(syscall.WaitStatus)
		if ok {
			return waitStatus.ExitStatus(), true
		}
	}
	return 0, false
}
