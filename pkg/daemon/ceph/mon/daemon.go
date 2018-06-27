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
	"net"
	"os"
	"strings"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util"
)

const (
	DefaultPort = 6790
)

type Config struct {
	Name     string
	Cluster  *ClusterInfo
	isDaemon bool
	Port     int32
}

func NewConfig(name string, cluster *ClusterInfo, isDaemon bool, port int32) *Config {
	return &Config{Name: name, Cluster: cluster, isDaemon: isDaemon, Port: port}
}

func FlattenMonEndpoints(mons map[string]*CephMonitorConfig) string {
	endpoints := []string{}
	for _, m := range mons {
		endpoints = append(endpoints, fmt.Sprintf("%s=%s", m.Name, m.Endpoint))
	}
	return strings.Join(endpoints, ",")
}

func ParseMonEndpoints(input string) map[string]*CephMonitorConfig {
	logger.Infof("parsing mon endpoints: %s", input)
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

func ToCephMon(name, ip string, port int32) *CephMonitorConfig {
	return &CephMonitorConfig{Name: name, Endpoint: joinHostPort(ip, port)}
}

func Run(context *clusterd.Context, config *Config) error {
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

func generateConfigFiles(context *clusterd.Context, config *Config) (string, string, error) {

	// write the keyring to disk
	if err := writeMonKeyring(context, config.Cluster, config.Name); err != nil {
		return "", "", err
	}

	// write the config file to disk
	confFilePath, err := GenerateConnectionConfigFile(context, config.Cluster, getMonRunDirPath(context.ConfigDir, config.Name),
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

func startMon(context *clusterd.Context, config *Config, confFilePath, monDataDir string) error {
	// call mon --mkfs in a child process
	logger.Infof("initializing mon")

	// generate the monmap
	monmapPath, err := generateMonMap(context, config.Cluster, getMonRunDirPath(context.ConfigDir, config.Name))
	if err != nil {
		return err
	}

	monNameArg := fmt.Sprintf("--name=mon.%s", config.Name)
	keyringPath := getMonKeyringPath(context.ConfigDir, config.Name)
	err = context.Executor.ExecuteCommand(
		false,
		fmt.Sprintf("mkfs-%s", config.Name),
		"ceph-mon",
		"--mkfs",
		monNameArg,
		fmt.Sprintf("--cluster=%s", config.Cluster.Name),
		fmt.Sprintf("--mon-data=%s", monDataDir),
		fmt.Sprintf("--conf=%s", confFilePath),
		fmt.Sprintf("--keyring=%s", keyringPath),
		fmt.Sprintf("--monmap=%s", monmapPath))
	if err != nil {
		return fmt.Errorf("failed mon %s --mkfs: %+v", config.Name, err)
	}

	// start the monitor daemon in the foreground with the given config
	logger.Infof("starting mon")

	util.WriteFileToLog(logger, confFilePath)

	args := []string{
		"--foreground",
		monNameArg,
		fmt.Sprintf("--cluster=%s", config.Cluster.Name),
		fmt.Sprintf("--mon-data=%s", monDataDir),
		fmt.Sprintf("--conf=%s", confFilePath),
		fmt.Sprintf("--keyring=%s", keyringPath),
		fmt.Sprintf("--public-addr=%s", joinHostPort(context.NetworkInfo.PublicAddr, config.Port)),
		fmt.Sprintf("--public-bind-addr=%s", joinHostPort(context.NetworkInfo.ClusterAddr, config.Port)),
	}
	if err = context.Executor.ExecuteCommand(false, config.Name, "ceph-mon", args...); err != nil {
		return fmt.Errorf("failed to start mon: %+v", err)
	}

	return nil
}

func joinHostPort(host string, port int32) string {
	return net.JoinHostPort(host, fmt.Sprintf("%d", port))
}
