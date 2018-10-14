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

package utils

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"syscall"

	"github.com/coreos/pkg/capnslog"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "testutil")

// CommandArgs is a warpper for cmd args
type CommandArgs struct {
	Command             string
	SubCommand          string
	CmdArgs             []string
	OptionalArgs        []string
	PipeToStdIn         string
	EnvironmentVariable []string
}

// CommandOut is a wrapper for cmd out returned after executing command args
type CommandOut struct {
	StdOut   string
	StdErr   string
	ExitCode int
	Err      error
}

// ExecuteCommand executes a os command with stdin and returns output
func ExecuteCommand(cmdStruct CommandArgs) CommandOut {
	logger.Infof("Running %s %v", cmdStruct.Command, cmdStruct.CmdArgs)

	var outBuffer, errBuffer bytes.Buffer

	cmd := exec.Command(cmdStruct.Command, cmdStruct.CmdArgs...)

	cmd.Env = append(cmd.Env, cmdStruct.EnvironmentVariable...)

	stdOut, err := cmd.StdoutPipe()

	if err != nil {
		return CommandOut{StdErr: errBuffer.String(), StdOut: outBuffer.String(), Err: err}
	}

	stdin, err := cmd.StdinPipe()

	if err != nil {
		return CommandOut{Err: err}
	}

	defer stdOut.Close()

	scanner := bufio.NewScanner(stdOut)
	go func() {
		for scanner.Scan() {
			outBuffer.WriteString(scanner.Text())
			fmt.Printf("%s\n", scanner.Text())
		}
	}()

	stdErr, err := cmd.StderrPipe()

	if err != nil {
		return CommandOut{StdErr: errBuffer.String(), StdOut: outBuffer.String(), Err: err}
	}

	defer stdErr.Close()

	stdErrScanner := bufio.NewScanner(stdErr)
	go func() {
		for stdErrScanner.Scan() {

			txt := stdErrScanner.Text()

			if !strings.Contains(txt, "no buildable Go source files in") {
				errBuffer.WriteString(txt)
				fmt.Printf("%s\n", txt)
			}
		}
	}()

	err = cmd.Start()
	if err != nil {
		return CommandOut{StdErr: errBuffer.String(), StdOut: outBuffer.String(), Err: err}
	}

	if cmdStruct.PipeToStdIn != "" {
		stdin.Write([]byte(cmdStruct.PipeToStdIn))
		stdin.Close()
	}

	if err := cmd.Wait(); err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			// The program has exited with an exit code != 0
			if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				log.Printf("Exit Status: %d", status.ExitStatus())
				return CommandOut{StdErr: errBuffer.String(), StdOut: outBuffer.String(), ExitCode: status.ExitStatus(), Err: exiterr}
			}
		} else {
			return CommandOut{StdErr: errBuffer.String(), StdOut: outBuffer.String(), Err: nil}
		}
	}

	return CommandOut{StdErr: errBuffer.String(), StdOut: outBuffer.String(), Err: err}
}
