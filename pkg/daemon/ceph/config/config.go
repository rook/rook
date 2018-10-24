/*
Copyright 2018 The Rook Authors. All rights reserved.

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

// Package config provides methods for creating and formatting Ceph configuration files for daemons.
package config

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/coreos/pkg/capnslog"
	"github.com/go-ini/ini"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/util"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "cephconfig")

// EtcCephDir is the default dir where Ceph stores its configs.
// This variable can be overridden with a temp dir for unit tests.
var EtcCephDir = "/etc/ceph"

// VarLibCephDir is the config dir used by Ceph daemons.
// This variable can be overridden with a temp dir for unit tests.
var VarLibCephDir = "/var/lib/ceph"

const (
	// AdminUsername is the fully qualified user name for the user able to connect to Ceph as admin.
	AdminUsername = "client.admin"

	// DefaultConfigFile is the default name of the file where Ceph stores its configs
	DefaultConfigFile = "ceph.conf"
	// DefaultKeyringFile is the default name of the file where Ceph stores its keyring info
	DefaultKeyringFile = "keyring"
)

// GlobalConfig represents the [global] sections of Ceph's config file.
type GlobalConfig struct {
	EnableExperimental       string `ini:"enable experimental unrecoverable data corrupting features,omitempty"`
	FSID                     string `ini:"fsid,omitempty"`
	RunDir                   string `ini:"run dir,omitempty"`
	MonMembers               string `ini:"mon initial members,omitempty"`
	MonHost                  string `ini:"mon host"`
	LogFile                  string `ini:"log file,omitempty"`
	MonClusterLogFile        string `ini:"mon cluster log file,omitempty"`
	PublicAddr               string `ini:"public addr,omitempty"`
	PublicNetwork            string `ini:"public network,omitempty"`
	ClusterAddr              string `ini:"cluster addr,omitempty"`
	ClusterNetwork           string `ini:"cluster network,omitempty"`
	MonKeyValueDb            string `ini:"mon keyvaluedb"`
	MonAllowPoolDelete       bool   `ini:"mon_allow_pool_delete"`
	MaxPgsPerOsd             int    `ini:"mon_max_pg_per_osd"`
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
	OsdObjectStore           string `ini:"osd objectstore,omitempty"`
	CrushLocation            string `ini:"crush location,omitempty"`
	RbdDefaultFeatures       int    `ini:"rbd_default_features,omitempty"`
	FatalSignalHandlers      string `ini:"fatal signal handlers"`
}

// CephConfig represents an entire Ceph config including all sections.
type CephConfig struct {
	*GlobalConfig `ini:"global,omitempty"`
}

// DefaultConfigFilePath returns the full path to Ceph's default config file
func DefaultConfigFilePath() string {
	return path.Join(EtcCephDir, DefaultConfigFile)
}

// DefaultKeyringFilePath returns the full path to Ceph's default keyring file
func DefaultKeyringFilePath() string {
	return path.Join(EtcCephDir, DefaultKeyringFile)
}

// NamespacedConfigDir returns the config dir that is namespaced to avoid collisions between data
// from different clusters (namespaces).
func NamespacedConfigDir(configDir, namespace string) string {
	return filepath.Join(configDir, namespace)
}

// DaemonRunDir returns the run dir for a daemon. Daemon type is one of mon, mgr, mds, rgw, osd.
// Daemon ID is often referenced by Rook as "name".
func DaemonRunDir(configDir, daemonType, daemonID string) string {
	var daemonDir string
	switch daemonType {
	case "mon":
		// mons follow the scheme "mon-<id>". Leave this as-is to support upgrades without requiring
		// extra upgrade logic.
		// To support legacy mons, do not prepend "mon-" to the daemon ID if the ID already
		// contains the string "mon". (e.g., legacy mon name = "rook-ceph-mon0")
		if strings.Contains(daemonID, "mon") {
			daemonDir = daemonID
			break
		}
		fallthrough // Use the same scheme for mon, mgr, mds, and rgw
	case "mgr", "mds", "rgw":
		// These daemons are stateless. If this parameter changes, there will be no ill effect, but
		// use same scheme as mons for consistency and for this scheme's readability.
		daemonDir = fmt.Sprintf("%s-%s", daemonType, daemonID)
	case "osd":
		// osds do not follow the naming scheme of the mon, as it does not have a dash between "osd"
		// and the ID. Leave this as-is to support upgrades without requiring extra upgrade logic.
		daemonDir = fmt.Sprintf("%s%s", daemonType, daemonID)
	default:
		// Should never occur during normal runtime and should quickly expose errors in development
		panic(fmt.Sprintf("unknown daemon type: %s", daemonType))
	}
	return path.Join(configDir, daemonDir)
}

// DaemonDataDir returns the data dir for a daemon. Daemon type is one of mon, mgr, mds, rgw, osd.
// Daemon ID is often referenced by Rook as "name".
func DaemonDataDir(configDir, daemonType, daemonID string) string {
	switch daemonType {
	case "mon":
		// mon data dir is not the run dir as it is with most daemons, it is a data dir in the run
		// dir. Leave this as-is to support upgrades without requiring extra upgrade logic.
		return path.Join(DaemonRunDir(configDir, daemonType, daemonID), "data")
	default: // mgr, mds, rgw, osd
		return DaemonRunDir(configDir, daemonType, daemonID)
	}
}

// DaemonKeyringFilePath returns the keyring file path for a daemon. Daemon type is one of mon, mgr,
// mds, rgw, osd. Daemon ID is often referenced by Rook as "name".
// This is a shortcut for saying that the keyring is in the daemon run dir.
func DaemonKeyringFilePath(configDir, daemonType, daemonID string) string {
	return filepath.Join(DaemonRunDir(configDir, daemonType, daemonID), DefaultKeyringFile)
}

// AdminKeyringFile returns the name of the admin keyring file.
func AdminKeyringFile() string {
	return fmt.Sprintf("%s.keyring", AdminUsername)
}

// GenerateAdminConnectionConfig calls GenerateAdminConnectionConfigWithSettings with no settings
// overridden.
func GenerateAdminConnectionConfig(context *clusterd.Context, cluster *ClusterInfo) error {
	return GenerateAdminConnectionConfigWithSettings(context, cluster, nil)
}

// GenerateAdminConnectionConfigWithSettings generates a Ceph config and keyring which will allow
// the daemon to connect as an admin. Default config file settings can be overridden by specifying
// some subset of settings.
func GenerateAdminConnectionConfigWithSettings(context *clusterd.Context, cluster *ClusterInfo, settings *CephConfig) error {
	root := NamespacedConfigDir(context.ConfigDir, cluster.Name)
	keyringFilePath := path.Join(root, AdminKeyringFile())
	err := writeKeyring(AdminKeyring(cluster), keyringFilePath)
	if err != nil {
		return fmt.Errorf("failed to write keyring to %s: %+v", keyringFilePath, err)
	}

	configPath := path.Join(root, DefaultConfigFile)
	err = GenerateConfigFile(context, cluster,
		configPath, AdminUsername, keyringFilePath, root, settings, nil)
	if err != nil {
		return fmt.Errorf("failed to write config to %s: %+v", configPath, err)
	}
	logger.Infof("generated admin config in %s", configPath)
	return nil
}

// GenerateConfigFile generates and writes a config file to disk.
// Params:
// - Config file path is the full path including file name for the config file to be written. Ceph
// daemons should use the default Ceph config location (/etc/ceph/ceph.conf), whereas the operator
// should use a location which will prevent collisions of multiple clusters
// (e.g., /var/lib/rook/<cluster-namespace>/ceph.conf)
func GenerateConfigFile(
	context *clusterd.Context,
	cluster *ClusterInfo,
	configFilePath, user, keyringFilePath, runDir string,
	globalConfig *CephConfig,
	clientSettings map[string]string,
) error {

	// create the config directory
	configDir := path.Dir(configFilePath)
	if err := os.MkdirAll(configDir, 0744); err != nil {
		logger.Warningf("failed to create config directory at %s: %+v", configDir, err)
	}

	configFile, err := createGlobalConfigFileSection(context, cluster, runDir, globalConfig)
	if err != nil {
		return fmt.Errorf("failed to create global config section, %+v", err)
	}

	qualifiedUser := getQualifiedUser(user)
	if err := addClientConfigFileSection(configFile, qualifiedUser, keyringFilePath, clientSettings); err != nil {
		return fmt.Errorf("failed to add admin client config section, %+v", err)
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
	logger.Infof("writing config file %s", configFilePath)
	if err := configFile.SaveTo(configFilePath); err != nil {
		return fmt.Errorf("failed to save config file %s. %+v", configFilePath, err)
	}

	util.WriteFileToLog(logger, configFilePath)

	return nil
}

// prepends "client." if a user namespace is not already specified
func getQualifiedUser(user string) string {
	if strings.Index(user, ".") == -1 {
		return fmt.Sprintf("client.%s", user)
	}

	return user
}

// CreateDefaultCephConfig creates a default ceph config file.
func CreateDefaultCephConfig(context *clusterd.Context, cluster *ClusterInfo, runDir string) *CephConfig {
	// extract a list of just the monitor names, which will populate the "mon initial members"
	// global config field
	monMembers := make([]string, len(cluster.Monitors))
	monHosts := make([]string, len(cluster.Monitors))
	i := 0
	for _, monitor := range cluster.Monitors {
		monMembers[i] = monitor.Name
		monHosts[i] = monitor.Endpoint
		i++
	}

	cephLogLevel := logLevelToCephLogLevel(context.LogLevel)

	return &CephConfig{
		GlobalConfig: &GlobalConfig{
			FSID: cluster.FSID,
			// run dir is /var/lib/ceph for daemons; operator/agent do not reference this field
			RunDir:                 runDir,
			MonMembers:             strings.Join(monMembers, " "),
			MonHost:                strings.Join(monHosts, ","),
			LogFile:                "/dev/stderr",
			MonClusterLogFile:      "/dev/stderr",
			PublicAddr:             context.NetworkInfo.PublicAddr,
			PublicNetwork:          context.NetworkInfo.PublicNetwork,
			ClusterAddr:            context.NetworkInfo.ClusterAddr,
			ClusterNetwork:         context.NetworkInfo.ClusterNetwork,
			MonKeyValueDb:          "rocksdb",
			MonAllowPoolDelete:     true,
			MaxPgsPerOsd:           1000,
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
			RbdDefaultFeatures:     3,
			FatalSignalHandlers:    "false",
		},
	}
}

// create a config file with global settings configured, and return an ini file
func createGlobalConfigFileSection(context *clusterd.Context, cluster *ClusterInfo, runDir string, userConfig *CephConfig) (*ini.File, error) {
	var ceph *CephConfig

	if userConfig != nil {
		// use the user config since it was provided
		ceph = userConfig
	} else {
		ceph = CreateDefaultCephConfig(context, cluster, runDir)
	}

	configFile := ini.Empty()
	err := ini.ReflectFrom(configFile, ceph)
	return configFile, err
}

// add client config to the ini file
func addClientConfigFileSection(configFile *ini.File, clientName, keyringFilePath string, settings map[string]string) error {
	s, err := configFile.NewSection(clientName)
	if err != nil {
		return err
	}

	if _, err := s.NewKey("keyring", keyringFilePath); err != nil {
		return err
	}

	for key, val := range settings {
		if _, err := s.NewKey(key, val); err != nil {
			return fmt.Errorf("failed to add key %s. %v", key, err)
		}
	}

	return nil
}

// convert a Rook log level to a corresponding Ceph log level
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
