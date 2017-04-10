package uitls

import (
	"bytes"
	"os/exec"
	"syscall"
	"bufio"
	"fmt"
	"strings"
)

func ExecuteCmd(Cmd string, cmdArgs []string) (stdout string, stderr string, exitCode int) {

	var outbuf, errbuf bytes.Buffer
	cmd := exec.Command(Cmd, cmdArgs...)
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
			exitCode = 1
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

//TODO add timeout parameter
func ExecuteCmdAndLogToConsole(command string, cmdArgs []string, cmdEnv []string) (stdout string, stderr string, err error)  {
	var outbuf, errbuf bytes.Buffer

	cmd := exec.Command(command, cmdArgs...)

	cmd.Env = append(cmd.Env, cmdEnv...)

	stdOut, err := cmd.StdoutPipe()
	if err != nil {
		return  "", "", err
	}

	defer stdOut.Close()

	scanner := bufio.NewScanner(stdOut)
	go func() {
		for scanner.Scan() {
			outbuf.WriteString(scanner.Text())
			fmt.Printf("%s\n", scanner.Text())
		}
	}()


	stdErr, err := cmd.StderrPipe()
	if err != nil {
		return  "", "", err
	}

	defer stdErr.Close()

	stdErrScanner := bufio.NewScanner(stdErr)
	go func()  {
		for stdErrScanner.Scan() {

			txt := stdErrScanner.Text()

			if !strings.Contains(txt, "no buildable Go source files in") {
				errbuf.WriteString(txt)
				fmt.Printf("%s\n", txt)
			}
		}
	}()

	err = cmd.Start()
	if err != nil {
		return  "", "", err
	}

	err = cmd.Wait()
	// go generate command will fail when no generate command find.
	if err != nil {
		if err.Error() != "exit status 1" {
			return  "", "", err
		}
	}

	return errbuf.String(), outbuf.String(), nil
}

