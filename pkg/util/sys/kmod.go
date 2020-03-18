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
package sys

import (
	"fmt"
	"os/exec"
	"strings"

	pkgexec "github.com/rook/rook/pkg/util/exec"
)

func getKernelVersion() (string, error) {
	var output []byte
	cmd := exec.Command("uname", "-r")
	output, err := cmd.Output()
	out := strings.TrimSpace(string(output))
	if err != nil {
		return out, err
	}

	return out, nil
}

func IsBuiltinKernelModule(name string, executor pkgexec.Executor) (bool, error) {
	kv, err := getKernelVersion()
	if err != nil {
		return false, fmt.Errorf("failed to get kernel version: %+v", err)
	}

	kv = fmt.Sprintf("/lib/modules/%s/modules.builtin", kv)
	out, err := executor.ExecuteCommandWithCombinedOutput("cat", kv)
	if err != nil {
		return false, fmt.Errorf("failed to cat %s: %+v", kv, err)
	}

	result := Grep(out, name)
	return result != "", nil
}

func LoadKernelModule(name string, options []string, executor pkgexec.Executor) error {
	if options == nil {
		options = []string{}
	}

	args := append([]string{name}, options...)

	if err := executor.ExecuteCommand("modprobe", args[:]...); err != nil {
		return fmt.Errorf("failed to load kernel module %s: %+v", name, err)
	}

	return nil
}

func CheckKernelModuleParam(name, param string, executor pkgexec.Executor) (bool, error) {
	out, err := executor.ExecuteCommandWithOutput("modinfo", "-F", "parm", name)
	if err != nil {
		return false, fmt.Errorf("failed to check for %s module %s param: %+v", name, param, err)
	}

	result := Grep(out, fmt.Sprintf("^%s", param))
	return result != "", nil
}
