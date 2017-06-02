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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"

	"github.com/rook/rook/pkg/ceph/client"
	"github.com/rook/rook/pkg/ceph/mon"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/clusterd/inventory"
	"github.com/rook/rook/pkg/util"
	"github.com/rook/rook/pkg/util/exec"
	"github.com/rook/rook/pkg/util/sys"
)

const (
	DevicesValue                = "devices"
	ForceFormatValue            = "forceFormat"
	diskByPartUUID              = "/dev/disk/by-partuuid"
	cephOsdKey                  = mon.CephKey + "/osd"
	desiredOsdRootKey           = cephOsdKey + "/" + clusterd.DesiredKey + "/%s"
	deviceDesiredKey            = desiredOsdRootKey + "/device"
	dirDesiredKey               = desiredOsdRootKey + "/dir"
	bootstrapOSDKeyringTemplate = `
[client.bootstrap-osd]
	key = %s
	caps mon = "allow profile bootstrap-osd"
`
)

type osdConfig struct {
	configRoot      string
	rootPath        string
	id              int
	uuid            uuid.UUID
	dir             bool
	partitionScheme *PerfSchemeEntry
}

type Device struct {
	Name   string `json:"name"`
	NodeID string `json:"nodeId"`
	Dir    bool   `json:"bool"`
}

type DeviceOsdMapping struct {
	Entries map[string]*DeviceOsdIDEntry // device name to OSD ID mapping entry
}

func NewDeviceOsdMapping() *DeviceOsdMapping {
	return &DeviceOsdMapping{
		Entries: map[string]*DeviceOsdIDEntry{},
	}
}

type DeviceOsdIDEntry struct {
	Data     int   // OSD ID that has data stored here
	Metadata []int // OSD IDs (multiple) that have metadata stored here
}

// format the given device for usage by an OSD
func formatDevice(context *clusterd.Context, config *osdConfig, forceFormat bool, storeConfig StoreConfig) error {
	dataDetails, err := getDataPartitionDetails(config)
	if err != nil {
		return err
	}

	// check if partitions belong to rook
	ownPartitions, devFS, err := checkIfDeviceAvailable(context.Executor, dataDetails.Device)
	if err != nil {
		return fmt.Errorf("failed to format device. %+v", err)
	}

	if !ownPartitions {
		if forceFormat {
			logger.Warningf("device %s is being formatted, but has partitions!!", dataDetails.Device)
		} else {
			logger.Warningf("device %s has partitions that will not be formatted. Skipping device.", dataDetails.Device)
		}
	}

	if devFS != "" {
		if forceFormat {
			// there's a filesystem on the device, but the user has specified to force a format. give a warning about that.
			logger.Warningf("device %s already formatted with %s, but forcing a format!!!", dataDetails.Device, devFS)
		} else {
			// disk is already formatted and the user doesn't want to force it, but we require partitioning
			return fmt.Errorf("device %s already formatted with %s", dataDetails.Device, devFS)
		}
	}

	// format the device
	dangerousToFormat := !ownPartitions || devFS != ""
	if !dangerousToFormat || forceFormat {
		err := partitionOSD(context, config)
		if err != nil {
			return fmt.Errorf("failed to partion device %s. %v", dataDetails.Device, err)
		}
	}

	return nil
}

func checkIfDeviceAvailable(executor exec.Executor, name string) (bool, string, error) {
	ownPartitions := true
	partitions, _, err := sys.GetDevicePartitions(name, executor)
	if err != nil {
		return false, "", fmt.Errorf("failed to get %s partitions. %+v", name, err)
	}
	if !rookOwnsPartitions(partitions) {
		ownPartitions = false
	}

	// check if there is a file system on the device
	devFS, err := sys.GetDeviceFilesystems(name, executor)
	if err != nil {
		return false, "", fmt.Errorf("failed to get device %s filesystem: %+v", name, err)
	}

	return ownPartitions, devFS, nil
}

