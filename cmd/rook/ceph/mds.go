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
	mdsdaemon "github.com/rook/rook/pkg/daemon/ceph/mds"
	mondaemon "github.com/rook/rook/pkg/daemon/ceph/mon"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
)

var (
	mdsName       string
	filesystemID  string
	activeStandby bool
	keyring       string
)

var mdsCmd = &cobra.Command{
	Use:    mdsdaemon.InitCommand,
	Short:  "Generates mds config and runs the mds daemon",
	Hidden: true,
}

func init() {
	mdsCmd.Flags().StringVar(&mdsName, "mds-name", "", "name of the mds")
	mdsCmd.Flags().StringVar(&keyring, "mds-keyring", "", "the mds keyring")
	mdsCmd.Flags().StringVar(&filesystemID, "filesystem-id", "", "ID of the filesystem the mds will serve")
	mdsCmd.Flags().BoolVar(&activeStandby, "active-standby", true, "whether to start in active standby mode")
	addCephFlags(mdsCmd)

	flags.SetFlagsFromEnv(mdsCmd.Flags(), rook.RookEnvVarPrefix)

	mdsCmd.RunE = initMds
}

func initMds(cmd *cobra.Command, args []string) error {
	required := []string{"mon-endpoints", "cluster-name", "admin-secret",
		"mds-name", "mds-keyring", "filesystem-id"}
	if err := flags.VerifyRequiredFlags(mdsCmd, required); err != nil {
		return err
	}

	if err := verifyRenamedFlags(mdsCmd); err != nil {
		return err
	}

	rook.SetLogLevel()

	rook.LogStartupInfo(mdsCmd.Flags())

	clusterInfo.Monitors = mondaemon.ParseMonEndpoints(cfg.monEndpoints)
	config := &mdsdaemon.Config{
		FilesystemID:  filesystemID,
		Name:          mdsName,
		ActiveStandby: activeStandby,
		Keyring:       keyring,
		ClusterInfo:   &clusterInfo,
	}

	err := mdsdaemon.Initialize(createContext(), config)
	if err != nil {
		rook.TerminateFatal(err)
	}

	return nil
}
