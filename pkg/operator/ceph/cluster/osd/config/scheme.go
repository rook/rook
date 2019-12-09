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
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/rook/rook/pkg/operator/k8sutil"
)

const (
	Filestore             = "filestore"
	Bluestore             = "bluestore"
	UseRemainingSpace     = -1
	schemeKeyName         = "partition-scheme"
	WalDefaultSizeMB      = 576
	DBDefaultSizeMB       = 20480
	JournalDefaultSizeMB  = 5120
	BluestoreDirBlockName = "bluestore-block"
	BluestoreDirWalName   = "bluestore-wal"
	BluestoreDirDBName    = "bluestore-db"
)

type PartitionType int

const (
	WalPartitionType PartitionType = iota
	DatabasePartitionType
	BlockPartitionType
	FilestoreDataPartitionType
)

// top level representation of an overall performance oriented partition scheme, with a dedicated metadata device
// and entries for all OSDs that define where their partitions live
type PerfScheme struct {
	Metadata *MetadataDeviceInfo `json:"metadata"`
	Entries  []*PerfSchemeEntry  `json:"entries"`
}

// represents an OSD and details about all of its partitions
type PerfSchemeEntry struct {
	ID         int                                           `json:"id"`
	OsdUUID    uuid.UUID                                     `json:"osdUuid"`
	Partitions map[PartitionType]*PerfSchemePartitionDetails `json:"partitions"` // mapping of partition name to its details
	StoreType  string                                        `json:"storeType,omitempty"`
	FSCreated  bool                                          `json:"fsCreated"`
}

// details for 1 OSD partition
type PerfSchemePartitionDetails struct {
	Device        string `json:"device"`
	DiskUUID      string `json:"diskUuid"`
	PartitionUUID string `json:"partitionUuid"`
	SizeMB        int    `json:"sizeMB"`
	OffsetMB      int    `json:"offsetMB"`
}

// represents a dedicated metadata device and all of the partitions stored on it
type MetadataDeviceInfo struct {
	Device     string                     `json:"device"`
	DiskUUID   string                     `json:"diskUuid"`
	Partitions []*MetadataDevicePartition `json:"partitions"`
}

// represents a specific partition on a metadata device, including details about which OSD it belongs to
type MetadataDevicePartition struct {
	ID            int           `json:"id"`
	OsdUUID       uuid.UUID     `json:"osdUuid"`
	Type          PartitionType `json:"type"`
	PartitionUUID string        `json:"partitionUuid"`
	SizeMB        int           `json:"sizeMB"`
	OffsetMB      int           `json:"offsetMB"`
}

func NewPerfScheme() *PerfScheme {
	return &PerfScheme{
		Entries: []*PerfSchemeEntry{},
	}
}

func NewPerfSchemeEntry(storeType string) *PerfSchemeEntry {
	return &PerfSchemeEntry{
		Partitions: map[PartitionType]*PerfSchemePartitionDetails{}, // mapping of partition name (e.g. WAL) to it's details
		StoreType:  storeType,
	}
}

func NewMetadataDeviceInfo(device string) *MetadataDeviceInfo {
	return &MetadataDeviceInfo{Device: device, Partitions: []*MetadataDevicePartition{}}
}

// Load the persistent partition info from the config directory.
func LoadScheme(kv *k8sutil.ConfigMapKVStore, storeName string) (*PerfScheme, error) {
	schemeRaw, err := kv.GetValue(storeName, schemeKeyName)
	if err != nil {
		if kerrors.IsNotFound(err) {
			// the scheme key doesn't exist yet, just return a new empty scheme with no error
			return NewPerfScheme(), nil
		}

		return nil, err
	}

	var scheme PerfScheme
	err = json.Unmarshal([]byte(schemeRaw), &scheme)
	if err != nil {
		return nil, err
	}

	return &scheme, nil
}

// Save the partition scheme to the config dir
func (s *PerfScheme) SaveScheme(kv *k8sutil.ConfigMapKVStore, storeName string) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}

	err = kv.SetValue(storeName, schemeKeyName, string(b))
	if err != nil {
		return err
	}

	return nil
}

func RemoveFromScheme(e *PerfSchemeEntry, kv *k8sutil.ConfigMapKVStore, storeName string) error {
	savedScheme, err := LoadScheme(kv, storeName)
	if err != nil {
		return errors.Wrapf(err, "failed to load the saved partition scheme")
	}
	if err := savedScheme.DeleteSchemeEntry(e); err != nil {
		return errors.Wrapf(err, "failed to delete partition scheme entry")
	}
	if err := savedScheme.SaveScheme(kv, storeName); err != nil {
		return errors.Wrapf(err, "failed to save partition scheme")
	}

	return nil
}

