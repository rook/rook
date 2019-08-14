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
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	exectest "github.com/rook/rook/pkg/util/exec/test"
	"github.com/rook/rook/pkg/util/sys"
	"github.com/stretchr/testify/assert"
)

func TestGetOSDInfo(t *testing.T) {
	// error when no info is found on disk
	configDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(configDir)
	config := &osdConfig{rootPath: configDir}

	err := loadOSDInfo(config)
	assert.NotNil(t, err)

	// write the info to disk
	whoFile := path.Join(configDir, "whoami")
	ioutil.WriteFile(whoFile, []byte("23"), 0644)
	defer os.Remove(whoFile)
	fsidFile := path.Join(configDir, "fsid")
	testUUID, _ := uuid.NewUUID()
	ioutil.WriteFile(fsidFile, []byte(testUUID.String()), 0644)
	defer os.Remove(fsidFile)

	// check the successful osd info
	err = loadOSDInfo(config)
	assert.Nil(t, err)
	assert.Equal(t, 23, config.id)
	assert.Equal(t, testUUID, config.uuid)
}

func TestOSDBootstrap(t *testing.T) {
	clusterName := "mycluster"
	configDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(configDir)

	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutputFile: func(debug bool, actionName string, command string, outFileArg string, args ...string) (string, error) {
			return "{\"key\":\"mysecurekey\"}", nil
		},
	}

	context := &clusterd.Context{Executor: executor, ConfigDir: configDir}
	defer os.RemoveAll(context.ConfigDir)
	err := createOSDBootstrapKeyring(context, clusterName, configDir)
	assert.Nil(t, err)

	targetPath := path.Join(configDir, bootstrapOsdKeyring)
	contents, err := ioutil.ReadFile(targetPath)
	assert.Nil(t, err)
	assert.NotEqual(t, -1, strings.Index(string(contents), "[client.bootstrap-osd]"))
	assert.NotEqual(t, -1, strings.Index(string(contents), "key = mysecurekey"))
	assert.NotEqual(t, -1, strings.Index(string(contents), "caps mon = \"allow profile bootstrap-osd\""))
}

func TestOverwriteRookOwnedPartitions(t *testing.T) {
	// set up a temporary config directory that will be cleaned up after test
	configDir, err := ioutil.TempDir("", "TestOverwriteRookOwnedPartitions")
	if err != nil {
		t.Fatalf("failed to create temp config dir: %+v", err)
	}
	defer os.RemoveAll(configDir)

	// set up mock execute so we can verify the partitioning happens on sda
	executor := &exectest.MockExecutor{}

	// set up a mock function to return "rook owned" partitions on the device and it does not have a filesystem
	outputExecCount := 0
	executor.MockExecuteCommandWithOutput = func(debug bool, name string, command string, args ...string) (string, error) {
		logger.Infof("OUTPUT %d for %s. %s %+v", outputExecCount, name, command, args)
		var output string
		switch outputExecCount {
		case 0: // we'll call this twice, once explicitly below to verify rook owns the partitions and a 2nd time within formatDevice
			assert.Equal(t, command, "lsblk")
			output = `NAME="sda" SIZE="65" TYPE="disk" PKNAME=""
NAME="sda1" SIZE="30" TYPE="part" PKNAME="sda"
NAME="sda2" SIZE="10" TYPE="part" PKNAME="sda"
NAME="sda3" SIZE="20" TYPE="part" PKNAME="sda"`
		case 1:
			assert.Equal(t, "udevadm info sda1", name)
			output = "ID_PART_ENTRY_NAME=ROOK-OSD0-WAL"
		case 2:
			assert.Equal(t, "udevadm info sda2", name)
			output = "ID_PART_ENTRY_NAME=ROOK-OSD0-DB"
		case 3:
			assert.Equal(t, "udevadm info sda3", name)
			output = "ID_PART_ENTRY_NAME=ROOK-OSD0-BLOCK"
		case 4:
			assert.Equal(t, "lsblk /dev/sda", name)
			output = ""
		case 5:
			assert.Equal(t, "get filesystem type for /dev/sda", name)
			output = ""
		}
		outputExecCount++
		return output, nil
	}

	storeConfig := config.StoreConfig{StoreType: config.Bluestore}

	// set up a partition scheme entry for sda (collocated metadata and data)
	entry := config.NewPerfSchemeEntry(storeConfig.StoreType)
	entry.ID = 1
	entry.OsdUUID = uuid.Must(uuid.NewRandom())
	config.PopulateCollocatedPerfSchemeEntry(entry, "sda", storeConfig)

	context := &clusterd.Context{Executor: executor, ConfigDir: configDir}
	context.Devices = []*sys.LocalDisk{
		{Name: "sda", Size: 65},
	}
	config := &osdConfig{configRoot: configDir, rootPath: filepath.Join(configDir, "osd1"), id: entry.ID,
		uuid: entry.OsdUUID, dir: false, partitionScheme: entry, kv: mockKVStore(), storeName: config.GetConfigStoreName("node123")}

	// ensure that our mocking makes it look like rook owns the partitions on sda
	partitions, _, err := sys.GetDevicePartitions("sda", context.Executor)
	assert.Nil(t, err)
	assert.True(t, sys.RookOwnsPartitions(partitions))

	// try to format the device.  even though the device has existing partitions, they are owned by rook, so it is safe
	// to format and the format/partitioning will happen.
	devInfo, err := formatDevice(context, config, false, storeConfig)
	assert.Nil(t, devInfo)
	assert.Nil(t, err)
	assert.Equal(t, 6, outputExecCount)
}

