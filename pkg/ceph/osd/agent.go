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
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/rook/rook/pkg/ceph/mon"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util"
	"github.com/rook/rook/pkg/util/kvstore"
	"github.com/rook/rook/pkg/util/proc"
)

const (
	osdAgentName        = "osd"
	deviceKey           = "device"
	dirKey              = "dir"
	osdIDDataKey        = "osd-id-data"
	osdIDMetadataKey    = "osd-id-metadata"
	dataDiskUUIDKey     = "data-disk-uuid"
	metadataDiskUUIDKey = "metadata-disk-uuid"
	configStoreNameFmt  = "rook-ceph-osd-%s-config"
	unassignedOSDID     = -1
)

type OsdAgent struct {
	cluster            *mon.ClusterInfo
	nodeName           string
	forceFormat        bool
	location           string
	osdProc            map[int]*proc.MonitoredProc
	desiredDevices     []string
	desiredDirectories []string
	devices            string
	usingDeviceFilter  bool
	metadataDevice     string
	directories        string
	storeConfig        StoreConfig
	kv                 kvstore.KeyValueStore
	configCounter      int32
	osdsCompleted      chan struct{}
}

func NewAgent(devices string, usingDeviceFilter bool, metadataDevice, directories string, forceFormat bool,
	location string, storeConfig StoreConfig, cluster *mon.ClusterInfo, nodeName string, kv kvstore.KeyValueStore) *OsdAgent {

	return &OsdAgent{devices: devices, usingDeviceFilter: usingDeviceFilter, metadataDevice: metadataDevice,
		directories: directories, forceFormat: forceFormat, location: location, storeConfig: storeConfig,
		cluster: cluster, nodeName: nodeName, kv: kv,
	}
}

func (a *OsdAgent) configureDirs(context *clusterd.Context, dirs map[string]int) error {
	if len(dirs) == 0 {
		return nil
	}

	succeeded := 0
	var lastErr error
	for dirPath, osdID := range dirs {
		config := &osdConfig{id: osdID, configRoot: dirPath, dir: true, storeConfig: a.storeConfig,
			kv: a.kv, storeName: getConfigStoreName(a.nodeName)}

		if config.id == unassignedOSDID {
			// the osd hasn't been registered with ceph yet, do so now to give it a cluster wide ID
			osdID, osdUUID, err := registerOSD(context, a.cluster.Name)
			if err != nil {
				return err
			}

			dirs[dirPath] = *osdID
			config.id = *osdID
			config.uuid = *osdUUID
		}

		err := a.startOSD(context, config)
		if err != nil {
			logger.Errorf("failed to config osd in path %s. %+v", dirPath, err)
			lastErr = err
		} else {
			succeeded++
		}
	}

	logger.Infof("%d/%d osd dirs succeeded on this node", succeeded, len(dirs))
	return lastErr

}

func (a *OsdAgent) configureDevices(context *clusterd.Context, devices *DeviceOsdMapping) error {
	if devices == nil || len(devices.Entries) == 0 {
		return nil
	}

	// compute an OSD layout scheme that will optimize performance
	scheme, err := a.getPartitionPerfScheme(context, devices)
	logger.Debugf("partition scheme: %+v, err: %+v", scheme, err)
	if err != nil {
		return fmt.Errorf("failed to get OSD partition scheme: %+v", err)
	}

	if scheme.Metadata != nil {
		// partition the dedicated metadata device
		if err := partitionMetadata(context, scheme.Metadata, a.kv, getConfigStoreName(a.nodeName)); err != nil {
			return fmt.Errorf("failed to partition metadata %+v: %+v", scheme.Metadata, err)
		}
	}

	// initialize and start all the desired OSDs using the computed scheme
	succeeded := 0
	for _, entry := range scheme.Entries {
		config := &osdConfig{id: entry.ID, uuid: entry.OsdUUID, configRoot: context.ConfigDir,
			partitionScheme: entry, storeConfig: a.storeConfig, kv: a.kv, storeName: getConfigStoreName(a.nodeName)}
		err := a.startOSD(context, config)
		if err != nil {
			return fmt.Errorf("failed to config osd %d. %+v", entry.ID, err)
		} else {
			succeeded++
		}
	}

	logger.Infof("%d/%d osd devices succeeded on this node", succeeded, len(scheme.Entries))
	return nil
}

