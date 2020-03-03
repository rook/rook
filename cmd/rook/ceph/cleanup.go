/*
Copyright 2020 The Rook Authors. All rights reserved.

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
	"os"
	"path"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/rook/rook/cmd/rook/rook"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
)

var (
	dataDirHostPath string
	namespaceDir    string
)

var cleanUpCmd = &cobra.Command{
	Use:   "clean",
	Short: "Starts the cleanup process on the disks after ceph cluster is deleted",
}

func init() {
	cleanUpCmd.Flags().StringVar(&dataDirHostPath, "data-dir-host-path", "", "dataDirHostPath on the node")
	cleanUpCmd.Flags().StringVar(&namespaceDir, "namespace-dir", "", "dataDirHostPath on the node")
	flags.SetFlagsFromEnv(cleanUpCmd.Flags(), rook.RookEnvVarPrefix)
	cleanUpCmd.RunE = startCleanUp
}

func startCleanUp(cmd *cobra.Command, args []string) error {
	rook.SetLogLevel()
	rook.LogStartupInfo(cleanUpCmd.Flags())

	logger.Info("starting cluster clean up")
	// Delete dataDirHostPath
	if dataDirHostPath != "" {
		cleanupDirPath := path.Join(dataDirHostPath, namespaceDir)
		if err := os.RemoveAll(cleanupDirPath); err != nil {
			logger.Errorf("failed to clean up %q directory. %v", cleanupDirPath, err)
		} else {
			logger.Infof("successfully cleaned up %q directory", cleanupDirPath)
		}
		// Remove all the mon directories.
		monDirs, err := filepath.Glob(path.Join(dataDirHostPath, "mon-*"))
		if err != nil {
			return errors.Wrapf(err, "failed to clean up mon directories on dataDirHostPath %q", dataDirHostPath)
		}
		if len(monDirs) > 0 {
			for _, monDir := range monDirs {
				if err := os.RemoveAll(monDir); err != nil {
					logger.Errorf("failed to clean up mon directory %q on dataDirHostPath. %v", monDir, err)
				} else {
					logger.Infof("successfully cleaned up mon directory %q on dataDirHostPath", monDir)
				}
			}
			logger.Info("completed clean up of the mon directories in the dataDirHostPath")
		}
	}

	return nil
}
