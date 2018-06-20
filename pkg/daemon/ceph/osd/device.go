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
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	rookalpha "github.com/rook/rook/pkg/apis/rook.io/v1alpha1"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/daemon/ceph/mon"
	"github.com/rook/rook/pkg/operator/cluster/ceph/osd/config"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util/display"
	"github.com/rook/rook/pkg/util/exec"
	"github.com/rook/rook/pkg/util/sys"
)

const (
	diskByPartUUID              = "/dev/disk/by-partuuid"
	bootstrapOSDKeyringTemplate = `
[client.bootstrap-osd]
	key = %s
	caps mon = "allow profile bootstrap-osd"
`
	// ratio of disk space that will be used by bluestore on a dir.  This is an upper bound and it
	// is not preallocated (it is thinly provisioned).
	bluestoreDirBlockSizeRatio = 1.0
)

type osdConfig struct {
	// the root for all local config (e.g., /var/lib/rook)
	configRoot string
	// the root directory for this OSD (e.g., /var/lib/rook/osd0)
	rootPath        string
	id              int
	uuid            uuid.UUID
	dir             bool
	storeConfig     rookalpha.StoreConfig
	partitionScheme *config.PerfSchemeEntry
	kv              *k8sutil.ConfigMapKVStore
	storeName       string
}

type Device struct {
	Name   string `json:"name"`
	NodeID string `json:"nodeId"`
	Dir    bool   `json:"bool"`
}

type DeviceOsdMapping struct {
	Entries map[string]*DeviceOsdIDEntry // device name to OSD ID mapping entry
}

type DeviceOsdIDEntry struct {
	Data     int   // OSD ID that has data stored here
	Metadata []int // OSD IDs (multiple) that have metadata stored here
}

func (m *DeviceOsdMapping) String() string {
	b, _ := json.Marshal(m)
	return string(b)
}

