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
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/coreos/pkg/capnslog"
	"github.com/go-ini/ini"
	"github.com/rook/rook/pkg/ceph/client"
	"github.com/rook/rook/pkg/clusterd"
)

type CephMonitorConfig struct {
	Name     string `json:"name"`
	Endpoint string `json:"endpoint"`
}

type cephConfig struct {
	*GlobalConfig `ini:"global,omitempty"`
}

type GlobalConfig struct {
	EnableExperimental       string `ini:"enable experimental unrecoverable data corrupting features"`
	FSID                     string `ini:"fsid,omitempty"`
	RunDir                   string `ini:"run dir,omitempty"`
	MonMembers               string `ini:"mon initial members,omitempty"`
	LogFile                  string `ini:"log file,omitempty"`
	MonClusterLogFile        string `ini:"mon cluster log file,omitempty"`
	PublicAddr               string `ini:"public addr,omitempty"`
	PublicNetwork            string `ini:"public network,omitempty"`
	ClusterAddr              string `ini:"cluster addr,omitempty"`
	ClusterNetwork           string `ini:"cluster network,omitempty"`
	MonKeyValueDb            string `ini:"mon keyvaluedb"`
	DebugLogDefaultLevel     int    `ini:"debug default"`
	DebugLogRadosLevel       int    `ini:"debug rados"`
	DebugLogMonLevel         int    `ini:"debug mon"`
	DebugLogOSDLevel         int    `ini:"debug osd"`
	DebugLogBluestoreLevel   int    `ini:"debug bluestore"`
	DebugLogFilestoreLevel   int    `ini:"debug filestore"`
	DebugLogJournalLevel     int    `ini:"debug journal"`
	DebugLogLevelDBLevel     int    `ini:"debug leveldb"`
	FileStoreOmapBackend     string `ini:"filestore_omap_backend"`
	OsdPgBits                int    `ini:"osd pg bits,omitempty"`
	OsdPgpBits               int    `ini:"osd pgp bits,omitempty"`
	OsdPoolDefaultSize       int    `ini:"osd pool default size,omitempty"`
	OsdPoolDefaultMinSize    int    `ini:"osd pool default min size,omitempty"`
	OsdPoolDefaultPgNum      int    `ini:"osd pool default pg num,omitempty"`
	OsdPoolDefaultPgpNum     int    `ini:"osd pool default pgp num,omitempty"`
	OsdMaxObjectNameLen      int    `ini:"osd max object name len,omitempty"`
	OsdMaxObjectNamespaceLen int    `ini:"osd max object namespace len,omitempty"`
	OsdObjectStore           string `ini:"osd objectstore"`
	RbdDefaultFeatures       int    `ini:"rbd_default_features,omitempty"`
	CrushtoolPath            string `ini:"crushtool"`
	FatalSignalHandlers      string `ini:"fatal signal handlers"`
}

// get the path of a given monitor's run dir
func getMonRunDirPath(configDir, monName string) string {
	return path.Join(configDir, monName)
}

// get the path of a given monitor's keyring
func getMonKeyringPath(configDir, monName string) string {
	return filepath.Join(getMonRunDirPath(configDir, monName), "keyring")
}

// get the path of a given monitor's data dir
func getMonDataDirPath(configDir, monName string) string {
	return filepath.Join(getMonRunDirPath(configDir, monName), fmt.Sprintf("mon.%s", monName))
}

// get the path of a given monitor's config file
func GetConfFilePath(root, clusterName string) string {
	return fmt.Sprintf("%s/%s.config", root, clusterName)
}

func GenerateAdminConnectionConfig(context *clusterd.Context, cluster *ClusterInfo) error {
	root := path.Join(context.ConfigDir, cluster.Name)
	keyring := fmt.Sprintf(AdminKeyringTemplate, cluster.AdminSecret)
	keyringPath := path.Join(root, fmt.Sprintf("%s.keyring", client.AdminUsername))
	err := writeKeyring(cluster, keyring, keyringPath)
	if err != nil {
		return fmt.Errorf("failed to write keyring to %s. %+v", root, err)
	}

	_, err = GenerateConfigFile(context, cluster, root, client.AdminUsername, keyringPath, false, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to write config to %s. %+v", root, err)
	}
	logger.Infof("generated admin config in %s", root)
	return nil
}

