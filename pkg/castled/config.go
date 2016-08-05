package castled

import (
	"strings"
)

type Config struct {
	EtcdURLs        []string
	PrivateIPv4     string
	MonName         string
	InitialMonitors []CephMonitorConfig
}

type CephMonitorConfig struct {
	Name     string
	Endpoint string
}

func NewConfig(etcdURLs, privateIPv4, monName, initMonitorNames string) Config {
	// caller should have provided a comma separated list of monitor names, split those into a
	// list/slice, then create a slice of CephMonitorConfig structs based off those names
	initMonNameSet := strings.Split(initMonitorNames, ",")
	initMonSet := make([]CephMonitorConfig, len(initMonNameSet))
	for i := range initMonNameSet {
		initMonSet[i] = CephMonitorConfig{Name: initMonNameSet[i]}
	}

	return Config{
		EtcdURLs:        strings.Split(etcdURLs, ","),
		PrivateIPv4:     privateIPv4,
		MonName:         monName,
		InitialMonitors: initMonSet,
	}
}