func rookOwnsPartitions(partitions []*sys.Partition) bool {

	// if there are partitions, they must all have the rook osd label
	for _, p := range partitions {
		if !strings.HasPrefix(p.Label, "ROOK-OSD") {
			return false
		}
	}

	// if there are no partitions, or the partitions are all from rook OSDs, then rook owns the device
	return true
}

// partitions a given device exclusively for metadata usage
func partitionMetadata(context *clusterd.Context, info *MetadataDeviceInfo, configRoot string) error {
	if len(info.Partitions) == 0 {
		return nil
	}

	// check to see if the metadata partition scheme has already been applied
	savedScheme, err := LoadScheme(configRoot)
	if err != nil {
		return fmt.Errorf("failed to load the saved partition scheme from %s: %+v", configRoot, err)
	}

	if savedScheme.Metadata != nil && len(savedScheme.Metadata.Partitions) > 0 {
		// TODO: there is already an existing metadata partition scheme that has been applied, we should be able to add to it
		// https://github.com/rook/rook/issues/341
		if info.DiskUUID == savedScheme.Metadata.DiskUUID {
			// the existing metadata partition scheme is the same disk as the desired metadata device.  no work to perform.
			return nil
		}
		return fmt.Errorf("metadata partition scheme already exists on %s (%s), cannot use desired metadata device %s (%s)",
			savedScheme.Metadata.Device, savedScheme.Metadata.DiskUUID, info.Device, info.DiskUUID)
	}

	// zap/clear all existing partitions
	err = sys.RemovePartitions(info.Device, context.Executor)
	if err != nil {
		return fmt.Errorf("failed to zap partitions on metadata device /dev/%s: %+v", info.Device, err)
	}

	// create the partitions
	err = sys.CreatePartitions(info.Device, info.GetPartitionArgs(), context.Executor)
	if err != nil {
		return fmt.Errorf("failed to partition metadata device /dev/%s. %+v", info.Device, err)
	}

	// save the metadata partition info to disk now that it has been committed
	savedScheme.Metadata = info
	if err := savedScheme.Save(configRoot); err != nil {
		return fmt.Errorf("failed to save partition scheme to %s: %+v", configRoot, err)
	}

	// associate the OSD IDs with the metadata device in etcd
	if context.EtcdClient != nil {
		idSet := util.NewSet()
		for _, part := range info.Partitions {
			idSet.Add(strconv.Itoa(part.ID))
		}
		if err := associateOSDIDsWithMetadataDevice(context.EtcdClient, context.NodeID, info.DiskUUID, strings.Join(idSet.ToSlice(), ",")); err != nil {
			return fmt.Errorf("failed to associate osd ids '%+v' with metadata device %s (%s): %+v", idSet, info.Device, info.DiskUUID, err)
		}
	}

	return nil
}

