package cephmgr

import (
	"fmt"
	"log"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"

	ctx "golang.org/x/net/context"

	etcd "github.com/coreos/etcd/client"
	"github.com/google/uuid"

	"github.com/quantum/castle/pkg/cephmgr/client"
	"github.com/quantum/castle/pkg/clusterd"
	"github.com/quantum/castle/pkg/util"
	"github.com/quantum/castle/pkg/util/proc"
)

const (
	osdAgentName = "osd"
	deviceKey    = "device"
	dirKey       = "dir"
)

type osdAgent struct {
	cluster     *ClusterInfo
	forceFormat bool
	location    string
	factory     client.ConnectionFactory
	osdProc     map[string]*proc.MonitoredProc
	devices     string
}

type osdInfo struct {
	id     int
	serial string
	dir    bool
}

type osdConfig struct {
	name      string
	rootPath  string
	id        int
	uuid      uuid.UUID
	bluestore bool
}

func newOSDAgent(factory client.ConnectionFactory, devices string, forceFormat bool, location string) *osdAgent {
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
		devices := strings.Split(a.devices, ",")
		for _, device := range devices {
			log.Printf("Adding device %s to desired state", device)
			err := AddDesiredDevice(context.EtcdClient, device, context.NodeID)
			if err != nil {
				return fmt.Errorf("failed to add desired device %s. %v", device, err)
			}
		}
	}

	// if no devices or directories were specified, use the current directory for an osd
	if len(a.devices) == 0 {
		log.Printf("Adding local path to local directory %s", context.ConfigDir)
		err := AddDesiredDir(context.EtcdClient, context.ConfigDir, context.NodeID)
		if err != nil {
			return fmt.Errorf("failed to add current dir %s. %v", context.ConfigDir, err)
		}
	}

	return nil
}

func (a *osdAgent) ConfigureLocalService(context *clusterd.Context) error {
	// check if the osd is in the desired state for this node
	key := path.Join(cephKey, osdAgentName, desiredKey, context.NodeID, "ready")
	osdDesired, err := context.EtcdClient.Get(ctx.Background(), key, nil)
	if err != nil {
		if util.IsEtcdKeyNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get osd desired state. %v", err)
	}

	if osdDesired.Node.Value != "1" {
		// The osd is not in desired state
		return nil
	}

	a.cluster, err = LoadClusterInfo(context.EtcdClient)
	if err != nil {
		return fmt.Errorf("failed to load cluster info: %v", err)
	}
	if a.cluster == nil {
		// the ceph cluster is not initialized yet
		return nil
	}

	// Connect to the ceph cluster
	adminConn, err := ConnectToClusterAsAdmin(context, a.factory, a.cluster)
	if err != nil {
		return err
	}
	defer adminConn.Shutdown()

	devices, err := loadDesiredDevices(context.EtcdClient, context.NodeID)
	if err != nil {
		return fmt.Errorf("failed to load desired devices. %v", err)
	}

	dirs, err := loadDesiredDirs(context.EtcdClient, context.NodeID)
	if err != nil {
		return fmt.Errorf("failed to load desired dirs. %v", err)
	}

	if err := a.startDesiredDevices(context, adminConn, devices, dirs); err != nil {
		return err
	}

	return a.stopUndesiredDevices(context, adminConn, devices)
}

func (a *osdAgent) startDesiredDevices(context *clusterd.Context, connection client.Connection, devices map[string]string, dirs *util.Set) error {

	// Start an OSD for each of the specified devices
	deviceMap, err := a.createOSDs(connection, context, devices, dirs)
	if err != nil {
		return fmt.Errorf("failed to create OSDs: %+v", err)
	}

	// successful, ensure all applied devices are saved to the cluster config store
	if err := a.saveAppliedOSDs(context, deviceMap); err != nil {
		return fmt.Errorf("failed to save applied OSDs: %+v", err)
	}

	return nil
}

func (a *osdAgent) stopUndesiredDevices(context *clusterd.Context, connection client.Connection, desiredDevices map[string]string) error {
	applied, err := GetAppliedOSDs(context.NodeID, context.EtcdClient)
	if err != nil {
		return fmt.Errorf("failed to get applied OSDs. %v", err)
	}

	desiredSerial := util.NewSet()
	for _, serial := range desiredDevices {
		desiredSerial.Add(serial)
	}

	var lastErr error
	for appliedSerial := range applied.Iter() {
		if desiredSerial.Contains(appliedSerial) {
			// the osd is desired and applied
			continue
		}

		err := a.removeOSD(context, connection, appliedSerial)
		if err != nil {
			lastErr = err
		}
	}

	return lastErr
}

func getIDFromSerial(context *clusterd.Context, serial string) (int, error) {
	key := path.Join(getAppliedKey(context.NodeID), deviceKey, serial, "id")
	val, err := context.EtcdClient.Get(ctx.Background(), key, nil)
	if err != nil {
		return -1, fmt.Errorf("failed to get device id from serial %s. %v", serial, err)
	}

	id, err := strconv.ParseInt(val.Node.Value, 10, 32)
	if err != nil {
		return -1, fmt.Errorf("failed to get parse id from serial %s. %v", serial, err)
	}

	return int(id), nil
}

