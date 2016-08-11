package castled

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"time"

	"github.com/quantum/castle/pkg/cephd"
	"github.com/quantum/castle/pkg/kvstore"
	"github.com/quantum/castle/pkg/proc"

	etcd "github.com/coreos/etcd/client"
	"golang.org/x/net/context"
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

type MonStatusResponse struct {
	Quorum []int `json:"quorum"`
	MonMap struct {
		Mons []MonMapEntry `json:"mons"`
	} `json:"monmap"`
}

type MonMapEntry struct {
	Name    string `json:"name"`
	Rank    int    `json:"rank"`
	Address string `json:"addr"`
}

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

func makeMonitorFileSystems(cfg Config, c clusterInfo) error {
	for _, monName := range cfg.MonNames {
		// write the keyring to disk
		keyringPath, err := writeMonitorKeyring(monName, c)
		if err != nil {
			return err
		}

		// write the config file to disk
		monConfFilePath, err := writeMonitorConfigFile(monName, cfg, c, keyringPath)
		if err != nil {
			return err
		}

		// create monitor data dir
		monDataDir := fmt.Sprintf("/tmp/%s/mon.%s", monName, monName)
		if err := os.MkdirAll(filepath.Dir(monDataDir), 0744); err != nil {
			fmt.Printf("failed to create monitor data directory at %s: %+v", monDataDir, err)
		}

		// call mon --mkfs in a child process
		err = proc.RunChildProcess(
			"mon",
			"--mkfs",
			fmt.Sprintf("--cluster=%s", cfg.ClusterName),
			fmt.Sprintf("--name=mon.%s", monName),
			fmt.Sprintf("--mon-data=%s", monDataDir),
			fmt.Sprintf("--conf=%s", monConfFilePath),
			fmt.Sprintf("--keyring=%s", keyringPath))
		if err != nil {
			return fmt.Errorf("failed mon %s --mkfs: %+v", monName, err)
		}
	}

	return nil
}

func writeMonitorKeyring(monName string, c clusterInfo) (string, error) {
	keyring := fmt.Sprintf(monitorKeyringTemplate, c.MonitorSecret, c.AdminSecret)
	keyringPath := fmt.Sprintf("/tmp/%s/tmp_keyring", monName)
	if err := os.MkdirAll(filepath.Dir(keyringPath), 0744); err != nil {
		return "", fmt.Errorf("failed to create keyring directory for %s: %+v", keyringPath, err)
	}
	if err := ioutil.WriteFile(keyringPath, []byte(keyring), 0644); err != nil {
		return "", fmt.Errorf("failed to write monitor keyring to %s: %+v", keyringPath, err)
	}

	return keyringPath, nil
}

func writeMonitorConfigFile(monName string, cfg Config, c clusterInfo, adminKeyringPath string) (string, error) {
	var contentBuffer bytes.Buffer

	if err := writeGlobalConfigFileSection(&contentBuffer, cfg, c, getMonRunDirPath(monName)); err != nil {
		return "", fmt.Errorf("failed to write mon %s global config section, %+v", monName, err)
	}

	_, err := contentBuffer.WriteString(fmt.Sprintf(adminClientConfigTemplate, adminKeyringPath))
	if err != nil {
		return "", fmt.Errorf("failed to write mon %s admin client config section, %+v", monName, err)
	}

	if err := writeInitialMonitorsConfigFileSections(&contentBuffer, cfg); err != nil {
		return "", fmt.Errorf("failed to write mon %s initial monitor config sections, %+v", monName, err)
	}

	// write the entire config to disk
	monConfFilePath := getMonConfFilePath(monName)
	if err := writeFile(monConfFilePath, contentBuffer); err != nil {
		return "", err
	}

	return monConfFilePath, nil
}

