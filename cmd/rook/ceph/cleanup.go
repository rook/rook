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
	"io/ioutil"
	"os"
	"path"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/rook/rook/cmd/rook/rook"
	"github.com/rook/rook/pkg/operator/ceph/cluster/mon"
	"github.com/rook/rook/pkg/util/flags"
	"github.com/spf13/cobra"
)

var (
	dataDirHostPath string
	namespaceDir    string
	monSecret       string
)

var cleanUpCmd = &cobra.Command{
	Use:   "clean",
	Short: "Starts the cleanup process on the disks after ceph cluster is deleted",
}

func init() {
	cleanUpCmd.Flags().StringVar(&dataDirHostPath, "data-dir-host-path", "", "dataDirHostPath on the node")
	cleanUpCmd.Flags().StringVar(&namespaceDir, "namespace-dir", "", "dataDirHostPath on the node")
	cleanUpCmd.Flags().StringVar(&monSecret, "mon-secret", "", "monitor secret from the keyring")
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
		cleanMonDirs()
	}

	return nil
}

func cleanMonDirs() {
	monDirs, err := filepath.Glob(path.Join(dataDirHostPath, "mon-*"))
	if err != nil {
		logger.Errorf("failed to find the mon directories on the dataDirHostPath %q. %v", dataDirHostPath, err)
		return
	}

	if len(monDirs) == 0 {
		logger.Infof("no mon directories are available for clean up in the dataDirHostPath %q", dataDirHostPath)
		return
	}

	for _, monDir := range monDirs {
		// Clean up mon directory only if mon secret matches with that in the keyring file.
		deleteMonDir, err := secretKeyMatch(monDir, monSecret)
		if err != nil {
			logger.Errorf("failed to clean up the mon directory %q on the dataDirHostPath %q. %v", monDir, dataDirHostPath, err)
			continue
		}
		if deleteMonDir {
			if err := os.RemoveAll(monDir); err != nil {
				logger.Errorf("failed to clean up the mon directory %q on the dataDirHostPath %q. %v", monDir, dataDirHostPath, err)
			} else {
				logger.Infof("successfully cleaned up the mon directory %q on the dataDirHostPath %q", monDir, dataDirHostPath)
			}
		} else {
			logger.Infof("skipped clean up of the mon directory %q as the secret key did not match", monDir)
		}
	}

	return
}

func secretKeyMatch(monDir, monSecret string) (bool, error) {
	keyringDirPath := path.Join(monDir, "/data/keyring")
	if _, err := os.Stat(keyringDirPath); os.IsNotExist(err) {
		return false, errors.Wrapf(err, "failed to read keyring %q for the mon directory %q", keyringDirPath, monDir)
	}
	contents, err := ioutil.ReadFile(filepath.Clean(keyringDirPath))
	if err != nil {
		return false, errors.Wrapf(err, "failed to read keyring %q for the mon directory %q", keyringDirPath, monDir)
	}
	extractedKey, err := mon.ExtractKey(string(contents))
	if err != nil {
		return false, errors.Wrapf(err, "failed to extract secret key from the keyring %q for the mon directory %q", keyringDirPath, monDir)
	}

	return monSecret == extractedKey, nil
}