// Partitions a device for use by a osd.
// If there are any partitions or formatting already on the device, it will be wiped.
func partitionOSD(context *clusterd.Context, config *osdConfig) error {
	dataDetails, err := getDataPartitionDetails(config)
	if err != nil {
		return err
	}

	// zap/clear all existing partitions on the device
	err = sys.RemovePartitions(dataDetails.Device, context.Executor)
	if err != nil {
		return fmt.Errorf("failed to zap partitions on metadata device /dev/%s: %+v", dataDetails.Device, err)
	}

	// create the partitions on the device
	err = sys.CreatePartitions(dataDetails.Device, config.partitionScheme.GetPartitionArgs(), context.Executor)
	if err != nil {
		return fmt.Errorf("failed to partition /dev/%s. %+v", dataDetails.Device, err)
	}

	if config.partitionScheme.StoreType == Filestore {
		// the OSD is using filestore, create a filesystem for the device and mount it under config root
		dataPartDetails := config.partitionScheme.Partitions[FilestoreDataPartitionType]
		dataPartPath := filepath.Join(diskByPartUUID, dataPartDetails.PartitionUUID)
		logger.Infof("waiting for partition path %s", dataPartPath)
		err = waitForPath(dataPartPath, context.Executor)
		if err != nil {
			return fmt.Errorf("failed waiting for %s: %+v", dataPartPath, err)
		}
		err = sys.FormatDevice(dataPartPath, context.Executor)
		if err != nil {
			logger.Warningf("first attempt to format partition %s on device %s failed.  Waiting 2 seconds then retrying: %+v",
				dataPartDetails.PartitionUUID, dataPartDetails.Device, err)
			<-time.After(2 * time.Second)
			err = sys.FormatDevice(dataPartPath, context.Executor)
			if err != nil {
				return fmt.Errorf("failed to format partition %s on device %s. %+v", dataPartDetails.PartitionUUID, dataPartDetails.Device, err)
			}
		}
		err = sys.MountDevice(dataPartPath, config.rootPath, context.Executor)
		if err != nil {
			return fmt.Errorf("failed to mount %s at %s: %+v", dataPartPath, config.configRoot, context.Executor)
		}
	}

	// save the partition scheme entry to disk now that it has been committed
	savedScheme, err := LoadScheme(config.configRoot)
	if err != nil {
		return fmt.Errorf("failed to load the saved partition scheme from %s: %+v", config.configRoot, err)
	}
	savedScheme.Entries = append(savedScheme.Entries, config.partitionScheme)
	if err := savedScheme.Save(config.configRoot); err != nil {
		return fmt.Errorf("failed to save partition scheme to %s: %+v", config.configRoot, err)
	}

	// update the uuid of the disk in the inventory in memory
	logger.Debugf("Updating disk uuid %s on device %s", dataDetails.DiskUUID, dataDetails.Device)
	for _, disk := range context.Inventory.Local.Disks {
		if disk.Name == dataDetails.Device {
			logger.Debugf("Updated uuid on device %s", dataDetails.Device)
			disk.UUID = dataDetails.DiskUUID
		}
	}

	// save the desired state of the osd for this device
	if context.EtcdClient != nil {
		err = associateOsdIDWithDevice(context.EtcdClient, context.NodeID, dataDetails.DiskUUID, config.id, false)
		if err != nil {
			return fmt.Errorf("failed to associate osd id %d with device %s (%s)", config.id, dataDetails.Device, dataDetails.DiskUUID)
		}
		if config.partitionScheme.IsCollocated() {
			// the metadata is on the same disk as the data, associate the osd ID with the device for metadata too
			err = associateOSDIDsWithMetadataDevice(
				context.EtcdClient, context.NodeID, dataDetails.DiskUUID, fmt.Sprintf("%d", config.id))
			if err != nil {
				return fmt.Errorf("failed to associate osd id %d with device %s (%s) for metadata",
					config.id, dataDetails.Device, dataDetails.DiskUUID)
			}
		}
	}

	return nil
}

func getDataPartitionDetails(config *osdConfig) (*PerfSchemePartitionDetails, error) {
	if config.partitionScheme == nil {
		return nil, fmt.Errorf("partition scheme missing from %+v", config)
	}

	dataPartitionType := config.partitionScheme.getDataPartitionType()

	dataDetails, ok := config.partitionScheme.Partitions[dataPartitionType]
	if !ok || dataDetails == nil {
		return nil, fmt.Errorf("data partition missing from %+v", config.partitionScheme)
	}

	return dataDetails, nil
}

func getMetadataPartitionDetails(config *osdConfig) (*PerfSchemePartitionDetails, error) {
	if config.partitionScheme == nil {
		return nil, fmt.Errorf("partition scheme missing from %+v", config)
	}

	metadataPartitionType := config.partitionScheme.getMetadataPartitionType()

	if config.partitionScheme.StoreType == Filestore {
		// TODO: support separate metadata device for filestore (just use the data partition details for now)
		return getDataPartitionDetails(config)
	}

	metadataDetails, ok := config.partitionScheme.Partitions[metadataPartitionType]
	if !ok || metadataDetails == nil {
		return nil, fmt.Errorf("metadata partition missing from %+v", config.partitionScheme)
	}

	return metadataDetails, nil
}

