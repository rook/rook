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

package nfs

import (
	"errors"

	"github.com/rook/rook/cmd/rook/rook"
	"github.com/rook/rook/pkg/operator/nfs"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Runs the NFS server to deploy and manage NFS server in kubernetes clusters",
	Long: `Runs the NFS operator to deploy and manage NFS server in kubernetes clusters.
https://github.com/rook/rook`,
}

var (
	ganeshaConfigPath *string
)

func init() {
	flags.SetFlagsFromEnv(serverCmd.Flags(), rook.RookEnvVarPrefix)
	flags.SetLoggingFlags(serverCmd.Flags())

	ganeshaConfigPath = serverCmd.Flags().String("ganeshaConfigPath", "", "ConfigPath of nfs ganesha")

	serverCmd.RunE = startServer
}

func startServer(cmd *cobra.Command, args []string) error {
	rook.SetLogLevel()
	rook.LogStartupInfo(serverCmd.Flags())
	if len(*ganeshaConfigPath) == 0 {
		return errors.New("--ganeshaConfigPath is a required parameter")
	}

	logger.Infof("Setting up NFS server!")

	err := nfs.Setup(*ganeshaConfigPath)
	if err != nil {
		logger.Fatalf("Error setting up NFS server: %v", err)
	}

	logger.Infof("starting NFS server")
	// This blocks until server exits (presumably due to an error)
	err = nfs.Run(*ganeshaConfigPath)
	if err != nil {
		logger.Errorf("NFS server Exited Unexpectedly with err: %v", err)
	}

	return nil
}
