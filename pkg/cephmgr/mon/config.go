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
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/coreos/pkg/capnslog"
	"github.com/go-ini/ini"
	"github.com/rook/rook/pkg/cephmgr/client"
	"github.com/rook/rook/pkg/clusterd"
)

type CephMonitorConfig struct {
	Name     string `json:"name"`
	Endpoint string `json:"endpoint"`
}

type cephConfig struct {
	*GlobalConfig `ini:"global,omitempty"`
}

// for ceph debug log settings see http://docs.ceph.com/docs/master/rados/troubleshooting/log-and-debug/#subsystem-log-and-debug-settings
type GlobalConfig struct {
	EnableExperimental        string `ini:"enable experimental unrecoverable data corrupting features"`
	FSID                      string `ini:"fsid,omitempty"`
	RunDir                    string `ini:"run dir,omitempty"`
	MonMembers                string `ini:"mon initial members,omitempty"`
	LogFile                   string `ini:"log file,omitempty"`
	MonClusterLogFile         string `ini:"mon cluster log file,omitempty"`
	MonKeyValueDb             string `ini:"mon keyvaluedb"`
	DebugLogAsokLevel         int    `ini:"debug asok"`
	DebugLogAuthLevel         int    `ini:"debug auth"`
	DebugLogBlueFSLevel       int    `ini:"debug bluefs"`
	DebugLogBluestoreLevel    int    `ini:"debug bluestore"`
	DebugLogBufferLevel       int    `ini:"debug buffer"`
	DebugLogClientLevel       int    `ini:"debug client"`
	DebugLogContextLevel      int    `ini:"debug context"`
	DebugLogCrushLevel        int    `ini:"debug crush"`
	DebugLogDefaultLevel      int    `ini:"debug default"`
	DebugLogFilerLevel        int    `ini:"debug filer"`
	DebugLogFilestoreLevel    int    `ini:"debug filestore"`
	DebugLogFinisherLevel     int    `ini:"debug finisher"`
	DebugLogHeartbeatmapLevel int    `ini:"debug heartbeatmap"`
	DebugLogJournalLevel      int    `ini:"debug journal"`
	DebugLogJournalerLevel    int    `ini:"debug journaler"`
	DebugLogLevelDBLevel      int    `ini:"debug leveldb"`
	DebugLogLockdepLevel      int    `ini:"debug lockdep"`
	DebugLogMDSLevel          int    `ini:"debug mds"`
	DebugLogMDSBalancerLevel  int    `ini:"debug mds balancer"`
	DebugLogMDSLockerLevel    int    `ini:"debug mds locker"`
	DebugLogMDSLogLevel       int    `ini:"debug mds log"`
	DebugLogMDSLogExpireLevel int    `ini:"debug mds log expire"`
	DebugLogMDSMigratorLevel  int    `ini:"debug mds migrator"`
	DebugLogMonLevel          int    `ini:"debug mon"`
	DebugLogMoncLevel         int    `ini:"debug monc"`
	DebugLogMSLevel           int    `ini:"debug ms"`
	DebugLogObjClassLevel     int    `ini:"debug objclass"`
	DebugLogObjectCacherLevel int    `ini:"debug objectcacher"`
	DebugLogObjecterLevel     int    `ini:"debug objecter"`
	DebugLogOptrackerLevel    int    `ini:"debug optracker"`
	DebugLogOSDLevel          int    `ini:"debug osd"`
	DebugLogPaxosLevel        int    `ini:"debug paxos"`
	DebugLogPerfcounterLevel  int    `ini:"debug perfcounter"`
	DebugLogRadosLevel        int    `ini:"debug rados"`
	DebugLogRBDLevel          int    `ini:"debug rbd"`
	DebugLogRGWLevel          int    `ini:"debug rgw"`
	DebugLogThrottleLevel     int    `ini:"debug throttle"`
	DebugLogTimerLevel        int    `ini:"debug timer"`
	DebugLogTPLevel           int    `ini:"debug tp"`
	FileStoreOmapBackend      string `ini:"filestore_omap_backend"`
	OsdPgBits                 int    `ini:"osd pg bits,omitempty"`
	OsdPgpBits                int    `ini:"osd pgp bits,omitempty"`
	OsdPoolDefaultSize        int    `ini:"osd pool default size,omitempty"`
	OsdPoolDefaultMinSize     int    `ini:"osd pool default min size,omitempty"`
	OsdPoolDefaultPgNum       int    `ini:"osd pool default pg num,omitempty"`
	OsdPoolDefaultPgpNum      int    `ini:"osd pool default pgp num,omitempty"`
	OsdMaxObjectNameLen       int    `ini:"osd max object name len,omitempty"`
	OsdMaxObjectNamespaceLen  int    `ini:"osd max object namespace len,omitempty"`
	OsdObjectStore            string `ini:"osd objectstore"`
	RbdDefaultFeatures        int    `ini:"rbd_default_features,omitempty"`
	CrushtoolPath             string `ini:"crushtool"`
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

func ConnectToClusterAsAdmin(context *clusterd.Context, factory client.ConnectionFactory, cluster *ClusterInfo) (client.Connection, error) {
	if len(cluster.Monitors) == 0 {
		return nil, errors.New("no monitors")
	}
	// write the monitor keyring to disk
	monName := getFirstMonitor(cluster)
	keyringPath := getMonKeyringPath(context.ConfigDir, monName)
	if err := writeMonitorKeyring(monName, cluster, keyringPath); err != nil {
		return nil, err
	}

	return ConnectToCluster(context, factory, cluster, getMonRunDirPath(context.ConfigDir, monName),
		"admin", getMonKeyringPath(context.ConfigDir, monName))
}

// get the path of a given monitor's config file
func GetConfFilePath(root, clusterName string) string {
	return fmt.Sprintf("%s/%s.config", root, clusterName)
}

func GenerateTempConfigFiles(context *clusterd.Context, cluster *ClusterInfo) (string, string, string, error) {
	root := path.Join(context.ConfigDir, "tmp")
	keyring := path.Join(root, "keyring")
	user := getFirstMonitor(cluster)
	err := writeMonitorKeyring(user, cluster, keyring)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to write keyring to %s", root)
	}

	configFile, err := GenerateConfigFile(context, cluster, root, user, keyring, false, nil, nil)
	return configFile, keyring, user, err
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

	configFile, err := createGlobalConfigFileSection(cluster, pathRoot, context.LogLevel, bluestore, userConfig)
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
	filePath := GetConfFilePath(pathRoot, cluster.Name)
	logger.Debugf("writing config file %s", filePath)
	if err := configFile.SaveTo(filePath); err != nil {
		return "", fmt.Errorf("failed to save config file %s. %+v", filePath, err)
	}

	return filePath, nil
}

