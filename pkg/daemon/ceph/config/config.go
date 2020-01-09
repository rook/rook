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
	"net"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/coreos/pkg/capnslog"
	"github.com/go-ini/ini"
	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/daemon/ceph/client"
	cephutil "github.com/rook/rook/pkg/daemon/ceph/util"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "cephconfig")

const (
	// DefaultKeyringFile is the default name of the file where Ceph stores its keyring info
	DefaultKeyringFile = "keyring"
	// Msgr2port is the listening port of the messenger v2 protocol
	Msgr2port   = 3300
	msgr1Prefix = "v1:"
	msgr2Prefix = "v2:"
)

var (
	// DefaultConfigDir is the default dir where Ceph stores its configs. Can be overridden for unit
	// tests.
	DefaultConfigDir = "/etc/ceph"

	// DefaultConfigFile is the default name of the file where Ceph stores its configs. Can be
	// overridden for unit tests.
	DefaultConfigFile = "ceph.conf"
)

// GlobalConfig represents the [global] sections of Ceph's config file.
type GlobalConfig struct {
	EnableExperimental       string `ini:"enable experimental unrecoverable data corrupting features,omitempty"`
	FSID                     string `ini:"fsid,omitempty"`
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
	OsdPoolDefaultPgNum      int    `ini:"osd pool default pg num,omitempty"`
	OsdPoolDefaultPgpNum     int    `ini:"osd pool default pgp num,omitempty"`
	OsdMaxObjectNameLen      int    `ini:"osd max object name len,omitempty"`
	OsdMaxObjectNamespaceLen int    `ini:"osd max object namespace len,omitempty"`
	OsdObjectStore           string `ini:"osd objectstore,omitempty"`
	RbdDefaultFeatures       int    `ini:"rbd_default_features,omitempty"`
	FatalSignalHandlers      string `ini:"fatal signal handlers"`
}

// CephConfig represents an entire Ceph config including all sections.
type CephConfig struct {
	*GlobalConfig `ini:"global,omitempty"`
}

// DefaultConfigFilePath returns the full path to Ceph's default config file
func DefaultConfigFilePath() string {
	return path.Join(DefaultConfigDir, DefaultConfigFile)
}

// DefaultKeyringFilePath returns the full path to Ceph's default keyring file
func DefaultKeyringFilePath() string {
	return path.Join(DefaultConfigDir, DefaultKeyringFile)
}

// GetConfFilePath gets the path of a given cluster's config file
func GetConfFilePath(root, clusterName string) string {
	return fmt.Sprintf("%s/%s.config", root, clusterName)
}

// GenerateAdminConnectionConfig calls GenerateAdminConnectionConfigWithSettings with no settings
// overridden.
func GenerateAdminConnectionConfig(context *clusterd.Context, cluster *ClusterInfo) (string, error) {
	return GenerateAdminConnectionConfigWithSettings(context, cluster, nil)
}

// GenerateAdminConnectionConfigWithSettings generates a Ceph config and keyring which will allow
// the daemon to connect as an admin. Default config file settings can be overridden by specifying
// some subset of settings.
func GenerateAdminConnectionConfigWithSettings(context *clusterd.Context, cluster *ClusterInfo, settings *CephConfig) (string, error) {
	root := path.Join(context.ConfigDir, cluster.Name)
	keyringPath := path.Join(root, fmt.Sprintf("%s.keyring", client.AdminUsername))
	err := writeKeyring(AdminKeyring(cluster), keyringPath)
	if err != nil {
		return "", errors.Wrapf(err, "failed to write keyring to %s", root)
	}

	filePath, err := GenerateConfigFile(context, cluster, root, client.AdminUsername, keyringPath, settings, nil)
	if err != nil {
		return "", errors.Wrapf(err, "failed to write config to %s", root)
	}
	logger.Infof("generated admin config in %s", root)
	return filePath, nil
}

// GenerateConfigFile generates and writes a config file to disk.
func GenerateConfigFile(context *clusterd.Context, cluster *ClusterInfo, pathRoot, user, keyringPath string, globalConfig *CephConfig, clientSettings map[string]string) (string, error) {

	// create the config directory
	if err := os.MkdirAll(pathRoot, 0744); err != nil {
		logger.Warningf("failed to create config directory at %q. %v", pathRoot, err)
	}

	configFile, err := createGlobalConfigFileSection(context, cluster, globalConfig)
	if err != nil {
		return "", errors.Wrapf(err, "failed to create global config section")
	}

	qualifiedUser := getQualifiedUser(user)
	if err := addClientConfigFileSection(configFile, qualifiedUser, keyringPath, clientSettings); err != nil {
		return "", errors.Wrapf(err, "failed to add admin client config section")
	}

	// write the entire config to disk
	filePath := GetConfFilePath(pathRoot, cluster.Name)
	logger.Infof("writing config file %s", filePath)
	if err := configFile.SaveTo(filePath); err != nil {
		return "", errors.Wrapf(err, "failed to save config file %s", filePath)
	}

	return filePath, nil
}

