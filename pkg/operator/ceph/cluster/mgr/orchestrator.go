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
	"fmt"

	cephv1 "github.com/rook/rook/pkg/apis/ceph.rook.io/v1"
	"github.com/rook/rook/pkg/daemon/ceph/client"
)

const (
	orchestratorModuleName = "orchestrator_cli"
	rookModuleName         = "rook"
)

// Ceph docs about the orchestrator modules: http://docs.ceph.com/docs/master/mgr/orchestrator_cli/
func (c *Cluster) configureOrchestratorModules() error {
	if !cephv1.VersionAtLeast(c.cephVersion.Name, cephv1.Nautilus) {
		logger.Infof("skipping enabling orchestrator modules on releases older than nautilus")
		return nil
	}

	if err := client.MgrEnableModule(c.context, c.Namespace, orchestratorModuleName, true); err != nil {
		return fmt.Errorf("failed to enable mgr orchestrator module. %+v", err)
	}
	if err := client.MgrEnableModule(c.context, c.Namespace, rookModuleName, true); err != nil {
		return fmt.Errorf("failed to enable mgr rook module. %+v", err)
	}
	if _, err := client.ExecuteCephCommand(c.context, c.Namespace, []string{"orchestrator", "set", "backend", "rook"}); err != nil {
		return fmt.Errorf("failed to set rook as the orchestrator backend. %+v", err)
	}

	return nil
}
