package kmod

import (
	"fmt"

	"github.com/quantum/castle/pkg/util/exec"
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
