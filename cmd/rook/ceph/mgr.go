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
	mgrdaemon "github.com/rook/rook/pkg/daemon/ceph/mgr"
	mondaemon "github.com/rook/rook/pkg/daemon/ceph/mon"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
)

var (
	mgrName             string
	mgrKeyring          string
	mgrModuleServerAddr string
	cephVersionName     string
)

var mgrCmd = &cobra.Command{
	Use:    mgrdaemon.InitCommand,
	Short:  "Generates mgr config",
	Hidden: true,
}

func init() {
	mgrCmd.Flags().StringVar(&mgrName, "mgr-name", "", "name of the mgr")
	mgrCmd.Flags().StringVar(&mgrKeyring, "mgr-keyring", "", "the mgr keyring")
	mgrCmd.Flags().StringVar(&mgrModuleServerAddr, "mgr-module-server-addr", "", "the server_addr to set for the mgr module bindings")
	mgrCmd.Flags().StringVar(&cephVersionName, "ceph-version-name", "", "the major version of ceph running")
	addCephFlags(mgrCmd)

	flags.SetFlagsFromEnv(mgrCmd.Flags(), rook.RookEnvVarPrefix)

	mgrCmd.RunE = initMgr
}

func initMgr(cmd *cobra.Command, args []string) error {
	required := []string{
		"mgr-name", "mgr-keyring", "ceph-version-name", "mgr-module-server-addr",
		"mon-endpoints", "cluster-name", "mon-secret", "admin-secret"}
	if err := flags.VerifyRequiredFlags(mgrCmd, required); err != nil {
		return err
	}

	if err := verifyRenamedFlags(mgrCmd); err != nil {
		return err
	}

	rook.SetLogLevel()

	rook.LogStartupInfo(mgrCmd.Flags())

	clusterInfo.Monitors = mondaemon.ParseMonEndpoints(cfg.monEndpoints)
	config := &mgrdaemon.Config{
		Name:             mgrName,
		Keyring:          mgrKeyring,
		ClusterInfo:      &clusterInfo,
		ModuleServerAddr: mgrModuleServerAddr,
		CephVersionName:  cephVersionName,
	}

	err := mgrdaemon.Initialize(createContext(), config)
	if err != nil {
		rook.TerminateFatal(err)
	}

	return nil
}