// create a keyring for access to the cluster, with the desired set of privileges
func CreateKeyring(conn client.Connection, username, keyringPath string, access []string, generateKeyring func(string) string) error {
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
	key, err := client.AuthGetOrCreateKey(conn, username, access)
	if err != nil {
		return fmt.Errorf("failed to get or create auth key for %s. %+v", username, err)
	}

	// write the keyring to disk
	keyringDir := filepath.Dir(keyringPath)
	if err := os.MkdirAll(keyringDir, 0744); err != nil {
		return fmt.Errorf("failed to create keyring dir at %s: %+v", keyringDir, err)
	}

	keyring := generateKeyring(key)
	logger.Debugf("Writing keyring to: %s", keyringPath)
	if err := ioutil.WriteFile(keyringPath, []byte(keyring), 0644); err != nil {
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

// opens a connection to the cluster that can be used for management operations
func ConnectToCluster(context *clusterd.Context, factory client.ConnectionFactory, cluster *ClusterInfo,
	basePath, user, keyringPath string) (client.Connection, error) {

	logger.Infof("connecting to ceph cluster %s with user %s", cluster.Name, user)

	confFilePath, err := GenerateConnectionConfigFile(context, cluster, basePath, user, keyringPath)
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

func CreateDefaultCephConfig(cluster *ClusterInfo, runDir string, logLevel capnslog.LogLevel, bluestore bool) *cephConfig {
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
		&GlobalConfig{
			EnableExperimental:        experimental,
			FSID:                      cluster.FSID,
			RunDir:                    runDir,
			MonMembers:                strings.Join(monMembers, " "),
			LogFile:                   "/dev/stdout",
			MonClusterLogFile:         "/dev/stdout",
			MonKeyValueDb:             "rocksdb",
			DebugLogDefaultLevel:      logLevelToCephLogLevel(logLevel),
			DebugLogAsokLevel:         getEnvLogLevel("ASOK", 1),
			DebugLogAuthLevel:         getEnvLogLevel("AUTH", 1),
			DebugLogBlueFSLevel:       getEnvLogLevel("BLUEFS", 0),
			DebugLogBluestoreLevel:    getEnvLogLevel("BLUESTORE", 0),
			DebugLogBufferLevel:       getEnvLogLevel("BUFFER", 0),
			DebugLogClientLevel:       getEnvLogLevel("CLIENT", 0),
			DebugLogContextLevel:      getEnvLogLevel("CONTEXT", 0),
			DebugLogCrushLevel:        getEnvLogLevel("CRUSH", 1),
			DebugLogFilerLevel:        getEnvLogLevel("FILER", 0),
			DebugLogFilestoreLevel:    getEnvLogLevel("FILESTORE", 1),
			DebugLogFinisherLevel:     getEnvLogLevel("FINISHER", 1),
			DebugLogHeartbeatmapLevel: getEnvLogLevel("HEARTBEAT_MAP", 1),
			DebugLogJournalLevel:      getEnvLogLevel("JOURNAL", 1),
			DebugLogJournalerLevel:    getEnvLogLevel("JOURNALER", 0),
			DebugLogLevelDBLevel:      getEnvLogLevel("LEVELDB", 0),
			DebugLogLockdepLevel:      getEnvLogLevel("LOCKDEP", 0),
			DebugLogMDSLevel:          getEnvLogLevel("MDS", 1),
			DebugLogMDSBalancerLevel:  getEnvLogLevel("MDS_BALANCER", 1),
			DebugLogMDSLockerLevel:    getEnvLogLevel("MDS_LOCKER", 1),
			DebugLogMDSLogLevel:       getEnvLogLevel("MDS_LOG", 1),
			DebugLogMDSLogExpireLevel: getEnvLogLevel("MDS_LOG_EXPIRE", 1),
			DebugLogMDSMigratorLevel:  getEnvLogLevel("MDS_MIGRATOR", 1),
			DebugLogMonLevel:          getEnvLogLevel("MON", 1),
			DebugLogMoncLevel:         getEnvLogLevel("MONC", 0),
			DebugLogMSLevel:           getEnvLogLevel("MS", 0),
			DebugLogObjClassLevel:     getEnvLogLevel("OBJ_CLASS", 0),
			DebugLogObjectCacherLevel: getEnvLogLevel("OBJECT_CACHER", 0),
			DebugLogObjecterLevel:     getEnvLogLevel("OBJECTER", 0),
			DebugLogOptrackerLevel:    getEnvLogLevel("OPTRACKER", 0),
			DebugLogOSDLevel:          getEnvLogLevel("OSD", 0),
			DebugLogPaxosLevel:        getEnvLogLevel("PAXOS", 0),
			DebugLogPerfcounterLevel:  getEnvLogLevel("PERF_COUNTER", 1),
			DebugLogRadosLevel:        getEnvLogLevel("RADOS", 0),
			DebugLogRBDLevel:          getEnvLogLevel("RBD", 0),
			DebugLogRGWLevel:          getEnvLogLevel("RGW", 1),
			DebugLogThrottleLevel:     getEnvLogLevel("THROTTLE", 1),
			DebugLogTimerLevel:        getEnvLogLevel("TIMER", 0),
			DebugLogTPLevel:           getEnvLogLevel("TP", 0),
			FileStoreOmapBackend:      "rocksdb",
			OsdPgBits:                 11,
			OsdPgpBits:                11,
			OsdPoolDefaultSize:        1,
			OsdPoolDefaultMinSize:     1,
			OsdPoolDefaultPgNum:       100,
			OsdPoolDefaultPgpNum:      100,
			OsdObjectStore:            store,
			RbdDefaultFeatures:        3,
			CrushtoolPath:             "",
		},
	}
}

func createGlobalConfigFileSection(cluster *ClusterInfo, runDir string, logLevel capnslog.LogLevel, bluestore bool, userConfig *cephConfig) (*ini.File, error) {
	var ceph *cephConfig

	if userConfig != nil {
		// use the user config since it was provided
		ceph = userConfig
	} else {
		ceph = CreateDefaultCephConfig(cluster, runDir, logLevel, bluestore)
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

// translate an environment variable to a log level
func getEnvLogLevel(variable string, defaultVal int) int {
	level, err := strconv.Atoi(os.Getenv("ROOKD_LOG_" + variable))

	// if the variable is not found, there is a parse error, or the value is out of bounds
	// we will return the default level
	if err != nil || level < -1 || level > 100 {
		return defaultVal
	}
	return level
}