func (s *PerfScheme) String() string {
	b, _ := json.Marshal(s)
	return string(b)
}

func (s *PerfScheme) UpdateSchemeEntry(e *PerfSchemeEntry) error {
	return s.doSchemeEntryAction(e, func(scheme *PerfScheme, index int, entry *PerfSchemeEntry) {
		// the action to perform if the entry is found is to update the entry
		scheme.Entries[index] = entry
	})
}

func (s *PerfScheme) DeleteSchemeEntry(e *PerfSchemeEntry) error {
	return s.doSchemeEntryAction(e, func(scheme *PerfScheme, index int, entry *PerfSchemeEntry) {
		// the action to perform if the entry is found is to delete the entry
		scheme.Entries = append(scheme.Entries[:index], scheme.Entries[index+1:]...)
	})
}

func (s *PerfScheme) doSchemeEntryAction(entry *PerfSchemeEntry, action func(*PerfScheme, int, *PerfSchemeEntry)) error {
	if entry == nil {
		return errors.New("cannot update a nil entry")
	}

	foundEntry := false
	for i := range s.Entries {
		if s.Entries[i].ID == entry.ID {
			// found the matching existing entry, perform the given action on it
			action(s, i, entry)
			foundEntry = true
			break
		}
	}

	if !foundEntry {
		return errors.Errorf("failed to find entry %+v from entries %+v", entry, s.Entries)
	}

	return nil
}

// populates a partition scheme entry for an OSD where all its partitions are collocated on a single device
func PopulateCollocatedPerfSchemeEntry(entry *PerfSchemeEntry, device string, storeConfig StoreConfig) error {

	if storeConfig.StoreType == Filestore {
		diskUUID, dataUUID, _, err := createFilestoreUUIDs()
		if err != nil {
			return err
		}

		// the filestore data partition will take up the entire given device (and we do not create a separate partition/entry
		// for the journal)
		entry.Partitions[FilestoreDataPartitionType] = &PerfSchemePartitionDetails{
			Device:        device,
			DiskUUID:      diskUUID.String(),
			PartitionUUID: dataUUID.String(),
			SizeMB:        UseRemainingSpace,
			OffsetMB:      1,
		}
	} else {
		diskUUID, walUUID, dbUUID, blockUUID, err := createBluestoreUUIDs()
		if err != nil {
			return err
		}

		walSize := WalDefaultSizeMB
		if storeConfig.WalSizeMB > 0 {
			walSize = storeConfig.WalSizeMB
		}
		dbSize := DBDefaultSizeMB
		if storeConfig.DatabaseSizeMB > 0 {
			dbSize = storeConfig.DatabaseSizeMB
		}

		offset := 1

		// layout the partitions for WAL, DB, and Block
		entry.Partitions[WalPartitionType] = &PerfSchemePartitionDetails{
			Device:        device,
			DiskUUID:      diskUUID.String(),
			PartitionUUID: walUUID.String(),
			SizeMB:        walSize,
			OffsetMB:      offset,
		}
		offset += entry.Partitions[WalPartitionType].SizeMB

		entry.Partitions[DatabasePartitionType] = &PerfSchemePartitionDetails{
			Device:        device,
			DiskUUID:      diskUUID.String(),
			PartitionUUID: dbUUID.String(),
			SizeMB:        dbSize,
			OffsetMB:      offset,
		}
		offset += entry.Partitions[DatabasePartitionType].SizeMB

		entry.Partitions[BlockPartitionType] = &PerfSchemePartitionDetails{
			Device:        device,
			DiskUUID:      diskUUID.String(),
			PartitionUUID: blockUUID.String(),
			SizeMB:        UseRemainingSpace,
			OffsetMB:      offset,
		}
	}

	return nil
}

