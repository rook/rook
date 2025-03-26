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

// Package exec embeds Rook's exec logic
package exec

import (
	"fmt"
	"os/exec"
	"syscall"

	kexec "k8s.io/client-go/util/exec"
)

// CephCLIError is Ceph CLI Error type
type CephCLIError struct {
	err    error
	output string
}

func (e *CephCLIError) Error() string {
	return fmt.Sprintf("%v", e.output)
}

// ExitStatus looks for the exec error code
func ExitStatus(err error) (int, bool) {
	switch e := err.(type) {
	case *exec.ExitError:
		waitStatus, ok := e.ProcessState.Sys().(syscall.WaitStatus)
		if ok {
			return waitStatus.ExitStatus(), true
		}
	case kexec.CodeExitError:
		return int(e.ExitStatus()), true
	case *CephCLIError:
		return ExitStatus(e.err)
	case syscall.Errno:
		return int(e), true
	}
	return 0, false
}
