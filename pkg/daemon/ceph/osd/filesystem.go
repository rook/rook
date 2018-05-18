/*
Copyright 2017 The Rook Authors. All rights reserved.

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
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/mon"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	"github.com/rook/rook/pkg/util"
)

const (
	maxFileBackupSize         = 1 * 1024 * 1024 // 1 MB
	osdFSStoreNameFmt         = "rook-ceph-osd-%d-fs-backup"
	bluestoreBlockSymlinkName = "block"
	bluestoreDBSymlinkName    = "block.db"
	bluestoreWalSymlinkName   = "block.wal"
)

// creates/initalizes the OSD filesystem and journal via a child process
func createOSDFileSystem(context *clusterd.Context, clusterName string, config *osdConfig) error {
	logger.Infof("Initializing OSD %d file system at %s...", config.id, config.rootPath)

	// get the current monmap, it will be needed for creating the OSD file system
	monMap, err := getMonMap(context, clusterName)
	if err != nil {
		return fmt.Errorf("failed to get mon map: %+v", err)
	}

	// the current monmap is needed to create the OSD, save it to a temp location so it is accessible
	monMapTmpPath := getOSDTempMonMapPath(config.rootPath)
	monMapTmpDir := filepath.Dir(monMapTmpPath)
	if err := os.MkdirAll(monMapTmpDir, 0744); err != nil {
		return fmt.Errorf("failed to create monmap tmp file directory at %s: %+v", monMapTmpDir, err)
	}
	if err := ioutil.WriteFile(monMapTmpPath, monMap, 0644); err != nil {
		return fmt.Errorf("failed to write mon map to tmp file %s, %+v", monMapTmpPath, err)
	}

	options := []string{
		"--mkfs",
		fmt.Sprintf("--id=%d", config.id),
		fmt.Sprintf("--cluster=%s", clusterName),
		fmt.Sprintf("--conf=%s", mon.GetConfFilePath(config.rootPath, clusterName)),
		fmt.Sprintf("--osd-data=%s", config.rootPath),
		fmt.Sprintf("--osd-uuid=%s", config.uuid.String()),
		fmt.Sprintf("--monmap=%s", monMapTmpPath),
		fmt.Sprintf("--keyring=%s", getOSDKeyringPath(config.rootPath)),
	}

	if isFilestore(config) {
		options = append(options, fmt.Sprintf("--osd-journal=%s", getOSDJournalPath(config.rootPath)))
	}

	// create the file system
	logName := fmt.Sprintf("mkfs-osd%d", config.id)
	if err = context.Executor.ExecuteCommand(false, logName, "ceph-osd", options...); err != nil {
		return fmt.Errorf("failed osd mkfs for OSD ID %d, UUID %s, dataDir %s: %+v",
			config.id, config.uuid.String(), config.rootPath, err)
	}

	// now that the OSD filesystem has been created, back it up so it can be restored/repaired
	// later on if needed.
	if err := backupOSDFileSystem(config, clusterName); err != nil {
		return fmt.Errorf("failed to backup OSD filesystem: %+v", err)
	}

	// update the scheme to indicate the OSD's filesystem has been created and backed up.
	if err := markOSDFileSystemCreated(config); err != nil {
		return err
	}

	return nil
}

// determines if the given OSD's filesystem has previously been created (via osd mkfs) and backed up
// successfully.  It may not exist on disk anymore and therefore needs to be repaired, but this
// determines if it was ever created and backed up in the past.
func isOSDFilesystemCreated(config *osdConfig) bool {
	if config.partitionScheme == nil {
		return false
	}

	return config.partitionScheme.FSCreated
}

// marks the given OSD's filesystem as created and backed up.
func markOSDFileSystemCreated(cfg *osdConfig) error {
	if cfg.partitionScheme == nil {
		return nil
	}

	savedScheme, err := config.LoadScheme(cfg.kv, cfg.storeName)
	if err != nil {
		return fmt.Errorf("failed to load the saved partition scheme: %+v", err)
	}

	// mark the OSD's filesystem as created and backed up.
	cfg.partitionScheme.FSCreated = true
	if err := savedScheme.UpdateSchemeEntry(cfg.partitionScheme); err != nil {
		return fmt.Errorf("failed to update partition scheme entry %+v", cfg.partitionScheme)
	}

	if err := savedScheme.SaveScheme(cfg.kv, cfg.storeName); err != nil {
		return fmt.Errorf("failed to save partition scheme: %+v", err)
	}

	return nil
}

func backupOSDFileSystem(config *osdConfig, clusterName string) error {
	if !isBluestore(config) {
		return nil
	}

	logger.Infof("Backing up OSD %d file system from %s", config.id, config.rootPath)

	storeName := fmt.Sprintf(osdFSStoreNameFmt, config.id)

	// ensure the store we are backing up to is clear first
	if err := config.kv.ClearStore(storeName); err != nil {
		return err
	}

	fis, err := ioutil.ReadDir(config.rootPath)
	if err != nil {
		return err
	}

	filter := util.CreateSet([]string{
		// filter out the rook.config file since it's always regenerated
		filepath.Base(mon.GetConfFilePath(config.rootPath, clusterName)),
		// filter out the keyring since we recreate it with "auth get-or-create" and we don't want
		// to store a secret in a non secret resource
		keyringFileName,
	})

	for _, fi := range fis {
		if !fi.Mode().IsRegular() {
			// the current file is not a regular file (it could be a socket, symlink, etc.), skip it
			continue
		}

		if filter.Contains(fi.Name()) {
			// the current file is in the filter list, skip it
			logger.Infof("skipping backup of file %s that is in the filter list", fi.Name())
			continue
		}

		if fi.Size() > maxFileBackupSize {
			logger.Warningf("skipping backup of file %s with size %d exceeding the maximum of %d",
				fi.Name(), fi.Size(), maxFileBackupSize)
			continue
		}

		content, err := ioutil.ReadFile(filepath.Join(config.rootPath, fi.Name()))
		if err != nil {
			logger.Warningf("failed to read file %s: %+v", fi.Name(), err)
			continue
		}

		if err := config.kv.SetValue(storeName, fi.Name(), string(content)); err != nil {
			logger.Warningf("failed to backup file %s: %+v", fi.Name(), err)
			continue
		}
	}

	logger.Infof("Completed backing up OSD %d file system from %s", config.id, config.rootPath)

	return nil
}

// repairs the given OSD's filesystem locally on disk because it had been created in the past but has
// since been lost somehow.
func repairOSDFileSystem(config *osdConfig) error {
	if !isBluestore(config) {
		return nil
	}

	logger.Infof("Repairing OSD %d file system at %s", config.id, config.rootPath)

	storeName := fmt.Sprintf(osdFSStoreNameFmt, config.id)
	store, err := config.kv.GetStore(storeName)
	if err != nil {
		return err
	}

	for fileName, content := range store {
		filePath := filepath.Join(config.rootPath, fileName)
		if err := ioutil.WriteFile(filePath, []byte(content), 0644); err != nil {
			logger.Warningf("failed to restore file %s: %+v", filePath, err)
			continue
		}
	}

	// create symlinks to device partitions
	walPath, dbPath, blockPath, err := getBluestorePartitionPaths(config)
	if err != nil {
		return err
	}

	if err := createBluestoreSymlink(config, walPath, bluestoreWalSymlinkName); err != nil {
		return err
	}
	if err := createBluestoreSymlink(config, dbPath, bluestoreDBSymlinkName); err != nil {
		return err
	}
	if err := createBluestoreSymlink(config, blockPath, bluestoreBlockSymlinkName); err != nil {
		return err
	}

	logger.Infof("Completed repairing OSD %d file system at %s", config.id, config.rootPath)

	return nil
}

func deleteOSDFileSystem(config *osdConfig) error {
	if !isBluestore(config) {
		return nil
	}

	logger.Infof("Deleting OSD %d file system", config.id)
	storeName := fmt.Sprintf(osdFSStoreNameFmt, config.id)
	return config.kv.ClearStore(storeName)
}

func createBluestoreSymlink(config *osdConfig, targetPath, symlinkName string) error {
	symlinkPath := filepath.Join(config.rootPath, symlinkName)
	if err := os.Symlink(targetPath, symlinkPath); err != nil {
		return fmt.Errorf("failed to create symlink from %s to %s", symlinkPath, targetPath)
	}

	return nil
}
