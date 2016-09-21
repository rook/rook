package castled

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"regexp"

	"github.com/quantum/castle/pkg/cephclient"
	"github.com/quantum/castle/pkg/clusterd"
	"github.com/quantum/castle/pkg/proc"
	"github.com/quantum/castle/pkg/util"
)

const (
	monitorAgentName = "monitor"
)

type monAgent struct {
	factory cephclient.ConnectionFactory
}

func (a *monAgent) Name() string {
	return monitorAgentName
}

func (a *monAgent) ConfigureLocalService(context *clusterd.Context) error {

	// check if the monitor is in the desired state for this node
	key := path.Join(cephKey, monitorAgentName, desiredKey, context.NodeID)
	monDesired, err := util.EtcdDirExists(context.EtcdClient, key)
	if err != nil {
		return err
	}
	if !monDesired {
		return nil
	}

	cluster, err := LoadClusterInfo(context.EtcdClient)
	if err != nil {
		return fmt.Errorf("failed to load cluster info: %+v", err)
	}

	var ok bool
	var monitor *CephMonitorConfig
	if monitor, ok = cluster.Monitors[context.NodeID]; !ok {
		return fmt.Errorf("failed to find monitor during config")
	}

	// initialze the file system for the monitor
	if err := a.makeMonitorFileSystem(context, cluster, monitor.Name); err != nil {
		return fmt.Errorf("failed to make monitor filesystems: %+v", err)
	}

	// run the monitor
	err = a.runMonitor(context, cluster, monitor)
	if err != nil {
		return fmt.Errorf("failed to run monitors: %+v", err)
	}

	log.Printf("successfully started monitor %s", monitor.Name)

	return err
}

func (a *monAgent) DestroyLocalService(context *clusterd.Context) error {
	return nil
}

// creates and initializes the given monitors file systems
func (a *monAgent) makeMonitorFileSystem(context *clusterd.Context, cluster *ClusterInfo, monName string) error {
	// write the keyring to disk
	if err := writeMonitorKeyring(monName, cluster); err != nil {
		return err
	}

	// write the config file to disk
	confFilePath, err := generateConfigFile(cluster, getMonRunDirPath(monName), "admin", getMonKeyringPath(monName))
	if err != nil {
		return err
	}

	// create monitor data dir
	monDataDir := getMonDataDirPath(monName)
	if err := os.MkdirAll(filepath.Dir(monDataDir), 0744); err != nil {
		fmt.Printf("failed to create monitor data directory at %s: %+v", monDataDir, err)
	}

	// call mon --mkfs in a child process
	err = context.ProcMan.Run(
		"mon",
		"--mkfs",
		fmt.Sprintf("--cluster=%s", cluster.Name),
		fmt.Sprintf("--name=mon.%s", monName),
		fmt.Sprintf("--mon-data=%s", monDataDir),
		fmt.Sprintf("--conf=%s", confFilePath),
		fmt.Sprintf("--keyring=%s", getMonKeyringPath(monName)))
	if err != nil {
		return fmt.Errorf("failed mon %s --mkfs: %+v", monName, err)
	}

	return nil
}

// runs the monitor in a child process
func (a *monAgent) runMonitor(context *clusterd.Context, cluster *ClusterInfo, monitor *CephMonitorConfig) error {
	if monitor.Endpoint == "" {
		return fmt.Errorf("missing endpoint for mon %s", monitor.Name)
	}

	// start the monitor daemon in the foreground with the given config
	log.Printf("starting monitor %s", monitor.Name)
	monNameArg := fmt.Sprintf("--name=mon.%s", monitor.Name)
	err := context.ProcMan.Start(
		"mon",
		regexp.QuoteMeta(monNameArg),
		proc.ReuseExisting,
		"--foreground",
		fmt.Sprintf("--cluster=%s", cluster.Name),
		monNameArg,
		fmt.Sprintf("--mon-data=%s", getMonDataDirPath(monitor.Name)),
		fmt.Sprintf("--conf=%s", getConfFilePath(getMonRunDirPath(monitor.Name), cluster.Name)),
		fmt.Sprintf("--public-addr=%s", monitor.Endpoint))
	if err != nil {
		return fmt.Errorf("failed to start monitor %s: %v", monitor.Name, err)
	}

	return nil
}

// writes the monitor keyring to disk
func writeMonitorKeyring(monName string, c *ClusterInfo) error {
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
