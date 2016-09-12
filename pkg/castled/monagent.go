package castled

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"time"

	ctx "golang.org/x/net/context"

	"github.com/quantum/clusterd/pkg/orchestrator"
	"github.com/quantum/clusterd/pkg/proc"
	"github.com/quantum/clusterd/pkg/util"
)

const (
	monitorAgentName = "monitor"
)

type monAgent struct {
}

func (a *monAgent) Name() string {
	return monitorAgentName
}

func (a *monAgent) ConfigureLocalService(context *orchestrator.ClusterContext) error {

	// check if the monitor is in the desired state for this node
	key := path.Join(cephKey, monitorAgentName, desiredKey, context.NodeID)
	monDesired, err := util.EtcdDirExists(context.EtcdClient, key)
	if err != nil {
		return err
	}
	if !monDesired {
		return nil
	}

	cluster, err := loadClusterInfo(context.EtcdClient)
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

func (a *monAgent) DestroyLocalService(context *orchestrator.ClusterContext) error {
	return nil
}

// wait for all expected initial monitors to register (report their names/endpoints)
func (a *monAgent) waitForMonitorRegistration(context *orchestrator.ClusterContext, monName string) error {
	key := getMonitorEndpointKey(monName)

	retryCount := 0
	retryMax := 40
	sleepTime := 5
	for {
		resp, err := context.EtcdClient.Get(ctx.Background(), key, nil)
		if err == nil && resp != nil && resp.Node != nil && resp.Node.Value != "" {
			log.Printf("monitor %s registered at %s", monName, resp.Node.Value)
			break
		}

		retryCount++
		if retryCount > retryMax {
			return fmt.Errorf("exceeded max retry count waiting for monitor %s to register", monName)
		}

		<-time.After(time.Duration(sleepTime) * time.Second)
	}

	return nil
}

// creates and initializes the given monitors file systems
func (a *monAgent) makeMonitorFileSystem(context *orchestrator.ClusterContext, cluster *clusterInfo, monName string) error {
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
func (a *monAgent) runMonitor(context *orchestrator.ClusterContext, cluster *clusterInfo, monitor *CephMonitorConfig) error {
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
func writeMonitorKeyring(monName string, c *clusterInfo) error {
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
