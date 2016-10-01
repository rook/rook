package cephmgr

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"

	ctx "golang.org/x/net/context"

	etcd "github.com/coreos/etcd/client"
	"github.com/google/uuid"

	"github.com/quantum/castle/pkg/cephmgr/client"
	"github.com/quantum/castle/pkg/clusterd"
	"github.com/quantum/castle/pkg/clusterd/inventory"
	"github.com/quantum/castle/pkg/proc"
	"github.com/quantum/castle/pkg/util"
)

const (
	osdAgentName    = "osd"
	devicesKey      = "devices"
	localDeviceName = "<local>"
)

type osdAgent struct {
	cluster     *ClusterInfo
	forceFormat bool
	location    string
	factory     client.ConnectionFactory
	osdCmd      map[string]*exec.Cmd
	devices     string
}

func newOSDAgent(factory client.ConnectionFactory, devices string, forceFormat bool, location string) *osdAgent {
	return &osdAgent{factory: factory, devices: devices, forceFormat: forceFormat, location: location}
}

func (a *osdAgent) Name() string {
	return osdAgentName
}

// set the desired state in etcd
func (a *osdAgent) Initialize(context *clusterd.Context) error {

	// add the devices to desired state
	devices := strings.Split(a.devices, ",")

	if len(devices) == 1 && devices[0] == "" {
		// no devices specified, just use the local file system
		devices = []string{localDeviceName}
	}

	for _, device := range devices {
		err := AddDesiredDevice(context.EtcdClient, &Device{Name: device, NodeID: context.NodeID})
		if err != nil {
			return fmt.Errorf("failed to add desired device %s", device)
		}
	}

	return nil
}

