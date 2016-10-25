package cephmgr

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-ini/ini"
	"github.com/rook/rook/pkg/cephmgr/client"
	"github.com/rook/rook/pkg/clusterd"
)

type CephMonitorConfig struct {
	Name     string `json:"name"`
	Endpoint string `json:"endpoint"`
}

type cephConfig struct {
	*cephGlobalConfig `ini:"global,omitempty"`
}

type cephGlobalConfig struct {
	EnableExperimental       string `ini:"enable experimental unrecoverable data corrupting features"`
	FSID                     string `ini:"fsid,omitempty"`
	RunDir                   string `ini:"run dir,omitempty"`
	MonMembers               string `ini:"mon initial members,omitempty"`
	LogFile                  string `ini:"log file,omitempty"`
	MonClusterLogFile        string `ini:"mon cluster log file,omitempty"`
	DebugLogDefaultLevel     int    `ini:"debug default"`
	DebugLogRadosLevel       int    `ini:"debug rados"`
	DebugLogMonLevel         int    `ini:"debug mon"`
	DebugLogBluestoreLevel   int    `ini:"debug bluestore"`
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
}

func ConnectToClusterAsAdmin(context *clusterd.Context, factory client.ConnectionFactory, cluster *ClusterInfo) (client.Connection, error) {
	if len(cluster.Monitors) == 0 {
		return nil, errors.New("no monitors")
	}
	// write the monitor keyring to disk
	monName := getFirstMonitor(cluster)
	if err := writeMonitorKeyring(context.ConfigDir, monName, cluster); err != nil {
		return nil, err
	}

	return connectToCluster(context, factory, cluster, getMonRunDirPath(context.ConfigDir, monName), "admin", getMonKeyringPath(context.ConfigDir, monName))
}

// get the path of a given monitor's config file
func getConfFilePath(root, clusterName string) string {
	return fmt.Sprintf("%s/%s.config", root, clusterName)
}

// generates and writes the monitor config file to disk
func generateConnectionConfigFile(context *clusterd.Context, cluster *ClusterInfo, pathRoot, user, keyringPath string) (string, error) {
	return generateConfigFile(context, cluster, pathRoot, user, keyringPath, false, nil, nil)
}

// generates and writes the monitor config file to disk
func generateConfigFile(context *clusterd.Context, cluster *ClusterInfo, pathRoot, user, keyringPath string, bluestore bool, userConfig *cephConfig, clientSettings map[string]string) (string, error) {
	if pathRoot == "" {
		pathRoot = getMonRunDirPath(context.ConfigDir, getFirstMonitor(cluster))
	}

	// create the config directory
	if err := os.MkdirAll(filepath.Dir(pathRoot), 0744); err != nil {
		fmt.Printf("failed to create config directory at %s: %+v", pathRoot, err)
	}

	configFile, err := createGlobalConfigFileSection(cluster, pathRoot, bluestore, userConfig)
	if err != nil {
		return "", fmt.Errorf("failed to create global config section, %+v", err)
	}

	if err := addClientConfigFileSection(configFile, getQualifiedUser(user), keyringPath, clientSettings); err != nil {
		return "", fmt.Errorf("failed to add admin client config section, %+v", err)
	}

	if err := addInitialMonitorsConfigFileSections(configFile, cluster); err != nil {
		return "", fmt.Errorf("failed to add initial monitor config sections, %+v", err)
	}

	// write the entire config to disk
	filePath := getConfFilePath(pathRoot, cluster.Name)
	if err := configFile.SaveTo(filePath); err != nil {
		return "", err
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

func getFirstMonitor(cluster *ClusterInfo) string {
	// Get the first monitor
	for _, m := range cluster.Monitors {
		return m.Name
	}

	return ""
}

// opens a connection to the cluster that can be used for management operations
func connectToCluster(context *clusterd.Context, factory client.ConnectionFactory, cluster *ClusterInfo, basePath, user, keyringPath string) (client.Connection, error) {
	log.Printf("connecting to ceph cluster %s with user %s", cluster.Name, user)

	confFilePath, err := generateConnectionConfigFile(context, cluster, basePath, user, keyringPath)
	if err != nil {
		return nil, fmt.Errorf("failed to generate config file: %v", err)
	}

	conn, err := factory.NewConnWithClusterAndUser(cluster.Name, getQualifiedUser(user))
	if err != nil {
		return nil, fmt.Errorf("failed to create rados connection for cluster %s and user %s: %+v", cluster.Name, user, err)
	}

	if err = conn.ReadConfigFile(confFilePath); err != nil {
		return nil, fmt.Errorf("failed to read config file for cluster %s: %+v", cluster.Name, err)
	}

	if err = conn.Connect(); err != nil {
		return nil, fmt.Errorf("failed to connect to cluster %s: %+v", cluster.Name, err)
	}

	return conn, nil
}

func createDefaultCephConfig(cluster *ClusterInfo, runDir string, bluestore bool) *cephConfig {
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

	return &cephConfig{
		&cephGlobalConfig{
			EnableExperimental:     experimental,
			FSID:                   cluster.FSID,
			RunDir:                 runDir,
			MonMembers:             strings.Join(monMembers, " "),
			LogFile:                "/dev/stdout",
			MonClusterLogFile:      "/dev/stdout",
			DebugLogDefaultLevel:   0,
			DebugLogRadosLevel:     0,
			DebugLogMonLevel:       0,
			DebugLogBluestoreLevel: 0,
			OsdPgBits:              11,
			OsdPgpBits:             11,
			OsdPoolDefaultSize:     1,
			OsdPoolDefaultMinSize:  1,
			OsdPoolDefaultPgNum:    100,
			OsdPoolDefaultPgpNum:   100,
			OsdObjectStore:         store,
			RbdDefaultFeatures:     3,
			CrushtoolPath:          "",
		},
	}
}

func createGlobalConfigFileSection(cluster *ClusterInfo, runDir string, bluestore bool, userConfig *cephConfig) (*ini.File, error) {
	var ceph *cephConfig

	if userConfig != nil {
		// use the user config since it was provided
		ceph = userConfig
	} else {
		ceph = createDefaultCephConfig(cluster, runDir, bluestore)
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