// format the given device for usage by an OSD
func formatDevice(context *clusterd.Context, config *osdConfig, forceFormat bool, storeConfig rookalpha.StoreConfig) error {
	dataDetails, err := getDataPartitionDetails(config)
	if err != nil {
		return err
	}

	// check if partitions belong to rook
	ownPartitions, devFS, err := sys.CheckIfDeviceAvailable(context.Executor, dataDetails.Device)
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

// partitions a given device exclusively for metadata usage
func partitionMetadata(context *clusterd.Context, info *config.MetadataDeviceInfo, kv *k8sutil.ConfigMapKVStore, storeName string) error {
	if len(info.Partitions) == 0 {
		return nil
	}

	// check to see if the metadata partition scheme has already been applied
	savedScheme, err := config.LoadScheme(kv, storeName)
	if err != nil {
		return fmt.Errorf("failed to load the saved partition scheme: %+v", err)
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

	// check one last time to make sure it's OK for us to format this metadata device
	ownPartitions, fs, err := sys.CheckIfDeviceAvailable(context.Executor, info.Device)
	if err != nil {
		return fmt.Errorf("failed to get metadata device %s info: %+v", info.Device, err)
	} else if fs != "" || !ownPartitions {
		return fmt.Errorf("metadata device %s is already in use (not by rook). fs: %s, ownPartitions: %t", info.Device, fs, ownPartitions)
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
	if err := savedScheme.SaveScheme(kv, storeName); err != nil {
		return fmt.Errorf("failed to save partition scheme: %+v", err)
	}

	return nil
}

// Partitions a device for use by a osd.
// If there are any partitions or formatting already on the device, it will be wiped.
func partitionOSD(context *clusterd.Context, cfg *osdConfig) error {
	dataDetails, err := getDataPartitionDetails(cfg)
	if err != nil {
		return err
	}

	// zap/clear all existing partitions on the device
	err = sys.RemovePartitions(dataDetails.Device, context.Executor)
	if err != nil {
		return fmt.Errorf("failed to zap partitions on metadata device /dev/%s: %+v", dataDetails.Device, err)
	}

	// create the partitions on the device
	err = sys.CreatePartitions(dataDetails.Device, cfg.partitionScheme.GetPartitionArgs(), context.Executor)
	if err != nil {
		return fmt.Errorf("failed to partition /dev/%s. %+v", dataDetails.Device, err)
	}

	if cfg.partitionScheme.StoreType == config.Filestore {
		// the OSD is using filestore, create a filesystem for the device (format it) and mount it under config root
		doFormat := true
		if err = prepareFilestoreDevice(context, cfg, doFormat); err != nil {
			return err
		}
	}

	// save the partition scheme entry to disk now that it has been committed
	savedScheme, err := config.LoadScheme(cfg.kv, cfg.storeName)
	if err != nil {
		return fmt.Errorf("failed to load the saved partition scheme: %+v", err)
	}
	savedScheme.Entries = append(savedScheme.Entries, cfg.partitionScheme)
	if err := savedScheme.SaveScheme(cfg.kv, cfg.storeName); err != nil {
		return fmt.Errorf("failed to save partition scheme: %+v", err)
	}

	// update the uuid of the disk in the inventory in memory
	logger.Debugf("Updating disk uuid %s on device %s", dataDetails.DiskUUID, dataDetails.Device)
	for _, disk := range context.Devices {
		if disk.Name == dataDetails.Device {
			logger.Debugf("Updated uuid on device %s", dataDetails.Device)
			disk.UUID = dataDetails.DiskUUID
		}
	}

	return nil
}

func prepareFilestoreDevice(context *clusterd.Context, cfg *osdConfig, doFormat bool) error {
	if !isFilestoreDevice(cfg) {
		return fmt.Errorf("osd is not a filestore device: %+v", cfg)
	}

	// wait for the special /dev/disk/by-partuuid path to show up
	dataPartDetails := cfg.partitionScheme.Partitions[config.FilestoreDataPartitionType]
	dataPartPath := filepath.Join(diskByPartUUID, dataPartDetails.PartitionUUID)
	logger.Infof("waiting for partition path %s", dataPartPath)
	err := waitForPath(dataPartPath, context.Executor)
	if err != nil {
		return fmt.Errorf("failed waiting for %s: %+v", dataPartPath, err)
	}

	if doFormat {
		// perform the format and retry if needed
		if err = sys.FormatDevice(dataPartPath, context.Executor); err != nil {
			logger.Warningf("first attempt to format partition %s on device %s failed.  Waiting 2 seconds then retrying: %+v",
				dataPartDetails.PartitionUUID, dataPartDetails.Device, err)
			<-time.After(2 * time.Second)
			if err = sys.FormatDevice(dataPartPath, context.Executor); err != nil {
				return fmt.Errorf("failed to format partition %s on device %s. %+v", dataPartDetails.PartitionUUID, dataPartDetails.Device, err)
			}
		}
	}

	// mount the device
	if err = sys.MountDevice(dataPartPath, cfg.rootPath, context.Executor); err != nil {
		return fmt.Errorf("failed to mount %s at %s: %+v", dataPartPath, cfg.rootPath, context.Executor)
	}

	return nil
}

// checks the given OSD config to determine if it is for filestore on a device.  If the device has already
// been partitioned then we need to remount the device to the OSD root path so that all the OSD config/data
// shows up under the config root once again.
func remountFilestoreDeviceIfNeeded(context *clusterd.Context, cfg *osdConfig) error {
	if !isFilestoreDevice(cfg) {
		// nothing to do
		return nil
	}

	savedScheme, err := config.LoadScheme(cfg.kv, cfg.storeName)
	if err != nil {
		return fmt.Errorf("failed to load the saved partition scheme from %s: %+v", cfg.configRoot, err)
	}

	for _, savedEntry := range savedScheme.Entries {
		if savedEntry.ID == cfg.id {
			// the current saved partition scheme entry exists, meaning the partitions have already been created.
			// we need to remount the device/partitions now so that the OSD's config will show up under the config
			// root again.
			doFormat := false
			if err = prepareFilestoreDevice(context, cfg, doFormat); err != nil {
				return err
			}
			break
		}
	}

	return nil
}

func getDataPartitionDetails(config *osdConfig) (*config.PerfSchemePartitionDetails, error) {
	if config.partitionScheme == nil {
		return nil, fmt.Errorf("partition scheme missing from %+v", config)
	}

	dataPartitionType := config.partitionScheme.GetDataPartitionType()

	dataDetails, ok := config.partitionScheme.Partitions[dataPartitionType]
	if !ok || dataDetails == nil {
		return nil, fmt.Errorf("data partition missing from %+v", config.partitionScheme)
	}

	return dataDetails, nil
}

func getMetadataPartitionDetails(cfg *osdConfig) (*config.PerfSchemePartitionDetails, error) {
	if cfg.partitionScheme == nil {
		return nil, fmt.Errorf("partition scheme missing from %+v", cfg)
	}

	metadataPartitionType := cfg.partitionScheme.GetMetadataPartitionType()

	if cfg.partitionScheme.StoreType == config.Filestore {
		// TODO: support separate metadata device for filestore (just use the data partition details for now)
		return getDataPartitionDetails(cfg)
	}

	metadataDetails, ok := cfg.partitionScheme.Partitions[metadataPartitionType]
	if !ok || metadataDetails == nil {
		return nil, fmt.Errorf("metadata partition missing from %+v", cfg.partitionScheme)
	}

	return metadataDetails, nil
}

func getDiskSize(context *clusterd.Context, name string) (uint64, error) {
	for _, device := range context.Devices {
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

func getStoreSettings(cfg *osdConfig) (map[string]string, error) {
	settings := map[string]string{}
	if isFilestore(cfg) {
		// add additional filestore settings for filestore
		journalSize := config.JournalDefaultSizeMB
		if cfg.storeConfig.JournalSizeMB > 0 {
			journalSize = cfg.storeConfig.JournalSizeMB
		}
		settings["osd journal size"] = strconv.Itoa(journalSize)
		return settings, nil
	}

	// initialize the full set of config settings for bluestore
	var walPath, dbPath, blockPath string
	var err error

	if isBluestoreDir(cfg) {
		// a directory is being used for bluestore, initialize all the required settings
		walPath, dbPath, blockPath, err = getBluestoreDirPaths(cfg)
		if err != nil {
			return nil, err
		}

		// ceph will create the block, db, and wal files for us
		settings["bluestore block wal create"] = "true"
		settings["bluestore block db create"] = "true"
		settings["bluestore block create"] = "true"

		// set the size of the wal and db files
		walSizeMB := config.WalDefaultSizeMB
		if cfg.storeConfig.WalSizeMB > 0 {
			walSizeMB = cfg.storeConfig.WalSizeMB
		}
		dbSizeMB := config.DBDefaultSizeMB
		if cfg.storeConfig.DatabaseSizeMB > 0 {
			dbSizeMB = cfg.storeConfig.DatabaseSizeMB
		}

		// ceph config uses bytes, not MB, so convert to bytes
		walSize := walSizeMB * 1024 * 1024
		settings["bluestore block wal size"] = strconv.Itoa(walSize)
		dbSize := dbSizeMB * 1024 * 1024
		settings["bluestore block db size"] = strconv.Itoa(dbSize)

		// Get the total size of the filesystem that contains the OSD root dir and make the bluestore block
		// file to be a percentage of that size.  Note that by default ceph will not preallocate the full
		// block file, so it's OK if the entire space is not available.  We will not see any errors until
		// the disk fills up.
		totalBytes, err := getSizeForPath(cfg.rootPath)
		if err != nil {
			return nil, err
		}

		logger.Infof("total bytes for %s: %d (%s)", cfg.rootPath, totalBytes, display.BytesToString(totalBytes))
		settings["bluestore block size"] = strconv.Itoa(int(float64(totalBytes) * bluestoreDirBlockSizeRatio))
	} else {
		// devices are being used for bluestore, all we need is their paths
		if cfg.partitionScheme == nil || cfg.partitionScheme.Partitions == nil {
			return nil, fmt.Errorf("failed to find partitions from config for osd %d", cfg.id)
		}

		walPath, dbPath, blockPath, err = getBluestorePartitionPaths(cfg)
		if err != nil {
			return nil, err
		}
	}

	settings["bluestore block wal path"] = walPath
	settings["bluestore block db path"] = dbPath
	settings["bluestore block path"] = blockPath

	return settings, nil
}

func writeConfigFile(cfg *osdConfig, context *clusterd.Context, cluster *mon.ClusterInfo, location string) error {
	cephConfig := mon.CreateDefaultCephConfig(context, cluster, cfg.rootPath)
	if isBluestore(cfg) {
		cephConfig.GlobalConfig.OsdObjectStore = config.Bluestore
	} else {
		cephConfig.GlobalConfig.OsdObjectStore = config.Filestore
	}
	cephConfig.CrushLocation = location

	if cfg.dir || isFilestoreDevice(cfg) {
		// using the local file system requires some config overrides
		// http://docs.ceph.com/docs/jewel/rados/configuration/filesystem-recommendations/#not-recommended
		cephConfig.GlobalConfig.OsdMaxObjectNameLen = 256
		cephConfig.GlobalConfig.OsdMaxObjectNamespaceLen = 64
	}

	// bluestore has some extra settings
	settings, err := getStoreSettings(cfg)
	if err != nil {
		return fmt.Errorf("failed to read store settings. %+v", err)
	}

	// write the OSD config file to disk
	_, err = mon.GenerateConfigFile(context, cluster, cfg.rootPath, fmt.Sprintf("osd.%d", cfg.id),
		getOSDKeyringPath(cfg.rootPath), cephConfig, settings)
	if err != nil {
		return fmt.Errorf("failed to write OSD %d config file: %+v", cfg.id, err)
	}

	return nil
}

func initializeOSD(config *osdConfig, context *clusterd.Context, cluster *mon.ClusterInfo, location string) error {

	err := writeConfigFile(config, context, cluster, location)
	if err != nil {
		return fmt.Errorf("failed to write config file: %+v", err)
	}

	// add auth privileges for the OSD, the bootstrap-osd privileges were very limited
	if err := addOSDAuth(context, cluster.Name, config.id, config.rootPath); err != nil {
		return err
	}

	// check to see if the OSD file system had been created (using osd mkfs) and backed up in the past
	if !isOSDFilesystemCreated(config) {
		// create/initialize the OSD file system and journal
		if err := createOSDFileSystem(context, cluster.Name, config); err != nil {
			return err
		}
	} else {
		// The OSD file system had previously been created and backed up, try to repair it now
		if err := repairOSDFileSystem(config); err != nil {
			return err
		}
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
		return 0, fmt.Errorf("failed to unmarshal response: %+v.  raw response: '%s'", err, string(buf[:]))
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

// add OSD auth privileges for the given OSD ID.  the bootstrap-osd privileges are limited and a real OSD needs more.
func addOSDAuth(context *clusterd.Context, clusterName string, osdID int, osdDataPath string) error {
	// get an existing auth or create a new auth for this OSD.  After this command is run, the new or existing
	// keyring will be written to the keyring path specified.
	osdEntity := fmt.Sprintf("osd.%d", osdID)
	caps := []string{"osd", "allow *", "mon", "allow profile osd"}

	return client.AuthGetOrCreate(context, clusterName, osdEntity, getOSDKeyringPath(osdDataPath), caps)
}

// adds the given OSD to the crush map
func addOSDToCrushMap(context *clusterd.Context, config *osdConfig, clusterName, location string) error {
	osdID := config.id
	osdDataPath := config.rootPath

	var totalBytes uint64
	var err error
	if !isBluestoreDevice(config) {
		// get the size of the volume containing the OSD data dir.  For filestore/bluestore directory
		// or device, this will be a mounted filesystem, so we can use Statfs
		totalBytes, err = getSizeForPath(osdDataPath)
		if err != nil {
			return err
		}
	} else {
		// for bluestore devices, the data partition will be raw, so we can't use Statfs.  Get the
		// full device properties of the data partition and then get the size from that.
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
	logger.Infof("adding %s (%s), bytes: %d, weight: %.4f, to crush map at '%s'",
		osdEntity, osdDataPath, totalBytes, weight, location)
	args := []string{"osd", "crush", "create-or-move", strconv.Itoa(osdID), fmt.Sprintf("%.4f", weight)}
	args = append(args, strings.Split(location, " ")...)
	_, err = client.ExecuteCephCommand(context, clusterName, args)
	if err != nil {
		return fmt.Errorf("failed adding %s to crush map: %+v", osdEntity, err)
	}

	return nil
}

func markOSDOut(context *clusterd.Context, clusterName string, id int) error {
	_, err := client.OSDOut(context, clusterName, id)
	return err
}

func purgeOSD(context *clusterd.Context, clusterName string, id int) error {
	// remove the OSD from the crush map
	_, err := client.CrushRemove(context, clusterName, fmt.Sprintf("osd.%d", id))
	if err != nil {
		return fmt.Errorf("failed to remove osd.%d from crush map. %v", id, err)
	}

	// delete the auth for the OSD
	err = client.AuthDelete(context, clusterName, fmt.Sprintf("osd.%d", id))
	if err != nil {
		return err
	}

	// delete the OSD from the cluster
	_, err = client.OSDRemove(context, clusterName, id)
	if err != nil {
		return fmt.Errorf("failed to rm osd.%d. %v", id, err)
	}
	return nil
}

func getBluestorePartitionPaths(cfg *osdConfig) (string, string, string, error) {
	if !isBluestoreDevice(cfg) {
		return "", "", "", fmt.Errorf("must be bluestore device to get bluestore partition paths: %+v", cfg)
	}
	parts := cfg.partitionScheme.Partitions
	walPartition, ok := parts[config.WalPartitionType]
	if !ok {
		return "", "", "", fmt.Errorf("failed to find wal partition for osd %d", cfg.id)
	}
	dbPartition, ok := parts[config.DatabasePartitionType]
	if !ok {
		return "", "", "", fmt.Errorf("failed to find db partition for osd %d", cfg.id)
	}
	blockPartition, ok := parts[config.BlockPartitionType]
	if !ok {
		return "", "", "", fmt.Errorf("failed to find block partition for osd %d", cfg.id)
	}

	return filepath.Join(diskByPartUUID, walPartition.PartitionUUID),
		filepath.Join(diskByPartUUID, dbPartition.PartitionUUID),
		filepath.Join(diskByPartUUID, blockPartition.PartitionUUID),
		nil

}

func getBluestoreDirPaths(cfg *osdConfig) (string, string, string, error) {
	if !isBluestoreDir(cfg) {
		return "", "", "", fmt.Errorf("must be bluestore dir to get bluestore dir paths: %+v", cfg)
	}

	return filepath.Join(cfg.rootPath, config.BluestoreDirWalName),
		filepath.Join(cfg.rootPath, config.BluestoreDirDBName),
		filepath.Join(cfg.rootPath, config.BluestoreDirBlockName),
		nil
}

// getSizeForPath returns the size of the filesystem at the given path.
func getSizeForPath(path string) (uint64, error) {
	s := syscall.Statfs_t{}
	if err := syscall.Statfs(path, &s); err != nil {
		return 0, fmt.Errorf("failed to statfs on %s, %+v", path, err)
	}

	return s.Blocks * uint64(s.Bsize), nil
}
