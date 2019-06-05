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
	"fmt"
	"os"
	"os/exec"
	"time"
)

// TranslateCommandExecutor is an exec.Executor that translates every command before executing it
// This is useful to run the commands in a job with `kubectl run ...` when running the operator outside
// of Kubernetes and need to run tools that require running inside the cluster.
type TranslateCommandExecutor struct {

	// Executor is probably a exec.CommandExecutor that will run the translated commands
	Executor Executor

	// Translator translates every command before running it
	Translator func(debug bool, actionName string, command string, arg ...string) (string, []string)
}

// StartExecuteCommand starts a process and return immediately
func (e *TranslateCommandExecutor) StartExecuteCommand(debug bool, actionName string, command string, arg ...string) (*exec.Cmd, error) {
	transCommand, transArgs := e.Translator(debug, actionName, command, arg...)
	return e.Executor.StartExecuteCommand(debug, actionName, transCommand, transArgs...)
}

// ExecuteCommand starts a process and wait for its completion
func (e *TranslateCommandExecutor) ExecuteCommand(debug bool, actionName string, command string, arg ...string) error {
	transCommand, transArgs := e.Translator(debug, actionName, command, arg...)
	return e.Executor.ExecuteCommand(debug, actionName, transCommand, transArgs...)
}

// ExecuteCommandWithOutput starts a process and wait for its completion
func (e *TranslateCommandExecutor) ExecuteCommandWithOutput(debug bool, actionName string, command string, arg ...string) (string, error) {
	transCommand, transArgs := e.Translator(debug, actionName, command, arg...)
	return e.Executor.ExecuteCommandWithOutput(debug, actionName, transCommand, transArgs...)
}

// ExecuteCommandWithCombinedOutput starts a process and returns its stdout and stderr combined.
func (e *TranslateCommandExecutor) ExecuteCommandWithCombinedOutput(debug bool, actionName string, command string, arg ...string) (string, error) {
	transCommand, transArgs := e.Translator(debug, actionName, command, arg...)
	return e.Executor.ExecuteCommandWithCombinedOutput(debug, actionName, transCommand, transArgs...)
}

// ExecuteCommandWithOutputFile starts a process and saves output to file
func (e *TranslateCommandExecutor) ExecuteCommandWithOutputFile(debug bool, actionName string, command, outfileArg string, arg ...string) (string, error) {
	transCommand, transArgs := e.Translator(debug, actionName, command, arg...)
	return e.Executor.ExecuteCommandWithOutputFile(debug, actionName, transCommand, outfileArg, transArgs...)
}

// ExecuteCommandWithOutputFileTimeout is the same as ExecuteCommandWithOutputFile but with a timeout limit.
func (e *TranslateCommandExecutor) ExecuteCommandWithOutputFileTimeout(
	debug bool, timeout time.Duration, actionName string,
	command, outfileArg string, arg ...string) (string, error) {
	transCommand, transArgs := e.Translator(debug, actionName, command, arg...)
	return e.Executor.ExecuteCommandWithOutputFileTimeout(debug, timeout, actionName, transCommand, outfileArg, transArgs...)
}

// ExecuteCommandWithTimeout starts a process and wait for its completion with timeout.
func (e *TranslateCommandExecutor) ExecuteCommandWithTimeout(debug bool, timeout time.Duration, actionName string, command string, arg ...string) (string, error) {
	transCommand, transArgs := e.Translator(debug, actionName, command, arg...)
	return e.Executor.ExecuteCommandWithTimeout(debug, timeout, actionName, transCommand, transArgs...)
}

// ExecuteStat returns a file stat
func (e *TranslateCommandExecutor) ExecuteStat(name string) (os.FileInfo, error) {
	return nil, fmt.Errorf("TODO: TranslateCommandExecutor.ExecuteStat() not implemented ... is it needed?")
}
