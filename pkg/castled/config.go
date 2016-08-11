package castled

import (
	"strings"
)

type Config struct {
	ClusterName     string
	EtcdURLs        []string
	PrivateIPv4     string
	MonNames        []string
	InitialMonitors []CephMonitorConfig
	Devices         []string
}

type CephMonitorConfig struct {
	Name     string
	Endpoint string
}

func NewConfig(clusterName, etcdURLs, privateIPv4, monNames, initMonitorNames, devices string) Config {
	// caller should have provided a comma separated list of monitor names, split those into a
	// list/slice, then create a slice of CephMonitorConfig structs based off those names
	initMonNameSet := splitList(initMonitorNames)
	initMonSet := make([]CephMonitorConfig, len(initMonNameSet))
	for i := range initMonNameSet {
		initMonSet[i] = CephMonitorConfig{Name: initMonNameSet[i]}
	}

	return Config{
		ClusterName:     clusterName,
		EtcdURLs:        splitList(etcdURLs),
		PrivateIPv4:     privateIPv4,
		MonNames:        splitList(monNames),
		InitialMonitors: initMonSet,
		Devices:         splitList(devices),
	}
}

func splitList(list string) []string {
	if list == "" {
		return nil
	}

	return strings.Split(list, ",")
}
