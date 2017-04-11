package transport

import (
	"bytes"
	"github.com/dangula/rook/e2e/rook-test-framework/utils"
	"os/exec"
	"syscall"
)

type dockerClient struct {
	env []string
}

func CreateDockerClient(dockerEnv []string) *dockerClient {
	return &dockerClient{env: dockerEnv}
}

func (k *dockerClient) Execute(cmdArgs []string) (stdout string, stderr string, err error) {
	initialArgs := []string{"exec"}
	cmdArgs = append(initialArgs, cmdArgs...)

	return utils.ExecuteCmdAndLogToConsole("docker", cmdArgs, k.env)
}

func (k *dockerClient) Run(cmdArgs []string) (stdout string, stderr string, err error) {
	initialArgs := []string{"run"}
	cmdArgs = append(initialArgs, cmdArgs...)

	return utils.ExecuteCmdAndLogToConsole("docker", cmdArgs, k.env)
}

func (k *dockerClient) Stop(cmdArgs []string) (stdout string, stderr string, exitCode int) {

	var outbuf, errbuf bytes.Buffer
	initialArgs := []string{"stop"}
	cmdArgs = append(initialArgs, cmdArgs...)
	cmd := exec.Command("docker", cmdArgs...)
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
			exitCode = defaultFailedCode
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
