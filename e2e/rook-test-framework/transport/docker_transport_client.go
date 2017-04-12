package transport

import (

	"github.com/dangula/rook/e2e/rook-test-framework/utils"
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

func (k *dockerClient) Stop(cmdArgs []string) (stdout string, stderr string, err error) {
	initialArgs := []string{"stop"}
	cmdArgs = append(initialArgs, cmdArgs...)

	return utils.ExecuteCmdAndLogToConsole("docker",  cmdArgs, k.env)
}
