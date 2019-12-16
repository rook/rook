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
	"path"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/google/uuid"

	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util"
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
	storeConfig     config.StoreConfig
	partitionScheme *config.PerfSchemeEntry
	kv              *k8sutil.ConfigMapKVStore
	storeName       string
}

type Device struct {
	Name   string `json:"name"`
	NodeID string `json:"nodeId"`
	Dir    bool   `json:"bool"`
}

// DesiredDevice keeps track of the desired settings for a device
type DesiredDevice struct {
	Name               string
	OSDsPerDevice      int
	MetadataDevice     string
	DatabaseSizeMB     int
	DeviceClass        string
	IsFilter           bool
	IsDevicePathFilter bool
}

type DeviceOsdMapping struct {
	Entries map[string]*DeviceOsdIDEntry // device name to OSD ID mapping entry
}

type DeviceOsdIDEntry struct {
	Data                  int           // OSD ID that has data stored here
	Metadata              []int         // OSD IDs (multiple) that have metadata stored here
	Config                DesiredDevice // Device specific config options
	LegacyPartitionsFound bool          // Whether legacy rook partitions were found
	PersistentDevicePaths []string
}

type devicePartInfo struct {
	// the path to the mount that needs to be unmounted after the configuration is completed
	pathToUnmount string

	// The UUID of the partition where the osd is found under /dev/disk/by-partuuid
	deviceUUID string
}

func (m *DeviceOsdMapping) String() string {
	b, _ := json.Marshal(m)
	return string(b)
}

// format the given device for usage by an OSD
func formatDevice(context *clusterd.Context, config *osdConfig, forceFormat bool, storeConfig config.StoreConfig) (*devicePartInfo, error) {
	dataDetails, err := getDataPartitionDetails(config)
	if err != nil {
		return nil, err
	}

	// check if partitions belong to rook
	pvcBackedOSD := false
	_, ownPartitions, devFS, err := sys.CheckIfDeviceAvailable(context.Executor, dataDetails.Device, pvcBackedOSD)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to format device")
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
			return nil, errors.Errorf("device %s already formatted with %s", dataDetails.Device, devFS)
		}
	}

	// format the device
	dangerousToFormat := !ownPartitions || devFS != ""
	var devPartInfo *devicePartInfo
	if !dangerousToFormat || forceFormat {
		devPartInfo, err = partitionOSD(context, config)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to partion device %s", dataDetails.Device)
		}
	}

	return devPartInfo, nil
}

// partitions a given device exclusively for metadata usage
func partitionMetadata(context *clusterd.Context, info *config.MetadataDeviceInfo, kv *k8sutil.ConfigMapKVStore, storeName string) error {
	if len(info.Partitions) == 0 {
		return nil
	}

	// check to see if the metadata partition scheme has already been applied
	savedScheme, err := config.LoadScheme(kv, storeName)
	if err != nil {
		return errors.Wrapf(err, "failed to load the saved partition scheme")
	}

	if savedScheme.Metadata != nil && len(savedScheme.Metadata.Partitions) > 0 {
		// TODO: there is already an existing metadata partition scheme that has been applied, we should be able to add to it
		// https://github.com/rook/rook/issues/341
		if info.DiskUUID == savedScheme.Metadata.DiskUUID {
			// the existing metadata partition scheme is the same disk as the desired metadata device.  no work to perform.
			return nil
		}
		return errors.Errorf("metadata partition scheme already exists on %s (%s), cannot use desired metadata device %s (%s)",
			savedScheme.Metadata.Device, savedScheme.Metadata.DiskUUID, info.Device, info.DiskUUID)
	}

	// check one last time to make sure it's OK for us to format this metadata device
	pvcBackedOSD := false
	_, ownPartitions, fs, err := sys.CheckIfDeviceAvailable(context.Executor, info.Device, pvcBackedOSD)
	if err != nil {
		return errors.Wrapf(err, "failed to get metadata device %s info", info.Device)
	} else if fs != "" || !ownPartitions {
		return errors.Errorf("metadata device %s is already in use (not by rook). fs: %s, ownPartitions: %t", info.Device, fs, ownPartitions)
	}

	// zap/clear all existing partitions
	err = sys.RemovePartitions(info.Device, context.Executor)
	if err != nil {
		return errors.Wrapf(err, "failed to zap partitions on metadata device /dev/%s", info.Device)
	}

	// create the partitions
	err = sys.CreatePartitions(info.Device, info.GetPartitionArgs(), context.Executor)
	if err != nil {
		return errors.Wrapf(err, "failed to partition metadata device /dev/%s", info.Device)
	}

	// save the metadata partition info to disk now that it has been committed
	savedScheme.Metadata = info
	if err := savedScheme.SaveScheme(kv, storeName); err != nil {
		return errors.Wrapf(err, "failed to save partition scheme")
	}

	return nil
}

