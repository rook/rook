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
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	oposd "github.com/rook/rook/pkg/operator/ceph/cluster/osd"
	"github.com/rook/rook/pkg/operator/ceph/cluster/osd/config"
	"github.com/rook/rook/pkg/operator/k8sutil"
	"github.com/rook/rook/pkg/util"
	"github.com/rook/rook/pkg/util/proc"
	"github.com/rook/rook/pkg/util/sys"
)

const (
	osdAgentName    = "osd"
	deviceKey       = "device"
	dirKey          = "dir"
	unassignedOSDID = -1
)

type OsdAgent struct {
	cluster        *cephconfig.ClusterInfo
	nodeName       string
	forceFormat    bool
	location       string
	osdProc        map[int]*proc.MonitoredProc
	devices        []DesiredDevice
	metadataDevice string
	directories    string
	procMan        *proc.ProcManager
	storeConfig    config.StoreConfig
	kv             *k8sutil.ConfigMapKVStore
	configCounter  int32
	osdsCompleted  chan struct{}
}

type device struct {
	name     string
	osdCount int
}

func NewAgent(context *clusterd.Context, devices []DesiredDevice, metadataDevice, directories string, forceFormat bool,
	location string, storeConfig config.StoreConfig, cluster *cephconfig.ClusterInfo, nodeName string, kv *k8sutil.ConfigMapKVStore) *OsdAgent {

	return &OsdAgent{
		devices:        devices,
		metadataDevice: metadataDevice,
		directories:    directories,
		forceFormat:    forceFormat,
		location:       location,
		storeConfig:    storeConfig,
		cluster:        cluster,
		nodeName:       nodeName,
		kv:             kv,
		procMan:        proc.New(context.Executor),
		osdProc:        make(map[int]*proc.MonitoredProc),
	}
}

func (a *OsdAgent) configureDirs(context *clusterd.Context, dirs map[string]int) ([]oposd.OSDInfo, error) {
	var osds []oposd.OSDInfo
	if len(dirs) == 0 {
		return osds, nil
	}

	succeeded := 0
	var lastErr error
	for dirPath, osdID := range dirs {
		config := &osdConfig{id: osdID, configRoot: dirPath, dir: true, storeConfig: a.storeConfig,
			kv: a.kv, storeName: config.GetConfigStoreName(a.nodeName)}

		if config.id == unassignedOSDID {
			// the osd hasn't been registered with ceph yet, do so now to give it a cluster wide ID
			osdID, osdUUID, err := registerOSD(context, a.cluster.Name)
			if err != nil {
				return osds, err
			}

			dirs[dirPath] = *osdID
			config.id = *osdID
			config.uuid = *osdUUID
		}

		osd, err := a.prepareOSD(context, config)
		if err != nil {
			logger.Errorf("failed to config osd in path %s. %+v", dirPath, err)
			lastErr = err
		} else {
			succeeded++
			osds = append(osds, *osd)
		}
	}

	logger.Infof("%d/%d osd dirs succeeded on this node", succeeded, len(dirs))
	return osds, lastErr
}

func (a *OsdAgent) removeDirs(context *clusterd.Context, removedDirs map[string]int) error {
	if len(removedDirs) == 0 {
		return nil
	}

	// walk through each of the OSD directories and remove them
	var failedDirs []string
	for dir, osdID := range removedDirs {
		if err := a.removeOSDConfigDir(dir, osdID); err != nil {
			errMsg := fmt.Sprintf("failed to remove osd.%d. %+v", osdID, err)
			logger.Error(errMsg)
			failedDirs = append(failedDirs, dir)
			continue
		}
	}

	if len(failedDirs) > 0 {
		// at least one OSD failed, return an overall error
		return fmt.Errorf("failed to cleanup directories: %+v", failedDirs)
	}

	return nil
}

