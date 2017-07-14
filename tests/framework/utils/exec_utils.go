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

	"github.com/rook/rook/tests/framework/objects"
)

//ExecuteCommand executes a os command and returs output
func ExecuteCommand(cmdStruct objects.CommandArgs) objects.CommandOut {
	var outBuffer, errBuffer bytes.Buffer

	cmd := exec.Command(cmdStruct.Command, cmdStruct.CmdArgs...)

	cmd.Env = append(cmd.Env, cmdStruct.EnvironmentVariable...)

	stdOut, err := cmd.StdoutPipe()

	if err != nil {
		return objects.CommandOut{StdErr: errBuffer.String(), StdOut: outBuffer.String(), Err: err}
	}

	stdin, err := cmd.StdinPipe()

	if err != nil {
		return objects.CommandOut{Err: err}
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
		return objects.CommandOut{StdErr: errBuffer.String(), StdOut: outBuffer.String(), Err: err}
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
		return objects.CommandOut{StdErr: errBuffer.String(), StdOut: outBuffer.String(), Err: err}
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
				return objects.CommandOut{StdErr: errBuffer.String(), StdOut: outBuffer.String(), Err: exiterr}
			}
		} else {
			return objects.CommandOut{StdErr: errBuffer.String(), StdOut: outBuffer.String(), Err: nil}
		}
	}

	return objects.CommandOut{StdErr: errBuffer.String(), StdOut: outBuffer.String(), Err: err}
}

//ExecuteCmd functions executes a command returs status,stdout and stderror
func ExecuteCmd(Cmd string, cmdArgs []string) (stdout string, stderr string, exitCode int) {

	var outbuf, errbuf bytes.Buffer
	cmd := exec.Command(Cmd, cmdArgs...)
	cmd.Stdout = &outbuf
	cmd.Stderr = &errbuf

	err := cmd.Run()
	stdout = outbuf.String()
	stderr = errbuf.String()

	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			ws := exitError.Sys().(syscall.WaitStatus)
			exitCode = ws.ExitStatus()
		} else {
			exitCode = 1
			if stderr == "" {
				stderr = err.Error() + stdout
			}
		}
	} else {
		ws := cmd.ProcessState.Sys().(syscall.WaitStatus)
		exitCode = ws.ExitStatus()
	}
	return
}

//ExecuteCmdAndLogToConsole functions executes a command returs status,stdout and stderror and logs output to console
func ExecuteCmdAndLogToConsole(command string, cmdArgs []string, cmdEnv []string) (stdout string, stderr string, err error) {
	var outbuf, errbuf bytes.Buffer

	cmd := exec.Command(command, cmdArgs...)

	cmd.Env = append(cmd.Env, cmdEnv...)

	stdOut, err := cmd.StdoutPipe()
	if err != nil {

		return errbuf.String(), outbuf.String(), err
	}

	defer stdOut.Close()

	scanner := bufio.NewScanner(stdOut)
	go func() {
		for scanner.Scan() {
			outbuf.WriteString(scanner.Text())
			fmt.Printf("%s\n", scanner.Text())
		}
	}()

	stdErr, err := cmd.StderrPipe()
	if err != nil {
		return errbuf.String(), outbuf.String(), err
	}

	defer stdErr.Close()

	stdErrScanner := bufio.NewScanner(stdErr)
	go func() {
		for stdErrScanner.Scan() {

			txt := stdErrScanner.Text()

			errbuf.WriteString(txt)
			fmt.Printf("%s\n", txt)
		}
	}()

	err = cmd.Start()
	if err != nil {

		return errbuf.String(), outbuf.String(), err
	}

	if err := cmd.Wait(); err != nil {
		if exiterr, ok := err.(*exec.ExitError); ok {
			// The program has exited with an exit code != 0
			if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				log.Printf("Exit Status: %d", status.ExitStatus())
				return errbuf.String(), outbuf.String(), exiterr
			}
		} else {
			return errbuf.String(), outbuf.String(), nil
		}
	}

	return errbuf.String(), outbuf.String(), err
}