func getDiskSize(context *clusterd.Context, name string) (uint64, error) {
	for _, device := range context.Inventory.Local.Disks {
		if device.Name == name {
			return device.Size, nil
		}
	}

	return 0, fmt.Errorf("device %s not found", name)
}

func waitForPath(path string, executor exec.Executor) error {
	retryCount := 0
	retryMax := 25
	sleepTime := 250
	for {
		_, err := executor.ExecuteStat(path)
		if err == nil {
			return nil
		}

		retryCount++
		if retryCount > retryMax {
			return err
		}
		<-time.After(time.Duration(sleepTime) * time.Millisecond)
	}
}

func registerOSD(context *clusterd.Context, clusterName string) (*int, *uuid.UUID, error) {
	var err error
	osdUUID, err := uuid.NewRandom()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate UUID for osd: %+v", err)
	}

	// create the OSD instance via a mon_command, this assigns a cluster wide ID to the OSD
	osdID, err := createOSD(context, clusterName, osdUUID)
	if err != nil {
		return nil, nil, err
	}

	logger.Infof("successfully created OSD %s with ID %d", osdUUID.String(), osdID)
	return &osdID, &osdUUID, nil
}

func getStoreSettings(config *osdConfig, storeConfig StoreConfig) (map[string]string, error) {
	settings := map[string]string{}
	if config.dir || (config.partitionScheme != nil && config.partitionScheme.StoreType == Filestore) {
		// add additional filestore settings for dirs and filestore devices
		journalSize := JournalDefaultSizeMB
		if storeConfig.JournalSizeMB > 0 {
			journalSize = storeConfig.JournalSizeMB
		}
		settings["osd journal size"] = strconv.Itoa(journalSize)
		return settings, nil
	}

	if config.partitionScheme == nil || config.partitionScheme.Partitions == nil {
		return nil, fmt.Errorf("failed to find partitions from config for osd %d", config.id)
	}

	// add additional bluestore settings
	parts := config.partitionScheme.Partitions
	walPartition, ok := parts[WalPartitionType]
	if !ok {
		return nil, fmt.Errorf("failed to find wal partition for osd %d", config.id)
	}
	dbPartition, ok := parts[DatabasePartitionType]
	if !ok {
		return nil, fmt.Errorf("failed to find db partition for osd %d", config.id)
	}
	blockPartition, ok := parts[BlockPartitionType]
	if !ok {
		return nil, fmt.Errorf("failed to find block partition for osd %d", config.id)
	}

	prefix := diskByPartUUID
	settings["bluestore block wal path"] = path.Join(prefix, walPartition.PartitionUUID)
	settings["bluestore block db path"] = path.Join(prefix, dbPartition.PartitionUUID)
	settings["bluestore block path"] = path.Join(prefix, blockPartition.PartitionUUID)

	return settings, nil
}

func writeConfigFile(config *osdConfig, context *clusterd.Context, cluster *mon.ClusterInfo, storeConfig StoreConfig) error {
	cephConfig := mon.CreateDefaultCephConfig(context, cluster, config.rootPath, isBluestore(config))

	if !isBluestore(config) {
		// using the local file system requires some config overrides
		// http://docs.ceph.com/docs/jewel/rados/configuration/filesystem-recommendations/#not-recommended
		cephConfig.GlobalConfig.OsdMaxObjectNameLen = 256
		cephConfig.GlobalConfig.OsdMaxObjectNamespaceLen = 64
	}

	// bluestore has some extra settings
	settings, err := getStoreSettings(config, storeConfig)
	if err != nil {
		return fmt.Errorf("failed to read store settings. %+v", err)
	}

	// write the OSD config file to disk
	_, err = mon.GenerateConfigFile(context, cluster, config.rootPath, fmt.Sprintf("osd.%d", config.id),
		getOSDKeyringPath(config.rootPath), isBluestore(config), cephConfig, settings)
	if err != nil {
		return fmt.Errorf("failed to write OSD %d config file: %+v", config.id, err)
	}

	return nil
}

