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
	"time"
)

// TranslateCommandExecutor is an exec.Executor that translates every command before executing it
// This is useful to run the commands in a job with `kubectl run ...` when running the operator outside
// of Kubernetes and need to run tools that require running inside the cluster.
type TranslateCommandExecutor struct {

	// Executor is probably a exec.CommandExecutor that will run the translated commands
	Executor Executor

	// Translator translates every command before running it
	Translator func(command string, arg ...string) (string, []string)
}

// ExecuteCommand starts a process and wait for its completion
func (e *TranslateCommandExecutor) ExecuteCommand(command string, arg ...string) error {
	transCommand, transArgs := e.Translator(command, arg...)
	return e.Executor.ExecuteCommand(transCommand, transArgs...)
}

// ExecuteCommandWithStdin starts a process, provides stdin and wait for its completion with timeout.
func (e *TranslateCommandExecutor) ExecuteCommandWithStdin(timeout time.Duration, command string, stdin *string, arg ...string) error {
	transCommand, transArgs := e.Translator(command, arg...)
	return e.Executor.ExecuteCommandWithStdin(timeout, transCommand, stdin, transArgs...)
}

// ExecuteCommandWithEnv starts a process with an env variable and wait for its completion
func (e *TranslateCommandExecutor) ExecuteCommandWithEnv(env []string, command string, arg ...string) error {
	transCommand, transArgs := e.Translator(command, arg...)
	return e.Executor.ExecuteCommandWithEnv(env, transCommand, transArgs...)
}

// ExecuteCommandWithOutput starts a process and wait for its completion
func (e *TranslateCommandExecutor) ExecuteCommandWithOutput(command string, arg ...string) (string, error) {
	transCommand, transArgs := e.Translator(command, arg...)
	return e.Executor.ExecuteCommandWithOutput(transCommand, transArgs...)
}

// ExecuteCommandWithCombinedOutput starts a process and returns its stdout and stderr combined.
func (e *TranslateCommandExecutor) ExecuteCommandWithCombinedOutput(command string, arg ...string) (string, error) {
	transCommand, transArgs := e.Translator(command, arg...)
	return e.Executor.ExecuteCommandWithCombinedOutput(transCommand, transArgs...)
}

// ExecuteCommandWithTimeout starts a process and wait for its completion with timeout.
func (e *TranslateCommandExecutor) ExecuteCommandWithTimeout(timeout time.Duration, command string, arg ...string) (string, error) {
	transCommand, transArgs := e.Translator(command, arg...)
	return e.Executor.ExecuteCommandWithTimeout(timeout, transCommand, transArgs...)
}
