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

package osd

import (
	"fmt"
	"io"
	"os"

	"path"
	"path/filepath"

	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
)

const (
	keyringFileName     = "keyring"
	bootstrapOsdKeyring = "bootstrap-osd/ceph.keyring"
)

// get the bootstrap OSD root dir
func getBootstrapOSDDir(configDir string) string {
	return path.Join(configDir, "bootstrap-osd")
}

func getOSDRootDir(root string, osdID int) string {
	return filepath.Join(root, fmt.Sprintf("osd%d", osdID))
}

// get the full path to the given OSD's config file
func getOSDConfFilePath(osdDataPath, clusterName string) string {
	return fmt.Sprintf("%s/%s.config", osdDataPath, clusterName)
}

// get the full path to the given OSD's keyring
func getOSDKeyringPath(osdDataPath string) string {
	return filepath.Join(osdDataPath, keyringFileName)
}

// get the full path to the given OSD's journal
func getOSDJournalPath(osdDataPath string) string {
	return filepath.Join(osdDataPath, "journal")
}

// get the full path to the given OSD's temporary mon map
func getOSDTempMonMapPath(osdDataPath string) string {
	return filepath.Join(osdDataPath, "tmp", "activate.monmap")
}

// create a keyring for the bootstrap-osd client, it gets a limited set of privileges
func createOSDBootstrapKeyring(context *clusterd.Context, clusterName, rootDir string) error {
	username := "client.bootstrap-osd"
	keyringPath := path.Join(rootDir, bootstrapOsdKeyring)
	access := []string{"mon", "allow profile bootstrap-osd"}
	keyringEval := func(key string) string {
		return fmt.Sprintf(bootstrapOSDKeyringTemplate, key)
	}

	return cephconfig.CreateKeyring(context, clusterName, username, keyringPath, access, keyringEval)
}

// CopyBinariesForDaemon copies the "tini" and "rook" binaries to a shared volume at the target path.
// This is necessary for the filestore on a device scenario when rook needs to mount a directory
// in the same container as the ceph process so it can be unmounted upon exit.
func CopyBinariesForDaemon(target string) error {
	if err := copyBinary("/usr/local/bin", target, "rook"); err != nil {
		return err
	}
	return copyBinary("/", target, "tini")
}

func copyBinary(sourceDir, targetDir, filename string) error {
	sourcePath := path.Join(sourceDir, filename)
	targetPath := path.Join(targetDir, filename)
	logger.Infof("copying %s to %s", sourcePath, targetPath)

	// Check if the target path exists, and skip the copy if it does
	if _, err := os.Stat(targetPath); err == nil {
		return nil
	}

	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destinationFile, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	defer destinationFile.Close()
	if _, err := io.Copy(destinationFile, sourceFile); err != nil {
		return err
	}

	return os.Chmod(targetPath, 0755)
}
