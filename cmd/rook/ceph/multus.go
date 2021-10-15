/*
Copyright 2021 The Rook Authors. All rights reserved.

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
	"github.com/rook/rook/cmd/rook/rook"
	"github.com/rook/rook/pkg/daemon/ceph/multus"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
)

var multusSetupCmd = &cobra.Command{
	Use:   "multus-setup",
	Short: "Runs the rook multus setup operation",
}

var multusTeardownCmd = &cobra.Command{
	Use:   "multus-teardown",
	Short: "Runs the rook multus teardown operation",
}

func init() {
	flags.SetFlagsFromEnv(multusSetupCmd.Flags(), rook.RookEnvVarPrefix)
	flags.SetFlagsFromEnv(multusTeardownCmd.Flags(), rook.RookEnvVarPrefix)
	multusSetupCmd.RunE = setupMultus
	multusTeardownCmd.RunE = teardownMultus
}

func setupMultus(cmd *cobra.Command, args []string) error {
	rook.SetLogLevel()

	rook.LogStartupInfo(multusSetupCmd.Flags())

	logger.Infof("Starting multus setup")
	err := multus.Setup()
	if err != nil {
		rook.TerminateFatal(err)
	}

	logger.Infof("Multus setup complete.")
	return nil
}

func teardownMultus(cmd *cobra.Command, args []string) error {
	rook.SetLogLevel()

	rook.LogStartupInfo(multusTeardownCmd.Flags())

	logger.Infof("Starting multus teardown")
	err := multus.Teardown()
	if err != nil {
		rook.TerminateFatal(err)
	}

	logger.Infof("Multus teardown complete.")
	return nil
}
