package proc

import (
	"fmt"
	"os"
	"os/exec"
)

// BUGUBG: we could use a better process manager here with support for rediecting
// stdout, stderr, and signals. And can monitor the child.

func RunChildProcess(daemon string, args ...string) error {
	cmd := createCmd(daemon, args...)
	return cmd.Run()
}

func StartChildProcess(daemon string, args ...string) (cmd *exec.Cmd, err error) {
	cmd = createCmd(daemon, args...)
	err = cmd.Start()
	if err != nil {
		return nil, err
	}
	return cmd, nil
}

func StopChildProcess(cmd *exec.Cmd) error {
	return cmd.Process.Kill()
}

func createCmd(daemon string, args ...string) (cmd *exec.Cmd) {
	cmd = exec.Command(os.Args[0], append([]string{"daemon", fmt.Sprintf("--type=%s", daemon), "--"}, args...)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd
}