func runMonitors(cfg Config) ([]*exec.Cmd, error) {
	procs := make([]*exec.Cmd, len(cfg.MonNames))

	for i, monName := range cfg.MonNames {
		monConfFilePath := getMonConfFilePath(monName)
		monDataDir := fmt.Sprintf("/tmp/%s/mon.%s", monName, monName)

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
			fmt.Sprintf("--mon-data=%s", monDataDir),
			fmt.Sprintf("--conf=%s", monConfFilePath),
			fmt.Sprintf("--public-addr=%v", monEndpoint))
		if err != nil {
			return nil, fmt.Errorf("failed to start monitor %s: %+v", monName, err)
		}

		procs[i] = cmd
	}

	return procs, nil
}

func getMonRunDirPath(monName string) string {
	return fmt.Sprintf("/tmp/%s", monName)
}

func getMonConfFilePath(monName string) string {
	return filepath.Join(getMonRunDirPath(monName), "tmp_config")
}

func createOSDs(adminConn *cephd.Conn, cfg Config, c clusterInfo, executor proc.Executor) ([]*exec.Cmd, error) {
	if err := createOSDBootstrapKeyring(adminConn, cfg.ClusterName, executor); err != nil {
		return nil, err
	}

	bootstrapOSDConfFilePath, err := writeBootstrapOSDConfFile(cfg, c, getBootstrapOSDKeyringPath(cfg.ClusterName))
	if err != nil {
		return nil, fmt.Errorf("failed to write bootstrap-osd conf file at %s: %+v", bootstrapOSDConfFilePath, err)
	}

	bootstrapConn, err := connectToCluster(cfg.ClusterName, "client.bootstrap-osd", bootstrapOSDConfFilePath)
	if err != nil {
		return nil, err
	}
	defer bootstrapConn.Shutdown()

	// initialize all the desired OSD volumes
	for i, device := range cfg.Devices {
		done, err := formatOSD(device, cfg.ForceFormat, executor)
		if err != nil {
			// attempting to format the volume failed, bail out with error
			return nil, err
		} else if !done {
			// the formatting was not done, probably because the drive already has a filesystem.
			// just move on to the next OSD.
			continue
		}

		osdDataDir := fmt.Sprintf("/tmp/osd%d", i)
		if err := mountOSD(device, osdDataDir, executor); err != nil {
			return nil, err
		}

		/*

			// find the OSD data dir
			osdDataPath, err := findOSDDataPath(osd.DataDir, ceph.ClusterName)
			if err != nil {
				return err
			}
			if _, err := os.Stat(filepath.Join(osdDataPath, "whoami")); os.IsNotExist(err) {
				// osd_data_dir/whoami does not exist yet
				log.Printf("initializing the osd directory %s", osd.DataDir)
				osdUUID, err := uuid.NewV4()
				if err != nil {
					return fmt.Errorf("failed to generate UUID for %s: %+v", osd.DataDir, err)
				}

				osdID, err := createOSD(bootstrapConn, osdUUID)
				if err != nil {
					return err
				}

				log.Printf("successfully created OSD %s with ID %d at %s", osdUUID.String(), osdID, osd.DataDir)

				if err := addOSDKeyringToConf(ceph.ClusterName, osdID); err != nil {
					return err
				}

				osdDataPath = filepath.Join(osd.DataDir, fmt.Sprintf("%s-%d", ceph.ClusterName, osdID))
				if err := os.MkdirAll(osdDataPath, 0777); err != nil {
					return fmt.Errorf("failed to create OSD data dir at %s, %+v", osdDataPath, err)
				}
				if err := chownCephPath(osdDataPath, executor); err != nil {
					return err
				}

				// create a link from the default location of the OSD directory to our mounted volume
				osdDefaultPath := filepath.Join(string(filepath.Separator), "var", "lib", "ceph", "osd", fmt.Sprintf("%s-%d", ceph.ClusterName, osdID))
				if err := os.Symlink(osdDataPath, osdDefaultPath); err != nil {
					return fmt.Errorf("failed to link OSD default path %s to %s: %+v", osdDefaultPath, osdDataPath, err)
				}

				monMapRaw, err := getMonMap(bootstrapConn)
				if err != nil {
					return fmt.Errorf("failed to get mon map: %+v", err)
				}

				if err := createOSDFileSystem(ceph, osdID, osdUUID, osdDefaultPath, monMapRaw, executor); err != nil {
					return err
				}

				if err := addOSDAuth(bootstrapConn, osdID, osdDefaultPath); err != nil {
					return err
				}

				// open a connection using the OSDs creds
				osdConn, err := connectToCluster(ceph.ClusterName, fmt.Sprintf("osd.%d", osdID))
				if err != nil {
					return err
				}
				defer osdConn.Shutdown()

				if err := addOSDToCrushMap(osdConn, osdID, osd.DataDir); err != nil {
					return err
				}

				if err := chownCephPath(osdDefaultPath+string(filepath.Separator), executor); err != nil {
					return err
				}

				// everything should be configured for the OSD, process its units now
				osdUnits := OSDUnits(ceph, osdID)
				if err := ProcessUnits(osdUnits, um); err != nil {
					return err
				}
			}
		*/
	}

	return nil, nil
}

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

		// create the bootstrap-osd keyring
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

