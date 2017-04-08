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
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestSchemeSaveLoad(t *testing.T) {
	// set up a temporary config directory that will be cleaned up after test
	configDir, err := ioutil.TempDir("", "TestSchemeSaveLoad")
	if err != nil {
		t.Fatalf("failed to create temp config dir: %+v", err)
	}
	defer os.RemoveAll(configDir)

	// loading the scheme when there is no scheme file should return an empty scheme with no error
	scheme, err := LoadScheme(configDir)
	assert.Nil(t, err)
	assert.NotNil(t, scheme)
	assert.Equal(t, 0, len(scheme.Entries))
	assert.Nil(t, scheme.Metadata)

	// add some entries to the scheme
	scheme.Metadata = NewMetadataDeviceInfo("sda")
	scheme.Metadata.DiskUUID = uuid.Must(uuid.NewRandom()).String()
	m1 := &MetadataDevicePartition{ID: 1, OsdUUID: uuid.Must(uuid.NewRandom()), Type: WalPartitionType,
		PartitionUUID: uuid.Must(uuid.NewRandom()).String(), SizeMB: 100, OffsetMB: 1}
	m2 := &MetadataDevicePartition{ID: 1, OsdUUID: m1.OsdUUID, Type: DatabasePartitionType,
		PartitionUUID: uuid.Must(uuid.NewRandom()).String(), SizeMB: 200, OffsetMB: 101}
	scheme.Metadata.Partitions = append(scheme.Metadata.Partitions, []*MetadataDevicePartition{m1, m2}...)

	e1 := &PerfSchemeEntry{ID: 1, OsdUUID: m1.OsdUUID}
	e1.Partitions = map[PartitionType]*PerfSchemePartitionDetails{
		BlockPartitionType: &PerfSchemePartitionDetails{
			Device:        "sdb",
			DiskUUID:      uuid.Must(uuid.NewRandom()).String(),
			PartitionUUID: uuid.Must(uuid.NewRandom()).String(),
			SizeMB:        -1,
			OffsetMB:      1,
		},
	}

	// save the scheme to disk, should be no error
	err = scheme.Save(configDir)
	assert.Nil(t, err)

	// now load the saved scheme, it should load an equal object to what was saved
	savedScheme, err := LoadScheme(configDir)
	assert.Nil(t, err)
	assert.Equal(t, scheme, savedScheme)
}

func TestPopulateCollocatedPerfSchemeEntry(t *testing.T) {
	entry := NewPerfSchemeEntry(Bluestore)
	entry.ID = 10
	entry.OsdUUID = uuid.Must(uuid.NewRandom())
	err := PopulateCollocatedPerfSchemeEntry(entry, "sda", StoreConfig{WalSizeMB: 1, DatabaseSizeMB: 2})
	assert.Nil(t, err)

	// verify the populated collocated partition entries
	assert.Equal(t, 3, len(entry.Partitions))
	verifyPartitionDetails(t, entry, WalPartitionType, "sda", 1, 1)
	verifyPartitionDetails(t, entry, DatabasePartitionType, "sda", 2, 2)
	verifyPartitionDetails(t, entry, BlockPartitionType, "sda", 4, -1)

}

func TestPopulateDistributedPerfSchemeEntry(t *testing.T) {
	metadata := NewMetadataDeviceInfo("sda")

	entry := NewPerfSchemeEntry(Bluestore)
	entry.ID = 20
	entry.OsdUUID = uuid.Must(uuid.NewRandom())

	err := PopulateDistributedPerfSchemeEntry(entry, "sdb", metadata, StoreConfig{WalSizeMB: 1, DatabaseSizeMB: 2})
	assert.Nil(t, err)

	// verify the populated distributed partition entries (metadata partitions should be on metadata device, block
	// partition should be on its own device)
	assert.Equal(t, 3, len(entry.Partitions))
	verifyPartitionDetails(t, entry, WalPartitionType, "sda", 1, 1)
	verifyPartitionDetails(t, entry, DatabasePartitionType, "sda", 2, 2)
	verifyPartitionDetails(t, entry, BlockPartitionType, "sdb", 1, -1)

	// verify that the metadata device info was populated as well
	assert.Equal(t, 2, len(metadata.Partitions))
	verifyMetadataDevicePartition(t, metadata, 0, entry.ID, entry.OsdUUID, WalPartitionType, 1, 1)
	verifyMetadataDevicePartition(t, metadata, 1, entry.ID, entry.OsdUUID, DatabasePartitionType, 2, 2)
}