func (a *osdAgent) removeOSD(context *clusterd.Context, connection client.Connection, name string) error {
	osdID, err := getIDFromSerial(context, name)
	if err != nil {
		return fmt.Errorf("failed to get osd %s id. %v", name, err)
	}

	// mark the OSD as out of the cluster so its data starts to migrate
	err = markOSDOut(connection, osdID)
	if err != nil {
		return fmt.Errorf("failed to mark out osd %s (%d). %v", name, osdID, err)
	}

	// stop the osd process if running
	proc, ok := a.osdProc[name]
	if ok {
		err := proc.Stop()
		if err != nil {
			log.Printf("failed to stop osd for device %s. %v", name, err)
			return err
		}

		delete(a.osdProc, name)
	}

	err = purgeOSD(connection, name, osdID)
	if err != nil {
		return fmt.Errorf("faild to remove osd %s from crush map. %v", name, err)
	}

	// remove the osd from the applied key
	appliedKey := path.Join(getAppliedKey(context.NodeID), deviceKey, name)
	_, err = context.EtcdClient.Delete(ctx.Background(), appliedKey, &etcd.DeleteOptions{Recursive: true, Dir: true})
	if err != nil {
		log.Printf("failed to remove device %s from desired state. %v", name, err)
		return err
	}

	log.Printf("Stopped and removed osd device %s", name)

	return nil
}

func (a *osdAgent) DestroyLocalService(context *clusterd.Context) error {
	// stop the OSD processes
	for device, proc := range a.osdProc {
		log.Printf("stopping osd on device %s", device)
		proc.Stop()
	}

	// clear out the osd procs
	a.osdProc = map[string]*proc.MonitoredProc{}
	return nil
}

func getAppliedKey(nodeID string) string {
	return path.Join(cephKey, osdAgentName, appliedKey, nodeID)
}

// create and initalize OSDs for all the devices specified in the given config
func (a *osdAgent) createOSDs(adminConn client.Connection, context *clusterd.Context, devices map[string]string, dirs *util.Set) (map[string]*osdInfo, error) {
	// generate and write the OSD bootstrap keyring
	if err := createOSDBootstrapKeyring(adminConn, context.ConfigDir, a.cluster.Name); err != nil {
		return nil, err
	}

	// connect to the cluster using the bootstrap-osd creds, this connection will be used for config operations
	bootstrapConn, err := connectToCluster(context, a.factory, a.cluster, getBootstrapOSDDir(context.ConfigDir), "bootstrap-osd", getBootstrapOSDKeyringPath(context.ConfigDir, a.cluster.Name))
	if err != nil {
		return nil, err
	}
	defer bootstrapConn.Shutdown()

	deviceMap := map[string]*osdInfo{}
	for dir := range dirs.Iter() {
		config, err := a.createLocalOSD(bootstrapConn, dir)
		if err != nil {
			return nil, fmt.Errorf("failed to create local OSD: %+v", err)
		}

		err = a.createOrStartOSD(context, bootstrapConn, config)
		if err != nil {
			return nil, fmt.Errorf("failed to start osd on device %s. %v", dir, err)
		}

		deviceMap[dir] = &osdInfo{id: config.id, dir: true}
	}

	// initialize all the desired OSD volumes
	for device, serial := range devices {
		config, err := a.prepareDeviceForOSD(device, serial, bootstrapConn, context)
		if err != nil {
			return nil, fmt.Errorf("failed to create mounted OSD on %s: %+v", device, err)
		}

		err = a.createOrStartOSD(context, bootstrapConn, config)
		if err != nil {
			return nil, fmt.Errorf("failed to start osd on device %s. %v", device, err)
		}

		deviceMap[device] = &osdInfo{id: config.id, serial: serial}
	}

	return deviceMap, nil
}

