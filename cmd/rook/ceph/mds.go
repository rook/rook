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
	"strings"

	"github.com/rook/rook/cmd/rook/rook"
	"github.com/rook/rook/pkg/daemon/ceph/mds"
	"github.com/rook/rook/pkg/daemon/ceph/mon"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
)

var (
	podName       string
	filesystemID  string
	activeStandby bool
)

var mdsCmd = &cobra.Command{
	Use:    "mds",
	Short:  "Generates mds config and runs the mds daemon",
	Hidden: true,
}

func init() {
	mdsCmd.Flags().StringVar(&podName, "pod-name", "", "name of the pod from which the mds ID is derived")
	mdsCmd.Flags().StringVar(&filesystemID, "filesystem-id", "", "ID of the filesystem this MDS will serve")
	mdsCmd.Flags().BoolVar(&activeStandby, "active-standby", true, "Whether to start in active standby mode")
	addCephFlags(mdsCmd)

	flags.SetFlagsFromEnv(mdsCmd.Flags(), rook.RookEnvVarPrefix)

	mdsCmd.RunE = startMDS
}

func startMDS(cmd *cobra.Command, args []string) error {
	required := []string{"mon-endpoints", "cluster-name", "admin-secret", "filesystem-id", "pod-name", "public-ipv4", "private-ipv4"}
	if err := flags.VerifyRequiredFlags(mdsCmd, required); err != nil {
		return err
	}

	rook.SetLogLevel()

	rook.LogStartupInfo(mdsCmd.Flags())

	// the MDS ID is the last part of the pod name
	id := podName
	dashIndex := strings.LastIndex(podName, "-")
	if dashIndex > 0 {
		id = podName[dashIndex+1:]
	}
	// ensure the id has a non-numerical prefix
	id = "m" + id

	clusterInfo.Monitors = mon.ParseMonEndpoints(cfg.monEndpoints)
	config := &mds.Config{
		FilesystemID:  filesystemID,
		ID:            id,
		ActiveStandby: activeStandby,
		ClusterInfo:   &clusterInfo,
	}

	err := mds.Run(createContext(), config)
	if err != nil {
		rook.TerminateFatal(err)
	}

	return nil
}
