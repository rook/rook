package castled

import (
	"bytes"
	"fmt"
	"strings"

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

func writeGlobalConfigFileSection(contentBuffer *bytes.Buffer, cfg Config, c clusterInfo, runDir string) error {
	// extract a list of just the monitor names, which will populate the "mon initial members"
	// global config field
	initialMonMembers := make([]string, len(cfg.InitialMonitors))
	for i := range cfg.InitialMonitors {
		initialMonMembers[i] = cfg.InitialMonitors[i].Name
	}

	// write the global config section to the content buffer
	_, err := contentBuffer.WriteString(fmt.Sprintf(
		globalConfigTemplate,
		c.FSID,
		runDir,
		strings.Join(initialMonMembers, " ")))
	return err
}

func writeInitialMonitorsConfigFileSections(contentBuffer *bytes.Buffer, cfg Config) error {
	// write the config for each individual monitor member of the cluster to the content buffer
	for i := range cfg.InitialMonitors {
		mon := cfg.InitialMonitors[i]
		_, err := contentBuffer.WriteString(fmt.Sprintf(monitorConfigTemplate, mon.Name, mon.Name, mon.Endpoint))
		if err != nil {
			return err
		}
	}

	return nil
}
