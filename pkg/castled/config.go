package castled

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-ini/ini"
	"github.com/quantum/castle/pkg/cephd"
)

type CephMonitorConfig struct {
	Name     string
	Endpoint string
}

type cephConfig struct {
	*cephGlobalConfig `ini:"global,omitempty"`
}

type cephGlobalConfig struct {
	FSID                  string `ini:"fsid,omitempty"`
	RunDir                string `ini:"run dir,omitempty"`
	MonMembers            string `ini:"mon initial members,omitempty"`
	OsdPgBits             int    `ini:"osd pg bits,omitempty"`
	OsdPgpBits            int    `ini:"osd pgp bits,omitempty"`
	OsdPoolDefaultSize    int    `ini:"osd pool default size,omitempty"`
	OsdPoolDefaultMinSize int    `ini:"osd pool default min size,omitempty"`
	OsdPoolDefaultPgNum   int    `ini:"osd pool default pg num,omitempty"`
	OsdPoolDefaultPgpNum  int    `ini:"osd pool default pgp num,omitempty"`
	RbdDefaultFeatures    int    `ini:"rbd_default_features,omitempty"`
}

// get the path of a given monitor's config file
func getConfFilePath(root, clusterName string) string {
	return fmt.Sprintf("%s/%s.config", root, clusterName)
}

// generates and writes the monitor config file to disk
func generateConfigFile(cluster *clusterInfo, pathRoot, user, keyringPath string) (string, error) {
	if pathRoot == "" {
		pathRoot = getMonRunDirPath(getFirstMonitor(cluster))
	}

	// create the config directory
	if err := os.MkdirAll(filepath.Dir(pathRoot), 0744); err != nil {
		fmt.Printf("failed to create config directory at %s: %+v", pathRoot, err)
	}

	configFile, err := createGlobalConfigFileSection(cluster, pathRoot)
	if err != nil {
		return "", fmt.Errorf("failed to create global config section, %+v", err)
	}

	if err := addClientConfigFileSection(configFile, getQualifiedUser(user), keyringPath); err != nil {
		return "", fmt.Errorf("failed to add %s admin client config section, %+v", err)
	}

	if err := addInitialMonitorsConfigFileSections(configFile, cluster); err != nil {
		return "", fmt.Errorf("failed to add %s initial monitor config sections, %+v", err)
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

func getFirstMonitor(cluster *clusterInfo) string {
	// Get the first monitor
	for _, m := range cluster.Monitors {
		return m.Name
	}

	return ""
}

func connectToClusterAsAdmin(cluster *clusterInfo) (*cephd.Conn, error) {

	// write the monitor keyring to disk
	monName := getFirstMonitor(cluster)
	if err := writeMonitorKeyring(monName, cluster); err != nil {
		return nil, err
	}

	return connectToCluster(cluster, getQualifiedUser("admin"), getMonKeyringPath(monName))
}

// opens a connection to the cluster that can be used for management operations
func connectToCluster(cluster *clusterInfo, user, keyringPath string) (*cephd.Conn, error) {
	log.Printf("connecting to ceph cluster %s with user %s", cluster.Name, user)

	confFilePath, err := generateConfigFile(cluster, "", user, keyringPath)
	if err != nil {
		return nil, fmt.Errorf("failed to generate config file: %v", err)
	}

	conn, err := cephd.NewConnWithClusterAndUser(cluster.Name, getQualifiedUser(user))
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

func createGlobalConfigFileSection(cluster *clusterInfo, runDir string) (*ini.File, error) {
	// extract a list of just the monitor names, which will populate the "mon initial members"
	// global config field
	monMembers := make([]string, len(cluster.Monitors))
	i := 0
	for _, monitor := range cluster.Monitors {
		monMembers[i] = monitor.Name
		i++
	}

	ceph := &cephConfig{
		&cephGlobalConfig{
			FSID:                  cluster.FSID,
			RunDir:                runDir,
			MonMembers:            strings.Join(monMembers, " "),
			OsdPgBits:             11,
			OsdPgpBits:            11,
			OsdPoolDefaultSize:    1,
			OsdPoolDefaultMinSize: 1,
			OsdPoolDefaultPgNum:   100,
			OsdPoolDefaultPgpNum:  100,
			RbdDefaultFeatures:    3,
		},
	}

	configFile := ini.Empty()
	err := ini.ReflectFrom(configFile, ceph)
	return configFile, err
}

func addClientConfigFileSection(configFile *ini.File, clientName, keyringPath string) error {
	s, err := configFile.NewSection(clientName)
	if err != nil {
		return err
	}

	if _, err := s.NewKey("keyring", keyringPath); err != nil {
		return err
	}

	return nil
}

func addInitialMonitorsConfigFileSections(configFile *ini.File, cluster *clusterInfo) error {
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