// populates a partition scheme entry for an OSD that will have distributed partitions: its metadata will live on a
// dedicated metadata device and its block data will live on a dedicated device
func PopulateDistributedPerfSchemeEntry(entry *PerfSchemeEntry, device string, metadataInfo *MetadataDeviceInfo,
	storeConfig StoreConfig) error {

	if storeConfig.StoreType == Filestore {
		// TODO: support separate metadata device for filestore
		return errors.New("filestore not yet supported for distributed partition scheme")
	}

	diskUUID, walUUID, dbUUID, blockUUID, err := createBluestoreUUIDs()
	if err != nil {
		return err
	}

	// the block partition will take up the entire given device
	entry.Partitions[BlockPartitionType] = &PerfSchemePartitionDetails{
		Device:        device,
		DiskUUID:      diskUUID.String(),
		PartitionUUID: blockUUID.String(),
		SizeMB:        UseRemainingSpace,
		OffsetMB:      1,
	}

	// the WAL and DB will be on a separate metadata device
	offset := 1
	numMetadataParts := len(metadataInfo.Partitions)
	if numMetadataParts == 0 {
		// the metadata device hasn't been used yet, create a disk UUID for it
		u, err := uuid.NewRandom()
		if err != nil {
			return errors.Wrapf(err, "failed to get metadata disk uuid")
		}
		metadataInfo.DiskUUID = u.String()
	} else {
		lastEntry := metadataInfo.Partitions[numMetadataParts-1]
		offset = lastEntry.OffsetMB + lastEntry.SizeMB
	}

	walSize := WalDefaultSizeMB
	if storeConfig.WalSizeMB > 0 {
		walSize = storeConfig.WalSizeMB
	}
	dbSize := DBDefaultSizeMB
	if storeConfig.DatabaseSizeMB > 0 {
		dbSize = storeConfig.DatabaseSizeMB
	}

	// record information about the WAL partition
	entry.Partitions[WalPartitionType] = &PerfSchemePartitionDetails{
		Device:        metadataInfo.Device,
		DiskUUID:      metadataInfo.DiskUUID,
		PartitionUUID: walUUID.String(),
		SizeMB:        walSize,
		OffsetMB:      offset,
	}
	walPartitionInfo := &MetadataDevicePartition{
		ID:            entry.ID,
		OsdUUID:       entry.OsdUUID,
		Type:          WalPartitionType,
		PartitionUUID: walUUID.String(),
		SizeMB:        walSize,
		OffsetMB:      offset,
	}
	metadataInfo.Partitions = append(metadataInfo.Partitions, walPartitionInfo)
	offset += entry.Partitions[WalPartitionType].SizeMB

	// record information about the DB partition
	entry.Partitions[DatabasePartitionType] = &PerfSchemePartitionDetails{
		Device:        metadataInfo.Device,
		DiskUUID:      metadataInfo.DiskUUID,
		PartitionUUID: dbUUID.String(),
		SizeMB:        dbSize,
		OffsetMB:      offset,
	}
	dbPartitionInfo := &MetadataDevicePartition{
		ID:            entry.ID,
		OsdUUID:       entry.OsdUUID,
		Type:          DatabasePartitionType,
		PartitionUUID: dbUUID.String(),
		SizeMB:        dbSize,
		OffsetMB:      offset,
	}
	metadataInfo.Partitions = append(metadataInfo.Partitions, dbPartitionInfo)

	return nil
}

func (m *MetadataDeviceInfo) GetPartitionArgs() []string {
	args := []string{}

	for partNum, part := range m.Partitions {
		partArgs := getPartitionArgs(partNum+1, part.PartitionUUID, part.OffsetMB, part.SizeMB, getPartitionLabel(part.ID, part.Type))
		args = append(args, partArgs...)
	}

	// append args for the whole device
	args = append(args, []string{fmt.Sprintf("--disk-guid=%s", m.DiskUUID), "/dev/" + m.Device}...)

	return args
}

func (e *PerfSchemeEntry) String() string {
	b, _ := json.Marshal(e)
	return string(b)
}

func (e *PerfSchemeEntry) GetPartitionArgs() []string {
	// first determine if all the partitions are collocated
	collocated := e.IsCollocated()

	args := []string{}
	partNum := 1

	// A device will use bluestore unless explicitly requested to be filestore (the default is blank)
	if collocated && e.StoreType != Filestore {
		// partitions are collocated, create the metadata partitions on the same device
		walDetails := e.Partitions[WalPartitionType]
		partArgs := getPartitionArgsFromDetails(partNum, WalPartitionType, e.ID, walDetails)
		args = append(args, partArgs...)
		partNum++

		dbDetails := e.Partitions[DatabasePartitionType]
		partArgs = getPartitionArgsFromDetails(partNum, DatabasePartitionType, e.ID, dbDetails)
		args = append(args, partArgs...)
		partNum++
	}

	dataPartitionType := e.GetDataPartitionType()

	// always create the data partition
	dataDetails := e.Partitions[dataPartitionType]
	dataPartArgs := getPartitionArgsFromDetails(partNum, dataPartitionType, e.ID, dataDetails)
	args = append(args, dataPartArgs...)

	// append args for the whole device
	args = append(args, []string{fmt.Sprintf("--disk-guid=%s", dataDetails.DiskUUID), "/dev/" + dataDetails.Device}...)

	return args
}