func writeMonKeyring(context *clusterd.Context, c *ClusterInfo, name string) error {
	keyringPath := getMonKeyringPath(context.ConfigDir, name)
	keyring := fmt.Sprintf(monitorKeyringTemplate, c.MonitorSecret, c.AdminSecret)
	return writeKeyring(c, keyring, keyringPath)
}

// writes the monitor keyring to disk
func writeKeyring(c *ClusterInfo, keyring, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0744); err != nil {
		return fmt.Errorf("failed to create keyring directory for %s: %+v", path, err)
	}
	if err := ioutil.WriteFile(path, []byte(keyring), 0644); err != nil {
		return fmt.Errorf("failed to write monitor keyring to %s: %+v", path, err)
	}

	return nil
}

// generates and writes the monitor config file to disk
func GenerateConnectionConfigFile(context *clusterd.Context, cluster *ClusterInfo, pathRoot, user, keyringPath string) (string, error) {
	return GenerateConfigFile(context, cluster, pathRoot, user, keyringPath, false, nil, nil)
}

// generates and writes the monitor config file to disk
func GenerateConfigFile(context *clusterd.Context, cluster *ClusterInfo, pathRoot, user, keyringPath string,
	bluestore bool, userConfig *cephConfig, clientSettings map[string]string) (string, error) {

	if pathRoot == "" {
		pathRoot = getMonRunDirPath(context.ConfigDir, getFirstMonitor(cluster))
	}

	// create the config directory
	if err := os.MkdirAll(pathRoot, 0744); err != nil {
		logger.Warningf("failed to create config directory at %s: %+v", pathRoot, err)
	}

	configFile, err := createGlobalConfigFileSection(context, cluster, pathRoot, bluestore, userConfig)
	if err != nil {
		return "", fmt.Errorf("failed to create global config section, %+v", err)
	}

	if err := addClientConfigFileSection(configFile, getQualifiedUser(user), keyringPath, clientSettings); err != nil {
		return "", fmt.Errorf("failed to add admin client config section, %+v", err)
	}

	if err := addInitialMonitorsConfigFileSections(configFile, cluster); err != nil {
		return "", fmt.Errorf("failed to add initial monitor config sections, %+v", err)
	}

	// if there's a config file override path given, process the given config file
	if context.ConfigFileOverride != "" {
		err := configFile.Append(context.ConfigFileOverride)
		if err != nil {
			// log the config file override failure as a warning, but proceed without it
			logger.Warningf("failed to add config file override from '%s': %+v", context.ConfigFileOverride, err)
		}
	}

	// write the entire config to disk
	filePath := GetConfFilePath(pathRoot, cluster.Name)
	logger.Infof("writing config file %s", filePath)
	if err := configFile.SaveTo(filePath); err != nil {
		return "", fmt.Errorf("failed to save config file %s. %+v", filePath, err)
	}

	return filePath, nil
}

// create a keyring for access to the cluster, with the desired set of privileges
func CreateKeyring(context *clusterd.Context, clusterName, username, keyringPath string, access []string, generateContents func(string) string) error {
	_, err := os.Stat(keyringPath)
	if err == nil {
		// no error, the file exists, bail out with no error
		logger.Debugf("keyring already exists at %s", keyringPath)
		return nil
	} else if !os.IsNotExist(err) {
		// some other error besides "does not exist", bail out with error
		return fmt.Errorf("failed to stat %s: %+v", keyringPath, err)
	}

	// get-or-create-key for the user account
	key, err := client.AuthGetOrCreateKey(context, clusterName, username, access)
	if err != nil {
		return fmt.Errorf("failed to get or create auth key for %s. %+v", username, err)
	}

	return WriteKeyring(keyringPath, key, generateContents)
}

func WriteKeyring(keyringPath, keyring string, generateContents func(string) string) error {
	// write the keyring to disk
	keyringDir := filepath.Dir(keyringPath)
	if err := os.MkdirAll(keyringDir, 0744); err != nil {
		return fmt.Errorf("failed to create keyring dir at %s: %+v", keyringDir, err)
	}

	contents := generateContents(keyring)
	logger.Debugf("Writing keyring to: %s", keyringPath)
	if err := ioutil.WriteFile(keyringPath, []byte(contents), 0644); err != nil {
		return fmt.Errorf("failed to write keyring to %s: %+v", keyringPath, err)
	}

	return nil
}

// prepends "client." if a user namespace is not already specified
func getQualifiedUser(user string) string {
	if strings.Index(user, ".") == -1 {
		return fmt.Sprintf("client.%s", user)
	}

	return user
}

