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
	"github.com/rook/rook/pkg/clusterd/inventory"
	"github.com/rook/rook/pkg/util"
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

	targetPath := getBootstrapOSDKeyringPath(configDir, clusterName)
	defer os.Remove(targetPath)

	executor := &exectest.MockExecutor{
		MockExecuteCommandWithOutput: func(actionName string, command string, args ...string) (string, error) {
			return "{\"key\":\"mysecurekey\"}", nil
		},
	}

	context := &clusterd.Context{Executor: executor, ConfigDir: configDir}
	defer os.RemoveAll(context.ConfigDir)
	err := createOSDBootstrapKeyring(context, clusterName)
	assert.Nil(t, err)

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

	nodeID := "node123"
	etcdClient := util.NewMockEtcdClient()

	// set up mock execute so we can verify the partitioning happens on sda
	execCount := 0
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommand = func(name string, command string, args ...string) error {
		logger.Infof("RUN %d for '%s'. %s %+v", execCount, name, command, args)
		assert.Equal(t, "sgdisk", command)
		switch execCount {
		case 0:
			assert.Equal(t, []string{"--zap-all", "/dev/sda"}, args)
		case 1:
			assert.Equal(t, []string{"--clear", "--mbrtogpt", "/dev/sda"}, args)
		case 2:
			assert.Equal(t, 11, len(args))
			assert.Equal(t, "--change-name=1:ROOK-OSD1-WAL", args[1])
			assert.Equal(t, "--change-name=2:ROOK-OSD1-DB", args[4])
			assert.Equal(t, "--change-name=3:ROOK-OSD1-BLOCK", args[7])
			assert.Equal(t, "/dev/sda", args[10])
		}
		execCount++
		return nil
	}

	// set up a mock function to return "rook owned" partitions on the device and it does not have a filesystem
	outputExecCount := 0
	executor.MockExecuteCommandWithOutput = func(name string, command string, args ...string) (string, error) {
		logger.Infof("OUTPUT %d for %s. %s %+v", outputExecCount, name, command, args)
		var output string
		switch outputExecCount {
		case 0, 1: // we'll call this twice, once explicitly below to verify rook owns the partitions and a 2nd time within formatDevice
			assert.Equal(t, command, "lsblk")
			output = `NAME="sda" SIZE="65" TYPE="disk" PKNAME="" PARTLABEL=""
NAME="sda1" SIZE="30" TYPE="part" PKNAME="sda" PARTLABEL="ROOK-OSD0-WAL"
NAME="sda2" SIZE="10" TYPE="part" PKNAME="sda" PARTLABEL="ROOK-OSD0-DB"
NAME="sda3" SIZE="20" TYPE="part" PKNAME="sda" PARTLABEL="ROOK-OSD0-BLOCK"`
		case 2:
			assert.Equal(t, command, "df")
			output = ""
		}
		outputExecCount++
		return output, nil
	}

	storeConfig := StoreConfig{StoreType: Bluestore}

	// set up a partition scheme entry for sda (collocated metadata and data)
	entry := NewPerfSchemeEntry(storeConfig.StoreType)
	entry.ID = 1
	entry.OsdUUID = uuid.Must(uuid.NewRandom())
	PopulateCollocatedPerfSchemeEntry(entry, "sda", storeConfig)

	context := &clusterd.Context{DirectContext: clusterd.DirectContext{EtcdClient: etcdClient, NodeID: nodeID, Inventory: createInventory()},
		Executor: executor, ConfigDir: configDir}
	context.Inventory.Local.Disks = []*inventory.LocalDisk{
		&inventory.LocalDisk{Name: "sda", Size: 65},
	}
	config := &osdConfig{configRoot: configDir, rootPath: filepath.Join(configDir, "osd1"), id: entry.ID,
		uuid: entry.OsdUUID, dir: false, partitionScheme: entry}

	// ensure that our mocking makes it look like rook owns the partitions on sda
	partitions, _, err := sys.GetDevicePartitions("sda", context.Executor)
	assert.Nil(t, err)
	assert.True(t, rookOwnsPartitions(partitions))

	// try to format the device.  even though the device has existing partitions, they are owned by rook, so it is safe
	// to format and the format/partitioning will happen.
	err = formatDevice(context, config, false, storeConfig)
	assert.Nil(t, err)
	assert.Equal(t, 3, execCount)
	assert.Equal(t, 3, outputExecCount)
}