func TestPartitionBluestoreMetadata(t *testing.T) {
	// set up a temporary config directory that will be cleaned up after test
	configDir, err := ioutil.TempDir("", "TestPartitionBluestoreMetadata")
	if err != nil {
		t.Fatalf("failed to create temp config dir: %+v", err)
	}
	defer os.RemoveAll(configDir)

	nodeID := "node123"

	execCount := 0
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommand = func(debug bool, name string, command string, args ...string) error {
		logger.Infof("RUN %d for '%s'. %s %+v", execCount, name, command, args)
		assert.Equal(t, "sgdisk", command)
		switch execCount {
		case 0:
			assert.Equal(t, []string{"--zap-all", "/dev/sda"}, args)
		case 1:
			assert.Equal(t, []string{"--clear", "--mbrtogpt", "/dev/sda"}, args)
		case 2:
			assert.Equal(t, 14, len(args))
			assert.Equal(t, "--change-name=1:ROOK-OSD1-WAL", args[1])
			assert.Equal(t, "--change-name=2:ROOK-OSD1-DB", args[4])
			assert.Equal(t, "--change-name=3:ROOK-OSD2-WAL", args[7])
			assert.Equal(t, "--change-name=4:ROOK-OSD2-DB", args[10])
		}
		execCount++
		return nil
	}

	context := &clusterd.Context{Executor: executor, ConfigDir: configDir}

	// create metadata partition information for 2 OSDs (sdb, sdc) storing their metadata on device sda
	storeConfig := config.StoreConfig{StoreType: config.Bluestore, WalSizeMB: 1, DatabaseSizeMB: 2}
	metadata := config.NewMetadataDeviceInfo("sda")

	e1 := config.NewPerfSchemeEntry(config.Bluestore)
	e1.ID = 1
	e1.OsdUUID = uuid.Must(uuid.NewRandom())
	config.PopulateDistributedPerfSchemeEntry(e1, "sdb", metadata, storeConfig)

	e2 := config.NewPerfSchemeEntry(config.Bluestore)
	e2.ID = 2
	e2.OsdUUID = uuid.Must(uuid.NewRandom())
	config.PopulateDistributedPerfSchemeEntry(e2, "sdc", metadata, storeConfig)

	// perform the metadata device partition
	err = partitionMetadata(context, metadata, mockKVStore(), config.GetConfigStoreName(nodeID))
	assert.Nil(t, err)
	assert.Equal(t, 3, execCount)

	// verify that the metadata device has been associated with the OSDs that are storing their metadata on it,
	// e.g. OSDs 1 and 2
}

func TestPartitionBluestoreMetadataSafe(t *testing.T) {
	// set up a temporary config directory that will be cleaned up after test
	configDir, err := ioutil.TempDir("", "TestPartitionBluestoreMetadataSafe")
	if err != nil {
		t.Fatalf("failed to create temp config dir: %+v", err)
	}
	defer os.RemoveAll(configDir)

	nodeID := "node123"
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommandWithOutput = func(debug bool, name string, command string, args ...string) (string, error) {
		if command == "udevadm" {
			// mock that the metadata device already has a filesystem, this should abort the partition effort
			if strings.Index(name, "nvme01") != -1 {
				return udevFSOutput, nil
			}
		}

		return "", nil
	}

	context := &clusterd.Context{Executor: executor, ConfigDir: configDir}

	// create metadata partition information for 1 OSD (sda) storing its metadata on device nvme01
	storeConfig := config.StoreConfig{StoreType: config.Bluestore, WalSizeMB: 1, DatabaseSizeMB: 2}
	metadata := config.NewMetadataDeviceInfo("nvme01")
	e1 := config.NewPerfSchemeEntry(config.Bluestore)
	e1.ID = 1
	e1.OsdUUID = uuid.Must(uuid.NewRandom())
	config.PopulateDistributedPerfSchemeEntry(e1, "sda", metadata, storeConfig)

	// attempt to perform the metadata device partition.  this should fail because we should detect
	// that the metadata device has a filesystem already (not safe to format)
	err = partitionMetadata(context, metadata, mockKVStore(), config.GetConfigStoreName(nodeID))
	assert.NotNil(t, err)
	assert.True(t, strings.Contains(err.Error(), "already in use (not by rook)"))
}