// determines if the given partition scheme entry is for a collocated OSD (all partitions on 1 device)
func (e *PerfSchemeEntry) IsCollocated() bool {
	collocated := true
	du := ""
	for _, details := range e.Partitions {
		if du == "" {
			du = details.DiskUUID
		} else if du != details.DiskUUID {
			// not all partitions are on the same disk, the partitions are not collocated
			collocated = false
			break
		}
	}

	return collocated
}

func (e *PerfSchemeEntry) GetDataPartitionType() PartitionType {
	if e.StoreType == Filestore {
		return FilestoreDataPartitionType
	} else {
		return BlockPartitionType
	}
}

// Get the arguments necessary to create an sgdisk partition with the given parameters.
// number is the partition number.
// The offset and length are in MB. Under the covers this is translated to sectors.
// If the length is -1, all remaining space will be assigned to the
func getPartitionArgs(number int, guid string, offset, length int, label string) []string {
	const sectorsPerMB = 2048
	var newPart string
	if length == UseRemainingSpace {
		// The partition gets the remainder of the device
		newPart = fmt.Sprintf("--largest-new=%d", number)
	} else {
		// The partition has a specific offset and length
		newPart = fmt.Sprintf("--new=%d:%d:+%d", number, offset*sectorsPerMB, length*sectorsPerMB)
	}

	return []string{
		newPart,
		fmt.Sprintf("--change-name=%d:%s", number, label),
		fmt.Sprintf("--partition-guid=%d:%s", number, guid),
	}
}

func getPartitionArgsFromDetails(number int, partType PartitionType, id int, details *PerfSchemePartitionDetails) []string {
	return getPartitionArgs(number, details.PartitionUUID, details.OffsetMB, details.SizeMB, getPartitionLabel(id, partType))
}

func getPartitionLabel(id int, partType PartitionType) string {
	switch partType {
	case WalPartitionType:
		return fmt.Sprintf("ROOK-OSD%d-WAL", id)
	case DatabasePartitionType:
		return fmt.Sprintf("ROOK-OSD%d-DB", id)
	case BlockPartitionType:
		return fmt.Sprintf("ROOK-OSD%d-BLOCK", id)
	case FilestoreDataPartitionType:
		return fmt.Sprintf("ROOK-OSD%d-FS-DATA", id)
	}

	return ""
}

func createBluestoreUUIDs() (*uuid.UUID, *uuid.UUID, *uuid.UUID, *uuid.UUID, error) {
	diskUUID, err := uuid.NewRandom()
	if err != nil {
		return nil, nil, nil, nil, errors.Wrapf(err, "failed to get disk uuid")
	}

	walUUID, err := uuid.NewRandom()
	if err != nil {
		return nil, nil, nil, nil, errors.Wrapf(err, "failed to get wal uuid")
	}

	dbUUID, err := uuid.NewRandom()
	if err != nil {
		return nil, nil, nil, nil, errors.Wrapf(err, "failed to get db uuid")
	}

	blockUUID, err := uuid.NewRandom()
	if err != nil {
		return nil, nil, nil, nil, errors.Wrapf(err, "failed to get block uuid")
	}

	return &diskUUID, &walUUID, &dbUUID, &blockUUID, nil
}

func createFilestoreUUIDs() (*uuid.UUID, *uuid.UUID, *uuid.UUID, error) {
	diskUUID, err := uuid.NewRandom()
	if err != nil {
		return nil, nil, nil, errors.Wrapf(err, "failed to get disk uuid")
	}

	dataUUID, err := uuid.NewRandom()
	if err != nil {
		return nil, nil, nil, errors.Wrapf(err, "failed to get data uuid")
	}

	journalUUID, err := uuid.NewRandom()
	if err != nil {
		return nil, nil, nil, errors.Wrapf(err, "failed to get journal uuid")
	}

	return &diskUUID, &dataUUID, &journalUUID, nil
}
