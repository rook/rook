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
	"strings"
	"time"

	"github.com/quantum/castle/pkg/cephd"
	"github.com/quantum/castle/pkg/kvstore"

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

[client.admin]
    keyring=%s
`
	monitorConfigTemplate = `
[mon.%s]
	name = %s
	mon addr = %s
`
)

type clusterInfo struct {
	FSID          string
	MonitorSecret string
	AdminSecret   string
}

func Bootstrap(cfg Config) ([]*exec.Cmd, error) {
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

	for _, device := range cfg.Devices {
		// for each allowed device, bootstrap an OSD on it
		log.Printf("initializing device %s", device)
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
		err = runChildProcess(
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

func writeMonitorConfigFile(monName string, cfg Config, c clusterInfo, keyringPath string) (string, error) {
	// extract a list of just the monitor names, which will populate the "mon initial members"
	// global config field
	initialMonMembers := make([]string, len(cfg.InitialMonitors))
	for i := range cfg.InitialMonitors {
		initialMonMembers[i] = cfg.InitialMonitors[i].Name
	}

	// write the global config section to the content buffer
	var contentBuffer bytes.Buffer
	_, err := contentBuffer.WriteString(fmt.Sprintf(
		globalConfigTemplate,
		c.FSID,
		getMonRunDirPath(monName),
		strings.Join(initialMonMembers, " "),
		keyringPath))
	if err != nil {
		return "", fmt.Errorf("failed to write mon %s global config section, %+v", monName, err)
	}

	// write the config for each individual monitor member of the cluster to the content buffer
	for i := range cfg.InitialMonitors {
		mon := cfg.InitialMonitors[i]
		_, err := contentBuffer.WriteString(fmt.Sprintf(monitorConfigTemplate, mon.Name, mon.Name, mon.Endpoint))
		if err != nil {
			return "", fmt.Errorf("failed to write mon %s monitor config section for mon %s, %+v", monName, mon.Name, err)
		}
	}

	// write the entire config to disk
	monConfFilePath := getMonConfFilePath(monName)
	if err := os.MkdirAll(filepath.Dir(monConfFilePath), 0744); err != nil {
		fmt.Printf("failed to create monitor config file directory for %s: %+v", monConfFilePath, err)
	}
	if err := ioutil.WriteFile(monConfFilePath, contentBuffer.Bytes(), 0644); err != nil {
		return "", fmt.Errorf("failed to write monitor config file to %s: %+v", monConfFilePath, err)
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
		cmd, err := startChildProcess(
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
