/*
Copyright 2016 The Rook Authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package mon

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util"
	"github.com/rook/rook/pkg/util/proc"
)

const (
	monitorAgentName       = "monitor"
	monitorKeyringTemplate = `
[mon.]
	key = %s
	caps mon = "allow *"` + AdminKeyringTemplate

	AdminKeyringTemplate = `
[client.admin]
	key = %s
	auid = 0
	caps mds = "allow"
	caps mon = "allow *"
	caps osd = "allow *"
`
)

type agent struct {
	context *clusterd.Context
	monProc *proc.MonitoredProc
}

func NewAgent() clusterd.ServiceAgent {
	return &agent{}
}

func (a *agent) Name() string {
	return monitorAgentName
}

func (a *agent) Initialize(context *clusterd.Context) error {
	a.context = context
	return nil
}

func (a *agent) ConfigureLocalService(context *clusterd.Context) error {
	// check if the monitor is in the desired state for this node
	key := path.Join(CephKey, monitorAgentName, clusterd.DesiredKey, context.NodeID)
	monDesired, err := util.EtcdDirExists(context.EtcdClient, key)
	if err != nil {
		return err
	}
	if !monDesired {
		return a.DestroyLocalService(context)
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

	logger.Infof("successfully started monitor %s", monitor.Name)

	return err
}

// stops and removes the monitor from this node
func (a *agent) DestroyLocalService(context *clusterd.Context) error {
	if a.monProc == nil {
		logger.Debugf("no need to stop a monitor that is not running")
		return nil
	}

	if err := a.monProc.Stop(); err != nil {
		logger.Errorf("failed to stop mon. %v", err)
		return err
	}

	logger.Debug("stopped ceph monitor")
	a.monProc = nil

	// TODO: Clean up the monitor folder

	return nil
}

// creates and initializes the given monitors file systems
func (a *agent) makeMonitorFileSystem(context *clusterd.Context, cluster *ClusterInfo, monName string) error {
	// write the keyring to disk
	if err := writeMonKeyring(context, cluster, monName); err != nil {
		return err
	}

	// write the config file to disk
	confFilePath, err := GenerateConnectionConfigFile(context, cluster, getMonRunDirPath(context.ConfigDir, monName),
		"admin", getMonKeyringPath(context.ConfigDir, monName))
	if err != nil {
		return err
	}

	// create monitor data dir
	monDataDir := getMonDataDirPath(context.ConfigDir, monName)
	if err := os.MkdirAll(monDataDir, 0744); err != nil {
		logger.Warningf("failed to create monitor data directory at %s: %+v", monDataDir, err)
	}

	// write the kv_backend file to force ceph to use rocksdb for the MON store
	if err := writeBackendFile(monDataDir, "rocksdb"); err != nil {
		return err
	}

	// call mon --mkfs in a child process
	err = context.ProcMan.Run(
		fmt.Sprintf("mkfs-%s", monName),
		"ceph-mon",
		"--mkfs",
		fmt.Sprintf("--cluster=%s", cluster.Name),
		fmt.Sprintf("--name=mon.%s", monName),
		fmt.Sprintf("--mon-data=%s", monDataDir),
		fmt.Sprintf("--conf=%s", confFilePath),
		fmt.Sprintf("--keyring=%s", getMonKeyringPath(context.ConfigDir, monName)))
	if err != nil {
		return fmt.Errorf("failed mon %s --mkfs: %+v", monName, err)
	}

	return nil
}

// runs the monitor in a child process
func (a *agent) runMonitor(context *clusterd.Context, cluster *ClusterInfo, monitor *CephMonitorConfig) error {
	if monitor.Endpoint == "" {
		return fmt.Errorf("missing endpoint for mon %s", monitor.Name)
	}

	confFile := GetConfFilePath(getMonRunDirPath(context.ConfigDir, monitor.Name), cluster.Name)
	util.WriteFileToLog(logger, confFile)

	// start the monitor daemon in the foreground with the given config
	logger.Infof("starting monitor %s", monitor.Name)
	monNameArg := fmt.Sprintf("--name=mon.%s", monitor.Name)
	monProc, err := context.ProcMan.Start(
		monitor.Name,
		"ceph-mon",
		regexp.QuoteMeta(monNameArg),
		proc.ReuseExisting,
		"--foreground",
		fmt.Sprintf("--cluster=%s", cluster.Name),
		monNameArg,
		fmt.Sprintf("--mon-data=%s", getMonDataDirPath(context.ConfigDir, monitor.Name)),
		fmt.Sprintf("--conf=%s", confFile),
		fmt.Sprintf("--public-addr=%s", monitor.Endpoint))
	if err != nil {
		return fmt.Errorf("failed to start monitor %s: %v", monitor.Name, err)
	}

	if monProc != nil {
		// if the process was already running Start will return nil in which case we don't want to overwrite it
		a.monProc = monProc
	}

	return nil
}

// writes the monitor backend file to disk
func writeBackendFile(monDataDir, backend string) error {
	backendPath := filepath.Join(monDataDir, "kv_backend")
	if err := ioutil.WriteFile(backendPath, []byte(backend), 0644); err != nil {
		return fmt.Errorf("failed to write kv_backend to %s: %+v", backendPath, err)
	}
	return nil
}
