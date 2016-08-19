package castled

import (
	"fmt"

	"github.com/quantum/clusterd/pkg/orchestrator"
)

const (
	cephKey = "/castle/ceph"
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
