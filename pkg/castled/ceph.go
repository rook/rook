package castled

import "github.com/quantum/castle/pkg/clusterd"

const (
	cephName         = "ceph"
	cephKey          = "/castle/services/ceph"
	cephInstanceName = "default"
	desiredKey       = "desired"
)

type clusterInfo struct {
	FSID          string
	MonitorSecret string
	AdminSecret   string
	Name          string
	Monitors      map[string]*CephMonitorConfig
}

func NewCephService(devices string, forceFormat bool, location *CrushLocation) *clusterd.ClusterService {
	return &clusterd.ClusterService{
		Name:   cephName,
		Leader: &cephLeader{},
		Agents: []clusterd.ServiceAgent{&monAgent{}, newOSDAgent(devices, forceFormat, location)},
	}
}