func (a *OsdAgent) configureDevices(context *clusterd.Context, devices *DeviceOsdMapping) ([]oposd.OSDInfo, error) {

	cvSupported, err := getCephVolumeSupported(context)
	if err != nil {
		logger.Errorf("failed to detect if ceph-volume is available. %+v", err)
	}
	if a.metadataDevice != "" {
		// ceph-volume still is work in progress for accepting fast devices for the metadata
		logger.Warningf("ceph-volume metadata support is experimental. osd provision might fail if vg on %s does not have enough space", a.metadataDevice)
	}

	var osds []oposd.OSDInfo
	if devices == nil || len(devices.Entries) == 0 {
		logger.Infof("no more devices to configure")
		if cvSupported {
			return getCephVolumeOSDs(context, a.cluster.Name, a.cluster.FSID)
		}
		return osds, nil
	}

	// Detect OSDs provisioned already with legacy rook
	// If ceph-volume is not supported, go ahead and configure the osds natively with rook

	// compute an OSD layout scheme that will optimize performance
	scheme, cvDevices, err := a.getPartitionPerfScheme(context, devices, cvSupported)
	logger.Debugf("partition scheme: %+v, err: %+v", scheme, err)
	if err != nil {
		return osds, fmt.Errorf("failed to get OSD partition scheme: %+v", err)
	}
	if scheme.Metadata != nil {
		// partition the dedicated metadata device
		if err := partitionMetadata(context, scheme.Metadata, a.kv, config.GetConfigStoreName(a.nodeName)); err != nil {
			return osds, fmt.Errorf("failed to partition metadata %+v: %+v", scheme.Metadata, err)
		}
	}
	// initialize and start all the desired OSDs using the computed scheme
	succeeded := 0
	nonCVTotal := len(scheme.Entries)
	for _, entry := range scheme.Entries {
		config := &osdConfig{id: entry.ID, uuid: entry.OsdUUID, configRoot: context.ConfigDir,
			partitionScheme: entry, storeConfig: a.storeConfig, kv: a.kv, storeName: config.GetConfigStoreName(a.nodeName)}
		osd, err := a.prepareOSD(context, config)
		if err != nil {
			return osds, fmt.Errorf("failed to config osd %d. %+v", entry.ID, err)
		}

		succeeded++
		osds = append(osds, *osd)
	}
	logger.Infof("%d/%d pre-ceph-volume osd devices succeeded on this node", succeeded, nonCVTotal)

	if !cvSupported {
		return osds, nil
	}

	// Now ask ceph-volume for osds already configured or to newly configure devices
	cvOSDs, err := a.configureCVDevices(context, cvDevices)
	if err != nil {
		return nil, fmt.Errorf("failed to configure devices with ceph-volume. %+v", err)
	}
	osds = append(osds, cvOSDs...)
	return osds, nil
}

func (a *OsdAgent) removeDevices(context *clusterd.Context, removedDevicesScheme *config.PerfScheme) error {
	if removedDevicesScheme == nil || len(removedDevicesScheme.Entries) == 0 {
		return nil
	}

	var errorMessages []string

	// now start removing each OSD since they should now be running
	for _, entry := range removedDevicesScheme.Entries {

		if err := a.removeOSDConfigDir(context.ConfigDir, entry.ID); err != nil {
			errMsg := fmt.Sprintf("failed to remove osd.%d. %+v", entry.ID, err)
			logger.Error(errMsg)
			errorMessages = append(errorMessages, errMsg)
			continue
		}

		// remove OSD from partition scheme map
		if err := config.RemoveFromScheme(entry, a.kv, config.GetConfigStoreName(a.nodeName)); err != nil {
			errMsg := fmt.Sprintf("failed to remove osd.%d from scheme. %+v", entry.ID, err)
			logger.Error(errMsg)
			errorMessages = append(errorMessages, errMsg)
			continue
		}
	}

	if len(errorMessages) > 0 {
		// at least one OSD failed, return an overall error
		return fmt.Errorf("%s", strings.Join(errorMessages, "\n"))
	}

	return nil
}

