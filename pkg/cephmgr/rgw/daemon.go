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
package rgw

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"

	"strconv"

	"github.com/rook/rook/pkg/cephmgr/mon"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util"
)

type Config struct {
	DataDir     string
	Host        string
	Port        int
	Keyring     string
	ClusterInfo *mon.ClusterInfo
	mon.CephLauncher
}

func Run(context *clusterd.DaemonContext, config *Config) error {

	err := generateConfigFiles(context, config)
	if err != nil {
		return fmt.Errorf("failed to generate rgw config files. %+v", err)
	}

	err = startRGW(context, config)
	if err != nil {
		return fmt.Errorf("failed to run rgw. %+v", err)
	}

	return err
}

func generateConfigFiles(context *clusterd.DaemonContext, config *Config) error {
	// create the rgw data directory
	dataDir := path.Join(getRGWConfDir(config.DataDir), "data")
	if err := os.MkdirAll(dataDir, 0744); err != nil {
		logger.Warningf("failed to create data directory %s: %+v", dataDir, err)
	}

	settings := map[string]string{
		"host":                           config.Host,
		"rgw port":                       strconv.Itoa(config.Port),
		"rgw data":                       dataDir,
		"rgw dns name":                   fmt.Sprintf("%s:%d", config.Host, config.Port),
		"rgw log nonexistent bucket":     "true",
		"rgw intent log object name utc": "true",
		"rgw enable usage log":           "true",
	}
	_, err := mon.GenerateConfigFile(toContext(context), config.ClusterInfo, getRGWConfDir(config.DataDir),
		"client.radosgw.gateway", getRGWKeyringPath(config.DataDir), false, nil, settings)
	if err != nil {
		return fmt.Errorf("failed to create config file. %+v", err)
	}

	keyringEval := func(key string) string {
		return fmt.Sprintf(keyringTemplate, key)
	}

	// create rgw config
	err = mon.WriteKeyring(getRGWKeyringPath(config.DataDir), config.Keyring, keyringEval)
	if err != nil {
		return fmt.Errorf("failed to save keyring. %+v", err)
	}

	// write the mime types config
	mimeTypesPath := getMimeTypesPath(config.DataDir)
	logger.Debugf("Writing mime types to: %s", mimeTypesPath)
	if err := ioutil.WriteFile(mimeTypesPath, []byte(mimeTypes), 0644); err != nil {
		return fmt.Errorf("failed to write mime types to %s: %+v", mimeTypesPath, err)
	}

	return nil
}

// TEMP: Convert a context to a daemon context. This should go away after all daemons convert
func toContext(context *clusterd.DaemonContext) *clusterd.Context {
	return &clusterd.Context{Executor: context.Executor, ConfigDir: context.ConfigDir, LogLevel: context.LogLevel, ConfigFileOverride: context.ConfigFileOverride}
}

func startRGW(context *clusterd.DaemonContext, config *Config) error {

	// start the monitor daemon in the foreground with the given config
	logger.Infof("starting rgw")

	confFile := getRGWConfFilePath(config.DataDir, config.ClusterInfo.Name)
	util.WriteFileToLog(logger, confFile)

	err := config.CephLauncher.Run(
		"rgw",
		"--foreground",
		"--name=client.radosgw.gateway",
		fmt.Sprintf("--cluster=%s", config.ClusterInfo.Name),
		fmt.Sprintf("--conf=%s", confFile),
		fmt.Sprintf("--keyring=%s", getRGWKeyringPath(config.DataDir)),
		fmt.Sprintf("--rgw-frontends=civetweb port=%d", config.Port),
		fmt.Sprintf("--rgw-mime-types-file=%s", getMimeTypesPath(config.DataDir)))
	if err != nil {
		return fmt.Errorf("failed to start rgw: %+v", err)
	}

	return nil
}

func getRGWConfFilePath(configDir, clusterName string) string {
	return path.Join(getRGWConfDir(configDir), fmt.Sprintf("%s.config", clusterName))
}

func getRGWConfDir(configDir string) string {
	return path.Join(configDir, "rgw")
}

func getRGWKeyringPath(configDir string) string {
	return path.Join(getRGWConfDir(configDir), "keyring")
}

func getMimeTypesPath(configDir string) string {
	return path.Join(getRGWConfDir(configDir), "mime.types")
}