// Partitions a device for use by a osd.
// If there are any partitions or formatting already on the device, it will be wiped.
func partitionOSD(context *clusterd.Context, cfg *osdConfig) (*devicePartInfo, error) {
	dataDetails, err := getDataPartitionDetails(cfg)
	if err != nil {
		return nil, err
	}

	// zap/clear all existing partitions on the device
	err = sys.RemovePartitions(dataDetails.Device, context.Executor)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to zap partitions on metadata device /dev/%s", dataDetails.Device)
	}

	// create the partitions on the device
	err = sys.CreatePartitions(dataDetails.Device, cfg.partitionScheme.GetPartitionArgs(), context.Executor)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to partition /dev/%s", dataDetails.Device)
	}

	var devPartInfo *devicePartInfo
	if cfg.partitionScheme.StoreType == config.Filestore {
		// the OSD is using filestore, create a filesystem for the device (format it) and mount it under config root
		doFormat := true
		devPartInfo, err = prepareFilestoreDevice(context, cfg, doFormat)
		if err != nil {
			return nil, err
		}
	}

	// save the partition scheme entry to disk now that it has been committed
	savedScheme, err := config.LoadScheme(cfg.kv, cfg.storeName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load the saved partition scheme")
	}
	savedScheme.Entries = append(savedScheme.Entries, cfg.partitionScheme)
	if err := savedScheme.SaveScheme(cfg.kv, cfg.storeName); err != nil {
		return nil, errors.Wrapf(err, "failed to save partition scheme")
	}

	// update the uuid of the disk in the inventory in memory
	logger.Debugf("Updating disk uuid %s on device %s", dataDetails.DiskUUID, dataDetails.Device)
	for _, disk := range context.Devices {
		if disk.Name == dataDetails.Device {
			logger.Debugf("Updated uuid on device %s", dataDetails.Device)
			disk.UUID = dataDetails.DiskUUID
		}
	}

	return devPartInfo, nil
}

func prepareFilestoreDevice(context *clusterd.Context, cfg *osdConfig, doFormat bool) (*devicePartInfo, error) {
	if !isFilestoreDevice(cfg) {
		return nil, errors.Errorf("osd is not a filestore device: %+v", cfg)
	}

	// wait for the special /dev/disk/by-partuuid path to show up
	dataPartDetails := cfg.partitionScheme.Partitions[config.FilestoreDataPartitionType]
	dataPartPath := filepath.Join(diskByPartUUID, dataPartDetails.PartitionUUID)
	logger.Infof("waiting for partition path %s", dataPartPath)
	err := waitForPath(dataPartPath, context.Executor)
	if err != nil {
		return nil, errors.Wrapf(err, "failed waiting for %s", dataPartPath)
	}

	if doFormat {
		// perform the format and retry if needed
		if err = sys.FormatDevice(dataPartPath, context.Executor); err != nil {
			logger.Warningf("first attempt to format partition %q on device %q failed.  Waiting 2 seconds then retrying. %v",
				dataPartDetails.PartitionUUID, dataPartDetails.Device, err)
			<-time.After(2 * time.Second)
			if err = sys.FormatDevice(dataPartPath, context.Executor); err != nil {
				return nil, errors.Wrapf(err, "failed to format partition %s on device %s", dataPartDetails.PartitionUUID, dataPartDetails.Device)
			}
		}
	}

	// mount the device
	if err = sys.MountDevice(dataPartPath, cfg.rootPath, context.Executor); err != nil {
		return nil, errors.Wrapf(err, "failed to mount %s at %s", dataPartPath, cfg.rootPath)
	}

	return &devicePartInfo{pathToUnmount: cfg.rootPath, deviceUUID: dataPartDetails.PartitionUUID}, nil
}

