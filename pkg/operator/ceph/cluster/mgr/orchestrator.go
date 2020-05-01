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
)

const (
	orchestratorModuleName = "orchestrator_cli"
	rookModuleName         = "rook"
	orchestratorOldCLIName = "orchestrator"
	orchestratorNewCLIName = "orch"
)

var (
	orchestratorInitWaitTime = 5 * time.Second
	orchestratorCLIName      = orchestratorOldCLIName
)

// Ceph docs about the orchestrator modules: http://docs.ceph.com/docs/master/mgr/orchestrator_cli/
func (c *Cluster) configureOrchestratorModules() error {
	if err := client.MgrEnableModule(c.context, c.Namespace, rookModuleName, true); err != nil {
		return errors.Wrap(err, "failed to enable mgr rook module")
	}
	if err := client.MgrEnableModule(c.context, c.Namespace, orchestratorModuleName, true); err != nil {
		return errors.Wrap(err, "failed to enable mgr orchestrator module")
	}
	if err := c.setRookOrchestratorBackend(); err != nil {
		return errors.Wrap(err, "failed to set rook orchestrator backend")
	}
	return nil
}

func (c *Cluster) setRookOrchestratorBackend() error {
	if c.clusterInfo.CephVersion.IsAtLeastOctopus() {
		orchestratorCLIName = orchestratorNewCLIName
	}
	// retry a few times in the case that the mgr module is not ready to accept commands
	_, err := client.ExecuteCephCommandWithRetry(func() (string, []byte, error) {
		args := []string{orchestratorCLIName, "set", "backend", "rook"}
		output, err := client.NewCephCommand(c.context, c.Namespace, args).RunWithTimeout(client.CmdExecuteTimeout)
		return "set rook backend", output, err
	}, c.exitCode, 5, invalidArgErrorCode, orchestratorInitWaitTime)
	if err != nil {
		return errors.Wrap(err, "failed to set rook as the orchestrator backend")
	}

	return nil
}
