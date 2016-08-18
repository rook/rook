package castled

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	etcd "github.com/coreos/etcd/client"
	"golang.org/x/net/context"

	"github.com/quantum/clusterd/pkg/orchestrator"
	"github.com/quantum/clusterd/pkg/store"
)

type monAgent struct {
	cluster     clusterInfo
	procMan     *orchestrator.ProcessManager
	privateIPv4 string
	etcdClient  etcd.KeysAPI
}

func (a *monAgent) ConfigureAgent(context *orchestrator.ClusterContext, changeList []orchestrator.ChangeElement) error {
	monName := "mon1" // FIX
	port := 6790
	if err := a.registerMonitor(monName, port); err != nil {
		return cluster, nil, fmt.Errorf("failed to register monitors: %+v", err)
	}

	// wait for monitor registration to complete for all expected initial monitors
	if err := a.waitForMonitorRegistration(); err != nil {
		return cluster, nil, fmt.Errorf("failed to wait for monitors to register: %+v", err)
	}

	// initialze the file systems for the monitors
	if err := a.makeMonitorFileSystem(monName); err != nil {
		return cluster, nil, fmt.Errorf("failed to make monitor filesystems: %+v", err)
	}

	// run the monitors
	procs, err := a.runMonitor()
	if err != nil {
		return cluster, nil, fmt.Errorf("failed to run monitors: %+v", err)
	}

	log.Printf("successfully started monitor %s", monName)

	return err
}

func (m *monAgent) DestroyAgent(context *orchestrator.ClusterContext) error {
	return nil
}

// register the names and endpoints of all monitors on this machine
func (a *monAgent) registerMonitor(monName string, port int) error {

	key := getMonitorEndpointKey(monName)
	val := fmt.Sprintf("%s:%d", a.privateIPv4, port)

	_, err := a.etcdClient.Set(context.Background(), key, val, nil)
	if err == nil || store.IsEtcdNodeExist(err) {
		log.Printf("registered monitor %s endpoint of %s", monName, val)
	} else {
		return fmt.Errorf("failed to register mon %s endpoint: %+v", monName, err)
	}

	return nil
}

// wait for all expected initial monitors to register (report their names/endpoints)
func (a *monAgent) waitForMonitorRegistration(monName string) error {
	key := getMonitorEndpointKey(monName)

	registered := false
	retryCount := 0
	retryMax := 40
	sleepTime := 5
	for {
		resp, err := a.etcdClient.Get(context.Background(), key, nil)
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
func (a *monAgent) makeMonitorFileSystem(monName string) error {
	// write the keyring to disk
	if err := a.writeMonitorKeyring(monName); err != nil {
		return err
	}

	// write the config file to disk
	if err := a.writeMonitorConfigFile(monName, getMonKeyringPath(monName)); err != nil {
		return err
	}

	// create monitor data dir
	monDataDir := getMonDataDirPath(monName)
	if err := os.MkdirAll(filepath.Dir(monDataDir), 0744); err != nil {
		fmt.Printf("failed to create monitor data directory at %s: %+v", monDataDir, err)
	}

	// call mon --mkfs in a child process
	err := a.procMan.Start(
		"mon",
		"--mkfs",
		fmt.Sprintf("--cluster=%s", a.clusterName),
		fmt.Sprintf("--name=mon.%s", monName),
		fmt.Sprintf("--mon-data=%s", monDataDir),
		fmt.Sprintf("--conf=%s", getMonConfFilePath(monName)),
		fmt.Sprintf("--keyring=%s", getMonKeyringPath(monName)))
	if err != nil {
		return fmt.Errorf("failed mon %s --mkfs: %+v", monName, err)
	}

	return nil
}
