package castled

import (
	"fmt"

	"github.com/quantum/clusterd/pkg/orchestrator"
)

const (
	cephKey = "/castle/ceph"

	globalConfigTemplate = `
[global]
	fsid=%s
	run dir=%s
	mon initial members = %s
	
	osd pg bits = 11
	osd pgp bits = 11
	osd pool default size = 1
	osd pool default min size = 1
	osd pool default pg num = 100
	osd pool default pgp num = 100

	rbd_default_features = 3
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
	Monitors      map[string]*CephMonitorConfig
}

// Get the root ceph service key
func GetRootCephServiceKey(applied bool) string {
	var prefix string
	if applied {
		prefix = orchestrator.ClusterConfigAppliedKey
	} else {
		prefix = orchestrator.ClusterConfigDesiredKey
	}

	return fmt.Sprintf("%s/services/ceph/default", prefix)
}
