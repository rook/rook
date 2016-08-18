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
	Name          string
}

func startOSDs(cluster *clusterInfo, executor proc.Executor) ([]*exec.Cmd, error) {
	user := "client.admin"
	config, err := getCephConnectionConfig(cluster)
	if err != nil {
		return nil, err
	}

	adminConn, err := connectToCluster(cluster.Name, user, config)
	if err != nil {
		return nil, err
	}
	defer adminConn.Shutdown()

	// create/start an OSD for each of the specified devices
	devices := []string{}
	if len(devices) > 0 {
		osdProcs, err := createOSDs(adminConn, cluster, executor)
		if err != nil {
			return nil, fmt.Errorf("failed to create OSDs: %+v", err)
		}

		return osdProcs, nil
	}

	return nil, nil
}
