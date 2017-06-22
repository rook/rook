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
	"regexp"

	"github.com/rook/rook/pkg/ceph/mon"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util"
	"github.com/rook/rook/pkg/util/proc"
)

type Config struct {
	ID          string
	Keyring     string
	InProc      bool
	ClusterInfo *mon.ClusterInfo
}

func Run(context *clusterd.Context, config *Config) error {

	err := generateConfigFiles(context, config)
	if err != nil {
		return fmt.Errorf("failed to generate mds config files. %+v", err)
	}

	_, err = startMDS(context, config)
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

	keyringPath := getMDSKeyringPath(context.ConfigDir, config.ID)
	_, err := mon.GenerateConnectionConfigFile(context, config.ClusterInfo, getMDSConfDir(context.ConfigDir, config.ID),
		fmt.Sprintf("mds.%s", config.ID), keyringPath)
	if err != nil {
		return fmt.Errorf("failed to create mds config file. %+v", err)
	}
	keyringEval := func(key string) string {
		return fmt.Sprintf(keyringTemplate, config.ID, key)
	}

	err = mon.WriteKeyring(keyringPath, config.Keyring, keyringEval)
	if err != nil {
		return fmt.Errorf("failed to create mds keyring. %+v", err)
	}

	return nil
}

func startMDS(context *clusterd.Context, config *Config) (mdsProc *proc.MonitoredProc, err error) {

	// start the mds daemon in the foreground with the given config
	logger.Infof("starting mds %s", config.ID)

	confFile := getMDSConfFilePath(context.ConfigDir, config.ID, config.ClusterInfo.Name)
	util.WriteFileToLog(logger, confFile)

	mdsNameArg := fmt.Sprintf("--name=mds.%s", config.ID)
	args := []string{
		"--foreground",
		mdsNameArg,
		fmt.Sprintf("--cluster=%s", config.ClusterInfo.Name),
		fmt.Sprintf("--conf=%s", confFile),
		fmt.Sprintf("--keyring=%s", getMDSKeyringPath(context.ConfigDir, config.ID)),
		"-i", config.ID,
	}

	name := fmt.Sprintf("mds%s", config.ID)
	if config.InProc {
		err = context.ProcMan.Run(name, "ceph-mds", args...)
	} else {
		mdsProc, err = context.ProcMan.Start(name, "ceph-mds", regexp.QuoteMeta(mdsNameArg), proc.ReuseExisting, args...)
	}
	if err != nil {
		err = fmt.Errorf("failed to start mds: %+v", err)
	}
	return
}

func getMDSConfDir(dir, id string) string {
	return path.Join(dir, fmt.Sprintf("mds%s", id))
}

func getMDSConfFilePath(dir, id, clusterName string) string {
	return path.Join(getMDSConfDir(dir, id), fmt.Sprintf("%s.config", clusterName))
}

func getMDSKeyringPath(dir, id string) string {
	return path.Join(getMDSConfDir(dir, id), "keyring")
}
