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
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	ctx "golang.org/x/net/context"

	etcd "github.com/coreos/etcd/client"

	"github.com/rook/rook/pkg/ceph/mon"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util"
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
	unassignedOSDID     = -1
)

type OsdAgent struct {
	cluster            *mon.ClusterInfo
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
	configCounter      int32
	osdsCompleted      chan struct{}
}

func NewAgent(devices string, usingDeviceFilter bool, metadataDevice, directories string, forceFormat bool,
	location string, storeConfig StoreConfig, cluster *mon.ClusterInfo) *OsdAgent {

	return &OsdAgent{devices: devices, usingDeviceFilter: usingDeviceFilter, metadataDevice: metadataDevice,
		directories: directories, forceFormat: forceFormat, location: location, storeConfig: storeConfig, cluster: cluster}
}

func (a *OsdAgent) Name() string {
	return osdAgentName
}

// set the desired state in etcd
func (a *OsdAgent) Initialize(context *clusterd.Context) error {

	if len(a.devices) > 0 {
		// add the devices to desired state
		a.desiredDevices = strings.Split(a.devices, ",")
		logger.Infof("desired devices for osds: %+v.  desired metadata device: %s", a.desiredDevices, a.metadataDevice)
	}

	if len(a.directories) > 0 {
		a.desiredDirectories = strings.Split(a.directories, ",")
	}

	// if no devices or directories were specified, use the current directory for an osd
	if len(a.desiredDevices) == 0 && len(a.desiredDirectories) == 0 {
		logger.Infof("Adding local path %s to desired state", context.ConfigDir)
		a.desiredDirectories = []string{context.ConfigDir}
	}

	if len(a.desiredDirectories) > 0 {
		for _, dir := range a.desiredDirectories {
			err := AddDesiredDir(context.EtcdClient, dir, context.NodeID)
			if err != nil {
				return fmt.Errorf("failed to add desired dir %s. %v", dir, err)
			}
		}
		logger.Infof("desired directories for osds: %+v.", a.desiredDirectories)
	}

	return nil
}

func (a *OsdAgent) ConfigureLocalService(context *clusterd.Context) error {
	required, err := a.osdConfigRequired(context)
	if err != nil {
		return err
	}
	if !required {
		return nil
	}

	// check if osd configuration is already in progress from a previous request
	if !a.tryStartConfig() {
		return nil
	}

	defer a.decrementConfigCounter()

	a.cluster, err = mon.LoadClusterInfo(context.EtcdClient)
	if err != nil {
		return fmt.Errorf("failed to load cluster info: %v", err)
	}
	if a.cluster == nil {
		// the ceph cluster is not initialized yet
		return nil
	}

	if err := a.createDesiredOSDs(context); err != nil {
		return err
	}

	return a.stopUndesiredDevices(context)
}

// check if osd configured is required at this time
// 1) the node should be marked in the desired state
// 2) osd configuration must not already be in progress from a previous orchestration
func (a *OsdAgent) osdConfigRequired(context *clusterd.Context) (bool, error) {
	key := path.Join(mon.CephKey, osdAgentName, clusterd.DesiredKey, context.NodeID, "ready")
	osdsDesired, err := context.EtcdClient.Get(ctx.Background(), key, nil)
	if err != nil {
		if util.IsEtcdKeyNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to get osd desired state. %v", err)
	}

	if osdsDesired.Node.Value != "1" {
		// The osd is not in desired state
		return false, nil
	}

	return true, nil
}

// Try to enter the critical section for configuring osds.
// If a configuration is already in progress, returns false.
// If configuration can be started, returns true.
// The caller of this method must call decrementConfigCounter() if true is returned.
func (a *OsdAgent) tryStartConfig() bool {
	counter := atomic.AddInt32(&a.configCounter, 1)
	if counter > 1 {
		counter = atomic.AddInt32(&a.configCounter, -1)
		logger.Debugf("osd configuration is already running. counter=%d", counter)
		return false
	}

	return true
}

// increment the config counter when a config step starts
func (a *OsdAgent) incrementConfigCounter() {
	atomic.AddInt32(&a.configCounter, 1)
}

// decrement the config counter when a config step is completed.
func (a *OsdAgent) decrementConfigCounter() {
	atomic.AddInt32(&a.configCounter, -1)
}

