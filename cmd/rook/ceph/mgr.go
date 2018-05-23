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
	"github.com/rook/rook/pkg/daemon/ceph/mgr"
	"github.com/rook/rook/pkg/daemon/ceph/mon"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
)

var (
	mgrName    string
	mgrKeyring string
)

var mgrCmd = &cobra.Command{
	Use:    "mgr",
	Short:  "Generates mgr config and runs the mgr daemon",
	Hidden: true,
}

func init() {
	mgrCmd.Flags().StringVar(&mgrName, "mgr-name", "", "the mgr name")
	mgrCmd.Flags().StringVar(&mgrKeyring, "mgr-keyring", "", "the mgr keyring")
	addCephFlags(mgrCmd)

	flags.SetFlagsFromEnv(mgrCmd.Flags(), rook.RookEnvVarPrefix)

	mgrCmd.RunE = startMgr
}

func startMgr(cmd *cobra.Command, args []string) error {
	required := []string{"mon-endpoints", "cluster-name", "mon-secret", "admin-secret", "public-ipv4", "private-ipv4"}
	if err := flags.VerifyRequiredFlags(mgrCmd, required); err != nil {
		return err
	}

	rook.SetLogLevel()

	rook.LogStartupInfo(mgrCmd.Flags())

	clusterInfo.Monitors = mon.ParseMonEndpoints(cfg.monEndpoints)
	config := &mgr.Config{
		Name:        mgrName,
		Keyring:     mgrKeyring,
		ClusterInfo: &clusterInfo,
	}

	err := mgr.Run(createContext(), config)
	if err != nil {
		rook.TerminateFatal(err)
	}

	return nil
}