func (a *osdAgent) ConfigureLocalService(context *clusterd.Context) error {

	// check if the osd is in the desired state for this node
	key := path.Join(cephKey, osdAgentName, desiredKey, context.NodeID, "ready")
	osdDesired, err := context.EtcdClient.Get(ctx.Background(), key, nil)
	if (err != nil && util.IsEtcdKeyNotFound(err)) || osdDesired.Node.Value != "1" {
		return nil
	} else if err != nil {
		return err
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
	adminConn, err := ConnectToClusterAsAdmin(a.factory, a.cluster)
	if err != nil {
		return err
	}
	defer adminConn.Shutdown()

	devices, err := loadDesiredDevices(context.EtcdClient, context.NodeID)
	if devices.Count() == 0 {
		return nil
	}

	if err := a.startDesiredDevices(context, adminConn, devices); err != nil {
		return err
	}

	return a.stopUndesiredDevices(context, adminConn, devices)
}

func (a *osdAgent) startDesiredDevices(context *clusterd.Context, connection client.Connection, devices *util.Set) error {

	// Start an OSD for each of the specified devices
	if err := a.createOSDs(connection, context, devices); err != nil {
		return fmt.Errorf("failed to create OSDs: %+v", err)
	}

	// successful, ensure all applied devices are saved to the cluster config store
	if err := a.saveAppliedOSDs(context, devices); err != nil {
		return fmt.Errorf("failed to save applied OSDs: %+v", err)
	}

	return nil
}

func (a *osdAgent) stopUndesiredDevices(context *clusterd.Context, connection client.Connection, desired *util.Set) error {
	applied, err := GetAppliedOSDs(context.NodeID, context.EtcdClient)
	if err != nil {
		return fmt.Errorf("failed to get applied OSDs. %v", err)
	}

	var lastErr error
	for device := range applied {
		if desired.Contains(device) {
			// the osd is desired and applied
			continue
		}

		cmd, ok := a.osdCmd[device]
		if ok {
			// stop the osd process
			err = context.ProcMan.Stop(cmd)
			if err != nil {
				log.Printf("failed to stop osd for device %s. %v", device, err)
				lastErr = err
				continue
			}

			delete(a.osdCmd, device)
		}

		appliedKey := path.Join(getAppliedKey(context.NodeID), devicesKey, device)
		_, err = context.EtcdClient.Delete(ctx.Background(), appliedKey, &etcd.DeleteOptions{Recursive: true, Dir: true})
		if err != nil {
			log.Printf("failed to remove device %s from desired state. %v", device, err)
			lastErr = err
			continue
		}

		log.Printf("Stopped and removed osd device %s", device)
	}

	return lastErr
}

func (a *osdAgent) DestroyLocalService(context *clusterd.Context) error {
	// stop the OSD processes
	for device, cmd := range a.osdCmd {
		log.Printf("stopping osd on device %s", device)
		context.ProcMan.Stop(cmd)
	}

	// clear out the osd procs
	a.osdCmd = map[string]*exec.Cmd{}
	return nil
}

func getAppliedKey(nodeID string) string {
	return path.Join(cephKey, osdAgentName, appliedKey, nodeID)
}

// create and initalize OSDs for all the devices specified in the given config
func (a *osdAgent) createOSDs(adminConn client.Connection, context *clusterd.Context, devices *util.Set) error {
	// generate and write the OSD bootstrap keyring
	if err := createOSDBootstrapKeyring(adminConn, a.cluster.Name); err != nil {
		return err
	}

	// connect to the cluster using the bootstrap-osd creds, this connection will be used for config operations
	bootstrapConn, err := connectToCluster(a.factory, a.cluster, getBootstrapOSDDir(), "bootstrap-osd", getBootstrapOSDKeyringPath(a.cluster.Name))
	if err != nil {
		return err
	}
	defer bootstrapConn.Shutdown()

	// initialize all the desired OSD volumes
	for device := range devices.Iter() {
		var osdID int
		var osdUUID uuid.UUID
		var osdDataRoot string

		if device == localDeviceName {
			osdDataRoot, osdID, osdUUID, err = a.createLocalOSD(bootstrapConn)
			if err != nil {
				return fmt.Errorf("failed to create local OSD: %+v", err)
			}
		} else {
			osdDataRoot, osdID, osdUUID, err = a.createMountedOSD(device, bootstrapConn, context)
			if err != nil {
				return fmt.Errorf("failed to create mounted OSD for device %s: %+v", device, err)
			}
		}

		// find the OSD data path (under the mount point/data dir)
		osdDataPath, err := findOSDDataPath(osdDataRoot, a.cluster.Name)
		if err != nil {
			return err
		}

		if isOSDDataNotExist(osdDataPath) {
			// osd_data_dir/whoami does not exist yet, create/initialize the OSD
			osdDataPath, err = initializeOSD(a.factory, context, osdDataRoot, osdID, osdUUID, device, bootstrapConn, a.cluster, a.location)
			if err != nil {
				return fmt.Errorf("failed to initialize OSD at %s: %+v", osdDataRoot, err)
			}
		} else {
			// osd_data_dir/whoami already exists, meaning the OSD is already set up.
			// look up some basic information about it so we can run it.
			osdID, osdUUID, err = getOSDInfo(osdDataPath)
			if err != nil {
				return fmt.Errorf("failed to get OSD information from %s: %+v", osdDataPath, err)
			}
		}

		// run the OSD in a child process now that it is fully initialized and ready to go
		err = a.runOSD(context, a.cluster.Name, osdID, osdUUID, osdDataPath, device)
		if err != nil {
			return fmt.Errorf("failed to run osd %d: %+v", osdID, err)
		}
	}

	return nil
}

func (a *osdAgent) createMountedOSD(device string, bootstrapConn client.Connection, context *clusterd.Context) (string, int, uuid.UUID, error) {
	var osdID int
	var osdUUID uuid.UUID

	mountPoint, err := inventory.GetDeviceMountPoint(device, context.Executor)
	if err != nil {
		return "", -1, uuid.UUID{}, fmt.Errorf("unable to get mount point for device %s: %+v", device, err)
	}

	if mountPoint == "" {
		// the device is not mounted, so proceed with the format and mounting
		if err := formatOSD(device, a.forceFormat, context.Executor); err != nil {
			// attempting to format the volume failed, bail out with error
			return "", -1, uuid.UUID{}, err
		}

		// register the OSD with the cluster now to get an official ID
		osdID, osdUUID, err = registerOSDWithCluster(device, bootstrapConn)
		if err != nil {
			return "", -1, uuid.UUID{}, fmt.Errorf("failed to regiser OSD %d with cluster: %+v", osdID, err)
		}

		// mount the device using a mount point that reflects the OSD's ID
		mountPoint = getOSDRootDir(osdID)
		if err := mountOSD(device, mountPoint, context.Executor); err != nil {
			return "", -1, uuid.UUID{}, err
		}
	}

	return mountPoint, osdID, osdUUID, nil
}

func (a *osdAgent) createLocalOSD(bootstrapConn client.Connection) (string, int, uuid.UUID, error) {
	var osdID int
	var osdUUID uuid.UUID
	var osdDataPath string

	root := "/tmp"
	osdDataRoot, err := findOSDDataRoot(root)
	if err != nil {
		return "", -1, uuid.UUID{}, fmt.Errorf("failed to find OSD data root under %s: %+v", root, err)
	}

	if osdDataRoot != "" {
		osdDataPath, err = findOSDDataPath(osdDataRoot, a.cluster.Name)
		if err != nil {
			return "", -1, uuid.UUID{}, fmt.Errorf("failed to find osd data path under %s: %+v", osdDataRoot, err)
		}
	}

	if isOSDDataNotExist(osdDataPath) {
		// register the OSD with the cluster now to get an official ID
		osdID, osdUUID, err = registerOSDWithCluster(osdDataRoot, bootstrapConn)
		if err != nil {
			return "", -1, uuid.UUID{}, fmt.Errorf("failed to register OSD %d with cluster: %+v", osdID, err)
		}

		osdDataRoot = getOSDRootDir(osdID)
		if err := os.MkdirAll(osdDataRoot, 0744); err != nil {
			return "", -1, uuid.UUID{}, fmt.Errorf("failed to make osdDataRoot at %s: %+v", osdDataRoot, err)
		}
	}

	return osdDataRoot, osdID, osdUUID, nil
}

// runs an OSD with the given config in a child process
func (a *osdAgent) runOSD(context *clusterd.Context, clusterName string, osdID int, osdUUID uuid.UUID, osdDataPath, device string) error {
	// start the OSD daemon in the foreground with the given config
	log.Printf("starting osd %d at %s", osdID, osdDataPath)
	osdUUIDArg := fmt.Sprintf("--osd-uuid=%s", osdUUID.String())
	cmd, err := context.ProcMan.Start(
		"osd",
		regexp.QuoteMeta(osdUUIDArg),
		proc.ReuseExisting,
		"--foreground",
		fmt.Sprintf("--id=%s", strconv.Itoa(osdID)),
		fmt.Sprintf("--cluster=%s", clusterName),
		fmt.Sprintf("--osd-data=%s", osdDataPath),
		fmt.Sprintf("--osd-journal=%s", getOSDJournalPath(osdDataPath)),
		fmt.Sprintf("--conf=%s", getOSDConfFilePath(osdDataPath, clusterName)),
		fmt.Sprintf("--keyring=%s", getOSDKeyringPath(osdDataPath)),
		osdUUIDArg)
	if err != nil {
		return fmt.Errorf("failed to start osd %d: %+v", osdID, err)
	}

	if a.osdCmd == nil {
		// initialize the osd map
		a.osdCmd = map[string]*exec.Cmd{}
	}
	if cmd != nil {
		// if the process was already running Start will return nil in which case we don't want to overwrite it
		a.osdCmd[device] = cmd
	}

	return nil
}

// For all applied OSDs, gets a mapping of their device names to their serial numbers
func GetAppliedOSDs(nodeID string, etcdClient etcd.KeysAPI) (map[string]string, error) {
	appliedKey := path.Join(getAppliedKey(nodeID), devicesKey)
	devices := map[string]string{}

	deviceKeys, err := etcdClient.Get(ctx.Background(), appliedKey, &etcd.GetOptions{Recursive: true})
	if err != nil {
		if util.IsEtcdKeyNotFound(err) {
			return devices, nil
		}
		return nil, err
	}

	// parse the device info from etcd
	for _, dev := range deviceKeys.Node.Nodes {
		name := util.GetLeafKeyPath(dev.Key)
		for _, setting := range dev.Nodes {
			if strings.HasSuffix(setting.Key, "/serial") {
				devices[name] = setting.Value
			}
		}
	}

	return devices, nil
}

func (a *osdAgent) saveAppliedOSDs(context *clusterd.Context, devices *util.Set) error {
	appliedKey := path.Join(getAppliedKey(context.NodeID), devicesKey)
	var settings = make(map[string]string)
	for d := range devices.Iter() {
		var serial string
		var err error
		if d == localDeviceName {
			serial = localDeviceName
		} else {
			serial, err = inventory.GetSerialFromDevice(d, context.NodeID, context.EtcdClient)
			if err != nil {
				return fmt.Errorf("failed to get serial for device %s: %+v", d, err)
			}
		}

		settings[path.Join(d, "serial")] = serial
	}

	if err := util.StoreEtcdProperties(context.EtcdClient, appliedKey, settings); err != nil {
		return fmt.Errorf("failed to mark devices as applied: %+v", err)
	}

	return nil
}
