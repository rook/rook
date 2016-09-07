package castled

import "github.com/quantum/clusterd/pkg/orchestrator"

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

func NewCephService(devices string, forceFormat bool) *orchestrator.ClusterService {
	return &orchestrator.ClusterService{
		Name:   cephName,
		Leader: newCephLeader(),
		Agents: []orchestrator.ServiceAgent{&monAgent{}, newOSDAgent(devices, forceFormat)},
	}
}