// computes a partitioning scheme for all the given desired devices.  This could be devics already in use,
// devices dedicated to metadata, and devices with all bluestore partitions collocated.
func (a *OsdAgent) getPartitionPerfScheme(context *clusterd.Context, devices *DeviceOsdMapping) (*PerfScheme, error) {

	// load the existing (committed) partition scheme from disk
	perfScheme, err := LoadScheme(a.kv, getConfigStoreName(a.nodeName))
	if err != nil {
		return nil, fmt.Errorf("failed to load partition scheme: %+v", err)
	}

	nameToUUID := map[string]string{}
	for _, disk := range context.Devices {
		if disk.UUID != "" {
			nameToUUID[disk.Name] = disk.UUID
		}
	}

	numDataNeeded := 0
	var metadataEntry *DeviceOsdIDEntry

	// enumerate the device to OSD mapping to see if we have any new data devices to create and any
	// metadata devices to store their metadata on
	for name, mapping := range devices.Entries {
		if isDeviceInUse(name, nameToUUID, perfScheme) {
			// device is already in use for either data or metadata, update the details for each of its partitions
			// (i.e. device name could have changed)
			refreshDeviceInfo(name, nameToUUID, perfScheme)
		} else if isDeviceDesiredForData(mapping) {
			// device needs data partitioning
			numDataNeeded++
		} else if isDeviceDesiredForMetadata(mapping, perfScheme) {
			// device is desired to store metadata for other OSDs
			if perfScheme.Metadata != nil {
				// TODO: this perf scheme creation algorithm assumes either zero or one metadata device, enhance to allow multiple
				// https://github.com/rook/rook/issues/341
				return nil, fmt.Errorf("%s is desired for metadata, but %s (%s) is already the metadata device",
					name, perfScheme.Metadata.Device, perfScheme.Metadata.DiskUUID)
			}

			metadataEntry = mapping
			perfScheme.Metadata = NewMetadataDeviceInfo(name)
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
				return nil, fmt.Errorf("failed to register OSD for device %s: %+v", name, err)
			}

			schemeEntry := NewPerfSchemeEntry(a.storeConfig.StoreType)
			schemeEntry.ID = *osdID
			schemeEntry.OsdUUID = *osdUUID

			if metadataEntry != nil && perfScheme.Metadata != nil {
				// we have a metadata device, so put the metadata partitions on it and the data partition on its own disk
				metadataEntry.Metadata = append(metadataEntry.Metadata, *osdID)
				mapping.Data = *osdID

				// populate the perf partition scheme entry with distributed partition details
				err := PopulateDistributedPerfSchemeEntry(schemeEntry, name, perfScheme.Metadata, a.storeConfig)
				if err != nil {
					return nil, fmt.Errorf("failed to create distributed perf scheme entry for %s: %+v", name, err)
				}
			} else {
				// there is no metadata device to use, store everything on the data device

				// update the device OSD mapping, saying this device will store the current OSDs data and metadata
				mapping.Data = *osdID
				mapping.Metadata = []int{*osdID}

				// populate the perf partition scheme entry with collocated partition details
				err := PopulateCollocatedPerfSchemeEntry(schemeEntry, name, a.storeConfig)
				if err != nil {
					return nil, fmt.Errorf("failed to create collocated perf scheme entry for %s: %+v", name, err)
				}
			}

			perfScheme.Entries = append(perfScheme.Entries, schemeEntry)
		}
	}

	return perfScheme, nil
}

