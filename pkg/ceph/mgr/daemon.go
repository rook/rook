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
	"regexp"

	"github.com/coreos/pkg/capnslog"
	"github.com/rook/rook/pkg/ceph/mon"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util"
	"github.com/rook/rook/pkg/util/proc"
)

var (
	logger          = capnslog.NewPackageLogger("github.com/rook/rook", "cephmgr")
	keyringTemplate = `
[mgr.%s]
	key = %s
	caps mon = "allow *"
`
)

const (
	cephmgr = "ceph-mgr"
)

type Config struct {
	InProc      bool
	ClusterInfo *mon.ClusterInfo
	Name        string
	Keyring     string
}

func Run(context *clusterd.Context, config *Config) error {
	logger.Infof("Starting MGR %s with keyring %s", config.Name, config.Keyring)
	err := generateConfigFiles(context, config)
	if err != nil {
		return fmt.Errorf("failed to generate mgr config files. %+v", err)
	}

	_, err = startMgr(context, config)
	if err != nil {
		return fmt.Errorf("failed to run mgr. %+v", err)
	}

	return err
}

func generateConfigFiles(context *clusterd.Context, config *Config) error {
	// write the latest config to the config dir
	/*if err := mon.GenerateAdminConnectionConfig(context, config.ClusterInfo); err != nil {
		return fmt.Errorf("failed to write connection config. %+v", err)
	}*/

	keyringPath := getMgrKeyringPath(context.ConfigDir, config.Name)
	confDir := getMgrConfDir(context.ConfigDir, config.Name)
	username := fmt.Sprintf("mgr.%s", config.Name)
	settings := map[string]string{
		"mgr data": confDir,
	}
	logger.Infof("Conf files: dir=%s keyring=%s", confDir, keyringPath)
	_, err := mon.GenerateConfigFile(context, config.ClusterInfo, confDir,
		username, keyringPath, false, nil, settings)
	if err != nil {
		return fmt.Errorf("failed to create config file. %+v", err)
	}

	keyringEval := func(key string) string {
		return fmt.Sprintf(keyringTemplate, config.Name, key)
	}

	err = mon.WriteKeyring(keyringPath, config.Keyring, keyringEval)
	if err != nil {
		return fmt.Errorf("failed to create mds keyring. %+v", err)
	}

	return nil
}

func startMgr(context *clusterd.Context, config *Config) (mgrProc *proc.MonitoredProc, err error) {

	// start the mgr daemon in the foreground with the given config
	logger.Infof("starting ceph-mgr")

	confFile := getMgrConfFilePath(context.ConfigDir, config.Name, config.ClusterInfo.Name)
	util.WriteFileToLog(logger, confFile)

	keyringPath := getMgrKeyringPath(context.ConfigDir, config.Name)
	util.WriteFileToLog(logger, keyringPath)
	args := []string{
		"--foreground",
		fmt.Sprintf("--cluster=%s", config.ClusterInfo.Name),
		fmt.Sprintf("--conf=%s", confFile),
		fmt.Sprintf("--keyring=%s", keyringPath),
		"-i", config.Name,
	}

	if config.InProc {
		err = context.ProcMan.Run(cephmgr, cephmgr, args...)
	} else {
		mgrProc, err = context.ProcMan.Start(cephmgr, cephmgr, regexp.QuoteMeta(cephmgr), proc.ReuseExisting, args...)
	}
	if err != nil {
		err = fmt.Errorf("failed to start mgr: %+v", err)
	}
	return
}

func getMgrConfDir(dir, name string) string {
	return path.Join(dir, fmt.Sprintf("mgr%s", name))
}

func getMgrConfFilePath(dir, name, clusterName string) string {
	return path.Join(getMgrConfDir(dir, name), fmt.Sprintf("%s.config", clusterName))
}

func getMgrKeyringPath(dir, name string) string {
	return path.Join(getMgrConfDir(dir, name), "keyring")
}
