package castled

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"syscall"
	"time"

	uuid "github.com/nu7hatch/gouuid"
	"github.com/quantum/castle/pkg/cephd"
	"github.com/quantum/castle/pkg/proc"
)

const (
	bootstrapOSDClientConfigTemplate = `
[client.bootstrap-osd]
    keyring=%s
`
	bootstrapOSDKeyringTemplate = `
[client.bootstrap-osd]
	key = %s
	caps mon = "allow profile bootstrap-osd"
`
	osdClientConfigTemplate = `
[osd.%d]
	keyring=%s
`
)

// get the bootstrap OSD root dir
func getBootstrapOSDDir() string {
	return "/tmp/bootstrap-osd"
}

// get the full path to the bootstrap OSD keyring
func getBootstrapOSDKeyringPath(clusterName string) string {
	return fmt.Sprintf("%s/%s", getBootstrapOSDDir(), fmt.Sprintf("%s.keyring", clusterName))
}

// get the full path to the bootstrap OSD config file
func getBootstrapOSDConfFilePath() string {
	return fmt.Sprintf("%s/tmp_config", getBootstrapOSDDir())
}

// get the full path to the given OSD's config file
func getOSDConfFilePath(osdDataPath string) string {
	return filepath.Join(osdDataPath, "config")
}

// get the full path to the given OSD's keyring
func getOSDKeyringPath(osdDataPath string) string {
	return filepath.Join(osdDataPath, "keyring")
}

// get the full path to the given OSD's journal
func getOSDJournalPath(osdDataPath string) string {
	return filepath.Join(osdDataPath, "journal")
}

// get the full path to the given OSD's temporary mon map
func getOSDTempMonMapPath(osdDataPath string) string {
	return filepath.Join(osdDataPath, "tmp", "activate.monmap")
}

// create and initalize OSDs for all the devices specified in the given config
func createOSDs(adminConn *cephd.Conn, cfg Config, c *clusterInfo, executor proc.Executor) ([]*exec.Cmd, error) {
	// generate and write the OSD bootstrap keyring
	if err := createOSDBootstrapKeyring(adminConn, cfg.ClusterName, executor); err != nil {
		return nil, err
	}

	// write the bootstrap OSD config file to disk
	bootstrapOSDConfFilePath := getBootstrapOSDConfFilePath()
	if err := writeBootstrapOSDConfFile(cfg, c, getBootstrapOSDKeyringPath(cfg.ClusterName)); err != nil {
		return nil, fmt.Errorf("failed to write bootstrap-osd conf file at %s: %+v", bootstrapOSDConfFilePath, err)
	}

	// connect to the cluster using the bootstrap-osd creds, this connection will be used for config operations
	bootstrapConn, err := connectToCluster(cfg.ClusterName, "client.bootstrap-osd", bootstrapOSDConfFilePath)
	if err != nil {
		return nil, err
	}
	defer bootstrapConn.Shutdown()

	osdProcs := []*exec.Cmd{}

	// initialize all the desired OSD volumes
	for i, device := range cfg.Devices {
		done, err := formatOSD(device, cfg.ForceFormat, executor)
		if err != nil {
			// attempting to format the volume failed, bail out with error
			return osdProcs, err
		} else if !done {
			// the formatting was not done, probably because the drive already has a filesystem.
			// just move on to the next OSD.
			continue
		}

		// TODO: this OSD data dir isn't consistent across multiple runs
		osdDataDir := fmt.Sprintf("/tmp/osd%d", i)
		if err := mountOSD(device, osdDataDir, executor); err != nil {
			return osdProcs, err
		}

		// find the OSD data dir
		osdDataPath, err := findOSDDataPath(osdDataDir, cfg.ClusterName)
		if err != nil {
			return osdProcs, err
		}

		if _, err := os.Stat(filepath.Join(osdDataPath, "whoami")); os.IsNotExist(err) {
			// osd_data_dir/whoami does not exist yet, create/initialize the OSD
			log.Printf("initializing the osd directory %s", osdDataDir)
			osdUUID, err := uuid.NewV4()
			if err != nil {
				return osdProcs, fmt.Errorf("failed to generate UUID for %s: %+v", osdDataDir, err)
			}

			// create the OSD instance via a mon_command, this assigns a cluster wide ID to the OSD
			osdID, err := createOSD(bootstrapConn, osdUUID)
			if err != nil {
				return osdProcs, err
			}

			log.Printf("successfully created OSD %s with ID %d at %s", osdUUID.String(), osdID, osdDataDir)

			// ensure that the OSD data directory is created
			osdDataPath = filepath.Join(osdDataDir, fmt.Sprintf("%s-%d", cfg.ClusterName, osdID))
			if err := os.MkdirAll(osdDataPath, 0777); err != nil {
				return osdProcs, fmt.Errorf("failed to create OSD data dir at %s, %+v", osdDataPath, err)
			}

			// write the OSD config file to disk
			if err := writeOSDConfFile(cfg, c, osdDataPath, osdID); err != nil {
				return osdProcs, fmt.Errorf("failed to write OSD %d config file: %+v", osdID, err)
			}

			// get the current monmap, it will be needed for creating the OSD file system
			monMapRaw, err := getMonMap(bootstrapConn)
			if err != nil {
				return osdProcs, fmt.Errorf("failed to get mon map: %+v", err)
			}

			// create/initalize the OSD file system and journal
			if err := createOSDFileSystem(cfg.ClusterName, osdID, osdUUID, osdDataPath, monMapRaw); err != nil {
				return osdProcs, err
			}

			// add auth privileges for the OSD, the bootstrap-osd privileges were very limited
			if err := addOSDAuth(bootstrapConn, osdID, osdDataPath); err != nil {
				return osdProcs, err
			}

			// open a connection to the cluster using the OSDs creds
			osdConn, err := connectToCluster(cfg.ClusterName, fmt.Sprintf("osd.%d", osdID), getOSDConfFilePath(osdDataPath))
			if err != nil {
				return osdProcs, err
			}
			defer osdConn.Shutdown()

			// add the new OSD to the cluster crush map
			if err := addOSDToCrushMap(osdConn, osdID, osdDataDir); err != nil {
				return osdProcs, err
			}

			// run the OSD in a child process now that it is fully initialized and ready to go
			osdProc, err := runOSD(cfg.ClusterName, osdID, osdUUID, osdDataPath)
			if err != nil {
				return osdProcs, fmt.Errorf("failed to run osd %d: %+v", osdID, err)
			}

			osdProcs = append(osdProcs, osdProc)
		}
	}

	return osdProcs, nil
}

