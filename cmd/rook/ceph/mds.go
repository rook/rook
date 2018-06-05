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
	"github.com/rook/rook/pkg/operator/ceph/file"
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
	required := []string{"mon-endpoints", "cluster-name", "admin-secret", "filesystem-id", "pod-name"}
	if err := flags.VerifyRequiredFlags(mdsCmd, required); err != nil {
		return err
	}

	if err := verifyRenamedFlags(mdsCmd); err != nil {
		return err
	}

	rook.SetLogLevel()

	rook.LogStartupInfo(mdsCmd.Flags())

	id := extractMdsID(podName)

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

func extractMdsID(mdsName string) string {
	prefix := file.AppName + "-"
	if strings.Index(mdsName, prefix) == 0 && len(mdsName) > len(prefix) {
		// remove the prefix from the mds name
		return mdsName[len(prefix):]
	}

	// return the original name if we did not find the prefix
	return mdsName
}
