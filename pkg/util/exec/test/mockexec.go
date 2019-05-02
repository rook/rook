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
	"os"
	"os/exec"
	"time"
)

// ******************** MockExecutor ********************
type MockExecutor struct {
	MockExecuteCommand                      func(debug bool, actionName string, command string, arg ...string) error
	MockStartExecuteCommand                 func(debug bool, actionName string, command string, arg ...string) (*exec.Cmd, error)
	MockExecuteCommandWithOutput            func(debug bool, actionName string, command string, arg ...string) (string, error)
	MockExecuteCommandWithCombinedOutput    func(debug bool, actionName string, command string, arg ...string) (string, error)
	MockExecuteCommandWithOutputFile        func(debug bool, actionName string, command, outfileArg string, arg ...string) (string, error)
	MockExecuteCommandWithOutputFileTimeout func(debug bool, timeout time.Duration, actionName string, command, outfileArg string, arg ...string) (string, error)
	MockExecuteCommandWithTimeout           func(debug bool, timeout time.Duration, actionName string, command string, arg ...string) (string, error)
	MockExecuteStat                         func(name string) (os.FileInfo, error)
}

func (e *MockExecutor) ExecuteCommand(debug bool, actionName string, command string, arg ...string) error {
	if e.MockExecuteCommand != nil {
		return e.MockExecuteCommand(debug, actionName, command, arg...)
	}

	return nil
}

func (e *MockExecutor) StartExecuteCommand(debug bool, actionName string, command string, arg ...string) (*exec.Cmd, error) {
	if e.MockStartExecuteCommand != nil {
		return e.MockStartExecuteCommand(debug, actionName, command, arg...)
	}

	args := []string{command}
	return &exec.Cmd{Args: append(args, arg...)}, nil
}

func (e *MockExecutor) ExecuteCommandWithOutput(debug bool, actionName string, command string, arg ...string) (string, error) {
	if e.MockExecuteCommandWithOutput != nil {
		return e.MockExecuteCommandWithOutput(debug, actionName, command, arg...)
	}

	return "", nil
}

func (e *MockExecutor) ExecuteCommandWithTimeout(debug bool, timeout time.Duration, actionName string, command string, arg ...string) (string, error) {

	if e.MockExecuteCommandWithTimeout != nil {
		return e.MockExecuteCommandWithTimeout(debug, time.Second, actionName, command, arg...)
	}

	return "", nil
}

func (e *MockExecutor) ExecuteCommandWithCombinedOutput(debug bool, actionName string, command string, arg ...string) (string, error) {
	if e.MockExecuteCommandWithCombinedOutput != nil {
		return e.MockExecuteCommandWithCombinedOutput(debug, actionName, command, arg...)
	}

	return "", nil
}

func (e *MockExecutor) ExecuteCommandWithOutputFile(debug bool, actionName string, command, outfileArg string, arg ...string) (string, error) {
	if e.MockExecuteCommandWithOutputFile != nil {
		return e.MockExecuteCommandWithOutputFile(debug, actionName, command, outfileArg, arg...)
	}

	return "", nil
}

func (e *MockExecutor) ExecuteCommandWithOutputFileTimeout(debug bool, timeout time.Duration, actionName string, command, outfileArg string, arg ...string) (string, error) {
	if e.MockExecuteCommandWithOutputFileTimeout != nil {
		return e.MockExecuteCommandWithOutputFileTimeout(debug, timeout, actionName, command, outfileArg, arg...)
	}

	return "", nil
}

func (e *MockExecutor) ExecuteStat(name string) (os.FileInfo, error) {
	if e.MockExecuteStat != nil {
		return e.MockExecuteStat(name)
	}

	return nil, nil
}