// create a keyring for the bootstrap-osd client, it gets a limited set of privileges
func createOSDBootstrapKeyring(conn *cephd.Conn, clusterName string, executor proc.Executor) error {
	bootstrapOSDKeyringPath := getBootstrapOSDKeyringPath(clusterName)
	if _, err := os.Stat(bootstrapOSDKeyringPath); os.IsNotExist(err) {
		// get-or-create-key for client.bootstrap-osd
		cmd := "auth get-or-create-key"
		command, err := json.Marshal(map[string]interface{}{
			"prefix": cmd,
			"format": "json",
			"entity": "client.bootstrap-osd",
			"caps":   []string{"mon", "allow profile bootstrap-osd"},
		})
		if err != nil {
			return fmt.Errorf("command %s marshall failed: %+v", cmd, err)
		}
		buf, _, err := conn.MonCommand(command)
		if err != nil {
			return fmt.Errorf("mon_command %s failed: %+v", cmd, err)
		}
		var resp map[string]interface{}
		err = json.Unmarshal(buf, &resp)
		if err != nil {
			return fmt.Errorf("failed to unmarshall %s response: %+v", cmd, err)
		}
		bootstrapOSDKey := resp["key"].(string)
		log.Printf("succeeded %s command, bootstrapOSDKey: %s", cmd, bootstrapOSDKey)

		// write the bootstrap-osd keyring to disk
		bootstrapOSDKeyringDir := filepath.Dir(bootstrapOSDKeyringPath)
		if err := os.MkdirAll(bootstrapOSDKeyringDir, 0744); err != nil {
			fmt.Printf("failed to create bootstrap OSD keyring dir at %s: %+v", bootstrapOSDKeyringDir, err)
		}
		bootstrapOSDKeyring := fmt.Sprintf(bootstrapOSDKeyringTemplate, bootstrapOSDKey)
		if err := ioutil.WriteFile(bootstrapOSDKeyringPath, []byte(bootstrapOSDKeyring), 0644); err != nil {
			return fmt.Errorf("failed to write bootstrap-osd keyring to %s: %+v", bootstrapOSDKeyringPath, err)
		}
	}

	return nil
}

