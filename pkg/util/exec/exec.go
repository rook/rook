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
	"bufio"
	"io"
	"os/exec"
	"strings"

	"github.com/coreos/pkg/capnslog"
)

type Executor interface {
	StartExecuteCommand(actionName string, command string, arg ...string) (*exec.Cmd, error)
	ExecuteCommand(actionName string, command string, arg ...string) error
	ExecuteCommandPipeline(actionName string, command string) (string, error)
	ExecuteCommandWithOutput(actionName string, command string, arg ...string) (string, error)
}

type CommandExecutor struct {
}

// Start a process and return immediately
func (*CommandExecutor) StartExecuteCommand(actionName string, command string, arg ...string) (*exec.Cmd, error) {
	cmd, stdout, stderr, err := startCommand(command, arg...)
	if err != nil {
		return cmd, createCommandError(err, actionName)
	}

	go logOutput(stdout, stderr)

	return cmd, nil
}

// Start a process and wait for its completion
func (*CommandExecutor) ExecuteCommand(actionName string, command string, arg ...string) error {
	cmd, stdout, stderr, err := startCommand(command, arg...)
	if err != nil {
		return createCommandError(err, actionName)
	}

	logOutput(stdout, stderr)

	if err := cmd.Wait(); err != nil {
		return createCommandError(err, actionName)
	}

	return nil
}

func (*CommandExecutor) ExecuteCommandWithOutput(actionName string, command string, arg ...string) (string, error) {
	logCommand(command, arg...)
	cmd := exec.Command(command, arg...)
	return runCommandWithOutput(actionName, cmd)
}

func (*CommandExecutor) ExecuteCommandPipeline(actionName string, command string) (string, error) {
	logCommand(command)
	cmd := exec.Command("bash", "-c", command)
	return runCommandWithOutput(actionName, cmd)
}

func startCommand(command string, arg ...string) (*exec.Cmd, io.ReadCloser, io.ReadCloser, error) {
	logCommand(command, arg...)

	cmd := exec.Command(command, arg...)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	err := cmd.Start()

	return cmd, stdout, stderr, err
}

func logOutput(stdout, stderr io.ReadCloser) {
	if stdout == nil || stderr == nil {
		logger.Warningf("failed to collect stdout and stderr")
		return
	}

	// The logger for this exec package is responsible for logging output from other processes.
	// Those child processes should appropriately be outputting at the desired global level.  Therefore,
	// we always log at INFO level here, so that log statements from child procs at higher levels
	// (e.g., WARNING) will still be displayed.  We are relying on the child procs to output appropriately.
	if !logger.LevelAt(capnslog.INFO) {
		rl, err := capnslog.GetRepoLogger("github.com/rook/rook")
		if err == nil {
			rl.SetLogLevel(map[string]capnslog.LogLevel{"exec": capnslog.INFO})
		}
	}

	// read command's stdout line by line and write it to the log
	in := bufio.NewScanner(io.MultiReader(stdout, stderr))
	lastLine := ""
	for in.Scan() {
		lastLine = in.Text()
		logger.Infof(lastLine)
	}
}

func runCommandWithOutput(actionName string, cmd *exec.Cmd) (string, error) {
	output, err := cmd.Output()
	if err != nil {
		return "", createCommandError(err, actionName)
	}

	return strings.TrimSpace(string(output)), nil
}

func logCommand(command string, arg ...string) {
	logger.Infof("Running command: %s %s", command, strings.Join(arg, " "))
}