func (a *OsdAgent) stopUndesiredDevices(context *clusterd.Context) error {
	desiredDevices, err := a.loadDesiredDevices(context)
	if err != nil {
		return fmt.Errorf("failed to load desired devices. %v", err)
	}

	desiredDirs, err := loadDesiredDirs(context.EtcdClient, context.NodeID)
	if err != nil {
		return fmt.Errorf("failed to load desired dirs. %v", err)
	}

	applied, err := GetAppliedOSDs(context.NodeID, context.EtcdClient)
	if err != nil {
		return fmt.Errorf("failed to get applied OSDs. %v", err)
	}

	desiredOSDs := map[int]interface{}{}
	for _, e := range desiredDevices.Entries {
		desiredOSDs[e.Data] = nil
	}
	for _, id := range desiredDirs {
		desiredOSDs[id] = nil
	}

	logger.Debugf("stopUndesiredDevices. applied=%+v, desired=%+v", applied, desiredOSDs)
	var lastErr error
	for appliedOSD := range applied {
		if _, ok := desiredOSDs[appliedOSD]; ok {
			// the osd is both desired and applied
			continue
		}

		logger.Infof("removing osd %d", appliedOSD)
		err := a.removeOSD(context, appliedOSD)
		if err != nil {
			lastErr = err
		}
	}

	return lastErr
}

func (a *OsdAgent) removeOSD(context *clusterd.Context, id int) error {

	// mark the OSD as out of the cluster so its data starts to migrate
	err := markOSDOut(context, a.cluster.Name, id)
	if err != nil {
		return fmt.Errorf("failed to mark out osd %d. %v", id, err)
	}

	// stop the osd process if running
	proc, ok := a.osdProc[id]
	if ok {
		err := proc.Stop()
		if err != nil {
			logger.Errorf("failed to stop osd %d. %v", id, err)
			return err
		}

		delete(a.osdProc, id)
	}

	err = purgeOSD(context, a.cluster.Name, id)
	if err != nil {
		return fmt.Errorf("faild to remove osd %d from crush map. %v", id, err)
	}

	// remove the osd from the applied key
	appliedKey := path.Join(getAppliedKey(context.NodeID), fmt.Sprintf("%d", id))
	_, err = context.EtcdClient.Delete(ctx.Background(), appliedKey, &etcd.DeleteOptions{Recursive: true, Dir: true})
	if err != nil {
		logger.Errorf("failed to remove osd %d from applied state. %v", id, err)
		return err
	}

	logger.Infof("Stopped and removed osd device %d", id)

	return nil
}

func (a *OsdAgent) DestroyLocalService(context *clusterd.Context) error {
	// stop the OSD processes
	for id, proc := range a.osdProc {
		logger.Infof("stopping osd %d", id)
		proc.Stop()
	}

	// clear out the osd procs
	a.osdProc = map[int]*proc.MonitoredProc{}
	return nil
}

func getAppliedKey(nodeID string) string {
	return path.Join(mon.CephKey, osdAgentName, clusterd.AppliedKey, nodeID)
}

// create and initalize OSDs for all the devices specified in the given config
func (a *OsdAgent) createDesiredOSDs(context *clusterd.Context) error {
	devices, err := a.loadDesiredDevices(context)
	if err != nil {
		return fmt.Errorf("failed to load desired devices. %v", err)
	}

	dirs, err := loadDesiredDirs(context.EtcdClient, context.NodeID)
	if err != nil {
		return fmt.Errorf("failed to load desired dirs. %v", err)
	}

	// generate and write the OSD bootstrap keyring
	if err := createOSDBootstrapKeyring(context, a.cluster.Name); err != nil {
		return err
	}

	// initialize the desired OSD directories
	err = a.configureDirs(context, dirs)
	if err != nil {
		return err
	}

	return a.configureDevices(context, devices)
}

