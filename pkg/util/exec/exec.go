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
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"

	"github.com/coreos/pkg/capnslog"
)

type Executor interface {
	StartExecuteCommand(actionName string, command string, arg ...string) (*exec.Cmd, error)
	ExecuteCommand(actionName string, command string, arg ...string) error
	ExecuteCommandWithOutput(actionName string, command string, arg ...string) (string, error)
	ExecuteCommandWithCombinedOutput(actionName string, command string, arg ...string) (string, error)
	ExecuteCommandWithOutputFile(actionName, command, outfileArg string, arg ...string) (string, error)
	ExecuteStat(name string) (os.FileInfo, error)
}

type CommandExecutor struct {
}

// Start a process and return immediately
func (*CommandExecutor) StartExecuteCommand(actionName string, command string, arg ...string) (*exec.Cmd, error) {
	cmd, stdout, stderr, err := startCommand(command, arg...)
	if err != nil {
		return cmd, createCommandError(err, actionName)
	}

	go logOutput(actionName, stdout, stderr)

	return cmd, nil
}

// Start a process and wait for its completion
func (*CommandExecutor) ExecuteCommand(actionName string, command string, arg ...string) error {
	cmd, stdout, stderr, err := startCommand(command, arg...)
	if err != nil {
		return createCommandError(err, actionName)
	}

	logOutput(actionName, stdout, stderr)

	if err := cmd.Wait(); err != nil {
		return createCommandError(err, actionName)
	}

	return nil
}

func (*CommandExecutor) ExecuteCommandWithOutput(actionName string, command string, arg ...string) (string, error) {
	logCommand(command, arg...)
	cmd := exec.Command(command, arg...)
	return runCommandWithOutput(actionName, cmd, false)
}

func (*CommandExecutor) ExecuteCommandWithCombinedOutput(actionName string, command string, arg ...string) (string, error) {
	logCommand(command, arg...)
	cmd := exec.Command(command, arg...)
	return runCommandWithOutput(actionName, cmd, true)
}

func (*CommandExecutor) ExecuteCommandWithOutputFile(actionName, command, outfileArg string, arg ...string) (string, error) {

	// create a temporary file to serve as the output file for the command to be run and ensure
	// it is cleaned up after this function is done
	outFile, err := ioutil.TempFile("", "")
	if err != nil {
		return "", fmt.Errorf("failed to open output file: %+v", err)
	}
	defer outFile.Close()
	defer os.Remove(outFile.Name())

	// append the output file argument to the list or args
	arg = append(arg, outfileArg, outFile.Name())

	logCommand(command, arg...)
	cmd := exec.Command(command, arg...)
	cmdOut, err := runCommandWithOutput(actionName, cmd, false)
	if err != nil {
		return cmdOut, err
	}

	// if there was anything that went to stdout/stderr then log it
	if cmdOut != "" {
		logger.Info(cmdOut)
	}

	// read the entire output file and return that to the caller
	fileOut, err := ioutil.ReadAll(outFile)
	return string(fileOut), err
}

func startCommand(command string, arg ...string) (*exec.Cmd, io.ReadCloser, io.ReadCloser, error) {
	logCommand(command, arg...)

	cmd := exec.Command(command, arg...)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	err := cmd.Start()

	return cmd, stdout, stderr, err
}

func (*CommandExecutor) ExecuteStat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}

func logOutput(name string, stdout, stderr io.ReadCloser) {
	if stdout == nil || stderr == nil {
		logger.Warningf("failed to collect stdout and stderr")
		return
	}

	// The child processes should appropriately be outputting at the desired global level.  Therefore,
	// we always log at INFO level here, so that log statements from child procs at higher levels
	// (e.g., WARNING) will still be displayed.  We are relying on the child procs to output appropriately.
	childLogger := capnslog.NewPackageLogger("github.com/rook/rook", name)
	if !childLogger.LevelAt(capnslog.INFO) {
		rl, err := capnslog.GetRepoLogger("github.com/rook/rook")
		if err == nil {
			rl.SetLogLevel(map[string]capnslog.LogLevel{name: capnslog.INFO})
		}
	}

	// read command's stdout line by line and write it to the log
	in := bufio.NewScanner(io.MultiReader(stdout, stderr))
	lastLine := ""
	for in.Scan() {
		lastLine = in.Text()
		childLogger.Infof(lastLine)
	}
}

func runCommandWithOutput(actionName string, cmd *exec.Cmd, combinedOutput bool) (string, error) {
	var output []byte
	var err error

	if combinedOutput {
		output, err = cmd.CombinedOutput()
	} else {
		output, err = cmd.Output()
	}

	out := strings.TrimSpace(string(output))

	if err != nil {
		return out, createCommandError(err, actionName)
	}

	return out, nil
}

func logCommand(command string, arg ...string) {
	logger.Infof("Running command: %s %s", command, strings.Join(arg, " "))
}