func getBootstrapOSDDir() string {
	return "/tmp/bootstrap-osd"
}

func getBootstrapOSDKeyringPath(clusterName string) string {
	return fmt.Sprintf("%s/%s", getBootstrapOSDDir(), fmt.Sprintf("%s.keyring", clusterName))
}

func getBootstrapOSDConfFilePath() string {
	return fmt.Sprintf("%s/tmp_config", getBootstrapOSDDir())
}

func writeBootstrapOSDConfFile(cfg Config, c clusterInfo, bootstrapOSDKeyringPath string) (string, error) {
	var contentBuffer bytes.Buffer
	bootstrapOSDConfFilePath := getBootstrapOSDConfFilePath()

	if err := writeGlobalConfigFileSection(&contentBuffer, cfg, c, getBootstrapOSDDir()); err != nil {
		return bootstrapOSDConfFilePath, fmt.Errorf("failed to write bootstrap-osd global config section, %+v", err)
	}

	_, err := contentBuffer.WriteString(fmt.Sprintf(bootstrapOSDClientConfigTemplate, bootstrapOSDKeyringPath))
	if err != nil {
		return bootstrapOSDConfFilePath, fmt.Errorf("failed to write bootstrap-osd client config section, %+v", err)
	}

	if err := writeInitialMonitorsConfigFileSections(&contentBuffer, cfg); err != nil {
		return bootstrapOSDConfFilePath, fmt.Errorf("failed to write bootstrap-osd initial monitor config sections, %+v", err)
	}

	// write the entire config to disk
	if err := writeFile(bootstrapOSDConfFilePath, contentBuffer); err != nil {
		return bootstrapOSDConfFilePath, err
	}

	return bootstrapOSDConfFilePath, nil
}

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
		log.Printf("WARNING: device %s already formatted with %s, but forcing a format!!!", device, devFS)
	}

	if devFS == "" || forceFormat {
		cmd = fmt.Sprintf("format %s", device)
		err = executor.ExecuteCommand(cmd, "sudo", "/usr/sbin/mkfs.btrfs", "-f", "-m", "single", "-n", "32768", fmt.Sprintf("/dev/%s", device))
		if err != nil {
			return false, fmt.Errorf("command %s failed: %+v", cmd, err)
		}
	} else {
		log.Printf("device %s already formatted with %s, cannot use for OSD", device, devFS)
		return false, nil
	}

	return true, nil
}

func mountOSD(device string, mountPath string, executor proc.Executor) error {
	cmd := fmt.Sprintf("lsblk %s", device)
	var diskUUID string

	retryCount := 0
	retryMax := 10
	sleepTime := 2
	for {
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

	return nil
}