func TestPartitionBluestoreMetadata(t *testing.T) {
	// set up a temporary config directory that will be cleaned up after test
	configDir, err := ioutil.TempDir("", "TestPartitionBluestoreMetadata")
	if err != nil {
		t.Fatalf("failed to create temp config dir: %+v", err)
	}
	defer os.RemoveAll(configDir)

	nodeID := "node123"
	etcdClient := util.NewMockEtcdClient()

	execCount := 0
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommand = func(name string, command string, args ...string) error {
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

	context := &clusterd.Context{DirectContext: clusterd.DirectContext{EtcdClient: etcdClient, NodeID: nodeID}, Executor: executor, ConfigDir: configDir}

	// create metadata partition information for 2 OSDs (sdb, sdc) storing their metadata on device sda
	storeConfig := StoreConfig{StoreType: Bluestore, WalSizeMB: 1, DatabaseSizeMB: 2}
	metadata := NewMetadataDeviceInfo("sda")

	e1 := NewPerfSchemeEntry(Bluestore)
	e1.ID = 1
	e1.OsdUUID = uuid.Must(uuid.NewRandom())
	PopulateDistributedPerfSchemeEntry(e1, "sdb", metadata, storeConfig)

	e2 := NewPerfSchemeEntry(Bluestore)
	e2.ID = 2
	e2.OsdUUID = uuid.Must(uuid.NewRandom())
	PopulateDistributedPerfSchemeEntry(e2, "sdc", metadata, storeConfig)

	// perform the metadata device partition
	err = partitionMetadata(context, metadata, configDir)
	assert.Nil(t, err)
	assert.Equal(t, 3, execCount)

	// verify that the metadata device has been associated with the OSDs that are storing their metadata on it,
	// e.g. OSDs 1 and 2
	desiredIDsRaw := etcdClient.GetValue(
		fmt.Sprintf("/rook/services/ceph/osd/desired/node123/device/%s/osd-id-metadata", metadata.DiskUUID))
	desiredIds := strings.Split(desiredIDsRaw, ",")
	assert.True(t, util.CreateSet(desiredIds).Equals(util.CreateSet([]string{"1", "2"})))
}

func TestPartitionOSD(t *testing.T) {
	testPartitionOSDHelper(t, StoreConfig{StoreType: Bluestore, WalSizeMB: 1, DatabaseSizeMB: 2})
	testPartitionOSDHelper(t, StoreConfig{StoreType: Filestore})
}

func testPartitionOSDHelper(t *testing.T, storeConfig StoreConfig) {
	// set up a temporary config directory that will be cleaned up after test
	configDir, err := ioutil.TempDir("", "TestPartitionBluestoreOSD")
	if err != nil {
		t.Fatalf("failed to create temp config dir: %+v", err)
	}
	defer os.RemoveAll(configDir)

	nodeID := "node123"
	etcdClient := util.NewMockEtcdClient()

	// setup the mock executor to validate the calls to partition the device
	execCount := 0
	executor := &exectest.MockExecutor{}
	executor.MockExecuteCommand = func(name string, command string, args ...string) error {
		logger.Infof("RUN %d for '%s'. %s %+v", execCount, name, command, args)
		if execCount <= 2 {
			assert.Equal(t, "sgdisk", command)
		}
		switch execCount {
		case 0:
			assert.Equal(t, []string{"--zap-all", "/dev/sda"}, args)
		case 1:
			assert.Equal(t, []string{"--clear", "--mbrtogpt", "/dev/sda"}, args)
		case 2:
			if storeConfig.StoreType == Bluestore {
				assert.Equal(t, 11, len(args))
				assert.Equal(t, "--change-name=1:ROOK-OSD1-WAL", args[1])
				assert.Equal(t, "--change-name=2:ROOK-OSD1-DB", args[4])
				assert.Equal(t, "--change-name=3:ROOK-OSD1-BLOCK", args[7])
			} else {
				assert.Equal(t, 5, len(args))
				assert.Equal(t, "--change-name=1:ROOK-OSD1-FS-DATA", args[1])
			}
		case 3:
			if storeConfig.StoreType == Bluestore {
				assert.Fail(t, fmt.Sprintf("unexpected case %d", execCount))
			} else {
				assert.Equal(t, "mkfs.ext4", command)
			}
		case 4:
			if storeConfig.StoreType == Bluestore {
				assert.Fail(t, fmt.Sprintf("unexpected case %d", execCount))
			} else {
				assert.Equal(t, "mount", command)
			}
		}
		execCount++
		return nil
	}

	// setup a context with 1 disk: sda
	context := &clusterd.Context{DirectContext: clusterd.DirectContext{EtcdClient: etcdClient, NodeID: nodeID, Inventory: createInventory()},
		Executor: executor, ConfigDir: configDir}
	context.Inventory.Local.Disks = []*inventory.LocalDisk{
		&inventory.LocalDisk{Name: "sda", Size: 100},
	}

	// setup a partition scheme for data and metadata to be collocated on sda
	entry := NewPerfSchemeEntry(storeConfig.StoreType)
	entry.ID = 1
	entry.OsdUUID = uuid.Must(uuid.NewRandom())
	PopulateCollocatedPerfSchemeEntry(entry, "sda", storeConfig)

	config := &osdConfig{configRoot: configDir, rootPath: filepath.Join(configDir, "osd1"), id: entry.ID,
		uuid: entry.OsdUUID, dir: false, partitionScheme: entry}

	// partition the OSD on sda now
	err = partitionOSD(context, config)
	assert.Nil(t, err)

	if storeConfig.StoreType == Bluestore {
		assert.Equal(t, 3, execCount)
	} else {
		assert.Equal(t, 5, execCount)
	}

	// verify that both the data and metadata have been associated with the device in etcd (since data/metadata are collocated)
	dataDetails, err := getDataPartitionDetails(config)
	assert.Nil(t, err)
	dataID := etcdClient.GetValue(
		fmt.Sprintf("/rook/services/ceph/osd/desired/node123/device/%s/osd-id-data", dataDetails.DiskUUID))
	assert.Equal(t, "1", dataID)

	metadataID := etcdClient.GetValue(
		fmt.Sprintf("/rook/services/ceph/osd/desired/node123/device/%s/osd-id-metadata", dataDetails.DiskUUID))
	assert.Equal(t, "1", metadataID)
}
