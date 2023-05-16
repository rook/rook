package main

import (
	"bytes"
	"fmt"
	"os/exec"
)

func (util *UtilityImpl) Exec(input []byte, name string, args ...string) ([]byte, []byte, error) {
	cmd := exec.Command(name, args...)
	if input != nil {
		cmd.Stdin = bytes.NewReader(input)
	}
	outBuf := new(bytes.Buffer)
	errBuf := new(bytes.Buffer)
	cmd.Stdout = outBuf
	cmd.Stderr = errBuf
	err := cmd.Run()
	if err != nil {
		return nil, nil, fmt.Errorf("command failed. name: %s, stdout: %s, stderr: %s, err: %v", name, outBuf.String(), errBuf.String(), err)
	}
	return outBuf.Bytes(), errBuf.Bytes(), nil
}

func (util *UtilityImpl) Kubectl(ns string, args ...string) ([]byte, []byte, error) {
	if ns == "" {
		return util.Exec(nil, "kubectl", args...)
	} else {
		passedArgs := []string{"-n", ns}
		passedArgs = append(passedArgs, args...)
		return util.Exec(nil, "kubectl", passedArgs...)
	}
}

func (util *UtilityImpl) Ceph(ns string, args ...string) ([]byte, []byte, error) {
	passedArgs := []string{"exec", "deploy/rook-ceph-tools", "--", "ceph"}
	passedArgs = append(passedArgs, args...)
	return util.Kubectl(ns, passedArgs...)
}
