package castled

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	etcd "github.com/coreos/etcd/client"
	"github.com/google/uuid"

	"github.com/quantum/castle/pkg/cephclient"
	"github.com/quantum/castle/pkg/clusterd"
	"github.com/quantum/castle/pkg/clusterd/inventory"
	"github.com/quantum/castle/pkg/proc"
	"github.com/quantum/castle/pkg/util"
)

const (
	osdAgentName = "osd"
)

type osdAgent struct {
	cluster     *ClusterInfo
	devices     []string
	forceFormat bool
	location    *CrushLocation
	factory     cephclient.ConnectionFactory
	osdCmd      map[int]*exec.Cmd
}

func newOSDAgent(factory cephclient.ConnectionFactory, rawDevices string, forceFormat bool, location *CrushLocation) *osdAgent {
	devices := strings.Split(rawDevices, ",")
	return &osdAgent{factory: factory, devices: devices, forceFormat: forceFormat, location: location}
}

func (a *osdAgent) Name() string {
	return osdAgentName
}

func (a *osdAgent) ConfigureLocalService(context *clusterd.Context) error {

	// check if the osd is in the desired state for this node
	key := path.Join(cephKey, osdAgentName, desiredKey, context.NodeID)
	osdDesired, err := util.EtcdDirExists(context.EtcdClient, key)
	if err != nil {
		return err
	}
	if !osdDesired {
		return nil
	}

	a.cluster, err = LoadClusterInfo(context.EtcdClient)
	if err != nil {
		return fmt.Errorf("failed to load cluster info: %v", err)
	}

	if len(a.devices) == 0 {
		return nil
	}

	// Connect to the ceph cluster
	adminConn, err := ConnectToClusterAsAdmin(a.factory, a.cluster)
	if err != nil {
		return err
	}
	defer adminConn.Shutdown()

	// Start an OSD for each of the specified devices
	err = a.createOSDs(adminConn, context)
	if err != nil {
		return fmt.Errorf("failed to create OSDs: %+v", err)
	}

	// successful, ensure all applied devices are saved to the cluster config store
	if err := a.saveAppliedOSDs(context); err != nil {
		return fmt.Errorf("failed to save applied OSDs: %+v", err)
	}

	return nil
}

func (a *osdAgent) DestroyLocalService(context *clusterd.Context) error {
	// stop the OSD processes
	for osdID, cmd := range a.osdCmd {
		log.Printf("stopping osd %d", osdID)
		context.ProcMan.Stop(cmd)
	}

	// clear out the osd procs
	a.osdCmd = map[int]*exec.Cmd{}
	return nil
}

func getAppliedKey(nodeID string) string {
	return path.Join(cephKey, osdAgentName, appliedKey, nodeID)
}

// create and initalize OSDs for all the devices specified in the given config
func (a *osdAgent) createOSDs(adminConn cephclient.Connection, context *clusterd.Context) error {
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
	for _, device := range a.devices {
		var osdID int
		var osdUUID uuid.UUID

		mountPoint, err := inventory.GetDeviceMountPoint(device, context.Executor)
		if err != nil {
			return fmt.Errorf("unable to get mount point for device %s: %+v", device, err)
		}

		if mountPoint == "" {
			// the device is not mounted, so proceed with the format and mounting
			if err := formatOSD(device, a.forceFormat, context.Executor); err != nil {
				// attempting to format the volume failed, bail out with error
				return err
			}

			// register the OSD with the cluster now to get an official ID
			osdID, osdUUID, err = registerOSDWithCluster(device, bootstrapConn)
			if err != nil {

			}

			// mount the device using a mount point that reflects the OSD's ID
			mountPoint = fmt.Sprintf("/tmp/osd%d", osdID)
			if err := mountOSD(device, mountPoint, context.Executor); err != nil {
				return err
			}
		}

		osdDataDir := mountPoint

		// find the OSD data path (under the mount point/data dir)
		osdDataPath, err := findOSDDataPath(osdDataDir, a.cluster.Name)
		if err != nil {
			return err
		}

		if _, err := os.Stat(filepath.Join(osdDataPath, "whoami")); os.IsNotExist(err) {
			// osd_data_dir/whoami does not exist yet, create/initialize the OSD
			osdDataPath, err = initializeOSD(a.factory, context, osdDataDir, osdID, osdUUID, bootstrapConn, a.cluster, a.location)
			if err != nil {
				return fmt.Errorf("failed to initialize OSD at %s: %+v", osdDataDir, err)
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
		err = a.runOSD(context, a.cluster.Name, osdID, osdUUID, osdDataPath)
		if err != nil {
			return fmt.Errorf("failed to run osd %d: %+v", osdID, err)
		}
	}

	return nil
}

// runs an OSD with the given config in a child process
func (a *osdAgent) runOSD(context *clusterd.Context, clusterName string, osdID int, osdUUID uuid.UUID, osdDataPath string) error {
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
		a.osdCmd = map[int]*exec.Cmd{}
	}
	if cmd != nil {
		// if the process was already running Start will return nil in which case we don't want to overwrite it
		a.osdCmd[osdID] = cmd
	}

	return nil
}

func GetAppliedOSDs(nodeID string, etcdClient etcd.KeysAPI) (*util.Set, error) {
	appliedKey := getAppliedKey(nodeID)
	return util.GetDirChildKeys(etcdClient, appliedKey)
}

func (a *osdAgent) saveAppliedOSDs(context *clusterd.Context) error {
	appliedKey := getAppliedKey(context.NodeID)
	for _, d := range a.devices {
		serial, err := inventory.GetSerialFromDevice(d, context.NodeID, context.EtcdClient)
		if err != nil {
			return fmt.Errorf("failed to get serial for device %s: %+v", d, err)
		}
		if err := util.CreateEtcdDir(context.EtcdClient, path.Join(appliedKey, serial)); err != nil {
			return fmt.Errorf("failed to mark device %s/%s as applied: %+v", d, serial, err)
		}
	}

	return nil
}