// write the bootstrap-osd config file to disk
func writeBootstrapOSDConfFile(cfg Config, c *clusterInfo, bootstrapOSDKeyringPath string) error {
	var contentBuffer bytes.Buffer
	bootstrapOSDConfFilePath := getBootstrapOSDConfFilePath()

	if err := writeGlobalConfigFileSection(&contentBuffer, cfg, c, getBootstrapOSDDir()); err != nil {
		return fmt.Errorf("failed to write bootstrap-osd global config section, %+v", err)
	}

	_, err := contentBuffer.WriteString(fmt.Sprintf(bootstrapOSDClientConfigTemplate, bootstrapOSDKeyringPath))
	if err != nil {
		return fmt.Errorf("failed to write bootstrap-osd client config section, %+v", err)
	}

	if err := writeInitialMonitorsConfigFileSections(&contentBuffer, cfg); err != nil {
		return fmt.Errorf("failed to write bootstrap-osd initial monitor config sections, %+v", err)
	}

	// write the entire config to disk
	if err := writeFile(bootstrapOSDConfFilePath, contentBuffer); err != nil {
		return err
	}

	return nil
}

// format the given device for usage by an OSD
func formatOSD(device string, forceFormat bool, executor proc.Executor) (bool, error) {
	// format the current volume
	cmd := fmt.Sprintf("blkid %s", device)
	devFS, err := executor.ExecuteCommandPipeline(
		cmd,
		fmt.Sprintf(`blkid /dev/%s | sed -nr 's/^.*TYPE=\"(.*)\"$/\1/p'`, device))
	if err != nil {
		return false, fmt.Errorf("command %s failed: %+v", cmd, err)
	}
	if devFS != "" && forceFormat {
		// there's a filesystem on the device, but the user has specified to force a format. give a warning about that.
		log.Printf("WARNING: device %s already formatted with %s, but forcing a format!!!", device, devFS)
	}

	if devFS == "" || forceFormat {
		// execute the format operation
		cmd = fmt.Sprintf("format %s", device)
		err = executor.ExecuteCommand(cmd, "sudo", "/usr/sbin/mkfs.btrfs", "-f", "-m", "single", "-n", "32768", fmt.Sprintf("/dev/%s", device))
		if err != nil {
			return false, fmt.Errorf("command %s failed: %+v", cmd, err)
		}
	} else {
		// disk is already formatted and the user doesn't want to force it, return no error, but also specify that no format was done
		log.Printf("device %s already formatted with %s, cannot use for OSD", device, devFS)
		return false, nil
	}

	return true, nil
}

// mount the OSD data directory onto the given device
func mountOSD(device string, mountPath string, executor proc.Executor) error {
	cmd := fmt.Sprintf("lsblk %s", device)
	var diskUUID string

	retryCount := 0
	retryMax := 10
	sleepTime := 2
	for {
		// there is lag in between when a filesytem is created and its UUID is available.  retry as needed
		// until we have a usable UUID for the newly formatted filesystem.
		var err error
		diskUUID, err = executor.ExecuteCommandWithOutput(cmd, "lsblk", fmt.Sprintf("/dev/%s", device), "-d", "-n", "-r", "-o", "UUID")
		if err != nil {
			return fmt.Errorf("command %s failed: %+v", cmd, err)
		}

		if diskUUID != "" {
			// we got the UUID from the disk.  Verify this UUID is up to date in the /dev/disk/by-uuid dir by
			// checking for it multiple times in a row.  For an existing device, the device UUID and the
			// by-uuid link can take a bit to get updated after getting formatted.  Increase our confidence
			// that we have the updated UUID by performing this check multiple times in a row.
			log.Printf("verifying UUID %s", diskUUID)
			uuidCheckOK := true
			uuidCheckCount := 0
			for uuidCheckCount < 3 {
				uuidCheckCount++
				if _, err := os.Stat(fmt.Sprintf("/dev/disk/by-uuid/%s", diskUUID)); os.IsNotExist(err) {
					// the UUID we got for the disk does not exist under /dev/disk/by-uuid.  Retry.
					uuidCheckOK = false
					break
				}
				<-time.After(time.Duration(1) * time.Second)
			}

			if uuidCheckOK {
				log.Printf("device %s UUID created: %s", device, diskUUID)
				break
			}
		}

		retryCount++
		if retryCount > retryMax {
			return fmt.Errorf("exceeded max retry count waiting for device %s UUID to be created", device)
		}

		<-time.After(time.Duration(sleepTime) * time.Second)
	}

	// mount the volume
	os.MkdirAll(mountPath, 0777)
	cmd = fmt.Sprintf("mount %s", device)
	if err := executor.ExecuteCommand(cmd, "sudo", "mount", "-o", "user_subvol_rm_allowed",
		fmt.Sprintf("/dev/disk/by-uuid/%s", diskUUID), mountPath); err != nil {
		return fmt.Errorf("command %s failed: %+v", cmd, err)
	}

	// chown for the current user since we had to format and mount with sudo
	currentUser, err := user.Current()
	if err != nil {
		log.Printf("unable to find current user: %+v", err)
	} else {
		cmd = fmt.Sprintf("chown %s", mountPath)
		if err := executor.ExecuteCommand(cmd, "sudo", "chown", "-R",
			fmt.Sprintf("%s:%s", currentUser.Username, currentUser.Username), mountPath); err != nil {
			log.Printf("command %s failed: %+v", cmd, err)
		}
	}

	return nil
}

