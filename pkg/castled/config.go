package castled

import (
	"fmt"
	"strings"

	"github.com/go-ini/ini"
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
	MonMembers            string `ini:"mon members,omitempty"`
	OsdPgBits             int    `ini:"osd pg bits,omitempty"`
	OsdPgpBits            int    `ini:"osd pgp bits,omitempty"`
	OsdPoolDefaultSize    int    `ini:"osd pool default size,omitempty"`
	OsdPoolDefaultMinSize int    `ini:"osd pool default min size,omitempty"`
	OsdPoolDefaultPgNum   int    `ini:"osd pool default pg num,omitempty"`
	OsdPoolDefaultPgpNum  int    `ini:"osd pool default pgp num,omitempty"`
	RbdDefaultFeatures    int    `ini:"rbd_default_features,omitempty"`
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

func getCephConnectionConfig(cluster *clusterInfo) (string, error) {
	return "config", nil
}
