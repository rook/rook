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

package ceph

import (
	"fmt"

	"github.com/rook/rook/cmd/rook/rook"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/agent"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/exec"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Runs the rook ceph agent",
}

func init() {
	flags.SetFlagsFromEnv(agentCmd.Flags(), rook.RookEnvVarPrefix)
	agentCmd.RunE = startAgent
}

func startAgent(cmd *cobra.Command, args []string) error {
	rook.SetLogLevel()

	rook.LogStartupInfo(agentCmd.Flags())

	clientset, apiExtClientset, rookClientset, err := rook.GetClientset()
	if err != nil {
		rook.TerminateFatal(fmt.Errorf("failed to get k8s client. %+v", err))
	}

	logger.Infof("starting rook ceph agent")
	context := &clusterd.Context{
		Executor:              &exec.CommandExecutor{},
		ConfigDir:             k8sutil.DataDir,
		NetworkInfo:           clusterd.NetworkInfo{},
		Clientset:             clientset,
		APIExtensionClientset: apiExtClientset,
		RookClientset:         rookClientset,
	}

	agent := agent.New(context)
	err = agent.Run()
	if err != nil {
		rook.TerminateFatal(fmt.Errorf("failed to run rook ceph agent. %+v\n", err))
	}

	return nil
}
