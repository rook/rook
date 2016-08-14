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
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"syscall"
	"time"

	etcd "github.com/coreos/etcd/client"
	"github.com/nu7hatch/gouuid"
	"golang.org/x/net/context"

	"github.com/quantum/castle/pkg/cephd"
	"github.com/quantum/castle/pkg/kvstore"
	"github.com/quantum/castle/pkg/proc"
)

const (
	cephKey = "/castle/ceph"

	monitorKeyringTemplate = `
[mon.]
	key = %s
	caps mon = "allow *"
[client.admin]
	key = %s
	auid = 0
	caps mds = "allow"
	caps mon = "allow *"
	caps osd = "allow *"
`
	globalConfigTemplate = `
[global]
	fsid=%s
	run dir=%s
	mon initial members = %s
`
	adminClientConfigTemplate = `
[client.admin]
    keyring=%s
`
	bootstrapOSDClientConfigTemplate = `
[client.bootstrap-osd]
    keyring=%s
`
	monitorConfigTemplate = `
[mon.%s]
	name = %s
	mon addr = %s
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

type clusterInfo struct {
	FSID          string
	MonitorSecret string
	AdminSecret   string
}

func Bootstrap(cfg Config, executor proc.Executor) ([]*exec.Cmd, error) {
	// TODO: some of these operations should be done by only one member of the cluster, (e.g. leader election)

	// get an etcd client to coordinate with the rest of the cluster and load/save config
	etcdClient, err := kvstore.GetEtcdClient(cfg.EtcdURLs)
	if err != nil {
		return nil, fmt.Errorf("failed to get etcdClient: %+v", err)
	}

	// load any existing cluster info that may have previously been created
	cluster, err := loadClusterInfo(etcdClient)
	if err != nil {
		return nil, fmt.Errorf("failed to load cluster info: %+v", err)
	}

	if !isClusterInfoSet(cluster) {
		// the cluster info is not yet set, go ahead and set it now
		cluster, err = createClusterInfo()
		if err != nil {
			return nil, fmt.Errorf("failed to create cluster info: %+v", err)
		}

		log.Printf("Created new cluster info: %+v", cluster)
		err = saveClusterInfo(cluster, etcdClient)
		if err != nil {
			return nil, fmt.Errorf("failed to save new cluster info: %+v", err)
		}
	} else {
		// the cluster has already been created
		log.Printf("Cluster already exists: %+v", cluster)
	}

	if err := registerMonitors(cfg, etcdClient); err != nil {
		return nil, fmt.Errorf("failed to register monitors: %+v", err)
	}

	// wait for monitor registration to complete for all expected initial monitors
	if err := waitForMonitorRegistration(cfg, etcdClient); err != nil {
		return nil, fmt.Errorf("failed to wait for monitors to register: %+v", err)
	}

	// initialze the file systems for the monitors
	if err := makeMonitorFileSystems(cfg, cluster); err != nil {
		return nil, fmt.Errorf("failed to make monitor filesystems: %+v", err)
	}

	// run the monitors
	procs, err := runMonitors(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to run monitors: %+v", err)
	}

	log.Printf("successfully started monitors")

	// open an admin connection to the cluster
	user := "client.admin"
	adminConn, err := connectToCluster(cfg.ClusterName, user, getMonConfFilePath(cfg.MonNames[0]))
	if err != nil {
		return procs, err
	}
	defer adminConn.Shutdown()

	// wait for monitors to establish quorum
	if err := waitForMonitorQuorum(adminConn, cfg); err != nil {
		return procs, fmt.Errorf("failed to wait for monitors to establish quorum: %+v", err)
	}

	if len(cfg.Devices) > 0 {
		osdProcs, err := createOSDs(adminConn, cfg, cluster, executor)
		procs = append(procs, osdProcs...)
		if err != nil {
			return procs, fmt.Errorf("failed to create OSDs: %+v", err)
		}
	}

	return procs, nil
}

// attempt to load any previously created and saved cluster info
func loadClusterInfo(etcdClient etcd.KeysAPI) (clusterInfo, error) {
	resp, err := etcdClient.Get(context.Background(), path.Join(cephKey, "fsid"), nil)
	if err != nil {
		return handleLoadClusterInfoErr(err)
	}
	fsid := resp.Node.Value

	secretsKey := path.Join(cephKey, "_secrets")

	resp, err = etcdClient.Get(context.Background(), path.Join(secretsKey, "monitor"), nil)
	if err != nil {
		return handleLoadClusterInfoErr(err)
	}
	monSecret := resp.Node.Value

	resp, err = etcdClient.Get(context.Background(), path.Join(secretsKey, "admin"), nil)
	if err != nil {
		return handleLoadClusterInfoErr(err)
	}
	adminSecret := resp.Node.Value

	return clusterInfo{
		FSID:          fsid,
		MonitorSecret: monSecret,
		AdminSecret:   adminSecret,
	}, nil
}

func handleLoadClusterInfoErr(err error) (clusterInfo, error) {
	if kvstore.IsEtcdKeyNotFound(err) {
		return clusterInfo{}, nil
	}
	return clusterInfo{}, err
}

func isClusterInfoSet(c clusterInfo) bool {
	return c.FSID != "" && c.MonitorSecret != "" && c.AdminSecret != ""
}

// create new cluster info (FSID, shared keys)
func createClusterInfo() (clusterInfo, error) {
	fsid, err := cephd.NewFsid()
	if err != nil {
		return clusterInfo{}, fmt.Errorf("failed to create FSID: %+v", err)
	}

	monSecret, err := cephd.NewSecretKey()
	if err != nil {
		return clusterInfo{}, fmt.Errorf("failed to create monitor secret: %+v", err)
	}

	adminSecret, err := cephd.NewSecretKey()
	if err != nil {
		return clusterInfo{}, fmt.Errorf("failed to create admin secret: %+v", err)
	}

	return clusterInfo{
		FSID:          fsid,
		MonitorSecret: monSecret,
		AdminSecret:   adminSecret,
	}, nil
}

// save the given cluster info to the key value store
func saveClusterInfo(c clusterInfo, etcdClient etcd.KeysAPI) error {
	_, err := etcdClient.Set(context.Background(), path.Join(cephKey, "fsid"), c.FSID, nil)
	if err != nil {
		return err
	}

	secretsKey := path.Join(cephKey, "_secrets")

	_, err = etcdClient.Set(context.Background(), path.Join(secretsKey, "monitor"), c.MonitorSecret, nil)
	if err != nil {
		return err
	}

	_, err = etcdClient.Set(context.Background(), path.Join(secretsKey, "admin"), c.AdminSecret, nil)
	if err != nil {
		return err
	}

	return nil
}

// opens a connection to the cluster that can be used for management operations
func connectToCluster(clusterName, user, confFilePath string) (*cephd.Conn, error) {
	log.Printf("connecting to ceph cluster %s with user %s", clusterName, user)

	conn, err := cephd.NewConnWithClusterAndUser(clusterName, user)
	if err != nil {
		return nil, fmt.Errorf("failed to create rados connection for cluster %s and user %s: %+v", clusterName, user, err)
	}

	if err = conn.ReadConfigFile(confFilePath); err != nil {
		return nil, fmt.Errorf("failed to read config file for cluster %s: %+v", clusterName, err)
	}

	if err = conn.Connect(); err != nil {
		return nil, fmt.Errorf("failed to connect to cluster %s: %+v", clusterName, err)
	}

	return conn, nil
}

// register the names and endpoints of all monitors on this machine
func registerMonitors(cfg Config, etcdClient etcd.KeysAPI) error {
	port := 6790
	for i, monName := range cfg.MonNames {
		key := getMonitorEndpointKey(monName)
		val := fmt.Sprintf("%s:%d", cfg.PrivateIPv4, port+i)

		_, err := etcdClient.Set(context.Background(), key, val, nil)
		if err == nil || kvstore.IsEtcdNodeExist(err) {
			log.Printf("registered monitor %s endpoint of %s", monName, val)
		} else {
			return fmt.Errorf("failed to register mon %s endpoint: %+v", monName, err)
		}
	}

	return nil
}

func getMonitorEndpointKey(name string) string {
	return fmt.Sprintf(path.Join(cephKey, "mons/%s/endpoint"), name)
}

// wait for all expected initial monitors to register (report their names/endpoints)
func waitForMonitorRegistration(cfg Config, etcdClient etcd.KeysAPI) error {
	for i := range cfg.InitialMonitors {
		monName := cfg.InitialMonitors[i].Name
		key := getMonitorEndpointKey(monName)

		registered := false
		retryCount := 0
		retryMax := 40
		sleepTime := 5
		for {
			resp, err := etcdClient.Get(context.Background(), key, nil)
			if err == nil && resp != nil && resp.Node != nil && resp.Node.Value != "" {
				cfg.InitialMonitors[i].Endpoint = resp.Node.Value
				registered = true
			}

			if registered {
				log.Printf("monitor %s registered at %s", monName, cfg.InitialMonitors[i].Endpoint)
				break
			}

			retryCount++
			if retryCount > retryMax {
				return fmt.Errorf("exceeded max retry count waiting for monitor %s to register", monName)
			}

			<-time.After(time.Duration(sleepTime) * time.Second)
		}
	}

	return nil
}

// represents the response from a mon_status mon_command (subset of all available fields, only
// marshal ones we care about)
type MonStatusResponse struct {
	Quorum []int `json:"quorum"`
	MonMap struct {
		Mons []MonMapEntry `json:"mons"`
	} `json:"monmap"`
}

// represents an entry in the monitor map
type MonMapEntry struct {
	Name    string `json:"name"`
	Rank    int    `json:"rank"`
	Address string `json:"addr"`
}

// waits for all expected initial monitors to establish and join quorum
func waitForMonitorQuorum(adminConn *cephd.Conn, cfg Config) error {
	retryCount := 0
	retryMax := 20
	sleepTime := 5
	for {
		retryCount++
		if retryCount > retryMax {
			return fmt.Errorf("exceeded max retry count waiting for monitors to reach quorum")
		}

		if retryCount > 1 {
			// only sleep after the first time
			<-time.After(time.Duration(sleepTime) * time.Second)
		}

		// get the mon_status response that contains info about all monitors in the mon map and
		// their quorum status
		monStatusResp, err := getMonStatus(adminConn)
		if err != nil {
			log.Printf("failed to get mon_status, err: %+v", err)
			continue
		}

		// check if each of the initial monitors is in quorum
		allInQuorum := true
		for _, im := range cfg.InitialMonitors {
			// first get the initial monitors corresponding mon map entry
			var monMapEntry *MonMapEntry
			for i := range monStatusResp.MonMap.Mons {
				if im.Name == monStatusResp.MonMap.Mons[i].Name {
					monMapEntry = &monStatusResp.MonMap.Mons[i]
					break
				}
			}

			if monMapEntry == nil {
				// found an initial monitor that is not in the mon map, bail out of this retry
				log.Printf("failed to find initial monitor %s in mon map", im.Name)
				allInQuorum = false
				break
			}

			// using the current initial monitor's mon map entry, check to see if it's in the quorum list
			// (a list of monitor rank values)
			inQuorumList := false
			for _, q := range monStatusResp.Quorum {
				if monMapEntry.Rank == q {
					inQuorumList = true
					break
				}
			}

			if !inQuorumList {
				// found an initial monitor that is not in quorum, bail out of this retry
				log.Printf("initial monitor %s is not in quorum list", im.Name)
				allInQuorum = false
				break
			}
		}

		if allInQuorum {
			log.Printf("all initial monitors are in quorum")
			break
		}
	}

	return nil
}

// calls mon_status mon_command
func getMonStatus(adminConn *cephd.Conn) (MonStatusResponse, error) {
	monCommand := "mon_status"
	command, err := json.Marshal(map[string]string{"prefix": monCommand, "format": "json"})
	if err != nil {
		return MonStatusResponse{}, fmt.Errorf("command %s marshall failed: %+v", monCommand, err)
	}
	buf, _, err := adminConn.MonCommand(command)
	if err != nil {
		return MonStatusResponse{}, fmt.Errorf("mon_command failed: %+v", err)
	}
	var resp MonStatusResponse
	err = json.Unmarshal(buf, &resp)
	if err != nil {
		return MonStatusResponse{}, fmt.Errorf("unmarshall failed: %+v.  raw buffer response: %s", err, string(buf[:]))
	}

	return resp, nil
}

// creates and initializes the given monitors file systems
func makeMonitorFileSystems(cfg Config, c clusterInfo) error {
	for _, monName := range cfg.MonNames {
		// write the keyring to disk
		if err := writeMonitorKeyring(monName, c); err != nil {
			return err
		}

		// write the config file to disk
		if err := writeMonitorConfigFile(monName, cfg, c, getMonKeyringPath(monName)); err != nil {
			return err
		}

		// create monitor data dir
		monDataDir := getMonDataDirPath(monName)
		if err := os.MkdirAll(filepath.Dir(monDataDir), 0744); err != nil {
			fmt.Printf("failed to create monitor data directory at %s: %+v", monDataDir, err)
		}

		// call mon --mkfs in a child process
		err := proc.RunChildProcess(
			"mon",
			"--mkfs",
			fmt.Sprintf("--cluster=%s", cfg.ClusterName),
			fmt.Sprintf("--name=mon.%s", monName),
			fmt.Sprintf("--mon-data=%s", monDataDir),
			fmt.Sprintf("--conf=%s", getMonConfFilePath(monName)),
			fmt.Sprintf("--keyring=%s", getMonKeyringPath(monName)))
		if err != nil {
			return fmt.Errorf("failed mon %s --mkfs: %+v", monName, err)
		}
	}

	return nil
}

// writes the monitor keyring to disk
func writeMonitorKeyring(monName string, c clusterInfo) error {
	keyring := fmt.Sprintf(monitorKeyringTemplate, c.MonitorSecret, c.AdminSecret)
	keyringPath := getMonKeyringPath(monName)
	if err := os.MkdirAll(filepath.Dir(keyringPath), 0744); err != nil {
		return fmt.Errorf("failed to create keyring directory for %s: %+v", keyringPath, err)
	}
	if err := ioutil.WriteFile(keyringPath, []byte(keyring), 0644); err != nil {
		return fmt.Errorf("failed to write monitor keyring to %s: %+v", keyringPath, err)
	}

	return nil
}

// generates and writes the monitor config file to disk
func writeMonitorConfigFile(monName string, cfg Config, c clusterInfo, adminKeyringPath string) error {
	var contentBuffer bytes.Buffer

	if err := writeGlobalConfigFileSection(&contentBuffer, cfg, c, getMonRunDirPath(monName)); err != nil {
		return fmt.Errorf("failed to write mon %s global config section, %+v", monName, err)
	}

	_, err := contentBuffer.WriteString(fmt.Sprintf(adminClientConfigTemplate, adminKeyringPath))
	if err != nil {
		return fmt.Errorf("failed to write mon %s admin client config section, %+v", monName, err)
	}

	if err := writeInitialMonitorsConfigFileSections(&contentBuffer, cfg); err != nil {
		return fmt.Errorf("failed to write mon %s initial monitor config sections, %+v", monName, err)
	}

	// write the entire config to disk
	if err := writeFile(getMonConfFilePath(monName), contentBuffer); err != nil {
		return err
	}

	return nil
}

// runs all the given monitors in child processes
func runMonitors(cfg Config) ([]*exec.Cmd, error) {
	procs := make([]*exec.Cmd, len(cfg.MonNames))

	for i, monName := range cfg.MonNames {
		// find the current monitor's endpoint
		var monEndpoint string
		for i := range cfg.InitialMonitors {
			if cfg.InitialMonitors[i].Name == monName {
				monEndpoint = cfg.InitialMonitors[i].Endpoint
				break
			}
		}
		if monEndpoint == "" {
			return nil, fmt.Errorf("failed to find endpoint for mon %s", monName)
		}

		// start the monitor daemon in the foreground with the given config
		log.Printf("starting monitor %s", monName)
		cmd, err := proc.StartChildProcess(
			"mon",
			"--foreground",
			fmt.Sprintf("--cluster=%v", cfg.ClusterName),
			fmt.Sprintf("--name=mon.%v", monName),
			fmt.Sprintf("--mon-data=%s", getMonDataDirPath(monName)),
			fmt.Sprintf("--conf=%s", getMonConfFilePath(monName)),
			fmt.Sprintf("--public-addr=%v", monEndpoint))
		if err != nil {
			return nil, fmt.Errorf("failed to start monitor %s: %+v", monName, err)
		}

		procs[i] = cmd
	}

	return procs, nil
}

// get the path of a given monitor's run dir
func getMonRunDirPath(monName string) string {
	return fmt.Sprintf("/tmp/%s", monName)
}

// get the path of a given monitor's config file
func getMonConfFilePath(monName string) string {
	return filepath.Join(getMonRunDirPath(monName), "config")
}

// get the path of a given monitor's keyring
func getMonKeyringPath(monName string) string {
	return filepath.Join(getMonRunDirPath(monName), "keyring")
}

// get the path of a given monitor's data dir
func getMonDataDirPath(monName string) string {
	return filepath.Join(getMonRunDirPath(monName), fmt.Sprintf("mon.%s", monName))
}

// create and initalize OSDs for all the devices specified in the given config
func createOSDs(adminConn *cephd.Conn, cfg Config, c clusterInfo, executor proc.Executor) ([]*exec.Cmd, error) {
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
func writeBootstrapOSDConfFile(cfg Config, c clusterInfo, bootstrapOSDKeyringPath string) error {
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
			if _, err := os.Stat(fmt.Sprintf("/dev/disk/by-uuid/%s", diskUUID)); err == nil {
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
	if err := executor.ExecuteCommand(cmd, "sudo", "mount", fmt.Sprintf("/dev/disk/by-uuid/%s", diskUUID), mountPath); err != nil {
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
func writeOSDConfFile(cfg Config, c clusterInfo, osdDataPath string, osdID int) error {
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

func getOSDConfFilePath(osdDataPath string) string {
	return filepath.Join(osdDataPath, "tmp_config")
}

func getOSDKeyringPath(osdDataPath string) string {
	return filepath.Join(osdDataPath, "keyring")
}

func getOSDJournalPath(osdDataPath string) string {
	return filepath.Join(osdDataPath, "journal")
}

func getOSDTempMonMapPath(osdDataPath string) string {
	return filepath.Join(osdDataPath, "tmp", "activate.monmap")
}

func getBootstrapOSDDir() string {
	return "/tmp/bootstrap-osd"
}

func getBootstrapOSDKeyringPath(clusterName string) string {
	return fmt.Sprintf("%s/%s", getBootstrapOSDDir(), fmt.Sprintf("%s.keyring", clusterName))
}

func getBootstrapOSDConfFilePath() string {
	return fmt.Sprintf("%s/tmp_config", getBootstrapOSDDir())
}