// computes a partitioning scheme for all the given desired devices.  This could be devices already in use,
// devices dedicated to metadata, and devices with all bluestore partitions collocated.
func (a *OsdAgent) getPartitionPerfScheme(context *clusterd.Context, devices *DeviceOsdMapping, skipNewDevices bool) (*config.PerfScheme, *DeviceOsdMapping, error) {
	skippedDevices := &DeviceOsdMapping{Entries: map[string]*DeviceOsdIDEntry{}}

	// load the existing (committed) partition scheme from disk
	perfScheme, err := config.LoadScheme(a.kv, config.GetConfigStoreName(a.nodeName))
	if err != nil {
		return nil, skippedDevices, fmt.Errorf("failed to load partition scheme: %+v", err)
	}

	nameToUUID := map[string]string{}
	for _, disk := range context.Devices {
		if disk.UUID != "" {
			nameToUUID[disk.Name] = disk.UUID
		}
	}
	for _, device := range context.Devices {
		logger.Debugf("context.Device: %+v", device)
	}

	numDataNeeded := 0
	var metadataEntry *DeviceOsdIDEntry

	// enumerate the device to OSD mapping to see if we have any new data devices to create and any
	// metadata devices to store their metadata on
	for name, mapping := range devices.Entries {
		if isDeviceInUse(name, nameToUUID, perfScheme) {
			// device is already in use for either data or metadata, update the details for each of its partitions
			// (i.e. device name could have changed)
			logger.Infof("device %s (%s) is already in use", name, nameToUUID)
			refreshDeviceInfo(name, nameToUUID, perfScheme)
		} else if isDeviceDesiredForData(mapping) {
			if skipNewDevices {
				logger.Infof("device %s to be configured by ceph-volume", name)
				skippedDevices.Entries[name] = mapping
			} else {
				// device needs data partitioning
				logger.Infof("configuring device %s (%s) for data", name, nameToUUID)
				numDataNeeded++
			}
		} else if isDeviceDesiredForMetadata(mapping, perfScheme) {
			// device is desired to store metadata for other OSDs
			logger.Infof("configuring device %s (%s) for metadata", name, nameToUUID)
			if perfScheme.Metadata != nil {
				// TODO: this perf scheme creation algorithm assumes either zero or one metadata device, enhance to allow multiple
				// https://github.com/rook/rook/issues/341
				return nil, nil, fmt.Errorf("%s is desired for metadata, but %s (%s) is already the metadata device",
					name, perfScheme.Metadata.Device, perfScheme.Metadata.DiskUUID)
			}

			metadataEntry = mapping
			perfScheme.Metadata = config.NewMetadataDeviceInfo(name)
		}
	}

	if numDataNeeded > 0 {
		// register each data device and compute its desired partition scheme
		for name, mapping := range devices.Entries {
			if !isDeviceDesiredForData(mapping) || isDeviceInUse(name, nameToUUID, perfScheme) {
				continue
			}

			// register/create the OSD with ceph, which will assign it a cluster wide ID
			osdID, osdUUID, err := registerOSD(context, a.cluster.Name)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to register OSD for device %s: %+v", name, err)
			}

			schemeEntry := config.NewPerfSchemeEntry(a.storeConfig.StoreType)
			schemeEntry.ID = *osdID
			schemeEntry.OsdUUID = *osdUUID

			if metadataEntry != nil && perfScheme.Metadata != nil {
				// we have a metadata device, so put the metadata partitions on it and the data partition on its own disk
				metadataEntry.Metadata = append(metadataEntry.Metadata, *osdID)
				mapping.Data = *osdID

				// populate the perf partition scheme entry with distributed partition details
				err := config.PopulateDistributedPerfSchemeEntry(schemeEntry, name, perfScheme.Metadata, a.storeConfig)
				if err != nil {
					return nil, nil, fmt.Errorf("failed to create distributed perf scheme entry for %s: %+v", name, err)
				}
			} else {
				// there is no metadata device to use, store everything on the data device

				// update the device OSD mapping, saying this device will store the current OSDs data and metadata
				mapping.Data = *osdID
				mapping.Metadata = []int{*osdID}

				// populate the perf partition scheme entry with collocated partition details
				err := config.PopulateCollocatedPerfSchemeEntry(schemeEntry, name, a.storeConfig)
				if err != nil {
					return nil, nil, fmt.Errorf("failed to create collocated perf scheme entry for %s: %+v", name, err)
				}
			}

			perfScheme.Entries = append(perfScheme.Entries, schemeEntry)
		}
	}

	return perfScheme, skippedDevices, nil
}

// determines if the given device name is already in use with existing/committed partitions
func isDeviceInUse(name string, nameToUUID map[string]string, scheme *config.PerfScheme) bool {
	parts := findPartitionsForDevice(name, nameToUUID, scheme)
	return len(parts) > 0
}

// determines if the given device OSD mapping is in need of a data partition (and possibly collocated metadata partitions)
func isDeviceDesiredForData(mapping *DeviceOsdIDEntry) bool {
	if mapping == nil {
		return false
	}

	return (mapping.Data == unassignedOSDID && mapping.Metadata == nil) ||
		(mapping.Data > unassignedOSDID && len(mapping.Metadata) == 1)
}

