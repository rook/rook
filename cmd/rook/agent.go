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
package main

import (
	"fmt"

	"github.com/rook/rook/pkg/agent"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
)

var agentCmd = &cobra.Command{
	Use:    "agent",
	Short:  "Runs the rook agent",
	Hidden: true,
}

func init() {
	flags.SetFlagsFromEnv(agentCmd.Flags(), "ROOK")
	agentCmd.RunE = startAgent
}

func startAgent(cmd *cobra.Command, args []string) error {

	setLogLevel()

	logStartupInfo(agentCmd.Flags())

	clientset, apiExtClientset, err := getClientset()
	if err != nil {
		terminateFatal(fmt.Errorf("failed to get k8s client. %+v", err))
	}

	logger.Info("starting rook agent")
	context := createContext()
	context.NetworkInfo = clusterd.NetworkInfo{}
	context.ConfigDir = k8sutil.DataDir
	context.Clientset = clientset
	context.APIExtensionClientset = apiExtClientset

	agent := agent.New(context)
	err = agent.Run()
	if err != nil {
		terminateFatal(fmt.Errorf("failed to run rook agent. %+v\n", err))
	}

	return nil
}
