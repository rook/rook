package etcdmgr

import (
	"log"

	"github.com/quantum/castle/pkg/clusterd"
	"github.com/quantum/castle/pkg/etcdmgr/bootstrap"
)

const (
	etcdMgrName = "etcdmgr"
)

// NewEtcdMgrService creates a new etcdmgr service
func NewEtcdMgrService(token string) *clusterd.ClusterService {
	log.Println("creating instances of etcdMgrLeader and etcdMgrAgent")

	return &clusterd.ClusterService{
		Name:   etcdMgrName,
		Leader: &etcdMgrLeader{context: &bootstrap.Context{ClusterToken: token}},
		Agents: []clusterd.ServiceAgent{
			&etcdMgrAgent{context: &bootstrap.Context{ClusterToken: token}, etcdFactory: &bootstrap.EmbeddedEtcdFactory{}},
		},
	}
}
