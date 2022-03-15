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
	"os"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/coreos/pkg/capnslog"
	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	kexec "k8s.io/utils/exec"
)

var (
	CephCommandsTimeout = 15 * time.Second
	NoCommandTimout     = 0 * time.Second
)

// Executor is the main interface for all the exec commands
type Executor interface {
	ExecuteCommand(command string, arg ...string) error
	ExecuteCommandWithEnv(env []string, command string, arg ...string) error
	ExecuteCommandWithOutput(timeout time.Duration, command string, arg ...string) (string, error)
	ExecuteCommandWithCombinedOutput(timeout time.Duration, command string, arg ...string) (string, error)
	ExecuteCommandWithTimeout(timeout time.Duration, command string, arg ...string) (string, error)
}

// CommandExecutor is the type of the Executor
type CommandExecutor struct{}

// ExecuteCommand starts a process and wait for its completion
func (c *CommandExecutor) ExecuteCommand(command string, arg ...string) error {
	return c.ExecuteCommandWithEnv([]string{}, command, arg...)
}

// ExecuteCommandWithEnv starts a process with env variables and wait for its completion
func (*CommandExecutor) ExecuteCommandWithEnv(env []string, command string, arg ...string) error {
	cmd, stdout, stderr, err := startCommand(env, command, arg...)
	if err != nil {
		return err
	}

	logOutput(stdout, stderr)

	if err := cmd.Wait(); err != nil {
		return err
	}

	return nil
}

// ExecuteCommandWithTimeout starts a process and wait for its completion with timeout.
func (*CommandExecutor) ExecuteCommandWithTimeout(timeout time.Duration, command string, arg ...string) (string, error) {
	logCommand(command, arg...)
	// #nosec G204 Rook controls the input to the exec arguments
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
					logger.Errorf("Failed to kill process %s: %v", command, err)
					e = fmt.Errorf("timeout waiting for the command %s to return after interrupt signal was sent. Tried to kill the process but that failed: %v", command, err)
				} else {
					e = fmt.Errorf("timeout waiting for the command %s to return", command)
				}
				return strings.TrimSpace(b.String()), e
			}

			logger.Infof("timeout waiting for process %s to return. Sending interrupt signal to the process", command)
			if err := cmd.Process.Signal(os.Interrupt); err != nil {
				logger.Errorf("Failed to send interrupt signal to process %s: %v", command, err)
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

// ExecuteCommandWithOutput executes a command with output
func (*CommandExecutor) ExecuteCommandWithOutput(timeout time.Duration, command string, arg ...string) (string, error) {
	logCommand(command, arg...)
	// #nosec G204 Rook controls the input to the exec arguments
	return runCommandWithOutput(timeout, command, false, arg...)
}

// ExecuteCommandWithCombinedOutput executes a command with combined output
func (*CommandExecutor) ExecuteCommandWithCombinedOutput(timeout time.Duration, command string, arg ...string) (string, error) {
	logCommand(command, arg...)
	// #nosec G204 Rook controls the input to the exec arguments
	return runCommandWithOutput(timeout, command, true, arg...)
}

func startCommand(env []string, command string, arg ...string) (*exec.Cmd, io.ReadCloser, io.ReadCloser, error) {
	logCommand(command, arg...)

	// #nosec G204 Rook controls the input to the exec arguments
	cmd := exec.Command(command, arg...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		logger.Warningf("failed to open stdout pipe: %+v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		logger.Warningf("failed to open stderr pipe: %+v", err)
	}

	if len(env) > 0 {
		cmd.Env = env
	}

	err = cmd.Start()

	return cmd, stdout, stderr, err
}

// read from reader line by line and write it to the log
func logFromReader(logger *capnslog.PackageLogger, reader io.ReadCloser) {
	l := logger.Debug
	// If we are an OSD we must log using Info to print out stdout/stderr
	if os.Getenv("ROOK_OSD_ID") != "" {
		l = logger.Info
	}
	in := bufio.NewScanner(reader)
	lastLine := ""
	for in.Scan() {
		lastLine = in.Text()
		l(lastLine)
	}
}

func logOutput(stdout, stderr io.ReadCloser) {
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

	go logFromReader(childLogger, stderr)
	logFromReader(childLogger, stdout)
}

func runCommandWithOutput(timeout time.Duration, command string, combinedOutput bool, args ...string) (string, error) {
	var output []byte
	var err error
	var out string
	var cmd *exec.Cmd

	logCommand(command, args...)

	if timeout == 0 {
		// #nosec G204 Rook controls the input to the exec arguments
		cmd = exec.Command(command, args...)
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		// #nosec G204 Rook controls the input to the exec arguments
		cmd = exec.CommandContext(ctx, command, args...)
	}

	if combinedOutput {
		output, err = cmd.CombinedOutput()
	} else {
		output, err = cmd.Output()
		if err != nil {
			output = []byte(fmt.Sprintf("%s. %s", string(output), assertErrorType(err)))
		}
	}

	out = strings.TrimSpace(string(output))

	if err != nil {
		return out, err
	}

	return out, nil
}

func logCommand(command string, arg ...string) {
	logger.Debugf("Running command with: %s %s", command, strings.Join(arg, " "))
}

func assertErrorType(err error) string {
	switch errType := err.(type) {
	case *exec.ExitError:
		return string(errType.Stderr)
	case *exec.Error:
		return errType.Error()
	}

	return ""
}

// ExtractExitCode attempts to get the exit code from the error returned by an Executor function.
// This should also work for any errors returned by the golang os/exec package and "k8s.io/utils/exec"
func ExtractExitCode(err error) (int, error) {
	switch errType := err.(type) {
	case *exec.ExitError:
		return errType.ExitCode(), nil

	case *kexec.CodeExitError:
		return errType.ExitStatus(), nil

	// have to check both *kexec.CodeExitError and kexec.CodeExitError because CodeExitError methods
	// are not defined with pointer receivers; both pointer and non-pointers are valid `error`s.
	case kexec.CodeExitError:
		return errType.ExitStatus(), nil

	case *kerrors.StatusError:
		return int(errType.ErrStatus.Code), nil

	default:
		logger.Debugf(err.Error())
		// This is ugly, but it's a decent backup just in case the error isn't a type above.
		if strings.Contains(err.Error(), "command terminated with exit code") {
			a := strings.SplitAfter(err.Error(), "command terminated with exit code")
			return strconv.Atoi(strings.TrimSpace(a[1]))
		}
		return -1, errors.Errorf("error %#v is an unknown error type: %v", err, reflect.TypeOf(err))
	}
}
