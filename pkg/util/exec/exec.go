package exec

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"
	"syscall"
)

type Executor interface {
	ExecuteCommand(actionName string, command string, arg ...string) error
	ExecuteCommandPipeline(actionName string, command string) (string, error)
	ExecuteCommandWithOutput(actionName string, command string, arg ...string) (string, error)
}

type CommandExecutor struct {
}

func (*CommandExecutor) ExecuteCommand(actionName string, command string, arg ...string) error {
	cmd := exec.Command(command, arg...)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	logCommand(command, arg...)
	if err := cmd.Start(); err != nil {
		return errors.New(fmt.Sprintf("Failed to start action '%s': %+v", actionName, err))
	}

	// read command's stdout line by line and write it to the log
	in := bufio.NewScanner(io.MultiReader(stdout, stderr))
	lastLine := ""
	for in.Scan() {
		lastLine = in.Text()
		log.Printf(lastLine)
	}

	if err := cmd.Wait(); err != nil {
		return createCommandError(err, actionName)
	}

	return nil
}

func (*CommandExecutor) ExecuteCommandWithOutput(actionName string, command string, arg ...string) (string, error) {
	cmd := exec.Command(command, arg...)
	logCommand(command, arg...)
	return runCommandWithOutput(actionName, cmd)
}

func (*CommandExecutor) ExecuteCommandPipeline(actionName string, command string) (string, error) {
	cmd := exec.Command("bash", "-c", command)

	logCommand(command)
	return runCommandWithOutput(actionName, cmd)
}

func runCommandWithOutput(actionName string, cmd *exec.Cmd) (string, error) {
	output, err := cmd.Output()
	if err != nil {
		return "", createCommandError(err, actionName)
	}

	return strings.TrimSpace(string(output)), nil
}

func logCommand(command string, arg ...string) {
	log.Printf("Running command: %s %s", command, strings.Join(arg, " "))
}

type CommandError struct {
	ActionName string
	Err        error
}

func (e *CommandError) Error() string {
	return fmt.Sprintf("Failed to complete %s: %+v", e.ActionName, e.Err)
}

func (e *CommandError) ExitStatus() int {
	exitStatus := -1
	exitErr, ok := e.Err.(*exec.ExitError)
	if ok {
		waitStatus, ok := exitErr.ProcessState.Sys().(syscall.WaitStatus)
		if ok {
			exitStatus = waitStatus.ExitStatus()
		}
	}
	return exitStatus
}

func createCommandError(err error, actionName string) error {
	return &CommandError{ActionName: actionName, Err: err}
}
