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

// Package mon provides methods for setting up Ceph configuration for mons daemons.
package mon

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path"
	"path/filepath"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
)

const (
	// InitCommand is the `rook ceph` subcommand which will perform mon initialization
	InitCommand = "mon-init"

	// DefaultPort is the default port Ceph mons use to communicate amongst themselves.
	DefaultPort = 6790

	// The final string field is for the admin keyring
	monitorKeyringTemplate = `
	[mon.]
		key = %s
		caps mon = "allow *"

	%s`
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "cephmon")

// Config contains the necessary parameters Rook needs to know to set up a mon for a Ceph cluster.
type Config struct {
	Name    string
	Cluster *cephconfig.ClusterInfo
	Port    int32
}

// Initialize generates configuration files for a Ceph mon
func Initialize(context *clusterd.Context, config *Config) error {
	logger.Infof("Creating config for MON %s with port %d", config.Name, config.Port)
	config.Cluster.Log(logger)

	// Delete legacy config and keyring files which may be persisted to disk and are no longer
	// needed. The legacy keyring contains the admin key, which is a security risk. The legacy
	// config may just end up being confusing for users if it is left.  Needed for upgrade from
	// Rook v0.8 to v0.9.
	legacyConfigPath := path.Join(cephconfig.DaemonRunDir(cephconfig.VarLibCephDir, "mon", config.Name),
		fmt.Sprintf("%s.config", config.Cluster.Name))
	legacyKeyringPath := path.Join(cephconfig.DaemonRunDir(cephconfig.VarLibCephDir, "mon", config.Name), "keyring")
	logger.Infof("Deleting legacy mon config file: %s", legacyConfigPath)
	if err := os.Remove(legacyConfigPath); err != nil && !os.IsNotExist(err) {
		logger.Errorf("failed to delete legacy mon config file %s. %+v", legacyConfigPath, err)
	}
	logger.Infof("Deleting legacy mon keyring file: %s", legacyKeyringPath)
	if err := os.Remove(legacyKeyringPath); err != nil && !os.IsNotExist(err) {
		logger.Errorf("failed to delete legacy mon keyring %s. %+v", legacyKeyringPath, err)
	}

	configPath := cephconfig.DefaultConfigFilePath()
	keyringPath := cephconfig.DaemonKeyringFilePath(cephconfig.EtcCephDir, "mon", config.Name)
	runDir := cephconfig.DaemonRunDir(cephconfig.VarLibCephDir, "mon", config.Name)
	username := fmt.Sprintf("mon.%s", config.Name)
	// public_bind_addr is set from the pod IP which can only be known at runtime, so set this
	// at config init int the Ceph config file.
	// See pkg/operator/ceph/cluster/mon/spec.go - makeMonDaemonContainer() comment notes for more
	privateAddr := net.JoinHostPort(context.NetworkInfo.ClusterAddr, fmt.Sprintf("%d", config.Port))
	settings := map[string]string{
		"public bind addr": privateAddr,
	}

	err := cephconfig.GenerateConfigFile(context, config.Cluster,
		configPath, username, keyringPath, runDir, nil, settings)
	if err != nil {
		return fmt.Errorf("failed to generate mon config file at %s. %+v", configPath, err)
	}

	keyringEval := func(_ string) string {
		return fmt.Sprintf(monitorKeyringTemplate,
			config.Cluster.MonitorSecret, cephconfig.AdminKeyring(config.Cluster))
	}
	if err := cephconfig.WriteKeyring(keyringPath, "", keyringEval); err != nil {
		return fmt.Errorf("failed to write mon keyring to %s. %+v", keyringPath, err)
	}

	// create monitor data dir
	monDataDir := cephconfig.DaemonDataDir(cephconfig.VarLibCephDir, "mon", config.Name)
	if err := os.MkdirAll(monDataDir, 0744); err != nil {
		logger.Warningf("failed to create monitor data directory at %s. %+v", monDataDir, err)
	}

	// write the kv_backend file to force ceph to use rocksdb for the MON store
	if err := writeBackendFile(monDataDir, "rocksdb"); err != nil {
		return err
	}

	return nil
}

// writeBackendFile writes the monitor backend file to disk
func writeBackendFile(monDataDir, backend string) error {
	backendPath := filepath.Join(monDataDir, "kv_backend")
	if err := ioutil.WriteFile(backendPath, []byte(backend), 0644); err != nil {
		return fmt.Errorf("failed to write kv_backend to %s. %+v", backendPath, err)
	}
	return nil
}