func initializeOSD(config *osdConfig, context *clusterd.Context,
	cluster *mon.ClusterInfo, location string, storeConfig StoreConfig) error {
	err := writeConfigFile(config, context, cluster, storeConfig)
	if err != nil {
		return fmt.Errorf("failed to write config file: %+v", err)
	}

	// get the current monmap, it will be needed for creating the OSD file system
	monMapRaw, err := getMonMap(context, cluster.Name)
	if err != nil {
		return fmt.Errorf("failed to get mon map: %+v", err)
	}

	// create/initalize the OSD file system and journal
	if err := createOSDFileSystem(context, cluster.Name, config, monMapRaw); err != nil {
		return err
	}

	// add auth privileges for the OSD, the bootstrap-osd privileges were very limited
	if err := addOSDAuth(context, cluster.Name, config.id, config.rootPath); err != nil {
		return err
	}

	// add the new OSD to the cluster crush map
	if err := addOSDToCrushMap(context, config, cluster.Name, location); err != nil {
		return err
	}

	return nil
}

// creates the OSD identity in the cluster via a mon_command
func createOSD(context *clusterd.Context, clusterName string, osdUUID uuid.UUID) (int, error) {
	// TODO: "entity": "client.bootstrap-osd",
	args := []string{"osd", "create", osdUUID.String()}
	buf, err := client.ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return 0, fmt.Errorf("failed to create osd %s: %+v", osdUUID, err)
	}

	var resp map[string]interface{}
	err = json.Unmarshal(buf, &resp)
	if err != nil {
		return 0, fmt.Errorf("failed to unmarshall response: %+v.  raw response: '%s'", err, string(buf[:]))
	}

	return int(resp["osdid"].(float64)), nil
}

// gets the current mon map for the cluster
func getMonMap(context *clusterd.Context, clusterName string) ([]byte, error) {
	// TODO: "entity": "client.bootstrap-osd",
	args := []string{"mon", "getmap"}
	buf, err := client.ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return nil, fmt.Errorf("failed to get mon map: %+v", err)
	}
	return buf, nil
}

// creates/initalizes the OSD filesystem and journal via a child process
func createOSDFileSystem(context *clusterd.Context, clusterName string, config *osdConfig, monMap []byte) error {
	logger.Infof("Initializing OSD %d file system at %s...", config.id, config.rootPath)

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
		"--mkkey",
		fmt.Sprintf("--id=%d", config.id),
		fmt.Sprintf("--cluster=%s", clusterName),
		fmt.Sprintf("--conf=%s", mon.GetConfFilePath(config.rootPath, clusterName)),
		fmt.Sprintf("--osd-data=%s", config.rootPath),
		fmt.Sprintf("--osd-uuid=%s", config.uuid.String()),
		fmt.Sprintf("--monmap=%s", monMapTmpPath),
	}

	if !isBluestore(config) {
		options = append(options, fmt.Sprintf("--osd-journal=%s", getOSDJournalPath(config.rootPath)))
		options = append(options, fmt.Sprintf("--keyring=%s", getOSDKeyringPath(config.rootPath)))
	}

	// create the OSD file system and journal
	err := context.ProcMan.Run(
		fmt.Sprintf("mkfs-osd%d", config.id),
		"ceph-osd",
		options...)

	if err != nil {
		return fmt.Errorf("failed osd mkfs for OSD ID %d, UUID %s, dataDir %s: %+v",
			config.id, config.uuid.String(), config.rootPath, err)
	}

	return nil
}

// add OSD auth privileges for the given OSD ID.  the bootstrap-osd privileges are limited and a real OSD needs more.
func addOSDAuth(context *clusterd.Context, clusterName string, osdID int, osdDataPath string) error {
	// create a new auth for this OSD
	osdEntity := fmt.Sprintf("osd.%d", osdID)
	args := []string{"auth", "add", osdEntity}
	capabilities := []string{"osd", "allow *", "mon", "allow profile osd"}
	args = append(args, capabilities...)
	_, err := client.ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return fmt.Errorf("command marshall failed: %+v", err)
	}

	/* not implemented
	_, info, err := bootstrapConn.MonCommandWithInputBuffer(command, keyringBuffer)
	if err != nil {
		return fmt.Errorf("mon_command failed: %+v", err)
	}
	logger.Debugf("succeeded command for %s. info: %s", osdEntity, info)
	*/

	return nil
}

