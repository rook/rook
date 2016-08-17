package castled

import (
	"fmt"
	"os/exec"

	"github.com/quantum/castle/pkg/proc"
)

const (
	cephKey = "/castle/ceph"

	globalConfigTemplate = `
[global]
	fsid=%s
	run dir=%s
	mon initial members = %s
`
	adminClientConfigTemplate = `
[client.admin]
    keyring=%s
`
)

type clusterInfo struct {
	FSID          string
	MonitorSecret string
	AdminSecret   string
}

func Bootstrap(cfg Config, executor proc.Executor) ([]*exec.Cmd, error) {

	// Start the monitors
	cluster, procs, err := startMonitors(cfg)
	if err != nil {
		return nil, err
	}

	// Start the OSDs
	osdProcs, err := startOSDs(cfg, cluster, executor)
	if err != nil {
		return nil, err
	}

	procs = append(procs, osdProcs...)

	return procs, nil
}

func startOSDs(cfg Config, cluster *clusterInfo, executor proc.Executor) ([]*exec.Cmd, error) {
	user := "client.admin"
	adminConn, err := connectToCluster(cfg.ClusterName, user, getMonConfFilePath(cfg.MonNames[0]))
	if err != nil {
		return nil, err
	}
	defer adminConn.Shutdown()

	// create/start an OSD for each of the specified devices
	if len(cfg.Devices) > 0 {
		osdProcs, err := createOSDs(adminConn, cfg, cluster, executor)
		if err != nil {
			return nil, fmt.Errorf("failed to create OSDs: %+v", err)
		}

		return osdProcs, nil
	}

	return nil, nil
}
