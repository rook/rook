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

// Package client provides methods for creating and formatting Ceph configuration files for daemons.
package client

import (
	"fmt"
	"net"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/rook/rook/pkg/operator/k8sutil"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/coreos/pkg/capnslog"
	"github.com/go-ini/ini"
	"github.com/pkg/errors"
	"github.com/rook/rook/pkg/clusterd"
	cephutil "github.com/rook/rook/pkg/daemon/ceph/util"
	cephver "github.com/rook/rook/pkg/operator/ceph/version"
)

var logger = capnslog.NewPackageLogger("github.com/rook/rook", "cephclient")

const (
	// DefaultKeyringFile is the default name of the file where Ceph stores its keyring info
	DefaultKeyringFile = "keyring"
	// Msgr2port is the listening port of the messenger v2 protocol
	Msgr2port = 3300
	// Msgr1port is the listening port of the messenger v1 protocol
	Msgr1port = 6789
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
	FSID       string `ini:"fsid,omitempty"`
	MonMembers string `ini:"mon initial members,omitempty"`
	MonHost    string `ini:"mon host"`
}

// CephConfig represents an entire Ceph config including all sections.
type CephConfig struct {
	*GlobalConfig `ini:"global,omitempty"`
}

// DefaultConfigFilePath returns the full path to Ceph's default config file
func DefaultConfigFilePath() string {
	return path.Join(DefaultConfigDir, DefaultConfigFile)
}

// getConfFilePath gets the path of a given cluster's config file
func getConfFilePath(root, clusterName string) string {
	return fmt.Sprintf("%s/%s.config", root, clusterName)
}

// GenerateConnectionConfig calls GenerateConnectionConfigWithSettings with no settings
// overridden.
func GenerateConnectionConfig(context *clusterd.Context, cluster *ClusterInfo) (string, error) {
	return GenerateConnectionConfigWithSettings(context, cluster, nil)
}

// GenerateConnectionConfigWithSettings generates a Ceph config and keyring which will allow
// the daemon to connect. Default config file settings can be overridden by specifying
// some subset of settings.
func GenerateConnectionConfigWithSettings(context *clusterd.Context, clusterInfo *ClusterInfo, settings *CephConfig) (string, error) {
	root := path.Join(context.ConfigDir, clusterInfo.Namespace)
	keyringPath := path.Join(root, fmt.Sprintf("%s.keyring", clusterInfo.CephCred.Username))
	err := writeKeyring(CephKeyring(clusterInfo.CephCred), keyringPath)
	if err != nil {
		return "", errors.Wrapf(err, "failed to write keyring %q to %s", clusterInfo.CephCred.Username, root)
	}

	filePath, err := generateConfigFile(context, clusterInfo, root, keyringPath, settings, nil)
	if err != nil {
		return "", errors.Wrapf(err, "failed to write config to %s", root)
	}
	logger.Infof("generated admin config in %s", root)
	return filePath, nil
}

// generateConfigFile generates and writes a config file to disk.
func generateConfigFile(context *clusterd.Context, clusterInfo *ClusterInfo, pathRoot, keyringPath string, globalConfig *CephConfig, clientSettings map[string]string) (string, error) {
	// create the config directory
	if err := os.MkdirAll(pathRoot, 0o744); err != nil {
		return "", errors.Wrapf(err, "failed to create config directory at %q", pathRoot)
	}

	configFile, err := createGlobalConfigFileSection(context, clusterInfo, globalConfig)
	if err != nil {
		return "", errors.Wrap(err, "failed to create global config section")
	}

	if err := mergeDefaultConfigWithRookConfigOverride(context, clusterInfo, configFile); err != nil {
		return "", errors.Wrapf(err, "failed to merge global config with %q", k8sutil.ConfigOverrideName)
	}

	qualifiedUser := getQualifiedUser(clusterInfo.CephCred.Username)
	if err := addClientConfigFileSection(configFile, qualifiedUser, keyringPath, clientSettings); err != nil {
		return "", errors.Wrap(err, "failed to add admin client config section")
	}

	// write the entire config to disk
	filePath := getConfFilePath(pathRoot, clusterInfo.Namespace)
	logger.Infof("writing config file %s", filePath)
	if err := configFile.SaveTo(filePath); err != nil {
		return "", errors.Wrapf(err, "failed to save config file %s", filePath)
	}

	return filePath, nil
}

func mergeDefaultConfigWithRookConfigOverride(clusterdContext *clusterd.Context, clusterInfo *ClusterInfo, configFile *ini.File) error {
	cm, err := clusterdContext.Clientset.CoreV1().ConfigMaps(clusterInfo.Namespace).Get(clusterInfo.Context, k8sutil.ConfigOverrideName, metav1.GetOptions{})
	if err != nil {
		if !kerrors.IsNotFound(err) {
			return errors.Wrapf(err, "failed to read configmap %q", k8sutil.ConfigOverrideName)
		}
		return nil
	}

	config, ok := cm.Data["config"]
	if !ok || config == "" {
		logger.Debugf("No ceph configuration override to merge as %q configmap is empty", k8sutil.ConfigOverrideName)
		return nil
	}

	if err := configFile.Append([]byte(config)); err != nil {
		return errors.Wrapf(err, "failed to load config data from %q", k8sutil.ConfigOverrideName)
	}

	// Remove any debug message setting from the config file
	// Debug messages will be printed on stdout, rendering the output of each command unreadable, especially json output
	// This call is idempotent and will not fail if the debug message is not present
	configFile.Section("global").DeleteKey("debug_ms")
	configFile.Section("global").DeleteKey("debug ms")

	return nil
}