// adds the given OSD to the crush map
func addOSDToCrushMap(context *clusterd.Context, config *osdConfig, clusterName, location string) error {
	osdID := config.id
	osdDataPath := config.rootPath

	var totalBytes uint64
	if !isBluestore(config) {
		// get the size of the volume containing the OSD data dir.  For filestore directory or device, this will be a
		// mounted filesystem, so we can use Statfs
		s := syscall.Statfs_t{}
		if err := syscall.Statfs(osdDataPath, &s); err != nil {
			return fmt.Errorf("failed to statfs on %s, %+v", osdDataPath, err)
		}
		totalBytes = s.Blocks * uint64(s.Bsize)
	} else {
		// for bluestore, the data partition will be raw, so we can't use Statfs.  Get the full device properties
		// of the data partition and then get the size from that.
		dataPartDetails, err := getDataPartitionDetails(config)
		if err != nil {
			return fmt.Errorf("failed to get data partition details for osd %d (%s): %+v", osdID, osdDataPath, err)
		}
		dataPartPath := filepath.Join(diskByPartUUID, dataPartDetails.PartitionUUID)
		devProps, err := sys.GetDevicePropertiesFromPath(dataPartPath, context.Executor)
		if err != nil {
			return fmt.Errorf("failed to get device properties for %s: %+v", dataPartPath, err)
		}
		if val, ok := devProps["SIZE"]; ok {
			if size, err := strconv.ParseUint(val, 10, 64); err == nil {
				totalBytes = size
			}
		}

		if totalBytes == 0 {
			return fmt.Errorf("failed to get size of %s: %+v.  Full properties: %+v", dataPartPath, err, devProps)
		}
	}

	// weight is ratio of (size in KB) / (1 GB)
	weight := float64(totalBytes/1024) / 1073741824.0
	weight, _ = strconv.ParseFloat(fmt.Sprintf("%.4f", weight), 64)

	osdEntity := fmt.Sprintf("osd.%d", osdID)
	locArgs, err := formatLocation(location)
	if err != nil {
		return err
	}

	logger.Infof("adding %s (%s), bytes: %d, weight: %.4f, to crush map at '%s'",
		osdEntity, osdDataPath, totalBytes, weight, strings.Join(locArgs, " "))
	args := []string{"osd", "crush", "create-or-move", strconv.Itoa(osdID), fmt.Sprintf("%.4f", weight)}
	args = append(args, locArgs...)
	_, err = client.ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return fmt.Errorf("failed adding %s to crush map: %+v", osdEntity, err)
	}

	if context.EtcdClient != nil {
		if err := inventory.SetLocation(context.EtcdClient, context.NodeID, strings.Join(locArgs, ",")); err != nil {
			return fmt.Errorf("failed to save CRUSH location for OSD %s: %+v", osdEntity, err)
		}
	}

	return nil
}

func markOSDOut(context *clusterd.Context, clusterName string, id int) error {
	args := []string{"osd", "out", strconv.Itoa(id)}
	_, err := client.ExecuteCephCommand(context, clusterName, args)
	return err
}

func purgeOSD(context *clusterd.Context, clusterName string, id int) error {
	// ceph osd crush remove <name>
	args := []string{"osd", "crush", "remove", fmt.Sprintf("osd.%d", id)}
	_, err := client.ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return fmt.Errorf("failed to remove osd %d from crush map. %v", id, err)
	}

	// ceph auth del osd.$osd_num
	err = client.AuthDelete(context, clusterName, fmt.Sprintf("osd.%d", id))
	if err != nil {
		return err
	}

	// ceph osd rm $osd_num
	args = []string{"osd", "rm", strconv.Itoa(id)}
	_, err = client.ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return fmt.Errorf("failed to rm osd %d. %v", id, err)
	}
	return nil
}
