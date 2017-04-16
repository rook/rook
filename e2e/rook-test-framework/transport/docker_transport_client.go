package transport

import (
	"github.com/dangula/rook/e2e/rook-test-framework/utils"
)

type dockerClient struct {
	env []string
}

const docker_executable = "docker"

func CreateDockerClient(dockerEnv []string) *dockerClient {
	return &dockerClient{env: dockerEnv}
}

func (k *dockerClient) Execute(cmdArgs []string) (stdout string, stderr string, err error) {
	initialArgs := []string{"exec"}
	cmdArgs = append(initialArgs, cmdArgs...)

	return utils.ExecuteCmdAndLogToConsole(docker_executable, cmdArgs, k.env)
}

func (k *dockerClient) Run(cmdArgs []string) (stdout string, stderr string, err error) {
	initialArgs := []string{"run"}
	cmdArgs = append(initialArgs, cmdArgs...)

	 return utils.ExecuteCmdAndLogToConsole(docker_executable, cmdArgs, k.env)
}

func (k *dockerClient) Stop(cmdArgs []string) (stdout string, stderr string, err error) {
	initialArgs := []string{"stop"}
	cmdArgs = append(initialArgs, cmdArgs...)

	return utils.ExecuteCmdAndLogToConsole(docker_executable, cmdArgs, k.env)
}

func (k *dockerClient) ExecuteCmd(cmdArgs []string) (stdout string, stderr string, err error) {
	return utils.ExecuteCmdAndLogToConsole(docker_executable, cmdArgs, k.env)
}
