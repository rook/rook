/*
Copyright 2019 The Rook Authors. All rights reserved.

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

package client

import (
	"testing"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

func TestEnableModuleRetries(t *testing.T) {
	moduleEnableRetries := 0
	moduleEnableWaitTime = 0
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutputFile = func(debug bool, actionName, command, outputFile string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		switch {
		case args[0] == "mgr" && args[1] == "module" && args[2] == "enable":
			if args[3] == "prometheus" || args[3] == "pg_autoscaler" || args[3] == "crash" {
				return "", nil
			}

		case args[0] == "mgr" && args[1] == "module" && args[2] == "disable":
			if args[3] == "prometheus" || args[3] == "pg_autoscaler" || args[3] == "crash" {
				return "", nil
			}
		}

		moduleEnableRetries = moduleEnableRetries + 1
		return "", errors.Errorf("unexpected ceph command %q", args)

	}

	_ = MgrEnableModule(&clusterd.Context{Executor: executor}, "clusterName", "invalidModuleName", false)
	assert.Equal(t, 5, moduleEnableRetries)

	moduleEnableRetries = 0
	_ = MgrEnableModule(&clusterd.Context{Executor: executor}, "clusterName", "pg_autoscaler", false)
	assert.Equal(t, 0, moduleEnableRetries)

}

func TestEnableModule(t *testing.T) {
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutputFile = func(debug bool, actionName, command, outputFile string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		switch {
		case args[0] == "mgr" && args[1] == "module" && args[2] == "enable":
			if args[3] == "prometheus" || args[3] == "pg_autoscaler" || args[3] == "crash" {
				return "", nil
			}

		case args[0] == "mgr" && args[1] == "module" && args[2] == "disable":
			if args[3] == "prometheus" || args[3] == "pg_autoscaler" || args[3] == "crash" {
				return "", nil
			}
		}

		return "", errors.Errorf("unexpected ceph command %q", args)
	}

	err := enableModule(&clusterd.Context{Executor: executor}, "clusterName", "pg_autoscaler", true, "enable")
	assert.NoError(t, err)

	err = enableModule(&clusterd.Context{Executor: executor}, "clusterName", "prometheus", true, "disable")
	assert.NoError(t, err)

	err = enableModule(&clusterd.Context{Executor: executor}, "clusterName", "invalidModuleName", false, "enable")
	assert.Error(t, err)

	err = enableModule(&clusterd.Context{Executor: executor}, "clusterName", "pg_autoscaler", false, "invalidCommandArgs")
	assert.Error(t, err)
}
