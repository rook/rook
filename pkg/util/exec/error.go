package exec

import (
	"fmt"
	"os/exec"
	"syscall"
)

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
