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
	"time"

	rook "github.com/rook/rook/cmd/rook/rook"
	"github.com/rook/rook/pkg/daemon/discover"
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

	// Uses ceph-volume inventory to extend devices information
	usesCVInventory bool
)

func init() {
	discoverCmd.Flags().DurationVar(&discoverDevicesInterval, "discover-interval", 60*time.Minute, "interval between discovering devices (default 60m)")
	discoverCmd.Flags().BoolVar(&usesCVInventory, "use-ceph-volume", false, "Use ceph-volume inventory to extend storage devices information (default false)")

	flags.SetFlagsFromEnv(discoverCmd.Flags(), rook.RookEnvVarPrefix)
	discoverCmd.RunE = startDiscover
}

func startDiscover(cmd *cobra.Command, args []string) error {
	rook.SetLogLevel()

	rook.LogStartupInfo(discoverCmd.Flags())

	context := rook.NewContext()
	ctx := cmd.Context()

	err := discover.Run(ctx, context, discoverDevicesInterval, usesCVInventory)
	if err != nil {
		rook.TerminateFatal(err)
	}

	return nil
}