// checks the given OSD config to determine if it is for filestore on a device.  If the device has already
// been partitioned then we need to remount the device to the OSD root path so that all the OSD config/data
// shows up under the config root once again.
func remountFilestoreDeviceIfNeeded(context *clusterd.Context, cfg *osdConfig) (*devicePartInfo, error) {
	if !isFilestoreDevice(cfg) {
		// nothing to do
		return nil, nil
	}

	savedScheme, err := config.LoadScheme(cfg.kv, cfg.storeName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to load the saved partition scheme from %s", cfg.configRoot)
	}

	var devPartInfo *devicePartInfo
	for _, savedEntry := range savedScheme.Entries {
		if savedEntry.ID == cfg.id {
			// the current saved partition scheme entry exists, meaning the partitions have already been created.
			// we need to remount the device/partitions now so that the OSD's config will show up under the config
			// root again.
			doFormat := false
			devPartInfo, err = prepareFilestoreDevice(context, cfg, doFormat)
			if err != nil {
				return nil, err
			}
			break
		}
	}

	return devPartInfo, nil
}

func getDataPartitionDetails(config *osdConfig) (*config.PerfSchemePartitionDetails, error) {
	if config.partitionScheme == nil {
		return nil, errors.Errorf("partition scheme missing from %+v", config)
	}

	dataPartitionType := config.partitionScheme.GetDataPartitionType()

	dataDetails, ok := config.partitionScheme.Partitions[dataPartitionType]
	if !ok || dataDetails == nil {
		return nil, errors.Errorf("data partition missing from %+v", config.partitionScheme)
	}

	return dataDetails, nil
}

func getDiskSize(context *clusterd.Context, name string) (uint64, error) {
	for _, device := range context.Devices {
		if device.Name == name {
			return device.Size, nil
		}
	}

	return 0, errors.Errorf("device %s not found", name)
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
		return nil, nil, errors.Wrapf(err, "failed to generate UUID for osd")
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
			basePath := fmt.Sprintf("/var/lib/ceph/osd/ceph-%d", cfg.id)
			settings["bluestore block path"] = path.Join(basePath, "block")
			settings["keyring"] = path.Join(basePath, "keyring")
			return settings, nil
			//FIX: return nil, errors.Wrapf(err, "failed to find partitions from config for osd %d", cfg.id)
		} else {
			walPath, dbPath, blockPath, err = getBluestorePartitionPaths(cfg)
			if err != nil {
				return nil, err
			}
		}
	}

	settings["bluestore block wal path"] = walPath
	settings["bluestore block db path"] = dbPath
	settings["bluestore block path"] = blockPath

	return settings, nil
}

func WriteConfigFile(context *clusterd.Context, cluster *cephconfig.ClusterInfo, kv *k8sutil.ConfigMapKVStore, osdID int, device bool, storeConfig config.StoreConfig, nodeName string) error {
	scheme, err := config.LoadScheme(kv, config.GetConfigStoreName(nodeName))
	if err != nil {
		return errors.Wrapf(err, "failed to load partition scheme")
	}

	cfg := &osdConfig{id: osdID, configRoot: context.ConfigDir, rootPath: getOSDRootDir(context.ConfigDir, osdID),
		storeConfig: storeConfig, kv: kv, storeName: config.GetConfigStoreName(nodeName)}

	// if a device, search the osd scheme for the requested osd id
	for _, entry := range scheme.Entries {
		if entry.ID == osdID {
			cfg.partitionScheme = entry
			cfg.uuid = entry.OsdUUID
			logger.Infof("found osd %d in device map for uuid %s", osdID, cfg.uuid.String())
			break
		}
	}
	// if not identified as a device, confirm that it is found in the map of directories
	if !device {
		cfg.dir = true
		dirMap, err := config.LoadOSDDirMap(kv, nodeName)
		if err != nil {
			return errors.Wrapf(err, "failed to load osd dir map")
		}

		id, ok := dirMap[context.ConfigDir]
		if !ok {
			return errors.Errorf("dir %s was not found in the dir map. %+v", context.ConfigDir, dirMap)
		}
		if id != osdID {
			return errors.Errorf("dir found in dirMap, but desired osd ID %d does not match dirMap id %d", osdID, id)
		}
		logger.Infof("found osd %d in dir map for path %s", osdID, context.ConfigDir)
	}

	logger.Infof("updating config for osd %d", osdID)
	err = writeConfigFile(cfg, context, cluster)
	if err != nil {
		return err
	}
	confFile := getOSDConfFilePath(cfg.rootPath, cluster.Name)
	util.WriteFileToLog(logger, confFile)
	return nil
}

