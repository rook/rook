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

// ******************** MockExecutor ********************
type MockExecutor struct {
	MockExecuteCommand                      func(suppressLogOutput bool, command string, arg ...string) error
	MockStartExecuteCommand                 func(suppressLogOutput bool, command string, arg ...string) (*exec.Cmd, error)
	MockExecuteCommandWithOutput            func(suppressLogOutput bool, command string, arg ...string) (string, error)
	MockExecuteCommandWithCombinedOutput    func(suppressLogOutput bool, command string, arg ...string) (string, error)
	MockExecuteCommandWithOutputFile        func(suppressLogOutput bool, command, outfileArg string, arg ...string) (string, error)
	MockExecuteCommandWithOutputFileTimeout func(suppressLogOutput bool, timeout time.Duration, command, outfileArg string, arg ...string) (string, error)
	MockExecuteCommandWithTimeout           func(suppressLogOutput bool, timeout time.Duration, command string, arg ...string) (string, error)
}

func (e *MockExecutor) ExecuteCommand(suppressLogOutput bool, command string, arg ...string) error {
	if e.MockExecuteCommand != nil {
		return e.MockExecuteCommand(suppressLogOutput, command, arg...)
	}

	return nil
}

func (e *MockExecutor) ExecuteCommandWithOutput(suppressLogOutput bool, command string, arg ...string) (string, error) {
	if e.MockExecuteCommandWithOutput != nil {
		return e.MockExecuteCommandWithOutput(suppressLogOutput, command, arg...)
	}

	return "", nil
}

func (e *MockExecutor) ExecuteCommandWithTimeout(suppressLogOutput bool, timeout time.Duration, command string, arg ...string) (string, error) {

	if e.MockExecuteCommandWithTimeout != nil {
		return e.MockExecuteCommandWithTimeout(suppressLogOutput, time.Second, command, arg...)
	}

	return "", nil
}

func (e *MockExecutor) ExecuteCommandWithCombinedOutput(suppressLogOutput bool, command string, arg ...string) (string, error) {
	if e.MockExecuteCommandWithCombinedOutput != nil {
		return e.MockExecuteCommandWithCombinedOutput(suppressLogOutput, command, arg...)
	}

	return "", nil
}

func (e *MockExecutor) ExecuteCommandWithOutputFile(suppressLogOutput bool, command, outfileArg string, arg ...string) (string, error) {
	if e.MockExecuteCommandWithOutputFile != nil {
		return e.MockExecuteCommandWithOutputFile(suppressLogOutput, command, outfileArg, arg...)
	}

	return "", nil
}

func (e *MockExecutor) ExecuteCommandWithOutputFileTimeout(suppressLogOutput bool, timeout time.Duration, command, outfileArg string, arg ...string) (string, error) {
	if e.MockExecuteCommandWithOutputFileTimeout != nil {
		return e.MockExecuteCommandWithOutputFileTimeout(suppressLogOutput, timeout, command, outfileArg, arg...)
	}

	return "", nil
}
