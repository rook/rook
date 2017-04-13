package transport

import (
	"github.com/dangula/rook/e2e/rook-test-framework/utils"
)

type k8sTransportClient struct {
}

func CreateNewk8sTransportClient() *k8sTransportClient {
	return &k8sTransportClient{}
}

const defaultFailedCode = 1

func (k *k8sTransportClient) ExecuteCmd(cmd []string) (stdout string, stderr string, err error) {
	return utils.ExecuteCmdAndLogToConsole("kubectl", cmd, []string{})
}

func (k *k8sTransportClient) Apply (cmdArgs []string) (stdout string, stderr string, err error) {
	initialArgs := []string{"replace", "--force",  "-f"}
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

	return utils.ExecuteCmd("kubectl", cmdArgs)
}

func (k *k8sTransportClient) Create(cmdArgs []string, optional []string) (stdout string, stderr string, exitCode int) {

	initialArgs := []string{"create", "-f"}
	cmdArgs = append(initialArgs, cmdArgs...)
	return utils.ExecuteCmd("kubectl", cmdArgs)
}

func (k *k8sTransportClient) Delete(cmdArgs []string, optional []string) (stdout string, stderr string, exitCode int) {

	initialArgs := []string{"delete", "-f"}
	cmdArgs = append(initialArgs, cmdArgs...)
	return utils.ExecuteCmd("kubectl", cmdArgs)
}
