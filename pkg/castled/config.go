package castled

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	etcd "github.com/coreos/etcd/client"
)

type Config struct {
	ClusterName     string
	EtcdClient      etcd.KeysAPI
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

func NewConfig(etcdClient etcd.KeysAPI, clusterName, privateIPv4, monNames, initMonitorNames, devices string, forceFormat bool) Config {
	// caller should have provided a comma separated list of monitor names, split those into a
	// list/slice, then create a slice of CephMonitorConfig structs based off those names
	initMonNameSet := splitList(initMonitorNames)
	initMonSet := make([]CephMonitorConfig, len(initMonNameSet))
	for i := range initMonNameSet {
		initMonSet[i] = CephMonitorConfig{Name: initMonNameSet[i]}
	}

	return Config{
		ClusterName:     clusterName,
		EtcdClient:      etcdClient,
		PrivateIPv4:     privateIPv4,
		MonNames:        splitList(monNames),
		InitialMonitors: initMonSet,
		Devices:         splitList(devices),
		ForceFormat:     forceFormat,
	}
}

func splitList(list string) []string {
	if list == "" {
		return nil
	}

	return strings.Split(list, ",")
}

func writeFile(filePath string, contentBuffer bytes.Buffer) error {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0744); err != nil {
		return fmt.Errorf("failed to create config file directory at %s: %+v", dir, err)
	}
	if err := ioutil.WriteFile(filePath, contentBuffer.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write config file to %s: %+v", filePath, err)
	}

	return nil
}

func writeGlobalConfigFileSection(contentBuffer *bytes.Buffer, cfg Config, c *clusterInfo, runDir string) error {
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
