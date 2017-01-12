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
package partition

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"

	"github.com/google/uuid"
)

const (
	WalPartitionName      = "wal"
	DatabasePartitionName = "db"
	BlockPartitionName    = "block"
	walPartition          = 1
	databasePartition     = 2
	blockPartition        = 3
	UseRemainingSpace     = -1
	schemeFilename        = "partition-scheme"
	WalDefaultSizeMB      = 576
	DBDefaultSizeMB       = 20480
)

type BluestoreConfig struct {
	WalSizeMB      int
	DatabaseSizeMB int
}

// top level representation of an overall performance oriented partition scheme, with a dedicated metadata device
// and entries for all OSDs that define where their partitions live
type PerfScheme struct {
	Metadata *MetadataDeviceInfo `json:"metadata"`
	Entries  []*PerfSchemeEntry  `json:"entries"`
}

// represents an OSD and details about all of its partitions
type PerfSchemeEntry struct {
	ID         int                                    `json:"id"`
	OsdUUID    uuid.UUID                              `json:"osdUuid"`
	Partitions map[string]*PerfSchemePartitionDetails `json:"partitions"` // mapping of partition name to its details
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

// representsa specific partition on a metadata device, including details about which OSD it belongs to
type MetadataDevicePartition struct {
	ID            int       `json:"id"`
	OsdUUID       uuid.UUID `json:"osdUuid"`
	Name          string    `json:"name"`
	PartitionUUID string    `json:"partitionUuid"`
	SizeMB        int       `json:"sizeMB"`
	OffsetMB      int       `json:"offsetMB"`
}

func NewPerfScheme() *PerfScheme {
	return &PerfScheme{
		Entries: []*PerfSchemeEntry{},
	}
}

func NewPerfSchemeEntry() *PerfSchemeEntry {
	return &PerfSchemeEntry{
		Partitions: map[string]*PerfSchemePartitionDetails{}, // mapping of partition name (e.g. WAL) to it's details
	}
}

func NewMetadataDeviceInfo(device string) *MetadataDeviceInfo {
	return &MetadataDeviceInfo{Device: device, Partitions: []*MetadataDevicePartition{}}
}

// Load the persistent partition info from the config directory.
func LoadScheme(configDir string) (*PerfScheme, error) {
	filePath := path.Join(configDir, schemeFilename)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		// the scheme file doesn't exist yet, just return a new empty scheme with no error
		return NewPerfScheme(), nil
	}

	b, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var scheme PerfScheme
	err = json.Unmarshal(b, &scheme)
	if err != nil {
		return nil, err
	}

	return &scheme, nil
}

// Save the partition scheme to the config dir
func (s *PerfScheme) Save(configDir string) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(path.Join(configDir, schemeFilename), b, 0644)
	if err != nil {
		return err
	}

	return nil
}

// populates a partition scheme entry for an OSD where all its partitions are collocated on a single device
func PopulateCollocatedPerfSchemeEntry(entry *PerfSchemeEntry, device string, bluestoreConfig BluestoreConfig) error {
	diskUUID, walUUID, dbUUID, blockUUID, err := createUUIDs()
	if err != nil {
		return err
	}

	walSize := WalDefaultSizeMB
	if bluestoreConfig.WalSizeMB > 0 {
		walSize = bluestoreConfig.WalSizeMB
	}
	dbSize := DBDefaultSizeMB
	if bluestoreConfig.DatabaseSizeMB > 0 {
		dbSize = bluestoreConfig.DatabaseSizeMB
	}

	offset := 1

	// layout the partitions for WAL, DB, and Block
	entry.Partitions[WalPartitionName] = &PerfSchemePartitionDetails{
		Device:        device,
		DiskUUID:      diskUUID.String(),
		PartitionUUID: walUUID.String(),
		SizeMB:        walSize,
		OffsetMB:      offset,
	}
	offset += entry.Partitions[WalPartitionName].SizeMB

	entry.Partitions[DatabasePartitionName] = &PerfSchemePartitionDetails{
		Device:        device,
		DiskUUID:      diskUUID.String(),
		PartitionUUID: dbUUID.String(),
		SizeMB:        dbSize,
		OffsetMB:      offset,
	}
	offset += entry.Partitions[DatabasePartitionName].SizeMB

	entry.Partitions[BlockPartitionName] = &PerfSchemePartitionDetails{
		Device:        device,
		DiskUUID:      diskUUID.String(),
		PartitionUUID: blockUUID.String(),
		SizeMB:        UseRemainingSpace,
		OffsetMB:      offset,
	}

	return nil
}