func (a *OsdAgent) configureDirs(context *clusterd.Context, dirs map[string]int) error {
	if len(dirs) == 0 {
		return nil
	}

	succeeded := 0
	var lastErr error
	for dirPath, osdID := range dirs {
		config := &osdConfig{id: osdID, configRoot: dirPath, dir: true}

		if config.id == unassignedOSDID {
			// the osd hasn't been registered with ceph yet, do so now to give it a cluster wide ID
			osdID, osdUUID, err := registerOSD(context, a.cluster.Name)
			if err != nil {
				return err
			}

			dirs[dirPath] = *osdID
			config.id = *osdID
			config.uuid = *osdUUID

			// set the desired state of the dir with the osd id
			if context.EtcdClient != nil {
				err = associateOsdIDWithDevice(context.EtcdClient, context.NodeID, dirPath, config.id, config.dir)
				if err != nil {
					return fmt.Errorf("failed to associate osd id %d with the data dir", config.id)
				}
			}
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

	// reset the signal that the osd config is in progress
	a.osdsCompleted = make(chan struct{})

	// asynchronously configure all of the devices with osds
	go func() {
		// set the signal that the osd config is completed
		defer close(a.osdsCompleted)

		a.incrementConfigCounter()
		defer a.decrementConfigCounter()

		// compute an OSD layout scheme that will optimize performance
		scheme, err := a.getPartitionPerfScheme(context, devices)
		logger.Debugf("partition scheme: %+v, err: %+v", scheme, err)
		if err != nil {
			logger.Errorf("failed to get OSD performance scheme: %+v", err)
			return
		}

		if scheme.Metadata != nil {
			// partition the dedicated metadata device
			if err := partitionMetadata(context, scheme.Metadata, context.ConfigDir); err != nil {
				logger.Errorf("failed to partition metadata %+v: %+v", scheme.Metadata, err)
				return
			}
		}

		// initialize and start all the desired OSDs using the computed scheme
		succeeded := 0
		for _, entry := range scheme.Entries {
			config := &osdConfig{id: entry.ID, uuid: entry.OsdUUID, configRoot: context.ConfigDir, partitionScheme: entry}
			err := a.startOSD(context, config)
			if err != nil {
				logger.Errorf("failed to config osd %d. %+v", entry.ID, err)
			} else {
				succeeded++
			}
		}

		logger.Infof("%d/%d osd devices succeeded on this node", succeeded, len(scheme.Entries))
	}()

	return nil
}

// computes a partitioning scheme for all the given desired devices.  This could be devics already in use,
// devices dedicated to metadata, and devices with all bluestore partitions collocated.
func (a *OsdAgent) getPartitionPerfScheme(context *clusterd.Context, devices *DeviceOsdMapping) (*PerfScheme, error) {

	// load the existing (committed) partition scheme from disk
	perfScheme, err := LoadScheme(context.ConfigDir)
	if err != nil {
		return nil, fmt.Errorf("failed to load partition scheme from %s: %+v", context.ConfigDir, err)
	}

	nameToUUID := map[string]string{}
	for _, disk := range context.Inventory.Local.Disks {
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
	newOSD := false
	config.rootPath = path.Join(config.configRoot, fmt.Sprintf("osd%d", config.id))
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
			savedScheme, err := LoadScheme(config.configRoot)
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
		err := initializeOSD(config, context, a.cluster, a.location, a.storeConfig)
		if err != nil {
			return fmt.Errorf("failed to initialize OSD at %s: %+v", config.rootPath, err)
		}

		// save the osd information to applied state
		if err := markOSDAsApplied(context, config); err != nil {
			return fmt.Errorf("failed to mark osd %d as applied: %+v", config.id, err)
		}
	} else {
		// update the osd config file
		err := writeConfigFile(config, context, a.cluster, a.storeConfig)
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

	if !isBluestore(config) {
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

// For all applied OSDs, gets a mapping of their osd IDs to their data device uuid
func GetAppliedOSDs(nodeID string, etcdClient etcd.KeysAPI) (map[int]string, error) {

	osds := map[int]string{}
	key := getAppliedKey(nodeID)
	osdKeys, err := etcdClient.Get(ctx.Background(), key, &etcd.GetOptions{Recursive: true})
	if err != nil {
		if util.IsEtcdKeyNotFound(err) {
			return osds, nil
		}
		return nil, err
	}

	// parse the osds from etcd
	for _, idKey := range osdKeys.Node.Nodes {
		id, err := strconv.Atoi(util.GetLeafKeyPath(idKey.Key))
		if err != nil {
			// skip the unexpected osd id
			continue
		}

		for _, setting := range idKey.Nodes {
			if strings.HasSuffix(setting.Key, "/"+dataDiskUUIDKey) {
				osds[id] = setting.Value
			}
		}
	}

	return osds, nil
}

func getPseudoDir(path string) string {
	// cut off the leading slash so we won't end up with a hidden etcd key
	if strings.HasPrefix(path, "/") {
		path = path[1:]
	}

	return strings.Replace(path, "/", "_", -1)
}

// loads information about all desired devices.  This includes devices that are already committed as well as
// new devices that are desired but have not been set up yet.
func (a *OsdAgent) loadDesiredDevices(context *clusterd.Context) (*DeviceOsdMapping, error) {
	// get the device UUID to device name mapping
	uuidToName := map[string]string{}
	for _, disk := range context.Inventory.Local.Disks {
		if disk.UUID != "" {
			uuidToName[disk.UUID] = disk.Name
		}
	}
	logger.Debugf("uuid to name map: %+v", uuidToName)

	// ensure all the desired devices are in the result list
	deviceOsdMapping := NewDeviceOsdMapping()
	for _, name := range a.desiredDevices {
		if _, ok := deviceOsdMapping.Entries[name]; !ok {
			// add the device to the desired list in an unassigned state
			deviceOsdMapping.Entries[name] = &DeviceOsdIDEntry{Data: unassignedOSDID, Metadata: nil}
		}
	}
	if a.metadataDevice != "" {
		if _, ok := deviceOsdMapping.Entries[a.metadataDevice]; !ok {
			// add the metadata device to the desired list in an unassigned state
			deviceOsdMapping.Entries[a.metadataDevice] = &DeviceOsdIDEntry{Data: unassignedOSDID, Metadata: []int{}}
		}
	}
	logger.Debugf("desired osd id mapping when new: %+v", deviceOsdMapping)

	if context.EtcdClient == nil {
		return deviceOsdMapping, nil
	}

	// parse the desired devices from etcd, which are based on the disk uuid
	key := path.Join(fmt.Sprintf(deviceDesiredKey, context.NodeID))
	devKeys, err := context.EtcdClient.Get(ctx.Background(), key, &etcd.GetOptions{Recursive: true})
	if err != nil {
		if util.IsEtcdKeyNotFound(err) {
			return deviceOsdMapping, nil
		}
		return nil, err
	}

	for _, dev := range devKeys.Node.Nodes {
		uuid := util.GetLeafKeyPath(dev.Key)
		osdDataID := unassignedOSDID
		osdMetadataIDs := []int{}

		// look for OSD data and metadata IDs that are stored on each device
		for _, setting := range dev.Nodes {
			if strings.HasSuffix(setting.Key, "/"+osdIDDataKey) {
				// convert OSD data ID for the current disk from string to int
				id, err := strconv.Atoi(setting.Value)
				if err == nil {
					logger.Debugf("found osd data id %d for disk uuid %s", id, uuid)
					osdDataID = id
				} else {
					logger.Warningf("invalid osd data id %s for disk uuid %s: %+v", setting.Value, uuid, err)
				}
			} else if strings.HasSuffix(setting.Key, "/"+osdIDMetadataKey) {
				// convert OSD metadata ID list for the current disk from strings to ints
				metadataIDStrings := strings.Split(setting.Value, ",")
				osdMetadataIDs = make([]int, len(metadataIDStrings))
				for i, midStr := range metadataIDStrings {
					if mid, err := strconv.Atoi(midStr); err == nil {
						logger.Debugf("found osd metadata id %d for disk uuid %s", mid, uuid)
						osdMetadataIDs[i] = mid
					} else {
						logger.Warningf("invalid osd metadata id %s for disk uuid %s: %+v", midStr, uuid, err)
					}
				}
			}
		}

		// translate the disk uuid to the device name and store its desired OSD data/metadata ID's in the list
		if name, ok := uuidToName[uuid]; ok {
			deviceOsdMapping.Entries[name] = &DeviceOsdIDEntry{Data: osdDataID, Metadata: osdMetadataIDs}
		} else {
			logger.Warningf("did not find name for disk uuid %s", uuid)
		}
	}

	logger.Debugf("final osd id mapping: %+v", deviceOsdMapping)

	return deviceOsdMapping, nil
}

func associateOsdIDWithDevice(etcdClient etcd.KeysAPI, nodeID, name string, id int, dir bool) error {
	var key string
	if dir {
		key = path.Join(fmt.Sprintf(dirDesiredKey, nodeID), getPseudoDir(name))
	} else {
		key = path.Join(fmt.Sprintf(deviceDesiredKey, nodeID), name)
	}

	_, err := etcdClient.Set(ctx.Background(), path.Join(key, osdIDDataKey), fmt.Sprintf("%d", id), nil)
	if err != nil {
		return fmt.Errorf("failed to associate osd %d with %s", id, name)
	}

	return nil
}

func associateOSDIDsWithMetadataDevice(etcdClient etcd.KeysAPI, nodeID, name, idList string) error {
	key := path.Join(fmt.Sprintf(deviceDesiredKey, nodeID), name)
	_, err := etcdClient.Set(ctx.Background(), path.Join(key, osdIDMetadataKey), idList, nil)
	return err
}

func markOSDAsApplied(context *clusterd.Context, config *osdConfig) error {
	if context.EtcdClient == nil {
		return nil
	}

	dataDiskUUID, metadataDiskUUID := "", ""
	if config.partitionScheme != nil {
		dataPartDetails, err := getDataPartitionDetails(config)
		if err != nil {
			return err
		}

		metadataPartDetails, err := getMetadataPartitionDetails(config)
		if err != nil {
			return err
		}

		dataDiskUUID = dataPartDetails.DiskUUID
		metadataDiskUUID = metadataPartDetails.DiskUUID
	}

	settings := map[string]string{
		"path":              config.configRoot,
		dataDiskUUIDKey:     dataDiskUUID,
		metadataDiskUUIDKey: metadataDiskUUID,
	}
	key := path.Join(getAppliedKey(context.NodeID), fmt.Sprintf("%d", config.id))
	if err := util.StoreEtcdProperties(context.EtcdClient, key, settings); err != nil {
		return err
	}

	return nil
}

// add a device to the desired state
func AddDesiredDevice(etcdClient etcd.KeysAPI, nodeID, uuid string, osdID int) error {
	key := path.Join(fmt.Sprintf(deviceDesiredKey, nodeID), uuid)
	err := util.CreateEtcdDir(etcdClient, key)
	if err != nil {
		return fmt.Errorf("failed to add device %s on node %s to desired. %v", uuid, nodeID, err)
	}

	return nil
}

func loadDesiredDirs(etcdClient etcd.KeysAPI, nodeID string) (map[string]int, error) {
	dirs := map[string]int{}
	key := path.Join(fmt.Sprintf(dirDesiredKey, nodeID))
	dirKeys, err := etcdClient.Get(ctx.Background(), key, &etcd.GetOptions{Recursive: true})
	if err != nil {
		if util.IsEtcdKeyNotFound(err) {
			return dirs, nil
		}
		return nil, err
	}

	// parse the dirs from etcd
	for _, dev := range dirKeys.Node.Nodes {
		id := unassignedOSDID
		var path string
		for _, setting := range dev.Nodes {
			if strings.HasSuffix(setting.Key, "/path") {
				path = setting.Value
			} else if strings.HasSuffix(setting.Key, "/"+osdIDDataKey) {
				osdID, err := strconv.Atoi(setting.Value)
				if err == nil {
					id = osdID
				}
			}
		}

		if path != "" {
			dirs[path] = id
		}
	}

	return dirs, nil
}

// add a device to the desired state
func AddDesiredDir(etcdClient etcd.KeysAPI, dir, nodeID string) error {
	key := path.Join(fmt.Sprintf(dirDesiredKey, nodeID), getPseudoDir(dir), "path")
	_, err := etcdClient.Set(ctx.Background(), key, dir, nil)
	if err != nil {
		return fmt.Errorf("failed to add desired dir %s on node %s. %v", dir, nodeID, err)
	}

	return nil
}

// remove a device from the desired state
func RemoveDesiredDevice(etcdClient etcd.KeysAPI, nodeID, uuid string) error {

	key := path.Join(fmt.Sprintf(deviceDesiredKey, nodeID), uuid)
	_, err := etcdClient.Delete(ctx.Background(), key, &etcd.DeleteOptions{Dir: true})
	if err != nil {
		return fmt.Errorf("failed to remove device uuid %s on node %s from desired. %v", uuid, nodeID, err)
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
	return !config.dir && config.partitionScheme != nil && config.partitionScheme.StoreType == Bluestore
}
