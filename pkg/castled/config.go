package castled

import (
	"bytes"
	"fmt"
	"strings"
)

type CephMonitorConfig struct {
	Name     string
	Endpoint string
}

func writeGlobalConfigFileSection(contentBuffer *bytes.Buffer, cluster *clusterInfo, runDir string) error {
	// extract a list of just the monitor names, which will populate the "mon initial members"
	// global config field
	monMembers := make([]string, len(cluster.Monitors))
	i := 0
	for _, monitor := range cluster.Monitors {
		monMembers[i] = monitor.Name
		i++
	}

	// write the global config section to the content buffer
	_, err := contentBuffer.WriteString(fmt.Sprintf(
		globalConfigTemplate,
		cluster.FSID,
		runDir,
		strings.Join(monMembers, " ")))
	return err
}

func writeMonitorsConfigFileSections(contentBuffer *bytes.Buffer, monitors map[string]*CephMonitorConfig) error {
	// write the config for each individual monitor member of the cluster to the content buffer
	for _, mon := range monitors {
		_, err := contentBuffer.WriteString(fmt.Sprintf(monitorConfigTemplate, mon.Name, mon.Name, mon.Endpoint))
		if err != nil {
			return err
		}
	}

	return nil
}

func getCephConnectionConfig(cluster *clusterInfo) (string, error) {
	return "config", nil
}
