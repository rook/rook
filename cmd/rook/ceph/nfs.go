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
	"github.com/rook/rook/cmd/rook/rook"
	"github.com/rook/rook/pkg/daemon/ceph/ganesha"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
)

var (
	ganeshaName             string
	ganeshaCopyBinariesPath string
)

var nfsCmd = &cobra.Command{
	Use:   "nfs",
	Short: "Configures and runs the ceph nfs daemon",
}
var ganeshaConfigCmd = &cobra.Command{
	Use:   "init",
	Short: "Updates ceph.conf for nfs",
}
var ganeshaRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Runs the ceph nfs daemon",
}

func init() {
	ganeshaRunCmd.Flags().StringVar(&ganeshaName, "ceph-nfs-name", "", "name of the nfs server")
	ganeshaConfigCmd.Flags().StringVar(&ganeshaCopyBinariesPath, "copy-binaries-path", "", "If specified, copy the rook binaries to this path for use by the daemon container")
	addCephFlags(ganeshaConfigCmd)

	nfsCmd.AddCommand(ganeshaConfigCmd, ganeshaRunCmd)

	flags.SetFlagsFromEnv(nfsCmd.Flags(), rook.RookEnvVarPrefix)
	flags.SetFlagsFromEnv(ganeshaConfigCmd.Flags(), rook.RookEnvVarPrefix)
	flags.SetFlagsFromEnv(ganeshaRunCmd.Flags(), rook.RookEnvVarPrefix)

	ganeshaConfigCmd.RunE = configGanesha
	ganeshaRunCmd.RunE = startGanesha
}

func configGanesha(cmd *cobra.Command, args []string) error {
	required := []string{"copy-binaries-path", "mon-endpoints", "cluster-name", "admin-secret", "public-ip", "private-ip"}
	if err := flags.VerifyRequiredFlags(ganeshaConfigCmd, required); err != nil {
		return err
	}

	rook.SetLogLevel()
	rook.LogStartupInfo(ganeshaConfigCmd.Flags())

	clusterInfo.Monitors = mon.ParseMonEndpoints(cfg.monEndpoints)

	// generate the ceph config
	err := ganesha.Initialize(createContext(), &clusterInfo)
	if err != nil {
		rook.TerminateFatal(err)
	}

	// copy the rook and tini binaries for use by the daemon container
	copyBinaries(ganeshaCopyBinariesPath)

	return nil
}

func startGanesha(cmd *cobra.Command, args []string) error {
	required := []string{"ceph-nfs-name"}
	if err := flags.VerifyRequiredFlags(ganeshaRunCmd, required); err != nil {
		return err
	}

	rook.SetLogLevel()
	rook.LogStartupInfo(nfsCmd.Flags())

	err := ganesha.Run(createContext(), ganeshaName)
	if err != nil {
		rook.TerminateFatal(err)
	}

	return nil
}