func isDeviceDesiredForMetadata(mapping *DeviceOsdIDEntry, scheme *config.PerfScheme) bool {
	return mapping.Data == unassignedOSDID && mapping.Metadata != nil && len(mapping.Metadata) == 0
}

// finds all the partition details that are on the given device name
func findPartitionsForDevice(name string, nameToUUID map[string]string, scheme *config.PerfScheme) []*config.PerfSchemePartitionDetails {
	if scheme == nil {
		return nil
	}

	diskUUID, ok := nameToUUID[name]
	if !ok {
		return nil
	}

	parts := []*config.PerfSchemePartitionDetails{}
	for _, e := range scheme.Entries {
		for _, p := range e.Partitions {
			if p.DiskUUID == diskUUID {
				parts = append(parts, p)
			}
		}
	}

	return parts
}

// if a device name has changed, this function will find all partition entries with the device's static UUID and
// then update the device name on them
func refreshDeviceInfo(name string, nameToUUID map[string]string, scheme *config.PerfScheme) {
	parts := findPartitionsForDevice(name, nameToUUID, scheme)
	if len(parts) == 0 {
		return
	}

	// make sure each partition that is using the given device has its most up to date name
	for _, p := range parts {
		p.Device = name
	}

	// also update the device name if the given device is in use as the metadata device
	if scheme.Metadata != nil {
		if diskUUID, ok := nameToUUID[name]; ok {
			if scheme.Metadata.DiskUUID == diskUUID {
				scheme.Metadata.Device = name
			}
		}
	}
}

func (a *OsdAgent) prepareOSD(context *clusterd.Context, cfg *osdConfig) (*oposd.OSDInfo, error) {
	cfg.rootPath = getOSDRootDir(cfg.configRoot, cfg.id)

	// if the osd is using filestore on a device and it's previously been formatted/partitioned,
	// go ahead and remount the device now.
	devPartInfo, err := remountFilestoreDeviceIfNeeded(context, cfg)
	if err != nil {
		return nil, err
	}

	// prepare the osd root dir, which will tell us if it's a new osd
	newOSD, err := prepareOSDRoot(cfg)
	if err != nil {
		return nil, err
	}

	if newOSD {
		if cfg.partitionScheme != nil {
			// format and partition the device if needed
			savedScheme, err := config.LoadScheme(a.kv, config.GetConfigStoreName(a.nodeName))
			if err != nil {
				return nil, fmt.Errorf("failed to load the saved partition scheme from %s: %+v", cfg.configRoot, err)
			}

			skipFormat := false
			for _, savedEntry := range savedScheme.Entries {
				if savedEntry.ID == cfg.id {
					// this OSD has already had its partitions created, skip formatting
					skipFormat = true
					break
				}
			}

			if !skipFormat {
				devPartInfo, err = formatDevice(context, cfg, a.forceFormat, a.storeConfig)
				if err != nil {
					return nil, fmt.Errorf("failed format/partition of osd %d. %+v", cfg.id, err)
				}

				logger.Notice("waiting after partition/format...")
				<-time.After(2 * time.Second)
			}
		}

		// osd_data_dir/ready does not exist yet, create/initialize the OSD
		err := initializeOSD(cfg, context, a.cluster, a.location)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize OSD at %s: %+v", cfg.rootPath, err)
		}
	} else {
		// update the osd config file
		err := writeConfigFile(cfg, context, a.cluster, a.location)
		if err != nil {
			logger.Warningf("failed to update config file. %+v", err)
		}

		// osd_data_dir/ready already exists, meaning the OSD is already set up.
		// look up some basic information about it so we can run it.
		err = loadOSDInfo(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to get OSD information from %s: %+v", cfg.rootPath, err)
		}
	}
	osdInfo := getOSDInfo(a.cluster.Name, cfg, devPartInfo)
	logger.Infof("completed preparing osd %+v", osdInfo)

	if devPartInfo != nil {
		sys.UnmountDevice(devPartInfo.pathToUnmount, context.Executor)
	}

	return osdInfo, nil
}

