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

	"github.com/rook/rook/pkg/ceph/mgr"
	"github.com/rook/rook/pkg/ceph/mon"
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

	flags.SetFlagsFromEnv(mgrCmd.Flags(), "ROOKD")

	mgrCmd.RunE = startMgr
}

func startMgr(cmd *cobra.Command, args []string) error {
	if err := flags.VerifyRequiredFlags(mgrCmd, []string{"mon-endpoints", "cluster-name", "mon-secret", "admin-secret"}); err != nil {
		return err
	}

	setLogLevel()

	clusterInfo.Monitors = mon.ParseMonEndpoints(cfg.monEndpoints)
	config := &mgr.Config{
		Name:        mgrName,
		Keyring:     mgrKeyring,
		ClusterInfo: &clusterInfo,
		InProc:      true,
	}

	err := mgr.Run(createContext(), config)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	return nil
}
