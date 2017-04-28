package utils

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/rook/rook/e2e/framework/objects"
	"log"
	"os/exec"
	"strings"
	"syscall"
)

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

func ExecuteCmdWithEnv(Cmd string, cmdArgs []string, env []string) (stdout string, stderr string, exitCode int) {

	var outbuf, errbuf bytes.Buffer
	cmd := exec.Command(Cmd, strings.Join(cmdArgs, " "))

	cmd.Env = append(cmd.Env, env...)

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

//TODO add timeout parameter
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