func verifyPartitionDetails(t *testing.T, entry *PerfSchemeEntry, partType PartitionType, device string, offset, size int) {
	part, ok := entry.Partitions[partType]
	assert.True(t, ok)
	assert.NotNil(t, part)
	assert.Equal(t, device, part.Device)
	assert.Equal(t, offset, part.OffsetMB)
	assert.Equal(t, size, part.SizeMB)
}

func verifyMetadataDevicePartition(t *testing.T, info *MetadataDeviceInfo, index int, osdID int, osdUUID uuid.UUID,
	partType PartitionType, offset, size int) {

	part := info.Partitions[index]
	assert.NotNil(t, part)
	assert.Equal(t, osdID, part.ID)
	assert.Equal(t, osdUUID, part.OsdUUID)
	assert.Equal(t, partType, part.Type)
	assert.Equal(t, offset, part.OffsetMB)
	assert.Equal(t, size, part.SizeMB)
}

func TestMetadataGetPartitionArgs(t *testing.T) {
	metadata := NewMetadataDeviceInfo("sda")

	e1 := NewPerfSchemeEntry(Bluestore)
	e1.ID = 1
	e1.OsdUUID = uuid.Must(uuid.NewRandom())

	e2 := NewPerfSchemeEntry(Bluestore)
	e2.ID = 2
	e2.OsdUUID = uuid.Must(uuid.NewRandom())

	storeConfig := StoreConfig{WalSizeMB: 1, DatabaseSizeMB: 2}
	err := PopulateDistributedPerfSchemeEntry(e1, "sdb", metadata, storeConfig)
	assert.Nil(t, err)
	err = PopulateDistributedPerfSchemeEntry(e2, "sdc", metadata, storeConfig)
	assert.Nil(t, err)

	expectedArgs := []string{
		"--new=1:2048:+2048", "--change-name=1:ROOK-OSD1-WAL", fmt.Sprintf("--partition-guid=1:%s", metadata.Partitions[0].PartitionUUID),
		"--new=2:4096:+4096", "--change-name=2:ROOK-OSD1-DB", fmt.Sprintf("--partition-guid=2:%s", metadata.Partitions[1].PartitionUUID),
		"--new=3:8192:+2048", "--change-name=3:ROOK-OSD2-WAL", fmt.Sprintf("--partition-guid=3:%s", metadata.Partitions[2].PartitionUUID),
		"--new=4:10240:+4096", "--change-name=4:ROOK-OSD2-DB", fmt.Sprintf("--partition-guid=4:%s", metadata.Partitions[3].PartitionUUID),
		fmt.Sprintf("--disk-guid=%s", metadata.DiskUUID), "/dev/sda",
	}

	// get the partition args and verify against expected
	args := metadata.GetPartitionArgs()
	assert.Equal(t, expectedArgs, args)
}

func TestSchemeEntryGetPartitionArgs(t *testing.T) {
	e1 := NewPerfSchemeEntry(Bluestore)
	e1.ID = 1
	e1.OsdUUID = uuid.Must(uuid.NewRandom())

	storeConfig := StoreConfig{WalSizeMB: 1, DatabaseSizeMB: 2}
	err := PopulateCollocatedPerfSchemeEntry(e1, "sdb", storeConfig)
	assert.Nil(t, err)

	expectedArgs := []string{
		"--new=1:2048:+2048", "--change-name=1:ROOK-OSD1-WAL", fmt.Sprintf("--partition-guid=1:%s", e1.Partitions[WalPartitionType].PartitionUUID),
		"--new=2:4096:+4096", "--change-name=2:ROOK-OSD1-DB", fmt.Sprintf("--partition-guid=2:%s", e1.Partitions[DatabasePartitionType].PartitionUUID),
		"--largest-new=3", "--change-name=3:ROOK-OSD1-BLOCK", fmt.Sprintf("--partition-guid=3:%s", e1.Partitions[BlockPartitionType].PartitionUUID),
		fmt.Sprintf("--disk-guid=%s", e1.Partitions[BlockPartitionType].DiskUUID), "/dev/sdb",
	}

	// get the partition args and verify against expected
	args := e1.GetPartitionArgs()
	assert.Equal(t, expectedArgs, args)

	logger.Noticef("%+v", args)
}
