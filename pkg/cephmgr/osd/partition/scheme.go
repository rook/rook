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
	SimpleVersion         = 1
	schemeFilename        = "partition-scheme"
)

type Scheme struct {
	Version        int               `json:"version"`
	SizeMB         int               `json:"sizeMb"`
	DiskUUID       string            `json:"diskUuid"`
	PartitionUUIDs map[string]string `json:"partitionUuids"`
}

// This is a simple scheme to create the wal and db each of size 10% of the disk,
// with the remainder of the disk being allocated for the raw data.
func GetSimpleScheme(sizeMB int) (*Scheme, error) {
	diskUUID, err := uuid.NewRandom()
	if err != nil {
		return nil, fmt.Errorf("failed to get disk uuid. %+v", err)
	}
	walUUID, err := uuid.NewRandom()
	if err != nil {
		return nil, fmt.Errorf("failed to get wal uuid. %+v", err)
	}
	dbUUID, err := uuid.NewRandom()
	if err != nil {
		return nil, fmt.Errorf("failed to get db uuid. %+v", err)
	}
	blockUUID, err := uuid.NewRandom()
	if err != nil {
		return nil, fmt.Errorf("failed to get block uuid. %+v", err)
	}

	uuids := map[string]string{
		WalPartitionName:      walUUID.String(),
		DatabasePartitionName: dbUUID.String(),
		BlockPartitionName:    blockUUID.String(),
	}

	return &Scheme{
		Version:        SimpleVersion,
		SizeMB:         sizeMB,
		DiskUUID:       diskUUID.String(),
		PartitionUUIDs: uuids,
	}, nil
}

// Turn the scheme into arguments for sgdisk. The result will be three partitions:
// 1. WAL: Write ahead log (10%)
// 2. DB: The bluestore database (10%)
// 3. Block: The raw storage for the data being written to the OSD. (80%)
//     Uses the remainder of the storage space after small partitions for the wal and db.
func (s *Scheme) GetArgs(name string) []string {
	offset := 1
	tenPercent := s.SizeMB / 10

	// append args for individual partitions
	args := s.getPartitionArgs(WalPartitionName, walPartition, offset, tenPercent, fmt.Sprintf("osd-wal-%s", s.DiskUUID))
	offset += tenPercent
	args = append(args, s.getPartitionArgs(DatabasePartitionName, databasePartition, offset, tenPercent, fmt.Sprintf("osd-db-%s", s.DiskUUID))...)
	offset += tenPercent
	args = append(args, s.getPartitionArgs(BlockPartitionName, blockPartition, offset, UseRemainingSpace, fmt.Sprintf("osd-block-%s", s.DiskUUID))...)

	// append args for the whole device
	args = append(args, fmt.Sprintf("--disk-guid=%s", s.DiskUUID))
	args = append(args, "/dev/"+name)

	return args
}

// Save the partition scheme to the config dir
func (s *Scheme) Save(configDir string) error {
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

// Load the persistent partition info from the config directory.
func LoadScheme(configDir string) (*Scheme, error) {
	b, err := ioutil.ReadFile(path.Join(configDir, schemeFilename))
	if err != nil {
		return nil, err
	}

	var scheme Scheme
	err = json.Unmarshal(b, &scheme)
	if err != nil {
		return nil, err
	}

	return &scheme, nil
}

// Get the arguments necessary to create an sgdisk partition with the given parameters.
// The id is the partition number.
// The offset and length are in MB. Under the covers this is translated to sectors.
// If the length is -1, all remaining space will be assigned to the partition.
func (s *Scheme) getPartitionArgs(name string, id int, offset, length int, label string) []string {
	const sectorsPerMB = 2048
	var newPart string
	if length == UseRemainingSpace {
		// The partition gets the remainder of the device
		newPart = fmt.Sprintf("--largest-new=%d", id)
	} else {
		// The partition has a specific offset and length
		newPart = fmt.Sprintf("--new=%d:%d:+%d", id, offset*sectorsPerMB, length*sectorsPerMB)
	}

	guid, ok := s.PartitionUUIDs[name]
	if !ok {
		logger.Warningf("could not find uuid for partition %s", name)
	}

	return []string{
		newPart,
		fmt.Sprintf("--change-name=%d:%s", id, label),
		fmt.Sprintf("--partition-guid=%d:%s", id, guid),
	}
}
