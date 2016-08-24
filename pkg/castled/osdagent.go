package castled

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"

	etcd "github.com/coreos/etcd/client"
	"github.com/google/uuid"

	"github.com/quantum/castle/pkg/cephd"
	"github.com/quantum/clusterd/pkg/orchestrator"
)

type osdAgent struct {
	cluster     *clusterInfo
	procMan     *orchestrator.ProcessManager
	privateIPv4 string
	etcdClient  etcd.KeysAPI
}

func (a *osdAgent) ConfigureAgent(context *orchestrator.ClusterContext, changeList []orchestrator.ChangeElement) error {
	if err := a.startOSDs(context); err != nil {
		return err
	}

	return nil
}

func (a *osdAgent) DestroyAgent(context *orchestrator.ClusterContext) error {
	return nil
}

func (a *osdAgent) startOSDs(context *orchestrator.ClusterContext) error {

	adminConn, err := connectToCluster(a.cluster, "admin", "")
	if err != nil {
		return err
	}
	defer adminConn.Shutdown()

	// create/start an OSD for each of the specified devices
	devices := []string{}
	if len(devices) > 0 {
		err := a.createOSDs(adminConn, context)
		if err != nil {
			return fmt.Errorf("failed to create OSDs: %+v", err)
		}

	}

	return nil
}

// create and initalize OSDs for all the devices specified in the given config
func (a *osdAgent) createOSDs(adminConn *cephd.Conn, context *orchestrator.ClusterContext) error {
	// generate and write the OSD bootstrap keyring
	if err := createOSDBootstrapKeyring(adminConn, a.cluster.Name, context.Executor); err != nil {
		return err
	}

	// connect to the cluster using the bootstrap-osd creds, this connection will be used for config operations
	bootstrapConn, err := connectToCluster(a.cluster, "bootstrap-osd", getBootstrapOSDKeyringPath(a.cluster.Name))
	if err != nil {
		return err
	}
	defer bootstrapConn.Shutdown()

	// initialize all the desired OSD volumes
	devices := []string{}
	forceFormat := false
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
			osdConn, err := connectToCluster(a.cluster, fmt.Sprintf("osd.%d", osdID), keyringPath)
			if err != nil {
				return err
			}
			defer osdConn.Shutdown()

			// add the new OSD to the cluster crush map
			if err := addOSDToCrushMap(osdConn, osdID, osdDataDir); err != nil {
				return err
			}

			// run the OSD in a child process now that it is fully initialized and ready to go
			err = a.runOSD(a.cluster.Name, osdID, osdUUID, osdDataPath)
			if err != nil {
				return fmt.Errorf("failed to run osd %d: %+v", osdID, err)
			}
		}
	}

	return nil
}

// runs an OSD with the given config in a child process
func (a *osdAgent) runOSD(clusterName string, osdID int, osdUUID uuid.UUID, osdDataPath string) error {
	// start the OSD daemon in the foreground with the given config
	log.Printf("starting osd %d at %s", osdID, osdDataPath)
	err := a.procMan.Start(
		"osd",
		"--foreground",
		fmt.Sprintf("--id=%s", strconv.Itoa(osdID)),
		fmt.Sprintf("--cluster=%s", clusterName),
		fmt.Sprintf("--osd-data=%s", osdDataPath),
		fmt.Sprintf("--osd-journal=%s", getOSDJournalPath(osdDataPath)),
		fmt.Sprintf("--conf=%s", getOSDConfFilePath(osdDataPath)),
		fmt.Sprintf("--keyring=%s", getOSDKeyringPath(osdDataPath)),
		fmt.Sprintf("--osd-uuid=%s", osdUUID.String()))
	if err != nil {
		return fmt.Errorf("failed to start osd %d: %+v", osdID, err)
	}

	return nil
}
