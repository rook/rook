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
	"os"

	"github.com/rook/rook/pkg/ceph/mds"
	"github.com/rook/rook/pkg/ceph/mon"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
)

var (
	mdsID      string
	mdsKeyring string
)

var mdsCmd = &cobra.Command{
	Use:    "mds",
	Short:  "Generates mds config and runs the mds daemon",
	Hidden: true,
}

func init() {
	mdsCmd.Flags().StringVar(&mdsID, "mds-id", "", "the mds ID")
	mdsCmd.Flags().StringVar(&mdsKeyring, "mds-keyring", "", "the mds keyring")
	addCephFlags(mdsCmd)

	flags.SetFlagsFromEnv(mdsCmd.Flags(), "ROOKD")

	mdsCmd.RunE = startMDS
}

func startMDS(cmd *cobra.Command, args []string) error {
	if err := flags.VerifyRequiredFlags(mdsCmd, []string{"mon-endpoints", "cluster-name", "mon-secret", "admin-secret", "mds-id", "mds-keyring"}); err != nil {
		return err
	}

	setLogLevel()

	clusterInfo.Monitors = mon.ParseMonEndpoints(cfg.monEndpoints)
	config := &mds.Config{
		ID:          mdsID,
		Keyring:     mdsKeyring,
		ClusterInfo: &clusterInfo,
		InProc:      true,
	}

	err := mds.Run(createDaemonContext(), config)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	return nil
}
