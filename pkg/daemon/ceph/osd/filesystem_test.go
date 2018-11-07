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
	"testing"

	"k8s.io/apimachinery/pkg/api/errors"

	"github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/stretchr/testify/assert"
)

func TestIsOSDFilesystemCreated(t *testing.T) {
	cfg := &osdConfig{}
	assert.False(t, isOSDFilesystemCreated(cfg))

	cfg = &osdConfig{partitionScheme: &config.PerfSchemeEntry{FSCreated: false}}
	assert.False(t, isOSDFilesystemCreated(cfg))

	cfg = &osdConfig{partitionScheme: &config.PerfSchemeEntry{FSCreated: true}}
	assert.True(t, isOSDFilesystemCreated(cfg))
}

func TestBackupOSDFileSystem(t *testing.T) {
	configDir, err := ioutil.TempDir("", "TestBackupOSDFileSystem")
	if err != nil {
		t.Fatalf("failed to create temp config dir: %+v", err)
	}
	defer os.RemoveAll(configDir)

	osdID := 123
	clusterName := "rook123"
	kv := mockKVStore()

	cfg := &osdConfig{
		rootPath:        configDir,
		id:              osdID,
		partitionScheme: &config.PerfSchemeEntry{StoreType: config.Bluestore},
		kv:              kv,
	}

	// seed a mocked OSD filesystem to the config dir
	createMockMetadata(t, configDir, "foo", "bar")
	createMockMetadata(t, configDir, "baz", "biz")

	// create a rook config file that should get filtered out during the backup
	configFileName := "rook123.config"
	createMockMetadata(t, configDir, configFileName, "mock config")

	// create a keyring file that should get filtered out during the backup
	createMockMetadata(t, configDir, keyringFileName, "mock keyring")

	// create a file larger than the max size that should be skipped during the backup
	oversizeFileName := "oversize.txt"
	oversizeFile, err := os.Create(filepath.Join(configDir, oversizeFileName))
	if err != nil {
		t.Fatalf("failed to create oversized file: %+v", err)
	}
	if err := oversizeFile.Truncate(maxFileBackupSize + 1); err != nil {
		t.Fatalf("failed to truncate oversized file: %+v", err)
	}

	// backup the OSD filesystem
	err = backupOSDFileSystem(cfg, clusterName)
	assert.Nil(t, err)

	// verify the backed up OSD filesystem (and the not backed up files too)
	assertBackedUpFile(t, cfg, kv, "foo", "bar")
	assertBackedUpFile(t, cfg, kv, "baz", "biz")
	assertNotBackedUpFile(t, cfg, kv, configFileName)
	assertNotBackedUpFile(t, cfg, kv, keyringFileName)
	assertNotBackedUpFile(t, cfg, kv, oversizeFileName)
}

func createMockMetadata(t *testing.T, configDir, name, content string) {
	err := ioutil.WriteFile(filepath.Join(configDir, name), []byte(content), 0644)
	assert.Nil(t, err)
}

func assertBackedUpFile(t *testing.T, c *osdConfig, kv *k8sutil.ConfigMapKVStore, name, expectedContent string) {
	storeName := fmt.Sprintf(config.OSDFSStoreNameFmt, c.id)
	val, err := kv.GetValue(storeName, name)
	assert.Nil(t, err)
	assert.Equal(t, expectedContent, val)
}

func assertNotBackedUpFile(t *testing.T, c *osdConfig, kv *k8sutil.ConfigMapKVStore, name string) {
	storeName := fmt.Sprintf(config.OSDFSStoreNameFmt, c.id)
	_, err := kv.GetValue(storeName, name)
	assert.NotNil(t, err)
	assert.True(t, errors.IsNotFound(err))
}

func TestRepairOSDFileSystem(t *testing.T) {
	configDir, err := ioutil.TempDir("", "TestRepairOSDFileSystem")
	if err != nil {
		t.Fatalf("failed to create temp config dir: %+v", err)
	}
	defer os.RemoveAll(configDir)

	osdID := 123
	storeConfig := config.StoreConfig{StoreType: config.Bluestore}
	kv := mockKVStore()
	schemeEntry, _, _ := mockPartitionSchemeEntry(t, osdID, "sdf", &storeConfig, kv, "node3930")
	cfg := &osdConfig{
		rootPath:        configDir,
		id:              osdID,
		partitionScheme: schemeEntry,
		kv:              kv,
	}

	// mock a backed up OSD filesystem
	storeName := fmt.Sprintf(config.OSDFSStoreNameFmt, cfg.id)
	kv.SetValue(storeName, "foo", "bar")
	kv.SetValue(storeName, "bif", "bonk")

	// perform the repair of the OSD filesystem
	err = repairOSDFileSystem(cfg)
	assert.Nil(t, err)

	// verify the OSD filesystem were restored/repaired
	assertRepairedFile(t, cfg, "foo", "bar")
	assertRepairedFile(t, cfg, "bif", "bonk")

	// verify the block/wal/db symlinks
	parts := cfg.partitionScheme.Partitions
	assertBluestoreSymlink(t, cfg, bluestoreBlockSymlinkName, parts[config.BlockPartitionType].PartitionUUID)
	assertBluestoreSymlink(t, cfg, bluestoreDBSymlinkName, parts[config.DatabasePartitionType].PartitionUUID)
	assertBluestoreSymlink(t, cfg, bluestoreWalSymlinkName, parts[config.WalPartitionType].PartitionUUID)
}

func assertRepairedFile(t *testing.T, cfg *osdConfig, name, expectedContent string) {
	content, err := ioutil.ReadFile(filepath.Join(cfg.rootPath, name))
	assert.Nil(t, err)
	assert.Equal(t, expectedContent, string(content))
}

func assertBluestoreSymlink(t *testing.T, config *osdConfig, symlinkName, partUUID string) {
	// assert that the symlink exists at the expected path and that it is actually a symlink
	symlinkPath := filepath.Join(config.rootPath, symlinkName)
	fi, err := os.Lstat(symlinkPath)
	assert.Nil(t, err)
	assert.NotEqual(t, 0, fi.Mode()&os.ModeSymlink)

	// assert that the target of the symlink is expected
	expectedTarget := filepath.Join(diskByPartUUID, partUUID)
	target, err := os.Readlink(symlinkPath)
	assert.Nil(t, err)
	assert.Equal(t, expectedTarget, target)
}
