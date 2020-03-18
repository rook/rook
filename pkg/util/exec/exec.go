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
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/coreos/pkg/capnslog"
)

type Executor interface {
	ExecuteCommand(suppressLogOutput bool, command string, arg ...string) error
	ExecuteCommandWithOutput(suppressLogCommand bool, command string, arg ...string) (string, error)
	ExecuteCommandWithCombinedOutput(suppressLogOutput bool, command string, arg ...string) (string, error)
	ExecuteCommandWithOutputFile(suppressLogOutput bool, command, outfileArg string, arg ...string) (string, error)
	ExecuteCommandWithOutputFileTimeout(suppressLogOutput bool, timeout time.Duration, command, outfileArg string, arg ...string) (string, error)
	ExecuteCommandWithTimeout(suppressLogOutput bool, timeout time.Duration, command string, arg ...string) (string, error)
}

type CommandExecutor struct {
}

// Start a process and wait for its completion
func (*CommandExecutor) ExecuteCommand(suppressLogOutput bool, command string, arg ...string) error {
	cmd, stdout, stderr, err := startCommand(suppressLogOutput, command, arg...)
	if err != nil {
		return err
	}

	logOutput(suppressLogOutput, stdout, stderr)

	if err := cmd.Wait(); err != nil {
		return err
	}

	return nil
}

// ExecuteCommandWithTimeout starts a process and wait for its completion with timeout.
func (*CommandExecutor) ExecuteCommandWithTimeout(suppressLogOutput bool, timeout time.Duration, command string, arg ...string) (string, error) {
	logCommand(suppressLogOutput, command, arg...)
	cmd := exec.Command(command, arg...)

	var b bytes.Buffer
	cmd.Stdout = &b
	cmd.Stderr = &b

	if err := cmd.Start(); err != nil {
		return "", err
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	interruptSent := false
	for {
		select {
		case <-time.After(timeout):
			if interruptSent {
				logger.Infof("timeout waiting for process %s to return after interrupt signal was sent. Sending kill signal to the process", command)
				var e error
				if err := cmd.Process.Kill(); err != nil {
					logger.Errorf("Failed to kill process %s: %+v", command, err)
					e = fmt.Errorf("timeout waiting for the command %s to return after interrupt signal was sent. Tried to kill the process but that failed: %+v", command, err)
				} else {
					e = fmt.Errorf("timeout waiting for the command %s to return", command)
				}
				return strings.TrimSpace(b.String()), e
			}

			logger.Infof("timeout waiting for process %s to return. Sending interrupt signal to the process", command)
			if err := cmd.Process.Signal(os.Interrupt); err != nil {
				logger.Errorf("Failed to send interrupt signal to process %s: %+v", command, err)
				// kill signal will be sent next loop
			}
			interruptSent = true
		case err := <-done:
			if err != nil {
				return strings.TrimSpace(b.String()), err
			}
			if interruptSent {
				return strings.TrimSpace(b.String()), fmt.Errorf("timeout waiting for the command %s to return", command)
			}
			return strings.TrimSpace(b.String()), nil
		}
	}
}

func (*CommandExecutor) ExecuteCommandWithOutput(suppressLogOutput bool, command string, arg ...string) (string, error) {
	logCommand(suppressLogOutput, command, arg...)
	cmd := exec.Command(command, arg...)
	return runCommandWithOutput(cmd, false)
}

func (*CommandExecutor) ExecuteCommandWithCombinedOutput(suppressLogOutput bool, command string, arg ...string) (string, error) {
	logCommand(suppressLogOutput, command, arg...)
	cmd := exec.Command(command, arg...)
	return runCommandWithOutput(cmd, true)
}

// Same as ExecuteCommandWithOutputFile but with a timeout limit.
func (*CommandExecutor) ExecuteCommandWithOutputFileTimeout(suppressLogOutput bool, timeout time.Duration,
	command, outfileArg string, arg ...string) (string, error) {

	outFile, err := ioutil.TempFile("", "")
	if err != nil {
		return "", fmt.Errorf("failed to open output file: %+v", err)
	}
	defer outFile.Close()
	defer os.Remove(outFile.Name())

	arg = append(arg, outfileArg, outFile.Name())
	logCommand(suppressLogOutput, command, arg...)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, command, arg...)
	cmdOut, err := cmd.CombinedOutput()

	// if there was anything that went to stdout/stderr then log it, even before
	// we return an error
	if string(cmdOut) != "" {
		writeToLog(suppressLogOutput, string(cmdOut))
	}

	if ctx.Err() == context.DeadlineExceeded {
		return string(cmdOut), ctx.Err()
	}

	if err != nil {
		return string(cmdOut), err
	}

	fileOut, err := ioutil.ReadAll(outFile)
	return string(fileOut), err
}