// populates a partition scheme entry for an OSD that will have distributed partitions: its metadata will live on a
// dedicated metadata device and its block data will live on a dedicated device
func PopulateDistributedPerfSchemeEntry(entry *PerfSchemeEntry, device string, metadataInfo *MetadataDeviceInfo,
	bluestoreConfig BluestoreConfig) error {

	diskUUID, walUUID, dbUUID, blockUUID, err := createUUIDs()
	if err != nil {
		return err
	}

	// the block partition will take up the entire given device
	entry.Partitions[BlockPartitionName] = &PerfSchemePartitionDetails{
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
			return fmt.Errorf("failed to get metadata disk uuid. %+v", err)
		}
		metadataInfo.DiskUUID = u.String()
	} else {
		lastEntry := metadataInfo.Partitions[numMetadataParts-1]
		offset = lastEntry.OffsetMB + lastEntry.SizeMB
	}

	walSize := WalDefaultSizeMB
	if bluestoreConfig.WalSizeMB > 0 {
		walSize = bluestoreConfig.WalSizeMB
	}
	dbSize := DBDefaultSizeMB
	if bluestoreConfig.DatabaseSizeMB > 0 {
		dbSize = bluestoreConfig.DatabaseSizeMB
	}

	// record information about the WAL partition
	entry.Partitions[WalPartitionName] = &PerfSchemePartitionDetails{
		Device:        metadataInfo.Device,
		DiskUUID:      metadataInfo.DiskUUID,
		PartitionUUID: walUUID.String(),
		SizeMB:        walSize,
		OffsetMB:      offset,
	}
	walPartitionInfo := &MetadataDevicePartition{
		ID:            entry.ID,
		OsdUUID:       entry.OsdUUID,
		Name:          WalPartitionName,
		PartitionUUID: walUUID.String(),
		SizeMB:        walSize,
		OffsetMB:      offset,
	}
	metadataInfo.Partitions = append(metadataInfo.Partitions, walPartitionInfo)
	offset += entry.Partitions[WalPartitionName].SizeMB

	// record information about the DB partition
	entry.Partitions[DatabasePartitionName] = &PerfSchemePartitionDetails{
		Device:        metadataInfo.Device,
		DiskUUID:      metadataInfo.DiskUUID,
		PartitionUUID: dbUUID.String(),
		SizeMB:        dbSize,
		OffsetMB:      offset,
	}
	dbPartitionInfo := &MetadataDevicePartition{
		ID:            entry.ID,
		OsdUUID:       entry.OsdUUID,
		Name:          DatabasePartitionName,
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
		partArgs := getPartitionArgs(partNum+1, part.PartitionUUID, part.OffsetMB, part.SizeMB, getPartitionLabel(part.ID, part.Name))
		args = append(args, partArgs...)
	}

	// append args for the whole device
	args = append(args, []string{fmt.Sprintf("--disk-guid=%s", m.DiskUUID), "/dev/" + m.Device}...)

	return args
}

func (e *PerfSchemeEntry) GetPartitionArgs() []string {
	// first determine if all the partitions are collocated
	collocated := e.IsCollocated()

	args := []string{}
	partNum := 1

	if collocated {
		// partitions are collocated, create the metadata partitions on the same device
		walDetails := e.Partitions[WalPartitionName]
		partArgs := getPartitionArgsFromDetails(partNum, WalPartitionName, e.ID, walDetails)
		args = append(args, partArgs...)
		partNum++

		dbDetails := e.Partitions[DatabasePartitionName]
		partArgs = getPartitionArgsFromDetails(partNum, DatabasePartitionName, e.ID, dbDetails)
		args = append(args, partArgs...)
		partNum++
	}

	// always create the data partition
	blockDetails := e.Partitions[BlockPartitionName]
	partArgs := getPartitionArgsFromDetails(partNum, BlockPartitionName, e.ID, blockDetails)
	args = append(args, partArgs...)

	// append args for the whole device
	args = append(args, []string{fmt.Sprintf("--disk-guid=%s", blockDetails.DiskUUID), "/dev/" + blockDetails.Device}...)

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

// Get the arguments necessary to create an sgdisk partition with the given parameters.
// number is the partition number.
// The offset and length are in MB. Under the covers this is translated to sectors.
// If the length is -1, all remaining space will be assigned to the partition.
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

func getPartitionArgsFromDetails(number int, name string, id int, details *PerfSchemePartitionDetails) []string {
	return getPartitionArgs(number, details.PartitionUUID, details.OffsetMB, details.SizeMB, getPartitionLabel(id, name))
}

func getPartitionLabel(id int, partName string) string {
	switch partName {
	case WalPartitionName:
		return fmt.Sprintf("ROOK-OSD%d-WAL", id)
	case DatabasePartitionName:
		return fmt.Sprintf("ROOK-OSD%d-DB", id)
	case BlockPartitionName:
		return fmt.Sprintf("ROOK-OSD%d-BLOCK", id)
	}

	return ""
}

func createUUIDs() (*uuid.UUID, *uuid.UUID, *uuid.UUID, *uuid.UUID, error) {
	diskUUID, err := uuid.NewRandom()
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to get disk uuid. %+v", err)
	}

	walUUID, err := uuid.NewRandom()
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to get wal uuid. %+v", err)
	}

	dbUUID, err := uuid.NewRandom()
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to get db uuid. %+v", err)
	}

	blockUUID, err := uuid.NewRandom()
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to get block uuid. %+v", err)
	}

	return &diskUUID, &walUUID, &dbUUID, &blockUUID, nil
}
