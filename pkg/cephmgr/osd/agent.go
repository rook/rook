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

	"github.com/rook/rook/pkg/cephmgr/client"
	"github.com/rook/rook/pkg/cephmgr/mon"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util"
	"github.com/rook/rook/pkg/util/proc"
)

const (
	osdAgentName    = "osd"
	deviceKey       = "device"
	dirKey          = "dir"
	unassignedOSDID = -1
)

type osdAgent struct {
	cluster        *mon.ClusterInfo
	forceFormat    bool
	location       string
	factory        client.ConnectionFactory
	osdProc        map[int]*proc.MonitoredProc
	desiredDevices []string
	devices        string
	configCounter  int32
	osdsCompleted  chan struct{}
}

func NewAgent(factory client.ConnectionFactory, devices string, forceFormat bool, location string) *osdAgent {
	a := &osdAgent{factory: factory, devices: devices, forceFormat: forceFormat, location: location}
	return a
}

func (a *osdAgent) Name() string {
	return osdAgentName
}

// set the desired state in etcd
func (a *osdAgent) Initialize(context *clusterd.Context) error {

	if len(a.devices) > 0 {
		// add the devices to desired state
		a.desiredDevices = strings.Split(a.devices, ",")
		logger.Infof("desired devices for osds: %+v", a.desiredDevices)
	}

	// if no devices or directories were specified, use the current directory for an osd
	if len(a.desiredDevices) == 0 {
		logger.Infof("Adding local path %s to desired state", context.ConfigDir)
		err := AddDesiredDir(context.EtcdClient, context.ConfigDir, context.NodeID)
		if err != nil {
			return fmt.Errorf("failed to add current dir %s. %v", context.ConfigDir, err)
		}
	}

	return nil
}

func (a *osdAgent) ConfigureLocalService(context *clusterd.Context) error {
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

	// Connect to the ceph cluster
	adminConn, err := mon.ConnectToClusterAsAdmin(context, a.factory, a.cluster)
	if err != nil {
		return err
	}
	defer adminConn.Shutdown()

	if err := a.createDesiredOSDs(adminConn, context); err != nil {
		return err
	}

	return a.stopUndesiredDevices(context, adminConn)
}

