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

// Package mgr for the Ceph manager.
package mgr

import (
	"time"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/util/exec"
)

const (
	rookModuleName = "rook"
)

var orchestratorInitWaitTime = 5 * time.Second

// Ceph docs about the orchestrator modules: http://docs.ceph.com/docs/master/mgr/orchestrator_cli/
func (c *Cluster) configureOrchestratorModules() error {
	if err := client.MgrEnableModule(c.context, c.clusterInfo, rookModuleName, true); err != nil {
		return errors.Wrap(err, "failed to enable mgr rook module")
	}
	if err := c.setRookOrchestratorBackend(); err != nil {
		return errors.Wrap(err, "failed to set rook orchestrator backend")
	}
	return nil
}

func (c *Cluster) setRookOrchestratorBackend() error {
	// retry a few times in the case that the mgr module is not ready to accept commands
	_, err := client.ExecuteCephCommandWithRetry(func() (string, []byte, error) {
		args := []string{"orch", "set", "backend", "rook"}
		output, err := client.NewCephCommand(c.context, c.clusterInfo, args).RunWithTimeout(exec.CephCommandsTimeout)
		return "set rook backend", output, err
	}, 5, orchestratorInitWaitTime)
	if err != nil {
		return errors.Wrap(err, "failed to set rook as the orchestrator backend")
	}

	return nil
}
