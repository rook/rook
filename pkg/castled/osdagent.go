package castled

import (
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	ctx "golang.org/x/net/context"

	"github.com/google/uuid"

	"github.com/quantum/castle/pkg/cephd"
	"github.com/quantum/clusterd/pkg/orchestrator"
)

const (
	osdAgentName = "ceph-osd"
)

type osdAgent struct {
	cluster *clusterInfo
}

func (a *osdAgent) GetName() string {
	return osdAgentName
}

func (a *osdAgent) ConfigureLocalService(context *orchestrator.ClusterContext) error {

	var err error
	a.cluster, err = loadClusterInfo(context.EtcdClient)
	if err != nil {
		return fmt.Errorf("failed to load cluster info: %v", err)
	}

	// Get the devices where OSDs should be started
	devices, forceFormat, err := getDevices(context)
	if err != nil {
		return err
	}

	if len(devices) == 0 {
		return nil
	}

	// Connect to the ceph cluster
	adminConn, err := connectToClusterAsAdmin(a.cluster)
	if err != nil {
		return err
	}
	defer adminConn.Shutdown()

	// Start an OSD for each of the specified devices
	err = a.createOSDs(adminConn, context, devices, forceFormat)
	if err != nil {
		return fmt.Errorf("failed to create OSDs: %+v", err)
	}

	return nil
}

func (a *osdAgent) DestroyLocalService(context *orchestrator.ClusterContext) error {
	return nil
}

func getDevices(context *orchestrator.ClusterContext) ([]string, bool, error) {
	devices := []string{}
	key := path.Join(orchestrator.DesiredNodesKey, context.NodeID)
	resp, err := context.EtcdClient.Get(ctx.Background(), path.Join(key, "devices"), nil)
	if err != nil {
		return nil, false, err
	}

	devices = strings.Split(resp.Node.Value, ",")

	resp, err = context.EtcdClient.Get(ctx.Background(), path.Join(key, "forceFormat"), nil)
	if err != nil {
		return nil, false, err
	}

	forceFormat, err := strconv.ParseBool(resp.Node.Value)
	if err != nil {
		return nil, false, err
	}

	log.Printf("Starting OSDs on devices %v. format=%t", devices, forceFormat)
	return devices, forceFormat, nil
}

// create and initalize OSDs for all the devices specified in the given config
func (a *osdAgent) createOSDs(adminConn *cephd.Conn, context *orchestrator.ClusterContext, devices []string, forceFormat bool) error {
	// generate and write the OSD bootstrap keyring
	if err := createOSDBootstrapKeyring(adminConn, a.cluster.Name, context.Executor); err != nil {
		return err
	}

	// connect to the cluster using the bootstrap-osd creds, this connection will be used for config operations
	bootstrapConn, err := connectToCluster(a.cluster, getBootstrapOSDDir(), "bootstrap-osd", getBootstrapOSDKeyringPath(a.cluster.Name))
	if err != nil {
		return err
	}
	defer bootstrapConn.Shutdown()

	// initialize all the desired OSD volumes
	for i, device := range devices {
		done, err := formatOSD(device, forceFormat, context.Executor)
		if err != nil {
			// attempting to format the volume failed, bail out with error
			return err
		} else if !done {
			// the formatting was not done, probably because the drive already has a filesystem.
			// just move on to the next OSD.
			continue
		}

		// TODO: this OSD data dir isn't consistent across multiple runs
		osdDataDir := fmt.Sprintf("/tmp/osd%d", i)
		if err := mountOSD(device, osdDataDir, context.Executor); err != nil {
			return err
		}

		// find the OSD data dir
		osdDataPath, err := findOSDDataPath(osdDataDir, a.cluster.Name)
		if err != nil {
			return err
		}

		if _, err := os.Stat(filepath.Join(osdDataPath, "whoami")); os.IsNotExist(err) {
			// osd_data_dir/whoami does not exist yet, create/initialize the OSD
			log.Printf("initializing the osd directory %s", osdDataDir)
			osdUUID, err := uuid.NewRandom()
			if err != nil {
				return fmt.Errorf("failed to generate UUID for %s: %+v", osdDataDir, err)
			}

			// create the OSD instance via a mon_command, this assigns a cluster wide ID to the OSD
			osdID, err := createOSD(bootstrapConn, osdUUID)
			if err != nil {
				return err
			}

			log.Printf("successfully created OSD %s with ID %d at %s", osdUUID.String(), osdID, osdDataDir)

			// ensure that the OSD data directory is created
			osdDataPath = filepath.Join(osdDataDir, fmt.Sprintf("%s-%d", a.cluster.Name, osdID))
			if err := os.MkdirAll(osdDataPath, 0777); err != nil {
				return fmt.Errorf("failed to create OSD data dir at %s, %+v", osdDataPath, err)
			}

			// write the OSD config file to disk
			keyringPath := getOSDKeyringPath(osdDataPath)
			_, err = generateConfigFile(a.cluster, osdDataPath, fmt.Sprintf("osd.%d", osdID), keyringPath)
			if err != nil {
				return fmt.Errorf("failed to write OSD %d config file: %+v", osdID, err)
			}

			// get the current monmap, it will be needed for creating the OSD file system
			monMapRaw, err := getMonMap(bootstrapConn)
			if err != nil {
				return fmt.Errorf("failed to get mon map: %+v", err)
			}

			// create/initalize the OSD file system and journal
			if err := createOSDFileSystem(a.cluster.Name, osdID, osdUUID, osdDataPath, monMapRaw); err != nil {
				return err
			}

			// add auth privileges for the OSD, the bootstrap-osd privileges were very limited
			if err := addOSDAuth(bootstrapConn, osdID, osdDataPath); err != nil {
				return err
			}

			// open a connection to the cluster using the OSDs creds
			osdConn, err := connectToCluster(a.cluster, osdDataDir, fmt.Sprintf("osd.%d", osdID), keyringPath)
			if err != nil {
				return err
			}
			defer osdConn.Shutdown()

			// add the new OSD to the cluster crush map
			if err := addOSDToCrushMap(osdConn, osdID, osdDataDir); err != nil {
				return err
			}

			// run the OSD in a child process now that it is fully initialized and ready to go
			err = a.runOSD(context, a.cluster.Name, osdID, osdUUID, osdDataPath)
			if err != nil {
				return fmt.Errorf("failed to run osd %d: %+v", osdID, err)
			}
		}
	}

	return nil
}

// runs an OSD with the given config in a child process
func (a *osdAgent) runOSD(context *orchestrator.ClusterContext, clusterName string, osdID int, osdUUID uuid.UUID, osdDataPath string) error {
	// start the OSD daemon in the foreground with the given config
	log.Printf("starting osd %d at %s", osdID, osdDataPath)
	err := context.ProcMan.Start(
		"osd",
		"--foreground",
		fmt.Sprintf("--id=%s", strconv.Itoa(osdID)),
		fmt.Sprintf("--cluster=%s", clusterName),
		fmt.Sprintf("--osd-data=%s", osdDataPath),
		fmt.Sprintf("--osd-journal=%s", getOSDJournalPath(osdDataPath)),
		fmt.Sprintf("--conf=%s", getOSDConfFilePath(osdDataPath, clusterName)),
		fmt.Sprintf("--keyring=%s", getOSDKeyringPath(osdDataPath)),
		fmt.Sprintf("--osd-uuid=%s", osdUUID.String()))
	if err != nil {
		return fmt.Errorf("failed to start osd %d: %+v", osdID, err)
	}

	return nil
}
