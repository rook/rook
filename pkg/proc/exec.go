package proc

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"
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

func createCommandError(err error, actionName string) error {
	return errors.New(fmt.Sprintf("Failed to complete %s. err=%s", actionName, err.Error()))
}

// ******************** MockExecutor ********************
type MockExecutor struct {
	MockExecuteCommand           func(actionName string, command string, arg ...string) error
	MockExecuteCommandPipeline   func(actionName string, command string) (string, error)
	MockExecuteCommandWithOutput func(actionName string, command string, arg ...string) (string, error)
}

func (e *MockExecutor) ExecuteCommand(actionName string, command string, arg ...string) error {
	if e.MockExecuteCommand != nil {
		return e.MockExecuteCommand(actionName, command, arg...)
	}

	return nil
}

func (e *MockExecutor) ExecuteCommandPipeline(actionName string, command string) (string, error) {
	if e.MockExecuteCommandPipeline != nil {
		return e.MockExecuteCommandPipeline(actionName, command)
	}

	return "", nil
}

func (e *MockExecutor) ExecuteCommandWithOutput(actionName string, command string, arg ...string) (string, error) {
	if e.MockExecuteCommandWithOutput != nil {
		return e.MockExecuteCommandWithOutput(actionName, command, arg...)
	}

	return "", nil
}
