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
	"context"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	cephclient "github.com/rook/rook/pkg/daemon/ceph/client"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
	"github.com/rook/rook/pkg/util/exec"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/stretchr/testify/assert"
)

func TestOrchestratorModules(t *testing.T) {
	executor := &exectest.MockExecutor{}
	rookModuleEnabled := false
	rookBackendSet := false
	backendErrorCount := 0
	exec.CephCommandsTimeout = 15 * time.Second
	executor.MockExecuteCommandWithOutput = func(command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[0] == "mgr" && args[1] == "module" && args[2] == "enable" {
			if args[3] == "rook" {
				rookModuleEnabled = true
				return "", nil
			}
		}
		return "", errors.Errorf("unexpected ceph command '%v'", args)
	}
	executor.MockExecuteCommandWithTimeout = func(timeout time.Duration, command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[0] == "orch" && args[1] == "set" && args[2] == "backend" && args[3] == "rook" {
			if backendErrorCount < 5 {
				backendErrorCount++
				return "", errors.New("test simulation failure")
			}
			rookBackendSet = true
			return "", nil
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}

	clusterInfo := &cephclient.ClusterInfo{
		CephVersion: cephver.Squid,
		Context:     context.TODO(),
	}
	context := &clusterd.Context{Executor: executor}

	c := &Cluster{clusterInfo: clusterInfo, context: context}
	c.exitCode = func(err error) (int, bool) {
		return invalidArgErrorCode, true
	}
	orchestratorInitWaitTime = 0

	err := c.configureOrchestratorModules()
	assert.Error(t, err)
	err = c.setRookOrchestratorBackend()
	assert.NoError(t, err)
	assert.True(t, rookModuleEnabled)
	assert.True(t, rookBackendSet)
	assert.Equal(t, 5, backendErrorCount)

	// the rook module will succeed
	err = c.configureOrchestratorModules()
	assert.NoError(t, err)
	err = c.setRookOrchestratorBackend()
	assert.NoError(t, err)
	assert.True(t, rookModuleEnabled)
	assert.True(t, rookBackendSet)

	c.clusterInfo.CephVersion = cephver.Squid
	err = c.setRookOrchestratorBackend()
	assert.NoError(t, err)
	executor.MockExecuteCommandWithTimeout = func(timeout time.Duration, command string, args ...string) (string, error) {
		logger.Infof("Command: %s %v", command, args)
		if args[0] == "orch" && args[1] == "set" && args[2] == "backend" && args[3] == "rook" {
			if backendErrorCount < 5 {
				backendErrorCount++
				return "", errors.New("test simulation failure")
			}
			rookBackendSet = true
			return "", nil
		}
		return "", errors.Errorf("unexpected ceph command %q", args)
	}
	err = c.setRookOrchestratorBackend()
	assert.NoError(t, err)
}
