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

// Package mds provides methods for setting up Ceph configuration for mds daemons.
// It also provides methods for creating/deleting/managing Ceph filesystems served by the mds.
package mds

import (
	"fmt"
	"path"
	"strconv"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	"github.com/rook/rook/pkg/util"
)

const (
	// InitCommand is the `rook ceph` subcommand which will perform mds initialization
	InitCommand = "mds-init"

	keyringTemplate = `
[mds.%s]
key = %s
caps mon = "allow profile mds"
caps osd = "allow *"
caps mds = "allow"
`
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "cephmds")

// Config contains the necessary parameters Rook needs to know to set up a mds for a Ceph cluster.
type Config struct {
	FilesystemID  string
	Name          string
	Keyring       string
	ActiveStandby bool
	ClusterInfo   *cephconfig.ClusterInfo
}

// Initialize generates configuration files for a Ceph mds
func Initialize(context *clusterd.Context, config *Config) error {
	logger.Infof("Creating config for mds %s [filesystem ID: %s] [active standby?: %+v]",
		config.Name, config.FilesystemID, config.ActiveStandby)
	err := generateConfigFiles(context, config)
	if err != nil {
		return fmt.Errorf("failed to generate mds config files: %+v", err)
	}

	util.WriteFileToLog(logger, cephconfig.DefaultConfigFilePath())

	return err
}

func generateConfigFiles(context *clusterd.Context, config *Config) error {
	// write the latest config to the config dir
	if err := cephconfig.GenerateAdminConnectionConfig(context, config.ClusterInfo); err != nil {
		return fmt.Errorf("failed to write connection config: %+v", err)
	}

	settings := map[string]string{
		"mds_standby_for_fscid": config.FilesystemID,
		"mds_standby_replay":    strconv.FormatBool(config.ActiveStandby),
	}

	keyringPath := getMdsKeyringPath(context.ConfigDir, config.Name)
	_, err := cephconfig.GenerateConfigFile(context, config.ClusterInfo,
		getMdsConfDir(context.ConfigDir, config.Name), fmt.Sprintf("mds.%s", config.Name),
		keyringPath, nil, settings)
	if err != nil {
		return fmt.Errorf("failed to create mds config file: %+v", err)
	}

	keyringEval := func(key string) string {
		r := fmt.Sprintf(keyringTemplate, config.Name, key)
		return r
	}

	err = cephconfig.WriteKeyring(keyringPath, config.Keyring, keyringEval)
	if err != nil {
		return fmt.Errorf("failed to create mds keyring: %+v", err)
	}

	return nil
}

func getMdsConfDir(dir, name string) string {
	return path.Join(dir, fmt.Sprintf("mds-%s", name))
}

func getMdsKeyringPath(dir, name string) string {
	return path.Join(getMdsConfDir(dir, name), "keyring")
}