func TestPartitionOSD(t *testing.T) {
	testPartitionOSDHelper(t, config.StoreConfig{StoreType: config.Bluestore, WalSizeMB: 1, DatabaseSizeMB: 2})
	testPartitionOSDHelper(t, config.StoreConfig{StoreType: config.Filestore})
}

func testPartitionOSDHelper(t *testing.T, storeConfig config.StoreConfig) {
	// set up a temporary config directory that will be cleaned up after test
	configDir, err := ioutil.TempDir("", "TestPartitionBluestoreOSD")
	if err != nil {
		t.Fatalf("failed to create temp config dir: %+v", err)
	}
	defer os.RemoveAll(configDir)

	// setup the mock executor to validate the calls to partition the device
	execCount := 0
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommand = func(debug bool, name string, command string, args ...string) error {
		logger.Infof("RUN %d for '%s'. %s %+v", execCount, name, command, args)
		switch execCount {
		case 0:
			assert.Equal(t, "sgdisk", command)
			assert.Equal(t, []string{"--zap-all", "/dev/sda"}, args)
		case 1:
			assert.Equal(t, "sgdisk", command)
			assert.Equal(t, []string{"--clear", "--mbrtogpt", "/dev/sda"}, args)
		}

		if storeConfig.StoreType == config.Bluestore {
			switch execCount {
			case 2:
				assert.Equal(t, 11, len(args))
				assert.Equal(t, "--change-name=1:ROOK-OSD1-WAL", args[1])
				assert.Equal(t, "--change-name=2:ROOK-OSD1-DB", args[4])
				assert.Equal(t, "--change-name=3:ROOK-OSD1-BLOCK", args[7])
			case 3:
				assert.Fail(t, fmt.Sprintf("unexpected case %d", execCount))
			case 4:
				assert.Fail(t, fmt.Sprintf("unexpected case %d", execCount))
			case 5:
				assert.Fail(t, "unexpected bluestore command")
			}
		} else { // filestore
			switch execCount {
			case 2:
				assert.Equal(t, 5, len(args))
				assert.Equal(t, "--change-name=1:ROOK-OSD1-FS-DATA", args[1])
			case 3:
				assert.Equal(t, "mkfs.ext4", command)
			case 4:
				assert.Equal(t, "mount", command)
			case 5:
				assert.Fail(t, "unexpected filestore command")
			}
		}
		execCount++
		return nil
	}

	// setup a context with 1 disk: sda
	context := &clusterd.Context{Executor: executor, ConfigDir: configDir}
	context.Devices = []*sys.LocalDisk{
		{Name: "sda", Size: 100},
	}

	// setup a partition scheme for data and metadata to be collocated on sda
	entry := config.NewPerfSchemeEntry(storeConfig.StoreType)
	entry.ID = 1
	entry.OsdUUID = uuid.Must(uuid.NewRandom())
	config.PopulateCollocatedPerfSchemeEntry(entry, "sda", storeConfig)

	cfg := &osdConfig{configRoot: configDir, rootPath: filepath.Join(configDir, "osd1"), id: entry.ID,
		uuid: entry.OsdUUID, dir: false, partitionScheme: entry, kv: mockKVStore(), storeName: config.GetConfigStoreName("node123")}

	// partition the OSD on sda now
	devPartInfo, err := partitionOSD(context, cfg)
	assert.Nil(t, err)

	if storeConfig.StoreType == config.Bluestore {
		assert.Equal(t, 3, execCount)
		assert.Nil(t, devPartInfo)
	} else {
		assert.Equal(t, 5, execCount)
		assert.NotEqual(t, "", devPartInfo.deviceUUID)
		assert.NotEqual(t, "", devPartInfo.pathToUnmount)
	}

	// verify that both the data and metadata have been associated with the device in etcd (since data/metadata are collocated)
	dataDetails, err := getDataPartitionDetails(cfg)
	assert.Nil(t, err)
	assert.NotEqual(t, "", dataDetails.DiskUUID)
}
