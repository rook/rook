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
package mds

import (
	"fmt"
	"path"
	"strconv"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	"github.com/rook/rook/pkg/daemon/ceph/mon"
	"github.com/rook/rook/pkg/util"
)

const (
	keyringTemplate = `
[mds.%s]
key = %s
caps mon = "allow profile mds"
caps osd = "allow *"
caps mds = "allow"
`
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "cephmds")

type Config struct {
	FilesystemID  string
	ID            string
	ActiveStandby bool
	ClusterInfo   *mon.ClusterInfo
}

func Run(context *clusterd.Context, config *Config) error {

	err := generateConfigFiles(context, config)
	if err != nil {
		return fmt.Errorf("failed to generate mds config files. %+v", err)
	}

	err = startMDS(context, config)
	if err != nil {
		return fmt.Errorf("failed to run mds. %+v", err)
	}

	return err
}

func generateConfigFiles(context *clusterd.Context, config *Config) error {
	// write the latest config to the config dir
	if err := mon.GenerateAdminConnectionConfig(context, config.ClusterInfo); err != nil {
		return fmt.Errorf("failed to write connection config. %+v", err)
	}

	// Create a keyring for the new mds instance. Each pod will have its own keyring.
	// If the instance fails, the operator will need to clean up the unused keyring.
	keyring, err := createKeyring(context, config)
	if err != nil {
		return fmt.Errorf("failed to create mds keyring. %+v", err)
	}

	settings := map[string]string{
		"mds_standby_for_fscid": config.FilesystemID,
		"mds_standby_replay":    strconv.FormatBool(config.ActiveStandby),
	}

	keyringPath := getMDSKeyringPath(context.ConfigDir)
	_, err = mon.GenerateConfigFile(context, config.ClusterInfo, getMDSConfDir(context.ConfigDir),
		fmt.Sprintf("mds.%s", config.ID), keyringPath, nil, settings)
	if err != nil {
		return fmt.Errorf("failed to create mds config file. %+v", err)
	}
	keyringEval := func(key string) string {
		r := fmt.Sprintf(keyringTemplate, config.ID, key)
		logger.Infof("keyring: %s", r)
		return r
	}

	err = mon.WriteKeyring(keyringPath, keyring, keyringEval)
	if err != nil {
		return fmt.Errorf("failed to create mds keyring. %+v", err)
	}

	return nil
}

func startMDS(context *clusterd.Context, config *Config) error {

	// start the mds daemon in the foreground with the given config
	logger.Infof("starting mds %s", config.ID)

	confFile := getMDSConfFilePath(context.ConfigDir, config.ClusterInfo.Name)
	util.WriteFileToLog(logger, confFile)

	mdsNameArg := fmt.Sprintf("--name=mds.%s", config.ID)
	args := []string{
		"--foreground",
		mdsNameArg,
		"-i", config.ID,
		fmt.Sprintf("--cluster=%s", config.ClusterInfo.Name),
		fmt.Sprintf("--conf=%s", confFile),
		fmt.Sprintf("--keyring=%s", getMDSKeyringPath(context.ConfigDir)),
	}

	name := fmt.Sprintf("mds%s", config.ID)
	if err := context.Executor.ExecuteCommand(false, name, "ceph-mds", args...); err != nil {
		return fmt.Errorf("failed to start mds. %+v", err)
	}
	return nil
}

func getMDSConfDir(dir string) string {
	return path.Join(dir, "mds")
}

func getMDSConfFilePath(dir, clusterName string) string {
	return path.Join(getMDSConfDir(dir), fmt.Sprintf("%s.config", clusterName))
}

func getMDSKeyringPath(dir string) string {
	return path.Join(getMDSConfDir(dir), "keyring")
}

// create a keyring for the mds client with a limited set of privileges
func createKeyring(context *clusterd.Context, config *Config) (string, error) {
	access := []string{"osd", "allow *", "mds", "allow", "mon", "allow profile mds"}
	username := fmt.Sprintf("mds.%s", config.ID)

	// get-or-create-key for the user account
	keyring, err := client.AuthGetOrCreateKey(context, config.ClusterInfo.Name, username, access)
	if err != nil {
		return "", fmt.Errorf("failed to get or create mds auth key for %s. %+v", username, err)
	}

	return keyring, nil
}