// prepends "client." if a user namespace is not already specified
func getQualifiedUser(user string) string {
	if strings.Index(user, ".") == -1 {
		return fmt.Sprintf("client.%s", user)
	}

	return user
}

// CreateDefaultCephConfig creates a default ceph config file.
func CreateDefaultCephConfig(context *clusterd.Context, cluster *ClusterInfo) (*CephConfig, error) {

	cephVersionEnv := os.Getenv("ROOK_CEPH_VERSION")
	if cephVersionEnv != "" {
		v, err := cephver.ExtractCephVersion(cephVersionEnv)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to extract ceph version")
		}
		cluster.CephVersion = *v
	}

	// extract a list of just the monitor names, which will populate the "mon initial members"
	// global config field
	monMembers := make([]string, len(cluster.Monitors))
	monHosts := make([]string, len(cluster.Monitors))
	i := 0
	for _, monitor := range cluster.Monitors {
		monMembers[i] = monitor.Name
		monIP := cephutil.GetIPFromEndpoint(monitor.Endpoint)

		// This tries to detect the current port if the mon already exists
		// This basically handles the transition between monitors running on 6790 to msgr2
		// So whatever the previous monitor port was we keep it
		currentMonPort := cephutil.GetPortFromEndpoint(monitor.Endpoint)

		monPorts := [2]string{strconv.Itoa(int(Msgr2port)), strconv.Itoa(int(currentMonPort))}
		msgr1Endpoint := net.JoinHostPort(monIP, monPorts[1])

		// Mimic daemons like OSD won't be able to parse this, so only the operator should get this config
		// they will fail with
		// 		unable to parse addrs in 'v1:10.104.92.199:6790,v1:10.110.137.107:6790,v1:10.102.38.86:6790'
		// 		server name not found: v1:10.104.92.199:6790 (Name or service not known)
		// 		2019-04-25 10:31:08.614 7f5971aae1c0 -1 monclient: get_monmap_and_config cannot identify monitors to contact
		// 		2019-04-25 10:31:08.614 7f5971aae1c0 -1 monclient: get_monmap_and_config cannot identify monitors to contact
		// 		failed to fetch mon config (--no-mon-config to skip)
		// The operator always fails this test since it does not have the env var 'ROOK_CEPH_VERSION'
		podName := os.Getenv("POD_NAME")
		if cluster.CephVersion.IsAtLeastNautilus() {
			monHosts[i] = msgr1Prefix + msgr1Endpoint
		} else if podName != "" && strings.Contains(podName, "operator") {
			// This is an operator and its version is always based on Nautilus
			// so it knows how to parse both msgr1 and msgr2 syntax
			prefix := msgrPrefix(currentMonPort)
			monHosts[i] = prefix + msgr1Endpoint
		} else {
			// This is not the operator, it's an OSD and its Ceph version is before Nautilus
			monHosts[i] = msgr1Endpoint
		}
		i++
	}

	cephLogLevel := logLevelToCephLogLevel(context.LogLevel)

	conf := &CephConfig{
		GlobalConfig: &GlobalConfig{
			FSID:                   cluster.FSID,
			MonMembers:             strings.Join(monMembers, " "),
			MonHost:                strings.Join(monHosts, ","),
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
			OsdPoolDefaultPgNum:    100,
			OsdPoolDefaultPgpNum:   100,
			RbdDefaultFeatures:     3,
			FatalSignalHandlers:    "false",
		},
	}

	// Everything before 14.2.1
	// These new flags control Ceph's daemon logging behavior to files
	// By default we set them to False so no logs get written on file
	// However they can be activated at any time via the centralized config store
	if !cluster.CephVersion.IsAtLeast(cephver.CephVersion{Major: 14, Minor: 2, Extra: 1}) {
		conf.LogFile = "/dev/stderr"
		conf.MonClusterLogFile = "/dev/stderr"
	}

	return conf, nil
}

// create a config file with global settings configured, and return an ini file
func createGlobalConfigFileSection(context *clusterd.Context, cluster *ClusterInfo, userConfig *CephConfig) (*ini.File, error) {

	var ceph *CephConfig

	if userConfig != nil {
		// use the user config since it was provided
		ceph = userConfig
	} else {
		var err error
		ceph, err = CreateDefaultCephConfig(context, cluster)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create default ceph config")
		}
	}

	configFile := ini.Empty()
	err := ini.ReflectFrom(configFile, ceph)
	return configFile, err
}

// add client config to the ini file
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
			return errors.Wrapf(err, "failed to add key %s", key)
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

func msgrPrefix(currentMonPort int32) string {
	// Some installation might only be listening on v2, so let's set the prefix accordingly
	if currentMonPort == Msgr2port {
		return msgr2Prefix
	}

	return msgr1Prefix

}
