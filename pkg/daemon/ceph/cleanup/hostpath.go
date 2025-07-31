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

package cleanup

import (
	"os"
	"path"
	"path/filepath"

	"github.com/pkg/errors"
	opcontroller "github.com/rook/rook/pkg/operator/ceph/controller"
)

// StartHostPathCleanup is the main entrypoint function to clean up dataDirHostPath and monitor store
func StartHostPathCleanup(namespaceDir, dataDirHostPath, monSecret string) {
	cleanupDirPath := path.Join(dataDirHostPath, namespaceDir)
	if err := os.RemoveAll(cleanupDirPath); err != nil {
		logger.Errorf("failed to clean up %q directory. %v", cleanupDirPath, err)
	} else {
		logger.Infof("successfully cleaned up %q directory", cleanupDirPath)
	}

	cleanMonDirs(dataDirHostPath, monSecret)
	cleanExporterDir(dataDirHostPath)
}

func cleanExporterDir(dataDirHostPath string) {
	exporterDir := path.Join(dataDirHostPath, "exporter")

	// Check if the exporter directory exists
	if _, err := os.Stat(exporterDir); os.IsNotExist(err) {
		logger.Infof("exporter directory %q does not exist, skipping cleanup", exporterDir)
		return
	}

	// Attempt to delete it
	if err := os.RemoveAll(exporterDir); err != nil {
		logger.Errorf("failed to clean up exporter directory %q. %v", exporterDir, err)
	} else {
		logger.Infof("successfully cleaned up exporter directory %q", exporterDir)
	}
}

func cleanMonDirs(dataDirHostPath, monSecret string) {
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
}

func secretKeyMatch(monDir, monSecret string) (bool, error) {
	keyringDirPath := path.Join(monDir, "/data/keyring")
	if _, err := os.Stat(keyringDirPath); os.IsNotExist(err) {
		return false, errors.Wrapf(err, "failed to read keyring %q for the mon directory %q", keyringDirPath, monDir)
	}
	contents, err := os.ReadFile(filepath.Clean(keyringDirPath))
	if err != nil {
		return false, errors.Wrapf(err, "failed to read keyring %q for the mon directory %q", keyringDirPath, monDir)
	}
	extractedKey, err := opcontroller.ExtractKey(string(contents))
	if err != nil {
		return false, errors.Wrapf(err, "failed to extract secret key from the keyring %q for the mon directory %q", keyringDirPath, monDir)
	}

	return monSecret == extractedKey, nil
}