func prepareOSDRoot(cfg *osdConfig) (newOSD bool, err error) {
	newOSD = isOSDDataNotExist(cfg.rootPath)
	if !newOSD {
		// osd is not new (it's ready), nothing to prepare
		logger.Infof("osd with path %s is not new, nothing to prepare", cfg.rootPath)
		return newOSD, nil
	}

	// osd is new (it's not ready), make sure there is no stale state in the OSD dir by deleting the entire thing
	logger.Infof("osd.%d appears to be new, cleaning the root dir at %s", cfg.id, cfg.rootPath)
	if err := os.RemoveAll(cfg.rootPath); err != nil {
		logger.Warningf("failed to clean osd.%d root dir at %s, will proceed with starting osd: %+v", cfg.id, cfg.rootPath, err)
	}

	// prepare the osd dir by creating it now
	if err := os.MkdirAll(cfg.rootPath, 0744); err != nil {
		return newOSD, fmt.Errorf("failed to make osd.%d config at %s: %+v", cfg.id, cfg.rootPath, err)
	}

	return newOSD, nil
}

func getOSDInfo(clusterName string, config *osdConfig, devPartInfo *devicePartInfo) *oposd.OSDInfo {
	confFile := getOSDConfFilePath(config.rootPath, clusterName)
	util.WriteFileToLog(logger, confFile)
	osd := &oposd.OSDInfo{
		ID:          config.id,
		DataPath:    config.rootPath,
		Config:      confFile,
		Cluster:     clusterName,
		KeyringPath: getOSDKeyringPath(config.rootPath),
		UUID:        config.uuid.String(),
		IsFileStore: isFilestore(config),
		IsDirectory: config.dir,
	}
	if devPartInfo != nil {
		osd.DevicePartUUID = devPartInfo.deviceUUID
	}

	if isFilestore(config) {
		osd.Journal = getOSDJournalPath(config.rootPath)
	}
	return osd
}

func (a *OsdAgent) removeOSDConfigDir(configRoot string, id int) error {
	// delete the OSD's local storage
	osdRootDir := getOSDRootDir(configRoot, id)
	logger.Infof("deleting osd dir: %s", osdRootDir)
	if err := os.RemoveAll(osdRootDir); err != nil {
		logger.Warningf("failed to delete osd.%d root dir from %s, it may need to be cleaned up manually: %+v",
			id, osdRootDir, err)
	}

	return nil
}

func isOSDDataNotExist(osdDataPath string) bool {
	_, err := os.Stat(filepath.Join(osdDataPath, "ready"))
	return os.IsNotExist(err)
}

func loadOSDInfo(config *osdConfig) error {
	idFile := filepath.Join(config.rootPath, "whoami")
	idContent, err := ioutil.ReadFile(idFile)
	if err != nil {
		return fmt.Errorf("failed to read OSD ID from %s: %+v", idFile, err)
	}

	osdID, err := strconv.Atoi(strings.TrimSpace(string(idContent[:])))
	if err != nil {
		return fmt.Errorf("failed to parse OSD ID from %s with content %s: %+v", idFile, idContent, err)
	}

	uuidFile := filepath.Join(config.rootPath, "fsid")
	fsidContent, err := ioutil.ReadFile(uuidFile)
	if err != nil {
		return fmt.Errorf("failed to read UUID from %s: %+v", uuidFile, err)
	}

	osdUUID, err := uuid.Parse(strings.TrimSpace(string(fsidContent[:])))
	if err != nil {
		return fmt.Errorf("failed to parse UUID from %s with content %s: %+v", uuidFile, string(fsidContent[:]), err)
	}

	config.id = osdID
	config.uuid = osdUUID
	return nil
}

func isBluestore(config *osdConfig) bool {
	return isBluestoreDevice(config) || isBluestoreDir(config)
}

func isBluestoreDevice(cfg *osdConfig) bool {
	// A device will use bluestore unless explicitly requested to be filestore (the default is blank)
	return !cfg.dir && cfg.partitionScheme != nil && cfg.partitionScheme.StoreType != config.Filestore
}

func isBluestoreDir(cfg *osdConfig) bool {
	// A dir will use filestore unless explicitly requested to be bluestore
	return cfg.dir && cfg.storeConfig.StoreType == config.Bluestore
}

func isFilestore(cfg *osdConfig) bool {
	return isFilestoreDevice(cfg) || isFilestoreDir(cfg)
}

func isFilestoreDevice(cfg *osdConfig) bool {
	// A device will use bluestore unless explicitly requested to be filestore (the default is blank)
	return !cfg.dir && cfg.partitionScheme != nil && cfg.partitionScheme.StoreType == config.Filestore
}

func isFilestoreDir(cfg *osdConfig) bool {
	// A dir will use filestore unless explicitly requested to be bluestore (the default is blank)
	return cfg.dir && cfg.storeConfig.StoreType != config.Bluestore
}