func writeConfigFile(cfg *osdConfig, context *clusterd.Context, cluster *cephconfig.ClusterInfo) error {
	cephConfig, err := cephconfig.CreateDefaultCephConfig(context, cluster)
	if err != nil {
		return errors.Wrapf(err, "failed to create default ceph config")
	}
	if isBluestore(cfg) {
		cephConfig.GlobalConfig.OsdObjectStore = config.Bluestore
	} else {
		cephConfig.GlobalConfig.OsdObjectStore = config.Filestore
	}

	if cfg.dir || isFilestoreDevice(cfg) {
		// using the local file system requires some config overrides
		// http://docs.ceph.com/docs/jewel/rados/configuration/filesystem-recommendations/#not-recommended
		cephConfig.GlobalConfig.OsdMaxObjectNameLen = 256
		cephConfig.GlobalConfig.OsdMaxObjectNamespaceLen = 64
	}

	// bluestore has some extra settings
	settings, err := getStoreSettings(cfg)
	if err != nil {
		return errors.Wrapf(err, "failed to read store settings")
	}

	// write the OSD config file to disk
	_, err = cephconfig.GenerateConfigFile(context, cluster, cfg.rootPath, fmt.Sprintf("osd.%d", cfg.id),
		getOSDKeyringPath(cfg.rootPath), cephConfig, settings)
	if err != nil {
		return errors.Wrapf(err, "failed to write OSD %d config file", cfg.id)
	}

	return nil
}

func initializeOSD(config *osdConfig, context *clusterd.Context, cluster *cephconfig.ClusterInfo) error {
	err := writeConfigFile(config, context, cluster)
	if err != nil {
		return errors.Wrapf(err, "failed to write config file")
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

	return nil
}

// creates the OSD identity in the cluster via a mon_command
func createOSD(context *clusterd.Context, clusterName string, osdUUID uuid.UUID) (int, error) {
	// TODO: "entity": "client.bootstrap-osd",
	args := []string{"osd", "create", osdUUID.String()}
	buf, err := client.NewCephCommand(context, clusterName, args).Run()
	if err != nil {
		return 0, errors.Wrapf(err, "failed to create osd %s", osdUUID)
	}

	var resp map[string]interface{}
	err = json.Unmarshal(buf, &resp)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to unmarshal response. raw response: %q", string(buf[:]))
	}

	return int(resp["osdid"].(float64)), nil
}

// gets the current mon map for the cluster
func getMonMap(context *clusterd.Context, clusterName string) ([]byte, error) {
	// TODO: "entity": "client.bootstrap-osd",
	args := []string{"mon", "getmap"}
	buf, err := client.NewCephCommand(context, clusterName, args).Run()
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get mon map")
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

func getBluestorePartitionPaths(cfg *osdConfig) (string, string, string, error) {
	if !isBluestoreDevice(cfg) {
		return "", "", "", errors.Errorf("must be bluestore device to get bluestore partition paths: %+v", cfg)
	}
	parts := cfg.partitionScheme.Partitions
	walPartition, ok := parts[config.WalPartitionType]
	if !ok {
		return "", "", "", errors.Errorf("failed to find wal partition for osd %d", cfg.id)
	}
	dbPartition, ok := parts[config.DatabasePartitionType]
	if !ok {
		return "", "", "", errors.Errorf("failed to find db partition for osd %d", cfg.id)
	}
	blockPartition, ok := parts[config.BlockPartitionType]
	if !ok {
		return "", "", "", errors.Errorf("failed to find block partition for osd %d", cfg.id)
	}

	return filepath.Join(diskByPartUUID, walPartition.PartitionUUID),
		filepath.Join(diskByPartUUID, dbPartition.PartitionUUID),
		filepath.Join(diskByPartUUID, blockPartition.PartitionUUID),
		nil

}

func getBluestoreDirPaths(cfg *osdConfig) (string, string, string, error) {
	if !isBluestoreDir(cfg) {
		return "", "", "", errors.Errorf("must be bluestore dir to get bluestore dir paths: %+v", cfg)
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
		return 0, errors.Wrapf(err, "failed to statfs on %s", path)
	}

	return s.Blocks * uint64(s.Bsize), nil
}
