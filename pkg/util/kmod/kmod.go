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
package kmod

import (
	"fmt"

	"github.com/rook/rook/pkg/util/exec"
)

func LoadKernelModule(name string, options []string, executor exec.Executor) error {
	if options == nil {
		options = []string{}
	}

	args := append([]string{"modprobe", name}, options...)

	if err := executor.ExecuteCommand(fmt.Sprintf("modprobe %s", name), "sudo", args[:]...); err != nil {
		return fmt.Errorf("failed to load kernel module %s: %+v", name, err)
	}

	return nil
}

func CheckKernelModuleParam(name, param string, executor exec.Executor) (bool, error) {
	cmd := fmt.Sprintf(`modinfo -F parm %s | grep "^%s" | awk '{print $0}'`, name, param)
	out, err := executor.ExecuteCommandPipeline("check kmod param", cmd)
	if err != nil {
		return false, fmt.Errorf("failed to check for %s module %s param: %+v", name, param, err)
	}

	return out != "", nil
}