// prepends "client." if a user namespace is not already specified
func getQualifiedUser(user string) string {
	if !strings.Contains(user, ".") {
		return fmt.Sprintf("client.%s", user)
	}

	return user
}

// CreateDefaultCephConfig creates a default ceph config file.
func CreateDefaultCephConfig(context *clusterd.Context, clusterInfo *ClusterInfo) (*CephConfig, error) {
	cephVersionEnv := os.Getenv("ROOK_CEPH_VERSION")
	if cephVersionEnv != "" {
		v, err := cephver.ExtractCephVersion(cephVersionEnv)
		if err != nil {
			return nil, errors.Wrap(err, "failed to extract ceph version")
		}
		clusterInfo.CephVersion = *v
	}

	// extract a list of just the monitor names, which will populate the "mon initial members"
	// and "mon hosts" global config field
	monMembers, monHosts := PopulateMonHostMembers(clusterInfo)

	conf := &CephConfig{
		GlobalConfig: &GlobalConfig{
			FSID:       clusterInfo.FSID,
			MonMembers: strings.Join(monMembers, " "),
			MonHost:    strings.Join(monHosts, ","),
		},
	}

	return conf, nil
}

// create a config file with global settings configured, and return an ini file
func createGlobalConfigFileSection(context *clusterd.Context, clusterInfo *ClusterInfo, userConfig *CephConfig) (*ini.File, error) {
	var ceph *CephConfig

	if userConfig != nil {
		// use the user config since it was provided
		ceph = userConfig
	} else {
		var err error
		ceph, err = CreateDefaultCephConfig(context, clusterInfo)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create default ceph config")
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

// PopulateMonHostMembers extracts a list of just the monitor names, which will populate the "mon initial members"
// and "mon hosts" global config field
func PopulateMonHostMembers(clusterInfo *ClusterInfo) ([]string, []string) {
	var monMembers []string
	var monHosts []string

	for _, monitor := range clusterInfo.AllMonitors() {
		if monitor.OutOfQuorum {
			logger.Warningf("skipping adding mon %q to config file, detected out of quorum", monitor.Name)
			continue
		}
		monMembers = append(monMembers, monitor.Name)
		monIP := cephutil.GetIPFromEndpoint(monitor.Endpoint)
		// Detect the current port if the mon already exists
		// so the same msgr1 port can be preserved if needed (6789 or 6790)
		currentMonPort := cephutil.GetPortFromEndpoint(monitor.Endpoint)

		if currentMonPort == Msgr2port {
			msgr2Endpoint := net.JoinHostPort(monIP, strconv.Itoa(int(Msgr2port)))
			monHosts = append(monHosts, "[v2:"+msgr2Endpoint+"]")
		} else {
			msgr2Endpoint := net.JoinHostPort(monIP, strconv.Itoa(int(Msgr2port)))
			msgr1Endpoint := net.JoinHostPort(monIP, strconv.Itoa(int(currentMonPort)))
			monHosts = append(monHosts, "[v2:"+msgr2Endpoint+",v1:"+msgr1Endpoint+"]")
		}
	}

	return monMembers, monHosts
}

// WriteCephConfig writes the ceph config so ceph commands can be executed
func WriteCephConfig(context *clusterd.Context, clusterInfo *ClusterInfo) error {
	// create the ceph.conf with the default settings
	cephConfig, err := CreateDefaultCephConfig(context, clusterInfo)
	if err != nil {
		return errors.Wrap(err, "failed to create default ceph config")
	}

	// write the latest config to the config dir
	confFilePath, err := GenerateConnectionConfigWithSettings(context, clusterInfo, cephConfig)
	if err != nil {
		return errors.Wrap(err, "failed to write connection config")
	}
	src, err := os.ReadFile(filepath.Clean(confFilePath))
	if err != nil {
		return errors.Wrap(err, "failed to copy connection config to /etc/ceph. failed to read the connection config")
	}
	err = os.WriteFile(DefaultConfigFilePath(), src, 0o600)
	if err != nil {
		return errors.Wrapf(err, "failed to copy connection config to /etc/ceph. failed to write %q", DefaultConfigFilePath())
	}
	dst, err := os.ReadFile(DefaultConfigFilePath())
	if err == nil {
		logger.Debugf("config file @ %s:\n%s", DefaultConfigFilePath(), dst)
	} else {
		logger.Warningf("wrote and copied config file but failed to read it back from %s for logging. %v", DefaultConfigFilePath(), err)
	}
	return nil
}
