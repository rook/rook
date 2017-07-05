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

//K8sTransportClient is struct for perform kubectl operations on args
type K8sTransportClient struct {
}

//CreateNewk8sTransportClient creates new instance of transport interface for k8s
func CreateNewk8sTransportClient() *K8sTransportClient {
	return &K8sTransportClient{}
}

const (
	kubectlExecutable = "kubectl"
)

//ExecuteCmd executes kubectl commands
func (k *K8sTransportClient) ExecuteCmd(cmd []string) (stdout string, stderr string, err error) {
	return utils.ExecuteCmdAndLogToConsole(kubectlExecutable, cmd, []string{})

}

//Apply executes kubectl apply commands
func (k *K8sTransportClient) Apply(cmdArgs []string) (stdout string, stderr string, err error) {
	initialArgs := []string{"replace", "--force", "-f"}
	cmdArgs = append(initialArgs, cmdArgs...)
	return utils.ExecuteCmdAndLogToConsole("kubectl", cmdArgs, []string{})
}

//Execute executes kubectl commands
func (k *K8sTransportClient) Execute(cmdArgs []string, optional []string) (stdout string, stderr string, exitCode int) {

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

	return utils.ExecuteCmd(kubectlExecutable, cmdArgs)
}

//CreateWithStdin executes kubectl create commands and logs output
func (k *K8sTransportClient) CreateWithStdin(stdinText string) (stdout string, stderr string, exitCode int) {
	cmdStruct := objects.CommandArgs{Command: kubectlExecutable, PipeToStdIn: stdinText, CmdArgs: []string{"create", "-f", "-"}}

	cmdOut := utils.ExecuteCommand(cmdStruct)

	return cmdOut.StdOut, cmdOut.StdErr, cmdOut.ExitCode
}

//Create executes kubectl create commands
func (k *K8sTransportClient) Create(cmdArgs []string, optional []string) (stdout string, stderr string, exitCode int) {
	initialArgs := []string{"create", "-f"}
	cmdArgs = append(initialArgs, cmdArgs...)
	return utils.ExecuteCmd(kubectlExecutable, cmdArgs)
}

//Delete executes kubectl delete commands
func (k *K8sTransportClient) Delete(cmdArgs []string, optional []string) (stdout string, stderr string, exitCode int) {

	initialArgs := []string{"delete", "-f"}
	cmdArgs = append(initialArgs, cmdArgs...)
	return utils.ExecuteCmd(kubectlExecutable, cmdArgs)
}
