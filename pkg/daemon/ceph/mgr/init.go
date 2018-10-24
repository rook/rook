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
	"os"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	cephconfig "github.com/rook/rook/pkg/daemon/ceph/config"
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
	ClusterInfo      *cephconfig.ClusterInfo
	Name             string
	Keyring          string
	ModuleServerAddr string
	CephVersionName  string
}

// Initialize generates configuration files for a Ceph mgr
func Initialize(context *clusterd.Context, config *Config) error {
	logger.Infof("Creating config for MGR %s with keyring %s", config.Name, config.Keyring)
	config.ClusterInfo.Log(logger)

	configPath := cephconfig.DefaultConfigFilePath()
	keyringPath := cephconfig.DaemonKeyringFilePath(cephconfig.VarLibCephDir, "mgr", config.Name)
	runDir := cephconfig.DaemonRunDir(cephconfig.VarLibCephDir, "mgr", config.Name)
	username := fmt.Sprintf("mgr.%s", config.Name)
	settings := map[string]string{}

	err := cephconfig.GenerateConfigFile(context, config.ClusterInfo,
		configPath, username, keyringPath, runDir, nil, settings)
	if err != nil {
		return fmt.Errorf("failed to create config file. %+v", err)
	}

	if err := setServerAddr(context, config, keyringPath); err != nil {
		return fmt.Errorf("failed to set mgr server addr. %+v", err)
	}

	keyringEval := func(key string) string {
		return fmt.Sprintf(keyringTemplate, config.Name, key)
	}
	if err = cephconfig.WriteKeyring(keyringPath, config.Keyring, keyringEval); err != nil {
		return fmt.Errorf("failed to create mgr keyring. %+v", err)
	}

	return nil
}

// Ideally this would be done from operator since it already has admin keyring, but we need to know
// the pod's IP for this task, so it must be in init container.
func setServerAddr(context *clusterd.Context, config *Config, keyringPath string) error {
	logger.Infof("setting server_addr for the prometheus and dashboard modules")

	// use the admin keyring for these operations
	adminKeyringEval := func(key string) string {
		return fmt.Sprintf(cephconfig.AdminKeyringTemplate, key)
	}
	err := cephconfig.WriteKeyring(keyringPath, config.ClusterInfo.AdminSecret, adminKeyringEval)
	if err != nil {
		return fmt.Errorf("failed to write admin keyring. %+v", err)
	}

	clusterName := "ceph"
	modules := []string{"prometheus", "dashboard"}
	for _, module := range modules {
		settingPath := fmt.Sprintf("mgr/%s/server_addr", module)
		_, err := client.MgrSetConfig(context, clusterName, config.Name, config.CephVersionName, settingPath, config.ModuleServerAddr)
		if err != nil {
			return fmt.Errorf("setting %s server_addr failed. %+v", module, err)
		}
	}

	// remove the admin keyring
	if err := os.Remove(keyringPath); err != nil {
		logger.Warningf("failed to remove admin keyring. %+v", err)
	}
	return nil
}
