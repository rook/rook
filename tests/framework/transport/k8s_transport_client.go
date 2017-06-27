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
	"github.com/rook/rook/tests/framework/objects"
	"github.com/rook/rook/tests/framework/utils"
)

type k8sTransportClient struct {
}

func CreateNewk8sTransportClient() *k8sTransportClient {
	return &k8sTransportClient{}
}

const (
	kubectl_executable = "kubectl"
)

func (k *k8sTransportClient) ExecuteCmd(cmd []string) (stdout string, stderr string, err error) {
	return utils.ExecuteCmdAndLogToConsole(kubectl_executable, cmd, []string{})

}

func (k *k8sTransportClient) Apply(cmdArgs []string) (stdout string, stderr string, err error) {
	initialArgs := []string{"replace", "--force", "-f"}
	cmdArgs = append(initialArgs, cmdArgs...)
	return utils.ExecuteCmdAndLogToConsole("kubectl", cmdArgs, []string{})
}

func (k *k8sTransportClient) Execute(cmdArgs []string, optional []string) (stdout string, stderr string, exitCode int) {

	if optional != nil {
		if len(optional) == 1 {
			initialArgs := []string{"exec", optional[0], "--"}
			cmdArgs = append(initialArgs, cmdArgs...)
		} else if len(optional) == 2 {
			initialArgs := []string{"exec", "-n", optional[1], optional[0], "--"}
			cmdArgs = append(initialArgs, cmdArgs...)
		} else {
			return "", "invalid number of optional params used", 1
		}
	} else {
		initialArgs := []string{"exec", "-n", "rook", "rook-tools", "--"}
		cmdArgs = append(initialArgs, cmdArgs...)
	}

	return utils.ExecuteCmd(kubectl_executable, cmdArgs)
}

func (k *k8sTransportClient) CreateWithStdin(stdinText string) (stdout string, stderr string, exitCode int) {
	cmdStruct := objects.CommandArgs{Command: kubectl_executable, PipeToStdIn: stdinText, CmdArgs: []string{"create", "-f", "-"}}

	cmdOut := utils.ExecuteCommand(cmdStruct)

	return cmdOut.StdOut, cmdOut.StdErr, cmdOut.ExitCode
}

func (k *k8sTransportClient) Create(cmdArgs []string, optional []string) (stdout string, stderr string, exitCode int) {
	initialArgs := []string{"create", "-f"}
	cmdArgs = append(initialArgs, cmdArgs...)
	return utils.ExecuteCmd(kubectl_executable, cmdArgs)
}

func (k *k8sTransportClient) Delete(cmdArgs []string, optional []string) (stdout string, stderr string, exitCode int) {

	initialArgs := []string{"delete", "-f"}
	cmdArgs = append(initialArgs, cmdArgs...)
	return utils.ExecuteCmd(kubectl_executable, cmdArgs)
}
