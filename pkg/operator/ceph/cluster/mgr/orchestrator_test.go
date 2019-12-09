/*
Copyright 2018 The Rook Authors. All rights reserved.

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
package mgr

import (
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

func TestOrchestratorModules(t *testing.T) {
	executor := &exectest.MockExecutor{}
	context := &clusterd.Context{Executor: executor}
	orchestratorModuleEnabled := false
	rookModuleEnabled := false
	rookBackendSet := false
	backendErrorCount := 0
	executor.MockExecuteCommandWithOutputFile = func(debug bool, actionName, command, outputFile string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[0] == "mgr" && args[1] == "module" && args[2] == "enable" {
			if args[3] == "orchestrator_cli" {
				orchestratorModuleEnabled = true
				return "", nil
			}
			if args[3] == "rook" {
				rookModuleEnabled = true
				return "", nil
			}
		}
		return "", errors.Errorf("unexpected ceph command '%v'", args)
	}
	executor.MockExecuteCommandWithOutputFileTimeout = func(debug bool, timeout time.Duration, actionName, command, outputFile string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[0] == "orchestrator" && args[1] == "set" && args[2] == "backend" && args[3] == "rook" {
			if backendErrorCount < 5 {
				backendErrorCount++
				return "", errors.New("test simulation failure")
			}
			rookBackendSet = true
			return "", nil
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}

	clusterInfo := &cephconfig.ClusterInfo{
		CephVersion: cephver.Mimic,
	}

	c := &Cluster{clusterInfo: clusterInfo, context: context}
	c.exitCode = func(err error) (int, bool) {
		return invalidArgErrorCode, true
	}
	orchestratorInitWaitTime = 0

	// the modules are skipped on mimic
	c.clusterInfo.CephVersion = cephver.Mimic
	err := c.configureOrchestratorModules()
	assert.NoError(t, err)
	err = c.setRookOrchestratorBackend()
	assert.NoError(t, err)
	assert.False(t, orchestratorModuleEnabled)
	assert.False(t, rookModuleEnabled)
	assert.False(t, rookBackendSet)
	assert.Equal(t, 0, backendErrorCount)

	// the modules are configured on nautilus
	// the rook module will fail to be set
	c.clusterInfo.CephVersion = cephver.Nautilus
	err = c.configureOrchestratorModules()
	assert.Error(t, err)
	err = c.setRookOrchestratorBackend()
	assert.NoError(t, err)
	assert.True(t, orchestratorModuleEnabled)
	assert.True(t, rookModuleEnabled)
	assert.True(t, rookBackendSet)
	assert.Equal(t, 5, backendErrorCount)

	// the rook module will succeed
	err = c.configureOrchestratorModules()
	assert.NoError(t, err)
	err = c.setRookOrchestratorBackend()
	assert.NoError(t, err)
	assert.True(t, orchestratorModuleEnabled)
	assert.True(t, rookModuleEnabled)
	assert.True(t, rookBackendSet)
}
