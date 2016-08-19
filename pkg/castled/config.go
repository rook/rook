package castled

import (
	"fmt"
	"strings"

	"github.com/go-ini/ini"
	"github.com/quantum/castle/pkg/util"
)

type Config struct {
	ClusterName     string
	EtcdURLs        []string
	PrivateIPv4     string
	MonNames        []string
	InitialMonitors []CephMonitorConfig
	Devices         []string
	ForceFormat     bool
}

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
	MonInitialMembers     string `ini:"mon initial members,omitempty"`
	OsdPgBits             int    `ini:"osd pg bits,omitempty"`
	OsdPgpBits            int    `ini:"osd pgp bits,omitempty"`
	OsdPoolDefaultSize    int    `ini:"osd pool default size,omitempty"`
	OsdPoolDefaultMinSize int    `ini:"osd pool default min size,omitempty"`
	OsdPoolDefaultPgNum   int    `ini:"osd pool default pg num,omitempty"`
	OsdPoolDefaultPgpNum  int    `ini:"osd pool default pgp num,omitempty"`
	RbdDefaultFeatures    int    `ini:"rbd_default_features,omitempty"`
}

func NewConfig(clusterName, etcdURLs, privateIPv4, monNames, initMonitorNames, devices string, forceFormat bool) Config {
	// caller should have provided a comma separated list of monitor names, split those into a
	// list/slice, then create a slice of CephMonitorConfig structs based off those names
	initMonNameSet := util.SplitList(initMonitorNames)
	initMonSet := make([]CephMonitorConfig, len(initMonNameSet))
	for i := range initMonNameSet {
		initMonSet[i] = CephMonitorConfig{Name: initMonNameSet[i]}
	}

	return Config{
		ClusterName:     clusterName,
		EtcdURLs:        util.SplitList(etcdURLs),
		PrivateIPv4:     privateIPv4,
		MonNames:        util.SplitList(monNames),
		InitialMonitors: initMonSet,
		Devices:         util.SplitList(devices),
		ForceFormat:     forceFormat,
	}
}

func createGlobalConfigFileSection(cfg Config, c clusterInfo, runDir string) (*ini.File, error) {
	// extract a list of just the monitor names, which will populate the "mon initial members"
	// global config field
	initialMonMembers := make([]string, len(cfg.InitialMonitors))
	for i := range cfg.InitialMonitors {
		initialMonMembers[i] = cfg.InitialMonitors[i].Name
	}

	ceph := &cephConfig{
		&cephGlobalConfig{
			FSID:                  c.FSID,
			RunDir:                runDir,
			MonInitialMembers:     strings.Join(initialMonMembers, " "),
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

func addInitialMonitorsConfigFileSections(configFile *ini.File, cfg Config) error {
	// write the config for each individual monitor member of the cluster to the content buffer
	for i := range cfg.InitialMonitors {
		mon := cfg.InitialMonitors[i]

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