func getFirstMonitor(cluster *ClusterInfo) string {
	// Get the first monitor
	for _, m := range cluster.Monitors {
		return m.Name
	}

	return ""
}

func CreateDefaultCephConfig(context *clusterd.Context, cluster *ClusterInfo, runDir string, bluestore bool) *cephConfig {
	// extract a list of just the monitor names, which will populate the "mon initial members"
	// global config field
	monMembers := make([]string, len(cluster.Monitors))
	i := 0
	for _, monitor := range cluster.Monitors {
		monMembers[i] = monitor.Name
		i++
	}

	experimental := ""
	store := "filestore"
	if bluestore {
		experimental = "bluestore rocksdb"
		store = "bluestore"
	}

	cephLogLevel := logLevelToCephLogLevel(context.LogLevel)

	return &cephConfig{
		&GlobalConfig{
			EnableExperimental:     experimental,
			FSID:                   cluster.FSID,
			RunDir:                 runDir,
			MonMembers:             strings.Join(monMembers, " "),
			LogFile:                "/dev/stdout",
			MonClusterLogFile:      "/dev/stdout",
			PublicAddr:             context.NetworkInfo.PublicAddrIPv4,
			PublicNetwork:          context.NetworkInfo.PublicNetwork,
			ClusterAddr:            context.NetworkInfo.ClusterAddrIPv4,
			ClusterNetwork:         context.NetworkInfo.ClusterNetwork,
			MonKeyValueDb:          "rocksdb",
			DebugLogDefaultLevel:   cephLogLevel,
			DebugLogRadosLevel:     cephLogLevel,
			DebugLogMonLevel:       cephLogLevel,
			DebugLogOSDLevel:       cephLogLevel,
			DebugLogBluestoreLevel: cephLogLevel,
			DebugLogFilestoreLevel: cephLogLevel,
			DebugLogJournalLevel:   cephLogLevel,
			DebugLogLevelDBLevel:   cephLogLevel,
			FileStoreOmapBackend:   "rocksdb",
			OsdPgBits:              11,
			OsdPgpBits:             11,
			OsdPoolDefaultSize:     1,
			OsdPoolDefaultMinSize:  1,
			OsdPoolDefaultPgNum:    100,
			OsdPoolDefaultPgpNum:   100,
			OsdObjectStore:         store,
			RbdDefaultFeatures:     3,
			CrushtoolPath:          "",
			FatalSignalHandlers:    "false",
		},
	}
}

func createGlobalConfigFileSection(context *clusterd.Context, cluster *ClusterInfo, runDir string,
	bluestore bool, userConfig *cephConfig) (*ini.File, error) {

	var ceph *cephConfig

	if userConfig != nil {
		// use the user config since it was provided
		ceph = userConfig
	} else {
		ceph = CreateDefaultCephConfig(context, cluster, runDir, bluestore)
	}

	configFile := ini.Empty()
	err := ini.ReflectFrom(configFile, ceph)
	return configFile, err
}

func addClientConfigFileSection(configFile *ini.File, clientName, keyringPath string, settings map[string]string) error {
	s, err := configFile.NewSection(clientName)
	if err != nil {
		return err
	}

	if _, err := s.NewKey("keyring", keyringPath); err != nil {
		return err
	}

	for key, val := range settings {
		if _, err := s.NewKey(key, val); err != nil {
			return fmt.Errorf("failed to add key %s. %v", key, err)
		}
	}

	return nil
}

func addInitialMonitorsConfigFileSections(configFile *ini.File, cluster *ClusterInfo) error {
	// write the config for each individual monitor member of the cluster to the content buffer
	for _, mon := range cluster.Monitors {

		s, err := configFile.NewSection(fmt.Sprintf("mon.%s", mon.Name))
		if err != nil {
			return err
		}

		if _, err := s.NewKey("name", mon.Name); err != nil {
			return err
		}

		if _, err := s.NewKey("mon addr", mon.Endpoint); err != nil {
			return err
		}
	}

	return nil
}

func logLevelToCephLogLevel(logLevel capnslog.LogLevel) int {
	switch logLevel {
	case capnslog.CRITICAL:
	case capnslog.ERROR:
	case capnslog.WARNING:
		return -1
	case capnslog.NOTICE:
	case capnslog.INFO:
		return 0
	case capnslog.DEBUG:
		return 10
	case capnslog.TRACE:
		return 100
	}

	return 0
}
