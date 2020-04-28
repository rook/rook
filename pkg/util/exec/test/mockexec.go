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
	"os/exec"
	"time"
)

// MockExecutor mocks all the exec commands
type MockExecutor struct {
	MockExecuteCommand                      func(command string, arg ...string) error
	MockExecuteCommandWithEnv               func(env []string, command string, arg ...string) error
	MockStartExecuteCommand                 func(command string, arg ...string) (*exec.Cmd, error)
	MockExecuteCommandWithOutput            func(command string, arg ...string) (string, error)
	MockExecuteCommandWithCombinedOutput    func(command string, arg ...string) (string, error)
	MockExecuteCommandWithOutputFile        func(command, outfileArg string, arg ...string) (string, error)
	MockExecuteCommandWithOutputFileTimeout func(timeout time.Duration, command, outfileArg string, arg ...string) (string, error)
	MockExecuteCommandWithTimeout           func(timeout time.Duration, command string, arg ...string) (string, error)
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

// ExecuteCommandWithOutputFile mocks ExecuteCommandWithOutputFile
func (e *MockExecutor) ExecuteCommandWithOutputFile(command, outfileArg string, arg ...string) (string, error) {
	if e.MockExecuteCommandWithOutputFile != nil {
		return e.MockExecuteCommandWithOutputFile(command, outfileArg, arg...)
	}

	return "", nil
}

// ExecuteCommandWithOutputFileTimeout mocks ExecuteCommandWithOutputFileTimeout
func (e *MockExecutor) ExecuteCommandWithOutputFileTimeout(timeout time.Duration, command, outfileArg string, arg ...string) (string, error) {
	if e.MockExecuteCommandWithOutputFileTimeout != nil {
		return e.MockExecuteCommandWithOutputFileTimeout(timeout, command, outfileArg, arg...)
	}

	return "", nil
}