func (a *osdAgent) createOrStartOSD(context *clusterd.Context, connection client.Connection, config *osdConfig) error {

	if isOSDDataNotExist(config.rootPath) {

		// osd_data_dir/whoami does not exist yet, create/initialize the OSD
		err := initializeOSD(config, a.factory, context, connection, a.cluster, a.location, context.Executor)
		if err != nil {
			return fmt.Errorf("failed to initialize OSD at %s: %+v", config.rootPath, err)
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

func (a *osdAgent) prepareDeviceForOSD(device, serial string, bootstrapConn client.Connection, context *clusterd.Context) (*osdConfig, error) {
	var osdID int
	var osdUUID uuid.UUID

	configDir := getOSDConfigDir(context.ConfigDir, serial)
	if isOSDDataNotExist(configDir) {
		// the device needs to be formatted
		err := formatDevice(context, device, serial, configDir, a.forceFormat)
		if err != nil {
			// attempting to format the volume failed, bail out with error
			return nil, err
		}

		// register the OSD with the cluster now to get an official ID
		osdID, osdUUID, err = registerOSDWithCluster(device, bootstrapConn)
		if err != nil {
			return nil, fmt.Errorf("failed to regiser OSD %d with cluster: %+v", osdID, err)
		}

		if err := os.MkdirAll(configDir, 0744); err != nil {
			return nil, fmt.Errorf("failed to create config dir at %s: %+v", configDir, err)
		}
	}

	// configure devices with bluestore, and local directories with filestore
	return &osdConfig{rootPath: configDir, id: osdID, uuid: osdUUID, name: device, bluestore: true}, nil
}

func (a *osdAgent) createLocalOSD(connection client.Connection, root string) (*osdConfig, error) {
	var osdID int
	var osdUUID uuid.UUID
	var osdDataPath string

	osdDataRoot, err := findOSDDataRoot(root)
	if err != nil {
		return nil, fmt.Errorf("failed to find OSD data root under %s: %+v", root, err)
	}

	if isOSDDataNotExist(osdDataPath) {
		// register the OSD with the cluster now to get an official ID
		osdID, osdUUID, err = registerOSDWithCluster(root, connection)
		if err != nil {
			return nil, fmt.Errorf("failed to register OSD %d with cluster: %+v", osdID, err)
		}

		osdDataRoot = getOSDRootDir(root, osdID)
		if err := os.MkdirAll(osdDataRoot, 0744); err != nil {
			return nil, fmt.Errorf("failed to make osdDataRoot at %s: %+v", osdDataRoot, err)
		}
	}

	return &osdConfig{rootPath: osdDataRoot, id: osdID, uuid: osdUUID, bluestore: false}, nil
}

// runs an OSD with the given config in a child process
func (a *osdAgent) runOSD(context *clusterd.Context, clusterName string, config *osdConfig) error {
	// start the OSD daemon in the foreground with the given config
	log.Printf("starting osd %d at %s", config.id, config.rootPath)

	osdUUIDArg := fmt.Sprintf("--osd-uuid=%s", config.uuid.String())

	params := []string{"--foreground",
		fmt.Sprintf("--id=%d", config.id),
		fmt.Sprintf("--cluster=%s", clusterName),
		fmt.Sprintf("--osd-data=%s", config.rootPath),
		fmt.Sprintf("--conf=%s", getOSDConfFilePath(config.rootPath, clusterName)),
		fmt.Sprintf("--keyring=%s", getOSDKeyringPath(config.rootPath)),
		osdUUIDArg,
	}

	if !config.bluestore {
		params = append(params, fmt.Sprintf("--osd-journal=%s", getOSDJournalPath(config.rootPath)))
	}

	process, err := context.ProcMan.Start(
		"osd",
		regexp.QuoteMeta(osdUUIDArg),
		proc.ReuseExisting,
		params...)
	if err != nil {
		return fmt.Errorf("failed to start osd %d: %+v", config.id, err)
	}

	if a.osdProc == nil {
		// initialize the osd map
		a.osdProc = make(map[string]*proc.MonitoredProc)
	}
	if process != nil {
		// if the process was already running Start will return nil in which case we don't want to overwrite it
		a.osdProc[config.name] = process
	}

	return nil
}

// For all applied OSDs, gets a mapping of their device names to their serial numbers
func GetAppliedOSDs(nodeID string, etcdClient etcd.KeysAPI) (*util.Set, error) {
	appliedKey := path.Join(getAppliedKey(nodeID), deviceKey)

	devices, err := util.GetDirChildKeys(etcdClient, appliedKey)
	if err != nil {
		if util.IsEtcdKeyNotFound(err) {
			return util.NewSet(), nil
		}
		return nil, err
	}

	return devices, nil
}

func (a *osdAgent) saveAppliedOSDs(context *clusterd.Context, devices map[string]*osdInfo) error {
	var settings = make(map[string]string)
	for name, info := range devices {
		var pseudoName string
		if info.serial == "" {
			// convert the local path to an id that will not contain path characters in the etcd key
			pseudoName = getPseudoDir(name)
		} else {
			pseudoName = info.serial
		}
		baseKey := deviceKey
		if info.dir {
			baseKey = dirKey
		}

		settings[path.Join(baseKey, pseudoName, "name")] = name
		settings[path.Join(baseKey, pseudoName, "id")] = strconv.FormatInt(int64(info.id), 10)
	}

	appliedKey := path.Join(getAppliedKey(context.NodeID))
	if err := util.StoreEtcdProperties(context.EtcdClient, appliedKey, settings); err != nil {
		return fmt.Errorf("failed to mark devices as applied: %+v", err)
	}

	return nil
}

func getPseudoDir(path string) string {
	// cut off the leading slash so we won't end up with a hidden etcd key
	if strings.HasPrefix(path, "/") {
		path = path[1:]
	}

	return strings.Replace(path, "/", "_", -1)
}
