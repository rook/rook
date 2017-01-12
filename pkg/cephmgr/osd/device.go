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

	"github.com/google/uuid"

	"github.com/rook/rook/pkg/cephmgr/client"
	"github.com/rook/rook/pkg/cephmgr/mon"
	"github.com/rook/rook/pkg/cephmgr/osd/partition"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/clusterd/inventory"
	"github.com/rook/rook/pkg/util"
	"github.com/rook/rook/pkg/util/exec"
	"github.com/rook/rook/pkg/util/sys"
)

const (
	DevicesValue                = "devices"
	ForceFormatValue            = "forceFormat"
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
	partitionScheme *partition.PerfSchemeEntry
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
func formatDevice(context *clusterd.Context, config *osdConfig, forceFormat bool) error {
	dangerousToFormat := false

	blockDetails, err := getBlockPartitionDetails(config)
	if err != nil {
		return err
	}

	// check if partitions belong to rook
	partitions, _, err := sys.GetDevicePartitions(blockDetails.Device, context.Executor)
	if err != nil {
		return fmt.Errorf("failed to get %s partitions. %+v", blockDetails.Device, err)
	}
	if !rookOwnsPartitions(partitions) {
		dangerousToFormat = true
		if forceFormat {
			logger.Warningf("device %s is being formatted!! partitions: %+v", blockDetails.Device, partitions)
		} else {
			logger.Warningf("device %s has partitions that will not be formatted. Skipping device.", blockDetails.Device)
		}
	}

	// check if there is a file system on the device
	devFS, err := sys.GetDeviceFilesystems(blockDetails.Device, context.Executor)
	if err != nil {
		return fmt.Errorf("failed to get device %s filesystem: %+v", blockDetails.Device, err)
	}
	if devFS != "" {
		dangerousToFormat = true
		if forceFormat {
			// there's a filesystem on the device, but the user has specified to force a format. give a warning about that.
			logger.Warningf("device %s already formatted with %s, but forcing a format!!!", blockDetails.Device, devFS)
		} else {
			// disk is already formatted and the user doesn't want to force it, but we require partitioning
			return fmt.Errorf("device %s already formatted with %s", blockDetails.Device, devFS)
		}
	}

	// format the device
	if !dangerousToFormat || forceFormat {
		err := partitionBluestoreOSD(context, config)
		if err != nil {
			return fmt.Errorf("failed to partion device %s. %v", blockDetails.Device, err)
		}
	}

	return nil
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

// partitions a given device exclusively for bluestore metadata usage
func partitionBluestoreMetadata(context *clusterd.Context, info *partition.MetadataDeviceInfo, configRoot string) error {
	if len(info.Partitions) == 0 {
		return nil
	}

	// check to see if the metadata partition scheme has already been applied
	savedScheme, err := partition.LoadScheme(configRoot)
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
	idSet := util.NewSet()
	for _, part := range info.Partitions {
		idSet.Add(strconv.Itoa(part.ID))
	}
	if err := associateOSDIDsWithMetadataDevice(context.EtcdClient, context.NodeID, info.DiskUUID, strings.Join(idSet.ToSlice(), ",")); err != nil {
		return fmt.Errorf("failed to associate osd ids '%+v' with metadata device %s (%s): %+v", idSet, info.Device, info.DiskUUID, err)
	}

	return nil
}

// Partitions a device for use by a bluestore osd.
// If there are any partitions or formatting already on the device, it will be wiped.
func partitionBluestoreOSD(context *clusterd.Context, config *osdConfig) error {
	blockDetails, err := getBlockPartitionDetails(config)
	if err != nil {
		return err
	}

	// zap/clear all existing partitions on the device
	err = sys.RemovePartitions(blockDetails.Device, context.Executor)
	if err != nil {
		return fmt.Errorf("failed to zap partitions on metadata device /dev/%s: %+v", blockDetails.Device, err)
	}

	// create the partitions on the device
	err = sys.CreatePartitions(blockDetails.Device, config.partitionScheme.GetPartitionArgs(), context.Executor)
	if err != nil {
		return fmt.Errorf("failed to partition /dev/%s. %+v", blockDetails.Device, err)
	}

	// save the partition scheme entry to disk now that it has been committed
	savedScheme, err := partition.LoadScheme(config.configRoot)
	if err != nil {
		return fmt.Errorf("failed to load the saved partition scheme from %s: %+v", config.configRoot, err)
	}
	savedScheme.Entries = append(savedScheme.Entries, config.partitionScheme)
	if err := savedScheme.Save(config.configRoot); err != nil {
		return fmt.Errorf("failed to save partition scheme to %s: %+v", config.configRoot, err)
	}

	// update the uuid of the disk in the inventory in memory
	logger.Debugf("Updating disk uuid %s on device %s", blockDetails.DiskUUID, blockDetails.Device)
	for _, disk := range context.Inventory.Local.Disks {
		if disk.Name == blockDetails.Device {
			logger.Debugf("Updated uuid on device %s", blockDetails.Device)
			disk.UUID = blockDetails.DiskUUID
		}
	}

	// save the desired state of the osd for this device
	err = associateOsdIDWithDevice(context.EtcdClient, context.NodeID, blockDetails.DiskUUID, config.id, false)
	if err != nil {
		return fmt.Errorf("failed to associate osd id %d with device %s (%s)", config.id, blockDetails.Device, blockDetails.DiskUUID)
	}
	if config.partitionScheme.IsCollocated() {
		// the metadata is on the same disk as the data, associate the osd ID with the device for metadata too
		err = associateOSDIDsWithMetadataDevice(
			context.EtcdClient, context.NodeID, blockDetails.DiskUUID, fmt.Sprintf("%d", config.id))
		if err != nil {
			return fmt.Errorf("failed to associate osd id %d with device %s (%s) for metadata",
				config.id, blockDetails.Device, blockDetails.DiskUUID)
		}
	}

	return nil
}

func getBlockPartitionDetails(config *osdConfig) (*partition.PerfSchemePartitionDetails, error) {
	if config.partitionScheme == nil {
		return nil, fmt.Errorf("partition scheme missing from %+v", config)
	}

	blockDetails, ok := config.partitionScheme.Partitions[partition.BlockPartitionName]
	if !ok || blockDetails == nil {
		return nil, fmt.Errorf("block partition missing from %+v", config.partitionScheme)
	}

	return blockDetails, nil
}

func getDiskSize(context *clusterd.Context, name string) (uint64, error) {
	for _, device := range context.Inventory.Local.Disks {
		if device.Name == name {
			return device.Size, nil
		}
	}

	return 0, fmt.Errorf("device %s not found", name)
}

func registerOSD(bootstrapConn client.Connection) (*int, *uuid.UUID, error) {
	var err error
	osdUUID, err := uuid.NewRandom()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate UUID for osd: %+v", err)
	}

	// create the OSD instance via a mon_command, this assigns a cluster wide ID to the OSD
	osdID, err := createOSD(bootstrapConn, osdUUID)
	if err != nil {
		return nil, nil, err
	}

	logger.Infof("successfully created OSD %s with ID %d", osdUUID.String(), osdID)
	return &osdID, &osdUUID, nil
}

func getStoreSettings(context *clusterd.Context, config *osdConfig) (map[string]string, error) {
	settings := map[string]string{}
	if config.dir {
		return settings, nil
	}

	if config.partitionScheme == nil || config.partitionScheme.Partitions == nil {
		return nil, fmt.Errorf("failed to find partitions from config for osd %d", config.id)
	}

	parts := config.partitionScheme.Partitions
	walPartition, ok := parts[partition.WalPartitionName]
	if !ok {
		return nil, fmt.Errorf("failed to find wal partition for osd %d", config.id)
	}
	dbPartition, ok := parts[partition.DatabasePartitionName]
	if !ok {
		return nil, fmt.Errorf("failed to find db partition for osd %d", config.id)
	}
	blockPartition, ok := parts[partition.BlockPartitionName]
	if !ok {
		return nil, fmt.Errorf("failed to find block partition for osd %d", config.id)
	}

	prefix := "/dev/disk/by-partuuid"
	settings["bluestore block wal path"] = path.Join(prefix, walPartition.PartitionUUID)
	settings["bluestore block db path"] = path.Join(prefix, dbPartition.PartitionUUID)
	settings["bluestore block path"] = path.Join(prefix, blockPartition.PartitionUUID)

	return settings, nil
}

func initializeOSD(config *osdConfig, factory client.ConnectionFactory, context *clusterd.Context,
	bootstrapConn client.Connection, cluster *mon.ClusterInfo, location string, executor exec.Executor) error {

	cephConfig := mon.CreateDefaultCephConfig(cluster, config.rootPath, context.LogLevel, !config.dir)

	if config.dir {
		// using the local file system requires some config overrides
		// http://docs.ceph.com/docs/jewel/rados/configuration/filesystem-recommendations/#not-recommended
		cephConfig.GlobalConfig.OsdMaxObjectNameLen = 256
		cephConfig.GlobalConfig.OsdMaxObjectNamespaceLen = 64
	}

	// bluestore has some extra settings
	settings, err := getStoreSettings(context, config)
	if err != nil {
		return fmt.Errorf("failed to read store settings. %+v", err)
	}

	// write the OSD config file to disk
	keyringPath := getOSDKeyringPath(config.rootPath)
	_, err = mon.GenerateConfigFile(context, cluster, config.rootPath, fmt.Sprintf("osd.%d", config.id),
		keyringPath, !config.dir, cephConfig, settings)
	if err != nil {
		return fmt.Errorf("failed to write OSD %d config file: %+v", config.id, err)
	}

	// get the current monmap, it will be needed for creating the OSD file system
	monMapRaw, err := getMonMap(bootstrapConn)
	if err != nil {
		return fmt.Errorf("failed to get mon map: %+v", err)
	}

	// create/initalize the OSD file system and journal
	if err := createOSDFileSystem(context, cluster.Name, config, monMapRaw); err != nil {
		return err
	}

	// add auth privileges for the OSD, the bootstrap-osd privileges were very limited
	if err := addOSDAuth(bootstrapConn, config.id, config.rootPath); err != nil {
		return err
	}

	// open a connection to the cluster using the OSDs creds
	osdConn, err := mon.ConnectToCluster(context, factory, cluster, path.Join(config.rootPath, "tmp"),
		fmt.Sprintf("osd.%d", config.id), keyringPath)
	if err != nil {
		return err
	}
	defer osdConn.Shutdown()

	// add the new OSD to the cluster crush map
	if err := addOSDToCrushMap(osdConn, context, config.id, config.rootPath, location); err != nil {
		return err
	}

	return nil
}

// creates the OSD identity in the cluster via a mon_command
func createOSD(bootstrapConn client.Connection, osdUUID uuid.UUID) (int, error) {
	cmd := map[string]interface{}{
		"prefix": "osd create",
		"entity": "client.bootstrap-osd",
		"uuid":   osdUUID.String(),
	}
	buf, err := client.ExecuteMonCommand(bootstrapConn, cmd, fmt.Sprintf("create osd %s", osdUUID))
	if err != nil {
		return 0, fmt.Errorf("failed to create osd %s: %+v", osdUUID, err)
	}

	var resp map[string]interface{}
	err = json.Unmarshal(buf, &resp)
	if err != nil {
		return 0, fmt.Errorf("failed to unmarshall %s response: %+v.  raw response: '%s'", cmd, err, string(buf[:]))
	}

	return int(resp["osdid"].(float64)), nil
}

// gets the current mon map for the cluster
func getMonMap(bootstrapConn client.Connection) ([]byte, error) {
	cmd := map[string]interface{}{
		"prefix": "mon getmap",
		"entity": "client.bootstrap-osd",
	}

	buf, err := client.ExecuteMonCommand(bootstrapConn, cmd, "get mon map")
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

	if config.dir {
		options = append(options, fmt.Sprintf("--osd-journal=%s", getOSDJournalPath(config.rootPath)))
		options = append(options, fmt.Sprintf("--keyring=%s", getOSDKeyringPath(config.rootPath)))
	}

	// create the OSD file system and journal
	err := context.ProcMan.Run(
		fmt.Sprintf("mkfs-osd%d", config.id),
		"osd",
		options...)

	if err != nil {
		return fmt.Errorf("failed osd mkfs for OSD ID %d, UUID %s, dataDir %s: %+v",
			config.id, config.uuid.String(), config.rootPath, err)
	}

	return nil
}

// add OSD auth privileges for the given OSD ID.  the bootstrap-osd privileges are limited and a real OSD needs more.
func addOSDAuth(bootstrapConn client.Connection, osdID int, osdDataPath string) error {
	// create a new auth for this OSD
	osdKeyringPath := getOSDKeyringPath(osdDataPath)
	keyringBuffer, err := ioutil.ReadFile(osdKeyringPath)
	if err != nil {
		return fmt.Errorf("failed to read OSD keyring at %s, %+v", osdKeyringPath, err)
	}

	cmd := "auth add"
	osdEntity := fmt.Sprintf("osd.%d", osdID)
	command, err := json.Marshal(map[string]interface{}{
		"prefix": cmd,
		"format": "json",
		"entity": osdEntity,
		"caps":   []string{"osd", "allow *", "mon", "allow profile osd"},
	})
	if err != nil {
		return fmt.Errorf("command %s marshall failed: %+v", cmd, err)
	}
	_, info, err := bootstrapConn.MonCommandWithInputBuffer(command, keyringBuffer)
	if err != nil {
		return fmt.Errorf("mon_command %s failed: %+v", cmd, err)
	}

	logger.Debugf("succeeded %s command for %s. info: %s", cmd, osdEntity, info)
	return nil
}

// adds the given OSD to the crush map
func addOSDToCrushMap(osdConn client.Connection, context *clusterd.Context, osdID int, osdDataPath, location string) error {
	// get the size of the volume containing the OSD data dir
	s := syscall.Statfs_t{}
	if err := syscall.Statfs(osdDataPath, &s); err != nil {
		return fmt.Errorf("failed to statfs on %s, %+v", osdDataPath, err)
	}
	all := s.Blocks * uint64(s.Bsize)

	// weight is ratio of (size in KB) / (1 GB)
	weight := float64(all/1024) / 1073741824.0
	weight, _ = strconv.ParseFloat(fmt.Sprintf("%.4f", weight), 64)

	osdEntity := fmt.Sprintf("osd.%d", osdID)
	logger.Infof("OSD %s at %s, bytes: %d, weight: %.4f", osdEntity, osdDataPath, all, weight)

	locArgs, err := formatLocation(location)
	if err != nil {
		return err
	}

	cmd := map[string]interface{}{
		"prefix": "osd crush create-or-move",
		"id":     osdID,
		"weight": weight,
		"args":   locArgs,
	}
	_, err = client.ExecuteMonCommand(osdConn, cmd, fmt.Sprintf("adding %s to crush map", osdEntity))
	if err != nil {
		return fmt.Errorf("failed adding %s to crush map: %+v", osdEntity, err)
	}

	if err := inventory.SetLocation(context.EtcdClient, context.NodeID, strings.Join(locArgs, ",")); err != nil {
		return fmt.Errorf("failed to save CRUSH location for OSD %s: %+v", osdEntity, err)
	}

	return nil
}

func markOSDOut(connection client.Connection, id int) error {
	command := map[string]interface{}{
		"prefix": "osd out",
		"ids":    []int{id},
	}
	_, err := client.ExecuteMonCommand(connection, command, fmt.Sprintf("mark osd %d out", id))
	return err
}

func purgeOSD(connection client.Connection, id int) error {
	// ceph osd crush remove <name>
	command := map[string]interface{}{
		"prefix": "osd crush remove",
		"name":   fmt.Sprintf("osd.%d", id),
	}
	_, err := client.ExecuteMonCommand(connection, command, fmt.Sprintf("remove osd %d from crush map", id))
	if err != nil {
		return fmt.Errorf("failed to remove osd %d from crush map. %v", id, err)
	}

	// ceph auth del osd.$osd_num
	err = client.AuthDelete(connection, fmt.Sprintf("osd.%d", id))
	if err != nil {
		return err
	}

	// ceph osd rm $osd_num
	command = map[string]interface{}{
		"prefix": "osd rm",
		"ids":    []int{id},
	}
	_, err = client.ExecuteMonCommand(connection, command, fmt.Sprintf("rm osd %d", id))
	if err != nil {
		return fmt.Errorf("failed to rm osd %d. %v", id, err)
	}

	return nil
}