// determines if the given device name is already in use with existing/committed partitions
func isDeviceInUse(name string, nameToUUID map[string]string, scheme *PerfScheme) bool {
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

func isDeviceDesiredForMetadata(mapping *DeviceOsdIDEntry, scheme *PerfScheme) bool {
	return mapping.Data == unassignedOSDID && mapping.Metadata != nil && len(mapping.Metadata) == 0
}

// finds all the partition details that are on the given device name
func findPartitionsForDevice(name string, nameToUUID map[string]string, scheme *PerfScheme) []*PerfSchemePartitionDetails {
	if scheme == nil {
		return nil
	}

	diskUUID, ok := nameToUUID[name]
	if !ok {
		return nil
	}

	parts := []*PerfSchemePartitionDetails{}
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
func refreshDeviceInfo(name string, nameToUUID map[string]string, scheme *PerfScheme) {
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

func (a *OsdAgent) startOSD(context *clusterd.Context, config *osdConfig) error {

	config.rootPath = path.Join(config.configRoot, fmt.Sprintf("osd%d", config.id))

	// if the osd is using filestore on a device and it's previously been formatted/partitioned,
	// go ahead and remount the device now.
	if err := remountFilestoreDeviceIfNeeded(context, config); err != nil {
		return err
	}

	newOSD := false
	if isOSDDataNotExist(config.rootPath) {
		// consider this a new osd if the "whoami" file is not found
		newOSD = true

		// ensure the config path exists
		if err := os.MkdirAll(config.rootPath, 0744); err != nil {
			return fmt.Errorf("failed to make osd %d config at %s: %+v", config.id, config.rootPath, err)
		}
	}

	if newOSD {
		if config.partitionScheme != nil {
			// format and partition the device if needed
			savedScheme, err := LoadScheme(a.kv, getConfigStoreName(a.nodeName))
			if err != nil {
				return fmt.Errorf("failed to load the saved partition scheme from %s: %+v", config.configRoot, err)
			}

			skipFormat := false
			for _, savedEntry := range savedScheme.Entries {
				if savedEntry.ID == config.id {
					// this OSD has already had its partitions created, skip formatting
					skipFormat = true
					break
				}
			}

			if !skipFormat {
				err = formatDevice(context, config, a.forceFormat, a.storeConfig)
				if err != nil {
					return fmt.Errorf("failed format/partition of osd %d. %+v", config.id, err)
				}

				logger.Notice("waiting after partition/format...")
				<-time.After(2 * time.Second)
			}
		}

		// osd_data_dir/whoami does not exist yet, create/initialize the OSD
		err := initializeOSD(config, context, a.cluster, a.location)
		if err != nil {
			return fmt.Errorf("failed to initialize OSD at %s: %+v", config.rootPath, err)
		}
	} else {
		// update the osd config file
		err := writeConfigFile(config, context, a.cluster)
		if err != nil {
			logger.Warningf("failed to update config file. %+v", err)
		}

		// osd_data_dir/whoami already exists, meaning the OSD is already set up.
		// look up some basic information about it so we can run it.
		err = loadOSDInfo(config)
		if err != nil {
			return fmt.Errorf("failed to get OSD information from %s: %+v", config.rootPath, err)
		}
	}

	// run the OSD in a child process now that it is fully initialized and ready to go
	err := a.runOSD(context, a.cluster.Name, config)
	if err != nil {
		return fmt.Errorf("failed to run osd %d: %+v", config.id, err)
	}

	return nil
}

// runs an OSD with the given config in a child process
func (a *OsdAgent) runOSD(context *clusterd.Context, clusterName string, config *osdConfig) error {
	// start the OSD daemon in the foreground with the given config
	logger.Infof("starting osd %d at %s", config.id, config.rootPath)

	confFile := getOSDConfFilePath(config.rootPath, clusterName)
	util.WriteFileToLog(logger, confFile)

	osdUUIDArg := fmt.Sprintf("--osd-uuid=%s", config.uuid.String())
	params := []string{"--foreground",
		fmt.Sprintf("--id=%d", config.id),
		fmt.Sprintf("--cluster=%s", clusterName),
		fmt.Sprintf("--osd-data=%s", config.rootPath),
		fmt.Sprintf("--conf=%s", confFile),
		fmt.Sprintf("--keyring=%s", getOSDKeyringPath(config.rootPath)),
		osdUUIDArg,
	}

	if isFilestore(config) {
		params = append(params, fmt.Sprintf("--osd-journal=%s", getOSDJournalPath(config.rootPath)))
	}

	process, err := context.ProcMan.Start(
		fmt.Sprintf("osd%d", config.id),
		"ceph-osd",
		regexp.QuoteMeta(osdUUIDArg),
		proc.ReuseExisting,
		params...)
	if err != nil {
		return fmt.Errorf("failed to start osd %d: %+v", config.id, err)
	}

	if a.osdProc == nil {
		// initialize the osd map
		a.osdProc = make(map[int]*proc.MonitoredProc)
	}
	if process != nil {
		// if the process was already running Start will return nil in which case we don't want to overwrite it
		a.osdProc[config.id] = process
	}

	return nil
}

func isOSDDataNotExist(osdDataPath string) bool {
	_, err := os.Stat(filepath.Join(osdDataPath, "whoami"))
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

func isBluestoreDevice(config *osdConfig) bool {
	return !config.dir && config.partitionScheme != nil && config.partitionScheme.StoreType == Bluestore
}

func isBluestoreDir(config *osdConfig) bool {
	return config.dir && config.storeConfig.StoreType == Bluestore
}

func isFilestore(config *osdConfig) bool {
	return isFilestoreDevice(config) || isFilestoreDir(config)
}

func isFilestoreDevice(config *osdConfig) bool {
	return !config.dir && config.partitionScheme != nil && config.partitionScheme.StoreType == Filestore
}

func isFilestoreDir(config *osdConfig) bool {
	return config.dir && config.storeConfig.StoreType == Filestore
}

func getConfigStoreName(nodeName string) string {
	return fmt.Sprintf(configStoreNameFmt, nodeName)
}
