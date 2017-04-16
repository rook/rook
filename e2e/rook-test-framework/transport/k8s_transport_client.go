package transport

import (
	"github.com/dangula/rook/e2e/rook-test-framework/utils"
	"github.com/dangula/rook/e2e/rook-test-framework/objects"
)

type k8sTransportClient struct {
}

func CreateNewk8sTransportClient() *k8sTransportClient {
	return &k8sTransportClient{}
}

const (
	defaultFailedCode = 1
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
		initialArgs := []string{"exec", "-n", "rook", "rook-client", "--"}
		cmdArgs = append(initialArgs, cmdArgs...)
	}

	return utils.ExecuteCmd(kubectl_executable, cmdArgs)
}

func (k *k8sTransportClient) CreateWithStdin(stdinText string) (stdout string, stderr string, exitCode int) {
	cmdStruct := objects.Command_Args{Command: kubectl_executable, PipeToStdIn: stdinText, CmdArgs: []string{"create", "-f", "-"}}

	cmdOut := utils.ExecuteCommand(cmdStruct)

	return cmdOut.StdOut, cmdOut.StdErr, cmdOut.ExitCode
}

func (k *k8sTransportClient) Create(cmdArgs []string, optional []string) (stdout string, stderr string, exitCode int) {

	//cmdArgs = append([]string{"create", "-f"}, cmdArgs...)

	//cmdStruct := objects.Command_Args{Command: kubectl_executable, CmdArgs: cmdArgs}
	cmdStruct := objects.Command_Args{Command: kubectl_executable, PipeToStdIn: cmdArgs[0], CmdArgs: []string{"create", "-f", "-"}}

	cmdOut := utils.ExecuteCommand(cmdStruct)

	return cmdOut.StdOut, cmdOut.StdErr, cmdOut.ExitCode
}

func (k *k8sTransportClient) Delete(cmdArgs []string, optional []string) (stdout string, stderr string, exitCode int) {

	initialArgs := []string{"delete", "-f"}
	cmdArgs = append(initialArgs, cmdArgs...)
	return utils.ExecuteCmd(kubectl_executable, cmdArgs)
}
