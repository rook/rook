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
	"os"
	"strings"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util"
)

const (
	IPAddressEnvVar = "MON_POD_IP"
)

type Config struct {
	Name    string
	Cluster *ClusterInfo
	CephLauncher
}

func ParseMonEndpoints(input string) map[string]*CephMonitorConfig {
	mons := map[string]*CephMonitorConfig{}
	rawMons := strings.Split(input, ",")
	for _, rawMon := range rawMons {
		parts := strings.Split(rawMon, "=")
		if len(parts) != 2 {
			logger.Warningf("ignoring invalid monitor %s", rawMon)
			continue
		}
		mons[parts[0]] = &CephMonitorConfig{Name: parts[0], Endpoint: parts[1]}
	}
	return mons
}

func Run(context *clusterd.DaemonContext, config *Config) error {

	configFile, monDataDir, err := generateConfigFiles(context, config)
	if err != nil {
		return fmt.Errorf("failed to generate mon config files. %+v", err)
	}

	err = startMon(context, config, configFile, monDataDir)
	if err != nil {
		return fmt.Errorf("failed to run mon. %+v", err)
	}

	return err
}

func generateConfigFiles(context *clusterd.DaemonContext, config *Config) (string, string, error) {

	// write the keyring to disk
	keyringPath := getMonKeyringPath(context.ConfigDir, config.Name)
	if err := writeMonitorKeyring(config.Name, config.Cluster, keyringPath); err != nil {
		return "", "", err
	}

	// write the config file to disk
	confFilePath, err := GenerateConnectionConfigFile(ToContext(context), config.Cluster, getMonRunDirPath(context.ConfigDir, config.Name),
		"admin", getMonKeyringPath(context.ConfigDir, config.Name))
	if err != nil {
		return "", "", err
	}

	// create monitor data dir
	monDataDir := getMonDataDirPath(context.ConfigDir, config.Name)
	if err := os.MkdirAll(monDataDir, 0744); err != nil {
		logger.Warningf("failed to create monitor data directory at %s: %+v", monDataDir, err)
	}

	// write the kv_backend file to force ceph to use rocksdb for the MON store
	if err := writeBackendFile(monDataDir, "rocksdb"); err != nil {
		return "", "", err
	}

	return confFilePath, monDataDir, nil
}

// TEMP: Convert a context to a daemon context. This should go away after all daemons convert
func ToContext(context *clusterd.DaemonContext) *clusterd.Context {
	return &clusterd.Context{Executor: context.Executor, ProcMan: context.ProcMan, ConfigDir: context.ConfigDir, LogLevel: context.LogLevel, ConfigFileOverride: context.ConfigFileOverride}
}

func startMon(context *clusterd.DaemonContext, config *Config, confFilePath, monDataDir string) error {
	// call mon --mkfs in a child process
	logger.Infof("initializing mon")

	keyringPath := getMonKeyringPath(context.ConfigDir, config.Name)
	err := context.ProcMan.Run(
		fmt.Sprintf("mkfs-%s", config.Name),
		"mon",
		"--mkfs",
		fmt.Sprintf("--cluster=%s", config.Cluster.Name),
		fmt.Sprintf("--name=mon.%s", config.Name),
		fmt.Sprintf("--mon-data=%s", monDataDir),
		fmt.Sprintf("--conf=%s", confFilePath),
		fmt.Sprintf("--keyring=%s", keyringPath))
	if err != nil {
		return fmt.Errorf("failed mon %s --mkfs: %+v", config.Name, err)
	}

	// start the monitor daemon in the foreground with the given config
	logger.Infof("starting mon")

	util.WriteFileToLog(logger, confFilePath)

	err = config.CephLauncher.Run(
		"mon",
		"--foreground",
		fmt.Sprintf("--cluster=%s", config.Cluster.Name),
		fmt.Sprintf("--name=mon.%s", config.Name),
		fmt.Sprintf("--mon-data=%s", monDataDir),
		fmt.Sprintf("--conf=%s", confFilePath),
		fmt.Sprintf("--keyring=%s", keyringPath))
	if err != nil {
		return fmt.Errorf("failed to start rgw: %+v", err)
	}

	return nil
}