// check if osd configured is required at this time
// 1) the node should be marked in the desired state
// 2) osd configuration must not already be in progress from a previous orchestration
func (a *osdAgent) osdConfigRequired(context *clusterd.Context) (bool, error) {
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
func (a *osdAgent) tryStartConfig() bool {
	counter := atomic.AddInt32(&a.configCounter, 1)
	if counter > 1 {
		counter = atomic.AddInt32(&a.configCounter, -1)
		logger.Debugf("osd configuration is already running. counter=%d", counter)
		return false
	}

	return true
}

// increment the config counter when a config step starts
func (a *osdAgent) incrementConfigCounter() {
	atomic.AddInt32(&a.configCounter, 1)
}

// decrement the config counter when a config step is completed.
func (a *osdAgent) decrementConfigCounter() {
	atomic.AddInt32(&a.configCounter, -1)
}

func (a *osdAgent) stopUndesiredDevices(context *clusterd.Context, connection client.Connection) error {
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
	for _, id := range desiredDevices {
		desiredOSDs[id] = nil
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
		err := a.removeOSD(context, connection, appliedOSD)
		if err != nil {
			lastErr = err
		}
	}

	return lastErr
}

func (a *osdAgent) removeOSD(context *clusterd.Context, connection client.Connection, id int) error {

	// mark the OSD as out of the cluster so its data starts to migrate
	err := markOSDOut(connection, id)
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

	err = purgeOSD(connection, id)
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

func (a *osdAgent) DestroyLocalService(context *clusterd.Context) error {
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
func (a *osdAgent) createDesiredOSDs(adminConn client.Connection, context *clusterd.Context) error {
	devices, err := a.loadDesiredDevices(context)
	if err != nil {
		return fmt.Errorf("failed to load desired devices. %v", err)
	}

	dirs, err := loadDesiredDirs(context.EtcdClient, context.NodeID)
	if err != nil {
		return fmt.Errorf("failed to load desired dirs. %v", err)
	}

	// generate and write the OSD bootstrap keyring
	if err := createOSDBootstrapKeyring(adminConn, context.ConfigDir, a.cluster.Name); err != nil {
		return err
	}

	// initialize the desired OSD directories
	err = a.configureDirs(context, dirs)
	if err != nil {
		return err
	}

	return a.configureDevices(context, devices)
}

func (a *osdAgent) configureDirs(context *clusterd.Context, dirs map[string]int) error {
	if len(dirs) == 0 {
		return nil
	}

	// connect to the cluster using the bootstrap-osd creds, this connection will be used for config operations
	connection, err := a.getBoostrapOSDConnection(context)
	if err != nil {
		return fmt.Errorf("failed to connect to cluster to config filestore osds. %+v", err)
	}
	defer connection.Shutdown()

	succeeded := 0
	var lastErr error
	for dir, osdID := range dirs {
		config := &osdConfig{id: osdID}
		err := a.createOrStartOSD(context, connection, config, dir, true)
		if err != nil {
			logger.Errorf("failed to config osd in path %s. %+v", dir, err)
			lastErr = err
		} else {
			succeeded++
		}
	}

	logger.Infof("%d/%d osds (filestore) succeeded on this node", succeeded, len(dirs))
	return lastErr

}

func (a *osdAgent) getBoostrapOSDConnection(context *clusterd.Context) (client.Connection, error) {
	return mon.ConnectToCluster(context, a.factory, a.cluster,
		getBootstrapOSDDir(context.ConfigDir), "bootstrap-osd",
		getBootstrapOSDKeyringPath(context.ConfigDir, a.cluster.Name))
}

func (a *osdAgent) configureDevices(context *clusterd.Context, devices map[string]int) error {
	if len(devices) == 0 {
		return nil
	}

	// reset the signal that the osd config is in progress
	a.osdsCompleted = make(chan struct{})

	// connect to the cluster using the bootstrap-osd creds, this connection will be used for config operations
	connection, err := a.getBoostrapOSDConnection(context)
	if err != nil {
		return fmt.Errorf("failed to connect to cluster to config bluestore osds. %+v", err)
	}

	// asynchronously configure all of the devices with osds
	go func() {
		defer connection.Shutdown()

		// set the signal that the osd config is completed
		defer close(a.osdsCompleted)

		a.incrementConfigCounter()
		defer a.decrementConfigCounter()

		// initialize all the desired OSD volumes
		succeeded := 0
		for device, osdID := range devices {
			config := &osdConfig{id: osdID, deviceName: device, bluestore: true}
			err := a.createOrStartOSD(context, connection, config, context.ConfigDir, false)
			if err != nil {
				logger.Errorf("failed to config osd on device %s. %+v", device, err)
			} else {
				succeeded++
			}
		}

		logger.Infof("%d/%d bluestore osds succeeded on this node", succeeded, len(devices))
	}()

	return nil
}

func (a *osdAgent) createOrStartOSD(context *clusterd.Context, connection client.Connection, config *osdConfig, configRoot string, dir bool) error {
	// create a new OSD in ceph unless already done previously
	if config.id == unassignedOSDID {
		err := registerOSD(connection, config)
		if err != nil {
			return err
		}

		// set the desired state of the dir with the osd id. If a device, we will delay setting this until we have the device uuid
		if dir {
			err = setOSDOnDevice(context.EtcdClient, context.NodeID, configRoot, config.id, dir)
			if err != nil {
				return fmt.Errorf("failed to associate osd id %d with the data dir", config.id)
			}
		}
	}

	newOSD := false
	config.rootPath = path.Join(configRoot, fmt.Sprintf("osd%d", config.id))
	if isOSDDataNotExist(config.rootPath) {
		// consider this a new osd if the "whoami" file is not found
		newOSD = true

		// ensure the config path exists
		if err := os.MkdirAll(config.rootPath, 0744); err != nil {
			return fmt.Errorf("failed to make osd %d config at %s: %+v", config.id, config.rootPath, err)
		}
	}

	if newOSD {
		if config.bluestore {
			// the device needs to be formatted
			err := formatDevice(context, config, a.forceFormat)
			if err != nil {
				return fmt.Errorf("failed device %s. %+v", config.deviceName, err)
			}

			logger.Notice("waiting after bluestore partition/format...")
			<-time.After(2 * time.Second)
		}

		// osd_data_dir/whoami does not exist yet, create/initialize the OSD
		err := initializeOSD(config, a.factory, context, connection, a.cluster, a.location, context.Executor)
		if err != nil {
			return fmt.Errorf("failed to initialize OSD at %s: %+v", config.rootPath, err)
		}

		// save the osd to applied state
		settings := map[string]string{
			"path":      configRoot,
			"disk-uuid": config.diskUUID,
		}
		key := path.Join(getAppliedKey(context.NodeID), fmt.Sprintf("%d", config.id))
		if err := util.StoreEtcdProperties(context.EtcdClient, key, settings); err != nil {
			return fmt.Errorf("failed to mark osd %d as applied: %+v", config.id, err)
		}

	} else {
		// osd_data_dir/whoami already exists, meaning the OSD is already set up.
		// look up some basic information about it so we can run it.
		err := loadOSDInfo(config)
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
func (a *osdAgent) runOSD(context *clusterd.Context, clusterName string, config *osdConfig) error {
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

	if !config.bluestore {
		params = append(params, fmt.Sprintf("--osd-journal=%s", getOSDJournalPath(config.rootPath)))
	}

	process, err := context.ProcMan.Start(
		fmt.Sprintf("osd%d", config.id),
		"osd",
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

// For all applied OSDs, gets a mapping of their osd IDs to their device uuid
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
			if strings.HasSuffix(setting.Key, "/disk-uuid") {
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

func (a *osdAgent) loadDesiredDevices(context *clusterd.Context) (map[string]int, error) {
	// get the device UUID to device name mapping
	uuidToName := map[string]string{}
	for _, disk := range context.Inventory.Local.Disks {
		if disk.UUID != "" {
			uuidToName[disk.UUID] = disk.Name
		}
	}
	logger.Debugf("uuid to name map: %+v", uuidToName)

	// ensure all the desired devices are in the list
	devices := map[string]int{}
	for _, name := range a.desiredDevices {
		if _, ok := devices[name]; !ok {
			// add the device to the desired list
			devices[name] = unassignedOSDID
		}
	}
	logger.Debugf("desired osd id mapping when new: %+v", devices)

	// parse the desired devices from etcd, which are based on the disk uuid.
	key := path.Join(fmt.Sprintf(deviceDesiredKey, context.NodeID))
	devKeys, err := context.EtcdClient.Get(ctx.Background(), key, &etcd.GetOptions{Recursive: true})
	if err != nil {
		if util.IsEtcdKeyNotFound(err) {
			return devices, nil
		}
		return nil, err
	}

	for _, dev := range devKeys.Node.Nodes {
		uuid := util.GetLeafKeyPath(dev.Key)
		osdID := unassignedOSDID

		for _, setting := range dev.Nodes {
			if strings.HasSuffix(setting.Key, "/osd-id") {
				id, err := strconv.Atoi(setting.Value)
				if err == nil {
					logger.Debugf("found osd id %d for disk uuid %s", id, uuid)
					osdID = id
				}
			}
		}

		// translate the disk uuid to the device name
		if name, ok := uuidToName[uuid]; ok {
			devices[name] = osdID
		} else {
			logger.Warningf("did not find name for disk uuid %s", uuid)
		}
	}

	logger.Debugf("final osd id mapping: %+v", devices)

	return devices, nil
}

func setOSDOnDevice(etcdClient etcd.KeysAPI, nodeID, name string, id int, dir bool) error {
	var key string
	if dir {
		key = path.Join(fmt.Sprintf(dirDesiredKey, nodeID), getPseudoDir(name))
	} else {
		key = path.Join(fmt.Sprintf(deviceDesiredKey, nodeID), name)
	}

	_, err := etcdClient.Set(ctx.Background(), path.Join(key, "osd-id"), fmt.Sprintf("%d", id), nil)
	if err != nil {
		return fmt.Errorf("failed to associate osd %d with %s", id, name)
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
			} else if strings.HasSuffix(setting.Key, "/osd-id") {
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
