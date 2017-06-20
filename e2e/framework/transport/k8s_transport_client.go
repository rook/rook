package transport

import (
	"github.com/rook/rook/e2e/framework/objects"
	"github.com/rook/rook/e2e/framework/utils"
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
