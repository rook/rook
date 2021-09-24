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

package test

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"
)

// MockExecutor mocks all the exec commands
type MockExecutor struct {
	MockExecuteCommand                   func(command string, arg ...string) error
	MockExecuteCommandWithEnv            func(env []string, command string, arg ...string) error
	MockStartExecuteCommand              func(command string, arg ...string) (*exec.Cmd, error)
	MockExecuteCommandWithOutput         func(command string, arg ...string) (string, error)
	MockExecuteCommandWithCombinedOutput func(command string, arg ...string) (string, error)
	MockExecuteCommandWithTimeout        func(timeout time.Duration, command string, arg ...string) (string, error)
}

// ExecuteCommand mocks ExecuteCommand
func (e *MockExecutor) ExecuteCommand(command string, arg ...string) error {
	if e.MockExecuteCommand != nil {
		return e.MockExecuteCommand(command, arg...)
	}

	return nil
}

// ExecuteCommandWithEnv mocks ExecuteCommandWithEnv
func (e *MockExecutor) ExecuteCommandWithEnv(env []string, command string, arg ...string) error {
	if e.MockExecuteCommandWithEnv != nil {
		return e.MockExecuteCommandWithEnv(env, command, arg...)
	}

	return nil
}

// ExecuteCommandWithOutput mocks ExecuteCommandWithOutput
func (e *MockExecutor) ExecuteCommandWithOutput(command string, arg ...string) (string, error) {
	if e.MockExecuteCommandWithOutput != nil {
		return e.MockExecuteCommandWithOutput(command, arg...)
	}

	return "", nil
}

// ExecuteCommandWithTimeout mocks ExecuteCommandWithTimeout
func (e *MockExecutor) ExecuteCommandWithTimeout(timeout time.Duration, command string, arg ...string) (string, error) {

	if e.MockExecuteCommandWithTimeout != nil {
		return e.MockExecuteCommandWithTimeout(time.Second, command, arg...)
	}

	return "", nil
}

// ExecuteCommandWithCombinedOutput mocks ExecuteCommandWithCombinedOutput
func (e *MockExecutor) ExecuteCommandWithCombinedOutput(command string, arg ...string) (string, error) {
	if e.MockExecuteCommandWithCombinedOutput != nil {
		return e.MockExecuteCommandWithCombinedOutput(command, arg...)
	}

	return "", nil
}

// Mock an executed command with the desired return values.
// STDERR is returned *before* STDOUT.
//
// This will return an error if the given exit code is nonzero. The error return is the primary
// benefit of using this method.
//
// In order for this to work in a `*_test.go` file, you MUST import TestMockExecHelperProcess
// exactly as shown below:
//   import exectest "github.com/rook/rook/pkg/util/exec/test"
//   // import TestMockExecHelperProcess
//   func TestMockExecHelperProcess(t *testing.T) {
//   	exectest.TestMockExecHelperProcess(t)
//   }
// Inspired by: https://github.com/golang/go/blob/master/src/os/exec/exec_test.go
func MockExecCommandReturns(t *testing.T, stdout, stderr string, retcode int) error {
	cmd := exec.Command(os.Args[0], "-test.run=TestMockExecHelperProcess") //nolint:gosec //Rook controls the input to the exec arguments
	cmd.Env = append(os.Environ(),
		"GO_WANT_HELPER_PROCESS=1",
		fmt.Sprintf("GO_HELPER_PROCESS_STDOUT=%s", stdout),
		fmt.Sprintf("GO_HELPER_PROCESS_STDERR=%s", stderr),
		fmt.Sprintf("GO_HELPER_PROCESS_RETCODE=%d", retcode),
	)
	err := cmd.Run()
	return err
}

// TestHelperProcess isn't a real test. It's used as a helper process for MockExecCommandReturns to
// simulate output from a command. Notably, this can return a realistic os/exec error.
// Inspired by: https://github.com/golang/go/blob/master/src/os/exec/exec_test.go
func TestMockExecHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	// test should set these in its environment to control the output of the test commands
	fmt.Fprint(os.Stderr, os.Getenv("GO_HELPER_PROCESS_STDERR")) // return stderr before stdout
	fmt.Fprint(os.Stdout, os.Getenv("GO_HELPER_PROCESS_STDOUT"))
	rc, err := strconv.Atoi(os.Getenv("GO_HELPER_PROCESS_RETCODE"))
	if err != nil {
		panic(err)
	}
	os.Exit(rc)
}
