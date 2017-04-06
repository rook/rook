package transport

import (
	"bytes"
	"os/exec"
	"syscall"
)

type k8sTransportClient struct {
}

func CreateNewk8sTransportClient() *k8sTransportClient {
	return &k8sTransportClient{}
}

const defaultFailedCode = 1

func (k *k8sTransportClient) Execute(cmdArgs []string) (stdout string, stderr string, exitCode int) {

	var outbuf, errbuf bytes.Buffer
	initialArgs := []string{"exec", "-n", "rook", "rook-client", "--"}
	cmdArgs = append(initialArgs, cmdArgs...)
	cmd := exec.Command("kubectl", cmdArgs...)
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

func (k *k8sTransportClient) Create(cmdArgs []string) (stdout string, stderr string, exitCode int) {

	var outbuf, errbuf bytes.Buffer
	initialArgs := []string{"create", "-f"}
	cmdArgs = append(initialArgs, cmdArgs...)
	cmd := exec.Command("kubectl", cmdArgs...)
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

func (k *k8sTransportClient) Delete(cmdArgs []string) (stdout string, stderr string, exitCode int) {

	var outbuf, errbuf bytes.Buffer
	initialArgs := []string{"delete", "-f"}
	cmdArgs = append(initialArgs, cmdArgs...)
	cmd := exec.Command("kubectl", cmdArgs...)
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
