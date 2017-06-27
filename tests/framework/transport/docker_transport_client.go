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

package transport

import (
	"github.com/rook/rook/tests/framework/utils"
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
