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
	"os"
	"path/filepath"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	"github.com/rook/rook/pkg/util"
)

const (
	// InitCommand is the `rook ceph` subcommand which will perform mon initialization
	InitCommand = "mon-init"

	// DefaultPort is the default port Ceph mons use to communicate amongst themselves.
	DefaultPort = 6789
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
	err := generateConfigFiles(context, config)
	if err != nil {
		return fmt.Errorf("failed to generate mon config files. %+v", err)
	}

	util.WriteFileToLog(logger, cephconfig.DefaultConfigFilePath())

	return err
}

func generateConfigFiles(context *clusterd.Context, config *Config) error {
	// write the keyring to disk
	if err := writeMonKeyring(context, config.Cluster, config.Name); err != nil {
		return err
	}

	// write the config file to disk
	_, err := generateConnectionConfigFile(context, config,
		GetMonRunDirPath(context.ConfigDir, config.Name), getMonKeyringPath(context.ConfigDir, config.Name))
	if err != nil {
		return err
	}

	// create monitor data dir
	monDataDir := GetMonDataDirPath(context.ConfigDir, config.Name)
	if err := os.MkdirAll(monDataDir, 0744); err != nil {
		logger.Warningf("failed to create monitor data directory at %s: %+v", monDataDir, err)
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
		return fmt.Errorf("failed to write kv_backend to %s: %+v", backendPath, err)
	}
	return nil
}
