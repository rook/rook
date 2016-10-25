package etcdmgr

import (
	"log"

	"github.com/rook/rook/pkg/clusterd"
	"github.com/rook/rook/pkg/etcdmgr/bootstrap"
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