// looks for an existing OSD data path under the given root
func findOSDDataPath(osdRoot, clusterName string) (string, error) {
	var osdDataPath string
	fl, err := ioutil.ReadDir(osdRoot)
	if err != nil {
		return "", fmt.Errorf("failed to read dir %s: %+v", osdRoot, err)
	}
	p := fmt.Sprintf(`%s-[A-Za-z0-9._-]+`, clusterName)
	for _, f := range fl {
		if f.IsDir() {
			matched, err := regexp.MatchString(p, f.Name())
			if err == nil && matched {
				osdDataPath = filepath.Join(osdRoot, f.Name())
				break
			}
		}
	}

	return osdDataPath, nil
}

// creates the OSD identity in the cluster via a mon_command
func createOSD(bootstrapConn *cephd.Conn, osdUUID *uuid.UUID) (int, error) {
	cmd := "osd create"
	command, err := json.Marshal(map[string]interface{}{
		"prefix": cmd,
		"format": "json",
		"entity": "client.bootstrap-osd",
		"uuid":   osdUUID.String(),
	})
	if err != nil {
		return 0, fmt.Errorf("command %s marshall failed: %+v", cmd, err)
	}
	buf, _, err := bootstrapConn.MonCommand(command)
	if err != nil {
		return 0, fmt.Errorf("mon_command %s failed: %+v", cmd, err)
	}
	var resp map[string]interface{}
	err = json.Unmarshal(buf, &resp)
	if err != nil {
		return 0, fmt.Errorf("failed to unmarshall %s response: %+v.  raw response: '%s'", cmd, err, string(buf[:]))
	}

	return int(resp["osdid"].(float64)), nil
}

// writes a config file to disk for the given OSD and config
func writeOSDConfFile(cfg Config, c *clusterInfo, osdDataPath string, osdID int) error {
	var contentBuffer bytes.Buffer
	osdConfFilePath := getOSDConfFilePath(osdDataPath)

	if err := writeGlobalConfigFileSection(&contentBuffer, cfg, c, osdDataPath); err != nil {
		return fmt.Errorf("failed to write osd %d global config section, %+v", osdID, err)
	}

	_, err := contentBuffer.WriteString(fmt.Sprintf(osdClientConfigTemplate, osdID, getOSDKeyringPath(osdDataPath)))
	if err != nil {
		return fmt.Errorf("failed to write osd %d config section, %+v", osdID, err)
	}

	if err := writeInitialMonitorsConfigFileSections(&contentBuffer, cfg); err != nil {
		return fmt.Errorf("failed to write osd %d initial monitor config sections, %+v", osdID, err)
	}

	// write the entire config to disk
	if err := writeFile(osdConfFilePath, contentBuffer); err != nil {
		return err
	}

	return nil
}

// gets the current mon map for the cluster
func getMonMap(bootstrapConn *cephd.Conn) ([]byte, error) {
	cmd := "mon getmap"
	command, err := json.Marshal(map[string]interface{}{
		"prefix": cmd,
		"format": "json",
		"entity": "client.bootstrap-osd",
	})
	if err != nil {
		return nil, fmt.Errorf("command %s marshall failed: %+v", cmd, err)
	}
	buf, _, err := bootstrapConn.MonCommand(command)
	if err != nil {
		return nil, fmt.Errorf("mon_command %s failed: %+v", cmd, err)
	}
	return buf, nil
}

