package castled

import (
	"bytes"
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
	run dir=/tmp/mon
	mon initial members = %s
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

func Bootstrap(cfg Config) (*exec.Cmd, error) {
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

	if err := registerMonitor(cfg, etcdClient); err != nil {
		return nil, fmt.Errorf("failed to register monitor: %+v", err)
	}

	// wait for monitor registration to complete for all expected initial monitors
	if err := waitForMonitorRegistration(cfg, etcdClient); err != nil {
		return nil, fmt.Errorf("failed to wait for monitors to register: %+v", err)
	}

	if err := makeMonitorFileSystem(cfg, cluster); err != nil {
		return nil, fmt.Errorf("failed to make monitor filesystem: %+v", err)
	}

	proc, err := runMonitor(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to run monitor: %+v", err)
	}

	log.Printf("successfully started monitor %s", cfg.MonName)

	return proc, nil
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

func registerMonitor(cfg Config, etcdClient etcd.KeysAPI) error {
	key := getMonitorEndpointKey(cfg.MonName)
	val := fmt.Sprintf("%s:%d", cfg.PrivateIPv4, 6790)

	_, err := etcdClient.Set(context.Background(), key, val, nil)
	if err == nil || kvstore.IsEtcdNodeExist(err) {
		log.Printf("registered monitor %s endpoint of %s", cfg.MonName, val)
	}
	return err
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

func makeMonitorFileSystem(cfg Config, c clusterInfo) error {
	// write the keyring to disk
	keyringPath, err := writeMonitorKeyring(c)
	if err != nil {
		return err
	}

	// write the config file to disk
	monConfFilePath, err := writeMonitorConfigFile(cfg, c)
	if err != nil {
		return err
	}

	// create monitor data dir
	monDataDir := fmt.Sprintf("/tmp/mon/mon.%s", cfg.MonName)
	if err := os.MkdirAll(filepath.Dir(monDataDir), 0744); err != nil {
		fmt.Printf("failed to create monitor data directory at %s: %+v", monDataDir, err)
	}

	// call mon --mkfs in a child process
	err = runChildProcess(
		"mon",
		"--mkfs",
		fmt.Sprintf("--cluster=%s", cfg.ClusterName),
		fmt.Sprintf("--name=mon.%s", cfg.MonName),
		fmt.Sprintf("--mon-data=%s", monDataDir),
		fmt.Sprintf("--conf=%s", monConfFilePath),
		fmt.Sprintf("--keyring=%s", keyringPath))
	if err != nil {
		return fmt.Errorf("failed mon --mkfs: %+v", err)
	}

	return nil
}

func writeMonitorKeyring(c clusterInfo) (string, error) {
	keyring := fmt.Sprintf(monitorKeyringTemplate, c.MonitorSecret, c.AdminSecret)
	keyringPath := "/tmp/mon/tmp_keyring"
	if err := os.MkdirAll(filepath.Dir(keyringPath), 0744); err != nil {
		return "", fmt.Errorf("failed to create keyring directory for %s: %+v", keyringPath, err)
	}
	if err := ioutil.WriteFile(keyringPath, []byte(keyring), 0644); err != nil {
		return "", fmt.Errorf("failed to write monitor keyring to %s: %+v", keyringPath, err)
	}

	return keyringPath, nil
}

func writeMonitorConfigFile(cfg Config, c clusterInfo) (string, error) {
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
		strings.Join(initialMonMembers, " ")))
	if err != nil {
		return "", fmt.Errorf("failed to write global config section, %+v", err)
	}

	// write the config for each individual monitor member of the cluster to the content buffer
	for i := range cfg.InitialMonitors {
		mon := cfg.InitialMonitors[i]
		_, err := contentBuffer.WriteString(fmt.Sprintf(monitorConfigTemplate, mon.Name, mon.Name, mon.Endpoint))
		if err != nil {
			return "", fmt.Errorf("failed to write monitor config section for mon %s, %+v", mon.Name, err)
		}
	}

	// write the entire config to disk
	monConfFilePath := "/tmp/mon/tmp_config"
	if err := os.MkdirAll(filepath.Dir(monConfFilePath), 0744); err != nil {
		fmt.Printf("failed to create monitor config file directory for %s: %+v", monConfFilePath, err)
	}
	if err := ioutil.WriteFile(monConfFilePath, contentBuffer.Bytes(), 0644); err != nil {
		return "", fmt.Errorf("failed to write monitor config file to %s: %+v", monConfFilePath, err)
	}

	return monConfFilePath, nil
}

func runMonitor(cfg Config) (*exec.Cmd, error) {
	monConfFilePath := "/tmp/mon/tmp_config"
	monDataDir := fmt.Sprintf("/tmp/mon/mon.%s", cfg.MonName)

	var monEndpoint string
	for i := range cfg.InitialMonitors {
		if cfg.InitialMonitors[i].Name == cfg.MonName {
			monEndpoint = cfg.InitialMonitors[i].Endpoint
		}
	}
	if monEndpoint == "" {
		return nil, fmt.Errorf("failed to find endpoint for mon %s", cfg.MonName)
	}

	// start the monitor daemon in the foreground with the given config
	log.Printf("starting monitor %s", cfg.MonName)
	cmd, err := startChildProcess(
		"mon",
		"--foreground",
		fmt.Sprintf("--cluster=%v", cfg.ClusterName),
		fmt.Sprintf("--name=mon.%v", cfg.MonName),
		fmt.Sprintf("--mon-data=%s", monDataDir),
		fmt.Sprintf("--conf=%s", monConfFilePath),
		fmt.Sprintf("--public-addr=%v", monEndpoint))
	if err != nil {
		return nil, fmt.Errorf("failed to start monitor %s: %+v", cfg.MonName, err)
	}

	return cmd, nil
}
