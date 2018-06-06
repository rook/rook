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
package config

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/rook/rook/pkg/operator/k8sutil"
	testop "github.com/rook/rook/pkg/operator/test"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSchemeSaveLoad(t *testing.T) {
	kv := mockKVStore()
	storeName := GetConfigStoreName("node123")

	// loading the scheme when there is no scheme file should return an empty scheme with no error
	scheme, err := LoadScheme(kv, storeName)
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
		BlockPartitionType: {
			Device:        "sdb",
			DiskUUID:      uuid.Must(uuid.NewRandom()).String(),
			PartitionUUID: uuid.Must(uuid.NewRandom()).String(),
			SizeMB:        -1,
			OffsetMB:      1,
		},
	}
	scheme.Entries = append(scheme.Entries, e1)

	// save the scheme to disk, should be no error
	err = scheme.SaveScheme(kv, storeName)
	assert.Nil(t, err)

	// now load the saved scheme, it should load an equal object to what was saved
	savedScheme, err := LoadScheme(kv, storeName)
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
	// A device will use bluestore unless explicitly requested to be filestore (the default is blank)
	// This test verifies we get the right values
	testPartitionArgs(t, "")

	//Test explicitly testing bluestore
	testPartitionArgs(t, Bluestore)

	//Test for Filestore
	testPartitionArgs(t, Filestore)
}

func testPartitionArgs(t *testing.T, store string) {
	//Create args to test the store
	e := NewPerfSchemeEntry(store)
	e.ID = 1
	e.OsdUUID = uuid.Must(uuid.NewRandom())
	storeConfig := StoreConfig{}
	expectedArgs := []string{}

	if store == Filestore {
		storeConfig = StoreConfig{StoreType: Filestore}
	} else {
		storeConfig = StoreConfig{WalSizeMB: 1, DatabaseSizeMB: 2}
	}
	err := PopulateCollocatedPerfSchemeEntry(e, "sdb", storeConfig)
	assert.Nil(t, err)

	if store == Filestore {
		expectedArgs = []string{
			"--largest-new=1",
			"--change-name=1:ROOK-OSD1-FS-DATA", fmt.Sprintf("--partition-guid=1:%s", e.Partitions[FilestoreDataPartitionType].PartitionUUID),
			fmt.Sprintf("--disk-guid=%s", e.Partitions[FilestoreDataPartitionType].DiskUUID), "/dev/sdb",
		}
	} else {
		expectedArgs = []string{
			"--new=1:2048:+2048", "--change-name=1:ROOK-OSD1-WAL", fmt.Sprintf("--partition-guid=1:%s", e.Partitions[WalPartitionType].PartitionUUID),
			"--new=2:4096:+4096", "--change-name=2:ROOK-OSD1-DB", fmt.Sprintf("--partition-guid=2:%s", e.Partitions[DatabasePartitionType].PartitionUUID),
			"--largest-new=3", "--change-name=3:ROOK-OSD1-BLOCK", fmt.Sprintf("--partition-guid=3:%s", e.Partitions[BlockPartitionType].PartitionUUID),
			fmt.Sprintf("--disk-guid=%s", e.Partitions[BlockPartitionType].DiskUUID), "/dev/sdb",
		}
	}

	args := e.GetPartitionArgs()
	assert.Equal(t, expectedArgs, args)

	logger.Noticef("%+v", args)
}

func TestDeleteSchemeEntry(t *testing.T) {
	storeConfig := StoreConfig{WalSizeMB: 1, DatabaseSizeMB: 2}

	// create a partition scheme with some entries
	scheme := NewPerfScheme()
	e1 := NewPerfSchemeEntry(Bluestore)
	e1.ID = 1
	e1.OsdUUID = uuid.Must(uuid.NewRandom())
	PopulateCollocatedPerfSchemeEntry(e1, "sdb", storeConfig)
	scheme.Entries = append(scheme.Entries, e1)

	e2 := NewPerfSchemeEntry(Bluestore)
	e2.ID = 2
	e2.OsdUUID = uuid.Must(uuid.NewRandom())
	PopulateCollocatedPerfSchemeEntry(e2, "sdc", storeConfig)
	scheme.Entries = append(scheme.Entries, e2)

	assert.Equal(t, 2, len(scheme.Entries))

	// delete 1 of the entries, the scheme should now contain 1 entry
	err := scheme.DeleteSchemeEntry(e1)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(scheme.Entries))

	// delete the last entry, the scheme should now be empty
	err = scheme.DeleteSchemeEntry(e2)
	assert.Nil(t, err)
	assert.Equal(t, 0, len(scheme.Entries))

	// try to delete an entry that doesn't exist, an error should be returned
	e3 := NewPerfSchemeEntry(Bluestore)
	err = scheme.DeleteSchemeEntry(e3)
	assert.NotNil(t, err)
}

func TestUpdateSchemeEntry(t *testing.T) {
	storeConfig := StoreConfig{WalSizeMB: 1, DatabaseSizeMB: 2}

	// create a partition scheme with an entry in it
	scheme := NewPerfScheme()
	e1 := NewPerfSchemeEntry(Bluestore)
	e1.ID = 835
	e1.OsdUUID = uuid.Must(uuid.NewRandom())
	PopulateCollocatedPerfSchemeEntry(e1, "sdx", storeConfig)
	scheme.Entries = append(scheme.Entries, e1)

	// create a new entry that we will use to update the existing entry (note that the new entry is for the same OSD ID)
	e1New := NewPerfSchemeEntry(Filestore)
	e1New.ID = e1.ID
	e1New.OsdUUID = uuid.Must(uuid.NewRandom())
	PopulateCollocatedPerfSchemeEntry(e1New, "sdy", storeConfig)

	// update the existing entry with the new entry and verify that the scheme only has the new entry now
	err := scheme.UpdateSchemeEntry(e1New)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(scheme.Entries))
	assert.Equal(t, e1New, scheme.Entries[0])
}

func mockKVStore() *k8sutil.ConfigMapKVStore {
	clientset := testop.New(1)
	return k8sutil.NewConfigMapKVStore("myns", clientset, metav1.OwnerReference{})
}