func (*CommandExecutor) ExecuteCommandWithOutputFile(suppressLogOutput bool, command, outfileArg string, arg ...string) (string, error) {

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

	logCommand(suppressLogOutput, command, arg...)
	cmd := exec.Command(command, arg...)
	cmdOut, err := cmd.CombinedOutput()
	// if there was anything that went to stdout/stderr then log it, even before we return an error
	if string(cmdOut) != "" {
		writeToLog(suppressLogOutput, string(cmdOut))
	}
	if err != nil {
		return string(cmdOut), err
	}

	// read the entire output file and return that to the caller
	fileOut, err := ioutil.ReadAll(outFile)
	return string(fileOut), err
}

func startCommand(suppressLogOutput bool, command string, arg ...string) (*exec.Cmd, io.ReadCloser, io.ReadCloser, error) {
	logCommand(suppressLogOutput, command, arg...)

	cmd := exec.Command(command, arg...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		logger.Warningf("failed to open stdout pipe: %+v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		logger.Warningf("failed to open stderr pipe: %+v", err)
	}

	err = cmd.Start()

	return cmd, stdout, stderr, err
}

// read from reader line by line and write it to the log
func logFromReader(suppressLogOutput bool, logger *capnslog.PackageLogger, reader io.ReadCloser) {
	in := bufio.NewScanner(reader)
	lastLine := ""
	for in.Scan() {
		lastLine = in.Text()
		writeToLog(suppressLogOutput, lastLine)
	}
}

func logOutput(suppressLogOutput bool, stdout, stderr io.ReadCloser) {
	if stdout == nil || stderr == nil {
		logger.Warningf("failed to collect stdout and stderr")
		return
	}

	// The child processes should appropriately be outputting at the desired global level.  Therefore,
	// we always log at INFO level here, so that log statements from child procs at higher levels
	// (e.g., WARNING) will still be displayed.  We are relying on the child procs to output appropriately.
	childLogger := capnslog.NewPackageLogger("github.com/rook/rook", "exec")
	if !childLogger.LevelAt(capnslog.INFO) {
		rl, err := capnslog.GetRepoLogger("github.com/rook/rook")
		if err == nil {
			rl.SetLogLevel(map[string]capnslog.LogLevel{"exec": capnslog.INFO})
		}
	}

	go logFromReader(suppressLogOutput, childLogger, stderr)
	logFromReader(suppressLogOutput, childLogger, stdout)
}

func runCommandWithOutput(cmd *exec.Cmd, combinedOutput bool) (string, error) {
	var output []byte
	var err error

	if combinedOutput {
		output, err = cmd.CombinedOutput()
	} else {
		output, err = cmd.Output()
	}

	out := strings.TrimSpace(string(output))

	if err != nil {
		return out, err
	}

	return out, nil
}

func logCommand(suppressLogOutput bool, command string, arg ...string) {
	msg := fmt.Sprintf("Running command: %s %s", command, strings.Join(arg, " "))
	writeToLog(suppressLogOutput, msg)
}

func writeToLog(suppressLogOutput bool, msg string) {
	if suppressLogOutput {
		logger.Debug(msg)
	} else {
		logger.Info(msg)
	}
}
