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

package mgr

import (
	"fmt"
	"path"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/clusterd"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
	"github.com/rook/rook/pkg/util"
)

var (
	logger          = capnslog.NewPackageLogger("github.com/rook/rook", "cephmgr")
	keyringTemplate = `
[mgr.%s]
	key = %s
	caps mon = "allow profile mgr"
	caps mds = "allow *"
	caps osd = "allow *"
`
)

const (
	// InitCommand is the `rook ceph` subcommand which will perform mgr initialization
	InitCommand = "mgr-init"

	cephmgr = "ceph-mgr"
)

// Config contains the necessary parameters Rook needs to know to set up a mgr for a Ceph cluster.
type Config struct {
	ClusterInfo *cephconfig.ClusterInfo
	Name        string
	Keyring     string
}

// Initialize generates configuration files for a Ceph mgr
func Initialize(context *clusterd.Context, config *Config) error {
	logger.Infof("Creating config for MGR %s with keyring %s", config.Name, config.Keyring)
	config.ClusterInfo.Log(logger)
	if err := generateConfigFiles(context, config); err != nil {
		return fmt.Errorf("failed to generate mgr config files. %+v", err)
	}

	util.WriteFileToLog(logger, cephconfig.DefaultConfigFilePath())

	return nil
}

func generateConfigFiles(context *clusterd.Context, config *Config) error {
	keyringPath := getMgrKeyringPath(context.ConfigDir, config.Name)
	confDir := getMgrConfDir(context.ConfigDir, config.Name)
	username := fmt.Sprintf("mgr.%s", config.Name)
	settings := map[string]string{
		"mgr data": confDir,
	}
	logger.Infof("Conf files: dir=%s keyring=%s", confDir, keyringPath)
	_, err := cephconfig.GenerateConfigFile(context, config.ClusterInfo, confDir,
		username, keyringPath, nil, settings)
	if err != nil {
		return fmt.Errorf("failed to create config file. %+v", err)
	}

	keyringEval := func(key string) string {
		return fmt.Sprintf(keyringTemplate, config.Name, key)
	}

	err = cephconfig.WriteKeyring(keyringPath, config.Keyring, keyringEval)
	if err != nil {
		return fmt.Errorf("failed to create mds keyring. %+v", err)
	}

	return nil
}

func getMgrConfDir(dir, name string) string {
	return path.Join(dir, fmt.Sprintf("mgr-%s", name))
}

func getMgrConfFilePath(dir, name, clusterName string) string {
	return path.Join(getMgrConfDir(dir, name), fmt.Sprintf("%s.config", clusterName))
}

func getMgrKeyringPath(dir, name string) string {
	return path.Join(getMgrConfDir(dir, name), "keyring")
}
