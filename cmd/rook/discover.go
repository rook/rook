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
package main

import (
	"fmt"
	"time"

	rook "github.com/rook/rook/cmd/rook/rook"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/discover"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/exec"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
)

var (
	discoverCmd = &cobra.Command{
		Use:   "discover",
		Short: "Discover devices",
	}

	// interval between discovering devices
	discoverDevicesInterval time.Duration
)

func init() {
	discoverCmd.Flags().DurationVar(&discoverDevicesInterval, "discover-interval", 60*time.Minute, "interval between discovering devices (default 60m)")

	flags.SetFlagsFromEnv(discoverCmd.Flags(), rook.RookEnvVarPrefix)
	discoverCmd.RunE = startDiscover
}

func startDiscover(cmd *cobra.Command, args []string) error {
	rook.SetLogLevel()

	rook.LogStartupInfo(discoverCmd.Flags())

	clientset, apiExtClientset, rookClientset, err := rook.GetClientset()
	if err != nil {
		rook.TerminateFatal(fmt.Errorf("failed to init k8s client. %+v", err))
	}

	context := &clusterd.Context{
		Executor:              &exec.CommandExecutor{},
		ConfigDir:             k8sutil.DataDir,
		NetworkInfo:           clusterd.NetworkInfo{},
		Clientset:             clientset,
		APIExtensionClientset: apiExtClientset,
		RookClientset:         rookClientset,
	}

	err = discover.Run(context, discoverDevicesInterval)
	if err != nil {
		rook.TerminateFatal(err)
	}

	return nil
}