// creates/initalizes the OSD filesystem and journal via a child process
func createOSDFileSystem(clusterName string, osdID int, osdUUID *uuid.UUID, osdDataPath string, monMap []byte) error {
	log.Printf("Initializing OSD %d file system at %s...", osdID, osdDataPath)

	// the current monmap is needed to create the OSD, save it to a temp location so it is accessible
	monMapTmpPath := getOSDTempMonMapPath(osdDataPath)
	monMapTmpDir := filepath.Dir(monMapTmpPath)
	if err := os.MkdirAll(monMapTmpDir, 0744); err != nil {
		return fmt.Errorf("failed to create monmap tmp file directory at %s: %+v", monMapTmpDir, err)
	}
	if err := ioutil.WriteFile(monMapTmpPath, monMap, 0644); err != nil {
		return fmt.Errorf("failed to write mon map to tmp file %s, %+v", monMapTmpPath, err)
	}

	// create the OSD file system and journal
	err := proc.RunChildProcess(
		"osd",
		"--mkfs",
		"--mkkey",
		fmt.Sprintf("--id=%s", strconv.Itoa(osdID)),
		fmt.Sprintf("--cluster=%s", clusterName),
		fmt.Sprintf("--osd-data=%s", osdDataPath),
		fmt.Sprintf("--osd-journal=%s", getOSDJournalPath(osdDataPath)),
		fmt.Sprintf("--conf=%s", getOSDConfFilePath(osdDataPath)),
		fmt.Sprintf("--keyring=%s", getOSDKeyringPath(osdDataPath)),
		fmt.Sprintf("--osd-uuid=%s", osdUUID.String()),
		fmt.Sprintf("--monmap=%s", monMapTmpPath))

	if err != nil {
		return fmt.Errorf("failed osd mkfs for OSD ID %d, UUID %s, dataDir %s: %+v",
			osdID, osdUUID.String(), osdDataPath, err)
	}

	return nil
}

// add OSD auth privileges for the given OSD ID.  the bootstrap-osd privileges are limited and a real OSD needs more.
func addOSDAuth(bootstrapConn *cephd.Conn, osdID int, osdDataPath string) error {
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

	log.Printf("succeeded %s command for %s. info: %s", cmd, osdEntity, info)
	return nil
}

// adds the given OSD to the crush map
func addOSDToCrushMap(osdConn *cephd.Conn, osdID int, osdDataPath string) error {
	// get the size of the volume containing the OSD data dir
	s := syscall.Statfs_t{}
	if err := syscall.Statfs(osdDataPath, &s); err != nil {
		return fmt.Errorf("failed to statfs on %s, %+v", osdDataPath, err)
	}
	all := s.Blocks * uint64(s.Bsize)

	// weight is ratio of (size in KB) / (1 GB)
	weight := float64(all/1024) / 1073741824.0
	weight, _ = strconv.ParseFloat(fmt.Sprintf("%.2f", weight), 64)

	osdEntity := fmt.Sprintf("osd.%d", osdID)
	log.Printf("OSD %s at %s, bytes: %d, weight: %.2f", osdEntity, osdDataPath, all, weight)

	hostName, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("failed to get hostname, %+v", err)
	}

	cmd := "osd crush create-or-move"
	command, err := json.Marshal(map[string]interface{}{
		"prefix": cmd,
		"format": "json",
		"id":     osdID,
		"weight": weight,
		"args":   []string{fmt.Sprintf("host=%s", hostName), "root=default"},
	})
	if err != nil {
		return fmt.Errorf("command %s marshall failed: %+v", cmd, err)
	}

	log.Printf("%s command: '%s'", cmd, string(command))

	_, info, err := osdConn.MonCommand(command)
	if err != nil {
		return fmt.Errorf("mon_command %s failed: %+v", cmd, err)
	}

	log.Printf("succeeded adding %s to crush map. info: %s", osdEntity, info)
	return nil
}

// runs an OSD with the given config in a child process
func runOSD(clusterName string, osdID int, osdUUID *uuid.UUID, osdDataPath string) (*exec.Cmd, error) {
	// start the OSD daemon in the foreground with the given config
	log.Printf("starting osd %d at %s", osdID, osdDataPath)
	cmd, err := proc.StartChildProcess(
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
		return nil, fmt.Errorf("failed to start osd %d: %+v", osdID, err)
	}

	return cmd, nil
}
